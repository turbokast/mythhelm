## Stage: change-review

The context below describes a change and carries its diff against a base revision. Review the diff for defects: `correctness`, `vacuous-test`, `error-handling`, `concurrency`, `portability`, `security`, `invariant` (against `knowledge/invariants.md`), and `public-hygiene` (secrets, personal e-mail addresses, home-directory paths or private material in a public repository).

Review only what the diff changes and the code it calls. Severity: `critical` corrupts data, leaks a secret or breaks a release; `important` is a real defect; `minor` is cosmetic.
