# Invariants

The normative invariants of [`docs/spec/master-spec.md`](../docs/spec/master-spec.md) §4, restated as rules an agent can check a change against. The spec is authoritative: when this page and §4 differ, §4 wins and this page is fixed in the same change. The sections after each rule are where the spec elaborates it.

MUST in §4 is release-blocking. A change that weakens one of these needs a decision record under `docs/decisions/` and a maintainer's approval, not a code comment.

---

| ID | Rule for agents | Spec sections |
|---|---|---|
| I01 | Drive the user's selected native harness through its own executable and a qualified interface. Never replace, impersonate or re-implement it; an SDK runtime needs recorded fidelity and entitlement qualification first. | §1.2, §9.1, §9.9 |
| I02 | Admission resolves identity, workspace, data destination, permission policy, billing posture and required capabilities before a task starts. Any unknown blocks: never map unknown, missing or unparseable to allowed. | §8.3, §13.10 |
| I03 | Nothing read from a repository, a model or a plugin grants permissions, spending or publication. Only the user's trusted configuration and explicit approvals do. | §12.3, §12.4, §14.11 |
| I04 | Never move a task to another provider, a paid billing route, a remote environment or a less restrictive sandbox without the user's explicit authorisation and a new admission. | §8.8, §12.2, §13.10 |
| I05 | One active writer per managed working directory. File reservations coordinate; they are never a security boundary. | §11.3 |
| I06 | Execution ownership survives the UI exiting. A requested stop is reported as requested until reconciliation confirms it. | §7.3, §7.7 |
| I07 | An agent reporting completion is not verification. Verification attaches to an exact artifact revision. | §11.5, §11.6 |
| I08 | Never modify the user's original checkout except through an explicitly requested, validated apply. | §11.2, §11.7 |
| I09 | Keep estimated, reported, observed and unknown values distinct in storage, APIs and the TUI. Never collapse unknown to zero or an estimate to a fact. | §13.3, §13.4, §17.1 |
| I10 | Advertise a hard limit only where its enforcement, including in-flight exposure, is established. | §13.6 |
| I11 | Plugins cannot override admission, approval, evidence or publication rules through supported extension APIs. Arbitrary trusted-host code is disclosed as full host trust. | §14.2, §14.11 |
| I12 | Recovery reconciles external side effects before retrying. Replaying journal events never replays a native tool call. | §7.5, §7.7 |
| I13 | The product works without MYTHHELM-owned network services, custom terminal fonts or optional paid routing. | §3.3, §12.6 |
| I14 | Every advertised platform or adapter capability has a versioned test result, or is visibly marked experimental or unsupported. | §9.14, §16.1 |
| I15 | `subscription-only` rejects separately metered inference, paid auxiliary work, purchased credits and paid overages. Missing entitlement or enforcement evidence blocks admission; a subscription login alone is not evidence. | §13.8, §13.9, §13.10 |
| I16 | Never replace the user's native harness selection with the same model in another harness, an SDK variant or a remote service without explicit authorisation and re-admission. | §9.9, §14.11 |
| I17 | Herdr pane status, terminal text and restored layout never certify completion, grant permissions, prove billing or create a duplicate attempt. | §16.6.2, §16.6.4 |
| I18 | A managed agent has one lifecycle owner and at most one interactive input writer. Host integrations and native viewers never add a second supervisor or writer. | §7.8, §16.6.2 |
| I19 | Never read, extract or replay native subscription secrets, and never disable native authentication to manufacture a subscription route. | §12.5, §13.1 |

---

## Citing an invariant in a task

- **In `tasks.md`.** List every invariant the task can affect under `Invariants touched`, by ID with the spec section it relies on: `I09 (§13.4: cost fields are typed and labelled)`. Name what the task does to keep it, not the invariant's title.
- **In tests.** Name the test after the behaviour that keeps the invariant, and put the ID in a comment beside the assertion or in the test name: `TestAdmissionBlocksUnknownBilling // I02`. The test must fail when the invariant is broken; see `.claude/rules/teeth-discipline.md`.
- **In a review.** A finding that an invariant is broken cites the ID and the `file:line` that breaks it. A plausible-but-unverified risk is a question, not a finding.
- **In code.** Cite an invariant in a comment only where the code would otherwise look wrong: a deliberately fail-closed branch, a refusal that looks over-cautious, an extra reconciliation step.
- **When a task cannot keep one.** Stop and escalate. A spec's honesty register (for example `design.md` §15 of a slice) records where a slice is weaker than the master spec; an implementer never decides that alone.
