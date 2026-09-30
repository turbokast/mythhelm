---
name: dispatching-parallel-agents
description: Use when facing two or more independent pieces of work that can run without shared state — when to dispatch at all, how to choose the agent, write a self-contained prompt, isolate writers, count what came back, and verify before integrating
argument-hint: "[description of the independent pieces]"
---

# Dispatching Parallel Agents

One agent per independent problem, all sent in one message, each with everything it needs in its prompt. `/run-spec` applies this to spec tasks with its own templates (`/run-spec-dispatch`); this page is the general method.

## Input

`$ARGUMENTS`: optionally, a description of the pieces of work. Usually the skill is applied to the work in hand.

## Invocation contexts

- **Slash command**: applies the checklist below to the described work and dispatches.
- **Model-invoked**: the same.
- **Non-interactive**: the same; a dispatched agent may nest-dispatch only what a skill it is running prescribes.

## When to dispatch

Dispatch has a floor as well as a ceiling.

- **Do it yourself** when a handful of tool calls finishes the work. A dispatch costs a fresh context, a prompt you must write and a result you must read and verify.
- **One agent** when one agent can do it: splitting a coherent task multiplies the merge surface without shortening the critical path.
- **Parallel agents** for two or more pieces with different root causes or subsystems, each understandable without the others, touching different files.
- **Never** to verify your own work: read the diff and the gate output yourself. A *different* reviewing agent (`code-reviewer`) is a separate, legitimate pattern.
- **Not yet** while you do not know what is wrong: investigate first (one `Explore` agent or your own reads), dispatch fixers second.
- **Sequentially** when the pieces share files, or one defines an interface the others consume: land the interface first.

## Steps

1. **Split by independence.** Group the work by package, test file or subsystem. Two failures in the same state machine are probably one bug: one agent.
2. **Choose each agent** by what it will do (`knowledge/agent-routing.md`):

   | Work | `subagent_type` |
   |---|---|
   | Read-only search or investigation | `Explore` |
   | Implementation in a domain | the domain's agent (`knowledge/domains.md`): `go-implementer`, `tui-implementer`, `release-engineer`, `agent-config-editor` |
   | Review of a finished change | `code-reviewer`, or `architect` across domains |
   | Mechanical bookkeeping | `completion-clerk`, `harness-clerk` (never pass them `effort`) |

   Always name `subagent_type`: an omitted one, `general-purpose` or `fork` runs on your own model and skips the agent's pins. Never pass `model`.
3. **Isolate every writer.** Two agents writing one working tree race on its index and files, and one agent's `git stash`, `git add -A` or `git checkout .` destroys the other's work. Give each writing agent `isolation: "worktree"`, have it cut its branch from `origin/main` explicitly (`git fetch origin main && git switch -c <branch> origin/main`), and have it commit and push before it reports: work left uncommitted in a worktree is not a result. Read-only agents need no worktree.
4. **Write self-contained prompts.** The agent sees nothing of your conversation. Each prompt names: the goal as checks ("these tests pass", not "fix it"); the files it owns and the files it must not touch; the context it needs (error output, file paths, the relevant spec section); the constraints (no background commands: a subagent is never notified when one finishes; commit named paths only; stop and report when a hook blocks); and the exact shape of what to return.
5. **Dispatch all of them in one message.** With `run_in_background: true` each result arrives as its own notification.
6. **Count before reading.** Print `dispatched=N returned=M failed=K`, with `N` the number of Agent calls you sent, never the number of results you can see; name the missing ones when `M + K < N`. A dead agent returns nothing, and "found nothing" must never be confused with "never ran" (`.claude/rules/agent-behavioral-posture.md` §5).
7. **Verify, then integrate.** For each result: read its summary against its diff; check it touched only its files; re-run its tests yourself; look for weakened assertions, broad mocks or test-only setup that hides a bug. Then run the combined gates (`scripts/harness/gate.sh all` in the tree you integrate into): parts that pass alone can fail together.
8. **Correct or re-dispatch.** An agent on the right track gets a correction through `SendMessage` to the same agent id, which keeps its context. One that misread the problem is replaced by a fresh agent with a sharper prompt. When two agents edited the same file despite the split, keep the better change and re-dispatch the other with that change as context.

## Output

The accounting line, then one line per agent: what it changed, the evidence you re-checked, and whether it was integrated.
