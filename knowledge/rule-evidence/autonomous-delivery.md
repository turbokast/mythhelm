# Evidence: autonomous-delivery

Why each statement of `.claude/rules/autonomous-delivery.md` exists. These are hazards seen when an agent harness delivered a backlog unattended on a private project before MYTHHELM, restated generically. The mechanism behind each is in `knowledge/autonomy.md` § Why each piece exists.

| Statement | Hazard it prevents |
|---|---|
| The grant is the maintainer's | The grant command was reachable from the agent's shell, so a session could grant itself autonomy, and revoke-then-grant restarted an expired window. A mistyped duration once armed a year of unattended operation. |
| "Go ahead" in the conversation is not a grant | Prose authority does not change harness behaviour: a session told it had full authority still ended its turn dozens of times overnight, and nothing recorded what had been authorised. |
| Stay inside the scope; never publish | Unattended sessions are exactly where a push, tag or release would go unreviewed; the publishing guards stay closed because a granted session may not arm them. |
| Merge only through the merge check | A check and a merge that are separate steps let the merged head differ from the checked one. Pinning the head closes that gap. |
| Never ask synchronously | Blocking questions stalled whole sessions with no other work in flight, and most were answered with the option the agent had already recommended. |
| The exit line is the last line only | A session that merely discussed the exit sentinel mid-message was released as if it had declared itself blocked. |
| Run state only through the script | Hand-written timestamps drifted by hours within one session, turning every duration computed from them into fiction; line-number citations into state files went stale when the files were archived. |
| Workers never write run state | A worker's writes carry its parent's session id, so they are indistinguishable from the orchestrator's and land unreviewed. A worker told to be read-only still committed and pushed, because a skill's own instructions overrode its prompt. |
| Lane ownership is a token | A worker that acquired and released a lock keyed by session id deleted the orchestrator's lock, since both carried the same id, and a concurrency cap silently stopped holding. |
| Staleness is a dead holder, not age | An age threshold either reaped long-running work that was still alive or left a crashed session's lock blocking everyone. |

## Instances

None recorded in this repository yet.

## Loosening criteria

A statement may be relaxed when the mechanism behind it moves somewhere stronger: for example, merges gated by a repository ruleset that pins the reviewed head, or run state kept in a store with its own clock and append-only guarantees.
