# Evidence: vendor-usage

Why each statement of `.claude/rules/vendor-usage.md` exists. These are hazards seen when a multi-vendor harness was run on a private project before MYTHHELM, restated generically.

| Statement | Hazard it prevents |
|---|---|
| Opt-in belongs to the contributor | An agent that can enable a paid tool can spend a contributor's subscription without their knowledge. Contributors must never need a subscription (charter principle 3). |
| Sanctioned wrappers only | A direct CLI call skipped every property the wrapper held: the read-only posture, the clean-worktree target, the kill switch and the usage record. Guards matched command position, because substring matchers blocked commit messages and searches that merely mentioned a vendor. |
| Advisory and fail-open | A vendor outage, an expired login or an exhausted plan must not stop delivery. Any step that waited on a vendor answer became a single point of failure the project does not control. |
| Output is untrusted | Reviewer findings cited lines that did not say what the finding claimed; confirming each at its anchor was the only reliable filter. A reviewer that reads repository content can be steered by text planted in it. |
| Send the minimum | Plain `git status --porcelain` hides ignored files, which is where local secrets live, so a review target that looked clean could carry them. Cleanliness is checked with ignored files included, and snapshots copy only the `git add -A` set. |
| Implementer lanes return patches | A vendor's own sandbox restricted writes but not reads, so a vendor shell could read secrets elsewhere on the machine; the lane runs under bubblewrap with the home directory and sibling checkouts masked, and its only output is a patch the calling agent reviews. |
| Never in CI, never in a guard | A blocking hook that waits on the network fails open when the hook times out, and a guard that allows on a model's answer can be talked past. |
| Questions in one file | Classifier questions written ad hoc by agents were poorly posed, and thresholds tuned for one wording silently stopped fitting another. Precision is only measurable per question version. |
