---
layout: default
title: Contributing
---

# Contributing

Thanks for helping. This page summarises how changes get in; the full guide
is `CONTRIBUTING.md` in the repository.

## Governance

MYTHHELM is an open-source project hosted by the TurboKast GitHub
organization. Contributors open issues, discussions and pull requests;
maintainers review and merge them; the lead maintainer breaks ties and holds
release authority. Routine changes merge on a maintainer's approval, while
significant changes — to security boundaries, billing and admission, the
public plugin/control protocol, user-facing configuration or persistence
formats — need a short decision record in `docs/decisions/` plus tests that
pin the decided behaviour. The complete core, built-in adapters, safety
controls, TUI, headless mode, official themes, plugin SDK and local routing
are and will remain free, with no licence fee, paid tier, feature gate or
required account. The full text is the
[governance page]({{ site.baseurl }}/mirror/GOVERNANCE.html).

## Contribution path

- **Small fixes** (typos, docs, obvious bugs): open a pull request directly.
- **Features and behaviour changes**: open an issue or a
  [Discussion](https://github.com/turbokast/mythhelm/discussions) first, so
  scope is agreed before you invest time.
- **Architectural changes** to security, billing, the public protocol or
  persistence: these need a short decision record and tests.
- **Security issues**: follow the
  [security policy]({{ site.baseurl }}/mirror/SECURITY.html). Never use a
  public issue.

Look for [`good first issue`](https://github.com/turbokast/mythhelm/labels/good%20first%20issue)
if you want a starting point. You need no paid subscription: the full
contributor test suite and the offline demo run without any agent
credentials, because a scripted fake adapter stands in for real agents.

Every commit must carry a `Signed-off-by` line under the Developer
Certificate of Origin, with a name and email matching the commit author:

```
Signed-off-by: Your Name <you@example.com>
```

`git commit -s` adds it for you. A CI check blocks pull requests with
unsigned commits.

## Pull requests

Keep each pull request focused on one change, include tests that fail without
it and pass with it, and update documentation when behaviour changes.
Titles follow Conventional Commits (`type(optional-scope): summary`); a CI
check enforces this. Every required CI check must pass, and every review
thread must be resolved before merge.

## Data states in documentation

Use the `state.html` Liquid include when prose names a recorded data state.
Its single argument, `state`, accepts `reported`, `estimated` or `unknown`:

<pre><code>&#123;% include state.html state="reported" %&#125;
&#123;% include state.html state="estimated" %&#125;
&#123;% include state.html state="unknown" %&#125;</code></pre>

The include renders an empty or unrecognised value as `unknown`; authors
should always supply one of the three accepted values. Each label retains its
word and a distinct underline: solid for reported, dashed for estimated and
dotted for unknown, on both Night and Paper. Leave ordinary English, output
examples, field paths and other enums unchanged.

Wrap existing declared-billing or entitlement explanations in a neutral note.
The `markdown="1"` attribute keeps Markdown inside the note working:

```html
<div class="declared-note" markdown="1">
<span class="declared-note-label">declared, not verified</span>

Existing explanation of what the user declares and MYTHHELM does not verify.
</div>
```

Use the note only for unverified declarations. Its border and visible label
carry the distinction without amber or a colour-only signal.

## Tested examples in documentation

A tested example needs both an entry in `docs/_data/examples.yml` and an
annotation around exactly one fenced block. For example, the guide quickstart
uses this source markup:

<pre><code>&lt;div class="example-block" data-example-anchor="docs/user-guide.md#quickstart[1]" markdown="1"&gt;

```sh
go run ./cmd/mythhelm demo
```

&lt;/div&gt;</code></pre>

The anchor combines the repository-relative page path, the preceding `##`
heading's slug and the fenced block's 1-based ordinal under that heading.
Use the same anchor in the annotation and the list entry. Each entry supplies
`anchor`, the verbatim `command`, and integer `expect_exit` (0–255). Keep the
guide quickstart first in the list; the schema comments describe the fields.

Only command examples are currently supported end to end. The schema
defines an optional `output_anchor`, but the layout does not yet generate its
marker, so keep output blocks unlisted and unmarked. Unlisted blocks and
provenance comments do not earn a marker.

The layout generates “tested in CI” labels only in builds whose example checks
gate publication. Do not write labels by hand or commit an enabled
`docs/_data/example_checks.yml`; the workflow supplies that temporary flag.
See the [automation record]({{ '/automation.html' | relative_url }}) for the content, browser and
sandboxed example checks that gate publication.
