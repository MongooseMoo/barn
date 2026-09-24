#!/usr/bin/env python3
"""Linux-native, alternating fresh-world Mongoose performance comparison.

Credentials come only from MONGOOSE_USERNAME / MONGOOSE_PASSWORD. See
docs/mongoose-performance.md for setup, interpretation, and limitations.
"""
from __future__ import annotations

import argparse
from collections import deque
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import re
import shutil
import signal
import socket
import statistics
import subprocess
import threading
import time
from urllib.request import urlopen

from bench_players import (Control, LineConn, compute_roster, create_account,
                           mongoose_login, wait_port)


# Each case returns a small, exact result. Exceptions never count as fast samples.
CASES = [
    ("empty_list_utils_range_10000", 'for i in ($list_utils:range(10000)) endfor return 1;', 1),
    ("list_utils_range_10000", 'n=0; for i in ($list_utils:range(10000)) n=n+1; endfor return n;', 10000),
    ("native_range_10000", 'n=0; for i in [1..10000] n=n+1; endfor return n;', 10000),
    ("integer_arithmetic", 'n=0; for i in [1..100000] n=n+i; endfor return n;', 5000050000),
    ("float_arithmetic", 'n=0.0; for i in [1..50000] n=n+0.5; endfor return n;', 25000.0),
    ("list_append_2000", 'v={}; for i in [1..2000] v={@v,i}; endfor return length(v);', 2000),
    ("list_index_10000", 'v=$list_utils:range(10000); n=0; for i in [1..10000] n=n+v[i]; endfor return n;', 50005000),
    ("string_concat_5000", 'v=""; for i in [1..5000] v=v+"x"; endfor return length(v);', 5000),
    ("map_updates_2000", 'v=[]; for i in [1..2000] v[i]=i; endfor return length(v);', 2000),
    ("property_reads_10000", 'n=0; for i in [1..10000] n=n+length(player.name); endfor return n==10000*length(player.name);', 1),
    ("verb_calls_2000", 'n=0; for i in [1..2000] n=n+length($list_utils:range(10)); endfor return n;', 20000),
    ("create_recycle_10", 'n=0; for i in [1..10] o=create(#1); recycle(o); n=n+!valid(o); endfor return n;', 10),
]

WORKER_VERB = [
    'r=eval(argstr);',
    'if(r[1]) notify(player,"=> "+toliteral(r[2]));',
    'else notify(player,"MATRIX_ERROR "+toliteral(r)); endif',
]


def sha(path):
    with open(path, "rb") as f:
        return hashlib.file_digest(f, "sha256").hexdigest()


def save(path, data):
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(data, indent=2) + "\n")
    tmp.replace(path)


def cpu_seconds(pid):
    fields = Path(f"/proc/{pid}/stat").read_text().rsplit(")", 1)[1].split()
    return (int(fields[11]) + int(fields[12])) / os.sysconf("SC_CLK_TCK")


def debug_counters(folder):
    match = re.search(r'debug endpoint listening" addr=(127\.0\.0\.1:\d+)',
                      (folder / "server.log").read_text(errors="replace"))
    if not match:
        return {}
    with urlopen(f"http://{match[1]}/debug/vars", timeout=5) as response:
        data = json.load(response)
    counters = {key: value for key, value in data.items() if key.startswith("barn.")}
    counters["go_memory"] = {key: data.get("memstats", {}).get(key) for key in
                             ("HeapAlloc", "HeapInuse", "HeapObjects", "NumGC", "PauseTotalNs", "GCCPUFraction")}
    return counters


def checked(ctl, code, expected=None):
    result, _ = ctl.eval(code, timeout=20)
    if expected is not None and result != expected:
        raise AssertionError(f"expected {expected!r}, got {result!r}")
    return result


def connect(port, user, password, worker=False):
    class RecordedConnection(LineConn):
        def send_line(self, line):
            if worker and line.startswith(";; "):
                line = "matrix_eval " + line[3:]
            super().send_line(line)

        def read_line(self, deadline):
            line = super().read_line(deadline)
            if line is not None:
                self.recent.append(line)
            return line

    conn = RecordedConnection("127.0.0.1", port)
    conn.recent = deque(maxlen=30)
    try:
        login = mongoose_login(conn, user, password,
                               f"PROXY TCP4 127.0.0.1 127.0.0.1 45678 {port}",
                               "auto", 0.5)
        return Control(conn), {"seconds": login.seconds,
                               "confunc_error": any("Confunc failed" in s for s in login.transcript)}
    except Exception as exc:
        conn.close()
        raise RuntimeError(f"login/control setup for {user}: {exc}; received={list(conn.recent)!r}") from exc


def measure_case(ctl, code, expected):
    # Server time excludes TCP, but includes the real MOO eval/compile path.
    quoted = json.dumps(code)
    start = time.perf_counter()
    result = checked(ctl, f't=ftime(1); try r=eval({quoted}); except e (ANY) r={{"error",toliteral(e)}}; endtry return {{ftime(1)-t,r}};')
    wall = time.perf_counter() - start
    if not isinstance(result, list) or len(result) != 2 or result[1] != [1, expected]:
        raise AssertionError(f"unsuccessful or incorrect eval: {result!r}")
    if not isinstance(result[0], (int, float)) or not math.isfinite(result[0]) or result[0] < 0:
        raise AssertionError(f"invalid elapsed server time: {result[0]!r}")
    return {"server_s": result[0], "wall_s": wall, "value": result[1][1]}


def parallel_case(controls, objects, shared, shape, operations, iterations):
    n = len(controls)
    targets = [shared if shape in ("hot_write", "read_only") else o for o in objects[:n]]
    for obj in set(targets):
        checked(controls[0], f'#{obj}.counter=0; return #{obj}.counter;', 0)
    barrier = threading.Barrier(n)

    def worker(index):
        ctl = controls[index]
        obj = targets[index]
        if shape == "read_only":
            code = f'n=0; for i in [1..{iterations}] n=n+#{obj}.counter; endfor return n;'
        else:
            # One commit per request; repeated reads/writes make overlapping
            # transactions likely. No suspend() inside a transaction.
            code = f'for i in [1..{iterations}] #{obj}.counter=#{obj}.counter+1; endfor return 1;'
        times = []
        barrier.wait(timeout=30)
        for _ in range(operations):
            start = time.perf_counter()
            checked(ctl, code, 0 if shape == "read_only" else 1)
            times.append(time.perf_counter() - start)
        return times

    start = time.perf_counter()
    with ThreadPoolExecutor(max_workers=n) as pool:
        results = list(pool.map(worker, range(n)))
    elapsed = time.perf_counter() - start
    actual = [checked(controls[0], f'return #{o}.counter;') for o in sorted(set(targets))]
    expected = ([0] if shape == "read_only" else
                [n * operations * iterations] if shape == "hot_write" else
                [operations * iterations] * n)
    if actual != expected:
        raise AssertionError(f"committed counter mismatch: {actual} != {expected}")
    flat = sorted(t for row in results for t in row)
    return {"clients": n, "shape": shape, "operations": n * operations,
            "iterations_per_operation": iterations, "elapsed_s": elapsed,
            "ops_s": n * operations / elapsed, "p50_s": statistics.median(flat),
            "p95_s": flat[min(len(flat)-1, int(len(flat)*0.95))],
            "max_s": max(flat), "counters": actual, "latencies_s": results}


def run_world(args, lane, index, root):
    folder = root / f"{index:02d}-{lane}"
    folder.mkdir()
    shutil.copy2(args.db, folder / "input.db")
    (folder / "files" / "sqlite").mkdir(parents=True)
    shutil.copy2(args.sound, folder / "files" / "sqlite" / "sound.sqlite")
    if args.tz:
        (folder / "executables").mkdir()
        shutil.copy2(args.tz, folder / "executables" / "tz")
        (folder / "executables" / "tz").chmod(0o755)
    engine = "toast" if lane == "toast" else "barn"
    env = os.environ.copy()
    # Credentials are client-only; never expose them to a server child.
    env = {key: value for key, value in env.items() if not key.startswith("MONGOOSE_")}
    if engine == "toast":
        command = [str(args.toast), "-o", "-i", "files", "input.db", "output.db", str(args.port)]
    else:
        env["GOMAXPROCS"] = lane.rsplit("-g", 1)[1]
        binary = args.barn_candidate if lane.startswith("barn-candidate-") else args.barn
        command = [str(binary), "--db", "input.db", "--listen", f"tcp://127.0.0.1:{args.port}",
                   "--promote-numbers", "--checkpoint-interval", "0", "--debug-addr", "127.0.0.1:0",
                   "--log-dir", "logs"]
    record = {"lane": lane, "round": index, "command": command,
              "gomaxprocs": env.get("GOMAXPROCS"), "samples": [], "parallel": [], "errors": []}
    controls = []
    process = None
    try:
        with socket.socket() as port_check:
            port_check.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            port_check.bind(("127.0.0.1", args.port))
        with (folder / "server.log").open("wb") as log:
            process = subprocess.Popen(command, cwd=folder, env=env, stdin=subprocess.DEVNULL,
                                       stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        record["listen_s"] = wait_port("127.0.0.1", args.port, 180, process)
        ctl, record["login"] = connect(args.port, os.environ["MONGOOSE_USERNAME"], os.environ["MONGOOSE_PASSWORD"])
        controls.append(ctl)
        record["identity"] = checked(ctl, 'return {player,player.wizard,server_version(),$prod()};')
        if record["identity"][1] != 1:
            raise RuntimeError("wizard control required")
        # Large straight-line interpreter cases deliberately need higher limits.
        record["original_limits"] = checked(ctl, 'return {$server_options.fg_ticks,$server_options.fg_seconds};')
        checked(ctl, '$server_options.fg_ticks=10000000; $server_options.fg_seconds=60; load_server_options(); return 1;', 1)
        time.sleep(args.settle)
        # Run PBT before lifecycle churn, which invokes Mongoose hooks.
        measure_commands(ctl, args, record, folder, lane, index)
        for name, code, expected in CASES:
            if not re.search(args.cases, name):
                continue
            for sample in range(args.samples + 1):
                entry = {"case": name, "sample": sample, "warmup": sample == 0}
                try:
                    entry.update(measure_case(ctl, code, expected))
                except Exception as exc:
                    entry["error"] = str(exc)
                    record["errors"].append({"case": name, "error": str(exc)})
                    record["samples"].append(entry)
                    break
                record["samples"].append(entry)
            save(folder / "result.json", record)
            print(lane, index, name, entry, flush=True)
        if args.levels:
            if args.login_trace:
                checked(ctl, 'c=verb_code(#10,"welcome"); for i in [1..length(c)] if(index(c[i],"unable to display welcome")) c[i]="notify(player, toliteral(e));"; endif endfor return set_verb_code(#10,"welcome",c);', [])
            # Existing ordinary players promoted to benchmark wizards need the
            # same inherited wizard preference as the older multiplayer driver.
            record["cloaked_repair"] = checked(ctl, 'return `property_info(#36,"cloaked") ! E_PROPNF => add_property(#36,"cloaked",0,{#36,"r"})\';')
            roster = [r for r in compute_roster(ctl) if r["obj"] > 100 and r["obj"] != record["identity"][0]][:max(args.levels)]
            record["worker_roster"] = roster
            if len(roster) != max(args.levels):
                raise RuntimeError("insufficient distinct players")
            workers = []
            password = os.urandom(16).hex()
            for i, player in enumerate(roster):
                checked(ctl, f'p=#{player["obj"]}; return `property_info(p,"cloaked") ! E_PROPNF => add_property(p,"cloaked",0,{{p,"r"}})\';')
                checked(ctl, f'#{player["obj"]}.wizard=1; #{player["obj"]}.programmer=1; return 1;', 1)
                program = "{" + ",".join(json.dumps(line) for line in WORKER_VERB) + "}"
                checked(ctl, f'p=#{player["obj"]}; add_verb(p,{{p,"rxd","matrix_eval"}},{{"any","any","any"}}); return set_verb_code(p,"matrix_eval",{program});', [])
                create_account(ctl, f"matrix{i}", password, player["obj"])
                worker, _ = connect(args.port, f"matrix{i}", password, worker=True)
                controls.append(worker)
                checked(worker, 'return player;', player["obj"])
                workers.append(worker)
            objs = [int(o) for o in checked(ctl, f'v={{}}; for i in [1..{len(workers)+1}] o=create(#-1); add_property(o,"counter",0,{{player,"rw"}}); v={{@v,o}}; endfor return v;')]
            for level in args.levels:
                for shape in ["read_only", "disjoint_write", "hot_write"]:
                    try:
                        before = debug_counters(folder) if engine == "barn" else {}
                        cpu_before = cpu_seconds(process.pid)
                        result = parallel_case(workers[:level], objs[1:], objs[0], shape, args.operations, args.iterations)
                        result["process_cpu_s"] = cpu_seconds(process.pid) - cpu_before
                        after = debug_counters(folder) if engine == "barn" else {}
                        result["debug_before"] = before
                        result["debug_after"] = after
                        record["parallel"].append(result)
                        print(lane, index, shape, level, round(result["ops_s"], 2), "ops/s", flush=True)
                    except Exception as exc:
                        record["errors"].append({"shape": shape, "clients": level, "error": str(exc)})
                        raise
                    save(folder / "result.json", record)
    except Exception as exc:
        record["errors"].append({"fatal": str(exc)})
        print(lane, index, "FAILED", str(exc), flush=True)
    finally:
        for ctl in controls:
            ctl.close()
        if process is not None and process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
            try:
                process.wait(15)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
        record["server_exit"] = None if process is None else process.returncode
        server_log = folder / "server.log"
        if server_log.exists():
            lines = server_log.read_text(errors="replace").splitlines()
            record["log_audit"] = {"warning_lines": sum("level=WARN" in line for line in lines),
                                   "error_lines": [line for line in lines if "level=ERROR" in line],
                                   "task_restore_failures": sum("failed to restore" in line for line in lines)}
        save(folder / "result.json", record)
    return record


def measure_commands(ctl, args, record, folder, lane, index):
    for command_text in ["@test $pbt"]:
        for sample in range(args.pbt_samples if command_text == "@test $pbt" else args.samples):
            start = time.perf_counter()
            ctl.conn.send_line(command_text)
            output = []
            saw_result = False
            while True:
                line = ctl.conn.read_line(start + 120)
                if line is None:
                    output = None
                    break
                output.append(line)
                saw_result = saw_result or "Total:" in line and "Failed:" in line
                if saw_result and line.strip().startswith("----------------"):
                    break
            entry = {"case": command_text, "sample": sample, "wall_s": time.perf_counter()-start}
            if output is None:
                entry["error"] = "terminal completion timeout"
            else:
                clean = re.sub(r"\x1b\[[0-9;]*m", "", "\n".join(output))
                (folder / f"command-{command_text.replace(' ', '_').replace('$', '')}-{sample}.txt").write_text(clean)
                if command_text == "@test $pbt":
                    match = re.search(r"Total:\s*(\d+)\s+Passed:\s*(\d+)\s+Failed:\s*(\d+)", clean)
                    if not match or tuple(map(int, match.groups())) != (65, 65, 0):
                        entry["error"] = "PBT did not report 65/65 passing"
                    duration = re.search(r"Failed:.*? in ([0-9.]+)(ms|s)", clean)
                    if duration:
                        entry["reported_s"] = float(duration[1]) / (1000 if duration[2] == "ms" else 1)
                elif not output or any(term in clean for term in ("Traceback", "Verb not found", "(End of traceback)")):
                    entry["error"] = "empty or erroneous command output"
            if "error" in entry:
                record["errors"].append(entry)
            record["samples"].append(entry)
            print(lane, index, entry, flush=True)
        save(folder / "result.json", record)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ("db", "sound", "barn", "toast", "out"):
        ap.add_argument("--"+name, type=Path, required=True)
    ap.add_argument("--tz", type=Path)
    ap.add_argument("--barn-candidate", type=Path, help="optional second Barn binary for alternating paired runs")
    ap.add_argument("--lanes", default="toast,barn-g4,barn-g1")
    ap.add_argument("--rounds", type=int, default=3)
    ap.add_argument("--samples", type=int, default=10)
    ap.add_argument("--pbt-samples", type=int, default=3)
    ap.add_argument("--cases", default=".", help="regular expression selecting interpreter cases")
    ap.add_argument("--login-trace", action="store_true", help="expose swallowed welcome exceptions on disposable fixture")
    ap.add_argument("--levels", default="1,4,8")
    ap.add_argument("--operations", type=int, default=10)
    ap.add_argument("--iterations", type=int, default=500)
    ap.add_argument("--settle", type=float, default=2)
    ap.add_argument("--port", type=int, default=17920)
    args = ap.parse_args()
    if platform.system() != "Linux":
        ap.error("run natively inside Linux/WSL so both engines share an OS")
    for name in ("db", "sound", "barn", "toast", "tz", "barn_candidate"):
        value = getattr(args, name)
        if value:
            value = value.resolve(strict=True)
            setattr(args, name, value)
    if not os.environ.get("MONGOOSE_USERNAME") or not os.environ.get("MONGOOSE_PASSWORD"):
        ap.error("set MONGOOSE_USERNAME and MONGOOSE_PASSWORD")
    lanes = args.lanes.split(",")
    if any(not re.fullmatch(r"toast|barn-(candidate-)?g[1-9][0-9]*", lane) for lane in lanes):
        ap.error("lanes must be toast, barn-gN, or barn-candidate-gN")
    if any(lane.startswith("barn-candidate-") for lane in lanes) and not args.barn_candidate:
        ap.error("barn-candidate lanes require --barn-candidate")
    args.levels = sorted({int(n) for n in args.levels.split(",") if n})
    if any(n <= 0 for n in args.levels) or min(args.rounds, args.samples, args.operations, args.iterations) < 1:
        ap.error("counts must be positive")
    root = args.out.resolve()
    root.mkdir(parents=True, exist_ok=False)
    metadata = {"platform": platform.platform(), "cpu_count": os.cpu_count(),
                "cpu_affinity": sorted(os.sched_getaffinity(0)), "args": vars(args).copy(),
                "artifacts": {n: {"path": str(getattr(args,n)), "sha256": sha(getattr(args,n))}
                              for n in ("db", "sound", "barn", "toast")},
                "runs": []}
    metadata["harness_sha256"] = sha(Path(__file__))
    metadata["client_sha256"] = sha(Path(__file__).with_name("bench_players.py"))
    metadata["cpu_model"] = next((line.split(":", 1)[1].strip() for line in Path("/proc/cpuinfo").read_text().splitlines() if line.startswith("model name")), "unknown")
    metadata["git_head"] = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=Path(__file__).resolve().parents[1], text=True).strip()
    if args.tz:
        metadata["artifacts"]["tz"] = {"path": str(args.tz), "sha256": sha(args.tz)}
    if args.barn_candidate:
        metadata["artifacts"]["barn_candidate"] = {"path": str(args.barn_candidate), "sha256": sha(args.barn_candidate)}
    metadata["args"] = {k: str(v) if isinstance(v, Path) else v for k,v in metadata["args"].items()}
    save(root / "matrix.json", metadata)
    for index in range(args.rounds):
        # Rotate first engine each round; every launch starts from source DB.
        order = lanes[index % len(lanes):] + lanes[:index % len(lanes)]
        for lane in order:
            metadata["runs"].append(run_world(args, lane, index, root))
            save(root / "matrix.json", metadata)
    return int(any(run["errors"] for run in metadata["runs"]))


if __name__ == "__main__":
    raise SystemExit(main())
