"""Bounded HTTP diagnostics for an already-owned Barn in its private namespace.

CPU requests run asynchronously for a fixed window. request_issued means only
that the controller began sending GET, never that the profiler became active.
Raw vars/profile bodies stay local. Successful transport and gzip integrity do
not establish pprof sample validity; validate profiles offline before analysis.
snapshot/finish_cpu raise DiagnosticError carrying safe capture metadata.
finish never replaces a primary gameplay exception and retains failed captures.
"""
from __future__ import annotations

import copy
import gzip
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import socket
import threading
import time

BODY_LIMITS = {"vars": 1 << 20, "heap": 64 << 20, "cpu": 32 << 20}
EXPANDED_LIMIT = 128 << 20
SNAPSHOT_TIMEOUT = 10
CPU_GRACE = 10
CPU_SCOPE = "sampled whole Barn process CPU; background, runtime, HTTP and observer work included; not exclusive action CPU"
HEAP_SCOPE = "sampled Go heap at latest completed GC; gc=0 adds no forced GC; alloc totals include process history"
VARS_SCOPE = "approximate process-lifetime counters/gauges; concurrent background and collector work included; phase durations overlap"


class DiagnosticError(RuntimeError):
    def __init__(self, evidence):
        self.evidence = evidence
        super().__init__(evidence.get("kind", "diagnostic") + " capture failed; inspect retained metadata/raw artifact")


class _Failure(RuntimeError):
    """Only fixed, payload-free reasons belong in capture metadata."""


class Capture:
    def __init__(self, owner, label, kind, path, timeout):
        self.owner = owner
        self.request_issued = threading.Event()
        self.done = threading.Event()
        self.cancel = threading.Event()
        self.socket = None
        self.thread = None
        self.lock = threading.Lock()
        self.deadline = time.monotonic() + timeout
        self.row = {"label": label, "kind": kind, "request_path": path,
                    "timeout_seconds": timeout, "scheduled_monotonic_ns": time.monotonic_ns(),
                    "status": "scheduled", "activation_acknowledged": False,
                    "request_issue_scope": "controller transmission attempt; not server activation acknowledgement",
                    "body_limit_bytes": BODY_LIMITS[kind], "artifact_final": False}


class Collector:
    def __init__(self, server, ownedoutdir):
        self.server = server
        if getattr(server, "engine", None) != "barn":
            raise ValueError("Go diagnostics require an owned Barn server")
        server.verify_owned()
        self.pid, self.birth = server.proc.pid, server.start_ticks
        address = server.identity.get("endpoints", {}).get("debug")
        match = re.fullmatch(r"127\.0\.0\.1:([0-9]+)", address or "")
        if not match or not 1 <= int(match[1]) <= 65535:
            raise ValueError("requires the owned server's private loopback debug endpoint")
        self.port = int(match[1])
        self.directory = Path(ownedoutdir).resolve()
        if not self.directory.is_relative_to(Path(server.run_dir).resolve()):
            raise ValueError("diagnostic output must belong to the owned run directory")
        self.directory.mkdir(exist_ok=False)
        self.handles = []
        self.active_cpu = None
        self.closed = False
        self.result = None
        self.manifest = {"schema_version": 1, "collector": "owned Barn HTTP diagnostics",
                         "pid": self.pid, "start_ticks": self.birth, "debug_endpoint": address,
                         "executable_sha256": server.identity.get("executable_sha256"),
                         "cpu_scope": CPU_SCOPE, "heap_scope": HEAP_SCOPE, "vars_scope": VARS_SCOPE,
                         "proxy_policy": "direct HTTPConnection; environment proxies unused",
                         "redirect_policy": "never followed", "raw_artifacts": "private local files",
                         "pprof_semantic_validation": "pending offline validation",
                         "retention_census": "reader/history/finalization census unavailable from these endpoints"}

    def _guard(self):
        self.server.verify_owned()
        if self.server.proc.pid != self.pid or self.server.start_ticks != self.birth:
            raise _Failure("owned process identity changed")

    def _launch(self, label, kind, path, timeout):
        if self.closed:
            raise RuntimeError("diagnostic collector is finished")
        if not isinstance(label, str) or not re.fullmatch(r"[A-Za-z0-9_-]{1,48}", label):
            raise ValueError("diagnostic labels must be bounded filename tokens")
        suffix = "json" if kind == "vars" else "pprof"
        artifact = self.directory / f"{label}-{kind}.{suffix}"
        handle = Capture(self, label, kind, path, timeout)
        handle.row["artifact"] = artifact.name
        handle.row["artifact_owned"] = False
        self.handles.append(handle)
        # Keep the exclusive descriptor through validation/hash; a replaced path
        # must never supply bytes that this collector claims to have captured.
        try:
            stream = artifact.open("x+b")
        except Exception as error:
            handle.row.update(status="failed", failure_type=type(error).__name__,
                              failure_reason="exclusive artifact reservation failed")
            handle.done.set()
            raise DiagnosticError(copy.deepcopy(handle.row)) from None
        handle.row["artifact_owned"] = True
        try:
            handle.thread = threading.Thread(target=self._capture,
                                             args=(handle, stream, artifact), daemon=True)
            handle.thread.start()
        except Exception as error:
            try:
                stream.close()
                handle.row["artifact_final"] = True
            except Exception as close_error:
                handle.row["close_failure_type"] = type(close_error).__name__
            handle.thread = None
            handle.row.update(status="failed", failure_type=type(error).__name__,
                              failure_reason="capture thread did not start")
            handle.done.set()
            raise DiagnosticError(copy.deepcopy(handle.row))
        return handle

    def _remaining(self, handle):
        remaining = handle.deadline - time.monotonic()
        if remaining <= 0 or handle.cancel.is_set():
            raise _Failure("capture deadline exceeded or cancelled")
        return remaining

    def _capture(self, handle, stream, artifact):
        connection = response = None
        row = handle.row
        try:
            self._guard()
            row["identity_before_verified"] = True
            # HTTPConnection does not consult proxies or follow redirects.
            connection = http.client.HTTPConnection("127.0.0.1", self.port,
                                                    timeout=self._remaining(handle))
            connection.connect()
            with handle.lock:
                handle.socket = connection.sock
            connection.sock.settimeout(self._remaining(handle))
            row["request_issued_monotonic_ns"] = time.monotonic_ns()
            row["request_issued_wall_ns"] = time.time_ns()
            handle.request_issued.set()
            connection.request("GET", row["request_path"])
            response = connection.getresponse()
            row["headers_received_monotonic_ns"] = time.monotonic_ns()
            row["http_status"] = response.status
            row["headers"] = {key: response.getheader(key) for key in
                              ("Content-Type", "Content-Length", "Content-Disposition", "Date")}
            written = 0
            while True:
                handle.socket.settimeout(self._remaining(handle))
                chunk = response.read(min(65536, row["body_limit_bytes"] - written + 1))
                if not chunk:
                    break
                allowed = min(len(chunk), row["body_limit_bytes"] - written)
                stream.write(chunk[:allowed])
                written += allowed
                if allowed != len(chunk):
                    raise _Failure("response body exceeds configured byte limit")
            stream.flush()
            stream.seek(0)
            row["body_completed_monotonic_ns"] = time.monotonic_ns()
            self._remaining(handle)
            if response.status != 200:
                raise _Failure("HTTP status is not 200; redirects and errors are not captures")
            content_type = (row["headers"]["Content-Type"] or "").split(";", 1)[0].strip().lower()
            expected_type = "application/json" if row["kind"] == "vars" else "application/octet-stream"
            if content_type != expected_type:
                raise _Failure("response content type does not match requested diagnostic")
            if row["kind"] == "vars":
                value = json.loads(stream.read())
                if not isinstance(value, dict):
                    raise _Failure("vars response is not a JSON object")
                if not all(isinstance(value.get(name), dict) for name in ("memstats", "barn.runtime_phases")):
                    raise _Failure("vars response lacks required Go/Barn metric objects")
                row["metric_availability"] = {name: name in value for name in
                    ("memstats", "barn.runtime_phases", "barn.gc_sweeps", "barn.commit_retries", "barn.tasks_live")}
            else:
                expanded = 0
                with gzip.GzipFile(fileobj=stream, mode="rb") as profile:
                    while chunk := profile.read(min(65536, EXPANDED_LIMIT - expanded + 1)):
                        expanded += len(chunk)
                        if expanded > EXPANDED_LIMIT:
                            raise _Failure("expanded profile exceeds configured byte limit")
                        self._remaining(handle)
                if expanded == 0:
                    raise _Failure("profile body is empty")
                row.update(gzip_integrity="validated", expanded_bytes=expanded,
                           pprof_semantic_validation="pending offline validation")
            row["status"] = "captured"
        except Exception as error:
            row.update(status="failed", failure_type=type(error).__name__)
            row.setdefault("identity_before_verified", False)
            if isinstance(error, _Failure):
                row["failure_reason"] = str(error)
        finally:
            try:
                self._guard()
                row["identity_after_verified"] = True
            except Exception as error:
                row.update(status="failed", identity_after_verified=False,
                           identity_failure_type=type(error).__name__)
            try:
                stream.flush()
                stream.seek(0)
                row["sha256"] = hashlib.file_digest(stream, "sha256").hexdigest()
                owned_stat, path_stat = os.fstat(stream.fileno()), artifact.stat()
                row["bytes"] = owned_stat.st_size
                if (owned_stat.st_dev, owned_stat.st_ino) != (path_stat.st_dev, path_stat.st_ino):
                    raise _Failure("owned artifact pathname was replaced")
                row["artifact_final"] = True
            except Exception as error:
                row.update(status="failed", artifact_failure_type=type(error).__name__, artifact_final=False)
                if isinstance(error, _Failure):
                    row["failure_reason"] = str(error)
            for resource in (stream, response, connection):
                if resource is not None:
                    try:
                        resource.close()
                    except Exception as error:
                        row.update(status="failed", close_failure_type=type(error).__name__)
            if handle.cancel.is_set():
                row.update(status="failed", failure_reason="capture deadline exceeded or cancelled")
            row["finished_monotonic_ns"] = time.monotonic_ns()
            handle.done.set()

    def _wait(self, handle):
        if not isinstance(handle, Capture) or handle.owner is not self:
            raise ValueError("capture handle belongs to another collector")
        if not handle.done.is_set() and handle.thread is not None:
            handle.thread.join(timeout=max(0, handle.deadline - time.monotonic()))
        if not handle.done.is_set():
            handle.cancel.set()
            with handle.lock:
                if handle.socket is not None:
                    try:
                        handle.socket.shutdown(socket.SHUT_RDWR)
                    except OSError:
                        pass
            if handle.thread is not None:
                handle.thread.join(timeout=1)
        row = copy.deepcopy(handle.row)
        if not handle.done.is_set():
            row.update(status="failed", failure_reason="capture worker did not stop after deadline",
                       artifact_final=False)
        if row["status"] != "captured":
            raise DiagnosticError(row)
        return row

    def snapshot(self, label):
        rows = {}
        for kind, path in (("vars", "/debug/vars"), ("heap", "/debug/pprof/heap?gc=0&debug=0")):
            try:
                rows[kind] = self._wait(self._launch(label, kind, path, SNAPSHOT_TIMEOUT))
            except DiagnosticError as error:
                rows[kind] = error.evidence
        if any(row["status"] != "captured" for row in rows.values()):
            raise DiagnosticError({"kind": "snapshot", "label": label, "captures": rows})
        return rows

    def start_cpu(self, label, seconds=10):
        if type(seconds) is not int or not 1 <= seconds <= 30:
            raise ValueError("CPU window must be an integer in 1..30 seconds")
        if self.active_cpu is not None:
            raise RuntimeError("another CPU capture is not yet finished")
        handle = self._launch(label, "cpu", f"/debug/pprof/profile?seconds={seconds}", seconds + CPU_GRACE)
        handle.row.update(requested_profile_seconds=seconds, cpu_scope=CPU_SCOPE)
        self.active_cpu = handle
        return handle

    def finish_cpu(self, handle):
        if not isinstance(handle, Capture) or handle.owner is not self or handle.row["kind"] != "cpu":
            raise ValueError("requires this collector's CPU handle")
        try:
            return self._wait(handle)
        finally:
            if handle.done.is_set() and self.active_cpu is handle:
                self.active_cpu = None

    def finish(self):
        if self.result is not None:
            return self.result
        self.closed = True
        captures = []
        for handle in self.handles:
            try:
                captures.append(self._wait(handle))
            except DiagnosticError as error:
                captures.append(error.evidence)
        result = copy.deepcopy(self.manifest)
        result["captures"] = captures
        result["status"] = ("empty" if not captures else
                            "captured" if all(row["status"] == "captured" for row in captures) else "failed")
        try:
            with (self.directory / "manifest.json").open("x", encoding="utf-8") as saved:
                json.dump(result, saved, indent=2)
                saved.write("\n")
        except Exception as error:
            result.update(status="failed", manifest_failure_type=type(error).__name__)
        self.result = result
        return result
