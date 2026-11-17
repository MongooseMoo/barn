"""Bounded leak evidence for owned Jepsen generations and native corpus replay."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time
import urllib.request

from fuzz_retention import native_corpus

ROOT = Path(__file__).resolve().parents[1]


def leak_count(text):
    match = re.search(r"^goroutineleak profile: total (\d+)$", text, re.M)
    if not match:
        raise ValueError("missing native goroutineleak profile header")
    return int(match[1])


def test_process_findings(directory):
    """Check captures independently of Go exit status, including fault runs."""
    findings = []
    processes = sorted(directory.glob("process-*"))
    if not processes:
        return ["incomplete memory evidence: no test process captures"]
    for process in processes:
        for boundary in ("before", "after"):
            try:
                sample = json.loads((process / boundary / "sample.json").read_text())
                count = sample["leaked_goroutines"]
                if type(count) is not int or count < 0:
                    raise ValueError("invalid leaked_goroutines")
                if count:
                    findings.append(f"{process.name}/{boundary}: {count} native goroutine leaks")
            except (OSError, ValueError, KeyError, TypeError) as error:
                findings.append(f"{process.name}/{boundary}: incomplete memory evidence: {error}")
    return findings


def capture_owned(server, directory):
    """Capture only the verified managed PID. Never query a SIGSTOPped server."""
    directory.mkdir(parents=True, exist_ok=False)
    server.verify_owned()
    pid = server.proc.pid
    result = {"pid": pid, "start_ticks": server.start_ticks, "monotonic_ns": time.monotonic_ns(),
              "paused": server.paused, "errors": [], "leaked_goroutines": None}
    proc = Path("/proc") / str(pid)
    for name in ("status", "smaps_rollup", "cgroup"):
        try:
            (directory / name).write_bytes((proc / name).read_bytes())
        except OSError as error:
            result["errors"].append(f"{name}: {error}")
    try:
        result["fds"] = len(list((proc / "fd").iterdir()))
    except OSError as error:
        result["errors"].append(f"fds: {error}")
    # Shared group and tmpfs counters are context, never per-input attribution.
    result["runner_scope"] = "shared cgroup/tmpfs; not attributable to this PID"
    try:
        lines = (proc / "cgroup").read_text().splitlines()
        group = next(line[3:] for line in lines if line.startswith("0::"))
        root = Path("/sys/fs/cgroup").resolve()
        path = (root / group.lstrip("/")).resolve()
        if not path.is_relative_to(root):
            raise ValueError("cgroup path escapes mount")
        for name in ("memory.events", "memory.stat", "memory.current"):
            (directory / name).write_bytes((path / name).read_bytes())
    except (OSError, ValueError, StopIteration) as error:
        result["runner_error"] = str(error)
    try:
        fs = os.statvfs("/dev/shm")
        result["tmpfs_used_bytes"] = (fs.f_blocks - fs.f_bfree) * fs.f_frsize
    except OSError as error:
        result["tmpfs_error"] = str(error)
    if not server.paused:
        address = (server.identity.get("endpoints") or {}).get("debug")
        # OwnedBarnServer validates that this endpoint is private loopback.
        for name, route in (("goroutineleak.txt", "pprof/goroutineleak?debug=1"),
                            ("heap.pprof", "pprof/heap?gc=1"),
                            ("goroutines.txt", "pprof/goroutine?debug=2"), ("vars.json", "vars")):
            try:
                if not address:
                    raise ValueError("owned debug address is missing")
                with urllib.request.urlopen(f"http://{address}/debug/{route}", timeout=10) as response:
                    with (directory / name).open("wb") as stream:
                        shutil.copyfileobj(response, stream)
            except (OSError, ValueError) as error:
                result["errors"].append(f"{name}: {error}")
        try:
            result["leaked_goroutines"] = leak_count((directory / "goroutineleak.txt").read_text())
        except (OSError, ValueError) as error:
            result["errors"].append(str(error))
    server.verify_owned()
    (directory / "sample.json").write_text(json.dumps(result, indent=2) + "\n")
    return result


def generation_findings(inputs, lifecycle, enabled):
    """Keep leak and incomplete verdicts separate from isolation/recovery."""
    rows = [row for row in lifecycle if row.get("event") == "memory"]
    leaks, errors = [], []
    if enabled:
        admitted = {row["generation"] for row in lifecycle if row.get("event") == "admit"}
        captured = {row["generation"] for row in rows if row.get("boundary") == "admitted"}
        errors.extend(f"generation {generation}: missing admitted memory capture" for generation in admitted - captured)
    for row in rows:
        where = f"generation {row['generation']} {row['boundary']}"
        try:
            path = inputs / row["path"]
            if not path.resolve().is_relative_to(inputs.resolve()):
                raise ValueError("memory artifact path escapes evidence directory")
            sample = json.loads((path / "sample.json").read_text())
            if sample["pid"] != row["pid"] or sample["start_ticks"] != row["start_ticks"]:
                raise ValueError("memory capture process identity mismatch")
            errors.extend(f"{where}: {error}" for error in sample["errors"])
            if not sample["paused"]:
                count = leak_count((path / "goroutineleak.txt").read_text())
                if count != sample["leaked_goroutines"]:
                    raise ValueError("profile and sample leak counts differ")
                if count:
                    leaks.append(f"{where}: {count} native goroutine leaks; {row['path']}")
        except (OSError, KeyError, ValueError, TypeError) as error:
            errors.append(f"{where}: incomplete memory capture: {error}")
    return {"captures": len(rows), "leaks": leaks, "errors": errors}


def replay_attempt(binary, source, directory, timeout):
    directory.mkdir()
    environment = dict(os.environ, BARN_LEAK_INPUT=str(source.resolve()),
                       BARN_LEAK_OUTPUT=str(directory.resolve()))
    command = [str(binary.resolve()), "-test.run=^TestMemorySourceReplay$", "-test.count=1",
               "-test.v", "-test.timeout=20s"]
    try:
        with (directory / "go.log").open("w") as log:
            code = subprocess.run(command, cwd=ROOT / "vm", env=environment, stdout=log,
                                  stderr=subprocess.STDOUT, timeout=timeout, check=False).returncode
        result = json.loads((directory / "replay.json").read_text())
        output = (directory / "go.log").read_text()
        verdict = result["verdict"]
        marker = "--- PASS: TestMemorySourceReplay" if verdict == "healthy" else "--- FAIL: TestMemorySourceReplay"
        expected = 0 if verdict == "healthy" else 1
        if verdict not in ("healthy", "leak", "heap-candidate") or code != expected or marker not in output:
            raise ValueError(f"incomplete test: exit={code}, verdict={verdict}")
        return {"verdict": verdict, "exit_code": code, "result": result, "evidence": str(directory)}
    except (OSError, ValueError, KeyError, subprocess.TimeoutExpired) as error:
        return {"verdict": "incomplete", "error": str(error), "evidence": str(directory)}


def replay(args):
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=False)
    started, rows = time.monotonic(), []
    result = {"verdict": "incomplete", "records": rows, "seconds_budget": args.seconds,
              "target": args.target, "commit": os.environ.get("GITHUB_SHA"), "argv": sys.argv}
    report = args.output / "results.json"
    report.write_text(json.dumps(result, indent=2) + "\n")
    # One executable avoids leaving a `go test` child running after a timeout.
    binary = args.output / ("replay.exe" if os.name == "nt" else "replay.test")
    try:
        with (args.output / "build.log").open("w") as log:
            code = subprocess.run(["go", "test", "-c", "-o", str(binary), "./vm"],
                                  cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=False,
                                  timeout=args.seconds).returncode
    except (OSError, subprocess.TimeoutExpired) as error:
        code = -1
        result["build_error"] = str(error)
    if code:
        result["error"] = f"replay build exited {code}"
    else:
        corpus = native_corpus(args.cache, "./vm", args.target)
        files = sorted(path for path in corpus.iterdir() if path.is_file()) if corpus.is_dir() else []
        # Rotate through real discoveries rather than checking the same prefix forever.
        if files:
            offset = int(os.environ.get("GITHUB_RUN_NUMBER", "0")) % len(files)
            files = files[offset:] + files[:offset]
        for source in files[:8]:
            remaining = args.seconds - (time.monotonic() - started)
            if remaining < 5:
                break
            digest = hashlib.sha256(source.read_bytes()).hexdigest()
            saved = args.output / digest
            saved.mkdir()
            record = saved / "input"
            record.write_bytes(source.read_bytes())
            attempts = [replay_attempt(binary, record, saved / "first", min(25, remaining))]
            verdict = attempts[0]["verdict"]
            if verdict in ("leak", "heap-candidate"):
                remaining = args.seconds - (time.monotonic() - started)
                if remaining >= 5:
                    attempts.append(replay_attempt(binary, record, saved / "confirm", min(25, remaining)))
                verdict = ("leak" if verdict == "leak" else "retained-growth") if (
                    len(attempts) == 2 and attempts[1]["verdict"] == verdict) else "inconclusive"
            rows.append({"input_path": str(record), "sha256": digest, "attempts": attempts, "verdict": verdict})
            result["elapsed_seconds"] = time.monotonic() - started
            report.write_text(json.dumps(result, indent=2) + "\n")
        if rows:
            result["verdict"] = ("finding" if any(r["verdict"] in ("leak", "retained-growth") for r in rows)
                                 else "incomplete" if any(r["verdict"] != "healthy" for r in rows) else "healthy")
        else:
            result["error"] = "no native input completed memory replay within the budget"
    result["elapsed_seconds"] = time.monotonic() - started
    report.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result), flush=True)
    return {"healthy": 0, "finding": 1, "incomplete": 2}[result["verdict"]]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cache", type=Path, required=True)
    parser.add_argument("--target", choices=["FuzzSourceExecution", "FuzzStructuredSourceExecution"], required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--seconds", type=int, default=60)
    args = parser.parse_args()
    if args.seconds < 5 or args.seconds > 90:
        parser.error("--seconds must be 5..90")
    return replay(args)


if __name__ == "__main__":
    sys.exit(main())
