#!/usr/bin/env python3
"""consult.py: one read-only, advisory consult of an external vendor CLI.

Entry points (the only spellings the guard hook admits for agents):
  scripts/codex/codex-consult.sh   --stage S --target DIR --context FILE [...]
  scripts/codex/codex-review.sh    --target DIR --base REF [...]
  scripts/vendors/muse-consult.sh  --stage S --target DIR --context FILE [...]

    consult.py --vendor codex|muse --stage STAGE --target DIR
               [--context FILE]... [--diff-base REF] [--out FILE] [--spec NAME]

Envelope order, so a refusal always happens before anything is spent:
  1. policy: the stage exists and may consult this vendor      policy-denied
  2. opted in                                                    disabled
  3. the target is a clean linked worktree (ignored files too)  target-refused
  4. prompt = shared preamble + stage prompt + redacted context,
     no larger than the stage's max_context_bytes                context-too-large
  5. CLI present, version pin, signed in (vendor status cmd)     cli-missing |
                                                                 version-too-old |
                                                                 not-signed-in
  6. one call counted against the daily caps                     over-quota
  7. the vendor runs read-only in the target, under a timeout    timeout | vendor-error
  8. the answer is JSON that matches the stage schema            unparseable-output |
                                                                 schema-mismatch
  9. the target is still clean                                    write-through

Output: one JSON line on stdout (scripts/vendors/schemas/response.schema.json);
`answer_path` names the validated answer on `completed`. The exact prompt and a
request record (request.schema.json) are kept under
.claude/data/vendor-calls/<call_id>/. Exit 0 on every run path, 64 on a usage
error. Never a gate: the caller adjudicates every finding at its anchor.

Residuals: the vendor's own read-only sandbox is trusted for the consult (the
post-run clean check catches writes to the target, not elsewhere); the prompt is
redacted mechanically, which cannot recognise private prose.
"""

from __future__ import annotations

import json
import os
import sys
import tempfile
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import vendors as V  # noqa: E402

PROMPT_DIR = os.path.join(V.SCRIPT_DIR, "prompts")


# ---- vendor adapters -------------------------------------------------------------

def codex_argv(cli, conf, stage_conf, schema_path, prompt_path, answer_path, target):
    argv = [cli, "--sandbox", "read-only", "exec", "--json", "--ephemeral", "--ignore-user-config",
            "-c", f"model_reasoning_effort={stage_conf.get('effort', 'medium')}",
            "--output-schema", schema_path, "-o", answer_path, "-"]
    if conf.get("model"):
        argv[4:4] = ["-m", conf["model"]]
    return argv, prompt_path


def codex_answer(stdout: bytes, answer_path: str):
    """(answer text, input tokens, output tokens) from the -o file, falling back to
    the last agent message in the event stream; raises Unavailable on a failed turn."""
    events = _ndjson(stdout)
    if any(e.get("type") == "turn.failed" for e in events):
        raise V.Unavailable("vendor-error", "the vendor reported a failed turn")
    done = [e for e in events if e.get("type") == "turn.completed"]
    usage = (done[-1].get("usage") if done else None) or {}
    text = ""
    if os.path.exists(answer_path):
        with open(answer_path, encoding="utf-8", errors="replace") as fh:
            text = fh.read()
    if not text.strip():
        msgs = [e.get("item", {}).get("text", "") for e in events
                if e.get("type") == "item.completed" and e.get("item", {}).get("type") == "agent_message"]
        text = msgs[-1] if msgs else ""
    if not done and not text.strip():
        raise V.Unavailable("vendor-error", "no turn.completed event and no answer")
    return text, _int(usage.get("input_tokens")), _int(usage.get("output_tokens"))


def muse_argv(cli, conf, stage_conf, schema_path, prompt_path, answer_path, target):
    argv = [cli, "exec", "--json", "--prompt-file", prompt_path,
            "--reasoning-effort", stage_conf.get("effort", "medium"),
            "--disable-write", "--disable-shell", "--disable-web-tools",
            "--sandbox-network", "restricted", "--approval-mode", "never",
            "--max-model-steps", str(stage_conf.get("max_steps", 40)),
            "--output-schema", schema_path, "--workspace", target]
    if conf.get("model"):
        argv[2:2] = ["--model", conf["model"]]
    return argv, None


def muse_answer(stdout: bytes, _answer_path: str):
    events = _ndjson(stdout)
    done = [e for e in events if e.get("payload_type") == "run.terminal.completed"
            and (e.get("payload") or {}).get("terminal") == "completed"]
    if not done:
        failed = [e for e in events if e.get("payload_type") == "task.lifecycle.failed"]
        why = ((failed[-1].get("payload") or {}).get("event") or {}).get("reason") if failed else None
        raise V.Unavailable("vendor-error", f"no completed run record{f': {why}' if why else ''}")
    return (done[-1].get("payload") or {}).get("text") or "", None, None


ADAPTERS = {"codex": (codex_argv, codex_answer), "muse": (muse_argv, muse_answer)}


def _ndjson(data: bytes) -> list[dict]:
    out = []
    for line in data.decode("utf-8", "replace").splitlines():
        try:
            obj = json.loads(line)
        except ValueError:
            continue
        if isinstance(obj, dict):
            out.append(obj)
    return out


def _int(v):
    return v if isinstance(v, int) and not isinstance(v, bool) else None


# ---- the consult ---------------------------------------------------------------------

def diff_context(target: str, base: str) -> str:
    rng = f"{base}...HEAD"
    log = V.git(target, "log", "--oneline", "--no-decorate", rng).stdout
    stat = V.git(target, "diff", "--stat", rng).stdout
    diff = V.git(target, "diff", "--no-color", rng).stdout
    return (f"## Change under review\n\nBase: `{base}`\n\nCommits:\n```\n{log}```\n\n"
            f"Diffstat:\n```\n{stat}```\n\nDiff:\n```diff\n{diff}```\n")


def build_prompt(stage: str, contexts: list[str], diff_base: str | None, target: str) -> tuple[str, int]:
    parts = []
    for name in ("_preamble.md", f"{stage}.md"):
        with open(os.path.join(PROMPT_DIR, name), encoding="utf-8") as fh:
            parts.append(fh.read())
    body = []
    for path in contexts:
        with open(path, encoding="utf-8", errors="replace") as fh:
            body.append(fh.read())
    if diff_base:
        if V.git(target, "rev-parse", "--verify", "--quiet", f"{diff_base}^{{commit}}", check=False).returncode != 0:
            raise V.UsageError(f"--diff-base {diff_base} is not a commit in the target")
        body.append(diff_context(target, diff_base))
    text, n = V.redact("\n\n---\n\n".join(body))
    return "".join(parts) + "\n## Context\n\n" + text + "\n", n


def consult(args) -> dict:
    started = time.monotonic()
    vendor, stage = args.vendor, args.stage
    call_id = V.new_id(vendor, stage)
    kind = "consult"
    extra = {"spec": args.spec, "answer_path": None}
    try:
        pol = V.load_policy()
        if stage not in pol["stages"]:
            raise V.UsageError(f"unknown stage {stage!r}; known: {', '.join(sorted(pol['stages']))}")
        vconf = V.vendor_conf(pol, vendor)
        if not V.is_enabled(pol, vendor):
            raise V.Unavailable("disabled", f"{vendor} is not enabled for this checkout (knowledge/vendors.md)")
        sconf = V.stage_conf(pol, vendor, stage)
        target = os.path.realpath(args.target)
        ok, why = V.is_clean_linked_worktree(target)
        if not ok:
            raise V.Unavailable("target-refused", why)
        for c in args.context:
            if not os.path.isfile(c):
                raise V.UsageError(f"--context {c} is not a readable file")
        if not args.context and not args.diff_base:
            raise V.UsageError("give --context FILE and/or --diff-base REF")
        prompt, redactions = build_prompt(stage, args.context, args.diff_base, target)
        max_ctx = int(sconf.get("max_context_bytes", 400000))
        if len(prompt.encode("utf-8")) > max_ctx:
            raise V.Unavailable("context-too-large", f"prompt is {len(prompt.encode('utf-8'))} bytes; stage cap {max_ctx}")
        info = V.check_available(pol, vendor)
        V.reserve(pol, vendor, stage, sconf.get("daily_cap"))
    except V.Unavailable as err:
        return V.unavailable(vendor, kind, stage, call_id, err, started=started, **extra)

    timeout = V.clamp_timeout(pol, sconf.get("timeout_s"))
    call_dir = os.path.join(V.data_dir(), "vendor-calls", call_id)
    os.makedirs(call_dir, exist_ok=True)
    prompt_path = os.path.join(call_dir, "prompt.md")
    with open(prompt_path, "w", encoding="utf-8") as fh:
        fh.write(prompt)
    head = V.git(target, "rev-parse", "HEAD", check=False).stdout.strip() or None
    request = {
        "schema_version": 1, "vendor": vendor, "kind": kind, "stage": stage, "call_id": call_id,
        "target": target, "target_head": head,
        "context_files": [os.path.abspath(c) for c in args.context] + ([f"diff:{args.diff_base}...HEAD"] if args.diff_base else []),
        "prompt_bytes": len(prompt.encode("utf-8")), "prompt_sha256": V.sha256_text(prompt),
        "redactions": redactions, "timeout_s": timeout, "effort": sconf.get("effort"), "model": vconf.get("model"),
    }
    with open(os.path.join(call_dir, "request.json"), "w", encoding="utf-8") as fh:
        json.dump(request, fh, indent=2, sort_keys=True)

    build_argv, read_answer = ADAPTERS[vendor]
    schema_path = os.path.join(V.SCHEMA_DIR, f"{sconf['schema']}.schema.json")
    with tempfile.TemporaryDirectory(prefix="vendor-consult-") as tmp:
        raw_answer = os.path.join(tmp, "last-message.json")
        argv, stdin_path = build_argv(info["cli"], vconf, sconf, schema_path, prompt_path, raw_answer, target)
        env = dict(os.environ, NO_COLOR="1", MUSE_NO_AUTO_UPDATE="1")
        rc, out, err, timed_out = V.run_bounded(argv, cwd=target, stdin_path=stdin_path, timeout=timeout, env=env)
        with open(os.path.join(call_dir, "stderr.txt"), "wb") as fh:
            fh.write(V.bound(err, 16384)[0])
        try:
            if timed_out:
                raise V.Unavailable("timeout", f"no answer within {timeout}s")
            text, tin, tout = read_answer(out, raw_answer)
            extra.update(input_tokens=tin, output_tokens=tout)
            if rc != 0 and not text.strip():
                raise V.Unavailable("vendor-error", f"exit {rc}")
            try:
                answer = json.loads(text)
            except ValueError as e:
                raise V.Unavailable("unparseable-output", f"final message is not JSON: {e}") from e
            errors = V.validate(answer, V.load_schema(sconf["schema"]))
            if errors:
                raise V.Unavailable("schema-mismatch", "; ".join(errors[:5]))
            ok, why = V.is_clean_linked_worktree(target)
            if not ok:
                raise V.Unavailable("write-through", f"target changed during a read-only consult: {why}")
        except V.Unavailable as e:
            return V.unavailable(vendor, kind, stage, call_id, e, spawned=True, started=started, **extra)

    data, truncated = V.bound(json.dumps(answer, indent=2).encode("utf-8"), int(pol.get("max_answer_bytes", 262144)))
    if truncated:
        return V.unavailable(vendor, kind, stage, call_id, V.Unavailable("unparseable-output", "answer exceeds max_answer_bytes"),
                             spawned=True, started=started, **extra)
    out_path = os.path.abspath(args.out) if args.out else os.path.join(call_dir, "answer.json")
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, "wb") as fh:
        fh.write(data)
    extra["answer_path"] = out_path
    return V.response(vendor, kind, stage, call_id, spawned=True, started=started, **extra)


def main(argv=None) -> int:
    p = V.Parser.make("consult.py", "one read-only advisory vendor consult")
    p.add_argument("--vendor", required=True, choices=sorted(ADAPTERS))
    p.add_argument("--stage", required=True)
    p.add_argument("--target", required=True)
    p.add_argument("--context", action="append", default=[])
    p.add_argument("--diff-base")
    p.add_argument("--out")
    p.add_argument("--spec")
    args = p.parse_args(argv)
    try:
        resp = consult(args)
    except V.UsageError as err:
        print(f"consult.py: {err}", file=sys.stderr)
        return V.EXIT_USAGE
    except Exception as err:  # noqa: BLE001 - fail open: an envelope bug is a skip, never a gate
        resp = V.response(args.vendor, "consult", args.stage, None, "unavailable", "internal-error",
                          detail=f"{type(err).__name__}: {err}")
    return V.emit(resp)


if __name__ == "__main__":
    sys.exit(main())
