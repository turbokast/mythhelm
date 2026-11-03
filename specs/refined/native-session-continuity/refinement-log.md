# Native session continuity — refinement log

`dispatched=2 returned=2 failed=0`

Verdict: **Ready** after two fresh-context assessment rounds.

## Round 1

Assessor: Codex `continuity_assessor`, actual requirements at merged PR #589 (`615e74a`). Returned **Needs work**. D1/D3/D4/D5/D6/D7 PASS; D2/D8 NEEDS_WORK. The only issue was deferred delivery/context/queue bounds in NFR-2 and OQ-2. No human-required question was identified.

Fix: under Liam's 3 November project delegation, the coordinator selected conservative finite deadlines, encoded/context/reference/queue limits, expiry behavior and safe retries. The effective smaller adapter/admission limit still binds; overflow preserves mandatory originals and uncertain effects receive no automatic replay. OQ-2 records the resolved policy.

## Round 2

Assessor: fresh Codex `continuity_assessor_r2`, revised requirements before lifecycle move. Returned **Ready**, all D1–D8 PASS, no auto-fixable or human-required issues. Grounding checks covered worker session emission, canonical journal/recovery, adapter capabilities, v2 coordination validation and migration inventory.

## Remaining notes for design

- Exact native operation/acknowledgement and complete S1/two-route evidence remain unavailable. Design credential-free contracts and explicit unsupported/refusal behavior; do not enable or advertise operations from fixture evidence.
- Preserve the three independently useful proposed streams and domain-pure task ownership. Choose the canonical lease/operation projection composition in the reviewed design; do not create another scheduler or writer.
- Implement and test every NFR-2 boundary, restart lifetime and uncertain-effect refusal. The finite policy is a requirement, not a benchmark or native qualification result.

Location: `specs/refined/native-session-continuity/`.
Next: specification design and domain-pure tasks through the ordinary lifecycle.
