#!/usr/bin/env python3
"""Check on and manage the dedicated continuous fuzz chain.

The chain is fuzz.yml dispatched as dedicated-continuous; fuzz-controller.yml
submits each successor (replenish) and resumes a stalled chain (watchdog),
both in scripts/fuzz_compute.cjs. Workers are the listeners in runner group
barn-bug-discovery, inside the compute guest on mongoose.world
(.github/runner/README.md).

  python scripts/fuzz_ops.py status [--host] [--json]
  python scripts/fuzz_ops.py stop [--cancel]
  python scripts/fuzz_ops.py start [--campaign native-only|native-and-toast]
  python scripts/fuzz_ops.py runner restart N

Needs an authenticated `gh` (org runner read access for the runner list) and,
for --host and runner restart, root SSH to mongoose.world.
"""

import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import re
import subprocess
import sys

REPO = "MongooseMoo/barn"
ORG = "MongooseMoo"
GROUP = "barn-bug-discovery"
DEDICATED = "Nightly fuzz / dedicated-"
CONTINUOUS = "Nightly fuzz / dedicated-continuous"
ACTIVE = {"queued", "in_progress", "waiting", "pending", "requested"}
ACCEPT = "Accept discovery cycle"
NOT_LANES = {"Validate compute routing", "ToastDifferential", ACCEPT}
# fuzz_compute.cjs WATCHDOG_GRACE_MS: replenish's window before the watchdog acts.
GRACE_MINUTES = 30
HOST = "root@mongoose.world"
KNOWN_HOSTS = (Path.home() / ".ssh" / "barn-compute-known-hosts").as_posix()
SLOTS = (1, 2, 3, 4)
LISTENER = "actions.runner.MongooseMoo.mongoose-barn-fuzz-{}.service"
SLICE = "barn-fuzz.slice"  # .github/runner/barn-fuzz.slice: one memory bound for all listeners
DECISION = re.compile(r"(Submitted one dedicated successor|no successor submitted|"
                      r"continuous discovery stops here|Watchdog: .*)")

GUEST_PROBE = (
    'for s in 1 2 3 4; do u=actions.runner.MongooseMoo.mongoose-barn-fuzz-$s.service; '
    'printf "listener %s %s %s %s\\n" "$s" "$(systemctl is-active $u)" '
    '"$(systemctl show $u -p NRestarts --value)" "$(systemctl show $u -p ControlGroup --value)"; done; '
    'printf "slice %s %s\\n" "$(systemctl show barn-fuzz.slice -p MemoryMax --value)" '
    '"$(systemctl show barn-fuzz.slice -p MemoryCurrent --value)"; '
    # One kernel line per kill, naming its cgroup and task. systemd's "killed
    # by the OOM killer" lines repeat on daemon-reload and are not kills.
    'journalctl -q -k --since -24h --no-pager -o short-iso --grep "oom-kill:" | sed "s/^/oom /"; '
    'df -P / | awk "NR==2{print \\"disk\\", \\$5}"; '
    'free -m | awk "NR==2{print \\"memory\\", \\$3, \\$2}"; '
    'cut -d" " -f1-3 /proc/loadavg | sed "s/^/load /"')
HOST_PROBE = (
    'printf "host %s %s\\n" "$(systemctl is-active barn-compute.service)" '
    '"$(systemctl show barn-compute.service -p NRestarts --value)"; '
    'df -P /var/lib/barncompute | awk "NR==2{print \\"hostdisk\\", \\$5}"')


def parse_time(value):
    if not value or value.startswith("0001-"):
        return None
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def minutes(start, end):
    return int((end - start).total_seconds() // 60)


def duration(total):
    return f"{total // 60}h {total % 60}m" if total >= 60 else f"{total}m"


def summarize_cycle(run, jobs, now):
    """One fuzz.yml run as status facts; `jobs` is `gh run view --json jobs`."""
    lanes = [j for j in jobs if j["name"] not in NOT_LANES]
    accept = next((j for j in jobs if j["name"] == ACCEPT), None)
    return {
        "id": run["databaseId"], "title": run["displayTitle"], "status": run["status"],
        "conclusion": run["conclusion"] or None,
        "age_minutes": minutes(parse_time(run["createdAt"]), now),
        "idle_minutes": None if run["status"] in ACTIVE else minutes(parse_time(run["updatedAt"]), now),
        "lanes_total": len(lanes),
        "lanes_done": sum(j["status"] == "completed" for j in lanes),
        "red": [j["name"] for j in lanes if j["conclusion"] == "failure"],
        "running": [{"name": j["name"], "minutes": minutes(parse_time(j["startedAt"]), now)}
                    for j in lanes if j["status"] == "in_progress"],
        "accept": accept["conclusion"] or accept["status"] if accept else None,
    }


def parse_probe(text):
    """The GUEST_PROBE/HOST_PROBE output lines as a dict."""
    probe = {"listeners": {}, "oom": []}
    for line in text.splitlines():
        kind, _, rest = line.partition(" ")
        fields = rest.split()
        if kind == "listener":
            probe["listeners"][int(fields[0])] = {
                "state": fields[1], "restarts": int(fields[2]) if fields[2:] else None,
                "cgroup": fields[3] if fields[3:] else None}
        elif kind == "slice":
            probe["slice"] = {"memory_max": fields[0], "memory_current": fields[1] if fields[1:] else None}
        elif kind == "oom":
            probe["oom"].append(rest)
        elif kind in ("disk", "hostdisk"):
            probe[kind] = int(fields[0].rstrip("%"))
        elif kind == "memory":
            probe["memory"] = {"used_mb": int(fields[0]), "total_mb": int(fields[1])}
        elif kind == "load":
            probe["load"] = [float(x) for x in fields]
        elif kind == "host":
            probe["host"] = {"state": fields[0], "restarts": int(fields[1])}
    return probe


def decisions(log):
    """The replenish/watchdog decision lines from a controller run log."""
    found = []
    for line in log.splitlines():
        match = DECISION.search(line)
        if match and "##[error]" not in line:
            found.append(match.group(0))
    return found


def verdict(enabled, cycles, runners, probe=None):
    """(one-line state, [problems]) for the chain."""
    problems = []
    for runner in runners:
        if runner["status"] != "online":
            problems.append(f"runner {runner['name']} is {runner['status']}")
    if probe:
        for slot, listener in sorted(probe["listeners"].items()):
            if listener["state"] != "active":
                problems.append(f"guest listener {slot} is {listener['state']} "
                                f"(python scripts/fuzz_ops.py runner restart {slot})")
            if not (listener["cgroup"] or "").startswith(f"/{SLICE}/"):
                problems.append(f"guest listener {slot} runs outside {SLICE} ({listener['cgroup']}): apply "
                                ".github/runner/listener.conf and restart it while its runner is idle")
        if probe.get("slice", {}).get("memory_max") in (None, "infinity"):
            problems.append(f"{SLICE} has no memory bound: install .github/runner/barn-fuzz.slice")
        if probe["oom"]:
            problems.append(f"{len(probe['oom'])} kernel OOM kills in the last 24h; last: {probe['oom'][-1]}")
        for key in ("disk", "hostdisk"):
            if probe.get(key, 0) >= 85:
                problems.append(f"{'guest' if key == 'disk' else 'host'} disk {probe[key]}% full")
        if probe.get("host", {}).get("state", "active") != "active":
            problems.append(f"host barn-compute.service is {probe['host']['state']}")
    active = next((c for c in cycles if c["status"] in ACTIVE), None)
    last = next((c for c in cycles if c["title"].startswith(CONTINUOUS)), None)
    if not enabled:
        state = "STOPPED: FUZZ_CONTINUOUS_ENABLED is not true"
    elif active:
        state = f"RUNNING: cycle {active['id']} is {active['status']}"
    elif last is None:
        state = "BROKEN: continuous mode is on but no continuous cycle exists; run start"
    elif last["accept"] != "success" and last["idle_minutes"] < GRACE_MINUTES:
        state = (f"WAITING FOR WATCHDOG: cycle {last['id']} was not accepted ({last['accept']}) and ended "
                 f"{last['idle_minutes']}m ago; the watchdog resumes it after {GRACE_MINUTES}m")
    elif last["idle_minutes"] < GRACE_MINUTES:
        state = f"BETWEEN CYCLES: cycle {last['id']} ended {last['idle_minutes']}m ago; the controller submits the next"
    else:
        state = (f"BROKEN: nothing queued or running {duration(last['idle_minutes'])} after cycle {last['id']} "
                 f"(acceptance {last['accept']}); the watchdog should have resumed it: check scheduled "
                 "controller runs, or run start")
    return state, problems


def gh(*args):
    return subprocess.run(["gh", *args], capture_output=True, text=True, check=True).stdout


def gh_json(*args):
    return json.loads(gh(*args))


def ssh(target, script):
    if target == "host":
        command = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", HOST, script]
    else:
        command = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
                   "-o", f"UserKnownHostsFile={KNOWN_HOSTS}", "-J", HOST, "-p", "22822", "root@127.0.0.1", script]
    return subprocess.run(command, capture_output=True, text=True, timeout=60)


def enabled_flag():
    variables = {v["name"]: v["value"] for v in gh_json("variable", "list", "--repo", REPO, "--json", "name,value")}
    return variables.get("FUZZ_CONTINUOUS_ENABLED") == "true", variables


def fuzz_runs():
    return gh_json("run", "list", "--repo", REPO, "-w", "fuzz.yml", "-L", "40", "--json",
                   "databaseId,displayTitle,status,conclusion,createdAt,updatedAt,event")


def collect(host):
    now = datetime.now(timezone.utc)
    enabled, variables = enabled_flag()
    dedicated = [r for r in fuzz_runs() if r["displayTitle"].startswith(DEDICATED)][:4]
    cycles = [summarize_cycle(r, gh_json("run", "view", str(r["databaseId"]), "--repo", REPO, "--json", "jobs")["jobs"], now)
              for r in dedicated]
    controllers = []
    for run in gh_json("run", "list", "--repo", REPO, "-w", "fuzz-controller.yml", "-L", "8",
                       "--json", "databaseId,event,status,conclusion,createdAt"):
        if run["conclusion"] in ("success", "failure") and len(controllers) < 4:
            log = subprocess.run(["gh", "run", "view", str(run["databaseId"]), "--repo", REPO, "--log"],
                                 capture_output=True, text=True).stdout
            controllers.append({**run, "decisions": decisions(log)})
    groups = gh_json("api", f"orgs/{ORG}/actions/runner-groups")["runner_groups"]
    group = next(g for g in groups if g["name"] == GROUP)
    runners = [{"name": r["name"], "status": r["status"], "busy": r["busy"]}
               for r in gh_json("api", f"orgs/{ORG}/actions/runner-groups/{group['id']}/runners")["runners"]]
    issues = [i for i in gh_json("issue", "list", "--repo", REPO, "--state", "open", "-L", "50",
                                 "--search", 'in:title "Nightly fuzz"', "--json", "number,title,updatedAt")
              if minutes(parse_time(i["updatedAt"]), now) <= 24 * 60]
    probe, probe_errors = None, []
    if host:
        text = ""
        for target, script in (("host", HOST_PROBE), ("guest", GUEST_PROBE)):
            result = ssh(target, script)
            text += result.stdout
            if result.returncode:
                probe_errors.append(f"{target} probe exited {result.returncode}: {result.stderr.strip()}")
        probe = parse_probe(text)
    state, problems = verdict(enabled, cycles, runners, probe)
    return {"state": state, "problems": probe_errors + problems, "enabled": enabled,
            "max_parallel": variables.get("FUZZ_MAX_PARALLEL"), "cycles": cycles,
            "controllers": controllers, "runners": runners, "issues": issues, "probe": probe}


def render(status):
    lines = [status["state"]]
    lines += [f"Problem: {p}" for p in status["problems"]] or ["No problems seen."]
    lines.append(f"Continuous mode {'on' if status['enabled'] else 'off'}, {status['max_parallel']} lanes at a time.")
    for i, c in enumerate(status["cycles"]):
        head = "Current cycle" if i == 0 and c["status"] in ACTIVE else "Cycle"
        outcome = c["status"] if c["status"] in ACTIVE else f"ended {duration(c['idle_minutes'])} ago, {c['conclusion']}"
        lines.append(f"{head} {c['id']}, {c['title'].removeprefix('Nightly fuzz / ')}, started "
                     f"{duration(c['age_minutes'])} ago, {outcome}, acceptance {c['accept'] or 'not run'}.")
        lines.append(f"  Lanes done {c['lanes_done']} of {c['lanes_total']}, red {len(c['red'])}"
                     + (f": {', '.join(c['red'])}" if c["red"] else "") + ".")
        for r in c["running"]:
            lines.append(f"  Running {r['name']} for {duration(r['minutes'])}.")
    for c in status["controllers"]:
        said = "; ".join(c["decisions"]) or "no decision line in log"
        lines.append(f"Controller run {c['databaseId']} ({c['event']}, {c['createdAt']}): {c['conclusion']}, {said}.")
    for r in status["runners"]:
        lines.append(f"Runner {r['name']}: {r['status']}, {'busy' if r['busy'] else 'idle'}.")
    probe = status["probe"]
    if probe:
        for slot, listener in sorted(probe["listeners"].items()):
            lines.append(f"Guest listener {slot}: {listener['state']}, restarts {listener['restarts']}, "
                         f"cgroup {listener['cgroup']}.")
        if "slice" in probe:
            lines.append(f"{SLICE}: memory {probe['slice']['memory_current']} of {probe['slice']['memory_max']} bytes.")
        if "memory" in probe:
            lines.append(f"Guest memory {probe['memory']['used_mb']} of {probe['memory']['total_mb']} MB used, "
                         f"disk {probe.get('disk')}% used, load {' '.join(map(str, probe.get('load', [])))}.")
        if "host" in probe:
            lines.append(f"Host barn-compute.service {probe['host']['state']}, restarts {probe['host']['restarts']}, "
                         f"disk {probe.get('hostdisk')}% used.")
    lines.append(f"Fuzz issues updated in the last 24h: {len(status['issues'])}.")
    lines += [f"  #{i['number']} {i['title']}" for i in status["issues"]]
    return "\n".join(lines)


def active_dedicated():
    return [r for r in fuzz_runs() if r["displayTitle"].startswith(DEDICATED) and r["status"] in ACTIVE]


def stop(cancel):
    gh("variable", "set", "FUZZ_CONTINUOUS_ENABLED", "--repo", REPO, "--body", "false")
    print("Continuous mode off: no successor follows the current cycle and the watchdog does nothing.")
    for run in active_dedicated():
        if cancel:
            gh("run", "cancel", str(run["databaseId"]), "--repo", REPO)
            print(f"Cancelled cycle {run['databaseId']}.")
        else:
            print(f"Cycle {run['databaseId']} keeps running to its end (stop --cancel cancels it).")


def start(campaign):
    running = active_dedicated()
    if running:
        print(f"Refused: dedicated cycle {running[0]['databaseId']} is already {running[0]['status']}.")
        return 1
    if campaign is None:
        last = next((r for r in fuzz_runs() if r["displayTitle"].startswith(CONTINUOUS)), None)
        campaign = "native-only" if last and last["displayTitle"].endswith("/ native-only") else "native-and-toast"
    gh("variable", "set", "FUZZ_CONTINUOUS_ENABLED", "--repo", REPO, "--body", "true")
    gh("workflow", "run", "fuzz.yml", "--repo", REPO, "--ref", "master",
       "-f", "compute-mode=dedicated-continuous", "-f", f"campaign={campaign}")
    print(f"Continuous mode on; dispatched one dedicated-continuous {campaign} cycle.")
    return 0


def restart_runner(slot):
    unit = LISTENER.format(slot)
    result = ssh("guest", f'u={unit}; s=$(systemctl is-active $u); '
                          f'if [ "$s" = active ]; then echo "Refused: $u is active."; exit 3; fi; '
                          f'systemctl restart $u && sleep 5; echo "$u: $(systemctl is-active $u)"')
    print((result.stdout + result.stderr).strip())
    return result.returncode


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    commands = parser.add_subparsers(dest="command")
    status = commands.add_parser("status", help="show the chain's state (default)")
    status.add_argument("--host", action="store_true", help="also probe the host and guest over SSH")
    status.add_argument("--json", action="store_true")
    commands.add_parser("stop", help="turn continuous mode off").add_argument(
        "--cancel", action="store_true", help="also cancel the running dedicated cycle")
    commands.add_parser("start", help="turn continuous mode on and start one cycle").add_argument(
        "--campaign", choices=["native-only", "native-and-toast"],
        help="default: the newest continuous cycle's campaign")
    runner = commands.add_parser("runner", help="guest listener management")
    runner.add_argument("action", choices=["restart"])
    runner.add_argument("slot", type=int, choices=SLOTS)
    args = parser.parse_args(argv)
    if args.command == "stop":
        stop(args.cancel)
        return 0
    if args.command == "start":
        return start(args.campaign)
    if args.command == "runner":
        return restart_runner(args.slot)
    result = collect(getattr(args, "host", False))
    print(json.dumps(result, indent=2) if getattr(args, "json", False) else render(result))
    return 0


if __name__ == "__main__":
    sys.exit(main())
