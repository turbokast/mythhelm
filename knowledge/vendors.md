# Optional consult vendors

The harness can consult three paid external tools. This page describes the optional **consult and patch lanes**, not which coding agent drives MYTHHELM work. Codex, Muse Code, Claude Code, Grok Build, Kimi Code and OpenCode may each be a primary session; none is the default. Consult entry points degrade to an advisory `unavailable` result when a vendor is absent, and the offline test suite runs against stubs. The rule for using consults is `.claude/rules/vendor-usage.md`.

| Vendor | What it is | Used for |
|---|---|---|
| Codex | OpenAI's coding-agent CLI (`codex`) | A second reviewer from another model family: read-only stage consults, diff review, and an implementer lane |
| Muse | Meta's coding-agent CLI (`muse`) | Few, very large prompts: whole-subsystem dossiers, whole-spec validation, reviews beside Codex, and an implementer lane |
| Jev | TypeSafe's System One classifier (HTTP API) | Cheap closed questions with calibrated probabilities at advisory sites |

## Stages

A stage is one kind of consult: a prompt in `scripts/vendors/prompts/`, an answer schema in `scripts/vendors/schemas/`, and a row in the policy saying which vendors may answer it and how often. Adding a stage is a reviewed edit to all three.

| Stage | Vendors | Lifecycle point | Answer |
|---|---|---|---|
| `premise-ground` | Codex, Muse | Before a proposal becomes a spec: do the cited claims hold in the code? | `grounding` |
| `spec-validate` | Codex, Muse | Spec validation: dangling references, infeasible steps, unfalsifiable criteria, Files gaps | `findings` |
| `task-review` | Codex, Muse | After each task, on its own diff | `findings` |
| `change-review` | Codex, Muse | A change without a spec, or a manual review (`/codex-review`) | `findings` |
| `stuck-oracle` | Codex, Muse | After repeated failed fixes: ranked mechanisms, each with a discriminating check | `oracle` |
| `dossier` | Muse | Before specifying work in a large subsystem | `dossier` |

The `/vendor-consult` skill runs any stage; `/codex-review` runs `change-review` on a worktree's diff. Each finding is adjudicated at its anchor and the verdicts recorded with `scripts/vendors/adjudicate.py record`; `adjudicate.py yield` shows what each stage has been worth, which is the evidence for keeping or dropping it.

## Classifier sites (Jev)

| Site | Question | Consumer |
|---|---|---|
| `triage-findings` | Does the cited code show the defect a review finding claims? | Orders the adjudication of a consult's findings |
| `private-material` | Does a paragraph of a PR description, commit message or issue carry private material? | Before publishing text; complements `scripts/ci/check-public-hygiene.sh` |
| `scope-drift` | Is each change outside a task's Files list needed for the task? | Task review |
| `task-class` | What kind of work is a task? | `scripts/vendors/route_lane.py` |

Questions and thresholds live only in `scripts/jev/questions.py`. `jev.py adjudicate` and `jev.py precision` measure each site per question version.

## Implementer lanes

`scripts/vendors/codex-implement.sh` and `scripts/vendors/muse-implement.sh` let an optional vendor implement one task in a fresh worktree under `.claude/worktrees/`, confined by bubblewrap: the home directory, `/tmp` and sibling checkouts are hidden, only the worktree is writable, and the git metadata is read-only. The result is a patch under `.claude/data/vendor-calls/<id>/`. A task that lists a protected path (`.github/`, `.claude/`, `knowledge/`, `scripts/`, `specs/`, licence files, and `go.mod`/`go.sum` unless listed) never goes to a lane, and a patch that touches anything outside the task's Files is refused. `route_lane.py` suggests an optional patch lane; the primary session may be any supported coding agent.

The committing agent reviews the patch, runs every gate, and commits under its own identity with the trailer the lane reports (`Vendor-Assisted-By: Codex CLI` or `Vendor-Assisted-By: Muse CLI`), plus the usual `Signed-off-by:`. The vendor never commits or pushes.

Lanes need Linux with a root-owned `bwrap` (`apt install bubblewrap`) that can start a user namespace. Without it a lane reports `sandbox-unavailable`.

## Enabling a vendor

From your own terminal, not through an agent (the guard hook blocks agents from opting you in):

1. Install the vendor's CLI and sign in with it natively (`codex login`, `muse login`). For Jev, export `TYPESAFE_API_KEY` in the environment Claude Code starts from. The harness never reads or stores credentials; it asks the CLI's own status command whether you are signed in.
2. `python3 scripts/vendors/vendors.py enable codex` (add `--lanes` to also allow its implementer lane).
3. `python3 scripts/vendors/vendors.py status` shows availability and today's usage against the caps.

To stop: `python3 scripts/vendors/vendors.py disable <vendor|all>` (`--lanes-only` keeps the consults). Either command works from inside an agent session too.

## Policy and caps

`.claude/data/vendor-policy.json` (tracked) holds the defaults: minimum CLI versions, the sign-in status command, per-vendor daily call caps, per-stage and per-site daily caps, timeouts, context-size caps, the lane tiers and protected paths. Your gitignored `.claude/data/vendor-policy.local.json` is merged over it; that is where `enabled` and `lanes_enabled` live, and where you lower a cap or pin a `model`. A call over a cap is an `unavailable` result with reason `over-quota`. Caps are counted locally per UTC day and know nothing about your plan's real limits.

## What stays on your machine

Everything the layer writes is under the main checkout's `.claude/data/` and gitignored: `vendor-calls.jsonl` (one row per call), `vendor-calls/<id>/` (the exact redacted prompt, a request record, stderr, and a lane's patch), `vendor-adjudications.jsonl`, `vendor-quota.json`, `lane-routing.jsonl` and `jev-adjudications.jsonl`. A contributor who never enables a vendor gets none of these files.

## What leaves your machine

Prompts: a shared preamble, the stage prompt, and the context files you pass, with tokens, private keys, secret-looking assignments, e-mail addresses and your home path masked. Redaction is mechanical; keep private prose out of context files.

Files: a consult runs in a snapshot worktree that holds tracked and untracked-but-not-ignored files only, and the vendor is pointed at it. The consult is **not** confined to it, though. Codex runs under its own `read-only` sandbox, and Muse runs with its write, shell and web tools disabled. Both settings prevent writes (the envelope also checks the snapshot afterwards). Whether the vendor can read other files your user can read depends on the vendor's sandbox and your operating system, not on this harness. Only the implementer lanes run under bubblewrap, where nothing outside the worktree is visible. Keep secrets out of files the vendor could reach, or leave the vendor disabled.

## Response shape

Every entry point prints one JSON object (`scripts/vendors/schemas/response.schema.json`): `outcome` is `completed` or `unavailable`, `reason` names why, `advisory` is always true, and `answer_path` or `patch_path` names the result.
