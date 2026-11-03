# Refinement log — public-plugin-contracts

Requirements readiness only; no implemented behavior or qualification is asserted.
The coordinating session received the fresh-context assessments before this
lifecycle publication; the publisher did not assess its own source as Ready.

`dispatched=2 returned=2 failed=0`

Verdict: Ready after 2 rounds.

## Assessment rounds

### Round 1

- Fresh assessor: `plugin_protocol_assessor`.
- Source capture: `3744f816`.
- Verdict: Needs work. D1/D3/D4/D5/D6/D7 PASS; D2/D8 NEEDS_WORK.
- Finding: W09 uninstall was missing from FR-6, the contributor journey and Definition of Done.
- Fix: add AC-6.5 and the scoped inactive-uninstall journey/check, preserving active pins, protected evidence, canonical history, bounded cleanup and idempotency. Retain active/uncertain ownership and partial-removal refusal cases.

### Round 2

- New fresh assessor: `plugin_protocol_assessor_r2`.
- Assessed frozen source: `bca77305d66dc315a6f31fc97aacae9233d7b25a`.
- Verdict: Ready. D1–D8 PASS; no issues or human-required questions.
- No further acceptance changes.

## Remaining design notes

9 FRs, 24 ACs, 4 NFRs and 4 bounded open questions remain intact.
Readiness covers bounded built-in-first offline public-contract design.
Two real built-in consumers per advertised interface gate public freeze;
actual native, host and containment qualification require separate evidence.
No extension was installed or enabled by refinement.

## Publication

The repository lifecycle script moved the single spec from `unrefined` to
`refined`. The requirements banner records the independent Ready result;
functional criteria and their implementation/qualification gates are preserved.
Next: write the reviewed design and domain-pure tasks through `/spec`.


## Hosted publication review — Presentation and question ledger

PR #615 identified unescaped alternation pipes inside Markdown table code spans and missing scratchpad question entries. Replace table commands with semantically equivalent repeated rg -e patterns so evidence cells render completely. Copy the four existing questions, conservative defaults, owners and resolution evidence verbatim to the spec-local scratchpad, satisfying the everywhere question-ledger rule without creating design/tasks or changing assessed criteria. All requirements, acceptance criteria, non-goals and dependency choices remain unchanged. Specs lint and public hygiene are checked before signed publication; exact-final-head CI and review remain required.
