# Measured backlog delivery

Every client delivering backlog work follows this procedure. MYTHHELM's admitted
native route owns execution when available. A direct session is an explicit
fallback with a recorded reason. Existing admission, approval and publication
rules still apply; this procedure grants none of those authorities.

## Before dispatch

1. Use `<main checkout>/.claude/data/dogfood-measurements` as the private shared
   ledger. `runspec.main_data_dir()` locates the main checkout's data directory
   from linked worktrees. Keep raw outputs and evidence private and ignored.
2. Read unfinished records with `dogfood_measurement.py report`. Reconcile an
   existing run before retrying it. Never backdate a start or infer success from
   a worker's disappearance.
3. Build the reviewed MYTHHELM revision and use normal admission. Preserve the
   original checkout and state. Required trust approval must cover concrete
   hooks/MCP effects and destinations; a digest alone is neither review nor
   approval. Record blocked routes. Never substitute metered inference or bypass
   a refusal to satisfy dogfooding.
4. Prepare the begin JSON record below and invoke `begin` before execution.
   Spec task IDs are exactly `<spec-name>-<task-number>`. Each attempt gets a
   fresh measurement ID; pass it and the ledger directory to the worker.
5. Append the usual `runspec.py event --kind dispatch` with
   `--measurement-id <id>` before starting the worker. The shared script refuses
   missing, closed, wrong-task and already-dispatched IDs. Historical rows remain
   historical; do not fabricate measurements to repair their missing attribution.
   This check binds bookkeeping, not native execution or client enforcement.

These are command templates. Supply private record filenames and the main
checkout's ledger directory; never paste account data into commands or git.

```sh
python3 scripts/harness/dogfood_measurement.py begin --state-dir "$TASK_METRICS_DIR" --record "$BEGIN_RECORD"
python3 scripts/harness/dogfood_measurement.py finish --state-dir "$TASK_METRICS_DIR" --id "$MEASUREMENT_ID" --record "$FINISH_RECORD"
python3 scripts/harness/dogfood_measurement.py observe --state-dir "$TASK_METRICS_DIR" --id "$MEASUREMENT_ID" --record "$OBSERVATION_RECORD"
python3 scripts/harness/dogfood_measurement.py report --state-dir "$TASK_METRICS_DIR"
```

## Records

All fields are required, including nullable fields. Unknown values are JSON
`null`, never guessed zero or the string `unknown`. Identifiers use ASCII letters,
numbers, dot, underscore or hyphen. No prompts, host paths, account data, raw
output or argv belong in these records. Hashes are lowercase SHA-256; Git
revisions are full object IDs. Detailed provenance stays in private evidence.

| Begin fields | Meaning |
|---|---|
| `id`, `task_id`, `repository_id`, `family` | Attempt, exact task, public repository identifier, and shared task-template family. |
| `source_revision`, `contract_sha256`, `checks_sha256` | Committed input and frozen task/check definitions. |
| `mode` | `mythhelm`, `direct-native` or `unavailable`. Direct execution is never labelled MYTHHELM-owned. |
| `harness`, `native_version`, `model`, `effort` | Actual native execution bundle. Version/model/effort are nullable. |
| `configuration_sha256`, `environment_sha256` | Audited effective non-secret manifests, or null. Hashing arbitrary metadata does not establish parity. |
| `fallback_reason` | Required identifier for non-MYTHHELM modes, such as `approval_blocked` or `comparison_baseline`. |
| `pair_id`, `protocol_sha256` | Null for routine work; otherwise the registered assignment and frozen protocol hash. |

| Finish fields | Meaning |
|---|---|
| `outcome` | `accepted`, `rejected`, `error`, `timeout` or `blocked`. Unavailable arms finish blocked. |
| `wall_seconds` | Request to independent acceptance or actual termination/observation, measured with a monotonic clock. Partial/unavailable coverage is null. Include verification, review and integration in accepted delivery. |
| `native_seconds`, `verification_seconds`, `review_seconds` | Separately observed durations, or null. Do not subtract unrelated clocks to invent native time. |
| `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens` | Separate native-reported/observed counters, preserving missing categories and scope. Never sum overlapping categories or treat retail estimates as charges. |
| `usage_source` | `reported`, `observed`, `unknown`; unknown requires null token counters. |
| `retries` | Additional native attempts for the same contract. Account for them in the registered budget; missing is null. |
| `bugs`, `errors`, `human_interventions` | Evidence-backed counts under a stated review rubric. Keep severity and tool/verification/infrastructure classification in private evidence. Missing review is not zero bugs. |
| `candidate_revision`, `acceptance_evidence_sha256` | Required for accepted delivery: exact candidate and independently checked evidence. Native exit 0 or a worker report alone proves nothing. |
| `defect_observation_closed` | False until follow-up review is complete; closing requires known bug/error counts. |

The coordinator performs independent acceptance and appends `finish` with the
actual outcome. A stopped session leaves its record unfinished for reconciliation;
unfinished records remain in the acceptance denominator. Record timeouts as
censored, preserving the ceiling and cleanup evidence. Do not relaunch to improve
a score or rewrite failed events.

Later regressions use `observe` with `bugs`, `errors`, `evidence_sha256` and
`defect_observation_closed`. These append-only observations require a finished
attempt and cannot decrease known counts. Preserve corrections needing another
interpretation as separate evidence for review. Directory/file permissions are
0700/0600; records and ledger have fixed bounds. A crashed writer can leave its
lock: reconcile the holder and run before an operator removes it. Never auto-break
the lock, truncate evidence or force-add ignored state.

## Controlled comparisons

Routine delivery is observational. Compare MYTHHELM driving one native harness
with that same harness directly first. Hold source, task, model, effort, tools,
permissions, settings, environment, cache/history policy, budget and independent
acceptance constant. Both arms receive isolated inputs and the same review and
integration work. Other products need separate qualified comparisons; unsupported
arms stay unavailable. Synthetic tests are not live-native performance evidence.

Freeze a private draft before assignments:

```sh
python3 scripts/harness/dogfood_statistics.py freeze --draft "$DRAFT_PROTOCOL" --output "$FROZEN_PROTOCOL"
python3 scripts/harness/dogfood_statistics.py analyze --protocol "$FROZEN_PROTOCOL" --state-dir "$TASK_METRICS_DIR"
```

Required draft fields:

- `experiment_id`, `population`, `phase` (`pilot`/`confirmatory`), `cohort`
  (`development`/`holdout`), integer `seed`, familywise `alpha`, `cap_seconds`,
  `minimum_gain_seconds`, `observation_window_days`, `max_started_attempts`.
- `native`: `harness`, `native_version`, `model`, `effort`,
  `configuration_sha256`, `environment_sha256`.
- `assignments`: each contains `pair_id`, `repository_id`, `family`, `task_id`,
  `source_revision`, `contract_sha256`, `checks_sha256`, `mythhelm_id`, `native_id`.
- `design`: null for an exploratory pilot, or `expected_time_gain_seconds`,
  `expected_acceptance_gain`, `power`, `prior_evidence_sha256`, from separate
  prospective evidence.

Freeze creates a protocol once and balances pair order using its seed. It launches
no agents. Put its hash in every begin record. Edited/late protocols and
unregistered attempts are refused. This implementation permits exactly one
attempt per assigned arm; the budget is twice the pair count. No hidden retries,
optional stopping or favorable-result selection. A new experiment requires a
fresh ID and registration while preserving prior failures.

Confirmatory protocols need a protected holdout, known parity, prior design
evidence, at least a seven-day defect window and sufficient independent components.
Independent review must audit actual sample selection, parity, funding, acceptance
and holdout protection; fields and hashes do not certify their own provenance.

## Statistical interpretation and publication

The primary endpoint is **restricted time to independent acceptance**. An accepted
result within ceiling T scores its observed elapsed time; a failure, timeout or
block scores T. This is not successful-run latency or a claim that failed work
finished at T. All assignments remain visible; missing/unfinished/unstarted arms
prevent inference.

Pairs sharing a repository **or** task family are joined transitively into one
component; the estimand gives each independent component equal weight. Repeating
a task in one repository cannot inflate the independent sample count. Dependence
outside those components or biased sampling invalidates population inference.

For n independent components, paired time differences lie in [-T,T], and
acceptance differences in [-1,1]. Conservative
[Hoeffding bounds](https://doi.org/10.1080/01621459.1963.10500830) split alpha
between the two prespecified endpoints with
[Bonferroni](https://www.itl.nist.gov/div898/handbook/prc/section4/prc463.htm).
The time interval half-width is `T * sqrt(2 * log(4/alpha) / n)`; the acceptance
one-sided lower bound subtracts `sqrt(2 * log(2/alpha) / n)`. Bounds are clipped to
their possible ranges; one component receives no inferential interval. Zero
observed variance does not prove equivalence.

Prospective design also budgets beta = 1-power across both sampling deviations,
using `sqrt(2 * log(2/beta))`. It requires the greater sample size needed to move
the time upper bound beyond the useful gain and acceptance lower bound to zero,
assuming the registered true effects. This is deliberately conservative. A more
practical variance-informed alternative needs a fresh reviewed protocol before
assignment. Do not reduce the sample after seeing results or reuse the pilot as
its holdout.

Publish an advantage only after independent review of the protocol, all-assigned
outcomes, sample design, uncertainty, delayed defects and limitations. The default
quality floor permits no registered regression; speed cannot compensate for a
safety violation. A closing defect observation must follow the registered window.
The script always returns `publication_decision=requires_independent_review`;
it never publishes, grants authority or certifies its own data. More endpoints
or competitors need prespecified multiplicity control or a fresh holdout.

Only privacy-safe reviewed aggregates belong in the [public benchmark
record](../benchmarks.md), README or site. Include date, revisions, scope,
independent sample count, all-assigned acceptance, uncertainty, missingness,
exclusions, method and public fixture/check definitions. No current measurement
establishes superiority.
