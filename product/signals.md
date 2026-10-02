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
- **Summary**: Raised by the maintainer and corroborated by a public Codex feature request plus vendor docs (https://platform.claude.com/docs/en/build-with-claude/prompt-caching, https://developers.openai.com/api/docs/guides/prompt-caching): prompt caches become eligible for eviction after an idle TTL (minimums, not guarantees: Anthropic 5m default/1h option, refreshed on hit; OpenAI GPT-5.6+ 30m minimum, refreshed on reuse; Claude Code widely reported as 1h on in-plan subscription, 5m on API/overage), so an idle supervised session risks a full-prefix rebuild on its next turn. A TTL-aware keep-alive was proposed and then rejected (see action). The per-route TTL table stays useful for MH-16 (predict rebuild cost, warn on cold sessions) and MH-4.
- **Cards**: MH-16,MH-4,MH-12
- **Suggested action**: no action: maintainer verdict 2026-10-02 — keep-alive savings are denominated in API per-token discounts, a currency subscribers do not spend, while each ping burns allowance/rate limits plus full-price output tokens, the binding constraint on subscriptions; API-route savings are real but out of scope while API billing is unsupported. Do not re-propose without subscription-allowance measurements.

### S-3 — 2026-10-02: Small "System-1" models do not yet make reliable routers; simple baselines and escalation remain the bar
- **Sources**: https://github.com/openai/codex/issues/42937
- **Count**: 1
- **Summary**: Raised by the maintainer after reviewing an external design note that proposed a local System-1 router (GLiNER task analyser, Laya/Jev-style decision model, hand-authored capability matrix, cheapest configuration above a success threshold). Public evidence: Laya scored 0.600 routing accuracy on 180 prompts (95% CI ±9.5), tying GPT-5 nano; rewording tier descriptions swung accuracy 21 points; downstream task success was not measured (https://github.com/glukicov/laya_router). GLiNER2.5-Decide (340M, Apache 2.0, released 2026-09-24) is now a routing model in its own right, but its 60.1% lead is on the vendor's own benchmark (https://fastino.ai/blog/gliner-2-5-decide-open-weight-decision-model). Research: kNN matches or beats learned routers (arXiv 2505.12601); commercial routers fail to reliably beat a simple baseline (arXiv 2601.07206); 21 router methods plateau far below the oracle because they learn global averages rather than per-query signal (arXiv 2606.07587). The cited Codex issue reports the stronger model stopping early more often, so capability scores are not reliability. These findings confirm master spec §8.1 and §8.4-§8.6; the note's capability matrix and point estimates conflict with §8.4.
- **Cards**: MH-5
- **Suggested action**: No rescore. When MH-5 is specced, require a baseline any router plugin must beat on success per allowance spent: kNN over local past outcomes plus a cheap-first escalation when a verifier fails; a measured calibration check before any probability threshold; and exploration logging so outcome data is not biased by observing only the chosen route. GLiNER2.5-Decide and Laya are candidate §8.6 plugins, not defaults.

### S-4 — 2026-10-02: Multiple subscriptions per harness: users want switching on exhaustion; only native-profile handoff fits terms and spec
- **Sources**: https://github.com/anthropics/claude-code/issues/34341, https://github.com/anthropics/claude-code/issues/64376, https://github.com/anthropics/claude-code/issues/12786, https://github.com/anthropics/claude-code/issues/94195, https://github.com/ex-machina-co/opencode-anthropic-auth/issues/129
- **Count**: 5
- **Summary**: Raised by the maintainer (two Claude Code subscriptions; wants a switch when one runs out, across all harnesses). Demand: Claude Code feature requests ask for named profiles, quick switch and opt-in automatic failover that keeps context; both were closed without a staff answer. Many community switchers exist (https://github.com/realiti4/claude-swap, https://github.com/fairy-pitta/cc-account-switcher, https://github.com/JoRo-Code/codex-account-switcher, https://github.com/rhuanbello/codexctl, https://github.com/marivaldojr/codex-accounts). Liked: one usage dashboard across accounts, switching before the limit, parallel sessions, binding a directory to an account, triggering only on structured limit errors with a cooldown until the reported reset. Disliked: token-copying tools break (refresh tokens destroyed, keychain fallback failures, lock races during refresh, token temp files written 0644), auto-select picks a nearly exhausted account, stale usage readings, failover not validated live, side effects not exactly-once, and device-level limit carry-over after switching (the two Claude Code bug reports). Terms: Anthropic documents one CLAUDE_CONFIG_DIR per account for work and personal use (https://code.claude.com/docs/en/authentication), forbids tools that collect, store or intermediate claude.ai credentials, and says limits assume ordinary individual usage (https://code.claude.com/docs/en/legal-and-compliance); the consumer terms forbid bypassing protective measures; OpenAI forbids circumventing rate limits. A third-party multi-account rotation plugin was archived after a legal request and a ban was reported (cause unconfirmed). No primary source settles rotation between one person's paid accounts. Master spec A04, §13.2 and §13.10 already forbid identity cycling to defeat a limit; selecting another already-authorised provider is allowed.
- **Cards**: MH-16,MH-15,MH-5,MH-12
- **Suggested action**: New card for /triage (maintainer accepted 2026-10-02): native multi-profile execution with exhaustion handoff. (1) profiles are documented native homes (CLAUDE_CONFIG_DIR, CODEX_HOME and equivalents) with native sign-in; MYTHHELM never reads or copies a token (I19); (2) per-profile quota view from documented sources with freshness and quota_bucket ids (MH-16); (3) act only on structured limit errors and honour the reported reset; (4) on exhaustion preserve work as waiting_for_allowance and offer a user-confirmed switch to another authorised profile via a handoff package (MH-15); cross-provider failover may be pre-approved by policy (I04, I16). Same-vendor automatic rotation stays out unless §13.2 is amended on written vendor confirmation.
