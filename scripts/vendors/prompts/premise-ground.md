## Stage: premise-ground

The context below is a proposal (an issue, a backlog item or a draft specification) that makes claims about the current code and cites where they hold (`path:line`, a symbol, a command). For every claim with a citation, read the cited place in the repository and give one verdict:

- `holds`: the code shows exactly what the claim says.
- `partial`: the mechanism is there, but a detail is imprecise, drifted (for example the line moved) or overstated.
- `refuted`: the code shows the claim is false: the behaviour is absent or does the opposite.
- `unverifiable`: the code cannot settle it (runtime behaviour, an external service, code outside the repository).

Put the smallest quote that decides the verdict in `evidence`.
