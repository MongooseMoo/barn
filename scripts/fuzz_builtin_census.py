#!/usr/bin/env python3
"""Per-builtin coverage census of one nightly fuzz cycle (issue #978).

For each builtin the given Toast binary registers (its ``function_info()``),
counts

- compared programs: rows of the Toast differential's ``results.json`` whose
  Toast and Barn records both completed as a probe outcome (the
  ``completed_pairs`` test of fuzz_differential.py) and that compiled on both
  engines, and whose source calls the builtin. Calls are read by Barn's parser
  (``cmd/moo-fuzz-source --builtins``); ``call_function("name", ...)`` with a
  literal name counts as a call to that name. A call that execution never
  reached still counts.
- conformance tests: tests of the moo-conformance-tests suite and of Barn's
  ``tests/mongoose-conformance`` whose code calls the builtin, found by the
  suite's own scan (moo_conformance.builtin_coverage.collect_builtin_calls), a
  name-followed-by-parenthesis scan outside string literals. It is an upper
  bound: any such name in test code counts. Suite setup and teardown count as
  one test per file, named ``suite``.

Writes to --out:

- ``builtin-census.tsv``: one builtin per line, sorted: name, compared
  programs, conformance tests.
- ``zero-coverage-builtins.txt``: the builtins at zero on both, in the format
  of the committed list (copy it over the committed list to accept a change).
- ``builtin-census.json``: totals, the zero list, and its difference from the
  committed list: ``newly_zero`` (a builtin that lost all coverage) and
  ``newly_covered`` (a listed builtin that gained some). The cycle's
  acceptance job (fuzz_compute.cjs accept) rejects the cycle when either is
  non-empty.

Exit status 0 when the census was written, whatever its difference from the
committed list; nonzero when it could not be taken.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from bench_differ import REPO, TOAST, lane_scripts, run_wsl, write_inputs  # noqa: E402
from fuzz_differential import outcome  # noqa: E402

ZERO_LIST = REPO / "scripts" / "fuzz_zero_coverage_builtins.txt"
ZERO_LIST_HEADER = (
    "# Builtins the pinned Toast registers that no compared generated program\n"
    "# and no conformance test calls. Kept by scripts/fuzz_builtin_census.py: the\n"
    "# nightly fuzz cycle fails when this list and its census differ.\n"
)
# Every builtin's name, as a list of strings on one result line.
FUNCTION_NAMES = ";;r = {}; for f in (function_info()) r = {@r, f[1]}; endfor return r;"

# Run by the conformance suite's own interpreter: argv is the builtin names
# (JSON) and the test roots; prints {builtin: sorted test ids}.
CONFORMANCE_SCAN = r"""
import json, re, sys
from pathlib import Path
from moo_conformance.builtin_coverage import collect_builtin_calls
names = set(json.loads(sys.argv[1]))
tests = {}
for root in sys.argv[2:]:
    for call in collect_builtin_calls(Path(root), names):
        test = re.sub(r"\.(setup|teardown|expect|steps|cleanup)\b.*$", "", call.context)
        tests.setdefault(call.name, set()).add(f"{Path(root).name}/{call.path}::{test}")
print(json.dumps({name: sorted(ids) for name, ids in tests.items()}))
"""


def toast_builtins(toast: str, db: Path, out: Path) -> list[str]:
    """The sorted names of every builtin the Toast binary registers."""
    path = out / "toast_function_info_in.txt"
    write_inputs(path, [FUNCTION_NAMES], "toast")
    # Only the Toast lane runs, so the Barn binary argument is unused.
    proc = run_wsl(lane_scripts(db, REPO, toast, path, path, 60)["toast"], 120)
    (out / "toast_function_info.txt").write_text(f"exit {proc.returncode}\n{proc.stdout}\n--- STDERR ---\n"
                                                 f"{proc.stderr}", encoding="utf-8")
    lines = [line for line in proc.stdout.splitlines() if line.startswith("=> {")]
    if proc.returncode != 0 or len(lines) != 1:
        raise RuntimeError(f"function_info() on {toast} exited {proc.returncode} with {len(lines)} result lines")
    names = re.findall(r'"([a-z_0-9]+)"', lines[0])
    if not names:
        raise RuntimeError(f"function_info() on {toast} named no builtins")
    return sorted(set(names))


def compared_sources(results: dict) -> list[str]:
    """Sources of the rows run on both engines and compared, excluding those
    that failed to compile on either (they call nothing)."""
    sources = []
    for row in results["rows"]:
        pair = (outcome(row["key"], row["toast_raw"]), outcome(row["key"], row["barn_raw"]))
        if None not in pair and not any(side.startswith("compile ") for side in pair):
            sources.append(row["source"])
    return sources


def builtin_calls(sources: list[str]) -> list[dict]:
    """{"builtins": [...]} or {"error": ...} per source, from Barn's parser."""
    if not sources:
        return []
    proc = subprocess.run(["go", "run", "./cmd/moo-fuzz-source", "--builtins"], cwd=REPO,
                          input="".join(json.dumps(source) + "\n" for source in sources),
                          capture_output=True, text=True, encoding="utf-8", check=True)
    calls = [json.loads(line) for line in proc.stdout.splitlines()]
    if len(calls) != len(sources):
        raise RuntimeError(f"moo-fuzz-source --builtins answered {len(calls)} of {len(sources)} programs")
    return calls


def conformance_tests(suite: Path, roots: list[Path], names: list[str]) -> dict[str, list[str]]:
    proc = subprocess.run(["uv", "run", "--project", str(suite), "--frozen", "python", "-I", "-c",
                           CONFORMANCE_SCAN, json.dumps(names), *map(str, roots)],
                          capture_output=True, text=True, encoding="utf-8")
    if proc.returncode != 0:
        raise RuntimeError(f"conformance scan exited {proc.returncode}: {proc.stderr.strip()}")
    return json.loads(proc.stdout)


def read_zero_list(text: str) -> list[str]:
    return sorted(line.strip() for line in text.splitlines() if line.strip() and not line.startswith("#"))


def render_zero_list(names: list[str]) -> str:
    return ZERO_LIST_HEADER + "".join(name + "\n" for name in sorted(names))


def census(names: list[str], calls: list[dict], tests: dict[str, list[str]], listed: list[str]) -> dict:
    """Per-builtin counts, the zero set and its difference from listed."""
    compared = {name: 0 for name in names}
    for call in calls:
        for name in call.get("builtins", []):
            if name in compared:
                compared[name] += 1
    rows = {name: [compared[name], len(tests.get(name, []))] for name in names}
    zero = sorted(name for name, (programs, conformance) in rows.items() if not programs and not conformance)
    return {
        "builtins": rows,
        "zero": zero,
        "newly_zero": sorted(set(zero) - set(listed)),
        "newly_covered": sorted(set(listed) - set(zero)),
        "unparsed_programs": sum(1 for call in calls if "error" in call),
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--results", type=Path, required=True, help="the differential's results.json")
    ap.add_argument("--suite", type=Path, required=True, help="moo-conformance-tests checkout")
    ap.add_argument("--toast", default=TOAST, help="Toast binary (path inside WSL on Windows)")
    ap.add_argument("--db", type=Path, default=REPO / "Test_conf.db")
    ap.add_argument("--out", type=Path, required=True)
    args = ap.parse_args()

    args.out.mkdir(parents=True, exist_ok=True)
    names = toast_builtins(args.toast, args.db, args.out)
    results = json.loads(args.results.read_text(encoding="utf-8"))
    sources = compared_sources(results)
    calls = builtin_calls(sources)
    roots = [args.suite / "src/moo_conformance/_tests", REPO / "tests/mongoose-conformance"]
    tests = conformance_tests(args.suite, roots, names)
    listed = read_zero_list(ZERO_LIST.read_text(encoding="utf-8"))
    result = census(names, calls, tests, listed)
    suite_commit = subprocess.run(["git", "-C", str(args.suite), "rev-parse", "HEAD"], capture_output=True,
                                  text=True, check=True).stdout.strip()

    (args.out / "builtin-census.tsv").write_text(
        "builtin\tcompared_programs\tconformance_tests\n"
        + "".join(f"{name}\t{programs}\t{conformance}\n"
                  for name, (programs, conformance) in sorted(result["builtins"].items())),
        encoding="utf-8", newline="\n")
    (args.out / "zero-coverage-builtins.txt").write_text(render_zero_list(result["zero"]), encoding="utf-8",
                                                         newline="\n")
    summary = {
        "toast": args.toast, "toast_builtins": len(names),
        "differential_rows": len(results["rows"]), "compared_programs": len(sources),
        "unparsed_programs": result["unparsed_programs"],
        "conformance_suite_commit": suite_commit,
        # Each root relative to the checkout it belongs to: the suite's, then Barn's.
        "conformance_roots": [roots[0].relative_to(args.suite).as_posix(), roots[1].relative_to(REPO).as_posix()],
        "conformance_count": "upper bound: test code naming the builtin before '('",
        "covered_by_programs": sum(1 for programs, _ in result["builtins"].values() if programs),
        "covered_by_conformance": sum(1 for _, conformance in result["builtins"].values() if conformance),
        "zero": result["zero"], "zero_list": ZERO_LIST.relative_to(REPO).as_posix(),
        "newly_zero": result["newly_zero"], "newly_covered": result["newly_covered"],
    }
    text = json.dumps(summary, indent=2, sort_keys=True) + "\n"
    (args.out / "builtin-census.json").write_text(text, encoding="utf-8", newline="\n")
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as stream:
            stream.write("## Builtin census\n\n```json\n" + text + "```\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
