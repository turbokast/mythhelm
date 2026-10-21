#!/usr/bin/env python3
"""Test-only pseudo-terminal driver for tests/e2e (standard library only).

Usage: tui_pty.py COLS ROWS DIR ARGV...

Starts ARGV as the session leader of a new pseudo-terminal sized COLS x ROWS,
working in DIR, with this process's environment, and plays the terminal for
it: output is read continuously so the child never blocks, and the two
startup queries a terminal answers (OSC 11 background colour and DSR cursor
position) are answered as a dark terminal would. No shell is involved.

The Go test drives it with one JSON object per stdin line and reads one JSON
object per stdout line. The first stdout line is {"ok": true, "pid": N}.

  {"op": "send", "data": S}                     write S to the terminal
  {"op": "wait", "text": S, "from": N,           wait until the visible text
   "timeout": T, "resend": S, "every": T}        after offset N contains S,
                                                 writing resend first and then
                                                 every `every` seconds
  {"op": "mark"}                                 current visible-text offset
  {"op": "wait_view_closed", "timeout": T}       wait for an alternate-screen exit
  {"op": "alive"}                                whether the child still runs
  {"op": "exit", "timeout": T}                   wait for the child and the end
                                                 of its terminal output; "post"
                                                 is the text printed after the
                                                 view last closed

Every reply carries "ok"; a failure carries "error" and the visible tail.
End of stdin kills a still-running child and reaps it before exiting.
"""

import codecs
import fcntl
import json
import os
import pty
import select
import signal
import struct
import sys
import termios
import threading
import time

QUERY_BACKGROUND = b"\x1b]11;?"
QUERY_CURSOR = b"\x1b[6n"
ANSWER_BACKGROUND = b"\x1b]11;rgb:0000/0000/0000\x1b\\"
ANSWER_CURSOR = b"\x1b[1;1R"
LEAVE_ALT_SCREEN = "?1049l"


class Screen:
    """Strips control sequences from the output stream incrementally.

    `text` holds the visible characters in output order; `views` holds the
    text offsets at which the child left the alternate screen, so a test can
    separate what the view drew from what the CLI printed after it closed.
    """

    def __init__(self):
        self.text = []
        self.size = 0
        self.views = []
        self.state = "text"
        self.params = ""

    def feed(self, chunk):
        out = []
        for ch in chunk:
            if self.state == "text":
                if ch == "\x1b":
                    self.state = "esc"
                elif ch in "\r\x07\x00":
                    continue
                else:
                    out.append(ch)
            elif self.state == "esc":
                if ch == "[":
                    self.state, self.params = "csi", ""
                elif ch == "]":
                    self.state = "osc"
                elif ch in "()":
                    self.state = "charset"
                else:
                    self.state = "text"
            elif self.state == "charset":
                self.state = "text"
            elif self.state == "csi":
                if "\x40" <= ch <= "\x7e":
                    if self.params + ch == LEAVE_ALT_SCREEN:
                        self.views.append(self.size + len(out))
                    self.state = "text"
                else:
                    self.params += ch
            elif self.state == "osc":
                if ch == "\x07":
                    self.state = "text"
                elif ch == "\x1b":
                    self.state = "osc-esc"
            elif self.state == "osc-esc":
                self.state = "text" if ch == "\\" else "osc"
        if out:
            self.text.append("".join(out))
            self.size += len(out)

    def visible(self):
        joined = "".join(self.text)
        self.text = [joined]
        return joined


class Terminal:
    def __init__(self, cols, rows, workdir, argv):
        pid, fd = pty.fork()
        if pid == 0:
            try:
                fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
                os.chdir(workdir)
                os.execv(argv[0], argv)
            finally:
                os._exit(127)
        self.pid, self.fd = pid, fd
        self.cond = threading.Condition()
        self.screen = Screen()
        self.raw = b""
        self.eof = False
        self.status = None
        self.write_lock = threading.Lock()
        self.reader = threading.Thread(target=self.read, daemon=True)
        self.reader.start()

    def write(self, data):
        with self.write_lock:
            view = memoryview(data)
            while view:
                n = os.write(self.fd, view)
                view = view[n:]

    def read(self):
        decoder = codecs.getincrementaldecoder("utf-8")("replace")
        scanned = 0
        while True:
            try:
                chunk = os.read(self.fd, 65536)
            except OSError:
                chunk = b""
            if not chunk:
                with self.cond:
                    self.eof = True
                    self.cond.notify_all()
                return
            with self.cond:
                self.raw += chunk
                # Answer each complete query once, in output order.
                while True:
                    window = self.raw[scanned:]
                    hits = [(window.find(q), q, a) for q, a in
                            ((QUERY_BACKGROUND, ANSWER_BACKGROUND), (QUERY_CURSOR, ANSWER_CURSOR))]
                    hits = [h for h in hits if h[0] >= 0]
                    if not hits:
                        scanned = max(scanned, len(self.raw) - 8)
                        break
                    at, query, answer = min(hits)
                    scanned += at + len(query)
                    self.write(answer)
                self.screen.feed(decoder.decode(chunk))
                self.cond.notify_all()

    def reap(self, block):
        if self.status is not None:
            return True
        pid, status = os.waitpid(self.pid, 0 if block else os.WNOHANG)
        if pid == 0:
            return False
        self.status = os.waitstatus_to_exitcode(status)
        return True

    def tail(self):
        return self.screen.visible()[-4000:]


def wait_for(term, cmd):
    text = cmd["text"]
    start = cmd.get("from", 0)
    deadline = time.monotonic() + cmd["timeout"]
    resend = cmd.get("resend")
    every = cmd.get("every", 0.5)
    next_send = time.monotonic()
    with term.cond:
        while True:
            visible = term.screen.visible()
            at = visible.find(text, start)
            if at >= 0:
                return {"ok": True, "at": at + len(text)}
            now = time.monotonic()
            if term.eof:
                return {"ok": False, "error": "terminal closed before %r" % text, "tail": term.tail()}
            if now >= deadline:
                return {"ok": False, "error": "timed out waiting for %r" % text, "tail": term.tail()}
            if resend is not None and now >= next_send:
                term.write(resend.encode())
                next_send = now + every
            wake = deadline if resend is None else min(deadline, next_send)
            term.cond.wait(max(0.0, wake - now))


def wait_view_closed(term, cmd):
    deadline = time.monotonic() + cmd["timeout"]
    with term.cond:
        while not term.screen.views:
            now = time.monotonic()
            if term.eof or now >= deadline:
                return {"ok": False, "error": "view did not close", "tail": term.tail()}
            term.cond.wait(deadline - now)
        return {"ok": True, "at": term.screen.views[-1]}


def wait_exit(term, cmd):
    deadline = time.monotonic() + cmd["timeout"]
    # Bounded poll: waitpid has no timeout of its own.
    while not term.reap(False):
        if time.monotonic() >= deadline:
            return {"ok": False, "error": "child still running", "tail": term.tail()}
        time.sleep(0.05)
    with term.cond:
        while not term.eof:
            now = time.monotonic()
            if now >= deadline:
                return {"ok": False, "error": "terminal output did not end", "tail": term.tail()}
            term.cond.wait(deadline - now)
        visible = term.screen.visible()
        post = visible[term.screen.views[-1]:] if term.screen.views else ""
        return {"ok": True, "code": term.status, "output": visible, "post": post}


def main():
    cols, rows, workdir, argv = int(sys.argv[1]), int(sys.argv[2]), sys.argv[3], sys.argv[4:]
    term = Terminal(cols, rows, workdir, argv)
    reply(ok=True, pid=term.pid)
    for line in sys.stdin:
        cmd = json.loads(line)
        op = cmd["op"]
        if op == "send":
            term.write(cmd["data"].encode())
            reply(ok=True)
        elif op == "wait":
            reply(**wait_for(term, cmd))
        elif op == "mark":
            with term.cond:
                reply(ok=True, at=len(term.screen.visible()))
        elif op == "wait_view_closed":
            reply(**wait_view_closed(term, cmd))
        elif op == "alive":
            reply(ok=True, alive=not term.reap(False))
        elif op == "exit":
            reply(**wait_exit(term, cmd))
        else:
            reply(ok=False, error="unknown op %r" % op)
    if not term.reap(False):
        os.kill(term.pid, signal.SIGKILL)
        term.reap(True)


def reply(**fields):
    sys.stdout.write(json.dumps(fields) + "\n")
    sys.stdout.flush()


if __name__ == "__main__":
    main()
