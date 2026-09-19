"""Paired live Mongoose measurements; uses disposable data and prompt-driven login.

Credentials come only from MONGOOSE_USERNAME/MONGOOSE_PASSWORD. See the report
for the measured workload and limits; this is not a conformance harness.
"""
import argparse
import contextlib
import ctypes
from collections import Counter
from datetime import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import socket
import statistics
import subprocess
import time
import urllib.request


def save(path, value):
    path.write_text(json.dumps(value, indent=2), encoding="utf-8")


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def cpu_seconds(proc):
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.GetProcessTimes.argtypes = [ctypes.c_void_p] + [ctypes.POINTER(ctypes.c_ulonglong)] * 4
    values = [ctypes.c_ulonglong() for _ in range(4)]
    if not kernel.GetProcessTimes(int(proc._handle), *(ctypes.byref(v) for v in values)):
        raise ctypes.WinError(ctypes.get_last_error())
    return (values[2].value + values[3].value) / 1e7


@contextlib.contextmanager
def paused_process(pid):
    if not pid:
        yield
        return
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.OpenProcess.restype = ctypes.c_void_p
    kernel.CloseHandle.argtypes = [ctypes.c_void_p]
    nt = ctypes.WinDLL("ntdll")
    nt.NtSuspendProcess.argtypes = nt.NtResumeProcess.argtypes = [ctypes.c_void_p]
    handle = kernel.OpenProcess(0x0800, False, pid)
    if not handle:
        raise ctypes.WinError(ctypes.get_last_error())
    suspended = False
    try:
        if nt.NtSuspendProcess(handle) != 0:
            raise RuntimeError("Cannot suspend comparison-interfering process")
        suspended = True
        print(f"Paused existing process {pid}", flush=True)
        yield
    finally:
        if suspended and nt.NtResumeProcess(handle) != 0:
            raise RuntimeError(f"RESUME FAILED for PID {pid}")
        kernel.CloseHandle(handle)
        print(f"Resumed existing process {pid}", flush=True)


class Client:
    def __init__(self, port, transcript):
        self.sock = socket.create_connection(("127.0.0.1", port), 10)
        self.sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
        self.sock.settimeout(0.25)
        self.buffer = ""
        self.transcript = transcript

    def send(self, line):
        self.sock.sendall((line + "\r\n").encode())

    def until(self, patterns, timeout=60):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            for pattern in patterns:
                found = re.search(pattern, self.buffer)
                if found:
                    consumed = self.buffer[:found.end()]
                    self.buffer = self.buffer[found.end():]
                    return pattern, consumed
            try:
                data = self.sock.recv(65536)
            except socket.timeout:
                continue
            if not data:
                raise RuntimeError("Connection closed before completion marker")
            text = data.decode("utf-8", errors="replace")
            self.transcript.write(text)
            self.transcript.flush()
            self.buffer += text
        raise TimeoutError("Expected completion marker was not received")

    def close(self):
        self.sock.close()


def vars_at(port):
    with urllib.request.urlopen(f"http://127.0.0.1:{port}/debug/vars", timeout=10) as r:
        return json.load(r)


def trial(args, label, binary, limit, repeat):
    run = args.output / f"{repeat:02}-{label}-{limit}"
    run.mkdir(parents=True, exist_ok=False)
    (run / "files/sqlite").mkdir(parents=True)
    (run / "executables").mkdir()
    shutil.copyfile(args.database, run / "mongoose.db.new")
    shutil.copyfile(args.sound, run / "files/sqlite/sound.sqlite")
    if args.support_dir:
        for path in args.support_dir.glob("*.sqlite"):
            if path.name != "sound.sqlite":
                shutil.copyfile(path, run / "files/sqlite" / path.name)
    shutil.copyfile(args.tz, run / "executables/tz.exe")
    admission_limit = args.admission_limit if args.admission_limit is not None else limit
    (run / "admission.conf").write_text(f"ADMISSION_LIMIT = {admission_limit}\n")
    env = dict(os.environ, GOMAXPROCS=str(limit))
    env.pop("MONGOOSE_USERNAME", None)
    env.pop("MONGOOSE_PASSWORD", None)
    command = [str(binary), "-db", "mongoose.db.new", "-promote-numbers",
               "-port", str(args.port), "-debug-addr", f"127.0.0.1:{args.debug_port}",
               "-checkpoint-interval", "0", "-log-level", "debug", "-log-dir", "logs"]
    if label == "new":
        command += ["-config", "admission.conf"]
    result = dict(label=label, limit=limit, repeat=repeat, binary_sha256=digest(binary),
                  database_sha256=digest(args.database), commands=[], login_attempts=[])
    result["admission_limit"] = admission_limit if label == "new" else None
    result["sqlite_sha256"] = {p.name: digest(p) for p in (run / "files/sqlite").glob("*.sqlite")}
    client = None
    started = time.monotonic()
    with (run / "stdout.txt").open("w") as out, (run / "stderr.txt").open("w") as err:
        proc = subprocess.Popen(command, cwd=run, env=env, stdout=out, stderr=err,
                                creationflags=subprocess.CREATE_NO_WINDOW)
        try:
            while True:
                if proc.poll() is not None:
                    raise RuntimeError(f"Server exited {proc.returncode}")
                if time.monotonic() - started > 90:
                    raise TimeoutError("Server startup exceeded 90 seconds")
                # Debug HTTP starts before database loading. Wait for the MOO
                # listener without creating an extra login/startup invocation.
                log = run / "logs/latest.jsonl"
                if log.exists() and re.search(r'"msg"\s*:\s*"listening"', log.read_text(encoding="utf-8")):
                    break
                time.sleep(.1)
            result["moo_ready_ms"] = (time.monotonic() - started) * 1000
            with (run / "transcript.txt").open("w", encoding="utf-8") as transcript:
                while time.monotonic() - started < args.login_deadline:
                    if (args.output / "STOP").exists():
                        raise InterruptedError("Operator stopped experiment")
                    attempt_start = time.monotonic()
                    client = Client(args.port, transcript)
                    client.send(f"PROXY TCP4 203.0.113.5 127.0.0.1 50000 {args.port}")
                    client.until(["Enter your username or email:"])
                    client.send(os.environ["MONGOOSE_USERNAME"])
                    client.until(["Password"])
                    password_at = time.monotonic()
                    client.send(os.environ["MONGOOSE_PASSWORD"])
                    matched, _ = client.until(["MESSAGE OF THE DAY:", "Confunc failed:", "Access Denied"])
                    result["login_attempts"].append(dict(outcome=matched,
                        connect_ms=(time.monotonic()-attempt_start)*1000,
                        password_ms=(time.monotonic()-password_at)*1000))
                    if matched == "MESSAGE OF THE DAY:":
                        break
                    if matched == "Confunc failed:" and args.accept_unclean:
                        break
                    client.close()
                    client = None
                    if matched == "Access Denied":
                        raise RuntimeError("Account login denied")
                    time.sleep(args.retry_delay)
                if client is None:
                    raise TimeoutError("No clean login before deadline")
                result["clean_login"] = matched == "MESSAGE OF THE DAY:"
                result["session_ready_from_start_ms"] = (time.monotonic()-started)*1000
                if result["clean_login"]:
                    result["clean_login_from_start_ms"] = result["session_ready_from_start_ms"]
                print(f"{run.name}: session ready {result['session_ready_from_start_ms']:.0f} ms, clean={result['clean_login']}", flush=True)
                result["idle_started_unix"] = time.time()
                time.sleep(args.settle)
                client.send(';return "admission-start";')
                client.until([re.escape('=> "admission-start"')])
                before = vars_at(args.debug_port)
                save(run / "vars-before.json", before)
                window = time.monotonic()
                cpu_before = cpu_seconds(proc)
                result["window_started_unix"] = time.time()
                while time.monotonic() - window < args.seconds:
                    if (args.output / "STOP").exists():
                        raise InterruptedError("Operator stopped experiment")
                    index = len(result["commands"])
                    cmd = ["look", "@who", ";return length(queued_tasks());"][index % 3]
                    marker = f"admission-bench-{index}"
                    begin = time.monotonic()
                    client.send(cmd)
                    client.send(';return "' + marker + '";')
                    _, response = client.until([re.escape('=> "' + marker + '"')], timeout=60)
                    result["commands"].append(dict(command=cmd, ms=(time.monotonic()-begin)*1000,
                        error=bool(re.search(r"Confunc failed:|Traceback|E_QUOTA|E_INVARG|E_TYPE|This database is not open|\.\.\. called from", response))))
                    time.sleep(args.interval)
                result["window_seconds"] = time.monotonic() - window
                result["cpu_seconds"] = cpu_seconds(proc) - cpu_before
                result["window_ended_unix"] = time.time()
                after = vars_at(args.debug_port)
                save(run / "vars-after.json", after)
                result["metrics_delta"] = {k: v-before.get(k, 0) for k, v in after.items()
                    if k.startswith("barn.") and isinstance(v, (float, int))}
                result["heap_after_bytes"] = after["memstats"]["HeapAlloc"]
                values = sorted(x["ms"] for x in result["commands"])
                result["latency_ms"] = dict(median=statistics.median(values),
                    p95=values[min(len(values)-1, int(.95*len(values)))], max=max(values))
                checkpoint_start = time.monotonic()
                checkpoint_log = run / "logs/latest.jsonl"
                log_offset = checkpoint_log.stat().st_size
                client.send(';dump_database();')
                while True:
                    with checkpoint_log.open("rb") as stream:
                        stream.seek(log_offset)
                        appended = stream.read().decode("utf-8", errors="replace")
                    terminal = None
                    for line in appended.splitlines():
                        try:
                            event = json.loads(line)
                        except json.JSONDecodeError:
                            continue
                        if event.get("msg") in ("checkpoint complete", "checkpoint failed", "dump_database() failed"):
                            terminal = event
                            break
                    if terminal:
                        result["checkpoint_event"] = terminal
                        result["checkpoint_success"] = terminal["msg"] == "checkpoint complete"
                        break
                    if (args.output / "STOP").exists():
                        raise InterruptedError("Operator stopped experiment")
                    if time.monotonic()-checkpoint_start > 90:
                        raise TimeoutError("Checkpoint did not complete in 90 seconds")
                    time.sleep(.1)
                result["checkpoint_terminal_ms"] = (time.monotonic()-checkpoint_start)*1000
        except Exception as exc:
            result["failure"] = str(exc)
            result["failed_after_ms"] = (time.monotonic()-started)*1000
            try:
                save(run / "vars-failure.json", vars_at(args.debug_port))
                with urllib.request.urlopen(f"http://127.0.0.1:{args.debug_port}/debug/pprof/goroutine?debug=2", timeout=10) as r:
                    (run / "stacks-failure.txt").write_bytes(r.read())
            except OSError as capture_error:
                result["capture_error"] = str(capture_error)
        finally:
            if client:
                client.close()
            if proc.poll() is None:
                proc.terminate()
            proc.wait(timeout=30)
    log_path = run / "logs/latest.jsonl"
    if log_path.exists():
        rows = []
        for line in log_path.read_text(encoding="utf-8").splitlines():
            if not line.strip():
                continue
            try:
                rows.append(json.loads(line))
            except json.JSONDecodeError:
                result["incomplete_log_records"] = result.get("incomplete_log_records", 0) + 1
        result["log_levels"] = dict(Counter(r.get("level") for r in rows))
        result["warnings_errors"] = dict(Counter(r.get("msg") for r in rows if r.get("level") in ("WARN", "ERROR")))
        if "window_ended_unix" in result:
            for name, start, end in (("idle", result["idle_started_unix"], result["window_started_unix"]),
                                     ("loaded", result["window_started_unix"], result["window_ended_unix"])):
                window_rows = [r for r in rows if start <= datetime.fromisoformat(r["time"]).timestamp() <= end]
                boundaries = [r for r in window_rows if r.get("msg") == "irreversible-effect boundary"]
                slices = [r for r in window_rows if r.get("msg") == "slow task slice" and r.get("verb") == "s_run"]
                result[name + "_background"] = dict(seconds=end-start,
                    schedule_publications=sum(r.get("verb") == "schedule" and r.get("renew") == "E_NONE" and r.get("published_writes") is True for r in boundaries),
                    s_run_slow_slices=len(slices), s_run_slow_seconds=sum(r["elapsed"] for r in slices)/1e9,
                    boundary_gate_wait_seconds=sum(r.get("gate_wait", 0) for r in boundaries)/1e9,
                    boundary_retries=sum(r.get("renew") == "E_INVARG" for r in boundaries))
    save(run / "summary.json", result)
    print(json.dumps({k: result[k] for k in ("label", "limit", "repeat", "latency_ms", "failure") if k in result}), flush=True)
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ("baseline", "new", "database", "sound", "tz", "output"):
        p.add_argument("--" + name, type=Path, required=True)
    p.add_argument("--repeats", type=int, default=3)
    p.add_argument("--support-dir", type=Path)
    p.add_argument("--accept-unclean", action="store_true", help="Measure an authenticated session after confunc failure, explicitly flagged unclean")
    p.add_argument("--labels", nargs="+", choices=["baseline", "new"], default=["baseline", "new"])
    p.add_argument("--limits", type=int, nargs="+", default=[1, 16])
    p.add_argument("--admission-limit", type=int, help="Override new-build admission capacity independently of GOMAXPROCS")
    p.add_argument("--seconds", type=int, default=30)
    p.add_argument("--settle", type=int, default=10)
    p.add_argument("--interval", type=float, default=.1)
    p.add_argument("--login-deadline", type=int, default=180)
    p.add_argument("--retry-delay", type=float, default=5)
    p.add_argument("--port", type=int, default=11505)
    p.add_argument("--debug-port", type=int, default=11506)
    p.add_argument("--pause-pid", type=int, default=0)
    args = p.parse_args()
    for name in ("baseline", "new", "database", "sound", "tz"):
        setattr(args, name, getattr(args, name).resolve(strict=True))
    args.output = args.output.resolve()
    if args.support_dir:
        args.support_dir = args.support_dir.resolve(strict=True)
        if any(args.support_dir.glob("*-wal")):
            p.error("Use a frozen SQLite backup directory without WAL sidecars")
    for credential in ("MONGOOSE_USERNAME", "MONGOOSE_PASSWORD"):
        if not os.environ.get(credential):
            p.error(f"Set {credential}")
    args.output.mkdir(parents=True, exist_ok=True)
    save(args.output / "manifest.json", {k: str(v) if isinstance(v, Path) else v for k, v in vars(args).items()})
    results = []
    with paused_process(args.pause_pid):
        for repeat in range(1, args.repeats+1):
            for limit in args.limits:
                order = ["baseline", "new"] if repeat % 2 else ["new", "baseline"]
                for label in order:
                    if label not in args.labels:
                        continue
                    if (args.output / "STOP").exists():
                        return
                    results.append(trial(args, label, getattr(args, label), limit, repeat))
                    save(args.output / "results.json", results)
    if any("failure" in r for r in results):
        raise SystemExit(1)


if __name__ == "__main__":
    main()
