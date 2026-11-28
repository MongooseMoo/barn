"""Decide whether an automatic repair's change may merge without a person reading it.

A repair is in scope when it proves it fixes a reproduced failure, either by adding a fuzz input under <package>/testdata/fuzz/<Target>/ that fuzz_red_green.py replays red before the change and green after it, or by a conformance test that the managed harness passes on Toast, fails on Barn at the merge base, and passes on Barn at the repair's head; and when it leaves every existing check alone: it changes, deletes or renames no existing test data, workflow, script, profile or agent rules file, and removes no line from an existing Go test.

Adding new tests, new test data, and new lines in an existing Go test is allowed.
The conformance proof is the JSON evidence file the automatic repair workflow
writes after running the conformance branch's added tests three times
(--conformance-evidence). Exit 0 when in scope, 1 when not; the reasons are
printed one per line.
"""

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from fuzz_red_green import INPUT  # noqa: E402

# Paths whose existing content decides whether a change is correct.
# (pattern, what it is, whether a change may add new files of this kind)
GUARDED = (
    (re.compile(r"(^|/)[^/]+_test\.go$"), "Go test", True),
    (re.compile(r"(^|/)testdata/"), "test data", True),
    (re.compile(r"^tests/"), "conformance regression", False),
    (re.compile(r"^\.github/"), "workflow file", False),
    (re.compile(r"^scripts/"), "script", False),
    (re.compile(r"^profiles/"), "runtime or oracle profile", False),
    (re.compile(r"(^|/)(CLAUDE|AGENTS)\.md$"), "agent rules file", False),
    (re.compile(r"^(Makefile|\.golangci\.yml|go\.mod|go\.sum)$"), "build or lint configuration", False),
)
VERBS = {"A": "adds", "D": "deletes", "M": "changes"}


def changes(base, head="HEAD", cwd="."):
    """(status, path, lines removed) per file the change touches; a rename is a delete plus an add."""
    def diff(flag):
        out = subprocess.run(["git", "diff", "--no-renames", flag, base, head],
                             cwd=cwd, check=True, capture_output=True, text=True).stdout
        return [line.split("\t") for line in out.splitlines() if line]

    # A binary file has "-" for its counts; any change to one counts as removing lines.
    removed = {path: int(gone) if gone.isdigit() else 1 for _, gone, path in diff("--numstat")}
    return [(status, path, removed.get(path, 0)) for status, path in diff("--name-status")]


def _selected(run):
    """The run's selected test ids, without capability admission."""
    ids = run.get("passed", []) + run.get("failed", []) + run.get("skipped", [])
    return frozenset(i for i in ids if "capability_admission" not in i)


def _admitted(run):
    return any("capability_admission" in i for i in run.get("passed", [])) and         not any("capability_admission" in i for i in run.get("failed", []) + run.get("skipped", []))


def _green(run):
    return run.get("exit") == 0 and _admitted(run) and not run.get("failed") and not run.get("skipped")


def conformance_proves(evidence):
    """True when one non-empty test selection passed on Toast, failed on Barn at the merge base and passed at the head."""
    if not evidence or not evidence.get("files"):
        return False
    runs = evidence.get("runs", {})
    toast, base, head = (runs.get(name) for name in ("toast", "barn_base", "barn_head"))
    if not (toast and base and head):
        return False
    if any(run.get("errors") for run in (toast, base, head)):
        return False
    selection = _selected(toast)
    if not selection or _selected(base) != selection or _selected(head) != selection:
        return False
    base_red = base.get("exit") != 0 and _admitted(base) and bool(set(base.get("failed", [])) & selection)
    return _green(toast) and base_red and _green(head)


def out_of_scope(changed, evidence=None):
    """Reasons the change needs a decision; empty when it is in scope."""
    reasons = []
    if not any(status == "A" and INPUT.match(path) for status, path, _ in changed) and             not conformance_proves(evidence):
        reasons.append("adds no fuzz input under testdata/fuzz, so nothing proves it fixes a reproduced failure")
    for status, path, removed in changed:
        for pattern, what, may_add in GUARDED:
            if pattern.search(path):
                if status == "M" and what == "Go test":
                    if removed:
                        reasons.append(f"removes {removed} line(s) from Go test {path}")
                elif not (status == "A" and may_add):
                    reasons.append(f"{VERBS.get(status, 'touches')} {what} {path}")
                break
    return reasons


def load_bound_evidence(path, base, head, suite):
    if path is None or not path.is_file() or not suite:
        return None
    evidence = json.loads(path.read_text(encoding="utf-8"))
    if (evidence.get("base"), evidence.get("head"), evidence.get("suite")) != (base, head, suite):
        return None
    return evidence


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", required=True, help="commit the change is compared against")
    parser.add_argument("--head", default="HEAD", help="the change's commit")
    parser.add_argument("--suite-head", help="exact published suite head used by the proof")
    parser.add_argument("--conformance-evidence", type=Path,
                        help="JSON record of the conformance test's Toast, merge-base and head runs")
    args = parser.parse_args()
    evidence = load_bound_evidence(args.conformance_evidence, args.base, args.head, args.suite_head)
    reasons = out_of_scope(changes(args.base, args.head), evidence)
    for reason in reasons:
        print(reason)
    if not reasons:
        print("In scope: proves it fixes a reproduced failure and leaves existing checks unchanged.")
    return 1 if reasons else 0


if __name__ == "__main__":
    sys.exit(main())
