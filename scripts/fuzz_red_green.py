"""Require every fuzz input a change adds to fail before the change and pass after it.

A fix that commits a failing input under <package>/testdata/fuzz/<Target>/ claims
the input reproduces the bug. This replays each added input against the base
commit's code, where it must fail, and against the change's code, where it must
pass. An input for a fuzz target the base does not have yet seeds a new target
and is not checked against the base.
"""

import argparse
from pathlib import Path
import re
import subprocess
import sys
import tempfile

INPUT = re.compile(r"^(?:(.+)/)?testdata/fuzz/([^/]+)/([^/]+)$")
TIMEOUT = "120s"


def added_inputs(base, head="HEAD", cwd="."):
    """(package, target, name, path) for each fuzz input added between base and head."""
    names = subprocess.run(["git", "diff", "--no-renames", "--diff-filter=A", "--name-only", base, head],
                           cwd=cwd, check=True, capture_output=True, text=True).stdout.split()
    return [("./" + m[1] if m[1] else ".", m[2], m[3], path)
            for path in names if (m := INPUT.match(path))]


def replay(tree, package, target, name):
    """Run one committed input as its fuzz target's seed; return (exit code, output)."""
    pattern = f"^{re.escape(target)}$/^{re.escape(name)}$"
    run = subprocess.run(["timeout", "--kill-after=5s", TIMEOUT, "go", "test", package, "-run", pattern,
                          "-count=1"], cwd=tree, capture_output=True, text=True)
    return run.returncode, run.stdout + run.stderr


def verdict(before, after):
    """Classify one input from its base and head replays: (ok, reason)."""
    (base_code, base_out), (head_code, _) = before, after
    if head_code != 0:
        return False, f"fails with the change (exit {head_code})"
    if "no tests to run" in base_out:
        return True, "new fuzz target; nothing to compare on the base"
    if re.search(r"\[(?:build|setup) failed\]", base_out):
        return False, "the base does not build, so it cannot show the input failing"
    if base_code == 0:
        return False, "passes on the base commit, so it does not reproduce the bug the change fixes"
    return True, f"fails on the base (exit {base_code}), passes with the change"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", required=True, help="commit the change is compared against")
    args = parser.parse_args()
    inputs = added_inputs(args.base)
    if not inputs:
        print("No fuzz inputs added.")
        return 0
    failed = 0
    with tempfile.TemporaryDirectory() as scratch:
        base_tree = Path(scratch) / "base"
        subprocess.run(["git", "worktree", "add", "--detach", str(base_tree), args.base], check=True,
                       capture_output=True)
        try:
            for package, target, name, path in inputs:
                copy = base_tree / path
                copy.parent.mkdir(parents=True, exist_ok=True)
                copy.write_bytes(Path(path).read_bytes())
                before = replay(base_tree, package, target, name)
                copy.unlink()
                after = replay(".", package, target, name)
                ok, reason = verdict(before, after)
                print(f"{'ok  ' if ok else 'FAIL'} {path}: {reason}")
                if not ok:
                    failed += 1
                    print(f"::error file={path}::{reason}")
                    for label, (code, output) in (("base", before), ("change", after)):
                        print(f"--- {label} (exit {code})\n{output.rstrip()}")
        finally:
            subprocess.run(["git", "worktree", "remove", "--force", str(base_tree)], check=False,
                           capture_output=True)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
