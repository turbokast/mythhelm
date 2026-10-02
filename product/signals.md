# Signals

What users and contributors are telling the project, grouped into themes. MYTHHELM has no central telemetry (spec §17.2), so signals come from public sources only: GitHub issues, discussions and pull requests. `/synthesize-signals` reads them with `gh`, groups them and drafts entries here; a theme that warrants work becomes a card through `/triage` and `/backlog add`.

## Schema

```markdown
### S-<n> — YYYY-MM-DD: <theme>
- **Sources**: links to the issues, discussions or pull requests behind the theme
- **Count**: how many distinct sources
- **Summary**: the theme in the reporters' terms, paraphrased, without personal details
- **Cards**: MH-<n> the theme bears on (or none)
- **Suggested action**: a new card, a rescore, a question to ask, or no action and why
```

Entries are appended, never edited. A theme that grows is a new entry naming the earlier one. Quote no more than a reporter wrote publicly, and never copy personal details, logs or private context from an issue.

## Signals

### S-2 — 2026-10-02: Idle sessions go cold: prompt caches expire between turns
- **Sources**: https://github.com/openai/codex/issues/41875
- **Count**: 1
- **Summary**: Raised by the maintainer and corroborated by a public Codex feature request plus vendor docs (https://platform.claude.com/docs/en/build-with-claude/prompt-caching, https://developers.openai.com/api/docs/guides/prompt-caching): prompt caches evict after an idle TTL (Anthropic 5m default/1h option, refreshed on hit; Claude Code 1h on in-plan subscription, 5m on API/overage; OpenAI GPT-5.6+ 30m minimum, refreshed on reuse), so an idle supervised session pays a full-prefix rebuild on its next turn. A TTL-aware keep-alive was proposed and then rejected (see action). The per-route TTL table stays useful for MH-16 (predict rebuild cost, warn on cold sessions) and MH-4.
- **Cards**: MH-16,MH-4,MH-12
- **Suggested action**: no action: maintainer verdict 2026-10-02 — keep-alive savings are denominated in API per-token discounts, a currency subscribers do not spend, while each ping burns allowance/rate limits plus full-price output tokens, the binding constraint on subscriptions; API-route savings are real but out of scope while API billing is unsupported. Do not re-propose without subscription-allowance measurements.
