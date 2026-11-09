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
never treats uncheckpointed replies as durable. Zero completed checkpoint
comparisons, or zero Elle-checked generations, is an incomplete campaign.
"""
from __future__ import annotations

import argparse
import json
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

ADMIT_CODE = (
    'if (!("jepsen_ledger" in properties(#0))) add_property(#0, "jepsen_ledger", [], {#0, "r"}); endif '
    'if (!("jepsen_rows" in properties(#0))) add_property(#0, "jepsen_rows", [], {#0, "r"}); endif '
    '#0.jepsen_rows = []; return #0.jepsen_ledger;')
LEDGER_CODE = "return #0.jepsen_ledger;"


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
# Lifecycle owner: one OwnedBarnServer per process generation

def free_port():
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        return probe.getsockname()[1]


class Campaign:
    def __init__(self, executable, source_db, output, transport_factory, *, server_class=None, port_source=free_port):
        self.executable, self.output = executable, output
        self.transport_factory = transport_factory
        self.server_class = server_class or (lambda *args, **kwargs: barn_process.OwnedBarnServer(
            *args, config=CONFIG, profile_id=PROFILE_ID, **kwargs))
        self.port_source = port_source
        self.lock = threading.Lock()        # guards self.live
        self.lifecycle = threading.Lock()   # serializes faults and launches
        self.live = None                    # (generation, port) while clients may send
        self.server = None
        self.generation = -1
        self.failures = []
        # The launch source: the original fixture, then each verified pair.
        self.source = {"database": str(source_db.resolve()),
                       "database_sha256": barn_process.bench.sha256_file(source_db),
                       "waif_identities": None, "waif_identities_sha256": None,
                       "baseline": None, "baseline_generation": None}
        self.log_path = output / "lifecycle.jsonl"

    def record(self, row):
        row = dict(row, monotonic_ns=time.monotonic_ns())
        with self.log_path.open("a", encoding="utf-8") as stream:
            stream.write(json.dumps(row, default=str) + "\n")

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
        return generation

    def _quiesce(self):
        with self.lock:
            live, self.live = self.live, None
        return live

    def pause(self):
        self.server.pause()
        self.record({"event": "pause", "generation": self.generation})
        return {"generation": self.generation}

    def resume(self):
        if self.server.paused:
            self.server.resume()
        self.record({"event": "resume", "generation": self.generation})
        return {"generation": self.generation}

    def crash(self):
        generation = self.generation
        self._quiesce()
        result = self.server.crash()
        self.record({"event": "crash", "generation": generation, "result": result})
        return {"crashed": generation, "generation": self.launch("crash")}

    def checkpoint(self):
        generation = self.generation
        if self.server.paused:
            self.server.resume()
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
            try:
                return {"type": "ok", "value": getattr(self, name)(), "generation": self.generation}
            except Exception as error:
                self._quiesce()
                return {"type": "info", "error": {"kind": "lifecycle", "message": self.fail(name, error)},
                        "generation": self.generation}

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

    def txn(self, txn):
        code = txn_code(txn)
        with self.campaign.lock:
            live = self.campaign.live
        if live is None:
            return {"type": "fail", "generation": None, "error": {"kind": "no-live-generation"}}
        generation, port = live
        if self.transport is None or self.generation != generation:
            self.close()
            transport = self.campaign.transport_factory(port)
            try:
                transport.connect("wizard")
            except Exception as error:
                transport.disconnect()
                # The transaction was never sent.
                return {"type": "fail", "generation": generation,
                        "error": {"kind": "connect", "message": f"{type(error).__name__}: {error}"}}
            self.transport, self.generation = transport, generation
        transport = self.transport
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
                    reply = session.txn(request["value"])
                elif request.get("op") in ("pause", "resume", "crash", "checkpoint"):
                    reply = campaign.fault(request["op"])
                else:
                    reply = {"type": "info", "error": {"kind": "unknown-request", "request": request}}
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
# Analysis: generation splitting, Elle EDN, checkpoint durability

def load_jsonl(path):
    return [json.loads(line) for line in Path(path).read_text(encoding="utf-8").splitlines() if line.strip()]


def split_generations(history):
    """Assign each client invocation/completion pair to its adapter generation.

    Returns ({generation: ops in history order}, counts). An operation the
    adapter never sent is :fail with no generation and is excluded. Any other
    completion without a generation cannot be placed in one process lifetime,
    which would make per-generation checking unsound, so it is an error.
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
            if op["type"] != "fail":
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


def analyze(inputs, output, jar, java):
    """Check one finished campaign directory; returns (results, errors)."""
    lifecycle = load_jsonl(inputs / "lifecycle.jsonl")
    errors = [row["error"] for row in lifecycle if row.get("event") == "failure"]
    try:
        history = load_jsonl(inputs / "history.jsonl")
    except FileNotFoundError:
        return {}, errors + ["incomplete campaign: Jepsen wrote no history.jsonl"]
    results = {"outcomes": outcome_counts(history)}
    for label, count in results["outcomes"].items():
        if label.endswith((":native-error", ":malformed-response", ":adapter-timeout", ":adapter-io")):
            errors.append(f"{count} client completions were {label}")
    try:
        buckets, results["split"] = split_generations(history)
    except ValueError as error:
        return results, errors + [f"history cannot be split per generation: {error}"]
    results["checkpoints"] = checkpoint_comparisons(buckets, lifecycle)
    if not results["checkpoints"]:
        errors.append("incomplete campaign: zero completed checkpoint comparisons")
    for row in results["checkpoints"]:
        errors += [f"generation {row['generation']} ({row['kind']}): {problem}" for problem in row["violations"]]
    results["elle"] = []
    for generation, ops in sorted(buckets.items()):
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
            errors.append(f"generation {generation}: {error}")
        if exit_code != 0 or row.get("verdict") != "true":
            errors.append(f"generation {generation}: Elle exit={exit_code}, verdict={row.get('verdict')}")
        results["elle"].append(row)
    if not any("verdict" in row for row in results["elle"]):
        errors.append("incomplete campaign: zero Elle-checked generations")
    return results, errors


# ---------------------------------------------------------------------------

def run_campaign(args, output):
    campaign = Campaign(args.barn.resolve(strict=True), args.db, output, socket_transport_factory())
    adapter = None
    try:
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
        results, errors = analyze(inputs, output, args.elle_jar.resolve(), args.java)
    except FileNotFoundError as error:
        results, errors = {}, [f"incomplete campaign: {error}"]
    results.update(argv=sys.argv, inputs=str(inputs), errors=errors,
                   analyze_command=[sys.executable, str(Path(__file__).resolve()), "--analyze", str(inputs),
                                    "--output", str(inputs) + "-analysis", "--elle-jar", str(args.elle_jar),
                                    "--java", args.java])
    (output / "results.json").write_text(json.dumps(results, indent=2, default=str) + "\n", encoding="utf-8")
    print(json.dumps({key: results.get(key) for key in ("outcomes", "split")}, default=str), flush=True)
    for row in results.get("checkpoints", []):
        print(f"checkpoint comparison generation {row['generation']} ({row['kind']}): "
              f"{len(row['violations'])} violations", flush=True)
    for row in results.get("elle", []):
        print(f"elle generation {row['generation']}: {row.get('verdict', row.get('skipped'))}", flush=True)
    for error in errors:
        print(error, file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
