"""Decide whether an automatic repair's change may merge without a person reading it.

A repair is in scope when it adds a fuzz input under <package>/testdata/fuzz/<Target>/,
so the required red/green replay (fuzz_red_green.py) proves the input fails before the
change and passes with it, and when it leaves every existing check alone: it changes,
deletes or renames no existing test data, workflow, script, profile or agent rules
file, and removes no line from an existing Go test. Adding new tests, new test data,
and new lines in an existing Go test is allowed.

Exit 0 when in scope, 1 when not; the reasons are printed one per line.
"""

import argparse
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


def out_of_scope(changed):
    """Reasons the change needs a decision; empty when it is in scope."""
    reasons = []
    if not any(status == "A" and INPUT.match(path) for status, path, _ in changed):
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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", required=True, help="commit the change is compared against")
    parser.add_argument("--head", default="HEAD", help="the change's commit")
    args = parser.parse_args()
    reasons = out_of_scope(changes(args.base, args.head))
    for reason in reasons:
        print(reason)
    if not reasons:
        print("In scope: adds a fuzz input and leaves existing checks unchanged.")
    return 1 if reasons else 0


if __name__ == "__main__":
    sys.exit(main())
