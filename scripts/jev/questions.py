"""Every Jev question and threshold the harness uses, in one reviewed file.

Agents write poor classifier questions, so the questions live here, where a human
reviews them, and nowhere else: jev.py only builds state and reads answers. A new
site is a reviewed edit here plus a subcommand plus a test with a kept broken
input, never a runtime flag.

Bump a site's QUESTION_VERSIONS entry in the same edit that changes its wording or
a threshold its findings are banded by, so precision is always read against the
version that produced the findings (jev.py precision).
"""

# --- Thresholds ---------------------------------------------------------------------

# A noul probability at or above this is reported as `yes`.
NOUL_YES_MIN = 0.7
# A noul probability in [REVIEW_BAND_MIN, NOUL_YES_MIN) is reported as `review`:
# the model is unsure, and that is a verdict to surface, never to round.
REVIEW_BAND_MIN = 0.55
# Below this confidence a choice answer is `uncertain`; never act on it.
CLASS_CONFIDENCE_MIN = 0.5
# route_lane.py sends a task to a vendor lane only at or above this class confidence.
TASK_CLASS_CONFIDENCE_MIN = 0.6
# Lines of code around a finding's anchor given to triage-findings.
SNIPPET_CONTEXT_LINES = 8

QUESTION_VERSIONS = {
    "triage-findings": "1",
    "private-material": "1",
    "scope-drift": "1",
    "task-class": "1",
}

TASK_CLASSES = {
    "mechanical_scripted": "A mechanical edit whose every step is spelled out: rename, move, add a table row, wire an existing function into a list",
    "tight_spec_code": "New code whose behaviour, signatures and tests the task specifies exactly, in one or two packages",
    "long_context_investigation": "Work that needs reading a large part of the codebase before changing it: cross-package refactors, tracing a behaviour through many files",
    "concurrency_risky": "Changes to goroutines, locking, process supervision, signals, file locking or anything where ordering and cancellation decide correctness",
    "ui_design": "Terminal user interface layout, interaction or visual design",
    "open_design": "The task leaves real design decisions open: which approach, which API shape, what to name things",
}


def finding_is_real(claim: str, code: str) -> dict:
    return {
        "type": "noul",
        "instructions": {
            "claim": claim,
            "code": code,
            "question": "Does `code` itself show the defect that `claim` describes, at the place it cites?",
        },
        "criteria": {
            "true": "The defect is visible in `code` as the claim states it",
            "false": "`code` does not show it, shows the opposite, or the claim is about something `code` does not contain",
        },
    }


def paragraph_is_private(text: str) -> dict:
    return {
        "type": "noul",
        "instructions": {
            "text": text,
            "question": (
                "Would publishing `text` in a public open-source repository disclose private material: a credential "
                "or where one is stored, a personal e-mail address or a private individual's name, a home-directory "
                "path, an internal hostname or IP address, customer, prospect or company-internal information, "
                "business metrics, or a raw transcript of an agent session?"
            ),
        },
        "criteria": {
            "true": "At least one such item appears in `text`",
            "false": "`text` is about the code and the change only, or uses placeholders such as example.com",
        },
    }


def change_is_needed(task_text: str, path: str, diff: str) -> dict:
    return {
        "type": "noul",
        "instructions": {
            "task": task_text,
            "path": path,
            "diff": diff,
            "question": "Is the change `diff` to `path` needed to carry out `task` as written, rather than unrelated work?",
        },
        "criteria": {
            "true": "The task cannot be done as written without this change (a caller, a test helper, generated output)",
            "false": "The change is unrelated clean-up, a feature the task does not ask for, or scope creep",
        },
    }


def task_class() -> dict:
    return {
        "type": "choice",
        "instructions": {
            "question": "Which class best describes the work the task in `task` asks for?",
            "focus": "Judge the work itself, not how it is phrased. Pick open_design when the task leaves a real decision open.",
        },
        "criteria": TASK_CLASSES,
    }
