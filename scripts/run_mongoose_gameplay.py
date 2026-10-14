#!/usr/bin/env python3
"""Run real flight and magic in a private, hash-pinned Mongoose checkpoint copy."""
from __future__ import annotations

import argparse
import hashlib
import importlib
import json
import os
from pathlib import Path
import shutil
import sys
import threading
import time

import profile_offline_mongoose as offline
from mongoose_scenario_support import ScenarioFailure


class RecordingControl:
    """Keep tagged native output and request hashes; never log login credentials."""
    def __init__(self, control, path):
        self.control = control
        self.output = path.open("x", encoding="utf-8")
        self.phase = "setup"

    def eval(self, code, **kwargs):
        started = time.monotonic_ns()
        record = {"phase": self.phase, "started_monotonic_ns": started,
                  "request_sha256": hashlib.sha256(code.encode()).hexdigest()}
        try:
            value, lines = self.control.eval(code, **kwargs)
            record.update(result=value, output=lines, status="returned")
            return value, lines
        except Exception as error:
            record.update(status="failed", failure_type=type(error).__name__, failure=str(error),
                          evidence=getattr(error, "evidence", None),
                          native_output=getattr(error, "native_output", None))
            raise
        finally:
            record["finished_monotonic_ns"] = time.monotonic_ns()
            self.output.write(json.dumps(record) + "\n")
            self.output.flush()

    def close(self):
        self.output.close()


class ProcessObserver:
    """External RSS/main-thread CPU chronology, including all background work."""
    def __init__(self, server, path):
        self.server = server
        self.path = path
        self.phase = "setup"
        self.error = None
        self.count = 0
        self.stop_event = threading.Event()
        self.thread = threading.Thread(target=self._collect, daemon=True)

    def _collect(self):
        try:
            with self.path.open("x", encoding="utf-8") as stream:
                while not self.stop_event.is_set():
                    row = offline.process_state(self.server.proc.pid)
                    if row["start_ticks"] != self.server.start_ticks:
                        raise RuntimeError("process observer identity changed")
                    row["phase"] = self.phase
                    stream.write(json.dumps(row) + "\n")
                    stream.flush()
                    self.count += 1
                    self.stop_event.wait(0.25)
        except Exception as error:
            self.error = {"failure_type": type(error).__name__, "failure": str(error)}

    def start(self):
        self.thread.start()

    def finish(self):
        self.stop_event.set()
        self.thread.join(timeout=5)
        if self.thread.is_alive():
            raise RuntimeError("process observer did not stop")
        return {"samples": self.count, "failure": self.error,
                "scope": "whole owned process RSS and main-thread CPU; all background work included"}


def save_profile(control, run_dir, prefix):
    started = time.monotonic_ns()
    try:
        ack, _ = control.eval(
            'p = moo_cpu_profile("stop"); '
            f'f = file_open("{prefix}-cpu-profile.json", "w-tn"); '
            'try file_writeline(f, generate_json(p)); finally file_close(f); endtry '
            'return {p["recorded"], p["sampling_valid"]};', timeout=120)
    finally:
        offline.write_json(run_dir / f"{prefix}-stop-control.json",
                           {"wall_ns": time.monotonic_ns() - started,
                            "scope": "queueing, stop, wrappers, export, serialization and FileIO"})
    raw = run_dir / "files" / f"{prefix}-cpu-profile.json"
    payload = json.loads(raw.read_text())
    validated = offline.report.validate(payload)
    (run_dir / f"{prefix}-cpu-report.md").write_text(offline.report.markdown(validated), encoding="utf-8")
    (run_dir / f"{prefix}-cpu.folded").write_text(offline.report.folded(validated), encoding="utf-8")
    return {"acknowledgement": ack, "raw_sha256": offline.bench.sha256_file(raw),
            "scope": "instrumented whole-main-thread CPU; not per-action CPU"}


def run_scenario(name, control, server, observer, options):
    module = importlib.import_module(f"mongoose_{name}_scenario")
    directory = options.run_dir / name
    directory.mkdir()
    result = {"scenario": name, "status": "initializing", "iterations_requested": options.iterations,
              "invocation_scope": "ordinary-permission domain entries; not client/network latency"}
    fixture = None
    profiling = False
    native = None
    primary_error = None
    try:
        observer.phase = control.phase = name + ":setup"
        fixture = module.setup(control, options)
        offline.write_json(directory / "fixture.json", fixture)
        result["before"] = module.snapshot(control, fixture)
        if options.profile != "off":
            request = offline.profile_start_request(options.hz, options.profile)
            # Mark the attempt before the RPC: an invalid response can arrive
            # after the native timer starts, so finally must still stop/export.
            profiling = True
            ack, _ = control.eval('p = ' + request.removeprefix('return ') +
                                  ' return {p["running"], p["metadata_mode"]};')
            result["moo_profile_start"] = ack
            if ack != [1, options.profile]:
                raise ScenarioFailure("MOO CPU sampler did not acknowledge the requested mode", ack)
        if options.native_mode != "off":
            native_module = importlib.import_module("mongoose_native_profile")
            native = native_module.begin(server, directory / "native", options)
        else:
            result["native_profile"] = {"status": "disabled"}
        observer.phase = control.phase = name + ":actions"
        server.verify_owned()
        before = offline.process_state(server.proc.pid)
        result["process_before"] = before
        started = time.monotonic_ns()
        action_error = None
        try:
            result["actions"] = module.run(control, fixture, options)
            if (not isinstance(result["actions"], dict)
                    or type(result["actions"].get("completed_iterations")) is not int
                    or result["actions"]["completed_iterations"] != options.iterations):
                raise ScenarioFailure("scenario did not prove every requested iteration", result.get("actions"))
        except Exception as error:
            action_error = error
            raise
        finally:
            result["action_wall_ns"] = time.monotonic_ns() - started
            try:
                result["process_after"] = offline.process_state(server.proc.pid)
                result["main_cpu_ns"] = result["process_after"]["main_cpu_ns"] - before["main_cpu_ns"]
            except Exception as error:
                result["measurement_failure"] = str(error)
                if action_error is None:
                    raise
        result["after"] = module.snapshot(control, fixture)
        result["status"] = "complete"
    except Exception as error:
        primary_error = error
        result.update(status="failed", failure_type=type(error).__name__, failure=str(error),
                      failure_evidence=getattr(error, "evidence", None))
    finally:
        observer.phase = control.phase = name + ":drain"
        if native is not None:
            try:
                result["native_profile"] = native.finish()
                if options.native_mode == "required" and result["native_profile"].get("status") != "collected":
                    raise ScenarioFailure("required native CPU profile was not collected", result["native_profile"])
            except Exception as error:
                result["native_profile_failure"] = str(error)
                primary_error = primary_error or error
        if profiling:
            try:
                result["moo_profile"] = save_profile(control, options.run_dir, name)
            except Exception as error:
                result["moo_profile_failure"] = str(error)
                primary_error = primary_error or error
        if fixture is not None:
            try:
                result["cleanup"] = module.cleanup(control, fixture)
            except Exception as error:
                result["cleanup_failure"] = str(error)
                primary_error = primary_error or error
        if primary_error:
            result["status"] = "failed"
        offline.write_json(directory / "result.json", result)
    if primary_error:
        raise ScenarioFailure(f"{name} scenario failed: {primary_error}", result) from primary_error
    return result


def finalize_verdict(manifest):
    checks = {"source_database": manifest.get("source_database_unchanged"),
              "executable": manifest.get("executable_unchanged"),
              "controller_sources": manifest.get("controller_sources_unchanged"),
              "sound_catalog": manifest.get("sound_catalog", {}).get("source_unchanged"),
              "owned_shutdown": manifest.get("shutdown", {}).get("exit_code") == 0}
    manifest["final_checks"] = checks
    if manifest["status"] == "complete" and not all(value is True for value in checks.values()):
        manifest["status"] = "invalidated"


def arguments():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--db", type=Path, required=True)
    parser.add_argument("--db-sha256", required=True)
    parser.add_argument("--moo", type=Path, required=True)
    parser.add_argument("--moo-sha256", required=True)
    parser.add_argument("--sound-db", type=Path, required=True)
    parser.add_argument("--sound-db-sha256", required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--source-commit")
    parser.add_argument("--scenario", choices=("both", "flight", "magic"), default="both")
    parser.add_argument("--iterations", type=int, default=1)
    parser.add_argument("--deadline-seconds", type=float, default=120)
    parser.add_argument("--warmup-seconds", type=float, default=30)
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument("--port", type=int, default=17778)
    parser.add_argument("--profile", choices=("off", "definitions", "detailed"), default="off")
    parser.add_argument("--hz", type=int, default=100)
    parser.add_argument("--native-profile", dest="native_mode", choices=("off", "auto", "required"), default="off")
    parser.add_argument("--native-hz", type=int, default=99)
    options = parser.parse_args()
    if sys.platform != "linux" or options.port == 7777 or not 1 <= options.port <= 65535:
        parser.error("requires Linux and a nonproduction port")
    if not 1 <= options.iterations <= 100 or not 0 < options.deadline_seconds <= 600:
        parser.error("iterations must be1..100 and deadline-seconds in(0,600]")
    if not 0 <= options.warmup_seconds <= 120 or not 10 <= options.hz <= 1000:
        parser.error("warmup-seconds must be0..120 and hz10..1000")
    if not 10 <= options.native_hz <= 1000:
        parser.error("native-hz must be10..1000")
    return options


def main():
    options = arguments()
    network = offline.check_network_isolation()
    source = options.db.resolve(strict=True)
    executable = options.moo.resolve(strict=True)
    if not source.is_file() or not executable.is_file():
        raise ValueError("requires regular database and executable files")
    source_sha = offline.bench.sha256_file(source)
    executable_sha = offline.bench.sha256_file(executable)
    if source_sha.lower() != options.db_sha256.lower() or executable_sha.lower() != options.moo_sha256.lower():
        raise ValueError("source database or executable SHA-256 mismatch")
    options.run_dir = options.out.resolve()
    options.host = "127.0.0.1"
    options.actor_connections = []
    options.run_dir.mkdir(parents=True, exist_ok=False)
    os.chmod(options.run_dir, 0o700)
    manifest = {"status": "initializing", "source_database": str(source), "source_database_sha256": source_sha,
                "executable": str(executable), "executable_sha256": executable_sha,
                "source_commit": options.source_commit, "network": network,
                "scenario": options.scenario, "seed": options.seed, "iterations": options.iterations,
                "profile": options.profile, "native_profile": options.native_mode,
                "controller_sha256": offline.bench.sha256_file(Path(__file__)),
                "scope": "owned offline domain-entry gameplay; all main-thread background CPU included"}
    selected = ("flight", "magic") if options.scenario == "both" else (options.scenario,)
    scripts = Path(__file__).resolve().parent
    controller_files = [Path(__file__), scripts / "mongoose_scenario_support.py",
                        scripts / "profile_offline_mongoose.py", scripts / "bench_players.py"] + [scripts / f"mongoose_{name}_scenario.py" for name in selected]
    if options.native_mode != "off":
        controller_files.append(scripts / "mongoose_native_profile.py")
    if options.profile != "off":
        controller_files.append(scripts / "report_moo_cpu_profile.py")
    manifest["controller_sources"] = {path.name: offline.bench.sha256_file(path) for path in controller_files}
    source_snapshot = options.run_dir / "controller-sources"
    source_snapshot.mkdir()
    for path in controller_files:
        destination = source_snapshot / path.name
        with path.open("rb") as incoming, destination.open("xb") as outgoing:
            shutil.copyfileobj(incoming, outgoing)
        if offline.bench.sha256_file(destination) != manifest["controller_sources"][path.name]:
            raise RuntimeError("controller source changed while recording its snapshot")
    offline.write_json(options.run_dir / "manifest-initial.json", manifest)
    control = recorded = observer = None
    database = options.run_dir / "mongoose.db"
    server = offline.OwnedServer(executable, database, options.run_dir, options.port)
    try:
        shutil.copyfile(source, database)
        if offline.bench.sha256_file(database) != source_sha:
            raise RuntimeError("source database changed during copy")
        (options.run_dir / "files").mkdir()
        (options.run_dir / "executables").mkdir()
        manifest["sound_catalog"] = offline.prepare_sound_catalog(options.sound_db, options.sound_db_sha256, options.run_dir)
        offline.adapt_bootstrap(executable, options.run_dir)
        proxy, user, password = offline.bench.bootstrap_control_account(options.run_dir, database, options.port)
        manifest["startup_seconds"] = server.start()
        control, _ = offline.bench.connect_control(server, proxy, user, password, "immediate", 0.25, "gameplay")
        identity, _ = control.eval('return {player, player.wizard, $prod(), server_version()};')
        manifest["control_identity"] = identity
        if identity[1] != 1 or identity[2] != 0:
            raise RuntimeError("requires private wizard setup and $prod()==0")
        recorded = RecordingControl(control, options.run_dir / "transcript.jsonl")
        manifest["before_warmup"] = offline.snapshot(recorded)
        if options.warmup_seconds:
            time.sleep(options.warmup_seconds)
        server.verify_owned()
        manifest["after_warmup"] = offline.snapshot(recorded)
        observer = ProcessObserver(server, options.run_dir / "process-samples.jsonl")
        observer.start()
        manifest["scenarios"] = []
        for name in selected:
            manifest["scenarios"].append(run_scenario(name, recorded, server, observer, options))
        manifest["status"] = "complete"
    except Exception as error:
        manifest.update(status="failed", failure_type=type(error).__name__, failure=str(error),
                        failure_evidence=getattr(error, "evidence", None))
        raise
    finally:
        manifest["finalization_errors"] = []
        for label, target in [("actor connection", conn) for conn in options.actor_connections] + [
                ("transcript", recorded), ("control connection", control)]:
            if target:
                try:
                    target.close()
                except Exception as error:
                    manifest["finalization_errors"].append({"component": label, "failure": str(error)})
        if observer:
            try:
                manifest["process_observer"] = observer.finish()
                if observer.error:
                    manifest["finalization_errors"].append({"component": "process observer", **observer.error})
            except Exception as error:
                manifest["finalization_errors"].append({"component": "process observer", "failure": str(error)})
        if manifest["finalization_errors"] and manifest["status"] == "complete":
            manifest["status"] = "measurement-failed"
        try:
            manifest["shutdown"] = server.stop()
        except Exception as error:
            manifest["shutdown"] = {"exit_code": None, "failure": str(error)}
            raise
        finally:
            manifest["source_database_final_sha256"] = offline.bench.sha256_file(source)
            manifest["source_database_unchanged"] = manifest["source_database_final_sha256"] == source_sha
            manifest["executable_final_sha256"] = offline.bench.sha256_file(executable)
            manifest["executable_unchanged"] = manifest["executable_final_sha256"] == executable_sha
            manifest["controller_sources_unchanged"] = all(
                offline.bench.sha256_file(path) == manifest["controller_sources"][path.name] for path in controller_files)
            if "sound_catalog" in manifest:
                catalog = manifest["sound_catalog"]
                catalog["source_final_sha256"] = offline.bench.sha256_file(options.sound_db)
                catalog["source_unchanged"] = catalog["source_final_sha256"] == catalog["source_sha256"]
            finalize_verdict(manifest)
            offline.write_json(options.run_dir / "manifest-final.json", manifest)
    if (manifest["status"] != "complete" or manifest["shutdown"]["exit_code"] != 0
            or not manifest["source_database_unchanged"] or not manifest["sound_catalog"]["source_unchanged"]
            or not manifest["controller_sources_unchanged"] or not manifest["executable_unchanged"]):
        raise RuntimeError("gameplay or measurement/immutable-source/shutdown checks failed; inspect final manifest")
    print(json.dumps({"status": "complete", "run_dir": str(options.run_dir),
                      "scenarios": [row["scenario"] for row in manifest["scenarios"]]}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
