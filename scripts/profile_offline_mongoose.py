#!/usr/bin/env python3
"""Own a network-isolated, disposable Mongoose CPU diagnostic session.

Run inside WSL/Linux: unshare --net -- bash -lc
'set -euo pipefail; ip link set lo up; python3 scripts/profile_offline_mongoose.py ...'
This captures all main-thread work, including naturally scheduled background work.
It neither contacts production nor claims ordinary gameplay performance.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import shutil
import signal
import socket
import sqlite3
import subprocess
import sys
import time

import bench_players as bench
import report_moo_cpu_profile as report


def write_json(path: Path, value) -> None:
    with path.open("x", encoding="utf-8") as stream:
        json.dump(value, stream, indent=2)
        stream.write("\n")


def prepare_sound_catalog(source: Path, expected_sha256: str, run_dir: Path) -> dict:
    """Copy an immutable complete catalog; never open the source with SQLite."""
    source = source.resolve(strict=True)
    if not source.is_file():
        raise ValueError("sound catalog must be a regular file")
    wal = Path(str(source) + "-wal")
    if wal.exists() and wal.stat().st_size:
        raise ValueError("sound catalog has a nonempty WAL; supply a complete checkpointed SQLite snapshot")
    original_hash = bench.sha256_file(source)
    if original_hash.lower() != expected_sha256.lower():
        raise ValueError("sound catalog hash mismatch")
    destination = run_dir / "files" / "sqlite" / "sound.sqlite"
    destination.parent.mkdir()
    with source.open("rb") as incoming, destination.open("xb") as outgoing:
        shutil.copyfileobj(incoming, outgoing)
    copied_hash = bench.sha256_file(destination)
    if copied_hash != original_hash or bench.sha256_file(source) != original_hash:
        raise RuntimeError("sound catalog changed while being copied")
    if wal.exists() and wal.stat().st_size:
        raise RuntimeError("sound catalog WAL changed while being copied")
    # Only the owned copy is opened, even for integrity/schema inspection.
    connection = sqlite3.connect(destination)
    try:
        integrity = connection.execute("PRAGMA quick_check").fetchall()
        if integrity != [("ok",)]:
            raise ValueError("copied sound catalog failed SQLite quick_check")
        count = connection.execute("SELECT count(*) FROM Sounds").fetchone()[0]
        if count <= 0:
            raise ValueError("copied sound catalog has no Sounds rows")
    finally:
        connection.close()
    return {"source": str(source), "source_sha256": original_hash,
            "copy_initial_sha256": copied_hash, "sounds_rows": count,
            "source_wal_bytes": 0, "sqlite_quick_check": "ok",
            "scope": "local catalog snapshot; no network or MOO repairs"}


def check_network_isolation() -> dict:
    own = os.readlink("/proc/self/ns/net")
    host = os.readlink("/proc/1/ns/net")
    links = subprocess.run(["ip", "-j", "link", "show"], check=True,
                           capture_output=True, text=True)
    interfaces = json.loads(links.stdout)
    routes = subprocess.run(["ip", "-j", "route", "show", "table", "all"],
                            check=True, capture_output=True, text=True)
    routes6 = subprocess.run(["ip", "-j", "-6", "route", "show", "table", "all"],
                             check=True, capture_output=True, text=True)
    route_rows = json.loads(routes.stdout) + json.loads(routes6.stdout)
    if own == host or {row["ifname"] for row in interfaces} != {"lo"}:
        raise RuntimeError("requires a separate network namespace containing only loopback")
    if any(row.get("dev") != "lo" for row in route_rows):
        raise RuntimeError("network namespace has a route outside loopback")
    if not any("UP" in row["flags"] for row in interfaces):
        raise RuntimeError("network namespace loopback is not up")
    return {"namespace": own, "host_namespace": host, "interfaces": interfaces,
            "routes": route_rows, "external_network": "unavailable"}


def split_emergency_bootstrap(script: str) -> str:
    """Preserve the helper's expected connection failure without catching it."""
    match = re.search(r"try (\$account_manager:create_account\([^\n]+?\);) except e \(ANY\) endtry ", script)
    if not match or script.count("except e (ANY)") != 1:
        raise RuntimeError("bootstrap helper changed; refusing to assume its error contract")
    prefix_start = script.rfind(";;", 0, match.start())
    if prefix_start < 0:
        raise RuntimeError("bootstrap statement prefix is missing")
    prefix = script[prefix_start + 2:match.start()]
    return (script[:prefix_start] + ";;" + match.group(1) + "\n;;" + prefix
            + script[match.end():])


def adapt_bootstrap(executable: Path, run_dir: Path) -> None:
    """Reuse bench_players' password/account protocol with Linux-native subprocesses."""
    bench.TOAST_MOO = str(executable)

    def native(*args: str, timeout: float = 60.0) -> str:
        if args[0] == "wslpath":
            return str(Path(args[-1]).resolve()) + "\n"
        if args[0] == "bash":
            script = run_dir / "bootstrap.moo"
            original = script.read_text(encoding="ascii")
            adapted = split_emergency_bootstrap(original)
            script.write_text(adapted, encoding="ascii")
        completed = subprocess.run(list(args), capture_output=True, text=True,
                                   timeout=timeout, check=True)
        if args[0] == "bash":
            # Emergency mode may echo code; remove the ephemeral password from retained output.
            passwords = re.findall(r'create_account\("", "benchboot", "([a-f0-9]+)"\)', adapted)
            safe_output = completed.stdout
            for password in passwords:
                safe_output = safe_output.replace(password, "[ephemeral password redacted]")
            (run_dir / "bootstrap.stdout.txt").write_text(safe_output, encoding="utf-8")
            return safe_output
        return completed.stdout

    bench.wsl = native


def process_state(pid: int) -> dict:
    stat = Path(f"/proc/{pid}/stat").read_text()
    suffix = stat[stat.rfind(")") + 2:].split()
    status = Path(f"/proc/{pid}/status").read_text()
    rss = re.search(r"^VmRSS:\s+(\d+) kB$", status, re.M)
    high = re.search(r"^VmHWM:\s+(\d+) kB$", status, re.M)
    cpu_ns = int(Path(f"/proc/{pid}/task/{pid}/schedstat").read_text().split()[0])
    return {"monotonic_ns": time.monotonic_ns(), "main_cpu_ns": cpu_ns,
            "start_ticks": int(suffix[19]), "rss_kib": int(rss.group(1)) if rss else None,
            "high_water_rss_kib": int(high.group(1)) if high else None}


class OwnedServer:
    host = "127.0.0.1"

    def __init__(self, executable: Path, db: Path, run_dir: Path, port: int):
        self.executable, self.db, self.run_dir, self.port = executable, db, run_dir, port
        self.proc = None
        self.log_file = None
        self.pidfd = None
        self.start_ticks = None

    def start(self) -> float:
        with socket.socket() as reservation:
            reservation.bind((self.host, self.port))
        self.log_file = (self.run_dir / "server.log").open("xb")
        self.proc = subprocess.Popen([str(self.executable), str(self.db), str(self.db) + ".new",
                                      str(self.port)], cwd=self.run_dir, stdin=subprocess.DEVNULL,
                                     stdout=self.log_file, stderr=subprocess.STDOUT)
        self.pidfd = os.pidfd_open(self.proc.pid)
        self.start_ticks = process_state(self.proc.pid)["start_ticks"]
        self.verify_owned()
        return bench.wait_port(self.host, self.port, 180.0, self.proc)

    def verify_owned(self) -> None:
        pid = self.proc.pid
        argv = Path(f"/proc/{pid}/cmdline").read_bytes().split(b"\0")
        if (os.path.realpath(f"/proc/{pid}/exe") != str(self.executable)
                or argv[1:3] != [os.fsencode(self.db), os.fsencode(str(self.db) + ".new")]
                or process_state(pid)["start_ticks"] != self.start_ticks):
            raise RuntimeError("owned Toast process identity changed; refusing to signal")

    def stop(self) -> dict:
        try:
            if self.proc and self.proc.poll() is None:
                self.verify_owned()
                signal.pidfd_send_signal(self.pidfd, signal.SIGTERM)
                try:
                    self.proc.wait(timeout=90)
                except subprocess.TimeoutExpired:
                    self.verify_owned()
                    signal.pidfd_send_signal(self.pidfd, signal.SIGKILL)
                    self.proc.wait(timeout=10)
                    return {"exit_code": self.proc.returncode, "shutdown": "forced after 90s"}
            return {"exit_code": self.proc.returncode if self.proc else None,
                    "shutdown": "owned PID stopped"}
        finally:
            if self.pidfd is not None:
                os.close(self.pidfd)
                self.pidfd = None
            if self.log_file:
                self.log_file.close()


def snapshot(control: bench.Control):
    value, _ = control.eval('q = queued_tasks(0, 1); s = $sunnet_scheduler.scheduled_tasks; overdue = 0; '
                            'limit = min(length(s), 500); '
                            'for i in [1..limit] if (s[i][3] < ftime()) overdue = overdue + 1; endif endfor '
                            'errors = $wiz_utils.traceback_log; unresolved = 0; other = 0; '
                            'for e in (errors) '
                            'if ((typeof(e) == LIST) && (length(e) >= 2) && (typeof(e[2]) == STR) '
                            '&& index(e[2], "cannot resolve")) unresolved = unresolved + 1; '
                            'else other = other + 1; endif endfor '
                            'return ["current_cycle" -> #4143.current_cycle, '
                            '"retained_errors" -> ["count" -> length(errors), '
                            '"limit" -> $wiz_utils.traceback_limit, "cannot_resolve" -> unresolved, '
                            '"other" -> other, "scope" -> "bounded retained history; not an arrival count"], '
                            '"sound_history_keys" -> length(#2585.last_picks), '
                            '"electronics" -> ["time" -> #29083.electronics_state["time"], '
                            '"mode" -> #29083.electronics_state["mode"], '
                            '"updating" -> #29083.electronics_state["updating"], '
                            '"host" -> tostr(#29083.electronics_state["host"])], '
                            '"queued_count" -> q, "scheduled_count" -> length(s), '
                            '"scheduled_inspected" -> limit, "overdue_inspected" -> overdue];')
    return value


def profile_start_request(hz: int, metadata_mode: str) -> str:
    if type(hz) is not int or not 10 <= hz <= 1000:
        raise ValueError("native sampling rate must be an integer in [10,1000]")
    if metadata_mode == "detailed":
        return f'return moo_cpu_profile("start", {hz}, 65536);'
    if metadata_mode == "definitions":
        return f'return moo_cpu_profile("start", {hz}, 65536, "definitions");'
    raise ValueError("unknown profiler metadata mode")


def capture(control: bench.Control, server: OwnedServer, run_dir: Path,
            seconds: float, hz: int, mode: str, metadata_mode: str = "detailed") -> dict:
    prefix = mode
    before = snapshot(control)
    before_start_control = process_state(server.proc.pid)
    start_request_ns = time.monotonic_ns()
    ack, _ = control.eval(profile_start_request(hz, metadata_mode))
    start_rpc_wall_ns = time.monotonic_ns() - start_request_ns
    states = []
    started = time.monotonic()
    try:
        while time.monotonic() - started < seconds:
            states.append(process_state(server.proc.pid))
            time.sleep(min(0.25, max(0, seconds - (time.monotonic() - started))))
    finally:
        before_stop_control = process_state(server.proc.pid)
        write_json(run_dir / f"{prefix}-process-window.json", {
            "before": before, "before_start_control": before_start_control,
            "before_stop_control": before_stop_control, "process_samples": states})
        # The raw profile is durable even if later assertions or report validation fail.
        stop_request_ns = time.monotonic_ns()
        try:
            exported, _ = control.eval(f'p = moo_cpu_profile("stop"); '
                                      f'f = file_open("{prefix}-cpu-profile.json", "w-tn"); '
                                      'try file_writeline(f, generate_json(p)); finally file_close(f); endtry '
                                      'return {p["recorded"], p["sampling_valid"]};', timeout=120)
        finally:
            stop_export_rpc_wall_ns = time.monotonic_ns() - stop_request_ns
            write_json(run_dir / f"{prefix}-control-window.json", {
                "start_rpc_wall_ns": start_rpc_wall_ns,
                "stop_export_rpc_wall_ns": stop_export_rpc_wall_ns,
                "scope": "local control round trips include server queueing, MOO wrappers, serialization and FileIO; not verb CPU"})
    states.append(process_state(server.proc.pid))
    after = snapshot(control)
    raw = run_dir / "files" / f"{prefix}-cpu-profile.json"
    result = {"mode": mode, "requested_wall_seconds": seconds, "hz": hz,
              "start_rpc_wall_ns": start_rpc_wall_ns,
              "stop_export_rpc_wall_ns": stop_export_rpc_wall_ns,
              "start_acknowledgement": ack, "export_acknowledgement": exported,
              "before": before, "after": after, "process_samples": states,
              "before_start_control": before_start_control,
              "before_stop_control": before_stop_control,
              "domain_progress": {
                  "cycle_advanced": after["current_cycle"] > before["current_cycle"],
                  "electronics_time_advanced": after["electronics"]["time"] > before["electronics"]["time"],
                  "electronics_updating_clear_after": after["electronics"]["updating"] == 0},
              "raw_profile": str(raw), "profile_sha256": bench.sha256_file(raw),
              "scope": "all server main-thread CPU, including background and control tasks",
              "observer_enabled": True}
    payload = json.loads(raw.read_text())
    result["metadata_mode_requested"] = metadata_mode
    result["metadata_mode_captured"] = payload.get("metadata_mode", "detailed")
    result["context_attribution"] = payload.get("context_attribution", "full (legacy detailed capture)")
    result["progress_scope"] = "before start control through after stop/export control"
    write_json(run_dir / f"{prefix}-capture.json", result)
    profile = report.validate(payload)
    if result["metadata_mode_captured"] != metadata_mode:
        raise RuntimeError("exported metadata mode differs from requested capture")
    (run_dir / f"{prefix}-cpu-report.md").write_text(report.markdown(profile), encoding="utf-8")
    (run_dir / f"{prefix}-cpu.folded").write_text(report.folded(profile), encoding="utf-8")
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--db", type=Path, required=True)
    parser.add_argument("--db-sha256", required=True)
    parser.add_argument("--moo", type=Path, required=True)
    parser.add_argument("--moo-sha256", required=True)
    parser.add_argument("--source-parent", required=True)
    parser.add_argument("--source-commit", help="recorded build source commit, when available")
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--port", type=int, default=17777)
    parser.add_argument("--seconds", type=float, default=10)
    parser.add_argument("--hz", type=int, default=100)
    parser.add_argument("--metadata-mode", choices=["detailed", "definitions"], default="detailed")
    parser.add_argument("--warmup-seconds", type=float, default=0, help="uncaptured background warmup before sampling")
    parser.add_argument("--sound-db", type=Path, help="complete local SQLite catalog snapshot with no pending WAL")
    parser.add_argument("--sound-db-sha256", help="required SHA-256 when --sound-db is supplied")
    parser.add_argument("--bootstrap-only", action="store_true")
    args = parser.parse_args()
    if bool(args.sound_db) != bool(args.sound_db_sha256):
        parser.error("--sound-db and --sound-db-sha256 must be supplied together")
    if sys.platform != "linux" or args.port == 7777 or not 1 <= args.port <= 65535:
        parser.error("requires Linux and a nonproduction listen port")
    if not 0 < args.seconds <= 60 or not 10 <= args.hz <= 1000:
        parser.error("seconds must be in (0,60] and hz in [10,1000]")
    if not 0 <= args.warmup_seconds <= 120:
        parser.error("warmup-seconds must be in [0,120]")
    network = check_network_isolation()
    source, executable, run_dir = args.db.resolve(strict=True), args.moo.resolve(strict=True), args.out.resolve()
    if not source.is_file() or not executable.is_file():
        raise RuntimeError("database and executable must be regular files")
    run_dir.mkdir(parents=True, exist_ok=False)
    os.chmod(run_dir, 0o700)
    manifest = {"status": "initializing", "source_database": str(source),
                "source_database_sha256": bench.sha256_file(source),
                "executable": str(executable), "executable_sha256": bench.sha256_file(executable),
                "source_parent_commit": args.source_parent,
                "source_commit": args.source_commit,
                "source_provenance": "supplied source revisions; verified SHA-256 identifies executable",
                "network": network, "port": args.port, "repairs": [],
                "metadata_mode_requested": args.metadata_mode,
                "bootstrap_adaptation": "expected create_account connection error remains uncaught; second emergency eval completes account"}
    write_json(run_dir / "manifest-initial.json", manifest)
    if manifest["source_database_sha256"].lower() != args.db_sha256.lower():
        raise RuntimeError("source checkpoint hash mismatch")
    if manifest["executable_sha256"].lower() != args.moo_sha256.lower():
        raise RuntimeError("candidate executable hash mismatch")
    db = run_dir / "mongoose.db"
    shutil.copyfile(source, db)
    manifest["copied_database_initial_sha256"] = bench.sha256_file(db)
    if manifest["copied_database_initial_sha256"] != manifest["source_database_sha256"]:
        raise RuntimeError("disposable checkpoint copy differs")
    (run_dir / "files").mkdir()
    (run_dir / "executables").mkdir()
    adapt_bootstrap(executable, run_dir)
    server, control = OwnedServer(executable, db, run_dir, args.port), None
    try:
        if args.sound_db:
            manifest["sound_catalog"] = prepare_sound_catalog(args.sound_db, args.sound_db_sha256, run_dir)
        proxy, user, password = bench.bootstrap_control_account(run_dir, db, args.port)
        manifest["bootstrapped_database_sha256"] = bench.sha256_file(db)
        manifest["bootstrap_completed"] = True
        manifest["startup_seconds"] = server.start()
        manifest["pid"] = server.proc.pid
        control, login = bench.connect_control(server, proxy, user, password, "immediate", 0.25, "offline profiler")
        identity, _ = control.eval('return {player, player.wizard, $prod(), server_version()};')
        manifest["control_identity"] = identity
        if identity[1] != 1 or identity[2] != 0:
            raise RuntimeError("requires wizard control and $prod()==0")
        manifest["ready_snapshot"] = snapshot(control)
        write_json(run_dir / "ready.json", manifest)
        if args.warmup_seconds:
            warmup = {"requested_seconds": args.warmup_seconds,
                      "before": manifest["ready_snapshot"], "scope": "background work; sampling inactive"}
            started = time.monotonic()
            time.sleep(args.warmup_seconds)
            server.verify_owned()
            warmup.update(elapsed_wall_seconds=time.monotonic() - started, after=snapshot(control))
            manifest["warmup"] = warmup
            write_json(run_dir / "warmup.json", warmup)
        if not args.bootstrap_only:
            manifest["capture"] = capture(control, server, run_dir, args.seconds, args.hz,
                                          "natural-background", args.metadata_mode)
        manifest["status"] = "bootstrap-ready" if args.bootstrap_only else "complete"
        print(json.dumps({"status": manifest["status"], "run_dir": str(run_dir),
                          "wizard": identity[1], "prod": identity[2]}), flush=True)
    except BaseException as exc:
        manifest["status"] = "failed"
        manifest["failure_type"] = type(exc).__name__
        manifest["failure"] = str(exc)
        raise
    finally:
        if control:
            control.close()
        try:
            manifest["shutdown"] = server.stop()
        finally:
            manifest["source_database_final_sha256"] = bench.sha256_file(source)
            if "sound_catalog" in manifest:
                manifest["sound_catalog"]["source_final_sha256"] = bench.sha256_file(args.sound_db)
            write_json(run_dir / "manifest-final.json", manifest)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
