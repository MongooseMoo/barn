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
one process per CHUNK programs per engine, each on a fresh copy of the
database and killed by the harness when it outruns its bound (see
run_bounded).  Each program becomes one input line::

    try r = eval("line 1", "line 2"); return {"F00001", r[1] ? "value" | "compile", r[1] ? r[2] | 0};
    except e (ANY) return {"F00001", "raised", e[1]}; endtry

so what is compared is whether the program compiles, the value it returns, or
the error code it raises.  Compile error text is not compared (only that
compilation failed).  A preamble bounds every input to FG_TICKS ticks and
FG_SECONDS seconds on both engines.  An input that one engine aborts (ticks,
seconds) has no probe result there; those are listed but not compared,
because tick accounting is not claimed to be Toast-exact.  An input the
harness killed (TIMED_OUT) counts as aborted on that engine and is also
listed under ``timeouts`` per engine.  A recovered Barn panic is always a
divergence.

Exit status: 0 positive completed coverage with no divergence or unknown outcome,
1 divergences (listed in ``<out>/report.md``), 2 inconclusive (zero completed
pairs, one-sided aborts, or malformed/incomplete output). Known both-engine
aborts can coexist with success, but are reported as partial coverage.
"""

from __future__ import annotations

import argparse
import ast
import hashlib
import json
import queue
import re
import subprocess
import sys
import threading
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from bench_differ import (KILL_GRACE, REPO, TOAST, WSL, build_barn_linux, lane_scripts, moo_str,  # noqa: E402
                          run_wsl, to_wsl, write_inputs)

FG_TICKS = 30000
FG_SECONDS = 2

# Each engine process runs the preamble and at most CHUNK probes, and is
# killed once no result record has arrived for PROBE_SECONDS (STARTUP_SECONDS
# before the first), and in any case after STARTUP_SECONDS plus PROBE_SECONDS
# per input. The engines' own limits do not bound every probe: on the pinned
# Toast an empty named while loop (`n = 1; while o (n) endwhile`) never
# reaches a tick or seconds check (issue #668). Every probe that ends ends
# within FG_SECONDS, so a killed process was stuck on the probe after its
# last record. That probe is recorded as TIMED_OUT and the chunk restarts
# without it, replaying the probes before it so the rest see the same
# database state.
CHUNK = 20
PROBE_SECONDS = 10
STARTUP_SECONDS = 30
# coreutils timeout's exit status after it killed the engine.
TIMEOUT_EXITS = (124, 137)
# Not a line either engine prints, nor one RECORD_START begins.
TIMED_OUT = "** timed out: the harness killed the engine **"
ABORTED = ("=> *Aborted*", TIMED_OUT)

DEFERRED_COMPARISONS = {
    "b594964e16a6b933676a449a39e87c920aa0d86ae1682bca0100ea546b9e0fbd",
    "0a6ccdc333f500419bbcf3364ae9183153e0365bdd3159309a68f7748df268cd",
}

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
    if hashlib.sha256(source.encode("utf-8")).hexdigest() in DEFERRED_COMPARISONS:
        return "deferred comparison"
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


def run_streaming(script: str, timeout: int, kill: str, startup: int,
                  probe_timeout: int) -> subprocess.CompletedProcess:
    """run_wsl(script, timeout), except that once no result record has
    arrived for probe_timeout seconds (startup before the first record) it
    runs the shell command kill and collects what the script printed."""
    lines: queue.Queue = queue.Queue()
    stderr: list[str] = []
    stdout: list[str] = []
    with subprocess.Popen(WSL + [script], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                          encoding="utf-8", errors="replace") as proc:
        def read_stdout():
            for line in proc.stdout:
                lines.put(line)
            lines.put(None)

        threading.Thread(target=read_stdout, daemon=True).start()
        err_reader = threading.Thread(target=lambda: stderr.append(proc.stderr.read()), daemon=True)
        err_reader.start()
        deadline = time.monotonic() + timeout
        stall_at = time.monotonic() + startup
        stalled = False
        while True:
            try:
                line = lines.get(timeout=max(0, min(deadline, stall_at) - time.monotonic()))
            except queue.Empty:
                run_wsl(kill, 60)
                if time.monotonic() >= deadline:
                    proc.kill()
                    raise subprocess.TimeoutExpired(proc.args, timeout, "".join(stdout))
                stalled, stall_at = True, deadline
                continue
            if line is None:
                break
            stdout.append(line)
            if not stalled and RECORD_START.match(line) and not NOT_A_RESULT.match(line):
                stall_at = time.monotonic() + probe_timeout
        proc.wait()
        err_reader.join()
    return subprocess.CompletedProcess(proc.args, proc.returncode, "".join(stdout), "".join(stderr))


def engine_runner(db: Path, barn_bin: Path, toast: str, out: Path, probe_timeout: int = PROBE_SECONDS,
                  startup: int = STARTUP_SECONDS):
    """run(lane, lines, bound): one engine process over lines on a private
    copy of db, killed after bound seconds, or sooner once no result record
    has arrived for probe_timeout seconds (startup before the first)."""
    def run(lane: str, lines: list[str], bound: int):
        path = out / f"{lane}_process_in.txt"
        write_inputs(path, lines, lane)
        pid = to_wsl(out / f"{lane}_process.pid")
        # The lane script records its shell's pid. Its child is coreutils
        # timeout, which puts itself and the engine in a process group of its
        # own; killing that group ends both, and the shell then exits 137
        # and removes its private directory.
        kill = f'for c in $(pgrep -P "$(cat {pid})"); do kill -KILL -- "-$c"; done'
        # The script's own timeout is the whole-process bound; the last
        # argument only guards the harness.
        script = f"echo $$ > {pid}; " + lane_scripts(db, barn_bin, toast, path, path, bound)[lane]
        return run_streaming(script, bound + KILL_GRACE + 60, kill, startup, probe_timeout)
    return run


def run_bounded(lane: str, run, lines: list[str], out: Path, probe_timeout: int = PROBE_SECONDS,
                startup: int = STARTUP_SECONDS, chunk: int = CHUNK) -> tuple[list, list[str], list[int]]:
    """Run lines (the preamble, then one probe per line) through one engine in
    bounded processes. Returns its record per line (TIMED_OUT for a probe the
    harness killed), record errors, and the indexes of the timed-out lines.
    Every process's transcript is appended to <lane>_raw.txt."""
    records: list = [None] * len(lines)
    errors: list[str] = []
    timed_out: list[int] = []
    transcript = out / f"{lane}_raw.txt"
    transcript.write_text("", encoding="utf-8")
    for start in range(1, len(lines), chunk):
        ids = range(start, min(start + chunk, len(lines)))
        stuck: set[int] = set()
        while True:
            run_ids = [i for i in ids if i not in stuck]
            if all(records[i] is not None for i in run_ids):
                break
            bound = startup + probe_timeout * (1 + len(run_ids))
            print(f"==> {lane}: inputs {run_ids[0]}..{run_ids[-1]} bound {bound}s", file=sys.stderr)
            proc = run(lane, [lines[0]] + [lines[i] for i in run_ids], bound)
            span = f"inputs {run_ids[0]}..{run_ids[-1]}"
            with transcript.open("a", encoding="utf-8") as f:
                f.write(f"--- process: {span} without {sorted(stuck)}, bound {bound}s, exit {proc.returncode} ---\n"
                        f"{proc.stdout}\n--- STDERR ---\n{proc.stderr}\n")
            killed = proc.returncode in TIMEOUT_EXITS
            if proc.returncode != 0 and not killed:
                raise RuntimeError(f"{lane} exited {proc.returncode}; see {transcript}")
            got = results(proc.stdout)
            if not got:
                raise RuntimeError(f"{lane} printed no preamble result (exit {proc.returncode}); see {transcript}")
            if records[0] is None:
                records[0] = got[0]
            for i, record in zip(run_ids, got[1:]):
                if records[i] is None:
                    records[i] = record
            done = len(got) - 1
            if killed and done >= len(run_ids):
                errors.append(f"{lane}: killed after every result of {span}; see {transcript}")
                break
            if not killed:
                if done != len(run_ids):
                    errors.append(f"{lane}: {done} result records for {len(run_ids)} {span}; see {transcript}")
                break
            # Killed: the probe after the last record never finished.
            hung = run_ids[done]
            stuck.add(hung)
            if records[hung] is None:
                records[hung] = TIMED_OUT
                timed_out.append(hung)
            else:
                errors.append(f"{lane}: input {hung} completed once but timed out on replay; see {transcript}")
    return records, errors, timed_out


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
    ap.add_argument("--probe-timeout", type=int, default=PROBE_SECONDS,
                    help="harness seconds per input; an engine process gets STARTUP_SECONDS plus this per input")
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
    lane_inputs = {"barn": [b for b, _ in inputs], "toast": [t for _, t in inputs]}
    for lane, lines in lane_inputs.items():
        write_inputs(args.out / f"{lane}_in.txt", lines, lane)
    run = engine_runner(args.db, barn_bin, args.toast, args.out, args.probe_timeout)
    records, record_errors, timeouts = {}, [], {}
    for lane in ("toast", "barn"):
        records[lane], errors, timed_out = run_bounded(lane, run, lane_inputs[lane], args.out, args.probe_timeout)
        # Without one record per input the positions mean nothing.
        for message in errors:
            print(message, file=sys.stderr)
        record_errors += errors
        timeouts[lane] = [keyed[i - 1][0] for i in timed_out]

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
        elif t_raw in ABORTED and b_raw in ABORTED:
            kind = "aborted on both"
        elif (t_raw in ABORTED and b is not None) or (b_raw in ABORTED and t is not None):
            kind = "aborted on " + ("toast" if t is None else "barn")
        elif t is None or b is None:
            kind = "unknown"
        else:
            kind = "match" if t == b else "divergence"
        rows.append({"key": key, "kind": kind, **record, "source": source,
                     "toast": t or t_raw, "barn": b or b_raw,
                     "toast_raw": t_raw, "barn_raw": b_raw,
                     "toast_timed_out": t_raw == TIMED_OUT, "barn_timed_out": b_raw == TIMED_OUT})

    counts: dict[str, int] = {}
    for row in rows:
        counts[row["kind"]] = counts.get(row["kind"], 0) + 1
    # Record-count errors prevent attribution to a source, but cannot erase
    # an explicit recovered Go panic in the preserved Barn transcript.
    recovered_panics = [record for record in records["barn"]
                        if record and record.startswith("Error: internal error")]
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
        "timeouts": timeouts, "lanes": corpus_lanes,
    }
    report = [
        "# Barn vs Toast fuzz differential", "",
        f"- outcome: {summary['outcome']}; partial coverage: {summary['partial']}",
        f"- programs loaded: {summary['loaded']}; eligible: {len(programs)}; completed comparisons: {completed}",
        f"- results: {json.dumps(counts, sort_keys=True)}",
        f"- record errors: {json.dumps(record_errors)}",
        f"- recovered Barn panics: {json.dumps(recovered_panics)}",
        f"- timed out (harness killed the engine): {json.dumps(timeouts)}",
        f"- excluded: {json.dumps(excluded, sort_keys=True)}",
        f"- bounds: fg_ticks {FG_TICKS}, fg_seconds {FG_SECONDS}; harness {args.probe_timeout}s per input, "
        f"{CHUNK} inputs per process; db `{args.db}`",
        f"- toast binary: `{args.toast}` sha256 `{toast_sha}` version `{toast_version}`", "",
        "## lanes", "",
    ]
    for lane, lane_counts in corpus_lanes.items():
        shown = {name: value for name, value in lane_counts.items() if name != "undecodable"}
        report.append(f"- `{lane}`: {json.dumps(shown)}; undecodable: {len(lane_counts['undecodable'])}")
    report.append("")
    for lane, keys in timeouts.items():
        if keys:
            report += [f"## timed out on {lane} ({len(keys)})", "",
                       f"No result within the harness bound; partial transcript in `{lane}_raw.txt`.", "",
                       *(f"- {key}" for key in keys), ""]
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
