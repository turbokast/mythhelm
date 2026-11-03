# Public plugin contracts — Scratchpad

Requirements-stage open-question ledger. The four entries below reproduce the assessed requirements without choosing a future API, qualification result or decomposition. Design and task work has not started.

## Open Questions

| ID | Question / what it blocks | Conservative default and owner | Resolution evidence |
|---|---|---|---|
| OQ-1 | Which exact two production built-ins exercise each advertised interface? Blocks public stabilization of that interface. | Root technical owner selects reuse/adaptation during design; no new public operation until two real consumers cover it. No founder question is needed for routine design choices. | Consumer matrix and named production-path tests under AC-1.1, independently reviewed. |
| OQ-2 | Exact framing, wire methods/schema layout, capability vocabulary, SDK packaging and additive persistence composition. Blocks design/implementation, not this capture. | Root technical owner chooses the smallest compatible design, preserving protocol/manifest/canonical version domains and current internal contracts. No guessed API or migration. | Reviewed design/ADR and compatibility goldens mapping every enabled public field to its authoritative producer. |
| OQ-3 | Which actual native/OS/host/containment combinations can support executable plugins without defeating required boundaries? Blocks only advertising/enabling those combinations. | Unknown remains blocked/unsupported; security/native/host owners gather separately authorized exact evidence. Trusted-host approval is explicit, never inferred from plugin content or this draft. | Versioned qualification with adversarial startup/descendant/effect cases; G12 suite for any alternative surface. |
| OQ-4 | Do bounded defaults and cross-domain composition warrant an epic? Blocks final task decomposition, not protocol/basic adapter documentation. | Root technical owner resolves finite limits and split after fresh refinement; retain one bounded capture and domain-pure later tasks rather than creating speculative children. | Independently reviewed scope/dependency matrix and measured boundary tests; no weakening of an invariant through scope reduction. |

