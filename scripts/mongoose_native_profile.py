"""Optional Linux perf capture for an already-owned disposable Mongoose.

begin(server, destination, options) creates destination exclusively. The caller
starts/stops the MOO profiler separately and excludes this diagnostic collector
from unprofiled CPU comparisons. finish() preserves errors and never raises.
Perf's --tid targets only the main thread; cpu-clock:u excludes kernel time.
Controls use acknowledged perf file descriptors, never MOO signals.
See Linux tools/perf/Documentation/perf-record.txt for control-fd semantics.
"""
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import platform
import select
import shutil
import signal
import subprocess
import sys
import tempfile
import time


class NativeProfileError(RuntimeError):
    def __init__(self, manifest):
        self.manifest = manifest
        self.evidence = manifest
        super().__init__(manifest.get("reason", "native profiling failed"))


def _sha(path):
    with Path(path).open("rb") as stream:
        digest = hashlib.file_digest(stream, "sha256")
    return digest.hexdigest()


def _json(path, value):
    with Path(path).open("x", encoding="utf-8") as stream:
        json.dump(value, stream, indent=2)
        stream.write("\n")


def _state(pid):
    stat = Path(f"/proc/{pid}/stat").read_text()
    fields = stat[stat.rindex(")") + 2:].split()
    return {"pid": pid, "parent_pid": int(fields[1]), "start_ticks": int(fields[19]),
            "executable": str(Path(f"/proc/{pid}/exe").resolve(strict=True))}


def _target(server):
    if server.proc is None or server.proc.poll() is not None:
        raise RuntimeError("owned Mongoose is not running")
    server.verify_owned()
    state = _state(server.proc.pid)
    if (state["start_ticks"] != server.start_ticks or
            state["executable"] != str(Path(server.executable).resolve(strict=True))):
        raise RuntimeError("owned Mongoose identity changed before native capture")
    if not Path(f'/proc/{state["pid"]}/task/{state["pid"]}').is_dir():
        raise RuntimeError("owned Mongoose main thread is unavailable")
    state["sha256"] = _sha(f'/proc/{state["pid"]}/exe')
    state["main_tid"] = state["pid"]
    return state


def _command(arguments, directory, name, *, timeout=15):
    """Save exact argv, exit and raw streams, including command failures."""
    row = {"argv": arguments, "started_monotonic_ns": time.monotonic_ns()}
    with (directory / (name + ".stdout")).open("xb") as stdout, \
            (directory / (name + ".stderr")).open("xb") as stderr:
        try:
            result = subprocess.run(arguments, stdout=stdout, stderr=stderr,
                                    timeout=timeout, check=False, env=_environment())
            row["exit_code"] = result.returncode
        except (OSError, subprocess.TimeoutExpired) as error:
            row.update(exit_code=None, error=str(error))
    row["finished_monotonic_ns"] = time.monotonic_ns()
    _json(directory / (name + ".command.json"), row)
    return row


def _environment():
    environment = os.environ.copy()
    # Local symbols only: report generation must not make network requests.
    environment["DEBUGINFOD_URLS"] = ""
    environment["LC_ALL"] = "C"
    return environment


def _record_arguments(perf, output, hz, callgraph="dwarf"):
    arguments = [str(perf), "record", "--event=cpu-clock:u", f"--freq={hz}",
                 "--strict-freq", "--no-inherit", "--no-buildid-cache",
                 "--mmap-pages=128", "--output=" + str(output)]
    if callgraph == "dwarf":
        arguments.append("--call-graph=dwarf,8192")
    elif callgraph != "none":
        raise ValueError("native callgraph must be dwarf or none")
    return arguments


def _copy(source, destination):
    with Path(source).open("rb") as origin, Path(destination).open("xb") as saved:
        shutil.copyfileobj(origin, saved)


def _samples(path):
    # Event-header rows are emitted once per sample; indented callchain rows
    # have no event field. Keep the original stream for independent inspection.
    text = Path(path).read_text(encoding="utf-8", errors="replace")
    return sum("cpu-clock:u:" in line and "LOST" not in line
               for line in text.splitlines() if not line.lstrip().startswith("#"))


def capability(directory, hz=99):
    """Probe benign owned Python CPU work, never a Mongoose/application PID."""
    result = {"platform": platform.system(), "kernel": platform.release(),
              "event": "cpu-clock:u", "frequency_hz": hz, "available": False,
              "probe_scope": "perf record over owned benign Python CPU work only; no Mongoose attachment",
              "callgrind_available": shutil.which("valgrind") is not None,
              "callgrind_fallback": False}
    if platform.system() != "Linux":
        result["reason"] = "native perf capture requires Linux"
        return result
    if not hasattr(os, "pidfd_open") or not hasattr(signal, "pidfd_send_signal"):
        result["reason"] = "owned collector shutdown requires Linux pidfd support"
        return result
    perf = shutil.which("perf")
    if not perf:
        result["reason"] = "perf executable unavailable"
        return result
    perf = Path(perf).resolve(strict=True)
    with perf.open("rb") as stream:
        if stream.read(4) != b"\x7fELF":
            result["reason"] = "perf launcher is not a directly identity-checkable ELF executable"
            return result
    result.update(perf=str(perf), perf_sha256=_sha(perf), effective_uid=os.geteuid())
    # WSL perf writing directly to /mnt/c failed EFAULT in the saved harmless
    # probe; identical argv on Linux /tmp succeeded. Retain local raw files and
    # copy bytes exclusively into the durable artifact directory after stop.
    workspace = Path(tempfile.mkdtemp(prefix="barn-native-perf-", dir="/tmp"))
    result.update(workspace=str(workspace), output_filesystem="Linux-local staging; durable exclusive byte copies",
                  callgraph="dwarf", user_stack_dump_bytes=8192)
    for setting in ("perf_event_paranoid", "perf_event_max_sample_rate"):
        source = Path("/proc/sys/kernel") / setting
        result[setting] = source.read_text().strip() if source.exists() else None
    version = _command([str(perf), "version"], directory, "perf-version")
    work = "import time\nend=time.thread_time()+0.15\nx=0\nwhile time.thread_time()<end: x+=1\n"
    probe = _command(_record_arguments(perf, workspace / "probe.data", hz) +
                     ["--", sys.executable, "-c", work], directory, "perf-permission-probe")
    result["version_command"] = version
    result["permission_probe"] = probe
    if version.get("exit_code") != 0 or probe.get("exit_code") != 0:
        result["reason"] = "perf record capability/permission probe failed; see raw stderr"
        return result
    if not (workspace / "probe.data").is_file():
        result["reason"] = "successful perf probe produced no data file"
        return result
    _copy(workspace / "probe.data", directory / "probe.data")
    result["probe_data_sha256"] = _sha(directory / "probe.data")
    decoded = _command([str(perf), "script", "--input=" + str(workspace / "probe.data")],
                       directory, "perf-probe-script")
    report = _command([str(perf), "report", "--stdio", "--no-children", "--sort=symbol",
                       "--percent-limit=0", "--input=" + str(workspace / "probe.data")], directory, "perf-probe-report")
    result.update(script_probe=decoded, report_probe=report, sample_count=_samples(directory / "perf-probe-script.stdout"))
    if decoded.get("exit_code") != 0 or report.get("exit_code") != 0 or result["sample_count"] == 0:
        result["reason"] = "perf probe failed decoding/reporting or retained zero CPU samples"
        return result
    warning = (directory / "perf-probe-report.stderr").read_text(encoding="utf-8", errors="replace")
    if "Please install libunwind or libdw" in warning:
        result.update(callgraph="none", user_stack_dump_bytes=0,
                      callgraph_unavailable_reason="perf build lacks libunwind/libdw; flat instruction-pointer symbols only",
                      dwarf_probe={"permission": probe, "script": decoded, "report": report,
                                   "sample_count": result["sample_count"]})
        flat = workspace / "probe-flat.data"
        probe = _command(_record_arguments(perf, flat, hz, "none") +
                         ["--", sys.executable, "-c", work], directory, "perf-flat-permission-probe")
        result["permission_probe"] = probe
        if probe.get("exit_code") != 0 or not flat.is_file():
            result["reason"] = "flat perf fallback capability probe failed; see raw stderr"
            return result
        _copy(flat, directory / "probe-flat.data")
        decoded = _command([str(perf), "script", "--input=" + str(flat)], directory, "perf-flat-probe-script")
        report = _command([str(perf), "report", "--stdio", "--no-children", "--sort=symbol",
                           "--percent-limit=0", "--input=" + str(flat)], directory, "perf-flat-probe-report")
        result.update(script_probe=decoded, report_probe=report,
                      sample_count=_samples(directory / "perf-flat-probe-script.stdout"),
                      probe_data_sha256=_sha(flat))
        if decoded.get("exit_code") != 0 or report.get("exit_code") != 0 or result["sample_count"] == 0:
            result["reason"] = "flat perf fallback failed decoding/reporting or retained zero CPU samples"
            return result
    result["available"] = True
    return result


class Collector:
    def __init__(self, server, directory, mode, hz):
        self.server, self.directory, self.mode, self.hz = server, directory, mode, hz
        self.process = None
        self.pidfd = None
        self.control_fd = self.ack_fd = None
        self.log_stdout = self.log_stderr = None
        self.collector_identity = None
        self.result = None
        self.manifest = {"schema_version": 1, "requested_mode": mode, "status": "starting",
                         "frequency_hz": hz, "collector": "Linux perf", "event": "cpu-clock:u",
                         "scope": "owned main-thread userspace CPU samples; kernel and worker threads excluded",
                         "interpretation": "native symbols include sampler/observer work; samples are estimates, not per-verb durations",
                         "instrumented_diagnostic": mode != "off", "commands": []}

    @property
    def active(self):
        return self.process is not None and self.process.poll() is None

    def _control(self, command):
        if not self.active:
            raise RuntimeError("owned perf collector exited before control acknowledgement")
        os.write(self.control_fd, command.encode("ascii") + b"\n")
        # Linux evlist__ctlfd_ack writes sizeof("ack\n"), including its C NUL.
        # Consume the entire frame even when the final NUL arrives separately.
        expected = b"ack\n\x00"
        deadline, received = time.monotonic() + 5, b""
        while received != expected:
            if not self.active:
                raise RuntimeError("owned perf collector exited before control acknowledgement")
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("perf control acknowledgement timed out: " + command)
            ready, _, _ = select.select([self.ack_fd], [], [], remaining)
            if not ready:
                raise TimeoutError("perf control acknowledgement timed out: " + command)
            chunk = os.read(self.ack_fd, len(expected) - len(received) + 1)
            if not chunk:
                raise RuntimeError("perf control acknowledgement pipe closed")
            received += chunk
            if not expected.startswith(received):
                raise RuntimeError("unexpected perf control acknowledgement: " + repr(received))
        ready, _, _ = select.select([self.ack_fd], [], [], 0)
        if ready:
            extra = os.read(self.ack_fd, 1)
            raise RuntimeError("unexpected data after perf control acknowledgement: " + repr(extra))
        if not self.active:
            raise RuntimeError("owned perf collector exited before control acknowledgement")
        self.manifest.setdefault("controls", []).append({"command": command, "ack": "ack",
                                                          "ack_hex": received.hex(),
                                                          "monotonic_ns": time.monotonic_ns()})

    def _verify_collector(self):
        if not self.active:
            return
        state = _state(self.process.pid)
        if self.collector_identity is None:
            raise RuntimeError("collector identity was never established")
        if state != self.collector_identity or state["parent_pid"] != os.getpid():
            raise RuntimeError("owned perf process identity changed; refusing to signal")

    def _stop(self):
        """Only this owned perf child is signalled; the MOO is never signalled."""
        if self.process is None:
            return
        if self.active:
            self._verify_collector()
            signal.pidfd_send_signal(self.pidfd, signal.SIGINT)
            self.manifest["collector_stop_signal"] = int(signal.SIGINT)
            try:
                self.process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                self._verify_collector()
                signal.pidfd_send_signal(self.pidfd, signal.SIGKILL)
                self.process.wait(timeout=5)
                self.manifest["collector_forced_shutdown"] = True
        self.manifest["record_exit_code"] = self.process.returncode

    def _close(self):
        for attribute in ("pidfd", "control_fd", "ack_fd"):
            value = getattr(self, attribute)
            if value is not None:
                os.close(value)
                setattr(self, attribute, None)
        for stream in (self.log_stdout, self.log_stderr):
            if stream is not None:
                stream.close()

    def _terminal(self, status, reason=None):
        self.manifest["status"] = status
        if reason is not None:
            self.manifest["reason"] = str(reason)
        self.manifest["finished_monotonic_ns"] = time.monotonic_ns()
        _json(self.directory / "native.json", self.manifest)
        self.result = self.manifest
        return self.result

    def finish(self):
        if self.result is not None:
            return self.result
        errors = []
        try:
            try:
                self._control("disable")
            except Exception as error:
                errors.append("disable: " + str(error))
            self.manifest["capture_end_monotonic_ns"] = time.monotonic_ns()
            self._stop()
            try:
                if _target(self.server) != self.manifest["target"]:
                    raise RuntimeError("target identity/hash changed during native capture")
            except Exception as error:
                errors.append("target: " + str(error))
        except Exception as error:
            errors.append("stop: " + str(error))
        finally:
            try:
                self._close()
            except Exception as error:
                errors.append("close: " + str(error))
        try:
            data = Path(self.manifest["capability"]["workspace"]) / "perf.data"
            if data.is_file():
                _copy(data, self.directory / "perf.data")
                self.manifest.update(data_sha256=_sha(data), data_bytes=data.stat().st_size)
                perf = self.manifest["capability"]["perf"]
                if _sha(perf) != self.manifest["capability"]["perf_sha256"]:
                    raise RuntimeError("perf executable changed before native report")
                reports = [
                    ("perf-script", [perf, "script", "--input=" + str(data)]),
                    ("perf-report-flat", [perf, "report", "--stdio", "--no-children", "--sort=symbol", "--percent-limit=0", "--input=" + str(data)])]
                if self.manifest["capability"].get("callgraph") == "dwarf":
                    reports.append(("perf-report-inclusive", [perf, "report", "--stdio", "--children", "--sort=symbol", "--percent-limit=0", "--input=" + str(data)]))
                for name, arguments in reports:
                    row = _command(arguments, self.directory, name, timeout=30)
                    self.manifest["commands"].append(row)
                    if row.get("exit_code") != 0:
                        errors.append(name + " failed; see raw output")
                self.manifest["sample_count"] = _samples(self.directory / "perf-script.stdout")
                self.manifest["sample_count_method"] = "perf script cpu-clock:u event-header rows; callchain rows excluded"
                empty = self.manifest["sample_count"] == 0
            else:
                empty = True
                errors.append("perf data file missing")
            # perf record__sig_exit deliberately re-raises its received signal.
            # Preserve raw -SIGINT, accepting it only after our own pidfd stop.
            requested_interrupt = (self.manifest.get("record_exit_code") == -signal.SIGINT and
                                   self.manifest.get("collector_stop_signal") == int(signal.SIGINT))
            if ((self.manifest.get("record_exit_code") != 0 and not requested_interrupt) or
                    self.manifest.get("collector_forced_shutdown")):
                errors.append("perf record did not shut down successfully")
            return self._terminal("failed" if errors else ("empty" if empty else "collected"),
                                  "; ".join(errors) if errors else None)
        except Exception as error:
            # Preserve native failures without replacing a gameplay exception.
            try:
                return self._terminal("failed", "; ".join(errors + [str(error)]))
            except Exception as artifact_error:
                self.manifest.update(status="failed", reason=str(error), artifact_error=str(artifact_error))
                self.result = self.manifest
                return self.result


def begin(server, run_dir, options):
    mode, hz = getattr(options, "native_mode", "off"), getattr(options, "native_hz", 99)
    if mode not in ("off", "auto", "required"):
        raise ValueError("native_mode must be off, auto or required")
    if mode != "off" and (type(hz) is not int or not 10 <= hz <= 1000):
        raise ValueError("native_hz must be an integer in [10,1000]")
    directory = Path(run_dir).absolute()
    directory.mkdir(exist_ok=False)
    collector = Collector(server, directory, mode, hz)
    if mode == "off":
        collector._terminal("disabled")
        return collector
    try:
        try:
            capabilities = capability(directory, hz)
        except Exception as error:
            capabilities = {"available": False, "reason": "native capability probe failed: " + str(error)}
        collector.manifest["capability"] = capabilities
        if not capabilities["available"]:
            collector._terminal("unavailable", capabilities["reason"])
            if mode == "required":
                raise NativeProfileError(collector.manifest)
            return collector
        collector.manifest["target"] = _target(server)
        control_read, control_write = os.pipe()
        ack_read, ack_write = os.pipe()
        collector.control_fd, collector.ack_fd = control_write, ack_read
        collector.manifest["callgraph"] = capabilities.get("callgraph", "none")
        command = _record_arguments(capabilities["perf"], Path(capabilities["workspace"]) / "perf.data", hz,
                                    collector.manifest["callgraph"]) + [
            "--tid=" + str(server.proc.pid), "--delay=-1", f"--control=fd:{control_read},{ack_write}"]
        collector.manifest["record_command"] = command
        collector.log_stdout = (directory / "perf-record.stdout").open("xb")
        collector.log_stderr = (directory / "perf-record.stderr").open("xb")
        try:
            collector.process = subprocess.Popen(command, stdout=collector.log_stdout,
                                                stderr=collector.log_stderr, pass_fds=(control_read, ack_write),
                                                env=_environment())
        finally:
            os.close(control_read); os.close(ack_write)
        collector.pidfd = os.pidfd_open(collector.process.pid)
        collector._control("enable")
        collector.collector_identity = _state(collector.process.pid)
        if (collector.collector_identity["executable"] != capabilities["perf"] or
                collector.collector_identity["parent_pid"] != os.getpid()):
            raise RuntimeError("launched perf identity differs from verified executable")
        collector.manifest["collector_identity"] = collector.collector_identity
        if _target(server) != collector.manifest["target"]:
            raise RuntimeError("owned Mongoose identity changed while starting perf")
        collector.manifest.update(status="running", capture_begin_monotonic_ns=time.monotonic_ns())
        _json(directory / "native-start.json", collector.manifest)
        return collector
    except NativeProfileError:
        raise
    except Exception as error:
        cleanup_error = None
        try:
            if collector.process is not None and collector.active:
                # A new, unreaped Popen child cannot have its PID reused. If
                # startup failed before pidfd acquisition, stop only that child.
                if collector.pidfd is None:
                    collector.process.terminate()
                    collector.process.wait(timeout=5)
                else:
                    if collector.collector_identity is None:
                        collector.collector_identity = _state(collector.process.pid)
                    collector._stop()
        except Exception as cleanup:
            cleanup_error = str(cleanup)
        finally:
            collector._close()
        if cleanup_error:
            collector.manifest["cleanup_error"] = cleanup_error
        collector._terminal("failed", error)
        if mode == "required":
            raise NativeProfileError(collector.manifest) from error
        return collector
