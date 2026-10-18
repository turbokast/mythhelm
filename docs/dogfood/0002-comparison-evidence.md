# Native CLI comparison: evidence and protocol

Status: exploratory historical evidence; a prospective pilot is pending route
checks. No comparative performance or quality claim is established.

MYTHHELM runs native coding agents. Its effect must be measured by comparing a
native CLI directly with MYTHHELM driving that same CLI, model and configuration.
Comparing different native models answers a different question. The governing
contract is [master specification v2, section 13](https://github.com/turbokast/mythhelm/blob/30062ecd128d3149564d6708b443681a7cfb077e/mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md#13-evaluation-and-promotion-evidence).

## Recovered evidence

Six historical paired tasks used Claude Code with a recorded Sonnet label.
The local ledger records all six pairs passing their four configured checks.
It records review preferences of four for MYTHHELM and two for the direct CLI.
Those preferences do not demonstrate equivalent correctness: the same reviews
identified important defects, including in candidates that passed every check.
The review process and configuration parity have not been independently audited.

Across these six task pairs, MYTHHELM's recorded wall time minus the direct
CLI's wall time had a mean of **+96.3 seconds**, median **+96 seconds**, and
range **−126 to +288 seconds**. These are descriptive summaries of a convenience
sample, not estimates of causal orchestration overhead or population performance.
The arms did different amounts of native work and did not have demonstrated
equivalent end-to-end acceptance timing. No inferential interval or significance
claim is justified by the recovered assignment records.

Later repeated trials on one additional task must remain a separate cohort.
They include a provider-limited attempt, a candidate with only three of four
checks passing despite a legacy `valid` label, and blocked launches. Repeating
one task does not create independent task samples. A blocked launch is neither a
successful attempt nor evidence of provider quota exhaustion.

The historical normalizer also treated some absent usage as zero. Unknown usage
and charges must remain unknown. Retail-equivalent estimates are not observed
charges. Raw evidence remains local; this report publishes only an aggregate
summary, which cannot be independently recomputed from the public material.
The hash identifies the private source; it is not a substitute for a published
per-pair dataset. The six-pair source ledger's SHA-256 is
`6c9c6a18230a8e35a202dda0937963c19234d0e4018ab752bef637845407083e`.

## Prospective pilot

The initial ceiling is 12 native attempts, 90 minutes elapsed, and 10 minutes
per attempt, with zero paid credits, metered API calls or fallback routes.
These are ceilings, not a required sample size. Admission must establish that
each chosen route fits the allowance; a subscription declaration alone does not
prove paid continuation is prevented. Stop new assignments when any ceiling is
reached. At either time deadline, request stop for every affected active native
process, escalate through its owned stop ladder, and reap it. Record deadline
exceedance and cleanup duration separately: 90 minutes bounds scheduled work,
not a guarantee that process cleanup finishes instantly. If termination cannot
be confirmed, record unresolved ownership and stop the pilot; do not retry or
reuse its workspace. Reconcile running processes before retrying. No automatic
repairs, replans or quota retries are included in this pilot.

The target matrix includes Claude Code, Codex, Muse, Antigravity (`agy`) and
OpenCode. At the inspected revision, native adapters exist only for Claude Code
and Codex (plus the synthetic test adapter). An installed executable does not
establish a qualified route. Unsupported wrapped arms are marked unavailable;
they cannot be replaced with a different harness and still called matched pairs.
Codex currently accepts only strict billing, and the repository records no
live-qualified route; its wrapped arm is therefore blocked pending qualification.
Claude's declared billing posture requires the user's account-bound declaration
that extra usage is disabled. Account changes require a fresh declaration and
start a new experimental block; never split a pair across them.

### Readiness inspection — 2026-10-15

A credential-free inspection of repository revision
`f2b486d9b2e426d0f288fce64ab1da0a7ef3e000` and installed tools found Claude Code
2.1.300, Codex 0.162.1, Muse Code 1.4.7 (1.4.7-R6101.1), and Antigravity 1.2.14.
OpenCode's executable was present, but its version probe failed while attempting
to open a log on a read-only filesystem; its version remains unobserved.
These are installation observations, not qualified routes. No inference,
authentication changes or live qualification was performed.

Codex has a transport blocker as well as the billing blocker above:
[`launch.go`](https://github.com/turbokast/mythhelm/blob/f2b486d9b2e426d0f288fce64ab1da0a7ef3e000/adapters/codex/launch.go) labels its `app-server` invocation a
placeholder and delivers the prompt directly on stdin.
[`compat.go`](https://github.com/turbokast/mythhelm/blob/f2b486d9b2e426d0f288fce64ab1da0a7ef3e000/adapters/codex/compat.go) uses synthetic version `0.0.0` and
reports native capabilities as unknown. A real structured transport and a tested
native-version baseline must precede a matched wrapped arm.

Model and effort parity also need evidence. The
[`run` CLI](https://github.com/turbokast/mythhelm/blob/f2b486d9b2e426d0f288fce64ab1da0a7ef3e000/internal/cli/run.go) exposes no model or effort selection;
the [Claude launch](https://github.com/turbokast/mythhelm/blob/f2b486d9b2e426d0f288fce64ab1da0a7ef3e000/adapters/claudecode/launch.go) supplies neither flag,
and [qualification key construction](https://github.com/turbokast/mythhelm/blob/f2b486d9b2e426d0f288fce64ab1da0a7ef3e000/internal/admission/qualify.go) records
`ModelSnapshot: "unknown"` and `EffortSettings: "none"`. A pilot must establish
and record the effective native configuration and observed model/effort, or add
reviewed support for carrying those selections through admission and launch.
Defaults and matching model labels alone do not demonstrate parity.

Local Muse launcher inspection found automatic updates, with a supported
`MUSE_NO_AUTO_UPDATE=1` control; native help also advertised an LLM approval judge
enabled by default. A future Muse manifest must evidence executable pinning,
effective update controls and the judge's auxiliary funding route. Neither
control establishes entitlement. Muse, Antigravity and OpenCode still have no
wrapped adapter here.

The next readiness work is to freeze and validate the manifest below without
launching inference, preserving blocked and unavailable arms. The shortest live
candidate is one Claude matched pair after resolving the managed-policy evidence
gap and completing the spec's maintainer live qualification task in
[`claude-strict-subscription`](https://github.com/turbokast/mythhelm/blob/f2b486d9b2e426d0f288fce64ab1da0a7ef3e000/specs/in-progress/claude-strict-subscription/tasks.md)
(pending Tasks 5 and 7 depend on Q1), once its prerequisites and required
authorization are met. That also requires account-bound evidence
of included funding and prevention of paid continuation, plus model/configuration
parity. The installed Claude version remains labelled untested by its
[compatibility policy](https://github.com/turbokast/mythhelm/blob/f2b486d9b2e426d0f288fce64ab1da0a7ef3e000/adapters/claudecode/compat.go). No route is made
ready merely by drafting the manifest or providing a declaration.

### Assignment manifest

Before the first assignment, freeze a manifest containing:

- task IDs, exact repository revisions, prompt and check hashes, task-family
  clusters, and a separate held-out task set;
- executable hashes, observed model identities, effort, tools, permissions,
  effective settings, account block, environment and cache/history policy;
- balanced randomized order within each matched pair, recorded seed and complete
  assignment schedule, including unstarted and unavailable arms;
- identical time/resource ceilings and independently run acceptance checks,
  with a blinded semantic review rubric and explicit defect severity rules;
- the hypothesis, all outcome definitions, exclusions, censoring rules, minimum
  useful effect, quality margin, analysis method, stopping rule and observation
  window, including the period for detecting escaped regressions.

Use realistic repair, feature and investigation tasks. Each pair starts from
isolated copies of the same input. Disable automatic version updates where the
native interface supports it and check observed versions afterwards. Differences
that cannot be controlled must be disclosed and can disqualify a matched pair.

## Analysis and improvement loop

The primary outcome is independent acceptance within the fixed attempt budget.
Report native completion separately. Also report time to independent acceptance,
human interventions, defects, review effort, resource usage and observed charges.
Failures and timeouts remain in assigned-arm acceptance denominators. Timeouts
are censored for latency; do not drop them or average only successful timings
as an overall speed claim. Missing measurements remain missing with counts.

Analyze the MYTHHELM effect within each native CLI first. Treat independent
repositories as the independent sampling units when generalizing across
repositories. Preserve matched arms and all task-family/repetition nesting
within each repository when estimating uncertainty, using a preregistered
repository-cluster or hierarchical method. Task families that cross repositories
require a method accounting for both dependencies. A confirmatory study should
estimate paired acceptance differences and uncertainty intervals under that
dependence structure, and use paired latency differences with a preregistered
treatment of censoring. With only one repository or too few independent clusters
for the chosen method, report descriptive results limited to the observed
repositories; do not claim a population interval or a promotion decision. Choose
sample size from the minimum useful effect and pilot variability, allowing for clustering,
before opening the holdout. Twelve attempts alone cannot substantiate a five-CLI
ranking. A sparse pilot is reported as inconclusive, not equivalence.

Use the pilot only to debug measurement and estimate feasibility. Freeze a fresh
confirmatory protocol after it. Account for multiple CLI/metric comparisons and
repeated looks, or reserve a fresh final holdout. Do not repeatedly test until a
favorable result appears. Safety and authority invariants remain hard gates;
speed cannot compensate for their violation.

The default promotion criterion allows no registered quality regression. The
preregistered one-sided uncertainty bound for the quality difference must meet
that floor alongside a useful target-metric gain. A different non-safety margin
requires explicit approval; an inconclusive test does not establish equivalence.

Candidate improvements suggested by the recovered evidence are acceptance-state
accuracy, missing-usage handling and separating verification time from native
work. Each needs a reproducible defect or measured bottleneck, a normal reviewed
change, and fresh evaluation. Neither native elapsed time nor wall-minus-native
alone isolates orchestration overhead. Publish updated aggregate results,
configuration provenance, uncertainty, exclusions and limitations alongside each
evaluated change; keep raw native output and account details private.
