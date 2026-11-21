#!/usr/bin/env python3
"""Resume generated suspended-stack programs across engines.

Reads FuzzSuspendedStackCheckpoint's Go fuzz corpus files (one ``[]byte``
value, the generator's choice bytes) from the given directories and renders
each with ``cmd/moo-fuzz-source --generator suspended-stack``. Every distinct
program becomes one test of a generated YAML suite, run by the managed
conformance harness through ``scripts/cross-engine-session.sh`` once with
Barn writing the checkpoint and Toast resuming it, and once the other way.

Each test, on a fresh copy of the suite's Test.db:

1. installs the program as a verb and runs it in a forked task (copy A),
   resuming every task it suspends until none is left: the run with no
   restart;
2. deletes A's ``#0.suspended_stack`` (the generator defines it), starts the
   program again (copy B) and lets its tasks reach their suspensions;
3. checkpoints with ``dump_database()`` and restarts on the other engine;
4. resumes every task of B until none is left;
5. requires B's result to equal A's: the root task's return value (or its
   error code and message) and every property of the program's test object,
   with object values replaced by a marker, since A and B create different
   objects.

A task the second engine did not restore leaves B unfinished and so differs
from A; a server that does not boot fails the restart step. Every failed
generated test is listed under ``## divergence``, the form
``scripts/fuzz_report.py`` reports, with its failure class (direction and the
step that failed) as the issue signature.

Programs run in chunks of CHUNK, one harness session per chunk and
direction, in the order sha256(--seed + name) gives; with --minutes no chunk
starts after that time, so a cycle covers part of a large corpus and the next
cycle's seed starts elsewhere.

Exit status: 0 every test passed in both directions, 1 divergences (listed in
``<out>/report.md``), 2 inconclusive (no programs, no harness results, or a
harness admission test failed).
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import shutil
import subprocess
import sys
import tempfile
import time
import xml.etree.ElementTree as ElementTree
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from bench_differ import moo_str  # noqa: E402
from fuzz_differential import is_choices_record, render_choices  # noqa: E402

REPO = Path(__file__).resolve().parent.parent
SESSION = REPO / "scripts" / "cross-engine-session.sh"
DIRECTIONS = {"barn-first": "Barn writes, Toast resumes", "toast-first": "Toast writes, Barn resumes"}
TEST_PREFIX = "generated_"
# Rounds of resuming before a test gives up on its tasks finishing.
DRAIN_ROUNDS = 500
# Programs per harness session; the time budget is checked between chunks.
CHUNK = 25
# What failed, by the number that starts the failing step's code.
STEPS = {
    "1": "the program did not install",
    "2": "the tasks of the run with no restart did not finish",
    "3": "the second copy did not start",
    "4": "the checkpoint failed",
    "6": "the restored tasks did not finish",
    "7": "the result differs after the restart",
}

# Verbs of the helper object #0.cross_engine. go runs the program and keeps
# what it returned or raised; drain resumes every task queued since the test
# began until none is left; snap describes the program's test object.
GO = [
    "which = args[1];",
    "try",
    '  r = {"return", this:run()};',
    "except e (ANY)",
    '  r = {"error", e[1], e[2]};',
    "endtry",
    'this.(which + "_result") = r;',
]
DRAIN = [
    f"for i in [1..{DRAIN_ROUNDS}]",
    "  live = {};",
    "  for t in (queued_tasks())",
    "    if (!(t[1] in this.base))",
    "      live = {@live, t[1]};",
    "    endif",
    "  endfor",
    "  if (!live)",
    "    return 1;",
    "  endif",
    "  for id in (live)",
    "    `resume(id) ! ANY';",
    "  endfor",
    "  suspend(0);",
    "endfor",
    f'return {{"tasks still queued after {DRAIN_ROUNDS} rounds of resuming", live}};',
]
SNAP = [
    "o = args[1];",
    "if (typeof(o) != OBJ || !valid(o))",
    '  return "no test object";',
    "endif",
    "s = {};",
    "for p in (properties(o))",
    "  x = o.(p);",
    '  s = {@s, {p, typeof(x) == OBJ ? "an object" | x}};',
    "endfor",
    "return s;",
]


def moo_list(lines: list[str]) -> str:
    return "{" + ", ".join(moo_str(line) for line in lines) + "}"


def install(name: str, code: list[str]) -> list[str]:
    return [
        f'add_verb(h, {{player, "rxd", "{name}"}}, {{"this", "none", "this"}});',
        f"c = set_verb_code(h, \"{name}\", {moo_list(code)});",
        f'if (c) raise(E_INVARG, "{name} does not compile", c); endif',
    ]


def test_case(name: str, source: str) -> dict:
    """The generated YAML test that runs source with and without a restart.
    Each step's code starts with a string naming it, which a failure message
    quotes (failure_class)."""
    setup = [
        '"1 install";',
        "h = create(#-1);",
        'add_property(#0, "cross_engine", h, {player, ""});',
        'for name in ({"base", "a_result", "b_result", "a_o"}) add_property(h, name, 0, {player, ""}); endfor',
        "h.base = {};",
        "for t in (queued_tasks()) h.base = {@h.base, t[1]}; endfor",
        *install("run", source.split("\n")),
        *install("go", GO),
        *install("drain", DRAIN),
        *install("snap", SNAP),
        'fork (0) h:go("a"); endfork',
        "return 0;",
    ]
    second = [
        '"3 start B";',
        "h = #0.cross_engine;",
        "h.a_o = `#0.suspended_stack ! ANY => #-1';",
        "`delete_property(#0, \"suspended_stack\") ! ANY';",
        'fork (0) h:go("b"); endfork',
        # B's tasks run to their suspensions before the checkpoint.
        "for i in [1..5] suspend(0); endfor",
        "return 0;",
    ]
    compare = [
        '"7 compare";',
        "h = #0.cross_engine;",
        "a = {h.a_result, h:snap(h.a_o)};",
        "b = {h.b_result, h:snap(`#0.suspended_stack ! ANY => #-1')};",
        'return equal(a, b) ? 1 | {"no restart", a, "restarted", b};',
    ]
    return {
        "name": name,
        "description": "The generated program ends the same way after a checkpoint the other engine resumes.",
        "permission": "wizard",
        "timeout_ms": 120000,
        "steps": [
            {"run": "\n".join(setup), "expect": {"value": 0}},
            {"run": '"2 run A";\nreturn #0.cross_engine:drain();', "expect": {"value": 1}},
            {"run": "\n".join(second), "expect": {"value": 0}},
            {"run": '"4 checkpoint";\nreturn dump_database();', "expect": {"value": 0}},
            {"restart_server": {"wait_ms": 300, "checkpoint_timeout_ms": 10000}},
            {"run": '"6 resume B";\nreturn #0.cross_engine:drain();', "expect": {"value": 1}},
            {"run": "\n".join(compare), "expect": {"value": 1}},
        ],
    }


def suite(programs: list[dict]) -> dict:
    """The generated suite. JSON is YAML, so it is written as JSON."""
    return {
        "name": "cross_engine_generated",
        "description": "Generated suspended-stack programs resumed by the other engine after a checkpoint.",
        "requires": {"config": ["managed_server"]},
        "tests": [test_case(program["name"], program["source"]) for program in programs],
    }


def load_programs(dirs: list[Path]) -> tuple[list[dict], dict[str, dict]]:
    """Distinct rendered programs, each with its first corpus record, and
    per-lane counts. Each corpus directory is one lane."""
    records: list[tuple[str, str]] = []
    lanes: dict[str, dict] = {}
    for directory in dirs:
        lane = str(directory)
        lanes[lane] = {"records": 0, "duplicates": 0, "programs": 0, "undecodable": []}
        if not directory.is_dir():
            print(f"corpus directory missing: {directory}", file=sys.stderr)
            continue
        for path in sorted(directory.iterdir()):
            if path.is_file():
                lanes[lane]["records"] += 1
                if is_choices_record(path.read_bytes()):
                    records.append((lane, str(path.resolve())))
                else:
                    lanes[lane]["undecodable"].append({"path": str(path), "error": "not one []byte value"})
    rendered = render_choices([path for _, path in records], "suspended-stack")
    programs, seen = [], set()
    for lane, path in records:
        record = rendered[path]
        source = record.get("source")
        if source is None:
            lanes[lane]["undecodable"].append({"path": path, "error": record.get("error")})
        elif source in seen:
            lanes[lane]["duplicates"] += 1
        else:
            seen.add(source)
            lanes[lane]["programs"] += 1
            # moo-fuzz-source omits empty fields: no choices means zero bytes.
            choices = record.get("choices", "")
            digest = hashlib.sha256(bytes.fromhex(choices)).hexdigest()[:16]
            programs.append({"name": TEST_PREFIX + digest, "origin": path, "lane": lane,
                             "choices": choices, "source": source})
    return programs, lanes


def junit_results(path: Path) -> dict[str, dict]:
    """Test case name -> {"outcome": passed|failed|skipped, "message"}. The
    message is the exception text pytest records, without its traceback."""
    results = {}
    for case in ElementTree.parse(path).iter("testcase"):
        outcome, message = "passed", ""
        for child in case:
            if child.tag in ("failure", "error"):
                outcome, message = "failed", child.get("message") or child.text or ""
            elif child.tag == "skipped" and outcome == "passed":
                outcome, message = "skipped", child.get("message") or ""
        results[case.get("name", "")] = {"outcome": outcome, "message": message.strip()}
    return results


def failure_class(direction: str, message: str) -> str:
    """The failure's direction and what failed: the step the harness names by
    the start of its code (each step's code starts with its STEPS key), or
    the restart, which names no step."""
    match = re.search(r"""step '"(\d) """, message)
    if "RestartCommandError" in message:
        what = "the other engine did not boot on the checkpoint"
    elif match and match[1] in STEPS:
        what = STEPS[match[1]]
    else:
        what = "the test failed"
    return f"{direction}: {what}"


def classify(programs: list[dict], sessions: list[tuple[str, list[dict], dict | None]]
             ) -> tuple[list[dict], list[str]]:
    """One row per program and direction, and the session problems that make
    the run inconclusive. Each session is (direction, its programs, its
    harness results or None)."""
    rows, problems = [], []
    for direction, chunk, results in sessions:
        if results is None:
            problems.append(f"{direction}: the harness wrote no results")
            results = {}
        for name, result in results.items():
            if TEST_PREFIX not in name and result["outcome"] == "failed":
                problems.append(f"{direction}: harness test {name} failed: {result['message'][:2000]}")
        for program in chunk:
            found = [result for name, result in results.items() if program["name"] in name]
            if len(found) != 1:
                kind, message = "missing", f"{len(found)} harness results for {program['name']}"
            else:
                kind = {"passed": "match", "failed": "divergence"}.get(found[0]["outcome"], found[0]["outcome"])
                message = found[0]["message"]
            row = {**program, "direction": direction, "kind": kind, "message": message}
            if kind == "divergence":
                row["class"] = failure_class(direction, message)
            rows.append(row)
    if any(row["kind"] in ("missing", "skipped") for row in rows):
        problems.append("a generated test has no pass or failure in a session")
    return rows, problems


def summary_line(message: str) -> str:
    """The failure on one line: from "expected" on when the harness compared a
    value; for a server that did not boot, the exception's first line and the
    last line of the server log tail it carries (where an engine says why it
    stopped); else the exception text."""
    lines = [line.strip() for line in message.splitlines() if line.strip()]
    text = " ".join(" ".join(lines).split())
    start = text.find("expected value")
    if start >= 0:
        text = text[start:]
    elif "--- end server log tail ---" in lines:
        tail = lines[:lines.index("--- end server log tail ---")]
        text = f"{lines[0]} {tail[-1]}" if len(tail) > 1 else lines[0]
    return text[:1000]


def identify(binary: Path) -> str:
    return hashlib.sha256(binary.read_bytes()).hexdigest()


def render_report(summary: dict, rows: list[dict]) -> str:
    report = [
        "# Cross-engine checkpoint of generated suspended stacks", "",
        f"- outcome: {summary['outcome']}",
        f"- programs: {summary['programs']}; results: {json.dumps(summary['counts'], sort_keys=True)}",
        f"- problems: {json.dumps(summary['problems'])}",
        f"- toast binary: `{summary['toast']}` sha256 `{summary['toast_sha256']}`",
        f"- barn binary: `{summary['barn']}` sha256 `{summary['barn_sha256']}`", "",
        "## lanes", "",
    ]
    for lane, counts in summary["lanes"].items():
        shown = {name: value for name, value in counts.items() if name != "undecodable"}
        report.append(f"- `{lane}`: {json.dumps(shown)}; undecodable: {len(counts['undecodable'])}")
    report.append("")
    # One issue per failure class (fuzz_report.py's signature line), not per
    # input: the inputs run differ from cycle to cycle.
    listed = sorted((row for row in rows if row["kind"] == "divergence"), key=lambda row: (row["class"], row["name"]))
    if listed:
        report += [f"## divergence ({len(listed)})", ""]
        for row in listed:
            report += [f"### {row['name']} `{Path(row['origin']).name}`", "",
                       f"- signature: {row['class']}",
                       f"- lane: `{row['lane']}`", f"- choices (hex): `{row['choices']}`", "",
                       "```moo", row["source"], "```", "",
                       f"- {DIRECTIONS[row['direction']]}: {summary_line(row['message'])}",
                       f"- test: `{row['name']}` in `{row['direction']}`", "",
                       "```text", row["message"][:6000], "```", ""]
    return "\n".join(report)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("corpus", type=Path, nargs="+", help="FuzzSuspendedStackCheckpoint corpus directories")
    ap.add_argument("--barn", type=Path, required=True, help="Linux barn binary")
    ap.add_argument("--toast", type=Path, required=True, help="stock ToastStunt moo binary")
    ap.add_argument("--suite", type=Path, required=True, help="moo-conformance-tests checkout")
    ap.add_argument("--out", type=Path, default=REPO / ".tmp" / "fuzz_cross_engine")
    # The nightly corpus holds thousands of inputs at about 4 s per test and
    # direction, so a cycle runs what fits and the next cycle starts elsewhere.
    ap.add_argument("--minutes", type=float, help="start no new chunk of programs after this many minutes")
    ap.add_argument("--seed", default="", help="order the programs by sha256(seed + name); the nightly passes its run id")
    args = ap.parse_args()

    programs, lanes = load_programs(args.corpus)
    if not programs:
        print("no suspended-stack programs in the corpus", file=sys.stderr)
        return 2
    programs.sort(key=lambda program: hashlib.sha256((args.seed + program["name"]).encode()).hexdigest())
    deadline = None if args.minutes is None else time.monotonic() + args.minutes * 60
    args.out.mkdir(parents=True, exist_ok=True)
    # Candidate suites and databases must lie inside this checkout.
    (REPO / ".tmp").mkdir(exist_ok=True)
    work = Path(tempfile.mkdtemp(prefix="cross-engine-generated.", dir=REPO / ".tmp"))
    sessions: list[tuple[str, list[dict], dict | None]] = []
    ran: list[dict] = []
    try:
        shutil.copytree(args.suite / "src" / "moo_conformance" / "_db", work / "db")
        for start in range(0, len(programs), CHUNK):
            if deadline is not None and ran and time.monotonic() >= deadline:
                break
            chunk = programs[start:start + CHUNK]
            index = start // CHUNK + 1
            root = work / f"suite-{index:03d}"
            root.mkdir()
            text = json.dumps(suite(chunk), indent=1)
            (root / "cross_engine_generated.yaml").write_text(text, encoding="utf-8")
            (args.out / f"cross_engine_generated-{index:03d}.yaml").write_text(text, encoding="utf-8")
            for direction, label in DIRECTIONS.items():
                print(f"=== chunk {index}: {label} ===", flush=True)
                junit = args.out / f"{direction}-{index:03d}.xml"
                junit.unlink(missing_ok=True)
                subprocess.run(["bash", str(SESSION), str(args.barn), str(args.toast), str(args.suite), direction,
                                f"--candidate-root={REPO}", f"--moo-suite-root={root}",
                                f"--server-db={work / 'db' / 'Test.db'}",
                                f"--server-db-dir={work / 'db' / 'startup'}",
                                f"--junitxml={junit}"], check=False)
                sessions.append((direction, chunk, junit_results(junit) if junit.exists() else None))
            ran += chunk
    finally:
        shutil.rmtree(work, ignore_errors=True)

    rows, problems = classify(ran, sessions)
    counts: dict[str, int] = {}
    for row in rows:
        counts[row["kind"]] = counts.get(row["kind"], 0) + 1
    status = 2 if problems else 1 if counts.get("divergence") else 0
    summary = {"outcome": {0: "success", 1: "divergence", 2: "inconclusive"}[status],
               "programs": len(programs), "ran": len(ran), "seed": args.seed, "minutes": args.minutes,
               "counts": counts, "problems": problems, "lanes": lanes,
               "toast": str(args.toast), "toast_sha256": identify(args.toast),
               "barn": str(args.barn), "barn_sha256": identify(args.barn)}
    text = render_report(summary, rows)
    (args.out / "report.md").write_text(text, encoding="utf-8")
    (args.out / "results.json").write_text(json.dumps({"summary": summary, "rows": rows}, indent=2),
                                           encoding="utf-8")
    print(text)
    if problems:
        print("[setup failed] " + "; ".join(problems)[:4000])
    return status


if __name__ == "__main__":
    sys.exit(main())
