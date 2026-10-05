# Knowledge

Curated reference for the agents that build MYTHHELM. Rules in `.claude/rules/` say what to do; the files here hold the facts those rules and the agent definitions point to, so the always-loaded context stays small.

Nothing here ships in a release, and nothing here overrides the current product specification identified by [`docs/spec/README.md`](../docs/spec/README.md) or [`docs/harness/charter.md`](../docs/harness/charter.md). When a file here disagrees with either, the file here is wrong: fix it in the same change that notices it.

## When to read what

| Read | When |
|---|---|
| [`domains.md`](domains.md) | Before editing anything: which domain a path belongs to, and which agent owns it. |
| [`invariants.md`](invariants.md) | Before touching admission, billing, process ownership, workspaces, recovery, plugins or credentials; before every review. |
| [`agent-routing.md`](agent-routing.md) | Before adding an agent, changing a `model:` or `effort:` pin, or dispatching a subagent. |
| [`spec-authoring.md`](spec-authoring.md) | Before writing, refining, validating or evaluating a spec: the file formats, the task-block fields, the criterion patterns and a worked example. |
| [`execution.md`](execution.md) | Before running or changing `/run-spec`, `/implement`, the gate markers or the task-completion hook: the execution model, the failure mode behind each mechanism, hook payload facts, where run state lives. |
| [`finalize.md`](finalize.md) | Before finalizing a spec or changing `/finalize-spec`, its sub-skills or `scripts/harness/finalize.py`: what "ready to finalize" and "done" mean, the failure mode behind each mechanism, where the record lives. |
| [`autonomy.md`](autonomy.md) | Before running `/deliver-backlog`, arming or relying on an autonomy grant, or changing the delivery scripts, lanes, the heartbeat or the autonomy hooks: what a grant permits, the failure mode behind each mechanism, where the run's state lives. |
| [`learning-loop.md`](learning-loop.md) | Before deciding or applying a proposal, writing an eval case, or changing `/apply-proposals`, `/health-check` or `scripts/harness/proposals.py`: the decision model, lane 0, the eval format, the health checks. |
| [`product-management.md`](product-management.md) | Before using or changing the product layer: how cards, scores, statuses and the approval queue work, and why. |
| [`vendors.md`](vendors.md) | Before using or changing the optional Codex, Muse or Jev layer: stages, sites, lanes, opt-in, caps, what leaves the machine. |
| [`rule-evidence/`](rule-evidence/README.md) | When auditing a rule, proposing to loosen one, or asking why it exists. Never needed to follow a rule. |

The `/bootstrap` skill reads the first two at the start of a session.

## Conventions

The rule `.claude/rules/knowledge-conventions.md` governs edits here. In short:

- State facts that cannot be derived cheaply from the code or git history, and cite where each one comes from (a spec section, a file, a check).
- Keep instructions out. A "do" or "never" sentence belongs in a rule or an agent definition, which then links here for the reason.
- Public by default: generic hazard descriptions only, never private incidents, people, hosts or dates of rulings.
- A CI check reads [`agent-routing.md`](agent-routing.md). Keep its table shapes intact; `scripts/ci/lint-agent-harness.sh` fails when they stop parsing.
