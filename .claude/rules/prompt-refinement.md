# Prompt Refinement

Before acting on a substantive request (implement, fix, refactor, investigate, review), orient in the repository so the work rests on real paths, the active spec and the conventions. Skip this for confirmations and short replies, answers to your own questions, slash commands, and areas you already oriented in this session.

Answer four questions before writing code:

- **Which domain?** Map the paths, packages or feature area to a domain with `knowledge/domains.md`, and route to its agent: core, adapters, protocol and docs to `go-implementer`; `internal/tui/` and `mods/` to `tui-implementer`; `.github/` and packaging to `release-engineer`; `.claude/`, `knowledge/` and `scripts/` to `agent-config-editor`. Work spanning domains gets an `architect` review.
- **Which files?** Find the few files involved, searching the domain's directories rather than the whole tree.
- **Is there a spec?** Look under `specs/{in-progress,todo,refined}/` for a matching directory. If one exists, its `tasks.md` (or `requirements.md` when unrefined) is the plan, and its requirements and design decisions govern. The master spec, `docs/spec/master-spec.md`, governs everything; read the section the work touches.
- **What changed recently?** `git log --oneline -10 -- <paths>`. For safety-relevant work read `knowledge/invariants.md`.

Then read the source before changing it, stay inside the domain, and tell the requester when the work touches three or more domains.
