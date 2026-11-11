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

The adapter journals every operation before it is sent and after it
completes (journal.jsonl), so an interrupted run keeps its evidence; analysis
reconciles that journal with Jepsen's history by operation id. Before each
crash and checkpoint the campaign reads every row its clients touched in the
generation, and after each resume and launch it requires a bounded
fresh-connection write and read. Analysis checks each scenario obligation and
each generation's runtime log, and reports one of four non-healthy result
classes (see EXIT_CODES) with every finding.
"""
from __future__ import annotations

import argparse
import json
import os
import platform
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
FAULTS = ("pause", "resume", "crash", "checkpoint")

CORRECTNESS = "correctness-violation"
RECOVERY = "recovery-failure"
INCOMPLETE = "incomplete-experiment"
INVALID = "invalid-harness-evidence"
# Exit status names the most severe class found; results.json lists them all.
PRECEDENCE = (CORRECTNESS, RECOVERY, INVALID, INCOMPLETE)
EXIT_CODES = {"healthy": 0, CORRECTNESS: 1, RECOVERY: 3, INCOMPLETE: 4, INVALID: 5}

ADMIT_CODE = (
    'if (!("jepsen_ledger" in properties(#0))) add_property(#0, "jepsen_ledger", [], {#0, "r"}); endif '
    'if (!("jepsen_rows" in properties(#0))) add_property(#0, "jepsen_rows", [], {#0, "r"}); endif '
    'if (!("jepsen_probe" in properties(#0))) add_property(#0, "jepsen_probe", 0, {#0, "r"}); endif '
    '#0.jepsen_rows = []; return #0.jepsen_ledger;')
LEDGER_CODE = "return #0.jepsen_ledger;"
PROBE_READ_CODE = "return #0.jepsen_probe;"


# ---------------------------------------------------------------------------
# MOO transactions and native outcome categorisation

def _list_at(name, key):
    return f"(maphaskey({name}, {key}) ? {name}[{key}] | {{}})"


def txn_code(txn):
    """One MOO task: append to the per-boot row and the durable ledger, read rows."""
    statements = ["rows = #0.jepsen_rows;", "led = #0.jepsen_ledger;", "out = {};"]
    appends = False
    for mop in txn:
        if not (isinstance(mop, list) and len(mop) == 3 and type(mop[1]) is int):
            raise ValueError(f"malformed micro-operation {mop!r}")
        f, key, value = mop
        if f == "append" and type(value) is int:
            appends = True
            statements.append(f"rows[{key}] = {{@{_list_at('rows', key)}, {value}}};")
            statements.append(f"led[{key}] = {{@{_list_at('led', key)}, {value}}};")
        elif f == "r" and value is None:
            statements.append(f"out = {{@out, {_list_at('rows', key)}}};")
        else:
            raise ValueError(f"malformed micro-operation {mop!r}")
    if appends:
        statements += ["#0.jepsen_rows = rows;", "#0.jepsen_ledger = led;"]
    statements.append("return out;")
    return " ".join(statements)


def observation_code(keys):
    """Read every listed per-boot row in one task."""
    return "rows = #0.jepsen_rows; return {" + ", ".join(_list_at("rows", key) for key in keys) + "};"


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
                       ("phase1", "phase3", "rate", "concurrency", "seed", "jepsen_timeout")},
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
                 probe_deadline=PROBE_DEADLINE):
        self.executable, self.output = executable, output
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
                value = self.evaluate(live[1], observation_code(keys))
                if not (isinstance(value, list) and len(value) == len(keys) and all(_int_list(v) for v in value)):
                    raise RuntimeError(f"malformed observation {value!r}")
                return value
            ok, value, errors, seconds = self.bounded(attempt)
            row.update(ok=ok, value=value, failed_attempts=errors, seconds=seconds,
                       deadline_seconds=self.probe_deadline)
        self.record(row)

    def launch(self, kind):
        """Start the next generation from an exact copy of the launch source."""
        self.generation += 1
        generation, source = self.generation, self.source
        run_dir = self.output / f"gen-{generation:03d}"
        run_dir.mkdir()
        database = run_dir / "input.db"
        shutil.copyfile(source["database"], database)
        if barn_process.bench.sha256_file(database) != source["database_sha256"]:
            raise RuntimeError("generation database copy does not match its verified source")
        if source["waif_identities"] is not None:
            sidecar = Path(str(database) + ".waifids")
            shutil.copyfile(source["waif_identities"], sidecar)
            if barn_process.bench.sha256_file(sidecar) != source["waif_identities_sha256"]:
                raise RuntimeError("generation WAIF identity copy does not match its verified source")
        port = self.port_source()
        server = self.server_class(Path(self.executable), database, run_dir, port)
        self.server = server
        server.start()
        recovered = parse_ledger(self.evaluate(port, ADMIT_CODE))
        self.record({"event": "admit", "generation": generation, "kind": kind, "port": port,
                     "source": {key: value for key, value in source.items() if key != "baseline"},
                     "baseline_generation": source["baseline_generation"],
                     "baseline": None if source["baseline"] is None else ledger_json(source["baseline"]),
                     "recovered": ledger_json(recovered), "server": server.identity})
        with self.lock:
            self.live = (generation, port)
        self.probe("launch")
        return generation

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

    def fault(self, name):
        with self.lifecycle:
            if self.server is None or self.failures:
                return {"type": "info", "error": {"kind": "campaign-failed", "failures": self.failures}}
            with self.lock:
                self.fault_seq += 1
                seq, self.active_fault = self.fault_seq, name
            self.journal({"event": "fault-begin", "fault_seq": seq, "f": name, "generation": self.generation})
            try:
                reply = {"type": "ok", "value": getattr(self, name)(), "generation": self.generation}
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
        code = txn_code(txn)
        campaign = self.campaign
        with campaign.lock:
            live, phase = campaign.live, campaign.phase()
        if live is None:
            return {"type": "fail", "generation": None, "error": {"kind": "no-live-generation"}}
        generation, port = live
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
    return {(key, value) for op in ops if op["type"] in types
            for f, key, value in op["value"] if f == "append"}


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


def outcome_counts(history):
    counts = {}
    for op in history:
        if type(op["process"]) is int and op["type"] != "invoke":
            kind = (op.get("error") or {}).get("kind", "")
            label = op["type"] + (":" + kind if kind else "")
            counts[label] = counts.get(label, 0) + 1
    return counts


def generation_stats(buckets):
    stats = {}
    for generation, ops in buckets.items():
        row = stats.setdefault(generation, {"completions": 0, "ok": 0, "ok_reads": 0, "ok_writes": 0})
        for op in ops:
            if op["type"] == "invoke":
                continue
            row["completions"] += 1
            if op["type"] == "ok":
                functions = {mop[0] for mop in op["value"]}
                row["ok"] += 1
                row["ok_reads"] += "r" in functions
                row["ok_writes"] += "append" in functions
    return stats


FAULT_EVENTS = ("admit", "pause", "resume", "crash", "checkpoint")


def obligations(buckets, lifecycle):
    """Every obligation of the fixed pause/checkpoint/crash scenario that the evidence does not meet.

    Returns (stats, [(class, message)]). A probe or observation that ran and
    failed is a recovery/progress failure; anything absent is incomplete.
    """
    stats = generation_stats(buckets)
    missing = []

    def useful(generation, when):
        row = stats.get(generation, {})
        for key, what in (("ok_reads", "read"), ("ok_writes", "write")):
            if not row.get(key):
                missing.append((INCOMPLETE, f"{when}: generation {generation} has no successful client "
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
    for name in ("pause", "crash", "checkpoint"):
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
    results["generations"], missing = obligations(buckets, lifecycle)
    results["missing_obligations"] = [message for _, message in missing]
    for kind, message in missing:
        find(kind, message)
    results["elle"] = []
    observed = {row["generation"] for row in lifecycle if row.get("event") == "observation"}
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


# ---------------------------------------------------------------------------

def run_campaign(args, output):
    campaign = Campaign(args.barn.resolve(strict=True), args.db, output, socket_transport_factory())
    adapter = None
    try:
        (output / "provenance.json").write_text(json.dumps(provenance(args, output), indent=2, default=str) + "\n",
                                                encoding="utf-8")
        campaign.launch("initial")
        adapter = AdapterServer(campaign)
        threading.Thread(target=adapter.serve_forever, daemon=True).start()
        command = [args.java, JVM_HEAP, "-Djava.awt.headless=true", "-jar", str(args.jepsen_jar.resolve(strict=True)),
                   "--adapter-port", str(adapter.server_address[1]), "--history", str(output / "history.jsonl"),
                   "--phase1", str(args.phase1), "--phase3", str(args.phase3), "--rate", str(args.rate),
                   "--concurrency", str(args.concurrency), "--seed", str(args.seed)]
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
    parser.add_argument("--rate", type=int, default=20, help="total transactions per second")
    parser.add_argument("--concurrency", type=int, default=8)
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument("--jepsen-timeout", type=int, default=900)
    args = parser.parse_args(argv)
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
    for finding in findings:
        print(f"{finding['class']}: {finding['message']}", file=sys.stderr)
    print(f"verdict: {verdict}", flush=True)
    return EXIT_CODES[verdict]


if __name__ == "__main__":
    sys.exit(main())
