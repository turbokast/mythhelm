"""jev_client.py: a minimal client for TypeSafe's System One API (the Jev model).

Jev answers closed questions about a JSON state with calibrated probabilities:
`noul` (probability of yes) and `choice` (one option, per-option probabilities and
a confidence). It never writes text, which is why the harness uses it for cheap,
closed judgements at advisory sites.

Contract (.claude/rules/vendor-usage.md):
  - The API key comes from the environment only (TYPESAFE_API_KEY); nothing reads
    or writes a key file.
  - Every request goes through the shared envelope (scripts/vendors/vendors.py):
    opt-in, the daily caps and one row in .claude/data/vendor-calls.jsonl.
  - Any failure raises vendors.Unavailable; callers print `jev: skipped (<reason>)`
    and carry on. Nothing may depend on Jev having answered.
  - The model is pinned in the policy (a versioned id), because thresholds in
    questions.py are tuned against one version.

TYPESAFE_API_URL may point the client at a loopback test server
(http://127.0.0.1:<port>/...) and nowhere else, so a key can never be sent to
another host by an environment override.
"""

from __future__ import annotations

import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

sys.path.insert(0, os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "vendors")))
import vendors as V  # noqa: E402

API_URL = "https://api.typesafe.ai/v1/systemone"
TIMEOUT_S = 30
MAX_RETRIES = 2
RETRYABLE = {429, 529}
MAX_QUESTIONS_PER_REQUEST = 60


def api_url() -> str:
    override = os.environ.get("TYPESAFE_API_URL", "")
    if override:
        u = urllib.parse.urlparse(override)
        if u.scheme == "http" and u.hostname in ("127.0.0.1", "localhost", "::1"):
            return override
        raise V.Unavailable("policy-denied", "TYPESAFE_API_URL may only name a loopback test server")
    return API_URL


class JevClient:
    def __init__(self, site: str, spec: str | None = None):
        self.site = site
        self.spec = spec
        self.pol = V.load_policy()
        V.check_available(self.pol, "jev")
        conf = V.vendor_conf(self.pol, "jev")
        self.model = conf.get("model") or "jev-latest"
        self.key = os.environ[conf.get("api_key_env", "TYPESAFE_API_KEY")].strip()
        self.url = api_url()
        self.site_cap = (self.pol.get("classifier_sites", {}).get(site) or {}).get("daily_cap", 0)

    def ask(self, state, questions: dict) -> dict:
        """Answers for every question key, over as many requests as needed."""
        answers: dict = {}
        keys = list(questions)
        for i in range(0, len(keys), MAX_QUESTIONS_PER_REQUEST):
            chunk = {k: questions[k] for k in keys[i:i + MAX_QUESTIONS_PER_REQUEST]}
            answers.update(self._request(state, chunk))
        return answers

    def _request(self, state, questions: dict) -> dict:
        started = time.monotonic()
        call_id = V.new_id("jev", self.site)
        extra = {"spec": self.spec, "questions": len(questions)}
        try:
            V.reserve(self.pol, "jev", self.site, self.site_cap)
        except V.Unavailable as err:
            V.append_row("vendor-calls.jsonl", V.unavailable("jev", "classify", self.site, call_id, err, started=started, **extra))
            raise
        try:
            raw = self._post({"state": state, "model": self.model, "questions": questions})
            answers = raw.get("answers") if isinstance(raw, dict) else None
            if not isinstance(answers, dict) or any(k not in answers for k in questions):
                raise V.Unavailable("unparseable-output", "response is missing answers")
        except V.Unavailable as err:
            V.append_row("vendor-calls.jsonl", V.unavailable("jev", "classify", self.site, call_id, err,
                                                             spawned=True, started=started, **extra))
            raise
        usage = raw.get("usage") or {}
        V.append_row("vendor-calls.jsonl", V.response(
            "jev", "classify", self.site, call_id, spawned=True, started=started,
            input_tokens=usage.get("input_tokens"), output_tokens=usage.get("output_tokens"), **extra))
        return answers

    def _post(self, body: dict) -> dict:
        data = json.dumps(body).encode("utf-8")
        req = urllib.request.Request(self.url, data=data, method="POST", headers={
            "Authorization": f"Bearer {self.key}", "Content-Type": "application/json",
            "User-Agent": "mythhelm-harness-jev/1"})
        delay = 1.0
        for attempt in range(MAX_RETRIES + 1):
            try:
                with urllib.request.urlopen(req, timeout=TIMEOUT_S) as resp:
                    return json.loads(resp.read().decode("utf-8"))
            except urllib.error.HTTPError as err:
                if err.code in RETRYABLE and attempt < MAX_RETRIES:
                    time.sleep(delay)
                    delay *= 2
                    continue
                raise V.Unavailable("vendor-error", f"HTTP {err.code}") from err
            except (urllib.error.URLError, TimeoutError, OSError) as err:
                raise V.Unavailable("network", str(err)[:200]) from err
            except ValueError as err:
                raise V.Unavailable("unparseable-output", "response is not JSON") from err
        raise V.Unavailable("vendor-error", "retries exhausted")


def skip(reason: str) -> None:
    """The one fail-open shape: a single line on stderr, nothing on stdout."""
    print(f"jev: skipped ({reason})", file=sys.stderr)
