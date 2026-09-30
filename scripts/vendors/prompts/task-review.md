## Stage: task-review

The context below names one task of a specification and the diff that implements it (HEAD~1..HEAD in the repository). Review the diff for defects in these categories:

- `correctness`: the change does not do what the task says, or breaks an existing caller.
- `vacuous-test`: a test that cannot fail: it asserts only that nothing crashed, accepts every outcome, or exercises data the code never reads.
- `error-handling`: an error dropped, a failure reported as success, or a partial write left behind.
- `concurrency`: a race, a leaked goroutine, a missing cancellation or a lock held across I/O.
- `portability`: behaviour that differs on Linux, macOS or Windows (paths, line endings, signals, file locking).
- `invariant`: a violation of `knowledge/invariants.md` or the master spec.
- `scope`: a change outside the task's Files list that the task does not need.

Review only the diff and the code it touches. Severity: `critical` corrupts data or state, `important` is a real defect, `minor` is cosmetic.
