#!/usr/bin/env python3
"""Jepsen checkpoint-durability campaign: Barn lifecycle owner and MOO adapter.

The Jepsen project in tests/jepsen-checkpoint schedules concurrent list-append
transactions and pause/resume/crash/checkpoint faults. Each operation is one
JSON line to this adapter, which owns every Barn process generation through
mongoose_barn_process.OwnedBarnServer (registered public Test.db profile) and
reaches MOO through moo-conformance-tests' SocketTransport.

Durability is judged only against explicit completed checkpoints: a graceful
owned SIGTERM whose database/WAIF pair the owner verified, preceded by a
frozen ledger read (the baseline). Every later generation launches from a copy
of the most recent verified pair. A command reply is never a durability
acknowledgment. Disconnects and timeouts are :info unknowns. Histories are
split per process generation and checked with the pinned Elle CLI, so Elle
never treats uncheckpointed replies as durable.

--workloads selects the row families the Jepsen clients mix (FAMILIES): the
smoke map rows, property rows spread across objects, and keyed-map rows.
Each selected family must show successful reads and writes around every
fault, and Elle checks all of them per generation.

The adapter journals every operation before it is sent and after it
completes (journal.jsonl), so an interrupted run keeps its evidence; analysis
reconciles that journal with Jepsen's history by operation id. Before each
crash and checkpoint the campaign reads every row its clients touched in the
generation, and after each resume and launch it requires a bounded
fresh-connection write and read. Analysis checks each scenario obligation and
each generation's runtime log, and reports one of four non-healthy result
classes (see EXIT_CODES) with every finding.

The live lane (plan section 2) checkpoints while clients continue: SIGALRM
requests one checkpoint at a time and its completion is acknowledged only by
Barn's `checkpoint complete` record plus the stable digest of the published
pair. A checkpoint-crash requests another checkpoint, waits a seeded random
delay, stops the owned process, classifies the frozen publication files,
kills it, copies the artifacts for forensics and restarts from the
interrupted checkpoint's actual output path, whose journal Barn's loader
recovers. Before admitting that generation, one task reads the recovered
rows; the snapshot oracle (snapshot_checks) checks them against the
generation's history and its completed (A) and pending (F) checkpoints.

The stage schedule (--stages, elle build) arms each generation with one
STAGE_SCHEDULE stop point; its stage-crash waits for the process's own
boundary record and stopped state instead of a random delay, then follows the
checkpoint-crash path. stage_coverage reports what each step reached, killed
and recovered. --control runs one negative control (CONTROLS) and passes only
on its designated witness.
"""
from __future__ import annotations

import argparse
import json
import os
import platform
import random
import shutil
import socket
import socketserver
import subprocess
import sys
import threading
import time
from pathlib import Path

import check_elle
import mongoose_barn_process as barn_process

PROFILE_ID = "barn-linux-testdb-outbound-on"
CONFIG = barn_process.ROOT / "profiles" / "barn" / "outbound-on.conf"
JVM_HEAP = "-Xmx2g"  # Same bound as tests/jepsen-checkpoint/project.clj.
# Ceiling for a post-heal/restart probe or a final observation. Not a measured
# healthy bound; each probe records its actual latency.
PROBE_DEADLINE = 30.0
# Elle process for the campaign's own final observations; Jepsen client
# processes stay far below it.
OBSERVER_PROCESS = 10 ** 9
FAULTS = ("pause", "resume", "crash", "checkpoint", "live-checkpoint", "checkpoint-crash", "stage-crash")

# Deterministic stage schedule (plan section 2 interruption table) for the
# elle build's stop points (internal/isolationprobe/checkpoint.go). Each step
# names the point and its occurrence in one process: a restart from P.new
# publishes to P.new.new, so occurrence 1 is the process's first publication
# and occurrence 2 follows one completed live checkpoint (a previous pair).
# "recovery" arms a second stop in the restarted process's startup recovery,
# between the restore renames, for the double crash. "frozen" is the stage
# mongoose_barn_process.checkpoint_artifacts must classify from the files the
# stop leaves; "cut" is the snapshot cut the plan table requires recovery to
# select (unchanged input, the completed checkpoint A, or the pending F).
STAGE_SCHEDULE = {
    "capture-1": {"point": "checkpoint/capture", "occurrence": 1, "frozen": "staging", "cut": "unchanged input"},
    "serialize-1": {"point": "checkpoint/serialize", "occurrence": 1, "frozen": "staging", "cut": "unchanged input"},
    "staged-2": {"point": "checkpoint/staged", "occurrence": 2, "frozen": "sidecar-prepared", "cut": "completed"},
    "journal-published-1": {"point": "checkpoint/journal-published", "occurrence": 1, "frozen": "journal-published",
                            "cut": "pending"},
    "journal-published-2": {"point": "checkpoint/journal-published", "occurrence": 2, "frozen": "journal-published",
                            "cut": "completed"},
    "database-renamed-1": {"point": "checkpoint/database-renamed", "occurrence": 1, "frozen": "database-renamed",
                           "cut": "pending"},
    "database-renamed-2": {"point": "checkpoint/database-renamed", "occurrence": 2, "frozen": "database-renamed",
                           "cut": "completed"},
    "pair-renamed-1": {"point": "checkpoint/pair-renamed", "occurrence": 1, "frozen": "both-renamed",
                       "cut": "pending"},
    "double-crash": {"point": "checkpoint/database-renamed", "occurrence": 2, "frozen": "database-renamed",
                     "cut": "completed", "recovery": "checkpoint/recovery-database-restored:1"},
}
STOP_ENV = "BARN_ELLE_CHECKPOINT_STOP"
FAULT_ENV = "BARN_ELLE_FAULT"
# Ceiling for a requested checkpoint to reach its stop point. Not measured.
STOP_DEADLINE = 60.0
# Negative controls: the elle-only fault, the one stage step that exposes it,
# and the witness the analysis must report (findings' "witness").
CONTROLS = {
    "checkpoint-mixed-pair": {"stage": "database-renamed-1", "witness": "mixed-pair"},
    "checkpoint-drop-append": {"stage": "pair-renamed-1", "witness": "lost-checkpointed-update", "family": "props"},
    "checkpoint-duplicate-append": {"stage": "pair-renamed-1", "witness": "duplicate-token", "family": "props"},
}
# Ceiling for one live checkpoint to log completion or failure. Not measured.
CHECKPOINT_DEADLINE = 60.0
# checkpoint-crash waits uniform(0, DELAY_SPAN x the last completed live
# checkpoint's duration) after SIGALRM before stopping the server, so the
# stop lands before, during or after publication; the frozen files say which.
DELAY_SPAN = 1.5
FIRST_DELAY_SECONDS = 0.05  # before any live checkpoint has completed
# Elle process for the recovered snapshot of a generation (plan section 2 rule 5).
RECOVERY_PROCESS = OBSERVER_PROCESS + 1

CORRECTNESS = "correctness-violation"
RECOVERY = "recovery-failure"
INCOMPLETE = "incomplete-experiment"
INVALID = "invalid-harness-evidence"
# Exit status names the most severe class found; results.json lists them all.
PRECEDENCE = (CORRECTNESS, RECOVERY, INVALID, INCOMPLETE)
EXIT_CODES = {"healthy": 0, CORRECTNESS: 1, RECOVERY: 3, INCOMPLETE: 4, INVALID: 5}
# A --control run exits 0 only with its designated witness, else this.
CONTROL_MISSED_EXIT = 6

ADMIT_CODE = (
    'if (!("jepsen_ledger" in properties(#0))) add_property(#0, "jepsen_ledger", [], {#0, "r"}); endif '
    'if (!("jepsen_rows" in properties(#0))) add_property(#0, "jepsen_rows", [], {#0, "r"}); endif '
    'if (!("jepsen_probe" in properties(#0))) add_property(#0, "jepsen_probe", 0, {#0, "r"}); endif '
    '#0.jepsen_rows = []; return #0.jepsen_ledger;')
LEDGER_CODE = "return #0.jepsen_ledger;"
PROBE_READ_CODE = "return #0.jepsen_probe;"


def epoch_code(generation):
    """Mark the database as admitted for generation, after its rows exist."""
    return ('if (!("jepsen_epoch" in properties(#0))) add_property(#0, "jepsen_epoch", -1, {#0, "r"}); endif '
            f'#0.jepsen_epoch = {generation}; return #0.jepsen_epoch;')

# Row families, selected per campaign. A key's family is key // FAMILY_SPAN;
# the Jepsen workload draws each family's transactions from its own Elle
# list-append generator over keys base + phase offset + n (core.clj txns).
#   smoke: entries of the map #0.jepsen_rows, mirrored in #0.jepsen_ledger;
#          every transaction copies and rewrites both whole properties.
#   props: key K is the list in property jpK of row object K mod 4, read and
#          appended directly.
#   keyed: key K is entry K of the map property jm of holder K mod 2 (a map
#          held directly, then a clear inherited slot), mostly through keyed
#          access; see keyed_code.
FAMILY_SPAN = 10 ** 7
FAMILIES = {"smoke": {"base": 0, "key-count": 5},
            "props": {"base": FAMILY_SPAN, "key-count": 8},
            "keyed": {"base": 2 * FAMILY_SPAN, "key-count": 4}}
# Fresh row objects for each generation, so its rows start empty: four
# property-row objects, then the keyed parent defining jm, a child holding
# its own map and a child whose slot stays clear.
ROWS_CODE = (
    'objs = {}; for i in [1..4] objs = {@objs, toint(create(#-1))}; endfor '
    'parent = create(#-1); add_property(parent, "jm", [], {#0, "r"}); '
    'own = create(parent); own.jm = []; inh = create(parent); '
    'return {@objs, toint(parent), toint(own), toint(inh)};')


def family_of(key):
    """The family owning key, or None."""
    return next((name for name, family in FAMILIES.items()
                 if family["base"] // FAMILY_SPAN == key // FAMILY_SPAN), None)


def parse_layout(value):
    """ROWS_CODE's reply: seven object numbers."""
    if not (isinstance(value, list) and len(value) == 7 and all(type(item) is int for item in value)):
        raise ValueError(f"row layout is not seven object numbers: {value!r}")
    return {"props": value[:4], "parent": value[4], "keyed": value[5:]}


# ---------------------------------------------------------------------------
# MOO transactions and native outcome categorisation

def _list_at(name, key):
    return f"(maphaskey({name}, {key}) ? {name}[{key}] | {{}})"


def props_code(f, key, value, layout):
    """A direct read or append of property row key; a missing property is {} until the first append adds it."""
    objects = layout["props"]
    obj, prop = f"#{objects[key % len(objects)]}", f"jp{key}"
    if f == "r":
        return f"out = {{@out, `{obj}.{prop} ! E_PROPNF => {{}}'}};"
    return (f"try {obj}.{prop} = {{@{obj}.{prop}, {value}}}; "
            f"except (E_PROPNF) add_property({obj}, \"{prop}\", {{{value}}}, {{#0, \"r\"}}); endtry")


def keyed_code(f, key, value, layout, mode):
    """A read or append of entry key of holder.jm.

    Mode 0 and 3: maphaskey(o.jm, K) and o.jm[K], read and assigned, which the
    compiler emits as keyed access (compiler/lower_expressions.go compileIndex
    and keyedMaphaskeyProperty, compiler/lower_assignments.go compileIndexAssign).
    Mode 1: a keyed read through a catch expression; the append's index
    expression first writes {} to the same entry, which the captured map must
    not see. Mode 2: whole-map read and replacement.
    """
    holders = layout["keyed"]
    obj = f"#{holders[key % len(holders)]}"
    if mode == 2:
        if f == "r":
            return f"m = {obj}.jm; out = {{@out, {_list_at('m', key)}}};"
        return f"m = {obj}.jm; m[{key}] = {{@{_list_at('m', key)}, {value}}}; {obj}.jm = m;"
    if mode == 1:
        if f == "r":
            return f"out = {{@out, `{obj}.jm[{key}] ! E_RANGE => {{}}'}};"
        captured = f"`{obj}.jm[({obj}.jm[{key}] = {{}}) == {{}} ? {key} | {key}] ! E_RANGE => {{}}'"
        return f"{obj}.jm[{key}] = {{@{captured}, {value}}};"
    entry = f"(maphaskey({obj}.jm, {key}) ? {obj}.jm[{key}] | {{}})"
    if f == "r":
        return f"out = {{@out, {entry}}};"
    return f"{obj}.jm[{key}] = {{@{entry}, {value}}};"


def txn_code(txn, layout=None, op_id=0):
    """One MOO task running txn's micro-operations in order and returning its reads.

    Smoke rows append to the per-boot row map and the durable ledger. Property
    and keyed rows need the generation's layout; op_id picks the keyed mode.
    """
    statements, smoke, appends = ["out = {};"], False, False
    for mop in txn:
        if not (isinstance(mop, list) and len(mop) == 3 and type(mop[1]) is int):
            raise ValueError(f"malformed micro-operation {mop!r}")
        f, key, value = mop
        if not ((f == "append" and type(value) is int) or (f == "r" and value is None)):
            raise ValueError(f"malformed micro-operation {mop!r}")
        family = family_of(key)
        if family is None:
            raise ValueError(f"micro-operation {mop!r} has a key in no row family")
        if family != "smoke" and layout is None:
            raise ValueError(f"{family} micro-operation {mop!r} needs the generation's row layout")
        if family == "props":
            statements.append(props_code(f, key, value, layout))
        elif family == "keyed":
            statements.append(keyed_code(f, key, value, layout, op_id % 4))
        elif f == "append":
            smoke = appends = True
            statements.append(f"rows[{key}] = {{@{_list_at('rows', key)}, {value}}};")
            statements.append(f"led[{key}] = {{@{_list_at('led', key)}, {value}}};")
        else:
            smoke = True
            statements.append(f"out = {{@out, {_list_at('rows', key)}}};")
    if smoke:
        statements[:0] = ["rows = #0.jepsen_rows;", "led = #0.jepsen_ledger;"]
    if appends:
        statements += ["#0.jepsen_rows = rows;", "#0.jepsen_ledger = led;"]
    statements.append("return out;")
    return " ".join(statements)


def observation_code(keys, layout):
    """Read every listed row in one task."""
    return txn_code([["r", key, None] for key in keys], layout)


def recovery_observation_code(generation, keys, layout):
    """One task, run before admission: {epoch marker, queued task count, rows}.

    The rows of generation (its keys through its layout) are read only when the
    marker says the recovered database includes that generation's admission;
    otherwise its row objects need not exist and the rows reply is {}.
    """
    reads = observation_code(keys, layout) if keys else "out = {}; return out;"
    if not reads.endswith("return out;"):
        raise ValueError("observation code does not end by returning its reads")
    return ('e = ("jepsen_epoch" in properties(#0)) ? #0.jepsen_epoch | -1; t = length(queued_tasks()); '
            f'if (e != {-2 if generation is None else generation}) return {{e, t, {{}}}}; endif '
            + reads[:-len("return out;")] + "return {e, t, out};")


def parse_recovery_observation(value, keys, generation):
    """{epoch, tasks, rows}; rows maps each key to its list when the marker names generation."""
    if not (isinstance(value, list) and len(value) == 3 and type(value[0]) is int and type(value[1]) is int
            and isinstance(value[2], list)):
        raise ValueError(f"malformed recovery observation {value!r}")
    epoch, tasks, rows = value
    if epoch == generation and keys:
        if not (len(rows) == len(keys) and all(_int_list(row) for row in rows)):
            raise ValueError(f"malformed recovered rows {rows!r}")
        return {"epoch": epoch, "tasks": tasks, "rows": [list(row) for row in rows]}
    if rows:
        raise ValueError(f"recovered rows {rows!r} returned for epoch {epoch}, generation {generation}")
    return {"epoch": epoch, "tasks": tasks, "rows": None if epoch != generation else []}


def probe_write_code(token):
    return f"#0.jepsen_probe = {token}; return #0.jepsen_probe;"


def _int_list(value):
    return isinstance(value, list) and all(type(item) is int for item in value)


def native_error(result):
    error = result.error
    return {"kind": "native-error",
            "code": None if error is None else str(getattr(error, "value", error)),
            "message": result.error_message}


def complete_txn(txn, result):
    """Map a SocketTransport ExecutionResult for txn to a Jepsen completion.

    A native MOO error does not prove its writes rolled back, so it is :info
    with its code and message; so is a reply whose shape does not match txn.
    """
    if not result.success:
        return {"type": "info", "error": native_error(result)}
    reads = [mop for mop in txn if mop[0] == "r"]
    value = result.value
    if not (isinstance(value, list) and len(value) == len(reads) and all(_int_list(item) for item in value)):
        return {"type": "info", "error": {"kind": "malformed-response", "value": repr(value)}}
    observed = iter(value)
    return {"type": "ok", "value": [[f, key, next(observed)] if f == "r" else [f, key, item]
                                    for f, key, item in txn]}


def parse_ledger(value):
    """A MOO map of integer keys to integer lists, as SocketTransport parses it."""
    if not (isinstance(value, dict) and all(type(key) is int and _int_list(items) for key, items in value.items())):
        raise ValueError(f"ledger is not a map of integer lists: {value!r}")
    return value


def ledger_json(ledger):
    return {str(key): list(items) for key, items in sorted(ledger.items())}


def ledger_from_json(data):
    return {int(key): list(items) for key, items in data.items()}


class _EofWatch:
    """Socket proxy recording whether the peer closed during a read."""

    def __init__(self, sock):
        self.sock, self.eof = sock, False

    def recv(self, size):
        data = self.sock.recv(size)
        if not data:
            self.eof = True
        return data

    def __getattr__(self, name):
        return getattr(self.sock, name)


def recording_transport_class(base):
    """SocketTransport keeping each raw framed reply; EOF before the frame ends raises."""

    class RecordingTransport(base):
        last_raw = None

        def _receive(self):
            self.last_raw = None
            real = self.sock
            watch = self.sock = _EofWatch(real)
            try:
                raw = super()._receive()
            finally:
                self.sock = real
            self.last_raw = raw
            if watch.eof:
                raise ConnectionError("server closed the connection before the framed reply completed")
            return raw

    return RecordingTransport


def socket_transport_factory():
    from moo_conformance.transport import SocketTransport
    transport = recording_transport_class(SocketTransport)
    return lambda port: transport("127.0.0.1", port, ensure_standard_properties=False)


# ---------------------------------------------------------------------------
# Provenance: every input the verdict depends on

def _file_row(path):
    path = Path(path)
    if not path.is_file():
        return {"path": str(path), "sha256": None}
    return {"path": str(path.resolve()), "size_bytes": path.stat().st_size,
            "sha256": barn_process.bench.sha256_file(path)}


def _command_output(command, cwd=None):
    try:
        result = subprocess.run(command, cwd=cwd, capture_output=True, text=True, timeout=60, check=False)
    except (OSError, subprocess.SubprocessError) as error:
        return f"unavailable: {type(error).__name__}: {error}"
    return (result.stdout + result.stderr).strip() if result.returncode == 0 else \
        f"unavailable: exit {result.returncode}: {(result.stdout + result.stderr).strip()}"


def _filesystem(path):
    """The mount holding path, from /proc/self/mountinfo (None off Linux)."""
    try:
        lines = Path("/proc/self/mountinfo").read_text().splitlines()
    except OSError:
        return None
    best = None
    target = str(Path(path).resolve())
    for line in lines:
        fields = line.split()
        if " - " not in line or len(fields) < 5:
            continue
        mount = fields[4]
        if target == mount or target.startswith(mount.rstrip("/") + "/"):
            tail = line.split(" - ", 1)[1].split()
            if best is None or len(mount) > len(best["mount_point"]):
                best = {"mount_point": mount, "fstype": tail[0] if tail else None,
                        "source": tail[1] if len(tail) > 1 else None}
    return best


def provenance(args, output):
    db = args.db.resolve()
    try:
        import moo_conformance.transport as transport_module
        transport = _file_row(transport_module.__file__)
    except ImportError as error:
        transport = {"unavailable": str(error)}
    try:
        os_release = Path("/etc/os-release").read_text()
    except OSError:
        os_release = None
    affinity = getattr(os, "sched_getaffinity", None)
    return {
        "parameters": {name: getattr(args, name) for name in
                       ("phase1", "phase3", "phase_live", "rate", "concurrency", "seed", "jepsen_timeout",
                        "workloads", "stages", "stage_seconds", "control")},
        "stage_schedule": {name: STAGE_SCHEDULE[name] for name in args.stages},
        "live_checkpoints": {"deadline_seconds": CHECKPOINT_DEADLINE, "delay_span": DELAY_SPAN,
                             "first_delay_seconds": FIRST_DELAY_SECONDS},
        "families": {name: FAMILIES[name] for name in args.workloads},
        "jvm_heap": JVM_HEAP, "probe_deadline_seconds": PROBE_DEADLINE,
        "barn": _file_row(args.barn), "config": _file_row(CONFIG), "profile_id": PROFILE_ID,
        "source_tree": {"head": _command_output(["git", "rev-parse", "HEAD"], barn_process.ROOT),
                        "tree": _command_output(["git", "rev-parse", "HEAD^{tree}"], barn_process.ROOT),
                        "status": _command_output(["git", "status", "--porcelain"], barn_process.ROOT)},
        "checker_jar": _file_row(args.elle_jar),
        "jepsen": {"jar": _file_row(args.jepsen_jar),
                   "project_clj": _file_row(barn_process.ROOT / "tests" / "jepsen-checkpoint" / "project.clj")},
        "java": _command_output([args.java, "-version"]),
        "fixture": {"database": _file_row(db), "waif_identities": _file_row(str(db) + ".waifids"),
                    "conformance_revision": _command_output(["git", "rev-parse", "HEAD"], db.parent),
                    "transport": transport},
        "runner": {"uname": platform.uname()._asdict(), "os_release": os_release,
                   "filesystem": _filesystem(output), "cpu_count": os.cpu_count(),
                   "cpu_affinity": len(affinity(0)) if affinity else None,
                   "environment": {key: os.environ.get(key) for key in
                                   ("RUNNER_NAME", "RUNNER_OS", "RUNNER_ENVIRONMENT", "GITHUB_RUN_ID",
                                    "GITHUB_RUN_ATTEMPT", "GITHUB_SHA", "GOMAXPROCS", "GOGC", "GOMEMLIMIT")}},
    }


# ---------------------------------------------------------------------------
# Lifecycle owner: one OwnedBarnServer per process generation

def free_port():
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        return probe.getsockname()[1]


class Campaign:
    def __init__(self, executable, source_db, output, transport_factory, *, server_class=None, port_source=free_port,
                 probe_deadline=PROBE_DEADLINE, seed=1, stages=(), control=None):
        self.executable, self.output = executable, output
        self.stages = list(stages)          # STAGE_SCHEDULE steps, armed one per generation in order
        self.next_stage = 0
        self.armed = {}                     # generation -> the step its stop spec selects
        self.control = control              # CONTROLS name: BARN_ELLE_FAULT for every process
        self.random = random.Random(seed)   # checkpoint-crash delays only
        self.checkpoint_seq = 0
        self.last_checkpoint_seconds = None
        self.transport_factory = transport_factory
        self.server_class = server_class or (lambda *args, **kwargs: barn_process.OwnedBarnServer(
            *args, config=CONFIG, profile_id=PROFILE_ID, **kwargs))
        self.port_source = port_source
        self.probe_deadline = probe_deadline
        self.lock = threading.Lock()        # guards live, touched, fault_seq, active_fault
        self.lifecycle = threading.Lock()   # serializes faults and launches
        self.journal_lock = threading.Lock()
        self.live = None                    # (generation, port) while clients may send
        self.touched = {}                   # generation -> every row key a client sent
        self.layouts = {}                   # generation -> its row objects (parse_layout)
        self.fault_seq, self.active_fault = 0, None
        self.probes = 0
        self.server = None
        self.generation = -1
        self.failures = []
        # The launch source: the original fixture, then each verified pair.
        self.source = {"database": str(source_db.resolve()),
                       "database_sha256": barn_process.bench.sha256_file(source_db),
                       "waif_identities": None, "waif_identities_sha256": None,
                       "baseline": None, "baseline_generation": None}
        self.log_path = output / "lifecycle.jsonl"
        self.journal_path = output / "journal.jsonl"

    def record(self, row):
        row = dict(row, monotonic_ns=time.monotonic_ns())
        with self.log_path.open("a", encoding="utf-8") as stream:
            stream.write(json.dumps(row, default=str) + "\n")

    def journal(self, row):
        """Append one adapter event durably before the caller acts on it."""
        line = json.dumps(dict(row, monotonic_ns=time.monotonic_ns()), default=str) + "\n"
        with self.journal_lock, self.journal_path.open("a", encoding="utf-8") as stream:
            stream.write(line)
            stream.flush()
            os.fsync(stream.fileno())

    def phase(self):
        """Fault phase for a journal record; the caller holds self.lock."""
        return {"fault_seq": self.fault_seq, "fault": self.active_fault,
                "paused": bool(self.server is not None and self.server.paused)}

    def touch(self, generation, keys):
        with self.lock:
            self.touched.setdefault(generation, set()).update(keys)

    def fail(self, event, error):
        message = f"{event}: {type(error).__name__}: {error}"
        self.failures.append(message)
        self.record({"event": "failure", "operation": event, "error": message})
        return message

    def evaluate(self, port, code):
        transport = self.transport_factory(port)
        try:
            transport.connect("wizard")
            result = transport.execute(code)
        finally:
            transport.disconnect()
        if not result.success:
            raise RuntimeError(f"lifecycle evaluation failed: {native_error(result)}; raw={transport.last_raw!r}")
        return result.value

    def bounded(self, attempt):
        """Retry attempt() on fresh connections until it succeeds or the deadline passes."""
        started, errors = time.monotonic(), []
        while True:
            try:
                return True, attempt(), errors, time.monotonic() - started
            except Exception as error:
                errors.append(f"{type(error).__name__}: {error}")
                if time.monotonic() - started >= self.probe_deadline:
                    return False, None, errors, time.monotonic() - started
                time.sleep(0.5)

    def probe(self, reason):
        """Require useful work after a heal or restart: a fresh write, then a fresh read."""
        generation, (_, port) = self.generation, self.live
        self.probes += 1
        token = self.probes

        def attempt():
            written = self.evaluate(port, probe_write_code(token))
            read = self.evaluate(port, PROBE_READ_CODE)
            if type(written) is not int or type(read) is not int:
                raise RuntimeError(f"probe replies are not integers: {written!r}, {read!r}")
            return {"written": written, "read": read}

        ok, value, errors, seconds = self.bounded(attempt)
        self.record({"event": "probe", "generation": generation, "reason": reason, "token": token, "ok": ok,
                     "value": value, "failed_attempts": errors, "seconds": seconds,
                     "deadline_seconds": self.probe_deadline})

    def observe(self, reason):
        """Read every row any client touched in this generation, retired keys included."""
        generation = self.generation
        with self.lock:
            keys, live = sorted(self.touched.get(generation, ())), self.live
        row = {"event": "observation", "generation": generation, "reason": reason, "keys": keys}
        if not keys:
            row.update(ok=True, value=[])
        elif live is None or self.server.paused:
            row.update(ok=False, skipped="generation not live and running")
        else:
            def attempt():
                value = self.evaluate(live[1], observation_code(keys, self.layouts.get(generation)))
                if not (isinstance(value, list) and len(value) == len(keys) and all(_int_list(v) for v in value)):
                    raise RuntimeError(f"malformed observation {value!r}")
                return value
            ok, value, errors, seconds = self.bounded(attempt)
            row.update(ok=ok, value=value, failed_attempts=errors, seconds=seconds,
                       deadline_seconds=self.probe_deadline)
        self.record(row)

    @staticmethod
    def _copy_pair(database, sidecar, sha256, sidecar_sha256, target):
        shutil.copyfile(database, target)
        if barn_process.bench.sha256_file(target) != sha256:
            raise RuntimeError("generation database copy does not match its verified source")
        if sidecar is not None:
            copied = Path(str(target) + ".waifids")
            shutil.copyfile(sidecar, copied)
            if barn_process.bench.sha256_file(copied) != sidecar_sha256:
                raise RuntimeError("generation WAIF identity copy does not match its verified source")

    def launch(self, kind, restart=None):
        """Start the next generation.

        Without restart, from an exact copy of the launch source. With a
        checkpoint-crash restart, from the interrupted checkpoint's actual
        output path in place (Barn's loader recovers its journal), or from a
        copy of the unchanged input when no checkpoint output existed.
        """
        self.generation += 1
        generation, source = self.generation, self.source
        run_dir = self.output / f"gen-{generation:03d}"
        run_dir.mkdir()
        database = run_dir / "input.db"
        extra = {}
        if restart is None:
            self._copy_pair(source["database"], source["waif_identities"], source["database_sha256"],
                            source["waif_identities_sha256"], database)
        else:
            extra["recovered_input"] = restart
            if restart["kind"] == "interrupted-checkpoint-output":
                database = Path(restart["database"])
            else:
                pair = restart["input_pair"]
                sidecar = Path(restart["database"] + ".waifids") if pair["waif_identities"] else None
                self._copy_pair(restart["database"], sidecar, pair["database"], pair["waif_identities"], database)
        if restart is not None and restart.get("recovery_stop"):
            self.recovery_stop(generation, database, restart)
        environment = self.environment()
        if self.next_stage < len(self.stages):
            step = self.stages[self.next_stage]
            self.next_stage += 1
            self.armed[generation] = step
            environment[STOP_ENV] = f"{STAGE_SCHEDULE[step]['point']}:{STAGE_SCHEDULE[step]['occurrence']}"
        if environment:
            extra["environment"] = environment
        port = self.port_source()
        server = self.server_class(Path(self.executable), database, run_dir, port, **extra)
        self.server = server
        server.start()
        self.observe_recovery(generation, port, restart)
        recovered = parse_ledger(self.evaluate(port, ADMIT_CODE))
        layout = parse_layout(self.evaluate(port, ROWS_CODE))
        if self.evaluate(port, epoch_code(generation)) != generation:
            raise RuntimeError(f"epoch marker of generation {generation} was not written")
        self.record({"event": "admit", "generation": generation, "kind": kind, "port": port,
                     "source": {key: value for key, value in source.items() if key != "baseline"},
                     "baseline_generation": source["baseline_generation"],
                     "baseline": None if source["baseline"] is None else ledger_json(source["baseline"]),
                     "recovered": ledger_json(recovered), "rows": layout, "restart": restart,
                     "stage": self.armed.get(generation), "server": server.identity})
        with self.lock:
            self.layouts[generation] = layout
            self.live = (generation, port)
        self.probe("launch")
        return generation

    def observe_recovery(self, generation, port, restart):
        """Before admission and before any client can send: the epoch marker, the
        queued task count and, after a checkpoint-crash, every row the crashed
        generation touched (plan section 2: R before admission resets rows)."""
        crashed = None if restart is None else restart["generation"]
        with self.lock:
            keys = sorted(self.touched.get(crashed, ())) if crashed is not None else []
            layout = self.layouts.get(crashed)
        row = {"event": "recovery-observation", "generation": generation, "observed_generation": crashed,
               "keys": keys}

        def attempt():
            return parse_recovery_observation(
                self.evaluate(port, recovery_observation_code(crashed, keys, layout)), keys, crashed)

        ok, value, errors, seconds = self.bounded(attempt)
        row.update(ok=ok, failed_attempts=errors, seconds=seconds, deadline_seconds=self.probe_deadline,
                   **(value or {}))
        self.record(row)
        if not ok:
            raise RuntimeError(f"recovery observation of generation {generation} failed: {errors[-1:]}")

    def _quiesce(self):
        with self.lock:
            live, self.live = self.live, None
        return live

    def pause(self):
        state = self.server.pause()
        self.record({"event": "pause", "generation": self.generation, "observed_state": state})
        return {"generation": self.generation}

    def resume(self):
        if self.server.paused:
            self.server.resume()
        self.record({"event": "resume", "generation": self.generation})
        self.probe("resume")
        return {"generation": self.generation}

    def crash(self):
        generation = self.generation
        self.observe("crash")
        self._quiesce()
        result = self.server.crash()
        self.record({"event": "crash", "generation": generation, "result": result})
        return {"crashed": generation, "generation": self.launch("crash")}

    def checkpoint(self):
        generation = self.generation
        if self.server.paused:
            self.server.resume()
        self.observe("checkpoint")
        _, port = self.live
        baseline = parse_ledger(self.evaluate(port, LEDGER_CODE))
        self._quiesce()
        result = self.server.stop()
        self.record({"event": "checkpoint", "generation": generation, "baseline": ledger_json(baseline),
                     "result": result})
        verdict = result["final_checkpoint"]
        if not verdict.get("verified"):
            raise RuntimeError(f"checkpoint of generation {generation} was not verified: {verdict.get('reason')}")
        outputs = verdict["outputs"]
        self.source = {"database": outputs["database"]["path"],
                       "database_sha256": outputs["database"]["sha256"],
                       "waif_identities": outputs["waif_identities"]["path"],
                       "waif_identities_sha256": outputs["waif_identities"]["sha256"],
                       "baseline": baseline, "baseline_generation": generation}
        return {"checkpointed": generation, "generation": self.launch("checkpoint")}

    def _request_checkpoint(self, purpose, **extra):
        """SIGALRM one checkpoint; the previous one was acknowledged or its process is gone."""
        if self.server.paused:
            self.server.resume()
        self.checkpoint_seq += 1
        boundary = self.server.request_checkpoint()
        self.record(dict({"event": "checkpoint-request", "generation": self.generation, "seq": self.checkpoint_seq,
                          "purpose": purpose, "boundary": boundary}, **extra))
        return self.checkpoint_seq, boundary

    def live_checkpoint(self):
        """Checkpoint while clients continue; acknowledged by `checkpoint complete` and the published pair."""
        generation = self.generation
        seq, boundary = self._request_checkpoint("live")
        result = self.server.wait_checkpoint(boundary, CHECKPOINT_DEADLINE)
        self.record(dict(result, event="checkpoint-" + result["status"], generation=generation, seq=seq))
        if result["status"] != "complete":
            raise RuntimeError(f"live checkpoint {seq} of generation {generation} failed: {result['failed']}")
        self.last_checkpoint_seconds = (result["duration_ms"] or 0) / 1000
        return {"checkpoint": seq, "generation": generation}

    def checkpoint_crash(self):
        """Request a checkpoint, stop and kill the owned process at a seeded random
        moment, classify the frozen files, and restart from the actual artifacts."""
        generation = self.generation
        if self.server.paused:
            self.server.resume()
        self.observe("checkpoint-crash")
        span = DELAY_SPAN * (self.last_checkpoint_seconds if self.last_checkpoint_seconds else FIRST_DELAY_SECONDS)
        delay = self.random.uniform(0, span)
        seq, boundary = self._request_checkpoint("crash", delay_seconds=delay, delay_span_seconds=span)
        time.sleep(delay)
        self._quiesce()
        state = self.server.pause()
        return self._kill_and_restart(generation, seq, boundary, {"observed_state": state})

    def stage_crash(self):
        """The armed schedule step: complete occurrence-1 live checkpoints, request
        one more, wait until the process stops itself at the step's point (its
        boundary record and /proc state T), then kill and restart as checkpoint_crash."""
        generation = self.generation
        step = self.armed.get(generation)
        if step is None:
            raise RuntimeError(f"generation {generation} has no armed stage step")
        spec = STAGE_SCHEDULE[step]
        if self.server.paused:
            self.server.resume()
        for _ in range(spec["occurrence"] - 1):
            self.live_checkpoint()
        self.observe("checkpoint-crash")
        requested = f"{spec['point']}:{spec['occurrence']}"
        seq, boundary = self._request_checkpoint("stage", stage=step, requested_stop=requested)
        try:
            stopped = self.server.wait_stop(boundary, requested, STOP_DEADLINE)
        except Exception as error:
            self.record({"event": "stage-unreached", "generation": generation, "seq": seq, "stage": step,
                         "requested_stop": requested, "error": f"{type(error).__name__}: {error}"})
            raise
        self._quiesce()
        return self._kill_and_restart(generation, seq, boundary, dict(stopped, stage=step),
                                      recovery_stop=spec.get("recovery"))

    def _kill_and_restart(self, generation, seq, boundary, stop, recovery_stop=None):
        """The owned process is stopped: classify the frozen files, SIGKILL it,
        preserve the artifacts and restart from them."""
        stopped_ns = time.monotonic_ns()
        database = self.server.db
        artifacts = barn_process.checkpoint_artifacts(database)
        try:
            log = self.server.checkpoint_log(boundary)
        except Exception as error:
            log = {"unavailable": f"{type(error).__name__}: {error}"}
        self.record(dict(stop, event="checkpoint-stop", generation=generation, seq=seq, stopped_ns=stopped_ns,
                         artifacts=artifacts, log=log))
        result = self.server.crash()
        forensic = barn_process.preserve_artifacts(database, self.output / f"gen-{generation:03d}-forensic")
        if artifacts["output"] is not None or artifacts["journal"] is not None:
            restart = {"kind": "interrupted-checkpoint-output", "database": str(Path(str(database) + ".new"))}
        else:
            restart = {"kind": "no-checkpoint-output", "database": str(database),
                       "input_pair": barn_process.pair_digest(database)}
        restart.update(generation=generation, seq=seq, stage=artifacts["stage"], crashed_database=str(database))
        if recovery_stop is not None and restart["kind"] == "interrupted-checkpoint-output":
            restart["recovery_stop"] = recovery_stop
        self.record({"event": "checkpoint-crash", "generation": generation, "seq": seq, "result": result,
                     "forensic": forensic, "restart": restart})
        return {"crashed": generation, "generation": self.launch("checkpoint-crash", restart)}

    def environment(self):
        """Variables every owned process of this campaign gets (the control's fault)."""
        return {FAULT_ENV: self.control} if self.control else {}

    def recovery_stop(self, generation, database, restart):
        """Double crash: start Barn on the interrupted output with a stop between
        the restore renames of its startup recovery, kill it there, and keep the
        second artifact set; the caller then recovers the same files again."""
        run_dir = self.output / f"gen-{generation:03d}-recovery-stop"
        run_dir.mkdir()
        environment = dict(self.environment(), **{STOP_ENV: restart["recovery_stop"]})
        server = self.server_class(Path(self.executable), database, run_dir, self.port_source(),
                                   recovered_input=restart, environment=environment)
        self.server = server
        stopped = server.start_until_stop(restart["recovery_stop"], STOP_DEADLINE)
        crashed = Path(restart["crashed_database"])
        artifacts = barn_process.checkpoint_artifacts(crashed)
        result = server.crash()
        forensic = barn_process.preserve_artifacts(crashed, self.output / f"gen-{generation:03d}-recovery-forensic")
        self.record(dict(stopped, event="recovery-stop", generation=generation,
                         crashed_generation=restart["generation"], seq=restart["seq"], artifacts=artifacts,
                         result=result, forensic=forensic, server=server.identity))

    def fault(self, name):
        with self.lifecycle:
            if self.server is None or self.failures:
                return {"type": "info", "error": {"kind": "campaign-failed", "failures": self.failures}}
            with self.lock:
                self.fault_seq += 1
                seq, self.active_fault = self.fault_seq, name
            self.journal({"event": "fault-begin", "fault_seq": seq, "f": name, "generation": self.generation})
            try:
                reply = {"type": "ok", "value": getattr(self, name.replace("-", "_"))(),
                         "generation": self.generation}
            except Exception as error:
                self._quiesce()
                reply = {"type": "info", "error": {"kind": "lifecycle", "message": self.fail(name, error)},
                         "generation": self.generation}
            finally:
                with self.lock:
                    self.active_fault = None
            self.journal({"event": "fault-end", "fault_seq": seq, "f": name, "type": reply["type"],
                          "generation": self.generation})
            return reply

    def finish(self):
        with self.lifecycle:
            self._quiesce()
            if self.server is None or self.server.proc is None or self.server.proc.poll() is not None:
                return
            try:
                if self.server.paused:
                    self.server.resume()
                self.record({"event": "final-stop", "generation": self.generation, "result": self.server.stop()})
            except Exception as error:
                self.fail("final-stop", error)


class ClientSession:
    """One Jepsen client connection: a SocketTransport bound to one generation."""

    def __init__(self, campaign):
        self.campaign, self.transport, self.generation = campaign, None, None

    def close(self):
        if self.transport is not None:
            self.transport.disconnect()
        self.transport = self.generation = None

    def txn(self, op_id, client, txn):
        """Journal the request, run it, and journal the completion before replying."""
        self.campaign.journal({"event": "request", "id": op_id, "client": client, "value": txn})
        reply = self._run(op_id, txn)
        self.campaign.journal(dict(reply, event="complete", id=op_id))
        return reply

    def _run(self, op_id, txn):
        campaign = self.campaign
        with campaign.lock:
            live, phase = campaign.live, campaign.phase()
            layout = None if live is None else campaign.layouts.get(live[0])
        if live is None:
            return {"type": "fail", "generation": None, "error": {"kind": "no-live-generation"}}
        generation, port = live
        code = txn_code(txn, layout, op_id)
        if self.transport is None or self.generation != generation:
            self.close()
            transport = campaign.transport_factory(port)
            try:
                transport.connect("wizard")
            except Exception as error:
                transport.disconnect()
                # The transaction was never sent.
                return {"type": "fail", "generation": generation,
                        "error": {"kind": "connect", "message": f"{type(error).__name__}: {error}"}}
            self.transport, self.generation = transport, generation
        transport = self.transport
        campaign.touch(generation, [key for _, key, _ in txn])
        campaign.journal({"event": "send", "id": op_id, "generation": generation, "port": port,
                          "code": code, "phase": phase})
        try:
            result = transport.execute(code)
        except Exception as error:
            raw = transport.last_raw
            self.close()
            return {"type": "info", "generation": generation, "raw": raw,
                    "error": {"kind": "transport", "message": f"{type(error).__name__}: {error}"}}
        return dict(complete_txn(txn, result), generation=generation, raw=transport.last_raw)


class AdapterHandler(socketserver.StreamRequestHandler):
    def handle(self):
        campaign = self.server.campaign
        session = ClientSession(campaign)
        try:
            for line in self.rfile:
                request = json.loads(line)
                if request.get("op") == "txn":
                    reply = session.txn(request.get("id"), request.get("process"), request["value"])
                elif request.get("op") in FAULTS:
                    reply = campaign.fault(request["op"])
                else:
                    reply = {"type": "info", "error": {"kind": "unknown-request", "request": request}}
                reply = dict(reply, id=request.get("id"))
                self.wfile.write((json.dumps(reply) + "\n").encode("utf-8"))
                self.wfile.flush()
        finally:
            session.close()


class AdapterServer(socketserver.ThreadingTCPServer):
    daemon_threads = True

    def __init__(self, campaign):
        super().__init__(("127.0.0.1", 0), AdapterHandler)
        self.campaign = campaign


# ---------------------------------------------------------------------------
# Analysis: reconciliation, generation splitting, Elle EDN, durability,
# scenario obligations and runtime logs

def load_jsonl(path):
    return [json.loads(line) for line in Path(path).read_text(encoding="utf-8").splitlines() if line.strip()]


ADAPTER_SIDE = ("adapter-timeout", "adapter-io")  # Jepsen gave up without the adapter's reply


def reconcile(history, journal):
    """Check Jepsen's client completions against the adapter journal by operation id.

    Returns (history, counts, contradictions). The returned history fills in
    the generation of an operation Jepsen stopped waiting for from the
    adapter's send record (it stays :info), and marks an operation the
    adapter never sent as unsent.
    """
    events, contradictions = {}, []
    for row in journal:
        if row.get("event") in ("request", "send", "complete"):
            slot = events.setdefault(row.get("id"), {})
            if row["event"] in slot:
                contradictions.append(f"journal records {row['event']} twice for operation {row.get('id')}")
            slot[row["event"]] = row
    counts = {"pending": 0, "late": 0, "unsent": 0, "never_reached_adapter": 0}
    invoked, seen, placed = {}, set(), []
    for position, op in enumerate(history):
        process = op.get("process")
        if type(process) is not int:
            placed.append(op)
            continue
        if op["type"] == "invoke":
            invoked[process] = op.get("value")
            placed.append(op)
            continue
        op = dict(op)
        placed.append(op)
        op_id = op.get("op-id")
        where = f"history position {position} (operation {op_id})"
        if op_id is None:
            contradictions.append(f"{where}: client completion has no operation id")
            continue
        if op_id in seen:
            contradictions.append(f"{where}: operation id completed twice")
        seen.add(op_id)
        slot = events.get(op_id, {})
        request, send, done = slot.get("request"), slot.get("send"), slot.get("complete")
        adapter_side = (op.get("error") or {}).get("kind") in ADAPTER_SIDE
        if request is None:
            if adapter_side:
                op["unsent"] = True
                counts["never_reached_adapter"] += 1
            else:
                contradictions.append(f"{where}: Jepsen recorded an adapter reply the adapter never journaled")
            continue
        if request.get("value") != invoked.get(process) or request.get("client") != process:
            contradictions.append(f"{where}: journal request (client {request.get('client')}, "
                                  f"{request.get('value')}) is not Jepsen's invocation "
                                  f"(process {process}, {invoked.get(process)})")
        if send is None:
            if adapter_side:
                op["unsent"] = True
                counts["unsent"] += 1
            elif op["type"] != "fail" or (done or {}).get("type") != "fail":
                contradictions.append(f"{where}: Jepsen recorded {op['type']} for an operation the adapter "
                                      f"never sent (journal completion {(done or {}).get('type')})")
            else:
                counts["unsent"] += 1
            continue
        generation = send.get("generation")
        if op["type"] == "fail":
            contradictions.append(f"{where}: Jepsen recorded fail for an operation sent in generation {generation}")
        elif op.get("generation") is None:
            if not adapter_side:
                contradictions.append(f"{where}: adapter reply without a generation for a sent operation")
            op["generation"] = generation
            counts["late" if done is not None else "pending"] += 1
        elif op["generation"] != generation:
            contradictions.append(f"{where}: Jepsen generation {op['generation']}, adapter sent it in "
                                  f"generation {generation}")
        elif done is None:
            contradictions.append(f"{where}: the adapter's completion is missing from the journal")
        elif (done.get("type"), done.get("generation")) != (op["type"], generation) or \
                (op["type"] == "ok" and done.get("value") != op.get("value")):
            contradictions.append(f"{where}: Jepsen recorded {op['type']} {op.get('value')}, journal recorded "
                                  f"{done.get('type')} {done.get('value')} in generation {done.get('generation')}")
    for op_id, slot in events.items():
        if op_id not in seen and "request" in slot:
            contradictions.append(f"journal operation {op_id} has no Jepsen completion")
    return placed, counts, contradictions


def split_generations(history):
    """Assign each client invocation/completion pair to its adapter generation.

    Returns ({generation: ops in history order}, counts). An operation the
    adapter never sent (a :fail with no generation, or one reconcile marked
    unsent) is excluded. Any other completion without a generation cannot be
    placed in one process lifetime, which would make per-generation checking
    unsound, so it is an error.
    """
    pending, placed = {}, {}
    counts = {"nemesis": 0, "unsent": 0}
    for position, op in enumerate(history):
        process = op["process"]
        if type(process) is not int:
            counts["nemesis"] += 1
            continue
        if op["type"] == "invoke":
            if process in pending:
                raise ValueError(f"process {process} invoked twice at history position {position}")
            pending[process] = position
            continue
        invoked = pending.pop(process, None)
        if invoked is None:
            raise ValueError(f"completion without invocation at history position {position}")
        generation = op.get("generation")
        if generation is None:
            if op["type"] != "fail" and not op.get("unsent"):
                raise ValueError(f"{op['type']} completion at history position {position} has no generation")
            counts["unsent"] += 1
            continue
        placed[invoked] = placed[position] = generation
    if pending:
        raise ValueError(f"invocations without completion at history positions {sorted(pending.values())}")
    buckets = {}
    for position, op in enumerate(history):
        if position in placed:
            buckets.setdefault(placed[position], []).append(op)
    return buckets, counts


def observation_ops(lifecycle, generation):
    """The campaign's final observations of a generation as read-only Elle transactions."""
    ops = []
    for row in lifecycle:
        if row.get("event") == "observation" and row.get("generation") == generation and row.get("ok") \
                and row.get("keys"):
            ops.append({"process": OBSERVER_PROCESS, "type": "invoke", "f": "txn",
                        "value": [["r", key, None] for key in row["keys"]]})
            ops.append({"process": OBSERVER_PROCESS, "type": "ok", "f": "txn",
                        "value": [["r", key, value] for key, value in zip(row["keys"], row["value"])]})
    # The snapshot recovered after this generation's checkpoint-crash is a read
    # of a historical cut of its rows, appended last; the serializable check
    # imposes no real-time order on it (plan section 2 rule 5). Its rows started
    # empty at admission, so no S0 prefix needs stripping.
    for row in lifecycle:
        if row.get("event") == "recovery-observation" and row.get("observed_generation") == generation \
                and row.get("ok") and row.get("epoch") == generation and row.get("keys"):
            ops.append({"process": RECOVERY_PROCESS, "type": "invoke", "f": "txn",
                        "value": [["r", key, None] for key in row["keys"]]})
            ops.append({"process": RECOVERY_PROCESS, "type": "ok", "f": "txn",
                        "value": [["r", key, value] for key, value in zip(row["keys"], row["rows"])]})
    return ops


def edn(value):
    if value is None:
        return "nil"
    if type(value) is int:
        return str(value)
    if isinstance(value, list):
        return "[" + " ".join(edn(item) for item in value) + "]"
    raise ValueError(f"unsupported EDN value {value!r}")


def elle_history(ops):
    """One generation's ops in the EDN form the Go Elle recorder writes."""
    lines = []
    for index, op in enumerate(ops):
        mops = " ".join(f"[:{f} {edn(key)} {edn(value)}]" for f, key, value in op["value"])
        lines.append(f"{{:index {index} :process {op['process']} :type :{op['type']} :f :txn :value [{mops}]}}")
    return "\n".join(lines) + "\n"


def append_tokens(ops, types):
    """Appended smoke tokens: only smoke rows are mirrored in the durable ledger."""
    return {(key, value) for op in ops if op["type"] in types
            for f, key, value in op["value"] if f == "append" and family_of(key) == "smoke"}


def durability_violations(baseline, recovered, acked, attempted):
    """Compare a recovered ledger with the explicit checkpoint baseline.

    Every baseline list must survive as a prefix, and every token acknowledged
    in the checkpointed generation must survive. Tokens after the prefix are
    allowed only if an :ok or :info append after the checkpoint attempted them.
    """
    problems = []
    for key, prefix in sorted(baseline.items()):
        got = recovered.get(key, [])
        if got[:len(prefix)] != prefix:
            problems.append(f"key {key}: checkpointed prefix {prefix} not preserved; recovered {got}")
    for key, got in sorted(recovered.items()):
        if len(set(got)) != len(got):
            problems.append(f"key {key}: duplicate tokens in recovered {got}")
        prefix = baseline.get(key, [])
        if got[:len(prefix)] == prefix:
            for value in got[len(prefix):]:
                if (key, value) not in attempted:
                    problems.append(f"key {key}: recovered token {value} was never attempted after the checkpoint")
    for key, value in sorted(acked):
        if value not in recovered.get(key, []):
            problems.append(f"key {key}: token {value} acknowledged before the checkpoint is missing")
    return problems


def checkpoint_comparisons(buckets, lifecycle):
    comparisons = []
    for row in lifecycle:
        if row.get("event") != "admit" or row.get("baseline") is None:
            continue
        start, generation = row["baseline_generation"], row["generation"]
        between = [op for g, ops in buckets.items() if start <= g < generation for op in ops]
        acked = append_tokens(buckets.get(start, []), {"ok"})
        attempted = append_tokens(between, {"ok", "info"})
        comparisons.append({"generation": generation, "kind": row["kind"], "baseline_generation": start,
                            "acknowledged_tokens": len(acked), "attempted_tokens": len(attempted),
                            "violations": durability_violations(ledger_from_json(row["baseline"]),
                                                                ledger_from_json(row["recovered"]),
                                                                acked, attempted)})
    return comparisons


# ---------------------------------------------------------------------------
# Snapshot oracle for checkpoint-crash recovery (plan section 2 rules 1-6)

def _pair(outputs):
    """{database, waif_identities} sha256 from published-pair evidence (stable_file_digest rows)."""
    if not outputs or "database" not in outputs:
        return None
    sidecar = outputs.get("waif_identities")
    return {"database": outputs["database"]["sha256"], "waif_identities": sidecar["sha256"] if sidecar else None}


def launch_pair(admit):
    """The pair a generation actually ran from: what recovery selected, else its input."""
    identity = admit.get("server") or {}
    return _pair(identity.get("recovered_pair")) or identity.get("input_pair")


def pending_pairs(artifacts, completed):
    """Every pair the frozen files identify as the pending checkpoint F: the
    journal's next pair, a staged temp pair with its sidecar, a recovery
    directory's next pair, or a published output that is not A.

    While a journal is present the output is a published pair only when it is
    the journal's next pair: between the two renames it holds the next
    database with the previous (or no) sidecar, a mixed pair that is never a
    permitted cut (plan section 2 table)."""
    pairs = []
    journal = artifacts.get("journal") or {}
    if journal.get("next"):
        pairs.append(journal["next"])
    if artifacts.get("temp") and artifacts["temp"].get("waif_identities"):
        pairs.append(artifacts["temp"])
    for members in (artifacts.get("recovery_directories") or {}).values():
        if members.get("next"):
            pairs.append(members["next"])
    output = artifacts.get("output")
    if output and output != completed and (not artifacts.get("journal") or output == journal.get("next")):
        pairs.append(output)
    unique = []
    for pair in pairs:
        if pair not in unique:
            unique.append(pair)
    return unique


def epoch_transactions(journal, generation):
    """Every operation the adapter sent in generation, with journal times.

    The send record is written before sending and the completion after the
    reply, so send_ns is no later than the real send and complete_ns no
    earlier than the real completion. An operation never sent is absent, so
    its tokens count as never attempted.
    """
    requests, sends, completes = {}, {}, {}
    for row in journal:
        table = {"request": requests, "send": sends, "complete": completes}.get(row.get("event"))
        if table is not None:
            table[row.get("id")] = row
    txns = []
    for op_id, send in sends.items():
        if send.get("generation") != generation:
            continue
        value = (requests.get(op_id) or {}).get("value") or []
        done = completes.get(op_id) or {}
        txns.append({"id": op_id, "value": value, "send_ns": send["monotonic_ns"],
                     "type": done.get("type"), "complete_ns": done.get("monotonic_ns"),
                     "error": done.get("error") or {}, "result": done.get("value") if done.get("type") == "ok" else None,
                     "writes": [(key, item) for f, key, item in value if f == "append"]})
    return txns


def _prefix_comparable(left, right):
    shorter = min(len(left), len(right))
    return left[:shorter] == right[:shorter]


def snapshot_common_violations(rows, txns, observations):
    """Checks independent of the selected cut, over the recovered rows R (D = R: rows start empty).

    Rule 4: an atomic transaction's writes are all present or all absent
    (native-error replies are excluded: an unseen exception is not a rollback).
    Rule 5: an included successful transaction's reads are included; every
    successful read and final observation is prefix-comparable with R; R holds
    no repeated or never-sent token.

    Returns [(witness, message)]; witness names the violated rule for
    negative controls (CONTROLS).
    """
    problems = []
    present = {(key, item) for key, items in rows.items() for item in items}
    attempted = {write for txn in txns for write in txn["writes"]}
    for key, items in sorted(rows.items()):
        if len(set(items)) != len(items):
            problems.append(("duplicate-token", f"key {key}: recovered row {items} repeats a token"))
        for item in items:
            if (key, item) not in attempted:
                problems.append(("never-sent-token",
                                 f"key {key}: recovered token {item} was never sent in this generation"))
    for txn in txns:
        if not txn["writes"]:
            continue
        included = [write for write in txn["writes"] if write in present]
        if included and len(included) != len(txn["writes"]) and txn["error"].get("kind") != "native-error":
            problems.append(("partial-transaction", f"operation {txn['id']} ({txn['type']}): recovered {included} "
                                                    f"of its writes {txn['writes']}, not all or none"))
        if txn["type"] == "ok" and len(included) == len(txn["writes"]):
            for f, key, items in txn["result"] or []:
                missing = [item for item in items if (key, item)
                           not in present] if f == "r" else []
                if missing:
                    problems.append(("missing-read-dependency",
                                     f"operation {txn['id']}: its writes were recovered but tokens {missing} it "
                                     f"read on key {key} were not"))
    reads = [(f"operation {txn['id']}", key, items) for txn in txns if txn["type"] == "ok"
             for f, key, items in txn["result"] or [] if f == "r"]
    reads += [(f"final observation {index}", key, items) for index, row in enumerate(observations)
              for key, items in zip(row["keys"], row["value"])]
    for who, key, items in reads:
        if key in rows and not _prefix_comparable(items, rows[key]):
            problems.append(("non-prefix-read",
                             f"{who}: read {items} on key {key} is not prefix-comparable with recovered {rows[key]}"))
    return problems


def snapshot_cut_violations(cut, generation, epoch, start_epoch, rows, txns):
    """Rule 3 for one candidate cut {name, request_ns, ack_ns}. Returns [(witness, message)]."""
    present = {(key, item) for key, items in (rows or {}).items() for item in items}
    if cut["name"] == "unchanged input":
        problems = []
        if epoch != start_epoch:
            problems.append(("epoch-mismatch", f"the input generation {generation} started from had epoch marker "
                                               f"{start_epoch}, recovered marker is {epoch}"))
        if present:
            problems.append(("rows-without-checkpoint", f"no checkpoint was recovered, yet rows hold {sorted(present)}"))
        return problems
    if epoch != generation:
        return [("epoch-mismatch", f"{cut['name']} was captured after generation {generation} was admitted, "
                                   f"but the recovered epoch marker is {epoch}")]
    problems = []
    for txn in txns:
        if txn["type"] == "ok" and txn["writes"] and txn["complete_ns"] is not None \
                and txn["complete_ns"] < cut["request_ns"]:
            missing = [write for write in txn["writes"] if write not in present]
            if missing:
                problems.append(("lost-checkpointed-update",
                                 f"operation {txn['id']} completed before the checkpoint request; "
                                 f"its writes {missing} are missing"))
        if cut.get("ack_ns") is not None and txn["send_ns"] > cut["ack_ns"]:
            extra = [write for write in txn["writes"] if write in present]
            if extra:
                problems.append(("post-acknowledgment-write",
                                 f"operation {txn['id']} was first sent after the checkpoint completed; "
                                 f"its writes {extra} are present"))
    return problems


def snapshot_checks(lifecycle, journal):
    """Check each checkpoint-crash recovery. Returns (rows, [(class, message)]).

    Each row's "witnesses" lists the rules its findings violated (see
    snapshot_common_violations and snapshot_cut_violations), which negative
    controls require."""
    findings, results = [], []

    def find(row, kind, witness, message):
        if witness not in row["witnesses"]:
            row["witnesses"].append(witness)
        if kind not in row["classes"]:
            row["classes"].append(kind)
        findings.append((kind, message))

    by_event = {}
    for row in lifecycle:
        by_event.setdefault(row.get("event"), []).append(row)
    admits = {row["generation"]: row for row in by_event.get("admit", [])}
    observed = {row["generation"]: row for row in by_event.get("recovery-observation", [])}
    requests = {row["seq"]: row for row in by_event.get("checkpoint-request", [])}
    stops = {row["seq"]: row for row in by_event.get("checkpoint-stop", [])}
    for crash in by_event.get("checkpoint-crash", []):
        generation, seq = crash["generation"], crash["seq"]
        where = f"checkpoint-crash of generation {generation}"
        admit, observation = admits.get(generation + 1), observed.get(generation + 1)
        stop, request, start = stops.get(seq), requests.get(seq), observed.get(generation)
        row = {"generation": generation, "seq": seq, "stage": (stop or {}).get("artifacts", {}).get("stage"),
               "witnesses": [], "classes": []}
        results.append(row)
        if None in (admit, observation, stop, request, start, admits.get(generation)) or not observation.get("ok"):
            row["skipped"] = "recovery evidence incomplete (see obligations)"
            continue
        recovered = launch_pair(admit) if (admit.get("server") or {}).get("recovered_pair") else None
        if recovered is None:
            find(row, INCOMPLETE, "no-recovered-pair", f"{where}: the recovered generation recorded no recovered pair")
            row["skipped"] = "no recovered pair"
            continue
        start_pair = launch_pair(admits[generation])
        completed = [item for item in by_event.get("checkpoint-complete", [])
                     if item["generation"] == generation and item["seq"] < seq]
        latest = completed[-1] if completed else None
        latest_pair = _pair(latest.get("outputs")) if latest else None
        if latest is not None and latest_pair is None:
            find(row, INCOMPLETE, "no-checkpoint-digest",
                 f"{where}: completed checkpoint {latest['seq']} has no captured pair digest")
        try:
            expected = barn_process.expected_recovery(stop["artifacts"]) or start_pair
        except ValueError as error:
            find(row, RECOVERY, "unreadable-journal", f"{where}: {error}")
            expected = None
        row.update(recovered=recovered, expected=expected)
        if expected is not None and expected != recovered:
            find(row, RECOVERY, "recovery-stage-mismatch", f"{where}: recovery selected {recovered}; frozen stage "
                                                           f"{row['stage']} requires {expected}")
        cuts = [{"name": "unchanged input", "pair": start_pair}]
        if latest_pair is not None:
            cuts.append({"name": f"completed checkpoint {latest['seq']}", "pair": latest_pair,
                         "request_ns": requests[latest["seq"]]["boundary"]["signal_ns"],
                         "ack_ns": latest["observed_ns"]})
        for pair in pending_pairs(stop["artifacts"], latest_pair):
            cuts.append({"name": f"pending checkpoint {seq}", "pair": pair,
                         "request_ns": request["boundary"]["signal_ns"], "ack_ns": None})
        matching = [cut for cut in cuts if cut["pair"] == recovered]
        row["candidate_cuts"] = [cut["name"] for cut in matching]
        if not matching:
            find(row, RECOVERY, "no-permitted-cut", f"{where}: recovered pair {recovered} matches no permitted cut "
                                                    f"({[cut['name'] for cut in cuts]})")
            # The publication contract never accepts a database with WAIF
            # identities other than its own pair's (plan section 2 table).
            mixed = next((cut for cut in cuts if cut["pair"] and cut["pair"]["database"] == recovered["database"]
                          and cut["pair"]["waif_identities"] != recovered["waif_identities"]), None)
            if mixed is not None:
                find(row, CORRECTNESS, "mixed-pair",
                     f"{where}: mixed pair accepted: the recovered database is {mixed['name']}'s, but its WAIF "
                     f"identities are {recovered['waif_identities']}, not {mixed['pair']['waif_identities']}")
            continue
        if observation["tasks"] != start["tasks"]:
            find(row, INCOMPLETE, "restored-task", f"{where}: {observation['tasks']} queued tasks recovered, "
                                                   f"{start['tasks']} at the generation's own launch; a restored "
                                                   f"task could have changed the observed rows")
        txns = epoch_transactions(journal, generation)
        rows = None if observation.get("rows") is None else dict(zip(observation["keys"], observation["rows"]))
        if rows is not None:
            finals = [item for item in by_event.get("observation", [])
                      if item.get("generation") == generation and item.get("ok") and item.get("keys")]
            for witness, problem in snapshot_common_violations(rows, txns, finals):
                find(row, CORRECTNESS, witness, f"{where}: {problem}")
        per_cut = {cut["name"]: snapshot_cut_violations(cut, generation, observation["epoch"], start["epoch"],
                                                        rows, txns) for cut in matching}
        row["cut_violations"] = {name: [message for _, message in problems] for name, problems in per_cut.items()}
        if all(per_cut.values()):
            for name, problems in per_cut.items():
                for witness, problem in problems:
                    find(row, CORRECTNESS, witness, f"{where}: no permitted cut fits; as {name}: {problem}")
        else:
            row["selected_cut"] = next(name for name, problems in per_cut.items() if not problems)
    return results, findings


def _table_cut(name, recovery):
    """Whether a selected cut name is the plan table's cut kind for a stage step."""
    return {"unchanged input": name == "unchanged input",
            "completed": name.startswith("completed checkpoint"),
            "pending": name.startswith("pending checkpoint")}[recovery]


def stage_coverage(lifecycle, selected, recoveries):
    """Per selected STAGE_SCHEDULE step: what the run actually reached, stopped,
    killed and recovered, and with what verdict. Returns (rows, [(class, message)]).

    The reached point is the process's own boundary record, never the request.
    A step without a record, a stop, a kill and a recovery is incomplete; a
    record from another process or point, or frozen files of another stage,
    is invalid evidence; recovery that is not the plan table's cut, or a double
    crash that does not converge, is a recovery failure. recoveries are
    snapshot_checks rows, whose findings the caller reports separately.
    """
    findings, rows = [], []
    by_seq = {row.get("seq"): row for row in recoveries}
    stops = {row.get("stage"): row for row in lifecycle if row.get("event") == "checkpoint-stop" and row.get("stage")}
    unreached = {row.get("stage"): row for row in lifecycle if row.get("event") == "stage-unreached"}
    crashes = {row.get("seq"): row for row in lifecycle if row.get("event") == "checkpoint-crash"}
    second = {row.get("seq"): row for row in lifecycle if row.get("event") == "recovery-stop"}
    for step in STAGE_SCHEDULE:
        spec = STAGE_SCHEDULE[step]
        requested = f"{spec['point']}:{spec['occurrence']}"
        row = {"stage": step, "requested": requested, "plan_cut": spec["cut"]}
        rows.append(row)
        if step not in selected:
            row["verdict"] = "not-selected"
            continue
        problems = []

        def problem(kind, message):
            problems.append(kind)
            findings.append((kind, f"stage {step}: {message}"))

        stop = stops.get(step)
        if stop is None:
            reason = (unreached.get(step) or {}).get("error", "no checkpoint-stop was recorded")
            problem(INCOMPLETE, f"{requested} was not reached: {reason}")
            row["verdict"] = INCOMPLETE
            continue
        record = stop.get("record") or {}
        seq = stop.get("seq")
        reached = f"{record.get('point')}:{record.get('occurrence')}"
        crash = crashes.get(seq) or {}
        recovery = by_seq.get(seq) or {}
        row.update(reached=reached, pid=record.get("pid"), owned_pid=stop.get("owned_pid"),
                   stopped_state=stop.get("observed_state"), frozen_stage=(stop.get("artifacts") or {}).get("stage"),
                   expected_frozen_stage=spec["frozen"], killed_exit=(crash.get("result") or {}).get("exit_code"),
                   restart=(crash.get("restart") or {}).get("kind"), recovered=recovery.get("recovered"),
                   candidate_cuts=recovery.get("candidate_cuts"), selected_cut=recovery.get("selected_cut"),
                   witnesses=recovery.get("witnesses", []))
        if reached != requested or record.get("pid") != stop.get("owned_pid"):
            problem(INVALID, f"boundary record {record} is not {requested} from owned PID {stop.get('owned_pid')}")
        if stop.get("observed_state") != "T":
            problem(INCOMPLETE, f"owned PID was not observed stopped at the record (state {stop.get('observed_state')!r})")
        if row["frozen_stage"] != spec["frozen"]:
            problem(INVALID, f"stopped at {reached} but the frozen files are stage {row['frozen_stage']!r}, "
                             f"not {spec['frozen']!r}")
        if row["killed_exit"] != -9:
            problem(INCOMPLETE, f"SIGKILL was not observed (exit code {row['killed_exit']})")
        if "recovery" in spec:
            again = second.get(seq)
            if again is None:
                problem(INCOMPLETE, f"no second stop during recovery ({spec['recovery']}) was recorded")
            else:
                second_record = again.get("record") or {}
                row.update(second_reached=f"{second_record.get('point')}:{second_record.get('occurrence')}",
                           second_stopped_state=again.get("observed_state"),
                           second_killed_exit=(again.get("result") or {}).get("exit_code"),
                           second_frozen_stage=(again.get("artifacts") or {}).get("stage"))
                if row["second_reached"] != spec["recovery"] or second_record.get("pid") != again.get("owned_pid"):
                    problem(INVALID, f"recovery boundary record {second_record} is not {spec['recovery']} from "
                                     f"owned PID {again.get('owned_pid')}")
                if again.get("observed_state") != "T" or row["second_killed_exit"] != -9:
                    problem(INCOMPLETE, f"second crash not observed: state {again.get('observed_state')!r}, "
                                        f"exit {row['second_killed_exit']}")
                if not again.get("forensic"):
                    problem(INCOMPLETE, "the second crash's artifact set was not preserved")
                try:
                    first = barn_process.expected_recovery(stop.get("artifacts") or {})
                    then = barn_process.expected_recovery(again.get("artifacts") or {})
                except (ValueError, KeyError, TypeError) as error:
                    problem(RECOVERY, f"double crash artifacts are unreadable: {error}")
                else:
                    row.update(first_expected=first, second_expected=then)
                    if first != then or (recovery.get("recovered") is not None and recovery["recovered"] != first):
                        problem(RECOVERY, f"repeated recovery does not converge: first crash requires {first}, "
                                          f"second crash requires {then}, recovered {recovery.get('recovered')}")
        if not recovery or recovery.get("skipped") or recovery.get("recovered") is None:
            problem(INCOMPLETE, f"no recovery was checked ({recovery.get('skipped', 'no recovery row')})")
        elif recovery.get("candidate_cuts") and not any(_table_cut(name, spec["cut"])
                                                        for name in recovery["candidate_cuts"]):
            problem(RECOVERY, f"recovered pair {recovery['recovered']} is {recovery['candidate_cuts']}, not the "
                              f"plan table's {spec['cut']} cut")
        failed = [kind for kind in PRECEDENCE if kind in problems or kind in recovery.get("classes", [])]
        row["verdict"] = failed[0] if failed else "healthy"
    return rows, findings


def outcome_counts(history):
    counts = {}
    for op in history:
        if type(op["process"]) is int and op["type"] != "invoke":
            kind = (op.get("error") or {}).get("kind", "")
            label = op["type"] + (":" + kind if kind else "")
            counts[label] = counts.get(label, 0) + 1
    return counts


def generation_stats(buckets):
    """Per generation: completions, successful transactions, and successful
    transactions with a read and with a write, overall and per row family."""
    stats = {}
    for generation, ops in buckets.items():
        row = stats.setdefault(generation, {"completions": 0, "ok": 0, "ok_reads": 0, "ok_writes": 0,
                                            "families": {}})
        for op in ops:
            if op["type"] == "invoke":
                continue
            row["completions"] += 1
            if op["type"] == "ok":
                functions = {mop[0] for mop in op["value"]}
                row["ok"] += 1
                row["ok_reads"] += "r" in functions
                row["ok_writes"] += "append" in functions
                for family in {family_of(key) for _, key, _ in op["value"]}:
                    counts = row["families"].setdefault(str(family), {"ok_reads": 0, "ok_writes": 0})
                    counts["ok_reads"] += any(f == "r" and family_of(k) == family for f, k, _ in op["value"])
                    counts["ok_writes"] += any(f == "append" and family_of(k) == family for f, k, _ in op["value"])
    return stats


def family_totals(stats):
    """Successful transactions with a read and with a write per family, over every generation."""
    totals = {}
    for row in stats.values():
        for family, counts in row["families"].items():
            total = totals.setdefault(family, {"ok_reads": 0, "ok_writes": 0})
            total["ok_reads"] += counts["ok_reads"]
            total["ok_writes"] += counts["ok_writes"]
    return dict(sorted(totals.items()))


FAULT_EVENTS = ("admit", "pause", "resume", "crash", "checkpoint")


def obligations(buckets, lifecycle, families=("smoke",), live=False, staged=False):
    """Every obligation of the fixed pause/checkpoint/crash scenario that the evidence does not meet.

    Returns (stats, [(class, message)]). A probe or observation that ran and
    failed is a recovery/progress failure; anything absent is incomplete.
    Each selected row family needs its own successful reads and writes. With
    the live lane, at least one live checkpoint must complete and at least one
    checkpoint-crash must stop, kill and recover its generation.
    """
    stats = generation_stats(buckets)
    missing = []

    def useful(generation, when):
        row = stats.get(generation, {}).get("families", {})
        for family in families:
            for key, what in (("ok_reads", "read"), ("ok_writes", "write")):
                if not row.get(family, {}).get(key):
                    missing.append((INCOMPLETE, f"{when}: generation {generation} has no successful {family} "
                                                f"transaction with a {what}"))

    def nearby(index, step, event, reason, generation):
        index += step
        while 0 <= index < len(lifecycle):
            row = lifecycle[index]
            if row.get("event") == event and row.get("reason") == reason and row.get("generation") == generation:
                return row
            if row.get("event") in FAULT_EVENTS:
                return None
            index += step
        return None

    def checked(row, kind, what):
        if row is None or row.get("skipped"):
            missing.append((INCOMPLETE, f"missing {kind} {what}"
                                        + (f" ({row['skipped']})" if row is not None else "")))
        elif not row.get("ok"):
            missing.append((RECOVERY, f"{kind} {what} did not succeed within {row.get('deadline_seconds')}s: "
                                      f"{row.get('failed_attempts')}"))

    faults = [row for row in lifecycle if row.get("event") in ("pause", "resume", "crash", "checkpoint")]
    # A stage schedule runs only stage crashes and the final checkpoint;
    # stage_coverage holds its per-step obligations.
    for name in ("checkpoint",) if staged else ("pause", "crash", "checkpoint"):
        if not any(row["event"] == name for row in faults):
            missing.append((INCOMPLETE, f"scenario: zero {name} faults were applied"))
    if faults and faults[-1]["event"] != "checkpoint":
        missing.append((INCOMPLETE, f"scenario: ended after {faults[-1]['event']} of generation "
                                    f"{faults[-1].get('generation')}, not at its final checkpoint"))
    admits = {row["generation"]: row for row in lifecycle if row.get("event") == "admit"}
    final_checkpoint = faults[-1] if faults and faults[-1]["event"] == "checkpoint" else None
    checkpointed = None
    for index, row in enumerate(lifecycle):
        event, generation = row.get("event"), row.get("generation")
        if event == "admit":
            checked(nearby(index, 1, "probe", "launch", generation), "progress probe after launch of generation",
                    str(generation))
        elif event == "pause" and row.get("observed_state") != "T":
            missing.append((INCOMPLETE, f"pause of generation {generation}: owned PID was not observed stopped "
                                        f"(state {row.get('observed_state')!r})"))
        elif event == "resume":
            checked(nearby(index, 1, "probe", "resume", generation), "progress probe after resume of generation",
                    str(generation))
        elif event in ("crash", "checkpoint"):
            what = f"{event} of generation {generation}"
            checked(nearby(index, -1, "observation", event, generation), "final observation before", what)
            useful(generation, f"before {what}")
            result = row.get("result") or {}
            if event == "crash" and result.get("exit_code") != -9:
                missing.append((INCOMPLETE, f"{what}: SIGKILL was not observed on the owned PID "
                                            f"(exit code {result.get('exit_code')})"))
            if event == "checkpoint":
                if not (result.get("final_checkpoint") or {}).get("verified"):
                    missing.append((INCOMPLETE, f"{what}: checkpoint was not completed and verified"))
                else:
                    checkpointed = generation
            admitted = admits.get(generation + 1) if type(generation) is int else None
            if admitted is None or admitted.get("kind") != event or \
                    admitted.get("baseline_generation") != checkpointed:
                missing.append((INCOMPLETE, f"{what}: generation {generation + 1 if type(generation) is int else '?'}"
                                            f" was not admitted from checkpointed generation {checkpointed}"))
            if row is not final_checkpoint:
                useful(generation + 1 if type(generation) is int else None, f"after {what}")
        elif event == "checkpoint-crash":
            what = f"checkpoint-crash of generation {generation}"
            checked(nearby(index, -1, "observation", event, generation), "final observation before", what)
            useful(generation, f"before {what}")
            useful(generation + 1, f"after {what}")
            seq = row.get("seq")
            stop = next((item for item in lifecycle if item.get("event") == "checkpoint-stop"
                         and item.get("seq") == seq), None)
            if not any(item.get("event") == "checkpoint-request" and item.get("seq") == seq for item in lifecycle):
                missing.append((INCOMPLETE, f"{what}: no checkpoint request was recorded"))
            if stop is None or stop.get("observed_state") != "T":
                missing.append((INCOMPLETE, f"{what}: owned PID was not observed stopped before the kill"))
            elif (stop.get("artifacts") or {}).get("stage") not in barn_process.CHECKPOINT_STAGES:
                missing.append((INCOMPLETE, f"{what}: frozen checkpoint stage was not classified "
                                            f"({(stop.get('artifacts') or {}).get('stage')!r})"))
            if (row.get("result") or {}).get("exit_code") != -9:
                missing.append((INCOMPLETE, f"{what}: SIGKILL was not observed on the owned PID "
                                            f"(exit code {(row.get('result') or {}).get('exit_code')})"))
            if not row.get("forensic"):
                missing.append((INCOMPLETE, f"{what}: no forensic copy of the artifacts was preserved"))
            admitted = admits.get(generation + 1)
            restart = (admitted or {}).get("restart") or {}
            if admitted is None or admitted.get("kind") != event or restart.get("seq") != seq:
                missing.append((INCOMPLETE, f"{what}: generation {generation + 1} was not admitted from its "
                                            f"interrupted checkpoint"))
            else:
                if not (admitted.get("server") or {}).get("recovered_pair"):
                    missing.append((INCOMPLETE, f"{what}: generation {generation + 1} recorded no recovered pair"))
                observation = next((item for item in lifecycle if item.get("event") == "recovery-observation"
                                    and item.get("generation") == generation + 1), None)
                checked(observation, "recovery observation of", what)
        elif event == "checkpoint-request" and row.get("purpose") == "live":
            if not any(item.get("event") == "checkpoint-complete" and item.get("seq") == row.get("seq")
                       and item.get("outputs") for item in lifecycle):
                missing.append((INCOMPLETE, f"live checkpoint {row.get('seq')} of generation {generation}: no "
                                            f"completion with a published pair digest"))
    if live:
        for event, what in (("checkpoint-complete", "completed live checkpoints"),
                            ("checkpoint-crash", "checkpoint-crash faults")):
            if not any(row.get("event") == event for row in lifecycle):
                missing.append((INCOMPLETE, f"scenario: zero {what}"))
    for generation, row in sorted(stats.items()):
        if row["completions"] and not row["ok"]:
            missing.append((INCOMPLETE, f"generation {generation}: {row['completions']} client completions and no "
                                        f"successful transaction (unexamined live interval)"))
    return stats, missing


PANIC_MESSAGES = ("panic in verb call", "panic in eval", "panic in task")
CHECKPOINT_FAILURES = ("checkpoint failed", "final checkpoint failed", "emergency dump failed")
RESTORE_FAILURES = ("failed to restore queued task", "failed to restore suspended task",
                    "dropped suspended task: saved program does not match its source")


def log_findings(inputs, lifecycle):
    """Scan each launched generation's runtime logs. Returns ({generation: records}, [(class, message)]).

    This scenario declares no expected diagnostic: a panic is a correctness
    violation, a checkpoint or task-restore failure a recovery failure, and
    any other ERROR record is unexamined evidence.
    """
    counts, findings = {}, []
    for generation in sorted(row["generation"] for row in lifecycle if row.get("event") == "admit"):
        directory = inputs / f"gen-{generation:03d}"
        try:
            lines = (directory / "logs" / "latest.jsonl").read_text(encoding="utf-8", errors="replace").splitlines()
        except FileNotFoundError:
            findings.append((INCOMPLETE, f"generation {generation}: runtime log logs/latest.jsonl is missing"))
            lines = []
        counts[generation] = 0
        for number, line in enumerate(lines, 1):
            if not line.strip():
                continue
            counts[generation] += 1
            where = f"generation {generation} logs/latest.jsonl:{number}"
            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                findings.append((INCOMPLETE, f"{where}: unparseable runtime log record: {line}"))
                continue
            message = record.get("msg") if isinstance(record, dict) else None
            message = message if isinstance(message, str) else ""
            if message.startswith("PANIC:") or message in PANIC_MESSAGES:
                findings.append((CORRECTNESS, f"{where}: panic: {line}"))
            elif message in CHECKPOINT_FAILURES:
                findings.append((RECOVERY, f"{where}: checkpoint failure: {line}"))
            elif message in RESTORE_FAILURES:
                findings.append((RECOVERY, f"{where}: task restore failure: {line}"))
            elif isinstance(record, dict) and record.get("level") == "ERROR":
                findings.append((INCOMPLETE, f"{where}: unexamined ERROR record: {line}"))
        try:
            text = (directory / "server.log").read_text(encoding="utf-8", errors="replace")
        except FileNotFoundError:
            findings.append((INCOMPLETE, f"generation {generation}: server.log is missing"))
            continue
        for number, line in enumerate(text.splitlines(), 1):
            if line.startswith(("panic: ", "fatal error: ")):
                findings.append((CORRECTNESS, f"generation {generation} server.log:{number}: Go runtime crash: {line}"))
    return counts, findings


def analyze(inputs, output, jar, java):
    """Check one finished campaign directory; returns (results, findings)."""
    findings = []

    def find(kind, message):
        findings.append({"class": kind, "message": message})

    lifecycle = load_jsonl(inputs / "lifecycle.jsonl")
    for row in lifecycle:
        if row.get("event") == "failure":
            find(INVALID if row.get("operation") in ("jepsen", "campaign") else RECOVERY, row["error"])
    results = {}
    try:
        results["provenance"] = json.loads((inputs / "provenance.json").read_text(encoding="utf-8"))
    except FileNotFoundError:
        find(INVALID, "no provenance.json: the campaign's inputs are not recorded")
    results["logs"], log_problems = log_findings(inputs, lifecycle)
    for kind, message in log_problems:
        find(kind, message)
    try:
        history = load_jsonl(inputs / "history.jsonl")
    except FileNotFoundError:
        find(INCOMPLETE, "Jepsen wrote no history.jsonl")
        return results, findings
    results["outcomes"] = outcome_counts(history)
    for label, count in results["outcomes"].items():
        if label.endswith((":native-error", ":malformed-response")):
            find(INCOMPLETE, f"{count} client completions were {label}")
        elif label.endswith(tuple(":" + kind for kind in ADAPTER_SIDE)):
            find(INVALID, f"{count} client completions were {label}")
    try:
        journal = load_jsonl(inputs / "journal.jsonl")
    except FileNotFoundError:
        find(INVALID, "no adapter journal.jsonl to reconcile with the Jepsen history")
        return results, findings
    history, results["reconciliation"], contradictions = reconcile(history, journal)
    for message in contradictions:
        find(INVALID, f"journal contradicts history: {message}")
    try:
        buckets, results["split"] = split_generations(history)
    except ValueError as error:
        find(INVALID, f"history cannot be split per generation: {error}")
        return results, findings
    results["checkpoints"] = checkpoint_comparisons(buckets, lifecycle)
    for row in results["checkpoints"]:
        for problem in row["violations"]:
            find(CORRECTNESS, f"generation {row['generation']} ({row['kind']}): {problem}")
    # Campaigns recorded before row families existed ran the smoke rows only.
    families = (results.get("provenance") or {}).get("parameters", {}).get("workloads") or ["smoke"]
    unknown = [name for name in families if name not in FAMILIES]
    if unknown:
        find(INVALID, f"provenance names unknown row families {unknown}")
    families = [name for name in families if name in FAMILIES]
    # Campaigns recorded before the live lane existed have no phase_live.
    parameters = (results.get("provenance") or {}).get("parameters", {})
    # Campaigns recorded before stage schedules existed have no stages.
    stages = parameters.get("stages") or []
    unknown = [name for name in stages if name not in STAGE_SCHEDULE]
    if unknown:
        find(INVALID, f"provenance names unknown stage steps {unknown}")
    live = bool(parameters.get("phase_live")) and not stages
    results["generations"], missing = obligations(buckets, lifecycle, families, live, staged=bool(stages))
    results["families"] = {"selected": families, "totals": family_totals(results["generations"])}
    results["missing_obligations"] = [message for _, message in missing]
    for kind, message in missing:
        find(kind, message)
    results["recoveries"], snapshot_problems = snapshot_checks(lifecycle, journal)
    for kind, message in snapshot_problems:
        find(kind, message)
    if stages:
        results["coverage"], coverage_problems = stage_coverage(lifecycle, stages, results["recoveries"])
        for kind, message in coverage_problems:
            find(kind, message)
    stages = {}
    for row in lifecycle:
        if row.get("event") == "checkpoint-stop":
            stage = (row.get("artifacts") or {}).get("stage")
            stages[stage] = stages.get(stage, 0) + 1
    results["checkpoint_stages"] = stages
    results["elle"] = []
    observed = {row["generation"] for row in lifecycle if row.get("event") == "observation"}
    observed |= {row["observed_generation"] for row in lifecycle
                 if row.get("event") == "recovery-observation" and row.get("observed_generation") is not None}
    for generation in sorted(set(buckets) | observed):
        ops = buckets.get(generation, []) + observation_ops(lifecycle, generation)
        if not any(op["type"] == "ok" for op in ops):
            results["elle"].append({"generation": generation, "skipped": "no :ok transactions"})
            continue
        history_path = output / f"gen-{generation:03d}.edn"
        history_path.write_text(elle_history(ops), encoding="utf-8")
        command = [java, "-jar", str(jar), "-m", "list-append", "-c", "serializable",
                   "--directory", str(output / f"graphs-gen-{generation:03d}"),
                   "--max-plot-bytes", "2000000", str(history_path)]
        exit_code, text = check_elle.run(command, output / f"gen-{generation:03d}-elle.log")
        row = {"generation": generation, "ops": len(ops), "command": command, "exit_code": exit_code}
        try:
            row["verdict"] = check_elle.verdict(text, history_path)
        except ValueError as error:
            find(INVALID, f"generation {generation}: {error}")
        verdict = row.get("verdict")
        if verdict == "false":
            find(CORRECTNESS, f"generation {generation}: Elle rejected the history (exit {exit_code})")
        elif verdict == "unknown":
            find(INCOMPLETE, f"generation {generation}: Elle could not decide (exit {exit_code})")
        elif verdict == "true" and exit_code != 0:
            find(INVALID, f"generation {generation}: Elle verdict true with exit {exit_code}")
        results["elle"].append(row)
    return results, findings


def verdict_of(findings):
    present = {finding["class"] for finding in findings}
    return next((kind for kind in PRECEDENCE if kind in present), "healthy")


def control_result(control, results):
    """A negative control passes only when its stage step was reached and its
    designated witness is among the recovery findings; any other outcome,
    healthy included, is a missed control."""
    spec = CONTROLS[control]
    observed = sorted({witness for row in results.get("recoveries") or [] for witness in row.get("witnesses", [])})
    stage = next((row for row in results.get("coverage") or [] if row["stage"] == spec["stage"]), {})
    reached = stage.get("reached") == stage.get("requested") and stage.get("killed_exit") == -9
    return {"name": control, "stage": spec["stage"], "witness": spec["witness"], "observed_witnesses": observed,
            "stage_reached": reached, "passed": reached and spec["witness"] in observed}


# ---------------------------------------------------------------------------

def run_campaign(args, output):
    campaign = Campaign(args.barn.resolve(strict=True), args.db, output, socket_transport_factory(), seed=args.seed,
                        stages=args.stages, control=args.control)
    adapter = None
    try:
        (output / "provenance.json").write_text(json.dumps(provenance(args, output), indent=2, default=str) + "\n",
                                                encoding="utf-8")
        campaign.launch("initial")
        adapter = AdapterServer(campaign)
        threading.Thread(target=adapter.serve_forever, daemon=True).start()
        command = [args.java, JVM_HEAP, "-Djava.awt.headless=true", "-jar", str(args.jepsen_jar.resolve(strict=True)),
                   "--adapter-port", str(adapter.server_address[1]), "--history", str(output / "history.jsonl"),
                   "--phase1", str(args.phase1), "--phase3", str(args.phase3),
                   "--phase-live", str(args.phase_live), "--rate", str(args.rate),
                   "--stages", str(len(args.stages)), "--stage-seconds", str(args.stage_seconds),
                   "--concurrency", str(args.concurrency), "--seed", str(args.seed),
                   "--families", json.dumps([dict(FAMILIES[name], name=name) for name in args.workloads])]
        campaign.record({"event": "jepsen", "command": command})
        with (output / "jepsen.log").open("wb") as log:
            # Jepsen writes its store/ (history.edn, jepsen.log) under its cwd.
            code = subprocess.run(command, cwd=output, stdout=log, stderr=subprocess.STDOUT,
                                  timeout=args.jepsen_timeout, check=False).returncode
        if code:
            campaign.fail("jepsen", RuntimeError(f"exited {code}; see jepsen.log"))
    except Exception as error:
        campaign.fail("campaign", error)
    finally:
        if adapter is not None:
            adapter.shutdown()
            adapter.server_close()
        campaign.finish()


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--output", type=Path, required=True, help="new evidence directory; existing ones are refused")
    parser.add_argument("--elle-jar", type=Path, required=True, help="built pinned elle-cli jar (see .github/workflows/elle.yml)")
    parser.add_argument("--java", default="java")
    parser.add_argument("--analyze", type=Path, help="re-check an existing campaign directory without launching anything")
    parser.add_argument("--barn", type=Path, help="Linux Barn executable")
    parser.add_argument("--db", type=Path, help="public Test.db fixture")
    parser.add_argument("--jepsen-jar", type=Path, help="tests/jepsen-checkpoint uberjar")
    parser.add_argument("--phase1", type=int, default=60)
    parser.add_argument("--phase3", type=int, default=90)
    parser.add_argument("--phase-live", type=int, default=60,
                        help="seconds of live checkpoints and checkpoint-crash recoveries; 0 disables the lane")
    parser.add_argument("--rate", type=int, default=20, help="total transactions per second")
    parser.add_argument("--concurrency", type=int, default=8)
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument("--jepsen-timeout", type=int, default=900)
    parser.add_argument("--workloads", default="smoke",
                        help=f"comma-separated row families to mix: {', '.join(FAMILIES)}")
    parser.add_argument("--stages", default="",
                        help="elle build only: 'all' or comma-separated STAGE_SCHEDULE steps "
                             f"({', '.join(STAGE_SCHEDULE)}); replaces the fixed and live phases")
    parser.add_argument("--stage-seconds", type=int, default=10, help="client seconds around each stage step")
    parser.add_argument("--control", choices=sorted(CONTROLS),
                        help="elle build only: run the negative control's fault and stage step; exit 0 only if "
                             "its designated witness is reported")
    args = parser.parse_args(argv)
    args.workloads = args.workloads.split(",")
    unknown = [name for name in args.workloads if name not in FAMILIES]
    if unknown or len(set(args.workloads)) != len(args.workloads):
        parser.error(f"--workloads must name distinct families from {', '.join(FAMILIES)}")
    args.stages = list(STAGE_SCHEDULE) if args.stages == "all" else [name for name in args.stages.split(",") if name]
    if [name for name in args.stages if name not in STAGE_SCHEDULE] or len(set(args.stages)) != len(args.stages):
        parser.error(f"--stages must be 'all' or distinct steps from {', '.join(STAGE_SCHEDULE)}")
    if args.control:
        if args.stages:
            parser.error("--control selects its own stage step; do not combine it with --stages")
        args.stages = [CONTROLS[args.control]["stage"]]
        family = CONTROLS[args.control].get("family")
        if family and family not in args.workloads:
            parser.error(f"--control {args.control} needs the {family} row family in --workloads")
    if not args.elle_jar.is_file():
        parser.error("--elle-jar must name the built pinned elle-cli jar")
    if args.analyze is None and not (args.barn and args.db and args.jepsen_jar):
        parser.error("a campaign needs --barn, --db and --jepsen-jar (or --analyze DIR)")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    inputs = output
    if args.analyze is None:
        run_campaign(args, output)
    else:
        inputs = args.analyze.resolve(strict=True)
    try:
        results, findings = analyze(inputs, output, args.elle_jar.resolve(), args.java)
    except FileNotFoundError as error:
        results, findings = {}, [{"class": INVALID, "message": f"campaign directory is incomplete: {error}"}]
    verdict = verdict_of(findings)
    results.update(verdict=verdict, classes=[kind for kind in PRECEDENCE
                                             if any(f["class"] == kind for f in findings)],
                   findings=findings, argv=sys.argv if argv is None else argv, inputs=str(inputs),
                   analyze_command=[sys.executable, str(Path(__file__).resolve()), "--analyze", str(inputs),
                                    "--output", str(inputs) + "-analysis", "--elle-jar", str(args.elle_jar),
                                    "--java", args.java])
    (output / "results.json").write_text(json.dumps(results, indent=2, default=str) + "\n", encoding="utf-8")
    print(json.dumps({key: results.get(key) for key in ("outcomes", "split", "reconciliation")}, default=str),
          flush=True)
    for row in results.get("checkpoints", []):
        print(f"checkpoint comparison generation {row['generation']} ({row['kind']}): "
              f"{len(row['violations'])} violations", flush=True)
    for row in results.get("elle", []):
        print(f"elle generation {row['generation']}: {row.get('verdict', row.get('skipped'))}", flush=True)
    for family, counts in (results.get("families") or {}).get("totals", {}).items():
        print(f"family {family}: {counts['ok_reads']} successful transactions with a read, "
              f"{counts['ok_writes']} with a write", flush=True)
    print(f"checkpoint-crash stages: {results.get('checkpoint_stages')}", flush=True)
    for row in results.get("recoveries", []):
        print(f"recovery after generation {row['generation']} (stage {row.get('stage')}): "
              f"{row.get('selected_cut', row.get('skipped', 'no permitted cut'))}", flush=True)
    for row in results.get("coverage", []):
        print(f"stage {row['stage']}: requested {row['requested']}, reached {row.get('reached')}, "
              f"state {row.get('stopped_state')}, frozen {row.get('frozen_stage')}, exit {row.get('killed_exit')}, "
              f"cut {row.get('selected_cut') or row.get('candidate_cuts')}: {row['verdict']}", flush=True)
    for finding in findings:
        print(f"{finding['class']}: {finding['message']}", file=sys.stderr)
    print(f"verdict: {verdict}", flush=True)
    control = (results.get("provenance") or {}).get("parameters", {}).get("control")
    if control:
        # A negative control is expected to fail with its witness; that is its pass.
        results["control"] = control_result(control, results)
        (output / "results.json").write_text(json.dumps(results, indent=2, default=str) + "\n", encoding="utf-8")
        print(f"control {control}: witness {results['control']['witness']} "
              f"{'reported' if results['control']['passed'] else 'MISSING'} "
              f"(observed {results['control']['observed_witnesses']})", flush=True)
        return 0 if results["control"]["passed"] else CONTROL_MISSED_EXIT
    return EXIT_CODES[verdict]


if __name__ == "__main__":
    sys.exit(main())
