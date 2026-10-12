---
layout: default
title: MYTHHELM
---

# MYTHHELM

**The open-source command deck for native coding agents.**

MYTHHELM orchestrates the coding agents you already use and trust with their
native harnesses intact.

> **Status: pre-alpha.** There is no usable release yet. See
> [Limitations]({{ site.baseurl }}/limitations.html) for what is supported.

## Scripted demo

![Kill the terminal mid-run, then recover](demos/hero.gif)
*Honest recording with the scripted agent, no network and no credentials: `mythhelm run` is killed with SIGKILL mid-attempt (its worker and the agent keep running), `mythhelm recover` reattaches to the same worker without relaunching it, the work finishes, the checks pass and `mythhelm review` shows the verified candidate. Rendered from `docs/demos/hero.tape` — regenerate it with `docs/demos/record.sh`. A worker killed outright is a different case and is not shown ([#391](https://github.com/turbokast/mythhelm/issues/391)).*

![Scripted demo recording](demos/demo.gif)
*Honest recording of `mythhelm demo --check pass`, rendered from `docs/demos/demo.tape` — regenerate it with `docs/demos/record.sh`.*

## Start here

- [User guide]({{ site.baseurl }}/user-guide.html): installation, quickstart and the scripted demo.
- [Contributing]({{ site.baseurl }}/contributing.html): governance, contribution path and DCO sign-off.
- [Source on GitHub](https://github.com/turbokast/mythhelm)
