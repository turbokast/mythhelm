# Account profiles and exhaustion — refinement log

`dispatched=1 returned=1 failed=0`

Verdict: **Ready** after one fresh-context assessment round.

Assessor: Codex `profile_assessor`, PR #595 requirements candidate `60c1b2649aafcfd3d003364362b6100f769d34e4`. D1–D8 all PASS; no auto-fixable or human-required issues. Grounding checks covered native identity derivation, admission bucket construction and transactional exhaustion effects. The assessment distinguished account/profile naming from exact identity and independent capacity.

The coordinator had already independently reviewed the capture and selected finite profile/count/size/freshness/deadline/retry limits under Liam's 3 November project delegation. No requirements changed after the fresh assessment; this lifecycle move and log are folded into the capture PR to avoid another publication cycle while preserving the independent refinement checkpoint.

## Remaining notes for design

- Exact native account identity, supported selector/home isolation, applicable policy permission, quota coupling and continuity qualification remain factual evidence gaps. Unknowns refuse the affected native operation; fixtures do not establish route eligibility.
- OQ-3's immutable schema and canonical transaction/ownership composition are reviewed design choices. Preserve existing run envelopes and the minimum MH-15 handoff producer boundary without a whole-card dependency cycle.
- Split core, adapter, client and documentation tasks by domain. No secret copying, paid fallback or identity cycling is authorized by these requirements.

Location: `specs/refined/account-profiles-exhaustion/`.
Next: reviewed design and domain-pure tasks through the ordinary lifecycle.
