---
layout: default
title: MYTHHELM
---

# MYTHHELM

**The open-source command deck for native coding agents.**

MYTHHELM orchestrates the coding agents you already use and trust with their
native harnesses intact.

> **Status: pre-alpha.** The only release, v0.0.1, is a source tag with no
> binary archives, so you build from source. See
> [Limitations]({{ site.baseurl }}/limitations.html) for what is supported.

## Scripted demo

![Kill the terminal mid-run: recover and finish, scripted](demos/hero.gif)
*Scripted recording, fake agent, offline, no credentials, rendered from `docs/demos/hero.tape`. It kills the foreground `mythhelm run` with SIGKILL while the scripted agent works: the worker and the agent are separate processes and survive, and `mythhelm recover` reattaches to the same worker and finishes the run without relaunching the agent. A hard-killed worker is a different case and is not shown (#391). The red kill marker is a shell annotation, not mythhelm output. Regenerate it with `docs/demos/record.sh`.*

![Scripted demo recording](demos/demo.gif)
*Honest recording of `mythhelm demo --check pass`, rendered from `docs/demos/demo.tape` — regenerate it with `docs/demos/record.sh`.*

## Start here

- [User guide]({{ site.baseurl }}/user-guide.html): installation, the scripted demo, running a task, stop and recover, execution profiles, billing and the budget ledger, and state migration.
- [Contributing]({{ site.baseurl }}/contributing.html): governance, contribution path and DCO sign-off.
- [Source on GitHub](https://github.com/turbokast/mythhelm)
