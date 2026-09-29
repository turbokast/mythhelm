# Evidence: agent-behavioral-posture

Record for `.claude/rules/agent-behavioral-posture.md`.

## Hazard

Each mandate answers a failure mode that coding agents show repeatedly and that tests do not catch:

1. **Silent guessing.** An ambiguous requirement resolved by the agent's first reading produces code that passes its own tests and solves the wrong problem. Nothing downstream re-asks the question.
2. **Scope creep.** Drive-by refactors and "while I was there" edits enlarge the diff, hide the intended change from review, and break code the task never needed to touch.
3. **Vague goals.** "Make it work" has no stopping condition, so the agent stops when it feels done rather than when a check passes.
4. **Over-engineering.** Agents add configuration, interfaces and defensive branches for futures that never arrive; each is code to test and maintain.
5. **Reasoning over missing data.** An empty or truncated tool result is easy to read as "no matches" or "clean". A piped command reports the last stage's exit status, so `cmd | tail; echo $?` reports `tail`, and a failing test run can be recorded as passing. When several subagents feed a vote, discarding the ones that returned nothing (`results.filter(Boolean)`) makes "found nothing" and "never ran" look identical and quietly raises the agreement of the survivors to a majority.
6. **Trusting labels.** A guard's comment, name or heading drifts from its code. Claims grounded in the label ("the hook blocks X") survive until the day X gets through.
7. **Blaming flakiness.** A retry that passes shows the failure is nondeterministic, not that it is load-related. A constant failing value across runs points away from contention, which scatters, and toward a deterministic cause: often a fixed quantum (a buffer size, a tick, a timeout), sometimes fixed input, configuration or logic. It does not identify the cause by itself; comparing the threshold with the mechanism's granularity does. A threshold finer than the sampling period fails by construction at some rate on all hardware.

## Mechanism

Prose, applied at the moment of decision. Mandate 5's piped-exit hazard is also covered by the gate commands in `.claude/skills/quality-gates/SKILL.md`, which never pipe a gate.

## Instances

None recorded in this repository yet.

## Loosening criteria

None: these are posture, not guards. Refine the wording when an instance shows a mandate was misread.
