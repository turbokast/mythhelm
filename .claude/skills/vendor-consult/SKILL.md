---
name: vendor-consult
description: Run one advisory stage consult (premise-ground, spec-validate, task-review, change-review, stuck-oracle, dossier) with an optional external vendor, then adjudicate the answer at its anchors
argument-hint: "<codex|muse> <stage> <context-file>... [--diff-base <ref>] [--spec <name>]"
---

# Vendor Consult

One stage consult through the shared envelope, for any lifecycle step that wants a second reviewer from another model family. The stages, their lifecycle points and their answer shapes are in `knowledge/vendors.md`; the vendors are optional and advisory (`.claude/rules/vendor-usage.md`).

## Input

`$ARGUMENTS`: the vendor (`codex` or `muse`), the stage, and one or more context files the stage needs (the task text, the spec files, the failure record). Optional: `--diff-base <ref>` to append the diff `<ref>...HEAD`, and `--spec <name>`.

**Missing vendor, stage or context (input step).** Take them from `$ARGUMENTS` and the invoking context. In a slash command or model invocation, ask for what is still missing. When the vendor is unspecified and both are available, prefer Codex, and Muse for `dossier` (the one stage only Muse answers). The context cap is per stage, so switching vendors never helps an oversized context: trim it instead.

## Invocation contexts

- **Slash command**: asks for missing inputs, runs the consult, adjudicates.
- **Model-invoked**: the same; another skill may call this one at its lifecycle step.
- **Non-interactive**: missing inputs stop the skill with the unanswered questions and nothing runs. Otherwise it runs, adjudicates (advisory steps) and records `auto-confirmed (non-interactive)`.

## Steps

1. **Check availability**: `python3 scripts/vendors/vendors.py status`. An unavailable vendor is reported with its reason, and the skill ends without error. Never enable a vendor.
2. **Write the context.** Put only what the stage needs in the context file(s): no credentials, no personal or customer data, no internal hostnames. The envelope redacts mechanically, but it cannot recognise private prose.
3. **Snapshot** the tree the vendor should read: `scripts/vendors/snapshot.sh create --from . --include-uncommitted [--only <path>]...` (use `--only` with a task's Files so a concurrent change is not reviewed). Read `snapshot_dir` from its output.
4. **Consult once**, through the vendor's wrapper only:

   ```bash
   bash scripts/codex/codex-consult.sh --stage <stage> --target <snapshot_dir> --context <file> [--diff-base <ref>] [--spec <name>]
   bash scripts/vendors/muse-consult.sh --stage <stage> --target <snapshot_dir> --context <file> [--diff-base <ref>] [--spec <name>]
   ```

   Read the single JSON response line. For a task diff in a snapshot, `--diff-base HEAD~1` sends exactly the copied changes.
5. **Remove the snapshot**: `scripts/vendors/snapshot.sh remove --dir <snapshot_dir>`.
6. **On `unavailable`**: report the reason and continue the calling workflow as if no consult had been asked for.
7. **On `completed`**: adjudicate every item of the answer at its anchor (advisory step). Findings and verdicts: open the file and line and CONFIRM or REJECT. Oracle hypotheses: run the discriminating tests in likelihood order. Dossier claims: confirm each one before carrying it into a spec. When Jev is available, `python3 scripts/jev/jev.py triage-findings <answer_path> --root .` orders the findings to check first; its bands decide nothing.
8. **Record**: `python3 scripts/vendors/adjudicate.py record --call-id <call_id> --findings <N> --confirmed <C> --rejected <R>`.

## Output

One block per item: `[<vendor>] <your severity> (<confirmed|rejected|unsettled>): <file>:<line>`, the claim restated in one line, and what you found at the anchor. End with the counts recorded in step 8, or the skip reason.
