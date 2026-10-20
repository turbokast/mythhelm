# Windows process support

No architecture or execution surface is qualified yet. Untested pairs report
`unknown` for both capabilities. Task 11 of `windows-process-ownership` owns
promotion after native Windows lifecycle evidence; this empty matrix does not
qualify a native harness or establish a security boundary.

Each row binds an exact architecture and execution surface to capability values
(`supported`, `unsupported` or `unknown`), crash behaviour (`kill_on_close` or
`survives`), backticked Windows test names, and a Windows CI job plus run URL.
CI job names are `windows-latest` (amd64) and `windows-11-arm` (arm64).
Run URLs identify the repository Actions run that supplies the evidence.
The exactness test checks test declarations in Windows test source; the run's
passing result and coverage of FR-1–FR-4 require Task 11's qualification review.

| arch | surface | process_tree_ownership | worker_detachment | crash behaviour | tests | CI job |
|---|---|---|---|---|---|---|
