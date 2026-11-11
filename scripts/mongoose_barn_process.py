"""Owned Linux Barn lifecycle for a registered runtime profile.

The unchanged real Mongoose scenarios use the default Mongoose profile; the
Jepsen checkpoint workload uses the registered public Test.db profile.
"""
from __future__ import annotations

import errno
import json
import hashlib
import os
from pathlib import Path
import re
import shutil
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


def log_records(data):
    """(message, line) for every text or JSON log line carrying a message."""
    records = []
    for line in data.decode("utf-8", errors="strict").splitlines():
        try:
            row = json.loads(line)
        except json.JSONDecodeError:
            match = re.search(r'(?:^|\s)msg=("(?:[^"\\]|\\.)*"|\S+)', line)
            message = json.loads(match.group(1)) if match and match.group(1).startswith('"') else (match.group(1) if match else None)
        else:
            message = row.get("msg") if isinstance(row, dict) else None
        if isinstance(message, str):
            records.append((message, line))
    return records


def log_messages(data):
    return [message for message, _ in log_records(data)]


def checkpoint_markers(data):
    """Checkpoint records Barn logged in data: `checkpoint started` (server/server.go:470),
    `checkpoint complete` with duration_ms (:493) and `checkpoint failed` (:396)."""
    markers = {"started": 0, "complete": [], "failed": []}
    for message, line in log_records(data):
        if message == "checkpoint started":
            markers["started"] += 1
        elif message == "checkpoint complete":
            match = re.search(r'"?duration_ms"?[=:]\s*(\d+)', line)
            markers["complete"].append(int(match.group(1)) if match else None)
        elif message == "checkpoint failed":
            markers["failed"].append(line)
    return markers


def _artifact_digest(path):
    """sha256 of a regular checkpoint artifact; None when it is absent."""
    row = file_identity(path)
    if row is None:
        return None
    if not stat.S_ISREG(row["mode"]):
        raise RuntimeError(f"checkpoint artifact is not a regular file: {path}")
    return bench.sha256_file(path)


def pair_digest(path):
    """{database, waif_identities} sha256 of a database and its sidecar; None without the database."""
    database = _artifact_digest(path)
    if database is None:
        return None
    return {"database": database, "waif_identities": _artifact_digest(Path(str(path) + ".waifids"))}


def journal_digest(row):
    """A publication journal digest as hex: Go encodes [32]byte as a JSON number list
    (checkpointDigest, db/format/checkpoint_publication.go:25-29)."""
    return {"database": bytes(row["Database"]).hex(),
            "waif_identities": bytes(row["Sidecar"]).hex() if row["HasSidecar"] else None}


# Frozen publication stages observable from files on the ordinary binary, in
# publication order (db/format/checkpoint_stage.go:53,135;
# checkpoint_publication.go:198,237,370-372,285-296). Capture, Serialize and the
# temp-file sync are indistinguishable from files and share "staging".
CHECKPOINT_STAGES = ("not-staged", "staging", "sidecar-prepared", "journal-published",
                     "database-renamed", "both-renamed", "cleanup")


def checkpoint_artifacts(db):
    """Classify the checkpoint files of a Barn launched with --db db.

    Call only while that process is stopped or dead, so the files are frozen.
    """
    db = Path(db)
    output, temp = Path(str(db) + ".new"), Path(str(db) + ".tmp")
    journal_path = Path(str(output) + ".publication")
    result = {"database": str(db), "output": pair_digest(output), "temp": pair_digest(temp),
              "journal": None, "recovery_directories": {}}
    if file_identity(journal_path) is not None:
        try:
            row = json.loads(journal_path.read_bytes())
            result["journal"] = {"generation": row["Generation"], "next": journal_digest(row["Next"]),
                                 "previous": None if row.get("Previous") is None else journal_digest(row["Previous"]),
                                 "sha256": bench.sha256_file(journal_path)}
        except (OSError, ValueError, KeyError, TypeError) as error:
            result["journal"] = {"error": f"{type(error).__name__}: {error}"}
    for directory in sorted(db.parent.glob(output.name + ".recovery-*")):
        result["recovery_directories"][directory.name] = {
            member: pair_digest(directory / member) for member in ("next", "previous")} | {
            "journal": _artifact_digest(directory / "journal")}
    temp_database = result["temp"] is not None
    temp_sidecar = file_identity(Path(str(temp) + ".waifids")) is not None
    if result["journal"] is not None:
        stage = "journal-published" if temp_database else "database-renamed" if temp_sidecar else "both-renamed"
    elif temp_database:
        stage = "sidecar-prepared" if temp_sidecar else "staging"
    elif temp_sidecar:
        stage = "unclassified"
    elif result["recovery_directories"]:
        stage = "cleanup"
    else:
        stage = "not-staged"
    result["stage"] = stage
    return result


def expected_recovery(artifacts):
    """The pair recovery must select from frozen artifacts (plan section 2 table;
    recoverCheckpointPair, db/format/checkpoint_publication.go:299-353).

    None means no checkpoint output exists, so the unchanged input is the restart pair.
    """
    journal = artifacts["journal"]
    if journal is not None:
        if "error" in journal:
            raise ValueError(f"unreadable publication journal: {journal['error']}")
        if artifacts["stage"] == "both-renamed":
            return journal["next"]
        return journal["previous"] or journal["next"]
    return artifacts["output"]


def preserve_artifacts(db, destination):
    """Copy (never move) the input pair and every checkpoint artifact of --db db into destination."""
    db = Path(db)
    destination.mkdir()
    sources = [db, Path(str(db) + ".waifids")]
    for stem in (Path(str(db) + ".new"), Path(str(db) + ".tmp")):
        sources += [stem, Path(str(stem) + ".waifids")]
    sources.append(Path(str(db) + ".new.publication"))
    sources += sorted(db.parent.glob(Path(str(db) + ".new").name + ".recovery-*"))
    copied = []
    for source in sources:
        row = file_identity(source)
        if row is None:
            continue
        target = destination / source.name
        if stat.S_ISDIR(row["mode"]):
            shutil.copytree(source, target, symlinks=True)
            members = sorted(path for path in target.rglob("*") if path.is_file())
        else:
            shutil.copyfile(source, target, follow_symlinks=False)
            members = [target]
        for member in members:
            original = source / member.relative_to(target) if member != target else source
            copied.append({"source": str(original), "copy": str(member), "sha256": bench.sha256_file(member)})
    return copied


class PairRejected(Exception):
    def __init__(self, status, reason):
        super().__init__(reason)
        self.status = status


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


def run_state(pid, *, proc_root=Path("/proc")):
    """The single-letter scheduler state of pid (e.g. 'T' once SIGSTOP has taken effect)."""
    stat = (proc_root / str(pid) / "stat").read_text()
    return stat[stat.rfind(")") + 2:].split()[0]


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

    def __init__(self, executable, db, run_dir, port, *, config=CONFIG, registry=REGISTRY, profile_id=PROFILE_ID,
                 recovered_input=None):
        """recovered_input declares that db is an interrupted checkpoint's output whose
        publication journal Barn's loader will recover (a dict naming its provenance).
        Without it, a pending journal on db refuses the launch."""
        self.recovered_input = recovered_input
        self.executable = executable.resolve(strict=True)
        self.db, self.run_dir, self.port = db.resolve(), run_dir.resolve(), port
        self.config, self.registry = config.resolve(strict=True), registry.resolve(strict=True)
        entries = json.loads(self.registry.read_text())["profiles"]
        entry = next((item for item in entries if item["profile_id"] == profile_id), None)
        if (entry is None or entry.get("runtime_os") != "linux" or
                entry.get("config_file") != self.config.name or entry.get("support_status") != "supported"):
            raise RuntimeError(f"registered Linux Barn profile {profile_id} is unavailable")
        self.profile_id = profile_id
        # The same rule as profile/registry.go Validate: every registered
        # expected feature must appear with that exact value in the manifest.
        self.expected_features = dict(entry.get("expected_features") or {})
        self.paused = False
        self.proc = self.log_file = self.pidfd = self.start_ticks = None
        self._ownership_established = False
        self._checkpoint_launch_verified = False
        self._log_identity = None
        self.checkpoint_path = Path(str(self.db) + ".new")
        self.sidecar_path = Path(str(self.checkpoint_path) + ".waifids")
        self.journal_path = Path(str(self.checkpoint_path) + ".publication")
        self.input_journal_path = Path(str(self.db) + ".publication")
        self.profile_path = self.run_dir / "barn-profile.json"
        self.log_path = self.run_dir / "server.log"
        log_level = "debug" if os.environ.get("BARN_RUNTIME_DIAGNOSTICS") == "1" else "info"
        self.command = [str(self.executable), "--db", str(self.db),
                        "--listen", f"tcp://{self.host}:{port}", "--checkpoint-interval", "0",
                        "--config", str(self.config), "--profile-id", profile_id,
                        "--profile-manifest", str(self.profile_path),
                        "--log-dir", str(self.run_dir / "logs"), "--log-level", log_level,
                        "--debug-addr", "127.0.0.1:0", "--operator-addr", "127.0.0.1:0"]
        self.identity = {"engine": "barn", "executable": str(self.executable),
                         "executable_sha256": bench.sha256_file(self.executable),
                         "config": str(self.config), "config_sha256": bench.sha256_file(self.config),
                         "profile_registry": str(self.registry), "profile_registry_sha256": bench.sha256_file(self.registry),
                         "profile_id": profile_id, "command": self.command,
                         "profile_manifest": str(self.profile_path), "server_log": str(self.log_path),
                         "runtime_logs": str(self.run_dir / "logs"),
                         "runtime_log_level": log_level, "recovered_input": recovered_input,
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
        features = row.get("features")
        if (row.get("profile_id") != self.profile_id or row.get("implementation") != "barn" or
                row.get("runtime_os") != "linux" or
                row.get("database_checksum") != self.identity["prepared_database_sha256"] or
                row.get("config_checksum") != self.identity["config_sha256"] or
                not isinstance(features, dict) or
                any(key not in features or type(features[key]) is not type(value) or features[key] != value
                    for key, value in self.expected_features.items())):
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
        if file_identity(self.db) is None:
            # Barn hashes --db for the manifest (internal/app/app.go:352) before
            # recovery could restore a first publication's database (app.go:388).
            raise RuntimeError("input database is absent; Barn cannot build its profile manifest before "
                               "recovery would create it")
        if self.recovered_input is None and file_identity(self.input_journal_path) is not None:
            raise RuntimeError("input database has a pending checkpoint publication journal; "
                               "only a declared recovered-input launch may recover it")
        self._checkpoint_launch_verified = True
        if bench.sha256_file(self.executable) != self.identity["executable_sha256"]:
            raise RuntimeError("Barn executable changed before launch")
        if bench.sha256_file(self.config) != self.identity["config_sha256"]:
            raise RuntimeError("Barn configuration changed before launch")
        # Barn hashes --db for its manifest (internal/app/app.go:352) before
        # LoadDatabase recovers a pending journal (app.go:388, reader.go:163),
        # so the manifest names this pre-launch file in both modes.
        self.identity["prepared_database_sha256"] = bench.sha256_file(self.db)
        self.identity["input_pair"] = pair_digest(self.db)
        self.identity["input_journal_sha256"] = _artifact_digest(self.input_journal_path)
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
        # Popen returns once the kernel releases the parent inside exec, before
        # it writes the new argv: /proc/<pid>/cmdline reads empty until then.
        # A listening Barn has finished exec, so its identity is checkable.
        bench.wait_port(self.host, self.port, 180, self.proc)
        self.verify_owned()
        self._ownership_established = True
        last_readiness = None
        while time.monotonic() - started < 180:
            if self.proc.poll() is not None:
                raise RuntimeError("Barn exited before operator readiness; inspect server.log")
            try:
                text = self.log_path.read_text(errors="replace")
            except OSError as error:
                if error.errno != errno.ENODATA:
                    raise
                stamp = time.monotonic_ns()
                observed = self.identity.setdefault("startup_log_read_errors", {
                    "errno": errno.ENODATA, "count": 0,
                    "first_monotonic_ns": stamp, "last_monotonic_ns": stamp})
                observed["count"] += 1
                observed["last_monotonic_ns"] = stamp
                self.verify_owned()
                last_readiness = {"failure_type": "startup_log_read", "errno": errno.ENODATA}
                time.sleep(0.1)
                continue
            addresses = endpoint_addresses(text)
            if set(addresses) == {"debug", "operator"}:
                try:
                    with urllib.request.urlopen("http://" + addresses["operator"] + "/readyz", timeout=1) as response:
                        body = response.read(4096).decode("utf-8")
                        last_readiness = {"status": response.status, "body": body}
                        if response.status == 200 and body.strip() == "ready":
                            self.verify_owned()
                            self._validate_profile()
                            if self.recovered_input is not None:
                                self.identity["recovered_pair"] = self._recovered_pair()
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

    def _log_since(self, offset, *, live=False):
        """The owned server.log bytes after offset, refusing replacement or unbounded growth.

        Without live, the process has exited and the log must not change while
        it is read. With live, Barn may still be appending: the log must stay
        the same file and never shrink, the read stops at the size seen before
        it, and only complete lines are returned.
        """
        log_row = file_identity(self.log_path)
        if (log_row is None or not stat.S_ISREG(log_row["mode"]) or
                (log_row["device"], log_row["inode"]) != self._log_identity or
                not 0 <= log_row["size_bytes"] - offset <= 8 * 1024 * 1024):
            raise PairRejected("unknown", "owned shutdown log changed or exceeds bounded collection")
        with os.fdopen(os.open(self.log_path, os.O_RDONLY | os.O_NOFOLLOW), "rb") as stream:
            opened = os.fstat(stream.fileno())
            if (opened.st_dev, opened.st_ino) != self._log_identity:
                raise PairRejected("unknown", "owned shutdown log replaced before reading")
            stream.seek(offset)
            data = stream.read(log_row["size_bytes"] - offset if live else 8 * 1024 * 1024 + 1)
        after = file_identity(self.log_path)
        if live:
            if (after is None or (after["device"], after["inode"]) != self._log_identity or
                    after["size_bytes"] < log_row["size_bytes"] or len(data) != log_row["size_bytes"] - offset):
                raise PairRejected("unknown", "owned live log was replaced or truncated while reading")
            return data[:data.rfind(b"\n") + 1]
        if after != log_row or len(data) != log_row["size_bytes"] - offset:
            raise PairRejected("unknown", "owned shutdown log changed while reading")
        return data

    def _published_pair(self, path, sidecar, outputs_before):
        """Digests of a freshly published database/sidecar pair whose header binds the database."""
        evidence = {}
        for name, member in (("database", path), ("waif_identities", sidecar)):
            row, header = stable_file_digest(member)
            old = outputs_before.get(str(member)) if outputs_before is not None else None
            if old is not None and (row["device"], row["inode"]) == (old["device"], old["inode"]):
                raise PairRejected("unknown", "output pair was not freshly atomically published after shutdown signal")
            if row["size_bytes"] == 0:
                raise PairRejected("failed", "empty checkpoint output")
            evidence[name] = row
            if name == "waif_identities":
                expected = ("barn-waif-identities-v1 " + evidence["database"]["sha256"] + "\n").encode("ascii")
                if header != expected:
                    raise PairRejected("failed", "waif identity header does not match published database SHA256")
        return evidence

    def _recovered_pair(self):
        """The pair Barn's loader selected for a recovered input. Barn never writes
        its --db path after loading, so this is stable once the server is ready."""
        if file_identity(self.input_journal_path) is not None:
            raise RuntimeError("recovered input still has its publication journal after startup")
        try:
            return self._published_pair(self.db, Path(str(self.db) + ".waifids"), None)
        except PairRejected as error:
            raise RuntimeError(f"recovered input pair is not intact: {error}") from error

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
            data = self._log_since(boundary["log_offset_bytes"])
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
            verdict["outputs"] = self._published_pair(self.checkpoint_path, self.sidecar_path,
                                                      boundary["outputs_before_signal"])
            return reject("verified", "fresh owned signal shutdown published an intact database/identity pair")
        except PairRejected as error:
            return reject(error.status, str(error))
        except Exception as error:
            return reject("unknown", "checkpoint verification unavailable: " + str(error))

    def request_checkpoint(self):
        """Ask the running owned Barn for one checkpoint with SIGALRM, which calls
        requestCheckpoint directly (server/signals_unix.go:41-42).

        The request channel holds one queued request (server/server.go:112) and
        drops a further one (:433-436), so callers wait for the previous
        request's completion or failure before asking again.
        """
        boundary = self._checkpoint_boundary()
        if boundary.get("failure"):
            raise RuntimeError(f"checkpoint request boundary unavailable: {boundary['failure']}")
        boundary["signal_ns"] = time.monotonic_ns()
        self._signal_running(signal.SIGALRM)
        boundary["signalled_ns"] = time.monotonic_ns()
        return boundary

    def checkpoint_log(self, boundary):
        """Checkpoint markers logged since a request boundary, while Barn may still be writing."""
        return checkpoint_markers(self._log_since(boundary["log_offset_bytes"], live=True))

    def wait_checkpoint(self, boundary, timeout):
        """Wait for the requested checkpoint to complete or fail; on completion
        return the published pair's stable digests (no later request exists to change it)."""
        deadline = time.monotonic() + timeout
        while True:
            markers = self.checkpoint_log(boundary)
            if markers["complete"] or markers["failed"]:
                break
            if self.proc.poll() is not None:
                raise RuntimeError(f"owned Barn exited (code {self.proc.returncode}) while a checkpoint was pending")
            if time.monotonic() >= deadline:
                raise TimeoutError(f"no checkpoint completion or failure logged within {timeout}s: {markers}")
            time.sleep(0.01)
        result = dict(markers, observed_ns=time.monotonic_ns())
        if markers["failed"]:
            return dict(result, status="failed")
        if file_identity(self.journal_path) is not None:
            raise RuntimeError("checkpoint publication journal remains after checkpoint complete")
        try:
            outputs = self._published_pair(self.checkpoint_path, self.sidecar_path, boundary["outputs_before_signal"])
        except PairRejected as error:
            raise RuntimeError(f"completed checkpoint pair is not intact: {error}") from error
        return dict(result, status="complete", duration_ms=markers["complete"][0], outputs=outputs)

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

    def _signal_running(self, number):
        if self.proc is None or self.proc.poll() is not None:
            raise RuntimeError("owned Barn process is not running")
        self.verify_owned()
        signal.pidfd_send_signal(self.pidfd, number)

    def pause(self, timeout=5.0):
        """Freeze the owned process (SIGSTOP); connections stay open.

        Returns the /proc state observed once the kernel has stopped it ('T').
        """
        self._signal_running(signal.SIGSTOP)
        self.paused = True
        deadline = time.monotonic() + timeout
        while True:
            state = run_state(self.proc.pid)
            if state == "T" or time.monotonic() >= deadline:
                break
            time.sleep(0.01)
        if state != "T":
            raise RuntimeError(f"owned Barn process was not observed stopped after SIGSTOP (state {state!r})")
        return state

    def resume(self):
        """Continue a process frozen by pause() (SIGCONT)."""
        self._signal_running(signal.SIGCONT)
        self.paused = False

    def crash(self):
        """Kill the owned process without a final checkpoint (SIGKILL)."""
        try:
            self._signal_running(signal.SIGKILL)
            self.proc.wait(timeout=10)
            self.paused = False
            return {"exit_code": self.proc.returncode, "shutdown": "owned SIGKILL crash", "forced": True,
                    "final_checkpoint": {"status": "none", "verified": False,
                                         "reason": "crash publishes no checkpoint"}}
        finally:
            if self.pidfd is not None:
                os.close(self.pidfd)
                self.pidfd = None
            if self.log_file:
                self.log_file.close()
