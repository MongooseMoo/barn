"""Owned Linux Barn lifecycle for the unchanged real Mongoose scenarios."""
from __future__ import annotations

import json
import hashlib
import os
from pathlib import Path
import re
import signal
import socket
import stat
import subprocess
import time
import urllib.error
import urllib.request

import bench_players as bench

ROOT = Path(__file__).resolve().parents[1]
CONFIG = ROOT / "profiles" / "barn" / "mongoose-outbound-on.conf"
REGISTRY = ROOT / "profiles" / "barn" / "profiles.json"
PROFILE_ID = "barn-linux-mongoose-outbound-on"
CPU_SCOPE = "whole owned process user+system CPU across all OS threads; background and observer work included; clock-tick resolution"


def file_identity(path):
    try:
        row = path.lstat()
    except FileNotFoundError:
        return None
    return {"device": row.st_dev, "inode": row.st_ino, "mode": row.st_mode,
            "size_bytes": row.st_size, "mtime_ns": row.st_mtime_ns, "ctime_ns": row.st_ctime_ns}


def stable_file_digest(path):
    """Reject symlinks, replacement, and concurrent changes while hashing evidence."""
    before = file_identity(path)
    if before is None or not stat.S_ISREG(before["mode"]):
        raise RuntimeError("checkpoint output is missing or not a regular file")
    with os.fdopen(os.open(path, os.O_RDONLY | os.O_NOFOLLOW), "rb") as stream:
        opened = os.fstat(stream.fileno())
        if (opened.st_dev, opened.st_ino) != (before["device"], before["inode"]):
            raise RuntimeError("checkpoint output changed before reading")
        header = stream.readline(256)
        stream.seek(0)
        digest = hashlib.sha256()
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
        after = file_identity(path)
        if before != after:
            raise RuntimeError("checkpoint output changed while reading")
    return dict(before, path=str(path), sha256=digest.hexdigest()), header


def log_messages(data):
    messages = []
    for line in data.decode("utf-8", errors="strict").splitlines():
        try:
            row = json.loads(line)
        except json.JSONDecodeError:
            match = re.search(r'(?:^|\s)msg=("(?:[^"\\]|\\.)*"|\S+)', line)
            message = json.loads(match.group(1)) if match and match.group(1).startswith('"') else (match.group(1) if match else None)
        else:
            message = row.get("msg") if isinstance(row, dict) else None
        if isinstance(message, str):
            messages.append(message)
    return messages


def process_state(pid, *, proc_root=Path("/proc"), clock_ticks=None):
    directory = proc_root / str(pid)
    stat = (directory / "stat").read_text()
    fields = stat[stat.rfind(")") + 2:].split()
    ticks = os.sysconf("SC_CLK_TCK") if clock_ticks is None else clock_ticks
    if type(ticks) is not int or ticks <= 0:
        raise RuntimeError("invalid process CPU clock frequency")
    user, system, started = int(fields[11]), int(fields[12]), int(fields[19])
    if min(user, system, started) < 0:
        raise RuntimeError("invalid owned process CPU/identity counters")
    status = (directory / "status").read_text()
    rss = re.search(r"^VmRSS:\s+(\d+) kB$", status, re.M)
    high = re.search(r"^VmHWM:\s+(\d+) kB$", status, re.M)
    return {"monotonic_ns": time.monotonic_ns(), "start_ticks": started,
            "process_user_cpu_ticks": user, "process_system_cpu_ticks": system,
            "cpu_clock_ticks_per_second": ticks,
            "process_total_cpu_ns": (user + system) * 1_000_000_000 // ticks,
            "cpu_scope": CPU_SCOPE,
            "rss_kib": int(rss.group(1)) if rss else None,
            "high_water_rss_kib": int(high.group(1)) if high else None}


def endpoint_addresses(text):
    addresses = {}
    for line in text.splitlines():
        try:
            row = json.loads(line)
        except json.JSONDecodeError:
            row = None
        for name in ("debug", "operator"):
            message = name + " endpoint listening"
            address = None
            if isinstance(row, dict) and row.get("msg") == message:
                address = row.get("addr")
            elif message in line:
                match = re.search(r'\baddr=(?:"([^"]+)"|(\S+))', line)
                if match:
                    address = match.group(1) or match.group(2)
            if address is not None:
                if not isinstance(address, str) or not re.fullmatch(r"127\.0\.0\.1:[0-9]+", address):
                    raise RuntimeError("Barn operator/debug endpoint is not private loopback")
                if not 1 <= int(address.rsplit(":", 1)[1]) <= 65535:
                    raise RuntimeError("invalid Barn operator/debug port")
                addresses[name] = address
    return addresses


class OwnedBarnServer:
    engine = "barn"
    host = "127.0.0.1"

    def __init__(self, executable, db, run_dir, port, *, config=CONFIG, registry=REGISTRY):
        self.executable = executable.resolve(strict=True)
        self.db, self.run_dir, self.port = db.resolve(), run_dir.resolve(), port
        self.config, self.registry = config.resolve(strict=True), registry.resolve(strict=True)
        entries = json.loads(self.registry.read_text())["profiles"]
        entry = next((item for item in entries if item["profile_id"] == PROFILE_ID), None)
        if (entry is None or entry.get("runtime_os") != "linux" or
                entry.get("config_file") != self.config.name or entry.get("support_status") != "supported"):
            raise RuntimeError("registered Linux Mongoose Barn profile is unavailable")
        self.proc = self.log_file = self.pidfd = self.start_ticks = None
        self._ownership_established = False
        self._checkpoint_launch_verified = False
        self._log_identity = None
        self.checkpoint_path = Path(str(self.db) + ".new")
        self.sidecar_path = Path(str(self.checkpoint_path) + ".waifids")
        self.journal_path = Path(str(self.checkpoint_path) + ".publication")
        self.profile_path = self.run_dir / "barn-profile.json"
        self.log_path = self.run_dir / "server.log"
        log_level = "debug" if os.environ.get("BARN_RUNTIME_DIAGNOSTICS") == "1" else "info"
        self.command = [str(self.executable), "--db", str(self.db),
                        "--listen", f"tcp://{self.host}:{port}", "--checkpoint-interval", "0",
                        "--config", str(self.config), "--profile-id", PROFILE_ID,
                        "--profile-manifest", str(self.profile_path),
                        "--log-dir", str(self.run_dir / "logs"), "--log-level", log_level,
                        "--debug-addr", "127.0.0.1:0", "--operator-addr", "127.0.0.1:0"]
        self.identity = {"engine": "barn", "executable": str(self.executable),
                         "executable_sha256": bench.sha256_file(self.executable),
                         "config": str(self.config), "config_sha256": bench.sha256_file(self.config),
                         "profile_registry": str(self.registry), "profile_registry_sha256": bench.sha256_file(self.registry),
                         "profile_id": PROFILE_ID, "command": self.command,
                         "profile_manifest": str(self.profile_path), "server_log": str(self.log_path),
                         "runtime_logs": str(self.run_dir / "logs"),
                         "runtime_log_level": log_level,
                         "cpu_scope": CPU_SCOPE, "collectors": "off",
                         "runtime_environment": {key: os.environ.get(key) for key in
                             ("GOGC", "GOMEMLIMIT", "GOMAXPROCS", "BARN_RUNTIME_DIAGNOSTICS")}}

    def verify_owned(self):
        if self.proc is None or self.pidfd is None or self.start_ticks is None:
            raise RuntimeError("Barn process ownership was not established")
        directory = Path("/proc") / str(self.proc.pid)
        argv = (directory / "cmdline").read_bytes().split(b"\0")
        if argv and argv[-1] == b"":
            argv.pop()
        if (os.path.realpath(directory / "exe") != str(self.executable) or
                os.path.realpath(directory / "cwd") != str(self.run_dir) or
                argv != [os.fsencode(value) for value in self.command] or
                process_state(self.proc.pid)["start_ticks"] != self.start_ticks):
            raise RuntimeError("owned Barn identity changed; refusing to signal")

    def _validate_profile(self):
        row = json.loads(self.profile_path.read_text())
        if (row.get("profile_id") != PROFILE_ID or row.get("implementation") != "barn" or
                row.get("runtime_os") != "linux" or
                row.get("database_checksum") != self.identity["prepared_database_sha256"] or
                row.get("config_checksum") != self.identity["config_sha256"] or
                row.get("features", {}).get("option.OUTBOUND_NETWORK") is not True or
                row.get("features", {}).get("option.PROMOTE_NUMBERS") is not True):
            raise RuntimeError("Barn runtime profile does not match the prepared input/configuration")
        self.identity["profile_manifest_sha256"] = bench.sha256_file(self.profile_path)
        self.identity["runtime_profile"] = row

    def start(self):
        started = time.monotonic()
        if self.profile_path.exists():
            raise RuntimeError("refusing to overwrite a Barn runtime profile")
        if any(file_identity(path) is not None for path in
               (self.checkpoint_path, self.sidecar_path, self.journal_path)):
            raise RuntimeError("checkpoint output pair/journal must be absent before launch")
        self._checkpoint_launch_verified = True
        if bench.sha256_file(self.executable) != self.identity["executable_sha256"]:
            raise RuntimeError("Barn executable changed before launch")
        if bench.sha256_file(self.config) != self.identity["config_sha256"]:
            raise RuntimeError("Barn configuration changed before launch")
        self.identity["prepared_database_sha256"] = bench.sha256_file(self.db)
        with socket.socket() as reservation:
            reservation.bind((self.host, self.port))
        self.log_file = self.log_path.open("xb")
        opened = os.fstat(self.log_file.fileno())
        self._log_identity = (opened.st_dev, opened.st_ino)
        try:
            self.proc = subprocess.Popen(self.command, cwd=self.run_dir, stdin=subprocess.DEVNULL,
                                         stdout=self.log_file, stderr=subprocess.STDOUT)
            self.pidfd = os.pidfd_open(self.proc.pid)
            self.start_ticks = process_state(self.proc.pid)["start_ticks"]
        except Exception:
            # Only this still-unreaped Popen child can enter this path. Before
            # initial metadata exists, pidfd pins it if available; otherwise
            # Popen's own unreaped-child lifecycle prevents PID reuse.
            try:
                self._cleanup_unregistered_child()
            except Exception as cleanup_error:
                self.identity["startup_cleanup_failure"] = str(cleanup_error)
            raise
        self.verify_owned()
        self._ownership_established = True
        bench.wait_port(self.host, self.port, 180, self.proc)
        last_readiness = None
        while time.monotonic() - started < 180:
            if self.proc.poll() is not None:
                raise RuntimeError("Barn exited before operator readiness; inspect server.log")
            addresses = endpoint_addresses(self.log_path.read_text(errors="replace"))
            if set(addresses) == {"debug", "operator"}:
                try:
                    with urllib.request.urlopen("http://" + addresses["operator"] + "/readyz", timeout=1) as response:
                        body = response.read(4096).decode("utf-8")
                        last_readiness = {"status": response.status, "body": body}
                        if response.status == 200 and body.strip() == "ready":
                            self.verify_owned()
                            self._validate_profile()
                            self.identity.update(endpoints=addresses, readiness=last_readiness,
                                                 startup_seconds=time.monotonic() - started)
                            return self.identity["startup_seconds"]
                except (urllib.error.URLError, TimeoutError) as error:
                    last_readiness = {"failure": str(error)}
            time.sleep(0.1)
        self.identity["last_readiness"] = last_readiness
        raise TimeoutError("Barn operator readiness did not complete; inspect server.log")

    def _cleanup_unregistered_child(self):
        if self._ownership_established or self.start_ticks is not None:
            raise RuntimeError("startup cleanup cannot bypass established ownership")
        try:
            if self.proc and self.proc.poll() is None:
                if self.pidfd is not None:
                    signal.pidfd_send_signal(self.pidfd, signal.SIGTERM)
                else:
                    self.proc.terminate()
                try:
                    self.proc.wait(timeout=90)
                except subprocess.TimeoutExpired:
                    if self.pidfd is not None:
                        signal.pidfd_send_signal(self.pidfd, signal.SIGKILL)
                    else:
                        self.proc.kill()
                    self.proc.wait(timeout=10)
            self.identity["startup_cleanup"] = {"exit_code": self.proc.returncode if self.proc else None,
                                                 "scope": "only original unreaped Popen child before ownership registration"}
        finally:
            if self.pidfd is not None:
                os.close(self.pidfd)
                self.pidfd = None
            if self.log_file:
                self.log_file.close()

    def _checkpoint_boundary(self):
        boundary = {"outputs_before_signal": {}, "log_offset_bytes": None}
        try:
            boundary["outputs_before_signal"] = {str(path): file_identity(path) for path in
                                                  (self.checkpoint_path, self.sidecar_path)}
            row = file_identity(self.log_path)
            if (not self._checkpoint_launch_verified or row is None or not stat.S_ISREG(row["mode"]) or
                    (row["device"], row["inode"]) != self._log_identity):
                raise RuntimeError("fresh owned shutdown log provenance unavailable")
            boundary["log_offset_bytes"] = row["size_bytes"]
        except Exception as error:
            boundary["failure"] = str(error)
        return boundary

    def _final_checkpoint(self, boundary):
        verdict = {"status": "unknown", "verified": False,
                   "scope": "published database/waif identity pair integrity, not semantic round-trip validation",
                   "success_publication_log": "not emitted by Barn signal shutdown path",
                   "checkpoint": str(self.checkpoint_path), "sidecar": str(self.sidecar_path),
                   "boundary": boundary}
        def reject(status, reason):
            verdict.update(status=status, reason=reason, verified=status == "verified")
            return verdict
        try:
            if boundary.get("failure") or boundary.get("log_offset_bytes") is None:
                return reject("unknown", "owned shutdown boundary unavailable")
            log_row = file_identity(self.log_path)
            offset = boundary["log_offset_bytes"]
            if (log_row is None or not stat.S_ISREG(log_row["mode"]) or
                    (log_row["device"], log_row["inode"]) != self._log_identity or
                    not 0 <= log_row["size_bytes"] - offset <= 8 * 1024 * 1024):
                return reject("unknown", "owned shutdown log changed or exceeds bounded collection")
            with os.fdopen(os.open(self.log_path, os.O_RDONLY | os.O_NOFOLLOW), "rb") as stream:
                opened = os.fstat(stream.fileno())
                if (opened.st_dev, opened.st_ino) != self._log_identity:
                    return reject("unknown", "owned shutdown log replaced before reading")
                stream.seek(offset)
                data = stream.read(8 * 1024 * 1024 + 1)
            if file_identity(self.log_path) != log_row or len(data) != log_row["size_bytes"] - offset:
                return reject("unknown", "owned shutdown log changed while reading")
            messages = log_messages(data)
            verdict["shutdown_log_sha256"] = hashlib.sha256(data).hexdigest()
            for failure in ("final checkpoint failed", "final checkpoint skipped (checkpointing disabled)"):
                if failure in messages:
                    return reject("failed", failure)
            if ("SHUTDOWN: shutdown signal received" not in messages or "shutdown complete" not in messages or
                    messages.index("SHUTDOWN: shutdown signal received") >= messages.index("shutdown complete")):
                return reject("unknown", "owned signal shutdown completion markers missing or out of order")
            if file_identity(self.journal_path) is not None:
                return reject("failed", "checkpoint publication recovery journal remains")
            evidence = {}
            for name, path in (("database", self.checkpoint_path), ("waif_identities", self.sidecar_path)):
                row, header = stable_file_digest(path)
                old = boundary["outputs_before_signal"][str(path)]
                if old is not None and (row["device"], row["inode"]) == (old["device"], old["inode"]):
                    return reject("unknown", "output pair was not freshly atomically published after shutdown signal")
                if row["size_bytes"] == 0:
                    return reject("failed", "empty checkpoint output")
                evidence[name] = row
                if name == "waif_identities":
                    expected = ("barn-waif-identities-v1 " + evidence["database"]["sha256"] + "\n").encode("ascii")
                    if header != expected:
                        return reject("failed", "waif identity header does not match published database SHA256")
            verdict["outputs"] = evidence
            return reject("verified", "fresh owned signal shutdown published an intact database/identity pair")
        except Exception as error:
            return reject("unknown", "checkpoint verification unavailable: " + str(error))

    def stop(self):
        final_checkpoint = {"status": "unknown", "verified": False, "reason": "owned signal shutdown not observed"}
        try:
            if self.proc and self.proc.poll() is None:
                self.verify_owned()
                boundary = self._checkpoint_boundary()
                signal.pidfd_send_signal(self.pidfd, signal.SIGTERM)
                try:
                    self.proc.wait(timeout=90)
                except subprocess.TimeoutExpired:
                    self.verify_owned()
                    signal.pidfd_send_signal(self.pidfd, signal.SIGKILL)
                    self.proc.wait(timeout=10)
                    return {"exit_code": self.proc.returncode, "shutdown": "forced after 90s", "forced": True,
                            "final_checkpoint": final_checkpoint}
                if self.proc.returncode == 0:
                    final_checkpoint = self._final_checkpoint(boundary)
            return {"exit_code": self.proc.returncode if self.proc else None,
                    "shutdown": "owned Barn PID stopped", "forced": False, "final_checkpoint": final_checkpoint}
        finally:
            if self.pidfd is not None:
                os.close(self.pidfd)
                self.pidfd = None
            if self.log_file:
                self.log_file.close()
