---
layout: default
title: MYTHHELM
cells:
  - title: Attention
    need: Approvals
    status: attention
    evidence: not-supported-yet
  - title: Recovery
    need: Session recovery
    status: recovery
    limit: recovery-limit
    evidence: not-supported-yet
  - title: Readable evidence
    need: Readable verification
    status: verification
    limit: verification-limit
    evidence: acceptance-limits
  - title: Honest budget
    need: Quota gauge
    status: quota-gauge
    evidence: not-supported-yet
---

<section class="landing-intro" aria-labelledby="landing-title">
  <h1 id="landing-title">Run the agents you already trust.<br> Know what they actually did.</h1>
  <p><a href="#status">What works, what’s in progress, what’s not yet</a></p>
</section>

<section id="quickstart" aria-labelledby="quickstart-title">
  <h2 id="quickstart-title">Try the scripted demo</h2>
  <pre><code>go run ./cmd/mythhelm demo</code></pre>
  <p class="quickstart-note">The first build needs the Go module cache populated. <a href="{{ '/user-guide.html#installation' | relative_url }}">Build from source</a>.</p>
</section>

<figure class="hero-recording" aria-labelledby="hero-caption">
  <video id="hero-player" controls preload="none" poster="{{ '/demos/hero-poster.png' | relative_url }}" width="1280" height="650" aria-describedby="hero-caption">
    <source src="{{ '/demos/hero.webm' | relative_url }}" type="video/webm">
    <source src="{{ '/demos/hero.mp4' | relative_url }}" type="video/mp4">
    <a href="{{ '/demos/hero.mp4' | relative_url }}">Download the scripted recovery recording</a>.
  </video>
  <div class="hero-chapters" role="group" hidden data-manifest="{{ '/demos/manifest.json' | relative_url }}" aria-label="Recording chapters">
    {% assign chapters = 'title,run,kill,recover,review' | split: ',' %}{% for chapter in chapters %}
    <button type="button" data-chapter="{{ chapter }}" aria-controls="hero-player">{{ chapter | capitalize }}</button>
    {% endfor %}
  </div>
  <p class="chapter-status" role="status"></p>
  <figcaption id="hero-caption">Scripted recording: kill the foreground run, recover the surviving worker, and review the candidate. Fake agent, offline, no credentials. A killed worker is a different case and is not shown. The red kill marker is a shell annotation. <a href="{{ '/demos/hero.transcript.txt' | relative_url }}">Read the transcript</a>.</figcaption>
</figure>

<section class="capabilities" aria-labelledby="capabilities-title">
  <h2 id="capabilities-title">Why not just tmux?</h2>
  <div class="capability-grid">
    {% for cell in page.cells %}{% assign item = site.data.status | where: 'id', cell.status | first %}
    <article class="capability" data-status-id="{{ item.id }}" data-status="{{ item.column }}">
      <p class="capability-need">{{ cell.need }}</p>
      <h3>{{ cell.title }}</h3>
      <span class="capability-label">{{ item.column | replace: '-', ' ' }}</span>
      <div class="capability-evidence">{{ item.quote | markdownify }}</div>
      {% if cell.limit %}{% assign caveat = site.data.status | where: 'id', cell.limit | first %}
      <div class="capability-limit" data-status-id="{{ caveat.id }}">{{ caveat.quote | markdownify }}</div>
      {% endif %}
      <a href="{{ '/limitations.html#' | append: cell.evidence | relative_url }}">Evidence and limits</a>
    </article>
    {% endfor %}
  </div>
</section>

<section id="status" class="status-panel" aria-labelledby="status-title">
  <h2 id="status-title">What ships today</h2>
  <div class="status-columns">
    {% assign columns = 'works,in-progress,not-yet' | split: ',' %}{% for column in columns %}
    <section data-status-column="{{ column }}" aria-labelledby="status-{{ column }}">
      <h3 id="status-{{ column }}">{{ column | replace: '-', ' ' | capitalize }}</h3>
      <ul>
        {% assign items = site.data.status | where: 'column', column %}{% for item in items %}{% unless item.id == 'verification-limit' %}
        <li data-status-id="{{ item.id }}" data-source-section="{{ item.section | escape }}">
          <div class="status-quote">{{ item.quote | markdownify }}</div>
          <a href="{% assign section_anchor = item.section | slugify %}{{ '/limitations.html#' | append: section_anchor | relative_url }}">Source</a>
          {% if item.id == 'verification' %}{% assign caveat = site.data.status | where: 'id', 'verification-limit' | first %}
          <div class="capability-limit" data-status-id="{{ caveat.id }}" data-source-section="{{ caveat.section | escape }}">
            <div class="status-quote">{{ caveat.quote | markdownify }}</div>
            <a href="{{ '/limitations.html#acceptance-limits' | relative_url }}">Acceptance limits</a>
          </div>
          {% endif %}
        </li>
        {% endunless %}{% endfor %}
      </ul>
    </section>
    {% endfor %}
  </div>
</section>
