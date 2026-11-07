#!/usr/bin/env python3
"""Barn vs Toast differential run over Go fuzz corpora of MOO source.

Reads Go fuzz corpus files (``go test fuzz v1`` + one ``string(...)`` value)
from the given directories -- the committed ``testdata/fuzz/<Target>`` inputs
and Go's fuzz cache, which the nightly campaigns of vm FuzzSourceExecution and
compiler FuzzCompileVerifiable fill -- and runs every program through both
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

Exit status: 0 no divergence, 1 divergences (listed in ``<out>/report.md``),
2 nothing was compared (no corpus, or a lane's output did not hold one result
per input).
"""

from __future__ import annotations

import argparse
import ast
import json
import re
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


def load_programs(dirs: list[Path]) -> tuple[dict[str, str], dict[str, int]]:
    """Unique compared sources -> first corpus file, and exclusion counts."""
    programs: dict[str, str] = {}
    excluded: dict[str, int] = {}
    seen = set()
    for directory in dirs:
        if not directory.is_dir():
            print(f"corpus directory missing: {directory}", file=sys.stderr)
            continue
        for path in sorted(directory.iterdir()):
            source = decode_corpus_file(path.read_bytes()) if path.is_file() else None
            if source is None or source in seen:
                continue
            seen.add(source)
            reason = exclusion(source) or (None if len(probe("F00000", source)) <= MAX_LINE
                                           else "longer than one emergency-mode line")
            if reason:
                excluded[reason] = excluded.get(reason, 0) + 1
            else:
                programs[source] = str(path)
    return programs, excluded


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

    programs, excluded = load_programs(args.corpus)
    if not programs:
        print("no comparable programs in the corpus", file=sys.stderr)
        return 2
    args.out.mkdir(parents=True, exist_ok=True)
    barn_bin = args.barn_linux or (args.out / "barn_linux")
    if not args.barn_linux:
        build_barn_linux(barn_bin)

    keyed = [("F%05d" % i, source, origin) for i, (source, origin) in enumerate(programs.items(), 1)]
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
    for lane, got in records.items():
        if len(got) != len(inputs):
            # Without one record per input the positions mean nothing.
            print(f"{lane}: {len(got)} result records for {len(inputs)} inputs; see {lane}_raw.txt",
                  file=sys.stderr)
            return 2

    rows = []
    for index, (key, source, origin) in enumerate(keyed, 1):
        t_raw, b_raw = records["toast"][index], records["barn"][index]
        t, b = outcome(key, t_raw), outcome(key, b_raw)
        if b_raw.startswith("Error: internal error"):
            # A recovered Go panic is a Barn defect whatever Toast does.
            kind = "divergence"
        elif t is None and b is None:
            kind = "aborted on both"
        elif t is None or b is None:
            kind = "aborted on " + ("toast" if t is None else "barn")
        else:
            kind = "match" if t == b else "divergence"
        rows.append({"key": key, "kind": kind, "origin": origin, "source": source,
                     "toast": t or t_raw, "barn": b or b_raw})

    counts: dict[str, int] = {}
    for row in rows:
        counts[row["kind"]] = counts.get(row["kind"], 0) + 1
    report = [
        "# Barn vs Toast fuzz differential", "",
        f"- programs compared: {len(rows)}; results: {json.dumps(counts, sort_keys=True)}",
        f"- excluded: {json.dumps(excluded, sort_keys=True)}",
        f"- bounds: fg_ticks {FG_TICKS}, fg_seconds {FG_SECONDS}; db `{args.db}`",
        f"- toast binary: `{args.toast}` sha256 `{toast_sha}` version `{toast_version}`", "",
    ]
    for kind in ("divergence", "aborted on barn", "aborted on toast"):
        listed = [r for r in rows if r["kind"] == kind]
        if listed:
            report += [f"## {kind} ({len(listed)})", ""]
            for r in listed:
                report += [f"### {r['key']} `{Path(r['origin']).name}`", "", "```moo", r["source"], "```", "",
                           f"- toast: `{r['toast']}`", f"- barn: `{r['barn']}`", ""]
    text = "\n".join(report)
    (args.out / "report.md").write_text(text, encoding="utf-8")
    (args.out / "results.json").write_text(json.dumps({"excluded": excluded, "rows": rows}, indent=2),
                                           encoding="utf-8")
    print(text)
    return 1 if counts.get("divergence") else 0


if __name__ == "__main__":
    sys.exit(main())
