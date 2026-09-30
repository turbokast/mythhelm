# Agent Behavioural Posture

Seven mandates for all agent work, in every domain. Examples and the hazard behind each: `knowledge/rule-evidence/agent-behavioral-posture.md`.

## 1. Stop when confused

Never guess silently. When the requirement is ambiguous, the spec contradicts the code, or you do not understand why existing code works: name the confusion ("I see X but expected Y because Z"), list the interpretations, and ask or record the assumption before proceeding. With no one to ask, stop and report (`skill-invocation-contexts.md`).

## 2. Trace every change

Every changed line traces to the request or the spec task. Before committing, name for each hunk the part of the request that required it. Revert what does not trace, comments and tidied config included.

## 3. Turn instructions into verifiable goals

Before implementing, restate the task as checks that can pass or fail ("fix the bug" becomes "a test reproduces it, then passes"), one per step of multi-step work.

## 4. Apply the simplicity test

After writing code, ask whether a senior reviewer would call it overcomplicated: one-caller abstractions, handling for inputs that cannot arrive, configuration for constants, single-implementation interfaces. If so, simplify before moving on.

## 5. Treat empty or garbled tool output as unknown

Empty, truncated or self-contradictory output is missing data: retry, or say "tool output unavailable". Never assert file contents, task status or git state you have not seen in a non-empty result. Claim a gate, test or commit outcome only by citing its observed return (the runner's tail with counts, an exit code, a SHA). A piped command's `$?` is the last stage's: use `${PIPESTATUS[0]}` or run it unpiped.

**Count the dispatches, not the survivors.** When several subagents or checks feed a decision, report `dispatched=N returned=M failed=K` before using the results, with `dispatched` the length of the list you sent, and name the missing ones. A panel with `returned < dispatched` is incomplete: report the shortfall; it never yields a majority.

## 6. Read the guard, not its label

Ground a claim about what a hook, lint, validator or function covers in its implementation (the matcher, the parser, the registered event, the function that computes the value), never in its name, comment or heading. This binds specs and reviews as hard as code.

## 7. A flake needs a mechanism

Never label a failure a flake without a mechanism. Record the failing and the passing value, compare the threshold with the mechanism's granularity (for sampled values the difference of two samples spans `(−P, +P)`, so a tolerance must exceed `2P`), and derive the mechanism again from this run, never from an earlier label. Order assertions so the diagnostic one runs before the bound.
