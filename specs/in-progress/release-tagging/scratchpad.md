# Release Tagging — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | If MH-18's checkpoint overturns SemVer for CalVer, what happens to this spec's `v<semver>` format? | The format follows the overturned scheme before either spec builds (honesty register; both checkpoints run together so the pair is reviewed as one). | Maintainer at checkpoint / blocks nothing unless overturned |
| 2 | Current tag-protection/ruleset state beyond `Protect main`? | Read-only `gh api` at implementation; recorded, never changed here. | Task 1 / blocks nothing |

Requirements Q1–Q4 were all resolved during design (see design §7 D1–D4): lightweight-via-publication; `v<semver>` with dotted markers; published-incl-prereleases tagged, drafts never; design-ahead fixed input.

## Research notes

- The decisive in-repo constraint: `.claude/skills/finalize-spec-tag/SKILL.md` mandates publish-creates-tag (`gh release edit vX.Y.Z --draft=false --target <merge>`) and "never create or push a `v*` tag by hand", guarded by `guard-publish.sh`. Tags created by release publication are lightweight GitHub tags.
- MH-7's attestation plan (issue #28, card Summary): SLSA build-provenance + keyless (cosign) signatures on artifacts — provenance lives on artifacts, not tag objects, which is why tag signing was rejected.
- Release authority: lead maintainer holds it (`GOVERNANCE.md:9`); W16 keeps tag/publish an explicit operator action.
- `gh api repos/turbokast/mythhelm/rulesets` verified readable 2026-10-06 (`Protect main`); classic tag-protection endpoint 404s.
- Release-drafter: `v`-prefixed templates, no prerelease section — pre-release marking happens at publish (operator).
- No GPG/SSH tag-signing practice anywhere in the tree (structural sweep with command + output recorded in design §1).

## Discoveries
