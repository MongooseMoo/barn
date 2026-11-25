#!/usr/bin/env python3
"""Barn writes a database, Toast loads that file, and both must see the same world.

Each case is one input database that Barn's production checkpoint writer
writes (``barn offline-checkpoint``: Runtime.TaskSnapshots, then
dbformat.WriteCheckpoint, as the running server does). Barn and Toast then
each load an identical copy of the written file, with no server: Barn through
``barn eval --file``, Toast in emergency mode (scripts/bench_differ.py
lane_scripts). Both walk every object header number in the file (#0 to the last,
anonymous objects' numbers included, which read as invalid) and print, per object:

- O: name, owner, player/programmer/wizard/r/w/f flags, parents, location,
  contents;
- P: every property it has, defined or inherited: definer, name,
  property_info, is_clear_property, and the value unless clear;
- V: every verb defined on it: verb_info, verb_args, verb_code.

The printed results must be identical, except the order of a map's WAIF and
anonymous-object keys inside a property value, which Toast orders by memory
address (same_record). Barn also reads the written file back
(``barn offline-checkpoint`` on a copy), and the counts of queued and
suspended (with interrupted) tasks that Barn wrote, Barn read back, and Toast
wrote when it saved its copy at ``quit`` must agree. A case fails when Toast
refuses the file, when anything printed or counted differs, or when Barn kept
a task in its own frame form (engine/task_factory.go TaskSnapshots logs each
with its reason), because Toast cannot load that task.

Subcommands:

``worlds [CORPUS_DIR...]``: generated worlds. Each choice record (Go fuzz
corpus file, e.g. FuzzWorldCheckpointEffects inputs) and each built-in seed is
rendered by ``moo-fuzz-source --generator toast-world`` (fuzzseed.ToastWorld),
run on a copy of ``--db`` by ``offline-checkpoint --run``, and compared. The
report (``<out>/report.md``) names each failing world with its choices and
source, in the form scripts/fuzz_report.py reads. The built-in seeds run
first; the corpus worlds follow in the order sha256(--seed + origin) gives,
and with --minutes no world starts after that time, so a cycle covers part of
a large corpus and the next cycle's seed starts elsewhere
(scripts/fuzz_cross_engine.py does the same).

``database PATH``: one existing database (the Mongoose dump), checkpointed
without running anything. It holds real players' data, so everything printed,
and report.md and results.json, hold only counts and fixed text; the raw
transcripts stay under ``--out``, which the caller keeps private.

Exit status: 0 every case compared and agreed, 1 a case failed, 2 a harness
failure or nothing compared.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from bench_differ import (REPO, TOAST, build_barn_linux, lane_scripts, run_wsl, sha256,  # noqa: E402
                          to_wsl, write_inputs)
from fuzz_differential import MAX_LINE, preamble, render_choices, results  # noqa: E402

# The walk reads every property and verb of every object in one input each,
# so it gets the largest budget both engines accept. One object's property
# record can outgrow the world's own size limits (Mongoose #42804's is 82 MB),
# and both engines abort a task whose concatenation passes them (Toast
# execute.cc PUSH_ERROR_UNLESS_QUOTA), so the walk sets them to 0, which both
# read as no limit (Toast options.h; Barn builtins/limits.go canonicalizeLimit).
WALK_TICKS = 2147483647
WALK_SECONDS = 36000
WALK_OPTIONS = {"max_string_concat": 0, "max_list_value_bytes": 0, "max_map_value_bytes": 0}
TIMEOUT = 7200

BARN_FORM = "Toast could not load this checkpoint: task kept in Barn's frame form"
WROTE_RE = re.compile(r"^wrote .*: (\d+) queued, (\d+) suspended or interrupted tasks", re.M)
READ_RE = re.compile(r"^read: (\d+) queued, (\d+) suspended or interrupted tasks", re.M)
TOAST_TASKS_RE = re.compile(r"^TOAST TASKS (\d+) (queued|suspended|interrupted) tasks$", re.M)
REPAIR_RE = re.compile(r"VALIDATE: (?!Phase \d|Check for |Done through #)")
RECORD_RE = re.compile(r'=> \{"([OPV])(\d+)", (.*)\}', re.DOTALL)

# Choice records that always run, so a cycle with no retained corpus still
# compares worlds: one per action of fuzzseed.ToastWorld, the text action with
# characters of one to four UTF-8 bytes, and a mixed history.
SEEDS = REPO / "scripts" / "testdata" / "toast-world-seeds"


def walk_inputs(max_object: int) -> list[str]:
    """The preamble, then max_object(), then the O, P and V inputs of each
    object. Each fits one emergency-mode line."""
    def guarded(key: str, body: str) -> str:
        return (f'try o = #{key[1:]}; if (!valid(o)) return {{"{key}", "invalid"}}; endif {body} '
                f'except e (ANY) return {{"{key}", "raised", e[1], e[2]}}; endtry')
    lines = [preamble(WALK_TICKS, WALK_SECONDS, WALK_OPTIONS), 'return {"M0", max_object()};']
    for n in range(max_object + 1):
        lines.append(guarded(f"O{n}", f'return {{"O{n}", o.name, o.owner, {{is_player(o), o.programmer, '
                                       'o.wizard, o.r, o.w, o.f}, parents(o), o.location, o.contents};'))
        # Definers are sorted: Barn's ancestors() order is not Toast's for
        # an object with several parents (a builtin difference, not the
        # file's). Not with sort(): Toast runs it in a thread, so in
        # emergency mode the input suspends. Values go through toliteral():
        # barn eval prints a WAIF as <waif #N> where Toast prints its literal.
        lines.append(guarded(f"P{n}", 'd = {}; for a in ({o, @ancestors(o)}) i = 1; '
                                       'while (i <= length(d) && d[i] < a) i = i + 1; endwhile '
                                       'd = listinsert(d, a, i); endfor '
                                       'r = {}; for a in (d) for p in (properties(a)) '
                                       'c = is_clear_property(o, p); '
                                       'r = {@r, {a, p, property_info(o, p), c, c ? 0 | toliteral(o.(p))}}; '
                                       'endfor endfor '
                                       f'return {{"P{n}", r}};'))
        lines.append(guarded(f"V{n}", 'r = {}; for i in [1..length(verbs(o))] '
                                       'r = {@r, {verb_info(o, i), verb_args(o, i), verb_code(o, i)}}; endfor '
                                       f'return {{"V{n}", r}};'))
    assert all(len(";;" + line) <= MAX_LINE for line in lines)
    return lines


def object_count(path: Path) -> int:
    """The highest object header number in a database file: the run of object
    headers "#0", "#1", ... ("# N recycled" for a recycled slot) in order.
    Anonymous objects are written after the permanent ones as further
    batches numbered on from them (Toast db_file.cc write_db_file), so this
    can exceed max_object(); the walk then reads those numbers as invalid
    on both engines."""
    expect = 0
    with path.open("rb") as f:
        for line in f:
            line = line.rstrip(b"\r\n")
            if line in (b"#%d" % expect, b"# %d recycled" % expect):
                expect += 1
    return expect - 1


def barn_form_tasks(log_dir: Path) -> list[dict]:
    """Each task the writer kept in Barn's frame form, from its WARN records."""
    found = []
    for log in sorted(log_dir.glob("*.jsonl")):
        for line in log.read_text(encoding="utf-8", errors="replace").splitlines():
            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                continue
            if record.get("msg") == BARN_FORM:
                found.append({"task_id": record.get("task_id"), "reason": record.get("reason", "")})
    return found


def go_formats() -> list[tuple[str, re.Pattern]]:
    """Every Errorf/errors.New/mismatch literal in Barn's non-test Go source,
    longest first, with a pattern matching what it formats to."""
    literal = re.compile(r'(?:Errorf|errors\.New|mismatch)\("((?:[^"\\\n]|\\.)*)"')
    formats = set()
    for path in REPO.rglob("*.go"):
        if path.name.endswith("_test.go") or ".tmp" in path.parts or ".fuzz" in path.parts:
            continue
        formats.update(literal.findall(path.read_text(encoding="utf-8", errors="replace")))
    compiled = []
    for text in sorted(formats, key=len, reverse=True):
        text = text.replace('\\"', '"').replace("\\n", "\n").replace("\\t", "\t").replace("\\\\", "\\")
        pieces = [piece for piece in re.split(r"(%%|%[-+# 0-9.]*[a-zA-Z])", text) if piece]
        # A format that is mostly verbs ("%s: %w") would say nothing.
        if sum(c.isalpha() for piece in pieces if not piece.startswith("%") for c in piece) < 5:
            continue
        # Only a format with one wrapped error is followed into it.
        wrapped = pieces.count("%w") == 1
        verbs = {"%%": "%", "%d": r"-?\d+", "%w": "(?P<w>.*)" if wrapped else ".*?"}
        pattern = "".join(re.escape(piece) if not piece.startswith("%") else verbs.get(piece, ".*?")
                          for piece in pieces)
        compiled.append((text, re.compile(pattern, re.S)))
    return compiled


def public_reason(reason: str, formats: list[tuple[str, re.Pattern]]) -> str:
    """reason spelled only with Barn's own format strings, which are public:
    what they formatted in (object numbers, verb names) is left out."""
    for text, pattern in formats:
        m = pattern.fullmatch(reason)
        if not m:
            continue
        if "w" not in pattern.groupindex:
            return text
        inner = public_reason(m["w"], formats)
        if inner != NOT_SHOWN:
            return text.replace("%w", inner)
    return NOT_SHOWN


NOT_SHOWN = "(not a fixed string in the Go source; not shown)"


class LiteralError(ValueError):
    pass


_ATOM = re.compile(r'"(?:[^"\\]|\\.)*"|#-?\d+|\*anonymous\*|E_[A-Z_]+|true|false|'
                   r'-?(?:\d+\.?\d*(?:[eE][-+]?\d+)?|inf|nan)', re.S)


def parse_literal(text: str):
    """A MOO value as toliteral() or eval prints it, as nested tuples:
    ("list", items), ("map", ((key, value), ...)), ("waif", class, owner), or
    ("atom", spelling). Raises LiteralError on anything else."""
    pos = 0

    def skip(token: str):
        nonlocal pos
        if not text.startswith(token, pos):
            raise LiteralError(f"expected {token!r} at {pos}")
        pos += len(token)

    def value():
        nonlocal pos
        if text.startswith("[[class = ", pos):
            skip("[[class = ")
            cls = value()
            skip(", owner = ")
            owner = value()
            skip("]]")
            return ("waif", cls, owner)
        if text.startswith("{", pos):
            skip("{")
            items = []
            while not text.startswith("}", pos):
                if items:
                    skip(", ")
                items.append(value())
            skip("}")
            return ("list", tuple(items))
        if text.startswith("[", pos):
            skip("[")
            entries = []
            while not text.startswith("]", pos):
                if entries:
                    skip(", ")
                key = value()
                skip(" -> ")
                entries.append((key, value()))
            skip("]")
            return ("map", tuple(entries))
        m = _ATOM.match(text, pos)
        if not m or not m.group(0):
            raise LiteralError(f"no value at {pos}")
        pos = m.end()
        return ("atom", m.group(0))

    result = value()
    if pos != len(text):
        raise LiteralError(f"trailing text at {pos}")
    return result


def _unordered_key(key) -> bool:
    return key[0] == "waif" or key == ("atom", "*anonymous*")


def canonical(v):
    """v with each map's run of WAIF or anonymous-object keys sorted. Toast
    orders those keys by memory address (src/utils.cc compare(), TYPE_WAIF and
    TYPE_ANON), so their order is not part of the database."""
    if v[0] == "list":
        return ("list", tuple(canonical(x) for x in v[1]))
    if v[0] == "map":
        entries = [(canonical(k), canonical(x)) for k, x in v[1]]
        out, run = [], []
        for entry in entries + [None]:
            if entry is not None and _unordered_key(entry[0]):
                run.append(entry)
                continue
            out += sorted(run, key=repr)
            run = []
            if entry is not None:
                out.append(entry)
        return ("map", tuple(out))
    return v


def unescape(spelling: str) -> str:
    """The text of a MOO string literal."""
    return re.sub(r"\\(.)", r"\1", spelling[1:-1], flags=re.S)


def same_record(toast: str, barn: str) -> bool:
    """Whether two walk records agree once the order of WAIF and anonymous
    keys inside each P value's literal is set aside. Anything that does not
    parse stays a difference."""
    if toast == barn:
        return True
    try:
        records = []
        for text in (toast, barn):
            if not text.startswith("=> "):
                return False
            record = parse_literal(text[3:])
            if record[0] != "list" or len(record[1]) != 2 or record[1][0][1][:2] != '"P':
                return False
            entries = []
            for entry in record[1][1][1]:
                fields = list(entry[1])
                if len(fields) == 5 and fields[3] == ("atom", "0") and fields[4][0] == "atom" \
                        and fields[4][1].startswith('"'):
                    fields[4] = canonical(parse_literal(unescape(fields[4][1])))
                entries.append(tuple(fields))
            records.append((record[1][0], tuple(entries)))
        return records[0] == records[1]
    except (LiteralError, IndexError, TypeError):
        return False


def first_difference(toast: str, barn: str, width: int = 300) -> tuple[str, str]:
    """Both records from just before their first differing character."""
    i = next((i for i, (a, b) in enumerate(zip(toast, barn)) if a != b), min(len(toast), len(barn)))
    start = max(0, i - 40)
    cut = lambda s: ("..." if start else "") + s[start:start + width] + ("..." if len(s) > start + width else "")
    return cut(toast), cut(barn)


def run_case(name: str, source_db: Path, work: Path, barn_bin: Path, toast: str, program: str | None) -> dict:
    """Barn writes source_db (after running program, if any); both engines
    load identical copies of the written file. Returns the case's facts."""
    work.mkdir(parents=True, exist_ok=True)
    case: dict = {"name": name, "findings": []}
    written = work / "barn.db"
    shutil.copyfile(source_db, written)
    run = ""
    if program is not None:
        (work / "world.moo").write_text(program, encoding="utf-8", newline="\n")
        run = f" --run {to_wsl(work / 'world.moo')}"
    log_dir = work / "writer-log"
    writer = run_wsl(f"set -euo pipefail; {to_wsl(barn_bin)} offline-checkpoint --db {to_wsl(written)}{run} "
                     f"--log-dir={to_wsl(log_dir)} --log-level=warn", TIMEOUT)
    (work / "writer.txt").write_text(f"exit {writer.returncode}\n{writer.stdout}\n--- STDERR ---\n{writer.stderr}",
                                     encoding="utf-8")
    wrote = WROTE_RE.search(writer.stdout)
    if writer.returncode != 0 or not wrote:
        case["error"] = f"barn offline-checkpoint exited {writer.returncode}"
        return case
    checkpoint = work / "barn.db.new"
    case["file"] = {"size": checkpoint.stat().st_size, "sha256": sha256(checkpoint)}
    case["tasks"] = {"barn wrote": [int(wrote[1]), int(wrote[2])]}
    reasons = barn_form_tasks(log_dir)
    if reasons:
        case["findings"].append({"kind": "barn-form task", "tasks": reasons})

    # Every reader loads its own copy; each copy's digest is recorded.
    copies = {}
    for reader in ("barn", "toast", "readback"):
        copy = work / f"{reader}-in.db"
        shutil.copyfile(checkpoint, copy)
        copies[reader] = {"size": copy.stat().st_size, "sha256": sha256(copy)}
    case["copies"] = copies
    if len({(c["size"], c["sha256"]) for c in copies.values()} | {(case["file"]["size"], case["file"]["sha256"])}) != 1:
        case["error"] = "the copies of the written file differ"
        return case

    readback = run_wsl(f"set -euo pipefail; d=$(mktemp -d); trap 'rm -rf \"$d\"' EXIT; "
                       f"cp {to_wsl(work / 'readback-in.db')} \"$d/in.db\"; "
                       f"{to_wsl(barn_bin)} offline-checkpoint --db \"$d/in.db\" --log-dir=", TIMEOUT)
    (work / "readback.txt").write_text(f"exit {readback.returncode}\n{readback.stdout}\n--- STDERR ---\n"
                                       f"{readback.stderr}", encoding="utf-8")
    read = READ_RE.search(readback.stdout)
    if readback.returncode != 0 or not read:
        case["findings"].append({"kind": "barn cannot read its file",
                                 "detail": f"offline-checkpoint exited {readback.returncode}"})
    else:
        case["tasks"]["barn read"] = [int(read[1]), int(read[2])]

    max_object = object_count(checkpoint)
    lines = walk_inputs(max_object)
    case["object headers"] = max_object + 1
    write_inputs(work / "barn_in.txt", lines, "barn")
    write_inputs(work / "toast_in.txt", [";;" + line for line in lines], "toast")
    scripts = {lane: lane_scripts(work / f"{lane}-in.db", barn_bin, toast, work / "barn_in.txt",
                                  work / "toast_in.txt")[lane] for lane in ("barn", "toast")}
    # Toast saves its copy at quit. Its task counts are what it loaded only
    # when it ran nothing: in the Mongoose world the walk's own inputs fork
    # tasks on Toast (each add_property of a new $server_options limit forks
    # two, and an uncaught error runs #0:handle_uncaught_error), so a
    # separate load that quits at once is counted.
    write_inputs(work / "toast_count_in.txt", [], "toast")
    scripts["toast_count"] = (lane_scripts(work / "toast-in.db", barn_bin, toast, work / "toast_count_in.txt",
                                           work / "toast_count_in.txt")["toast"]
                              + " && { grep -E '^[0-9]+ (queued|suspended|interrupted) tasks$' \"$d/out.db\""
                              " || true; } | sed 's/^/TOAST TASKS /'")
    records = {}
    for lane, script in scripts.items():
        proc = run_wsl(script, TIMEOUT)
        (work / f"{lane}_raw.txt").write_text(f"exit {proc.returncode}\n{proc.stdout}\n--- STDERR ---\n{proc.stderr}",
                                              encoding="utf-8")
        if proc.returncode != 0:
            if lane != "barn":
                case["findings"].append({"kind": "toast refused the file", "exit": proc.returncode,
                                         # Toast's errlog lines begin "*** ".
                                         "log": [l for l in proc.stderr.splitlines() if "*** " in l][-5:]})
            else:
                case["error"] = f"barn eval exited {proc.returncode}"
            return case
        if lane == "toast_count":
            counts = {kind: int(n) for n, kind in reversed(TOAST_TASKS_RE.findall(proc.stdout))}
            case["tasks"]["toast read"] = [counts.get("queued"), (counts.get("suspended") or 0)
                                           + (counts.get("interrupted") or 0)]
            continue
        records[lane] = results(proc.stdout)
        if lane == "toast":
            # Toast's load prints VALIDATE phase banners and "Done through #N"
            # progress every time; any other VALIDATE line is a repair of
            # what Barn wrote (db_file.cc).
            repairs = [l for l in proc.stderr.splitlines() if REPAIR_RE.search(l)]
            case["toast_repairs"] = len(repairs)
            if repairs:
                case["findings"].append({"kind": "toast repaired the file", "log": repairs[:5]})

    if len({tuple(v) for v in case["tasks"].values()}) != 1:
        case["findings"].append({"kind": "task counts", "counts": case["tasks"]})
    if any(len(records[lane]) != len(lines) for lane in records):
        case["error"] = (f"result records: toast {len(records['toast'])}, barn {len(records['barn'])}, "
                         f"inputs {len(lines)}")
        return case
    # Toast's max_object() is compared with the other records below.
    m0 = re.fullmatch(r'=> \{"M0", #(-?\d+)\}', records["barn"][1])
    if not m0 or int(m0[1]) > max_object:
        case["error"] = f"barn's max_object() is past #{max_object}, the file's last object header"
        return case
    case["objects"] = int(m0[1]) + 1
    differing = []
    for index, (t, b) in enumerate(zip(records["toast"][1:], records["barn"][1:]), 1):
        if not same_record(t, b):
            m = RECORD_RE.fullmatch(b) or RECORD_RE.fullmatch(t)
            differing.append({"input": index, "key": m[1] + m[2] if m else "?", "kind": m[1] if m else "?",
                              "toast": t, "barn": b})
    case["records"] = len(lines) - 1
    if differing:
        case["findings"].append({"kind": "walk", "records": differing})
    return case


def finding_lines(case: dict) -> tuple[str, str, str]:
    """(summary, toast line, barn line) of a case's first finding."""
    finding = case["findings"][0]
    kind = finding["kind"]
    if kind == "walk":
        first = finding["records"][0]
        toast, barn = first_difference(first["toast"], first["barn"])
        return (f"{len(finding['records'])} walk records differ; first {first['key']}",
                json.dumps(toast, ensure_ascii=False), json.dumps(barn, ensure_ascii=False))
    if kind == "barn-form task":
        return (f"{len(finding['tasks'])} task(s) kept in Barn's frame form",
                "cannot load a task in Barn's frame form",
                json.dumps(finding["tasks"], ensure_ascii=False))
    if kind == "toast refused the file":
        return ("Toast refused the file", f"exit {finding['exit']}: {json.dumps(finding['log'])}", "wrote it")
    if kind == "toast repaired the file":
        return ("Toast repaired the file while loading it", json.dumps(finding["log"]), "wrote it")
    if kind == "task counts":
        return ("task counts differ", f"{case['tasks'].get('toast read')}", json.dumps(case["tasks"]))
    return (kind, "", json.dumps(finding))


def divergence_section(failed: list[dict]) -> list[str]:
    """Each failed world in the form scripts/fuzz_report.py reads from
    scripts/fuzz_differential.py's report: the world's origin, choices and
    source, then one toast and one barn line."""
    if not failed:
        return []
    lines = [f"## divergence ({len(failed)})", ""]
    for case in failed:
        summary, toast, barn = finding_lines(case)
        lines += [f"### {case['name']} `{case['origin']}`", "",
                  f"- choices (hex): `{case.get('choices', '')}`",
                  f"- findings: {', '.join(f['kind'] for f in case['findings'])}; {summary}",
                  # scripts/fuzz_report.py opens one issue per class.
                  f"- signature: {', '.join(sorted({f['kind'] for f in case['findings']}))}",
                  f"- written file: {case['file']['size']} bytes, sha256 `{case['file']['sha256']}`",
                  "", "```moo", case["source"].strip("\n"), "```", "",
                  f"- toast: {toast}", f"- barn: {barn}", ""]
    return lines


def load_worlds(dirs: list[Path]) -> list[dict]:
    """Each choice record of SEEDS and then of dirs, rendered as a world:
    {"origin", "choices", "source"} or {"origin", "error"}. A seed's origin
    is its file name, so its report signature is the same every cycle."""
    paths = []
    for directory in [SEEDS, *dirs]:
        if not directory.is_dir():
            print(f"corpus directory missing: {directory}", file=sys.stderr)
            continue
        paths += [str(p.resolve()) for p in sorted(directory.iterdir()) if p.is_file()]
    rendered = render_choices(paths, "toast-world")
    return [{"origin": f"seed:{Path(p).name}" if Path(p).parent == SEEDS.resolve() else p, **rendered[p]}
            for p in paths]


# database mode creates this file in --out just before any engine reads the
# database. Until it exists nothing has read players' data, so a failure's own
# text is safe to print; after it, only its type is (failure_line).
DATA_READ_MARKER = "data-read-started"


def failure_line(error: BaseException, data_read: bool) -> str:
    """The fixed-text line a harness failure prints on stdout."""
    if not data_read:
        return f"- harness failure before the database was read: {type(error).__name__}: {error}"
    return f"- harness failure after the engines began reading the database: {type(error).__name__} (message not shown)"


def preflight(args) -> list[str]:
    """database mode's inputs that are missing, as fixed text with paths
    (which the caller chose) and no contents."""
    problems = []
    if not args.path.is_file():
        problems.append(f"input database {args.path} is not a file")
    if args.barn_linux and not args.barn_linux.is_file():
        problems.append(f"--barn-linux {args.barn_linux} is not a file")
    if os.name != "nt" and not Path(args.toast).is_file():
        problems.append(f"--toast {args.toast} is not a file")
    return problems


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="mode", required=True)
    worlds_cmd = sub.add_parser("worlds", help="generated worlds from choice corpora and the built-in seeds")
    worlds_cmd.add_argument("corpus", type=Path, nargs="*", help="Go fuzz corpus directories of choice records")
    # Test_conf.db is the stock oracle manifest's fixture (fuzz_differential.py --db).
    worlds_cmd.add_argument("--db", type=Path, default=REPO / "Test_conf.db", help="database each world starts from")
    worlds_cmd.add_argument("--minutes", type=float, help="start no new world after this many minutes")
    worlds_cmd.add_argument("--seed", default="", help="order the corpus worlds by sha256(seed + origin); the nightly passes its run id")
    database_cmd = sub.add_parser("database", help="one existing database; prints counts only")
    database_cmd.add_argument("path", type=Path)
    for cmd in (worlds_cmd, database_cmd):
        cmd.add_argument("--toast", default=TOAST, help="Toast binary (path inside WSL on Windows)")
        cmd.add_argument("--barn-linux", type=Path, help="prebuilt linux barn (default: cross-compile now)")
        cmd.add_argument("--out", type=Path, default=REPO / ".tmp" / "toast_reads_barn_db")
    args = ap.parse_args()
    if args.mode != "database":
        return run(args)
    # Every failure is printed on stdout as fixed text: in full until an
    # engine reads the database, by type only after.
    problems = preflight(args)
    for problem in problems:
        print(f"- harness failure before the database was read: {problem}")
    if problems:
        return 2
    try:
        return run(args)
    except Exception as error:  # noqa: BLE001 -- every failure gets a printed reason
        print(failure_line(error, (args.out / DATA_READ_MARKER).exists()))
        return 2


def run(args) -> int:
    args.out.mkdir(parents=True, exist_ok=True)
    (args.out / DATA_READ_MARKER).unlink(missing_ok=True)
    barn_bin = args.barn_linux or (args.out / "barn_linux")
    if not args.barn_linux:
        build_barn_linux(barn_bin)
    ident = run_wsl(f"set -euo pipefail; sha256sum {args.toast} | cut -d' ' -f1; {args.toast} -v 2>&1 || true", 60)
    toast_sha, toast_version = (ident.stdout.splitlines() + ["", ""])[:2]
    if ident.returncode != 0 or not toast_version.startswith("ToastStunt version "):
        # Toast's own identification, before any database is read.
        print(f"- harness failure before the database was read: cannot identify toast binary: "
              f"{ident.stdout.strip()} {ident.stderr.strip()}")
        return 2
    header = [f"- toast binary: `{args.toast}` sha256 `{toast_sha}` version `{toast_version}`",
              f"- barn binary sha256 `{sha256(barn_bin)}`"]
    if args.mode == "database":
        return database_main(args, barn_bin, header)
    return worlds_main(args, barn_bin, header)


def order_worlds(worlds: list[dict], seed: str) -> list[dict]:
    """The built-in seeds in their own order, then the corpus worlds in the
    order sha256(seed + origin) gives."""
    builtin = [w for w in worlds if w["origin"].startswith("seed:")]
    corpus = [w for w in worlds if not w["origin"].startswith("seed:")]
    corpus.sort(key=lambda w: hashlib.sha256((seed + w["origin"]).encode()).hexdigest())
    return builtin + corpus


def worlds_main(args, barn_bin: Path, header: list[str]) -> int:
    deadline = None if args.minutes is None else time.monotonic() + args.minutes * 60
    worlds = order_worlds(load_worlds(args.corpus), args.seed)
    cases, failed, errors = [], [], []
    ran = 0
    for index, world in enumerate(worlds, 1):
        if deadline is not None and ran and time.monotonic() >= deadline:
            break
        ran += 1
        key = "W%05d" % index
        if "source" not in world:
            errors.append(f"{key} {world['origin']}: {world.get('error')}")
            continue
        print(f"==> {key} {world['origin']}", file=sys.stderr)
        case = run_case(key, args.db, args.out / key, barn_bin, args.toast, world["source"])
        case.update(world)
        cases.append(case)
        if case.get("error"):
            errors.append(f"{key} {world['origin']}: {case['error']}")
        elif case["findings"]:
            failed.append(case)
    compared = sum(1 for c in cases if not c.get("error"))
    # Outside the f-string: Python before 3.12 (the runners' python3 is 3.11)
    # rejects a backslash in an f-string expression.
    unicode_worlds = sum(1 for c in cases if re.search(r"[^\x00-\x7f]", c["source"]))
    status = 1 if failed else 2 if errors or not compared else 0
    outcome = {0: "success", 1: "divergence", 2: "inconclusive"}[status]
    report = ["# Toast reads Barn-written databases", "",
              f"- outcome: {outcome}", f"- worlds: {len(worlds)}; ran: {ran} (seed {args.seed!r}, minutes {args.minutes}); "
              f"compared: {compared}; failed: {len(failed)}",
              f"- harness errors: {json.dumps(errors)}",
              f"- unicode worlds (a string above 127): {unicode_worlds}",
              f"- base database: `{args.db}` sha256 `{sha256(args.db)}`", *header, ""]
    text = "\n".join(report + divergence_section(failed))
    (args.out / "report.md").write_text(text, encoding="utf-8")
    (args.out / "results.json").write_text(json.dumps({"summary": {"outcome": outcome, "worlds": len(worlds),
                                                                   "ran": ran, "seed": args.seed,
                                                                   "minutes": args.minutes, "compared": compared, "failed": len(failed),
                                                                   "errors": errors},
                                                       "cases": cases}, indent=2, ensure_ascii=False),
                                           encoding="utf-8")
    print(text)
    return status


def database_main(args, barn_bin: Path, header: list[str]) -> int:
    """Counts only: nothing from the database's contents is printed or kept
    outside args.out's case directory."""
    formats = go_formats()  # before the long run, so a failure here costs nothing
    (args.out / DATA_READ_MARKER).touch()
    case = run_case("database", args.path, args.out / "case", barn_bin, args.toast, None)
    lines = ["# Toast reads the Barn-written checkpoint of one database", "",
             f"- input: {args.path.stat().st_size} bytes, sha256 `{sha256(args.path)}`"]
    if "file" in case:
        lines.append(f"- written by Barn: {case['file']['size']} bytes, sha256 `{case['file']['sha256']}`")
        for reader, copy in case.get("copies", {}).items():
            lines.append(f"- {reader} loaded a copy: {copy['size']} bytes, sha256 `{copy['sha256']}`")
    counts: dict = {"object headers": case.get("object headers"), "objects": case.get("objects"), "records compared": case.get("records"),
                    "tasks": case.get("tasks"), "toast repairs": case.get("toast_repairs")}
    for finding in case["findings"]:
        if finding["kind"] == "walk":
            by_kind: dict[str, int] = {}
            for record in finding["records"]:
                by_kind[record["kind"]] = by_kind.get(record["kind"], 0) + 1
            counts["differing records"] = by_kind
        elif finding["kind"] == "barn-form task":
            reasons: dict[str, int] = {}
            for task in finding["tasks"]:
                reason = public_reason(task["reason"], formats)
                reasons[reason] = reasons.get(reason, 0) + 1
            counts["tasks in Barn's frame form"] = reasons
        elif finding["kind"] == "toast refused the file":
            counts["toast refused the file"] = f"exit {finding['exit']}"
    error = case.get("error")
    status = 2 if error else 1 if case["findings"] else 0
    outcome = {0: "success", 1: "divergence", 2: "inconclusive"}[status]
    lines += [f"- outcome: {outcome}", f"- findings: {json.dumps([f['kind'] for f in case['findings']])}",
              f"- harness error: {json.dumps(error)}", *header, "", "```json",
              json.dumps(counts, indent=2), "```"]
    text = "\n".join(lines)
    (args.out / "report.md").write_text(text, encoding="utf-8")
    (args.out / "results.json").write_text(json.dumps({"summary": {"outcome": outcome, **counts}}, indent=2),
                                           encoding="utf-8")
    print(text)
    return status


if __name__ == "__main__":
    sys.exit(main())
