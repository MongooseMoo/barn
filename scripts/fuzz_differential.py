#!/usr/bin/env python3
"""Barn vs Toast differential run over Go fuzz corpora of MOO source.

Reads Go fuzz corpus files (``go test fuzz v1`` + one value) from the given
directories -- the committed ``testdata/fuzz/<Target>`` inputs and Go's fuzz
cache, which the nightly campaigns fill. A ``string(...)`` value (vm
FuzzSourceExecution, compiler FuzzCompileVerifiable) is the program. A
``[]byte(...)`` value is the choice bytes of a structured target
(FuzzStructured*); ``cmd/moo-fuzz-source -corpus`` decodes it and renders the
program with internal/fuzzseed, and each row keeps the choice bytes (hex) with
the rendered source. Each directory is one lane with its own counts; a record
neither form decodes is excluded as undecodable. It runs every program through both
engines with bench_differ's lanes (Barn ``-eval-file``, Toast emergency mode),
one process per engine.  Each program becomes one input line::

    try r = eval("line 1", "line 2"); return {"F00001", r[1] ? "value" | "compile", r[1] ? r[2] | 0};
    except e (ANY) return {"F00001", "raised", e[1]}; endtry

so what is compared is whether the program compiles, the value it returns, or
the error code it raises.  Compile error text is not compared (only that
compilation failed).  A preamble bounds every input to FG_TICKS ticks and
FG_SECONDS seconds on both engines.  An input that one engine aborts (ticks,
seconds) has no probe result there; those are listed but not compared,
because tick accounting is not claimed to be Toast-exact.  A recovered Barn
panic is always a divergence.

Exit status: 0 positive completed coverage with no divergence or unknown outcome,
1 divergences (listed in ``<out>/report.md``), 2 inconclusive (zero completed
pairs, one-sided aborts, or malformed/incomplete output). Known both-engine
aborts can coexist with success, but are reported as partial coverage.
"""

from __future__ import annotations

import argparse
import ast
import json
import re
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from bench_differ import REPO, TOAST, build_barn_linux, moo_str, run_lanes, run_wsl  # noqa: E402

FG_TICKS = 30000
FG_SECONDS = 2

PREAMBLE = (
    'for p in ({"fg_ticks", "fg_seconds"}) '
    "try add_property($server_options, p, 0, {$server_options.owner, \"r\"}); except (ANY) endtry endfor "
    f"$server_options.fg_ticks = {FG_TICKS}; $server_options.fg_seconds = {FG_SECONDS}; "
    "return load_server_options();"
)

# Toast reads each emergency-mode line with fgets into a 4096-byte buffer
# (src/dependencies/linenoise.c LINENOISE_MAX_LINE); a longer line would be
# split into two commands.
MAX_LINE = 4000

# Programs naming these are not compared, each for the reason given: their
# results legitimately differ between two runs or two engines.
EXCLUDED_CALLS = {
    # Wall clock and randomness.
    "time": "clock", "ctime": "clock", "ftime": "clock",
    "random": "randomness", "frandom": "randomness", "random_bytes": "randomness",
    "reseed_random": "randomness", "salt": "randomness",
    # Task identity, limits and scheduling: ids and remaining budgets are
    # per-process counters; suspension needs a scheduler neither lane runs.
    "task_id": "task identity", "queued_tasks": "task identity", "kill_task": "task identity",
    "resume": "task identity", "suspend": "scheduler", "read": "scheduler",
    "ticks_left": "tick budget", "seconds_left": "tick budget",
    "queue_info": "task identity", "task_stack": "task identity", "finished_tasks": "task identity",
    # Process and memory facts of the engine, not MOO semantics.
    "memory_usage": "engine memory", "value_bytes": "engine memory", "object_bytes": "engine memory",
    "db_disk_size": "engine memory", "server_version": "engine identity", "usage": "engine memory",
    # Whole-process effects that would end or reshape the rest of the run.
    "shutdown": "process effect", "dump_database": "process effect",
    "load_server_options": "process effect",
}
CALL_RE = re.compile(r"\b(" + "|".join(sorted(EXCLUDED_CALLS)) + r")\s*\(")
# fork returns a task id; $server_options changes the preamble's bounds.
WORD_RE = re.compile(r"\b(fork|server_options)\b")

RESULT_RE = re.compile(r'=> \{"(F\d{5})", "(value|compile|raised)", (.*)\}', re.DOTALL)
# Lines that begin one record of engine output. Any other line continues the
# record before it (a printed string can hold a newline) or, before the first
# record, is Toast's banner. Barn's -eval-file prints one record per input
# (dbtool.EvalFile), plus "-- skipped" for the empty last line; Toast prints
# one "=> ..." per input (server.cc emergency_mode), notify() output as
# "#N <- ..." (server.cc emergency_notify), and "Bye." on quit.
RECORD_START = re.compile(r"(=> |Error: |Compile error: |-- skipped|#-?\d+ <- |\*\* \d+ errors during parsing|Bye\.)")
NOT_A_RESULT = re.compile(r"(-- skipped|#-?\d+ <- |Bye\.)")


def decode_corpus_file(data: bytes) -> str | None:
    """Return the single string value of a Go fuzz corpus file, or None when
    the file is not one string value."""
    lines = data.decode("ascii", errors="replace").splitlines()
    if len(lines) != 2 or lines[0] != "go test fuzz v1":
        return None
    m = re.fullmatch(r'string\(("(?:[^"\\]|\\.)*")\)', lines[1])
    if not m:
        return None
    # Go writes the value with strconv.Quote. For ASCII text its escapes
    # (\a \b \f \n \r \t \v \\ \" \xNN) are Python's too; a \u, \U or \x80+
    # escape means non-ASCII text, which exclusion() drops, so keep it as
    # Python decodes it.
    return ast.literal_eval(m.group(1))


def is_choices_record(data: bytes) -> bool:
    """Whether a Go fuzz corpus file holds one []byte value, the choice
    encoding of the structured targets (internal/fuzzseed)."""
    lines = data.decode("utf-8", errors="replace").splitlines()
    return len(lines) == 2 and lines[0] == "go test fuzz v1" and lines[1].startswith("[]byte(")


def render_choices(paths: list[str]) -> dict[str, dict[str, str]]:
    """Path -> {"choices": hex, "source": MOO} or {"error": text} for each
    structured corpus file, decoded and rendered by cmd/moo-fuzz-source, the
    one owner of the choice encoding and of the generated program."""
    if not paths:
        return {}
    proc = subprocess.run(["go", "run", "./cmd/moo-fuzz-source", "-corpus"], cwd=REPO,
                          input="".join(path + "\n" for path in paths), capture_output=True,
                          text=True, encoding="utf-8", check=True)
    rendered = {}
    for line in proc.stdout.splitlines():
        record = json.loads(line)
        rendered[record.pop("path")] = record
    if sorted(rendered) != sorted(paths):
        raise RuntimeError(f"moo-fuzz-source rendered {len(rendered)} of {len(paths)} structured records")
    return rendered


def exclusion(source: str) -> str | None:
    """Why a program is not compared, or None."""
    # Barn strings are Unicode by design and Toast strings are bytes, so
    # non-ASCII input diverges on purpose. Control characters other than tab
    # and newline cannot travel on one emergency-mode line.
    if any(ord(c) > 0x7E or (ord(c) < 0x20 and c not in "\t\n") for c in source):
        return "non-ASCII or control character"
    m = CALL_RE.search(source)
    if m:
        return f"{m.group(1)}(): {EXCLUDED_CALLS[m.group(1)]}"
    m = WORD_RE.search(source)
    if m:
        return m.group(1)
    return None


def probe(key: str, source: str) -> str:
    lines = ", ".join(moo_str(line) for line in source.split("\n"))
    return (f'try r = eval({lines}); return {{"{key}", r[1] ? "value" | "compile", r[1] ? r[2] | 0}}; '
            f'except e (ANY) return {{"{key}", "raised", e[1]}}; endtry')


def results(stdout: str) -> list[str]:
    """One engine's result record per input, in input order."""
    records: list[str] = []
    for line in stdout.split("\n"):
        if RECORD_START.match(line):
            records.append(line)
        elif records:
            records[-1] += "\n" + line
    return [r.rstrip("\n") for r in records if not NOT_A_RESULT.match(r)]


def outcome(key: str, record: str | None) -> str | None:
    """'value V', 'compile 0' or 'raised E' for a completed probe, else None."""
    m = RESULT_RE.fullmatch(record or "")
    return f"{m.group(2)} {m.group(3)}" if m and m.group(1) == key else None


def load_programs(dirs: list[Path]) -> tuple[dict[str, dict[str, str]], dict[str, int], dict[str, dict]]:
    """Unique compared sources -> their first corpus record (origin path,
    lane, and the choice bytes of a structured record), exclusion counts,
    and per-lane counts. Each corpus directory is one lane."""
    records: list[tuple[str, str, str | None]] = []  # lane, path, string source
    choice_paths: list[str] = []
    lanes: dict[str, dict] = {}
    for directory in dirs:
        lane = str(directory)
        lanes[lane] = {"records": 0, "duplicates": 0, "excluded": 0, "eligible": 0, "completed": 0,
                       "undecodable": []}
        if not directory.is_dir():
            print(f"corpus directory missing: {directory}", file=sys.stderr)
            continue
        for path in sorted(directory.iterdir()):
            if not path.is_file():
                continue
            data = path.read_bytes()
            records.append((lane, str(path.resolve()), decode_corpus_file(data)))
            if is_choices_record(data):
                choice_paths.append(records[-1][1])
    rendered = render_choices(choice_paths)

    programs: dict[str, dict[str, str]] = {}
    excluded: dict[str, int] = {}
    seen = set()
    for lane, path, source in records:
        counts = lanes[lane]
        counts["records"] += 1
        record = {"origin": path, "lane": lane}
        if path in rendered:
            # moo-fuzz-source omits empty fields: no choices means zero bytes.
            record["choices"] = rendered[path].get("choices", "")
            source = rendered[path].get("source")
            error = rendered[path].get("error")
        else:
            error = None if source is not None else "not one string or []byte value"
        if source is None:
            reason = "undecodable corpus record"
            counts["undecodable"].append({"path": path, "error": error})
        elif source in seen:
            counts["duplicates"] += 1
            continue
        else:
            seen.add(source)
            reason = exclusion(source) or (None if len(probe("F00000", source)) <= MAX_LINE
                                           else "longer than one emergency-mode line")
        if reason:
            excluded[reason] = excluded.get(reason, 0) + 1
            counts["excluded"] += 1
        else:
            programs[source] = record
            counts["eligible"] += 1
    return programs, excluded, lanes


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("corpus", type=Path, nargs="+", help="Go fuzz corpus directories")
    # Test_conf.db is byte-identical to the fixture the Toast oracle manifest
    # pins (profiles/toast/stock-wsl-testdb.json database_checksum). Toast
    # segfaults loading the repository's larger Test.db.
    ap.add_argument("--db", type=Path, default=REPO / "Test_conf.db")
    ap.add_argument("--toast", default=TOAST, help="Toast binary (path inside WSL on Windows)")
    ap.add_argument("--barn-linux", type=Path, help="prebuilt linux barn (default: cross-compile now)")
    ap.add_argument("--out", type=Path, default=REPO / ".tmp" / "fuzz_differential")
    ap.add_argument("--timeout", type=int, default=900, help="seconds per engine lane")
    args = ap.parse_args()

    programs, excluded, corpus_lanes = load_programs(args.corpus)
    if not programs:
        print("no comparable programs in the corpus", file=sys.stderr)
        return 2
    args.out.mkdir(parents=True, exist_ok=True)
    barn_bin = args.barn_linux or (args.out / "barn_linux")
    if not args.barn_linux:
        build_barn_linux(barn_bin)

    keyed = [("F%05d" % i, source, record) for i, (source, record) in enumerate(programs.items(), 1)]
    inputs = [(line, ";;" + line) for line in [PREAMBLE] + [probe(k, s) for k, s, _ in keyed]]
    # The nightly job builds its own Toast from the pinned ref, so the report
    # names the exact binary: sha256 and its -v version line. Toast's -v
    # prints to stderr and exits 1 (server.cc main, case 'v').
    ident = run_wsl(f"set -euo pipefail; sha256sum {args.toast} | cut -d' ' -f1; {args.toast} -v 2>&1 || true", 60)
    toast_sha, toast_version = (ident.stdout.splitlines() + ["", ""])[:2]
    if ident.returncode != 0 or not toast_version.startswith("ToastStunt version "):
        print(f"cannot identify toast binary: {ident.stdout.strip()} {ident.stderr.strip()}", file=sys.stderr)
        return 2
    lanes = run_lanes(args.db, barn_bin, args.toast, inputs, args.out, args.timeout)
    records = {lane: results(stdout) for lane, (stdout, _) in lanes.items()}
    record_errors = []
    for lane, got in records.items():
        if len(got) != len(inputs):
            # Without one record per input the positions mean nothing.
            message = f"{lane}: {len(got)} result records for {len(inputs)} inputs; see {lane}_raw.txt"
            print(message, file=sys.stderr)
            record_errors.append(message)

    rows = []
    completed = 0
    for index, (key, source, record) in enumerate(keyed if not record_errors else [], 1):
        t_raw, b_raw = records["toast"][index], records["barn"][index]
        t, b = outcome(key, t_raw), outcome(key, b_raw)
        if t is not None and b is not None:
            completed += 1
            corpus_lanes[record["lane"]]["completed"] += 1
        if b_raw.startswith("Error: internal error"):
            # A recovered Go panic is a Barn defect whatever Toast does.
            kind = "divergence"
        elif t_raw == "=> *Aborted*" and b_raw == "=> *Aborted*":
            kind = "aborted on both"
        elif (t_raw == "=> *Aborted*" and b is not None) or (b_raw == "=> *Aborted*" and t is not None):
            kind = "aborted on " + ("toast" if t is None else "barn")
        elif t is None or b is None:
            kind = "unknown"
        else:
            kind = "match" if t == b else "divergence"
        rows.append({"key": key, "kind": kind, **record, "source": source,
                     "toast": t or t_raw, "barn": b or b_raw,
                     "toast_raw": t_raw, "barn_raw": b_raw})

    counts: dict[str, int] = {}
    for row in rows:
        counts[row["kind"]] = counts.get(row["kind"], 0) + 1
    # Record-count errors prevent attribution to a source, but cannot erase
    # an explicit recovered Go panic in the preserved Barn transcript.
    recovered_panics = [record for record in records["barn"]
                        if record.startswith("Error: internal error")]
    inconclusive = bool(record_errors or not completed or counts.get("unknown")
                        or counts.get("aborted on toast") or counts.get("aborted on barn"))
    status = 1 if counts.get("divergence") or recovered_panics else 2 if inconclusive else 0
    summary = {
        "outcome": {0: "success", 1: "divergence", 2: "inconclusive"}[status],
        "loaded": len(programs) + sum(excluded.values()),
        "eligible": len(programs), "excluded": sum(excluded.values()),
        "completed_pairs": completed,
        "aborted_pairs": sum(counts.get(kind, 0) for kind in
                             ("aborted on both", "aborted on toast", "aborted on barn")),
        "unknown_pairs": counts.get("unknown", 0),
        "partial": completed != len(programs),
        "counts": counts, "record_errors": record_errors, "recovered_panics": recovered_panics,
        "lanes": corpus_lanes,
    }
    report = [
        "# Barn vs Toast fuzz differential", "",
        f"- outcome: {summary['outcome']}; partial coverage: {summary['partial']}",
        f"- programs loaded: {summary['loaded']}; eligible: {len(programs)}; completed comparisons: {completed}",
        f"- results: {json.dumps(counts, sort_keys=True)}",
        f"- record errors: {json.dumps(record_errors)}",
        f"- recovered Barn panics: {json.dumps(recovered_panics)}",
        f"- excluded: {json.dumps(excluded, sort_keys=True)}",
        f"- bounds: fg_ticks {FG_TICKS}, fg_seconds {FG_SECONDS}; db `{args.db}`",
        f"- toast binary: `{args.toast}` sha256 `{toast_sha}` version `{toast_version}`", "",
        "## lanes", "",
    ]
    for lane, lane_counts in corpus_lanes.items():
        shown = {name: value for name, value in lane_counts.items() if name != "undecodable"}
        report.append(f"- `{lane}`: {json.dumps(shown)}; undecodable: {len(lane_counts['undecodable'])}")
    report.append("")
    for kind in ("divergence", "aborted on barn", "aborted on toast", "aborted on both", "unknown"):
        listed = [r for r in rows if r["kind"] == kind]
        if listed:
            report += [f"## {kind} ({len(listed)})", ""]
            for r in listed:
                report += [f"### {r['key']} `{Path(r['origin']).name}`", "", f"- lane: `{r['lane']}`"]
                if "choices" in r:
                    report.append(f"- choices (hex): `{r['choices']}`")
                report += ["", "```moo", r["source"], "```", "",
                           f"- toast: `{r['toast']}`", f"- barn: `{r['barn']}`", ""]
    text = "\n".join(report)
    (args.out / "report.md").write_text(text, encoding="utf-8")
    (args.out / "results.json").write_text(json.dumps({"summary": summary, "excluded": excluded, "rows": rows,
                                                       "records": records}, indent=2),
                                           encoding="utf-8")
    print(text)
    return status


if __name__ == "__main__":
    sys.exit(main())
