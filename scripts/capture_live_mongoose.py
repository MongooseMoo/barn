#!/usr/bin/env python3
"""Read-only live Mongoose process/queue capture; no debugger, signals or restart.

python scripts/capture_live_mongoose.py --moo-state --out .tmp/live-mongoose.json
The optional Ops API query reads metadata once. Its bearer token is read by
the remote collector and excluded from collector output. It cannot see the
currently executing interpreter's MOO stack. Native profiling is separate.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import statistics
import subprocess
import sys


MOO_STATE = r"""
now = ftime();
queue_count = queued_tasks(0, 1);
q = queue_count <= 500 ? queued_tasks() | {};
groups = [];
rows = {};
bytes = 0;
overflow = 0;
for t in (q)
  key = tostr(t[6], ":", t[7]);
  if (maphaskey(groups, key) || length(mapkeys(groups)) < 64)
    groups[key] = groups:get(key, 0) + 1;
  else
    overflow = overflow + 1;
  endif
  bytes = bytes + t[10];
  if (length(rows) < 32)
    rows = {@rows, ["id" -> t[1], "status_or_due" -> tostr(t[2]),
      "programmer" -> tostr(t[5]), "definer" -> tostr(t[6]),
      "verb" -> t[7], "line" -> t[8], "receiver" -> tostr(t[9]),
      "estimated_bytes" -> t[10]]};
  endif
endfor
jobs = $sunnet_scheduler.scheduled_tasks;
scheduled = {};
overdue = 0;
oldest = 0.0;
examined = 0;
for n in (jobs)
  if (examined >= 500)
    break;
  endif
  examined = examined + 1;
  lag = max(0.0, now - n[3]);
  if (lag > 0.0)
    overdue = overdue + 1;
  endif
  oldest = max(oldest, lag);
  if (length(scheduled) < 64)
    scheduled = {@scheduled, ["id" -> n[2], "interval_s" -> n[1],
      "due_in_s" -> n[3] - now, "programmer" -> tostr(n[4]),
      "receiver" -> tostr(n[12]), "verb" -> n[13]]};
  endif
endfor
return generate_json(["server_time" -> now, "queued_count" -> queue_count,
  "queue_inspection_limited" -> (queue_count > 500),
  "queued_shown" -> length(rows), "queued_estimated_bytes" -> bytes,
  "groups" -> groups, "group_overflow" -> overflow, "queued" -> rows,
  "scheduled_count" -> length(jobs), "scheduled_shown" -> length(scheduled),
  "scheduled_examined" -> examined, "scheduled_inspection_limited" -> (length(jobs) > examined),
  "scheduled_overdue" -> overdue, "oldest_scheduled_lag_s" -> oldest,
  "scheduled" -> scheduled, "current_cycle" -> $space_objects.universe.current_cycle,
  "universe_cycling" -> $space_objects.universe.cycling]);
"""


# Sent over SSH stdin and executed by python3, without a remote script/file.
REMOTE = r'''
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time
from urllib.request import Request, urlopen

interval, count = float(sys.argv[1]), int(sys.argv[2])
with_moo = sys.argv[3] == "1"
query = MOO_QUERY
properties = subprocess.run(
    ["systemctl", "show", "mongoose.service",
     "--property=MainPID,ActiveState,SubState,NRestarts,ControlGroup,MemoryCurrent"],
    check=True, capture_output=True, text=True).stdout
service = dict(line.split("=", 1) for line in properties.splitlines() if "=" in line)
pid = int(service["MainPID"])
if pid <= 0:
    raise RuntimeError("service has no live main process")
base = Path("/proc") / str(pid)
exe = os.readlink(base / "exe")
digest = hashlib.sha256()
with (base / "exe").open("rb") as binary:
    for chunk in iter(lambda: binary.read(1048576), b""):
        digest.update(chunk)
hz, page = os.sysconf("SC_CLK_TCK"), os.sysconf("SC_PAGE_SIZE")

def process(path, identity):
    raw = (path / "stat").read_text()
    fields = raw.rsplit(")", 1)[1].split()
    return {
        "pid": identity, "start_ticks": int(fields[19]),
        "cpu_ticks": int(fields[11]) + int(fields[12]),
        "state": fields[0], "rss_bytes": int(fields[21]) * page,
        "monotonic": time.monotonic(),
    }

def sample():
    item = process(base, pid)
    item["utc"] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    item["threads"] = []
    for path in sorted((base / "task").iterdir()):
        try:
            row = process(path, int(path.name))
            row["comm"] = (path / "comm").read_text().strip()
            row["wait_channel"] = (path / "wchan").read_text().strip()
            item["threads"].append(row)
        except FileNotFoundError:
            pass  # Threads may finish during an approximate procfs sample.
    status = (base / "status").read_text().splitlines()
    item["status"] = {line.split(":", 1)[0]: line.split(":", 1)[1].strip()
                      for line in status if line.startswith(
                          ("VmRSS:", "VmHWM:", "VmSwap:", "Threads:"))}
    return item

result = {
    "protocol_version": 1, "service": service, "pid": pid, "executable": exe,
    "executable_sha256": digest.hexdigest(), "clock_ticks": hz,
    "logical_cpus": os.cpu_count(), "samples": [], "cgroup_processes": [],
    "moo_state": {"requested": with_moo, "ok": False},
}
cgroup = Path("/sys/fs/cgroup") / service["ControlGroup"].lstrip("/")
if (cgroup / "cgroup.procs").is_file():
    for number in (cgroup / "cgroup.procs").read_text().splitlines():
        path = Path("/proc") / number
        try:
            entry = process(path, int(number))
            entry["comm"] = (path / "comm").read_text().strip()
            result["cgroup_processes"].append(entry)
        except FileNotFoundError:
            pass
    result["cgroup_processes"].sort(key=lambda entry: entry["rss_bytes"], reverse=True)

for index in range(count + 1):
    try:
        result["samples"].append(sample())
    except FileNotFoundError:
        result["sampling_error"] = "main process disappeared"
        break
    if index < count:
        time.sleep(interval)
if with_moo:
    start = time.monotonic()
    try:
        token = (Path.home() / "mongoose/files/moo-agent.token").read_text().strip()
        request = Request("https://mongoose.world/api/ops/eval",
            data=json.dumps({"code": query}).encode(),
            headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
        with urlopen(request, timeout=15) as response:
            result["moo_state"] = {"requested": True, "ok": True,
                                   "response": json.load(response)}
    except Exception as exc:
        # Do not retain headers, token, request body or arbitrary error payloads.
        result["moo_state"] = {"requested": True, "ok": False,
                               "error_type": type(exc).__name__}
    result["moo_state"]["roundtrip_s"] = time.monotonic() - start
result["service_after"] = dict(
    line.split("=", 1) for line in subprocess.run(
        ["systemctl", "show", "mongoose.service",
         "--property=MainPID,ActiveState,SubState,NRestarts"],
        check=True, capture_output=True, text=True).stdout.splitlines() if "=" in line)
print(json.dumps(result))
'''


def cpu_delta(before, after, hz):
    """Percent of one logical CPU, only for the same process incarnation."""
    if (before["pid"], before["start_ticks"]) != (after["pid"], after["start_ticks"]):
        return None
    elapsed = after["monotonic"] - before["monotonic"]
    ticks = after["cpu_ticks"] - before["cpu_ticks"]
    if elapsed <= 0 or ticks < 0 or hz <= 0:
        return None
    return 100.0 * ticks / hz / elapsed


def decode_moo_state(payload):
    if payload.get("ok") != 1 or not isinstance(payload.get("value"), str):
        raise ValueError("Ops query did not return a compiled result")
    state = json.loads(json.loads(payload["value"]))
    if not isinstance(state, dict) or not {"queued_count", "queued_shown", "groups", "current_cycle"} <= state.keys():
        raise ValueError("Ops query did not return the expected diagnostic state")
    return state


def summarize(capture):
    samples = capture["samples"]
    rates = []
    for before, after in zip(samples, samples[1:]):
        first = next((t for t in before["threads"] if t["pid"] == before["pid"]), None)
        last = next((t for t in after["threads"] if t["pid"] == after["pid"]), None)
        if first is not None and last is not None:
            rate = cpu_delta(first, last, capture["clock_ticks"])
            if rate is not None:
                rates.append(rate)
    result = {
        "mean_main_cpu_percent": statistics.mean(rates) if rates else None,
        "main_cpu_intervals": len(rates),
        "peak_main_rss_bytes": max((s["rss_bytes"] for s in samples), default=None),
        "running_moo_verb": "unknown",
        "scope": "main-thread CPU; queued/suspended metadata is not running-task attribution",
    }
    state = capture.get("moo_state", {})
    if state.get("ok"):
        try:
            result["moo_state"] = decode_moo_state(state["response"])
        except (ValueError, TypeError, KeyError) as exc:
            result["moo_state_error"] = type(exc).__name__
    return result


def capture_verdict(capture, summary, expected_intervals, moo_requested):
    """Coverage and observed identities, not an atomic procfs snapshot claim."""
    def integer(value):
        if isinstance(value, bool) or not isinstance(value, (int, str)):
            return None
        return int(value) if re.fullmatch(r"[0-9]+", str(value)) else None

    service, after = capture.get("service", {}), capture.get("service_after", {})
    pid = integer(capture.get("pid"))
    before_pid, after_pid = integer(service.get("MainPID")), integer(after.get("MainPID"))
    before_restarts, after_restarts = integer(service.get("NRestarts")), integer(after.get("NRestarts"))
    samples = capture["samples"]
    identities = [(integer(row.get("pid")), integer(row.get("start_ticks"))) for row in samples]
    identity_known = bool(identities) and all(p is not None and p > 0 and start is not None
                                             for p, start in identities)
    main_threads_match = all(any(integer(thread.get("pid")) == p and
                                integer(thread.get("start_ticks")) == start
                                for thread in row.get("threads", []))
                             for row, (p, start) in zip(samples, identities))
    checks = {
        "service_identity_known": all(value is not None for value in
                                      (pid, before_pid, after_pid, before_restarts, after_restarts))
                                  and all(value > 0 for value in (pid or 0, before_pid or 0, after_pid or 0)),
        "same_service_pid": pid is not None and pid == before_pid == after_pid,
        "same_restart_count": before_restarts is not None and before_restarts == after_restarts,
        "service_running": all(item.get("ActiveState") == "active" and item.get("SubState") == "running"
                               for item in (service, after)),
        "same_sample_incarnation": identity_known and len(set(identities)) == 1
                                   and identities[0][0] == pid and main_threads_match,
        "no_sampling_error": "sampling_error" not in capture,
        "complete_sample_coverage": len(samples) == expected_intervals + 1,
        "complete_cpu_interval_coverage": summary["main_cpu_intervals"] == expected_intervals,
        "requested_moo_metadata": not moo_requested or "moo_state" in summary,
    }
    changed = ((before_pid is not None and after_pid is not None and before_pid != after_pid)
               or (before_restarts is not None and after_restarts is not None and before_restarts != after_restarts)
               or (identity_known and (len(set(identities)) != 1 or
                                       (pid is not None and identities[0][0] != pid))))
    return ("process_changed" if changed else "capture_complete" if all(checks.values()) else "capture_partial"), checks


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="mongoose@mongoose.world")
    parser.add_argument("--interval", type=float, default=1.0)
    parser.add_argument("--samples", type=int, default=12)
    parser.add_argument("--moo-state", action="store_true")
    parser.add_argument("--ssh-exe", help="SSH executable; normally discovered automatically")
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9_.-]+@[A-Za-z0-9_.-]+", args.host):
        parser.error("--host must be user@hostname")
    if not 0.2 <= args.interval <= 5 or not 1 <= args.samples <= 30 or args.interval * args.samples > 30:
        parser.error("capture must have 1..30 samples, 0.2..5s interval and at most 30s duration")
    ssh_exe = args.ssh_exe or shutil.which("ssh")
    if ssh_exe is None and sys.platform == "win32" and sys.maxsize < 2**32:
        native_ssh = Path(os.environ.get("WINDIR", "C:/Windows")) / "Sysnative/OpenSSH/ssh.exe"
        if native_ssh.is_file():
            ssh_exe = str(native_ssh)
    if ssh_exe is None:
        parser.error("SSH executable unavailable; supply --ssh-exe")
    args.out.parent.mkdir(parents=True, exist_ok=True)
    # Reserve output before touching the remote system; never overwrite evidence.
    with args.out.open("x", encoding="utf-8") as out:
        json.dump({"host": args.host, "state": "capture_started"}, out)
    command = [ssh_exe, "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", args.host,
               f"python3 - {args.interval} {args.samples} {int(args.moo_state)}"]
    # The Ops endpoint calls eval(code) with one STR argument (one source line).
    source = REMOTE.replace("query = MOO_QUERY", "query = " + repr(" ".join(MOO_STATE.splitlines())))
    record = {"host": args.host, "collector_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              "remote_exit": None}
    try:
        completed = subprocess.run(command, input=source.encode(), capture_output=True,
                                   timeout=args.interval * args.samples + 35)
        record["remote_exit"] = completed.returncode
        record["raw_stdout"] = completed.stdout.decode(errors="replace")
        record["raw_stderr"] = completed.stderr.decode(errors="replace")
        if completed.returncode != 0:
            raise RuntimeError("remote collector failed; see saved raw output")
        capture = json.loads(record["raw_stdout"])
        if capture.get("protocol_version") != 1:
            raise ValueError("unexpected capture protocol")
        record["capture"] = capture
        record["summary"] = summarize(capture)
        record["state"], record["capture_checks"] = capture_verdict(
            capture, record["summary"], args.samples, args.moo_state)
    except Exception as exc:
        record["state"] = "capture_failed"
        record["error_type"] = type(exc).__name__
    args.out.write_text(json.dumps(record, indent=2) + "\n", encoding="utf-8")
    if "summary" in record:
        # The full bounded metadata and raw evidence are in --out.
        summary = {k: v for k, v in record["summary"].items() if k != "moo_state"}
        if "moo_state" in record["summary"]:
            moo = record["summary"]["moo_state"]
            summary["moo"] = {k: moo[k] for k in (
                "queued_count", "queued_shown", "scheduled_count", "scheduled_overdue",
                "oldest_scheduled_lag_s", "current_cycle", "universe_cycling")}
            summary["queued_groups"] = moo["groups"]
        summary["remote_exit"] = record["remote_exit"]
        summary["evidence"] = str(args.out.resolve())
        print(json.dumps(summary, indent=2))
    else:
        print(json.dumps({"state": record["state"], "error_type": record.get("error_type"),
                          "evidence": str(args.out.resolve())}))
    return 0 if record["state"] == "capture_complete" else 1


if __name__ == "__main__":
    raise SystemExit(main())
