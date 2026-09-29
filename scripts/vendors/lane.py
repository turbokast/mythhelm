#!/usr/bin/env python3
"""lane.py: the implementer lane. A vendor CLI implements one task of a spec in a
fresh linked worktree, confined by bubblewrap, and the result comes back as a
patch. The calling Claude agent reviews the patch, runs every gate and commits it
under its own identity; the vendor never commits, pushes or touches the caller's
tree.

Entry points: scripts/vendors/codex-implement.sh, scripts/vendors/muse-implement.sh

    lane.py --vendor codex|muse --spec-dir DIR --task N [--from DIR]
            [--include-uncommitted] [--tier trivial|standard|complex]
            [--prompt-file FILE] [--keep]

Order (every refusal happens before a worktree exists or anything is spent):
  1. the vendor and its lane are opted in                       disabled
  2. the task parses and lists Files                            task-unreadable
  3. no listed file is protected (lanes.protected_paths)        protected-path
  4. Linux, and a root-owned, non-writable bwrap that starts    sandbox-unavailable
  5. CLI, version pin, signed in                                cli-missing | ...
  6. one call counted against the daily caps                    over-quota
  7. base = the --from checkout's HEAD, plus its uncommitted non-ignored changes
     with --include-uncommitted (built in a temporary index; the checkout is not
     touched); worktree <main>/.claude/worktrees/<id> on branch vendor-lane/<id>
  8. the vendor runs inside bwrap, under the tier's timeout      timeout | vendor-error
  9. the envelope commits whatever the vendor left, then checks
     the diff: no symlinks or submodules, nothing protected,
     nothing outside the task's Files                            symlink | protected-path |
                                                                 out-of-scope | no-changes
 10. the patch (git diff --binary) is written under
     .claude/data/vendor-calls/<id>/change.patch                patch-too-large
The worktree and branch are removed afterwards unless --keep.

Confinement (bwrap): the sandbox root holds only what is bound into it, so the
caller's checkout (its .env files included), SSH keys, other checkouts and other
vendors' state are absent. Bound: read-only /usr, /etc and the merged-usr links;
fresh empty tmpfs at /tmp and $HOME; the worktree read-write;
the git common dir and the worktree's .git file read-only (the vendor cannot
commit or repoint git); the vendor's install directory read-only and its OWN
native state directory read-write (it authenticates itself; the envelope never
reads or copies a credential); the Go toolchain and module cache read-only with
a per-run build cache; a scrubbed environment (env -i plus an allowlist); a new
pid namespace torn down with the run. The network namespace is shared, because
the vendor reaches its API over it. Without bwrap there is no run.

Output: one JSON response line (schemas/response.schema.json) with patch_path,
base, files_changed and the attribution trailer the committing agent adds.
Exit 0 on every run path, 64 on a usage error.
"""

from __future__ import annotations

import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import vendors as V  # noqa: E402

PROMPT = os.path.join(V.SCRIPT_DIR, "prompts", "implement.md")
IDENTITY = {"GIT_AUTHOR_NAME": "vendor-lane", "GIT_AUTHOR_EMAIL": "vendor-lane@example.com",
            "GIT_COMMITTER_NAME": "vendor-lane", "GIT_COMMITTER_EMAIL": "vendor-lane@example.com"}
SAFE_GIT = ("-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "core.fsmonitor=false")


# ---- sandbox -----------------------------------------------------------------------

def resolve_bwrap() -> str:
    """A root-owned, not group/other-writable bwrap that can start the namespaces the
    lane uses. A user-owned bwrap earlier on PATH could run its argv unconfined."""
    if not sys.platform.startswith("linux"):
        raise V.Unavailable("sandbox-unavailable", "the implementer lane needs Linux and bubblewrap")
    found = shutil.which("bwrap")
    if not found:
        raise V.Unavailable("sandbox-unavailable", "bwrap is not installed")
    real = os.path.realpath(found)
    st = os.stat(real)
    if st.st_uid != 0 or st.st_mode & (stat.S_IWGRP | stat.S_IWOTH):
        raise V.Unavailable("sandbox-unavailable", f"{real} is not root-owned or is group/other-writable")
    probe = [real, "--die-with-parent", "--unshare-pid", "--unshare-ipc", "--new-session",
             *system_args(), "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp", "--", "/usr/bin/true"]
    if subprocess.run(probe, capture_output=True, timeout=30).returncode != 0:
        raise V.Unavailable("sandbox-unavailable", "bwrap cannot start a user-namespace sandbox on this host")
    return real


def system_args() -> list[str]:
    out = ["--ro-bind", "/usr", "/usr"]
    for d in ("bin", "sbin", "lib", "lib32", "lib64", "libx32"):
        p = f"/{d}"
        if os.path.islink(p):
            out += ["--symlink", os.readlink(p), p]
        elif os.path.isdir(p):
            out += ["--ro-bind", p, p]
    return out


def vendor_state_dirs(vendor: str) -> list[str]:
    """The vendor's own native state, bound read-write so it can authenticate and
    refresh itself. The envelope never opens these directories."""
    home = os.path.expanduser("~")
    if vendor == "codex":
        return [os.environ.get("CODEX_HOME") or os.path.join(home, ".codex")]
    cfg = os.environ.get("XDG_CONFIG_HOME") or os.path.join(home, ".config")
    data = os.environ.get("XDG_DATA_HOME") or os.path.join(home, ".local", "share")
    return [os.path.join(cfg, "muse"), os.path.join(data, "muse")]


def install_root(vendor: str, cli_real: str) -> str:
    if vendor == "codex":
        pkg = os.path.dirname(os.path.dirname(cli_real))
        if os.path.isfile(os.path.join(pkg, "codex-package.json")):
            return pkg
    return os.path.dirname(cli_real)


def go_toolchain() -> tuple[list[str], dict]:
    go = shutil.which("go")
    if not go:
        return [], {}
    r = subprocess.run([go, "env", "GOROOT", "GOMODCACHE"], capture_output=True, text=True, timeout=30)
    if r.returncode != 0:
        return [], {}
    goroot, modcache = (r.stdout.split("\n") + ["", ""])[:2]
    binds = [p for p in (goroot, modcache) if p and os.path.isdir(p)]
    env = {"GOMODCACHE": modcache} if modcache else {}
    return binds, env | {"GOROOT": goroot, "_GOBIN": os.path.join(goroot, "bin")}


def sandbox_argv(bwrap, worktree, run_dir, ro, rw) -> list[str]:
    argv = [bwrap, "--die-with-parent", "--unshare-pid", "--unshare-ipc", "--unshare-uts",
            "--unshare-cgroup-try", "--new-session", *system_args(),
            "--ro-bind", "/etc", "/etc", "--ro-bind-try", "/run/systemd/resolve", "/run/systemd/resolve",
            "--proc", "/proc", "--dev", "/dev"]
    # The new root holds nothing but what is bound below. /tmp and $HOME are fresh,
    # empty and writable, so the vendor has scratch space and a home of its own.
    home = os.path.realpath(os.path.expanduser("~"))
    for m in sorted({"/tmp", home} - {"/"}, key=len):
        argv += ["--tmpfs", m]
    common = V.git(worktree, "rev-parse", "--path-format=absolute", "--git-common-dir").stdout.strip()
    binds = [("--ro-bind", os.path.realpath(p)) for p in ro if p and os.path.exists(p)]
    binds += [("--bind", os.path.realpath(p)) for p in rw if p and os.path.exists(p)]
    binds += [("--ro-bind", os.path.realpath(common)), ("--bind", os.path.realpath(worktree)),
              ("--bind", os.path.realpath(run_dir))]
    seen = set()
    for flag, path in binds:
        if (flag, path) not in seen:
            seen.add((flag, path))
            argv += [flag, path, path]
    gitfile = os.path.join(worktree, ".git")
    if os.path.isfile(gitfile) and not os.path.islink(gitfile):
        argv += ["--ro-bind", gitfile, gitfile]
    return argv + ["--chdir", os.path.realpath(worktree), "--"]


# ---- base and worktree -------------------------------------------------------------

def base_commit(src: str, include_uncommitted: bool) -> str:
    head = V.git(src, "rev-parse", "HEAD").stdout.strip()
    if not include_uncommitted:
        return head
    with tempfile.TemporaryDirectory(prefix="vendor-lane-index-") as tmp:
        env = {"GIT_INDEX_FILE": os.path.join(tmp, "index"), **IDENTITY}
        V.git(src, "read-tree", "HEAD", env=env)
        V.git(src, "add", "-A", "--", ".", ":(exclude).claude/data", env=env)
        tree = V.git(src, "write-tree", env=env).stdout.strip()
        if tree == V.git(src, "rev-parse", "HEAD^{tree}").stdout.strip():
            return head
        return V.git(src, "commit-tree", tree, "-p", head, "-m", "lane base: uncommitted changes", env=env).stdout.strip()


def commit_residual(worktree: str) -> None:
    V.git(worktree, *SAFE_GIT, "add", "-A", env=IDENTITY)
    if V.git(worktree, "diff", "--cached", "--quiet", check=False).returncode != 0:
        V.git(worktree, *SAFE_GIT, "commit", "-q", "--no-verify", "-m", "vendor lane: changes", env=IDENTITY)


def check_diff(pol, worktree, base, listed) -> list[str]:
    raw = V.git(worktree, "diff", "--raw", "--no-renames", base, "HEAD").stdout
    paths = []
    for line in raw.splitlines():
        meta, _, path = line.partition("\t")
        modes = meta.split()
        if len(modes) >= 2 and ("120000" in modes[:2] or "160000" in modes[:2]):
            raise V.Unavailable("symlink", f"{path} is a symlink or submodule")
        paths.append(path)
    if not paths:
        raise V.Unavailable("no-changes", "the vendor left no changes")
    hits = V.protected_hits(pol, paths, listed=listed)
    if hits:
        raise V.Unavailable("protected-path", ", ".join(hits[:5]))
    outside = [p for p in paths if not V.path_in(p, listed)]
    if outside:
        raise V.Unavailable("out-of-scope", "outside the task's Files: " + ", ".join(outside[:5]))
    return paths


# ---- vendor argv ------------------------------------------------------------------------

def vendor_argv(vendor, cli, conf, tier, prompt_path, answer_path, worktree):
    if vendor == "codex":
        argv = [cli, "exec", "--json", "--sandbox", "workspace-write", "--ephemeral", "--ignore-user-config",
                "-c", f"model_reasoning_effort={tier.get('effort', 'medium')}", "-o", answer_path, "-"]
        if conf.get("model"):
            argv[2:2] = ["-m", conf["model"]]
        return argv, prompt_path
    argv = [cli, "exec", "--json", "--prompt-file", prompt_path, "--workspace", worktree, "--trust-workspace",
            "--approval-mode", "never", "--sandbox-network", "restricted", "--disable-shell",
            "--disable-web-tools", "--max-model-steps", str(tier.get("max_steps", 80)),
            "--reasoning-effort", tier.get("effort", "medium")]
    if conf.get("model"):
        argv[2:2] = ["--model", conf["model"]]
    return argv, None


def vendor_failed(vendor, stdout: bytes) -> str | None:
    events = []
    for line in stdout.decode("utf-8", "replace").splitlines():
        try:
            e = json.loads(line)
        except ValueError:
            continue
        if isinstance(e, dict):
            events.append(e)
    if vendor == "codex":
        if any(e.get("type") == "turn.failed" for e in events):
            return "the vendor reported a failed turn"
        if not any(e.get("type") == "turn.completed" for e in events):
            return "no turn.completed event"
        return None
    if not any(e.get("payload_type") == "run.terminal.completed" for e in events):
        return "no completed run record"
    return None


# ---- the lane ------------------------------------------------------------------------------

def lane(args) -> dict:
    started = time.monotonic()
    vendor = args.vendor
    call_id = V.new_id(vendor, "lane")
    extra = {"patch_path": None, "base": None, "files_changed": [], "attribution": None}
    worktree = branch = main = None
    try:
        pol = V.load_policy()
        vconf = V.vendor_conf(pol, vendor)
        extra["attribution"] = vconf.get("attribution")
        if not V.lanes_enabled(pol, vendor):
            raise V.Unavailable("disabled", f"the {vendor} lane is not enabled for this checkout (knowledge/vendors.md)")
        try:
            task = V.parse_task(args.spec_dir, args.task)
        except (OSError, KeyError) as err:
            raise V.Unavailable("task-unreadable", str(err)) from err
        if not task["files"]:
            raise V.Unavailable("task-unreadable", f"task {args.task} lists no Files")
        tiers = pol.get("lanes", {}).get("tiers", {})
        tier_name = args.tier or V.budget_word(task["budget"]) or "standard"
        if tier_name not in tiers:
            raise V.UsageError(f"unknown tier {tier_name!r}; known: {', '.join(sorted(tiers))}")
        hits = V.protected_hits(pol, task["files"], listed=task["files"])
        if hits:
            raise V.Unavailable("protected-path", "the task lists protected files: " + ", ".join(hits[:5]))
        src = V.toplevel(os.path.abspath(args.from_dir or os.getcwd()))
        if not src:
            raise V.UsageError("--from is not inside a git work tree")
        main = V.main_checkout(src)
        bwrap = resolve_bwrap()
        info = V.check_available(pol, vendor)
        V.reserve(pol, vendor, "lane", pol.get("lanes", {}).get("daily_cap"))
    except V.Unavailable as err:
        return V.unavailable(vendor, "lane", "lane", call_id, err, started=started, **extra)

    call_dir = os.path.join(V.data_dir(), "vendor-calls", call_id)
    os.makedirs(call_dir, exist_ok=True)
    run_dir = tempfile.mkdtemp(prefix="vendor-lane-run-")
    try:
        base = base_commit(src, args.include_uncommitted)
        extra["base"] = base
        worktree = os.path.join(main, ".claude", "worktrees", call_id)
        branch = f"vendor-lane/{call_id}"
        V.git(main, "worktree", "add", "-q", "-b", branch, worktree, base)

        if args.prompt_file:
            with open(args.prompt_file, encoding="utf-8") as fh:
                body = fh.read()
        else:
            with open(PROMPT, encoding="utf-8") as fh:
                body = fh.read() + task["text"] + "\n"
        prompt, redactions = V.redact(body)
        prompt_path = os.path.join(run_dir, "prompt.md")
        for path in (prompt_path, os.path.join(call_dir, "prompt.md")):
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(prompt)
        tier = tiers[tier_name]
        timeout = V.clamp_timeout(pol, tier.get("timeout_s"))
        request = {
            "schema_version": 1, "vendor": vendor, "kind": "lane", "stage": "lane", "call_id": call_id,
            "target": worktree, "target_head": base,
            "context_files": [os.path.abspath(args.prompt_file)] if args.prompt_file else [f"task:{args.spec_dir}#{args.task}"],
            "prompt_bytes": len(prompt.encode("utf-8")), "prompt_sha256": V.sha256_text(prompt),
            "redactions": redactions, "timeout_s": timeout, "effort": tier.get("effort"), "model": vconf.get("model"),
        }
        with open(os.path.join(call_dir, "request.json"), "w", encoding="utf-8") as fh:
            json.dump(request, fh, indent=2, sort_keys=True)

        cli_real = os.path.realpath(info["cli"])
        go_binds, go_env = go_toolchain()
        gocache = os.path.join(run_dir, "gocache")
        os.makedirs(gocache)
        answer_path = os.path.join(run_dir, "last-message.txt")
        argv, stdin_path = vendor_argv(vendor, cli_real, vconf, tier, prompt_path, answer_path, worktree)
        box = sandbox_argv(bwrap, worktree, run_dir,
                           ro=[install_root(vendor, cli_real), *go_binds], rw=vendor_state_dirs(vendor))
        path_dirs = [d for d in (go_env.get("_GOBIN"), os.path.dirname(cli_real)) if d] + ["/usr/local/bin", "/usr/bin", "/bin"]
        env = {"PATH": ":".join(path_dirs), "HOME": os.path.expanduser("~"), "TMPDIR": "/tmp",
               "LANG": os.environ.get("LANG", "C.UTF-8"), "TERM": os.environ.get("TERM", "dumb"),
               "NO_COLOR": "1", "MUSE_NO_AUTO_UPDATE": "1", "GOCACHE": gocache, "GOTOOLCHAIN": "local",
               "GOPROXY": "off", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1",
               **{k: v for k, v in go_env.items() if not k.startswith("_")}}
        if vendor == "codex" and os.environ.get("CODEX_HOME"):
            env["CODEX_HOME"] = os.environ["CODEX_HOME"]
        for k in ("XDG_CONFIG_HOME", "XDG_DATA_HOME"):
            if vendor == "muse" and os.environ.get(k):
                env[k] = os.environ[k]
        rc, out, err, timed_out = V.run_bounded(box + argv, cwd=worktree, stdin_path=stdin_path, timeout=timeout, env=env)
        for name, data in (("stdout.jsonl", out), ("stderr.txt", err)):
            with open(os.path.join(call_dir, name), "wb") as fh:
                fh.write(V.bound(data, 65536)[0])

        commit_residual(worktree)
        if timed_out:
            raise V.Unavailable("timeout", f"no result within {timeout}s")
        why = vendor_failed(vendor, out)
        if why:
            raise V.Unavailable("vendor-error", f"{why} (exit {rc})")
        extra["files_changed"] = check_diff(pol, worktree, base, task["files"])
        patch = V.git(worktree, "diff", "--binary", "--no-renames", base, "HEAD", text=False).stdout
        cap = int(pol.get("lanes", {}).get("max_patch_bytes", 5000000))
        if len(patch) > cap:
            raise V.Unavailable("patch-too-large", f"{len(patch)} bytes; cap {cap}")
        extra["patch_path"] = os.path.join(call_dir, "change.patch")
        with open(extra["patch_path"], "wb") as fh:
            fh.write(patch)
        return V.response(vendor, "lane", "lane", call_id, spawned=True, started=started,
                          detail=f"worktree kept at {worktree}" if args.keep else None, **extra)
    except V.Unavailable as err:
        return V.unavailable(vendor, "lane", "lane", call_id, err, spawned=True, started=started, **extra)
    finally:
        shutil.rmtree(run_dir, ignore_errors=True)
        if worktree and not args.keep:
            V.git(main, "worktree", "remove", "--force", worktree, check=False)
            V.git(main, "worktree", "prune", check=False)
            V.git(main, "branch", "-D", branch, check=False)


def main(argv=None) -> int:
    p = V.Parser.make("lane.py", "one sandboxed vendor implementer run, returned as a patch")
    p.add_argument("--vendor", required=True, choices=["codex", "muse"])
    p.add_argument("--spec-dir", required=True)
    p.add_argument("--task", required=True, type=int)
    p.add_argument("--from", dest="from_dir")
    p.add_argument("--include-uncommitted", action="store_true")
    p.add_argument("--tier")
    p.add_argument("--prompt-file")
    p.add_argument("--keep", action="store_true")
    args = p.parse_args(argv)
    try:
        resp = lane(args)
    except V.UsageError as err:
        print(f"lane.py: {err}", file=sys.stderr)
        return V.EXIT_USAGE
    except Exception as err:  # noqa: BLE001 - fail open: an envelope bug is a skip, never a gate
        resp = V.response(args.vendor, "lane", "lane", None, "unavailable", "internal-error",
                          detail=f"{type(err).__name__}: {err}")
    return V.emit(resp)


if __name__ == "__main__":
    sys.exit(main())
