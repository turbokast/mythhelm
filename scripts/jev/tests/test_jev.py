"""Tests for the Jev client and its sites. Offline: a loopback HTTP server stands in
for the API, and the real endpoint is unreachable because every test either leaves
TYPESAFE_API_KEY unset or points TYPESAFE_API_URL at the loopback server."""

from __future__ import annotations

import http.server
import json
import os
import sys
import threading
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.normpath(os.path.join(HERE, "..", "..", "vendors", "tests")))
from fixture import Fixture, load_vendors_module  # noqa: E402

V = load_vendors_module()
RESPONSE = V.load_schema("response")
KEY = "test-key-not-a-secret"

TASKS_MD = """### Task 1 — Add foo

- **Budget**: trivial
- **Files**:
  - `internal/foo/foo.go`
"""


class StubAPI:
    """Serves queued (status, body) replies and records every request."""

    def __init__(self):
        self.replies: list = []
        self.requests: list = []
        api = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_POST(self):  # noqa: N802 - http.server naming
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                api.requests.append({"auth": self.headers.get("Authorization"), "body": body})
                status, reply = api.replies.pop(0) if api.replies else (500, {})
                if callable(reply):
                    reply = reply(body)
                data = json.dumps(reply).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def log_message(self, *_args):
                pass

        self.server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self.server.server_port}/v1/systemone"
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def close(self):
        self.server.shutdown()
        self.server.server_close()


def nouls(values):
    def reply(body):
        keys = list(body["questions"])
        return {"model": body["model"], "answers": {k: {"noul": values[i]} for i, k in enumerate(keys)},
                "usage": {"input_tokens": 50, "output_tokens": 0}}
    return reply


class JevSites(unittest.TestCase):
    def setUp(self):
        self.f = Fixture()
        self.addCleanup(self.f.cleanup)
        self.api = StubAPI()
        self.addCleanup(self.api.close)
        self.env = {"TYPESAFE_API_KEY": KEY, "TYPESAFE_API_URL": self.api.url}
        self.f.write("pkg/a.go", "package pkg\n\nfunc A() error {\n\treturn nil\n}\n")
        self.answer = self.f.write("answer.json", json.dumps({"summary": "s", "findings": [
            {"severity": "minor", "category": "correctness", "file": "pkg/a.go", "line": 4, "claim": "ignores the error", "evidence": "x"},
            {"severity": "important", "category": "correctness", "file": "pkg/a.go", "line": 3, "claim": "wrong signature", "evidence": "y"},
            {"severity": "minor", "category": "scope", "file": "missing.go", "line": 1, "claim": "gone", "evidence": "z"},
        ]}), root=self.f.tmp)

    def jev(self, *args, env=None):
        return self.f.run("scripts/jev/jev.py", *args, env=dict(self.env, **(env or {})))

    def test_disabled_by_default_is_a_silent_skip(self):
        rc, out, err = self.jev("triage-findings", self.answer, "--root", self.f.repo)
        self.assertEqual((rc, out), (0, ""))
        self.assertTrue(err.startswith("jev: skipped (disabled"), err)
        self.assertEqual(self.api.requests, [])
        self.assertEqual(self.f.rows(), [])

    def test_missing_key_and_foreign_url_never_reach_a_server(self):
        self.f.enable("jev")
        rc, out, err = self.jev("private-material", self.answer, env={"TYPESAFE_API_KEY": ""})
        self.assertEqual((rc, out), (0, ""))
        self.assertIn("no-api-key", err)
        rc, out, err = self.jev("private-material", self.answer, env={"TYPESAFE_API_URL": "http://192.0.2.1/v1"})
        self.assertEqual((rc, out), (0, ""))
        self.assertIn("policy-denied", err)
        self.assertEqual(self.api.requests, [])

    def test_triage_orders_findings_and_never_asks_about_unanchored_ones(self):
        self.f.enable("jev")
        self.api.replies.append((200, nouls([0.2, 0.9])))
        rc, out, err = self.jev("triage-findings", self.answer, "--root", self.f.repo)
        self.assertEqual(rc, 0, err)
        lines = out.splitlines()
        self.assertEqual(lines[0], "question_version=1")
        self.assertEqual(lines[1:], ["[yes 0.90] finding 1 pkg/a.go:3", "[no 0.20] finding 0 pkg/a.go:4",
                                     "[unanchored    -] finding 2 missing.go:1"])
        (req,) = self.api.requests
        self.assertEqual(req["auth"], f"Bearer {KEY}")
        self.assertEqual(req["body"]["model"], "jev-1.13.0")
        self.assertEqual(len(req["body"]["questions"]), 2)
        self.assertIn("4: \treturn nil", json.dumps(req["body"]["questions"]["f0"]).replace("\\t", "\t"))
        (row,) = self.f.rows()
        self.assertEqual(V.validate(row, RESPONSE), [])
        self.assertEqual((row["vendor"], row["kind"], row["outcome"], row["questions"]), ("jev", "classify", "completed", 2))

    def test_server_errors_and_partial_answers_fail_open(self):
        self.f.enable("jev")
        self.api.replies.append((500, {}))
        rc, out, err = self.jev("triage-findings", self.answer, "--root", self.f.repo)
        self.assertEqual((rc, out), (0, ""))
        self.assertIn("vendor-error", err)
        self.api.replies.append((200, {"answers": {"f0": {"noul": 0.5}}}))
        rc, out, err = self.jev("triage-findings", self.answer, "--root", self.f.repo)
        self.assertEqual((rc, out), (0, ""))
        self.assertIn("unparseable-output", err)
        self.assertEqual([r["reason"] for r in self.f.rows()], ["vendor-error", "unparseable-output"])

    def test_a_rate_limited_request_is_retried(self):
        self.f.enable("jev")
        self.api.replies += [(429, {}), (200, nouls([0.6, 0.6]))]
        rc, out, _err = self.jev("triage-findings", self.answer, "--root", self.f.repo)
        self.assertIn("[review 0.60]", out)
        self.assertEqual(len(self.api.requests), 2)

    def test_private_material_flags_only_paragraphs_over_the_band(self):
        self.f.enable("jev")
        text = self.f.write("pr.md", "Adds the foo package.\n\nFixes a bug reported by a customer.\n\nTests pass.\n", root=self.f.tmp)
        self.api.replies.append((200, nouls([0.05, 0.93, 0.56])))
        rc, out, _err = self.jev("private-material", text)
        self.assertEqual(out.splitlines()[1:], ["[yes 0.93] paragraph 2: Fixes a bug reported by a customer.",
                                                "[review 0.56] paragraph 3: Tests pass."])

    def test_scope_drift_lists_outside_files_even_without_jev(self):
        self.f.write("specs/todo/demo/tasks.md", TASKS_MD)
        self.f.git("add", "-A")
        self.f.git("commit", "-q", "-m", "spec")
        base = self.f.git("rev-parse", "HEAD")
        self.f.write("internal/foo/foo.go", "package foo\n")
        self.f.write("cmd/other.go", "package main\n")
        self.f.git("add", "-A")
        self.f.git("commit", "-q", "-m", "work")
        spec = os.path.join(self.f.repo, "specs/todo/demo")
        rc, out, err = self.jev("scope-drift", "--spec-dir", spec, "--task", "1", "--base", base, "--root", self.f.repo)
        self.assertEqual(out.splitlines(), ["question_version=1", "outside-files: cmd/other.go"])
        self.assertIn("jev: skipped (disabled", err)
        self.f.enable("jev")
        self.api.replies.append((200, nouls([0.1])))
        rc, out, _err = self.jev("scope-drift", "--spec-dir", spec, "--task", "1", "--base", base, "--root", self.f.repo)
        self.assertEqual(out.splitlines()[1], "outside-files: cmd/other.go needed=[no 0.10]")

    def test_task_class_feeds_the_router(self):
        self.f.write("specs/todo/demo/tasks.md", TASKS_MD)
        spec = os.path.join(self.f.repo, "specs/todo/demo")
        rc, out, _err = self.jev("task-class", "--spec-dir", spec, "--task", "1")
        self.assertEqual(out, "")
        self.f.enable("jev")
        self.api.replies.append((200, {"answers": {"c": {"choice": "tight_spec_code", "confidence": 0.8}}}))
        self.assertEqual(self.jev("task-class", "--spec-dir", spec, "--task", "1")[1], "class=tight_spec_code confidence=0.80\n")
        self.api.replies.append((200, {"answers": {"c": {"choice": "tight_spec_code", "confidence": 0.3}}}))
        self.assertEqual(self.jev("task-class", "--spec-dir", spec, "--task", "1")[1], "class=uncertain confidence=0.30\n")
        self.api.replies.append((200, {"answers": {"c": {"choice": "tight_spec_code", "confidence": 0.9}}}))
        rc, out, _err = self.f.run("scripts/vendors/route_lane.py", "--spec-dir", spec, "--task", "1", env=self.env)
        self.assertIn("class=tight_spec_code", out)

    def test_site_cap_stops_requests(self):
        self.f.enable("jev", classifier_sites={"private-material": {"daily_cap": 1}})
        text = self.f.write("pr.md", "One paragraph.\n", root=self.f.tmp)
        self.api.replies += [(200, nouls([0.1])), (200, nouls([0.1]))]
        self.jev("private-material", text)
        rc, out, err = self.jev("private-material", text)
        self.assertEqual(out, "")
        self.assertIn("over-quota", err)
        self.assertEqual(len(self.api.requests), 1)

    def test_adjudicate_and_precision(self):
        for verdict in ("confirmed", "confirmed", "rejected"):
            self.assertEqual(self.jev("adjudicate", "--site", "triage-findings", "--ref", "a", "--verdict", verdict)[0], 0)
        self.assertEqual(self.jev("adjudicate", "--site", "nope", "--ref", "a", "--verdict", "confirmed")[0], 64)
        rc, out, _err = self.jev("precision")
        self.assertEqual(out, "precision: triage-findings v1 confirmed=2 rejected=1 precision=0.67\n")


if __name__ == "__main__":
    unittest.main()
