# 0006. Guarded branch apply

- Status: proposed
- Date: 2026-09-30

## Context

The dogfood slice applies a frozen candidate by creating one branch in the admitted source repository (I08, design §8). The source working tree, index, HEAD, configuration and existing refs must stay unchanged. A no-force fetch into a destination ref can still fast-forward a branch that another process creates after preflight. The run owner lock serializes MYTHHELM commands for one run, but does not lock out the user's Git commands.

## Decision

Import the candidate objects with a source-only fetch, an empty refmap, no tags, no submodule recursion and no FETCH_HEAD write. Then create `refs/heads/<branch>` in a prepared `git update-ref --stdin` transaction with `option no-deref`. Preparing locks the exact destination; check for a symbolic destination while that lock is held, then commit the create-only update. This refuses both existing commits and dangling symbolic refs, including refs created during fetch. Git's zero expected object ID alone accepts dangling symbolic refs, so the locked identity check is necessary. See [Git transaction documentation](https://git-scm.com/docs/git-update-ref/2.43.0). Every Git call uses the safe runner's disabled hooks, fsmonitor, automatic GC and maintenance; fetch also disables commit-graph writes.

The supervisor holds the run owner lock, verifies the version 1 receipt against its journaled digest, preserves its exact bytes, and journals apply intent before importing objects. A branch already at the frozen candidate is reconciled without fetching only when matching apply intent was recorded. Initial apply refuses any existing branch, as required by AC-8.3; this tightens the design's broadly stated reconcile-first step while preserving AC-8.4 crash retry. A branch elsewhere is refused. Intent binds retries to the source repository, branch and frozen commit. The supervisor journals the result, transitions to completed and atomically writes and journals receipt version 2 with the observed branch effect.

## Consequences

- A competing writer's branch is never advanced, even when the candidate would be a fast-forward. A failed compare-and-swap can leave imported objects, but changes no existing ref or checkout state.
- A policy conflict after intent makes the run terminal `blocked` and refreshes its journaled receipt. A preflight refusal leaves the run ready for another branch choice.
- The implementation imports objects and uses a Git ref transaction instead of the design's destination-refspec fetch. The branch creation is the externally visible effect and is atomic.
- A crash after branch creation is reconciled on retry without another fetch. A crash after receipt replacement but before its digest is journaled requires Task 15 recovery using the preserved version 1 bytes and their journaled digest.
- Acceptance tests cover source fingerprints, user hook suppression, policy refusals, competing ordinary and symbolic refs created during fetch, reflog shorthand rejection, and retry with the managed clone unavailable.
