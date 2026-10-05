"""Tests for scripts/pm/pm.py: the product-layer validator and draft writer. Every
check has a kept broken input that must fail it, next to the clean fixture that must
pass. Each test builds a throwaway git repository; nothing touches the real tree."""

import io
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
import pm  # noqa: E402

OBJECTIVES = "# Objectives\n\n> Current stage: 1\n"

PREAMBLE = "# Backlog\n\nSchema notes.\n\n## Schema\n\n```markdown\n### MH-<n>: <title>\n```\n"


def card(n, title, status="triaged", stage="1", score="3.3 = (value 5 + urgency 5 + risk 3) / effort 4",
         spec="(none)", issue="(none)", gates="G09", extra=()):
    lines = ["### MH-%d: %s" % (n, title), "- **Status**: %s" % status, "- **Stage**: %s" % stage,
             "- **Gates**: %s" % gates, "- **Score**: %s" % score, "- **Spec**: %s" % spec,
             "- **Issue**: %s" % issue, "- **Source**: master spec §15", "- **Summary**: Does a thing."]
    return "\n".join(lines + list(extra))


def backlog(open_cards, closed_cards=()):
    return (PREAMBLE + "\n## Open\n\n" + ("\n\n".join(open_cards) if open_cards else "No open cards.")
            + "\n\n## Closed\n\n" + ("\n\n".join(closed_cards) if closed_cards else "No closed cards.") + "\n")


CLEAN_BACKLOG = backlog([
    card(2, "Second", score="4.3 = (value 4 + urgency 5 + risk 4) / effort 3", gates="G01, G10"),
    card(1, "First", status="implementing", spec="`demo`",
         issue="[#7](https://github.com/turbokast/mythhelm/issues/7)",
         score="3.0 = (value 5 + urgency 5 + risk 5) / effort 5"),
    card(3, "Third", stage="2", score="2.5 = (value 4 + urgency 3 + risk 3) / effort 4", gates="none"),
])

DECISIONS = ("# Decisions\n\n## Schema\n\ntext\n\n## Log\n\n"
             "### D-1 — 2026-09-29: Seed\n- **Type**: backlog-seed\n- **Decision**: Seed it.\n"
             "- **Rationale**: Because.\n- **Cards**: MH-1, MH-2\n")
SIGNALS = "# Signals\n\n## Signals\n\nNo signals recorded yet.\n"


class Repo:
    def __init__(self):
        self.root = tempfile.mkdtemp(prefix="pm-test-")
        env = dict(os.environ, GIT_CONFIG_GLOBAL="/dev/null", GIT_CONFIG_NOSYSTEM="1")
        subprocess.run(["git", "init", "-q", "-b", "main", self.root], check=True, env=env)
        os.makedirs(os.path.join(self.root, "specs", "todo", "demo"))
        os.makedirs(os.path.join(self.root, "specs", "done", "old"))
        self.write("product/objectives.md", OBJECTIVES)
        self.write("product/README.md", "# Product\n\nAgents propose; maintainers approve.\n")
        self.write("product/backlog.md", CLEAN_BACKLOG)
        self.write("product/decisions.md", DECISIONS)
        self.write("product/signals.md", SIGNALS)
        bl = pm.Backlog(CLEAN_BACKLOG, pm.Findings())
        self.write("product/roadmap.md", pm.render_roadmap(bl, 1))

    def path(self, rel):
        return os.path.join(self.root, rel)

    def write(self, rel, text):
        os.makedirs(os.path.dirname(self.path(rel)), exist_ok=True)
        with open(self.path(rel), "w", encoding="utf-8") as f:
            f.write(text)

    def read(self, rel):
        with open(self.path(rel), encoding="utf-8") as f:
            return f.read()

    def replace(self, rel, old, new):
        text = self.read(rel)
        assert old in text, "fixture lacks %r" % old
        self.write(rel, text.replace(old, new, 1))

    def run(self, *args):
        out, err = io.StringIO(), io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            try:
                rc = pm.main(["--root", self.root] + list(args))
            except SystemExit as e:
                rc = e.code
        return rc, out.getvalue(), err.getvalue()

    def errors(self):
        return pm.validate(self.root).errors


class Base(unittest.TestCase):
    def setUp(self):
        pm.PRODUCT_DIR = None
        self.r = Repo()
        self.scratch = tempfile.mkdtemp(prefix="pm-out-")
        self.addCleanup(shutil.rmtree, self.r.root, True)
        self.addCleanup(shutil.rmtree, self.scratch, True)

    def out(self, name="draft.md"):
        return os.path.join(self.scratch, name)

    def assertFinding(self, needle):
        errs = self.r.errors()
        self.assertTrue(any(needle in e for e in errs), "no finding containing %r in %r" % (needle, errs))


class CleanFixture(Base):
    def test_clean_fixture_validates(self):
        self.assertEqual(self.r.errors(), [])
        rc, out, _ = self.r.run("validate")
        self.assertEqual(rc, 0)
        self.assertIn("product: OK", out)

    def test_list_orders_open_cards_by_score(self):
        rc, out, _ = self.r.run("list")
        self.assertEqual(rc, 0)
        self.assertEqual([l.split()[0] for l in out.strip().split("\n")], ["MH-2", "MH-1", "MH-3"])

    def test_next_recommends_the_highest_card_without_a_spec(self):
        rc, out, _ = self.r.run("next")
        self.assertEqual(rc, 0)
        self.assertTrue(out.startswith("### MH-2: Second"))


class BacklogBrokenInputs(Base):
    def test_duplicate_id(self):
        self.r.replace("product/backlog.md", "### MH-3: Third", "### MH-2: Third")
        self.assertFinding("MH-2 is defined twice")

    def test_unknown_status(self):
        self.r.replace("product/backlog.md", "- **Status**: implementing", "- **Status**: in-progress")
        self.assertFinding("status 'in-progress' is not one of")

    def test_score_arithmetic(self):
        self.r.replace("product/backlog.md", "4.3 = (value 4", "4.4 = (value 4")
        self.assertFinding("score 4.4 does not equal")

    def test_urgency_must_match_the_stage(self):
        self.r.replace("product/backlog.md", "2.5 = (value 4 + urgency 3", "3.0 = (value 4 + urgency 5")
        self.assertFinding("urgency 5 does not match stage 2")

    def test_score_shape(self):
        self.r.replace("product/backlog.md", "4.3 = (value 4 + urgency 5 + risk 4) / effort 3", "high")
        self.assertFinding("score must read")

    def test_spec_link_must_resolve(self):
        self.r.replace("product/backlog.md", "`demo`", "`ghost`")
        self.assertFinding("spec 'ghost' resolves to 0 spec directories")

    def test_spec_in_two_states(self):
        os.makedirs(self.r.path("specs/in-progress/demo"))
        self.assertFinding("spec 'demo' resolves to 2 spec directories")

    def test_malformed_spec_field(self):
        self.r.replace("product/backlog.md", "`demo`", "specs/todo/demo")
        self.assertFinding("spec must be '(none)' or backticked spec names")

    def test_shipped_needs_every_spec_done(self):
        self.r.replace("product/backlog.md", "- **Status**: implementing", "- **Status**: shipped")
        self.assertFinding("shipped, but spec 'demo' is in todo/")

    def test_implementing_needs_a_spec(self):
        self.r.replace("product/backlog.md", "`demo`", "(none)")
        self.assertFinding("a implementing card names its spec")

    def test_triaged_card_with_a_spec(self):
        self.r.replace("product/backlog.md", "effort 3\n- **Spec**: (none)", "effort 3\n- **Spec**: `demo`")
        self.assertFinding("a card with a spec is specced or later")

    def test_issue_link_shape(self):
        self.r.replace("product/backlog.md", "[#7](https://github.com/turbokast/mythhelm/issues/7)", "#7")
        self.assertFinding("issue must be '(none)' or")

    def test_issue_number_mismatch(self):
        self.r.replace("product/backlog.md", "[#7](", "[#8](")
        self.assertFinding("link text #8 and the URL's issue 7 differ")

    def test_issue_links_to_one_repository(self):
        self.r.replace("product/backlog.md", "effort 3\n- **Spec**: (none)\n- **Issue**: (none)",
                       "effort 3\n- **Spec**: (none)\n- **Issue**: [#2](https://github.com/someone/fork/issues/2)")
        self.assertFinding("more than one repository")

    def test_unknown_field(self):
        self.r.replace("product/backlog.md", "- **Summary**: Does a thing.", "- **Summary**: Does a thing.\n- **Owner**: me", )
        self.assertFinding("unknown field 'Owner'")

    def test_missing_field(self):
        self.r.replace("product/backlog.md", "- **Stage**: 2\n", "")
        self.assertFinding("missing field 'Stage'")

    def test_gates_vocabulary(self):
        self.r.replace("product/backlog.md", "G01, G10", "G17")
        self.assertFinding("gates must be 'none' or G01-G16")

    def test_stage_vocabulary(self):
        self.r.replace("product/backlog.md", "- **Stage**: 2", "- **Stage**: soon")
        self.assertFinding("stage 'soon' is not one of")

    def test_order_is_canonical(self):
        text = self.r.read("product/backlog.md")
        a, b = text.index("### MH-2"), text.index("### MH-1")
        c = text.index("### MH-3")
        self.r.write("product/backlog.md", text[:a] + text[b:c] + text[a:b] + text[c:])
        self.assertFinding("not in canonical form")

    def test_closed_card_in_the_open_section(self):
        self.r.replace("product/backlog.md", "### MH-3: Third\n- **Status**: triaged", "### MH-3: Third\n- **Status**: dropped")
        self.assertFinding("a dropped card belongs under '## Closed'")

    def test_stray_text_between_cards(self):
        self.r.replace("product/backlog.md", "### MH-3: Third", "Some prose.\n\n### MH-3: Third")
        self.assertFinding("unexpected line")

    def test_malformed_heading(self):
        self.r.replace("product/backlog.md", "### MH-3: Third", "### MH3: Third")
        self.assertFinding("malformed card heading")

    def test_missing_section(self):
        self.r.replace("product/backlog.md", "## Closed\n\nNo closed cards.\n", "")
        self.assertFinding("missing section '## Closed'")


class LogBrokenInputs(Base):
    def append_decision(self, head, fields="- **Type**: rescore\n- **Decision**: d\n- **Rationale**: r\n- **Cards**: none\n"):
        self.r.write("product/decisions.md", self.r.read("product/decisions.md") + "\n" + head + "\n" + fields)

    def test_a_valid_append(self):
        self.append_decision("### D-2 — 2026-09-30: Rescore")
        self.assertEqual(self.r.errors(), [])

    def test_ids_increase(self):
        self.append_decision("### D-1 — 2026-09-30: Again")
        self.assertFinding("D-1 follows D-1")

    def test_dates_never_go_back(self):
        self.append_decision("### D-2 — 2026-09-01: Backdated")
        self.assertFinding("dated 2026-09-01, before the entry above it")

    def test_type_vocabulary(self):
        self.append_decision("### D-2 — 2026-09-30: T", "- **Type**: whim\n- **Decision**: d\n- **Rationale**: r\n- **Cards**: none\n")
        self.assertFinding("type 'whim' is not one of")

    def test_cards_exist(self):
        self.append_decision("### D-2 — 2026-09-30: T", "- **Type**: rescore\n- **Decision**: d\n- **Rationale**: r\n- **Cards**: MH-99\n")
        self.assertFinding("card MH-99 is not in product/backlog.md")

    def test_required_fields(self):
        self.append_decision("### D-2 — 2026-09-30: T", "- **Type**: rescore\n- **Decision**: d\n")
        self.assertFinding("D-2: missing field 'Rationale'")

    def test_signal_sources_link_github(self):
        self.r.write("product/signals.md", "# Signals\n\n## Signals\n\n### S-1 — 2026-09-30: Theme\n"
                     "- **Sources**: a forum post\n- **Count**: 1\n- **Summary**: s\n- **Cards**: none\n"
                     "- **Suggested action**: none\n")
        self.assertFinding("sources must link GitHub issues")

    def test_placeholder_beside_entries(self):
        self.r.write("product/signals.md", "# Signals\n\n## Signals\n\nNo signals recorded yet.\n\n"
                     "### S-1 — 2026-09-30: Theme\n- **Sources**: https://github.com/o/r/issues/1\n"
                     "- **Count**: 1\n- **Summary**: s\n- **Cards**: none\n- **Suggested action**: none\n")
        self.assertFinding("placeholder is left beside signal entries")


class OtherFiles(Base):
    def test_objectives_need_the_current_stage(self):
        self.r.write("product/objectives.md", "# Objectives\n")
        self.assertFinding("exactly one '> Current stage: N' line")

    def test_roadmap_names_real_cards(self):
        self.r.write("product/roadmap.md", self.r.read("product/roadmap.md") + "- **MH-42** ghost\n")
        self.assertFinding("card MH-42 is not in product/backlog.md")

    def test_a_stale_roadmap_is_a_warning(self):
        self.r.write("product/roadmap.md", "# Roadmap\n")
        f = pm.validate(self.r.root)
        self.assertEqual(f.errors, [])
        self.assertTrue(any("stale" in w for w in f.warnings))


class Drafts(Base):
    def test_drafts_never_write_into_product(self):
        rc, _, err = self.r.run("fmt", "--out", self.r.path("product/backlog.md"))
        self.assertEqual(rc, 2)
        self.assertIn("inside product/", err)

    def test_fmt_is_idempotent_on_canonical_input(self):
        rc, _, _ = self.r.run("fmt", "--out", self.out())
        self.assertEqual(rc, 0)
        with open(self.out(), encoding="utf-8") as f:
            self.assertEqual(f.read(), CLEAN_BACKLOG)

    def test_add_files_a_scored_card_in_score_order(self):
        rc, out, err = self.r.run("add", "--title", "New", "--stage", "1", "--gates", "G07", "--value", "5",
                                  "--risk", "5", "--effort", "1", "--source", "§12", "--summary", "S.",
                                  "--issue", "12", "--out", self.out())
        self.assertEqual(rc, 0, err)
        self.assertIn("card MH-4", out)
        with open(self.out(), encoding="utf-8") as f:
            text = f.read()
        self.assertIn("- **Score**: 15.0 = (value 5 + urgency 5 + risk 5) / effort 1", text)
        self.assertLess(text.index("### MH-4"), text.index("### MH-2"))
        self.r.write("product/backlog.md", text)
        self.assertEqual(self.r.errors(), [])

    def test_ids_are_reserved_and_never_reused(self):
        first = self.r.run("next-id", "MH")[1].strip()
        second = self.r.run("next-id", "MH")[1].strip()
        self.assertEqual((first, second), ("MH-4", "MH-5"))

    def test_next_id_counts_pending_requests(self):
        req = self.r.path("orchestration/requests/x")
        os.makedirs(req)
        with open(os.path.join(req, "request.json"), "w", encoding="utf-8") as f:
            f.write('{"path": "product/backlog.md"}')
        with open(os.path.join(req, "proposed"), "w", encoding="utf-8") as f:
            f.write("### MH-9: Pending\n")
        self.assertEqual(self.r.run("next-id", "MH")[1].strip(), "MH-10")

    def test_shipping_waits_for_every_spec(self):
        rc, _, err = self.r.run("set-status", "MH-1", "shipped", "--out", self.out())
        self.assertEqual(rc, 1)
        self.assertIn("names specs not yet in done/", err)
        self.assertFalse(os.path.exists(self.out()))
        rc, _, err = self.r.run("set-status", "MH-1", "shipped", "--spec", "old", "--out", self.out())
        self.assertEqual(rc, 0, err)
        with open(self.out(), encoding="utf-8") as f:
            text = f.read()
        closed = text.split("## Closed")[1]
        self.assertIn("### MH-1: First\n- **Status**: shipped", closed)

    def test_a_draft_that_would_not_validate_is_refused(self):
        rc, _, err = self.r.run("set-status", "MH-2", "specced", "--out", self.out())
        self.assertEqual(rc, 1)
        self.assertIn("a specced card names its spec", err)
        self.assertFalse(os.path.exists(self.out()))

    def test_set_issue(self):
        rc, _, err = self.r.run("set", "MH-3", "issue", "#31", "--out", self.out())
        self.assertEqual(rc, 0, err)
        with open(self.out(), encoding="utf-8") as f:
            self.assertIn("[#31](https://github.com/turbokast/mythhelm/issues/31)", f.read())

    def test_advancing_the_stage_needs_a_rescore(self):
        self.r.replace("product/objectives.md", "Current stage: 1", "Current stage: 2")
        self.assertFinding("urgency 3 does not match stage 2 at current stage 2")
        rc, _, err = self.r.run("set", "MH-3", "notes", "x", "--out", self.out())
        self.assertEqual(rc, 1, "other drafts refuse a backlog whose urgencies are stale")
        rc, _, err = self.r.run("rescore", "--out", self.out())
        self.assertEqual(rc, 0, err)
        with open(self.out(), encoding="utf-8") as f:
            text = f.read()
        self.assertIn("3.0 = (value 4 + urgency 5 + risk 3) / effort 4", text)
        self.r.write("product/backlog.md", text)
        self.r.write("product/roadmap.md", pm.render_roadmap(pm.Backlog(text, pm.Findings()), 2))
        self.assertEqual(self.r.errors(), [])

    def test_rescore_one_card(self):
        rc, _, err = self.r.run("rescore", "MH-3", "--stage", "1", "--effort", "3", "--out", self.out())
        self.assertEqual(rc, 0, err)
        with open(self.out(), encoding="utf-8") as f:
            self.assertIn("### MH-3: Third\n- **Status**: triaged\n- **Stage**: 1\n- **Gates**: none\n"
                          "- **Score**: 4.0 = (value 4 + urgency 5 + risk 3) / effort 3", f.read())

    def test_decide_appends_the_next_entry(self):
        rc, out, err = self.r.run("decide", "--type", "rescore", "--title", "Rescore", "--decision", "d",
                                  "--rationale", "r", "--cards", "MH-3", "--date", "2026-09-30", "--out", self.out())
        self.assertEqual(rc, 0, err)
        with open(self.out(), encoding="utf-8") as f:
            text = f.read()
        self.assertTrue(text.startswith(DECISIONS))
        self.assertIn("### D-2 — 2026-09-30: Rescore\n- **Type**: rescore", text)

    def test_first_signal_replaces_the_placeholder(self):
        rc, _, err = self.r.run("signal", "--title", "Theme", "--sources", "https://github.com/o/r/issues/3",
                                "--count", "1", "--summary", "s", "--action", "none", "--date", "2026-09-30",
                                "--out", self.out())
        self.assertEqual(rc, 0, err)
        with open(self.out(), encoding="utf-8") as f:
            text = f.read()
        self.assertNotIn("No signals recorded yet.", text)
        self.r.write("product/signals.md", text)
        self.assertEqual(self.r.errors(), [])

    def test_a_staged_chain_of_drafts(self):
        stage = os.path.join(self.scratch, "stage")
        self.assertEqual(self.r.run("stage", stage)[0], 0)
        os.makedirs(self.r.path("specs/unrefined/badge"))
        rc, _, err = self.r.run("--product-dir", stage, "set-status", "MH-2", "specced", "--spec", "badge",
                                "--out", os.path.join(stage, "backlog.md"))
        self.assertEqual(rc, 0, err)
        rc, _, err = self.r.run("--product-dir", stage, "decide", "--type", "lifecycle-sync", "--title", "MH-2 specced",
                                "--decision", "d", "--rationale", "r", "--cards", "MH-2", "--date", "2026-09-30",
                                "--out", os.path.join(stage, "decisions.md"))
        self.assertEqual(rc, 0, err)
        self.assertEqual(self.r.run("--product-dir", stage, "roadmap", "--out", os.path.join(stage, "roadmap.md"))[0], 0)
        rc, out, _ = self.r.run("--product-dir", stage, "validate")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.r.read("product/backlog.md"), CLEAN_BACKLOG, "staging never touches product/")
        pm.PRODUCT_DIR = None

    def test_stage_refuses_product(self):
        self.assertEqual(self.r.run("stage", self.r.path("product/x"))[0], 2)

    def test_product_dir_reads_a_staged_draft_set(self):
        staged = tempfile.mkdtemp(prefix="pm-staged-")
        self.addCleanup(shutil.rmtree, staged, True)
        for name in ("objectives.md", "backlog.md", "decisions.md", "signals.md", "roadmap.md"):
            shutil.copy(self.r.path("product/" + name), os.path.join(staged, name))
        shutil.rmtree(self.r.path("product"))
        rc, out, _ = self.r.run("--product-dir", staged, "validate")
        pm.PRODUCT_DIR = None
        self.assertEqual(rc, 0, out)
        self.assertIn("product: OK", out)


class AdaptiveProductDrafts(Base):
    def test_later_stages_and_gates_are_scored_and_validated(self):
        self.r.replace("product/objectives.md", "Current stage: 1", "Current stage: 5")
        rc, _, err = self.r.run("rescore", "MH-3", "--stage", "6", "--out", self.out())
        self.assertEqual(rc, 0, err)
        with open(self.out(), encoding="utf-8") as f:
            draft = f.read()
        self.assertIn("- **Stage**: 6", draft)
        self.assertIn("2.5 = (value 4 + urgency 3 + risk 3) / effort 4", draft)
        self.r.write("product/backlog.md", draft)
        rc, _, err = self.r.run("set", "MH-3", "gates", "G13, G14, G15, G16", "--out", self.out())
        self.assertEqual(rc, 0, err)
        with open(self.out(), encoding="utf-8") as f:
            self.assertIn("G13, G14, G15, G16", f.read())

    def test_current_stage_rejects_out_of_range_and_duplicate_values(self):
        for text in ("# Objectives\n\n> Current stage: 7\n",
                     "# Objectives\n\n> Current stage: 1\n> Current stage: 2\n"):
            with self.subTest(text=text):
                self.r.write("product/objectives.md", text)
                self.assertFinding("exactly one '> Current stage: N' line")

    def source(self, name, text):
        path = self.out(name)
        with open(path, "w", encoding="utf-8") as f:
            f.write(text)
        return path

    def test_objective_draft_is_reviewable_without_product_write(self):
        source = self.source("objectives-source.md", "# Objectives\n\n> Current stage: 4\n")
        rc, _, err = self.r.run("draft-text", "objectives", "--from", source, "--out", self.out())
        self.assertEqual(rc, 0, err)
        with open(self.out(), encoding="utf-8") as f:
            self.assertIn("Current stage: 4", f.read())
        self.assertEqual(self.r.read("product/objectives.md"), OBJECTIVES)
        bad = self.source("bad.md", "# Objectives\n\n> Current stage: 7\n")
        rc, _, err = self.r.run("draft-text", "objectives", "--from", bad, "--out", self.out("bad-out.md"))
        self.assertEqual(rc, 2, err)
        self.assertFalse(os.path.exists(self.out("bad-out.md")))

    def test_readme_draft_stays_outside_product(self):
        source = self.source("readme-source.md", "# Product\n\nCurrent spec and signed approvals.\n")
        rc, _, err = self.r.run("draft-text", "readme", "--from", source, "--out", self.out())
        self.assertEqual(rc, 0, err)
        with open(self.out(), encoding="utf-8") as f:
            self.assertIn("Current spec", f.read())
        rc, _, err = self.r.run("draft-text", "readme", "--from", source,
                               "--out", self.r.path("product/README.md"))
        self.assertEqual(rc, 2, err)
        self.assertIn("inside product/", err)
        self.assertEqual(self.r.read("product/README.md"), "# Product\n\nAgents propose; maintainers approve.\n")

    def test_readme_draft_refuses_hardlink_to_product(self):
        source = self.source("readme-source.md", "# Product\n\nA proposed change.\n")
        target = self.out("linked-draft.md")
        os.link(self.r.path("product/README.md"), target)
        before = self.r.read("product/README.md")
        rc, _, err = self.r.run("draft-text", "readme", "--from", source, "--out", target)
        self.assertEqual(rc, 2, err)
        self.assertIn("multiple hardlinks", err)
        self.assertEqual(self.r.read("product/README.md"), before)
        with open(target, encoding="utf-8") as f:
            self.assertEqual(f.read(), before)

    def test_preamble_draft_preserves_cards_and_rejects_injected_card(self):
        source = self.source("preamble.md", PREAMBLE.replace("Schema notes.", "Stages 0–6; gates G01–G16."))
        rc, _, err = self.r.run("draft-text", "backlog-preamble", "--from", source, "--out", self.out())
        self.assertEqual(rc, 0, err)
        with open(self.out(), encoding="utf-8") as f:
            draft = f.read()
        before = pm.Backlog(CLEAN_BACKLOG, pm.Findings())
        after = pm.Backlog(draft, pm.Findings())
        self.assertEqual([c.render() for c in before.cards], [c.render() for c in after.cards])
        self.assertIn("Stages 0–6", draft)
        injected = self.source("injected.md", PREAMBLE + "\n## Open\n\n" + card(99, "Injected"))
        rc, _, err = self.r.run("draft-text", "backlog-preamble", "--from", injected,
                               "--out", self.out("injected-out.md"))
        self.assertNotEqual(rc, 0, err)
        self.assertFalse(os.path.exists(self.out("injected-out.md")))

    def test_stage_includes_readme_without_changing_product(self):
        stage = self.out("stage")
        rc, _, err = self.r.run("stage", stage)
        self.assertEqual(rc, 0, err)
        with open(os.path.join(stage, "README.md"), encoding="utf-8") as f:
            self.assertEqual(f.read(), self.r.read("product/README.md"))

    def test_stage_rejects_symlink_before_copying_any_file(self):
        stage = self.out("stage")
        os.mkdir(stage)
        target = os.path.join(stage, "README.md")
        os.symlink(self.r.path("product/README.md"), target)
        before = self.r.read("product/README.md")
        rc, _, err = self.r.run("stage", stage)
        self.assertEqual(rc, 2, err)
        self.assertIn("already exists", err)
        self.assertEqual(self.r.read("product/README.md"), before)
        self.assertEqual(os.listdir(stage), ["README.md"], "preflight must precede every copy")

    def test_stage_rejects_hardlink_without_truncating_product(self):
        stage = self.out("stage")
        os.mkdir(stage)
        os.link(self.r.path("product/objectives.md"), os.path.join(stage, "objectives.md"))
        rc, _, err = self.r.run("stage", stage)
        self.assertEqual(rc, 2, err)
        self.assertEqual(self.r.read("product/objectives.md"), OBJECTIVES)

    def test_stage_preserves_existing_scratch_drafts(self):
        stage = self.out("stage")
        os.mkdir(stage)
        with open(os.path.join(stage, "backlog.md"), "w", encoding="utf-8") as f:
            f.write("Reviewed but not yet filed draft\n")
        rc, _, err = self.r.run("stage", stage)
        self.assertEqual(rc, 2, err)
        with open(os.path.join(stage, "backlog.md"), encoding="utf-8") as f:
            self.assertEqual(f.read(), "Reviewed but not yet filed draft\n")
        self.assertEqual(os.listdir(stage), ["backlog.md"])


class Scoring(unittest.TestCase):
    def test_urgency_by_distance(self):
        self.assertEqual([pm.urgency_for(s, 1) for s in ("0", "1", "2", "3", "later")], [5, 5, 3, 2, 1])

    def test_rounds_half_up(self):
        self.assertEqual(pm.score_of(4, 5, 4, 4), "3.3")
        self.assertEqual(pm.score_of(5, 3, 3, 4), "2.8")
        self.assertEqual(pm.score_of(2, 5, 2, 1), "9.0")


if __name__ == "__main__":
    unittest.main()
