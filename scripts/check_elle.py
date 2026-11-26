#!/usr/bin/env python3
"""Generate Barn histories and require explicit Elle verdicts, retaining evidence."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys

from memory_check import test_process_findings

# Each BARN_ELLE_FAULT removes one protection; Elle must reject every one of
# them. Kept equal to elleFaults in engine/elle_workload_test.go by
# test_check_elle.py.
FAULTS = ["property-reads", "scalar-reads", "verb-reads", "relationship-reads",
          "waif-reads", "map-key-reads", "map-key-rebase", "map-key-capture",
          "commit-exclusion", "valid-reads", "ring-reads", "map-key-metadata",
          "map-key-presence", "map-key-whole-read", "hot-map-carry",
          "hot-memo-guard", "property-scans", "property-shapes", "verb-scans",
          "verb-memo"]


def verdict(output, history):
    # Exit status alone cannot distinguish accepted from inconclusive histories.
    # Accept exactly one verdict for
    # the requested input, never a stray 'true' in an exception or graph message.
    matches = re.findall(r"^" + re.escape(str(history)) + r"\s+(true|false|unknown)\s*$",
                         output, re.MULTILINE)
    if len(matches) != 1:
        raise ValueError(f"expected one verdict for {history}, got {matches}")
    return matches[0]


def run(command, log, env=None):
    with log.open("w", encoding="utf-8") as stream:
        result = subprocess.run(command, stdout=stream, stderr=subprocess.STDOUT,
                                env=env, check=False)
    return result.returncode, log.read_text(encoding="utf-8")


def check_history(java, jar, history, case, record):
    errors = []
    label, model, fault = record["case"], record["model"], record["fault"]
    if not history.is_file() or history.stat().st_size == 0:
        errors.append(f"{label}/{model}: missing or empty history")
        return record, errors
    graphs = case / f"graphs-{model}"
    cli = [java, "-jar", str(jar), "-m", model, "-c", "serializable",
           "--directory", str(graphs), "--max-plot-bytes", "2000000", str(history)]
    exit_code, output = run(cli, case / f"{model}-elle.log")
    record.update(elle_command=cli, exit_code=exit_code)
    try:
        actual = verdict(output, history)
        record["verdict"] = actual
        expected = "false" if fault else "true"
        expected_exit = 1 if fault else 0
        if exit_code != expected_exit or actual != expected:
            errors.append(f"{label}/{model}: exit={exit_code}, verdict={actual}, expected={expected}/{expected_exit}")
        if fault and actual == "false":
            witnesses = list(graphs.rglob("*.txt"))
            required = record.get("witness_terms", ())
            if not any(all(term in path.read_text(encoding="utf-8") for term in required) for path in witnesses):
                errors.append(f"{label}/{model}: rejected without a witness containing {required}")
        print(f"{label}/{model}: {actual} (expected {expected})", flush=True)
    except ValueError as error:
        errors.append(f"{label}/{model}: {error}")
    return record, errors


def lifecycle_verdict(code, output, fault):
    name = "TestUncaughtHandlerBeforeYieldedParent"
    status = "FAIL" if fault else "PASS"
    marker = rf"^--- {status}: {name} \([^)]+\)$"
    footer = r"^FAIL\s+github.com/MongooseMoo/barn/engine\s" if fault else r"^ok\s+github.com/MongooseMoo/barn/engine\s"
    expected_code = 1 if fault else 0
    witness = "parent observed {194, 0}, want {194, 1}; dispatched while handler held=1"
    if (code != expected_code or not re.search(marker, output, re.MULTILINE)
            or not re.search(footer, output, re.MULTILINE)
            or (fault and witness not in output)):
        raise ValueError(f"expected {name} {status} with exit {expected_code}"
                         + (" and the premature-read witness" if fault else ""))
    return "rejected" if fault else "passed"


def check_isolation_cases(args, cases):
    results, errors = [], []
    for label, seed, fault in cases:
        case = args.output / label
        env = os.environ.copy()
        env.update(BARN_ELLE_OUTPUT=str(case), BARN_ELLE_SEED=str(seed),
                   BARN_ELLE_ROUNDS=str(args.rounds), GOMAXPROCS="4",
                   BARN_ELLE_PATHS="",
                   BARN_LEAK_OUTPUT=str(args.output / (label + "-memory")),
                   BARN_ELLE_FAULT=fault)
        command = ["go", "test", "-tags", "elle", "./engine", "-run",
                   "^TestElleIsolationHistories$", "-count=1", "-v", "-timeout=5m"]
        code, _ = run(command, args.output / f"{label}-go.log", env)
        errors.extend(f"{label}: {finding}" for finding in test_process_findings(args.output / (label + "-memory")))
        if code:
            errors.append(f"{label}: Go workload exited {code}")
        # Every fault targets append read-modify-write isolation; blind-write
        # register checking is required for healthy cases only.
        for model in ("list-append",) if fault else ("list-append", "rw-register"):
            record, failures = check_history(args.java, args.jar, case / f"{model}.edn", case, {
                "case": label, "seed": seed, "model": model, "fault": fault,
                "go_command": command})
            results.append(record)
            errors.extend(failures)
        if fault == "hot-memo-guard":
            projection = args.output / "hot-memo-pair"
            projection.mkdir()
            record, failures = check_history(args.java, args.jar, case / "hot-memo-pair.edn", projection, {
                "case": "hot-memo-pair", "model": "list-append", "fault": fault,
                "witness_terms": ("hot-memo", "212", "property"), "go_command": command})
            results.append(record)
            errors.extend(failures)
    return results, errors


def write_results(output, results, errors):
    (output / "results.json").write_text(json.dumps(
        {"results": results, "errors": errors}, indent=2) + "\n", encoding="utf-8")
    for error in errors:
        print(error, file=sys.stderr)
    return bool(errors)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--jar", type=Path, required=True)
    parser.add_argument("--java", default="java")
    parser.add_argument("--output", type=Path, required=True,
                        help="new evidence directory; existing directories are refused")
    parser.add_argument("--seeds", nargs="+", type=int, default=[412, 395, 396])
    parser.add_argument("--rounds", type=int, default=40)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--replay", type=Path, help="check a saved EDN history without running Barn")
    mode.add_argument("--fault", choices=FAULTS, help="run just this negative control with the first seed")
    parser.add_argument("--model", choices=("list-append", "rw-register"), default="list-append",
                        help="checker model for --replay")
    parser.add_argument("--expect-rejection", action="store_true", help="require a witness for --replay")
    args = parser.parse_args()
    if not args.jar.is_file():
        parser.error("--jar must name the built pinned elle-cli jar")
    args.output = args.output.resolve()
    args.jar = args.jar.resolve()
    args.output.mkdir(parents=True, exist_ok=False)
    results, errors = [], []
    if args.replay is not None:
        record, failures = check_history(args.java, args.jar, args.replay.resolve(), args.output, {
            "case": "replay", "model": args.model, "fault": "replay" if args.expect_rejection else ""})
        return write_results(args.output, [record], failures)
    if args.fault:
        records, failures = check_isolation_cases(args, [(f"fault-{args.fault}", args.seeds[0], args.fault)])
        return write_results(args.output, records, failures)
    # Preserve and replay the real CI input that deadlocked Elle 0.2.7.
    case = args.output / "checkpoint-drop-append"
    case.mkdir()
    history = case / "list-append.edn"
    fixture = Path(__file__).resolve().parent / "fixtures/elle/checkpoint-drop-append.edn"
    history.write_bytes(fixture.read_bytes())
    record, failures = check_history(args.java, args.jar, history, case, {
        "case": case.name, "seed": None, "model": "list-append",
        "fault": "checkpoint-drop-append", "go_command": None})
    results.append(record)
    errors.extend(failures)
    # Serializable histories do not establish scheduler order. Require the real
    # fork/handler/parent assertion separately, with its historical defect.
    for fault in ("", "finished-task-index"):
        label = "lifecycle-" + (fault or "healthy")
        env = os.environ.copy()
        env.update(BARN_ELLE_FAULT=fault, GOMAXPROCS="4",
                   BARN_LEAK_OUTPUT=str(args.output / (label + "-memory")))
        command = ["go", "test", "-tags", "elle", "./engine", "-run",
                   "^TestUncaughtHandlerBeforeYieldedParent$", "-count=1", "-v", "-timeout=1m"]
        code, output = run(command, args.output / f"{label}-go.log", env)
        errors.extend(f"{label}: {finding}" for finding in test_process_findings(args.output / (label + "-memory")))
        record = {"case": label, "fault": fault, "go_command": command, "exit_code": code}
        try:
            record["verdict"] = lifecycle_verdict(code, output, fault)
            print(f"{label}: {record['verdict']}", flush=True)
        except ValueError as error:
            errors.append(f"{label}: {error}; actual exit={code}")
        results.append(record)
    held = args.output / "held"
    env = dict(os.environ, BARN_ELLE_OUTPUT=str(held), BARN_ELLE_FAULT="", GOMAXPROCS="4")
    command = ["go", "test", "-tags", "elle", "./engine", "-run", "^TestElleHeldHistories$", "-count=1", "-v", "-timeout=1m"]
    code, _ = run(command, args.output / "held-go.log", env)
    if code:
        errors.append(f"held scenarios: Go workload exited {code}")
    for name in ("history-prune", "room-membership", "mixed-contention"):
        case = args.output / name
        case.mkdir()
        record, failures = check_history(args.java, args.jar, held / f"{name}.edn", case, {
            "case": name, "model": "list-append", "fault": "", "go_command": command})
        results.append(record)
        errors.extend(failures)
    case = args.output / "fault-hot-map-carry-after-prune"
    case.mkdir()
    command = [*command[:6], "^TestElleHeldHistories$/^history-prune$", *command[7:]]
    env.update(BARN_ELLE_OUTPUT=str(case), BARN_ELLE_FAULT="hot-map-carry")
    code, _ = run(command, case / "go.log", env)
    if code:
        errors.append(f"{case.name}: Go workload exited {code}")
    record, failures = check_history(args.java, args.jar, case / "history-prune.edn", case, {
        "case": case.name, "model": "list-append", "fault": "hot-map-carry", "go_command": command})
    results.append(record)
    errors.extend(failures)
    # Same workload and seed under healthy validation and under each fault. The
    # synchronized waves and the held write-skew pair make every negative control
    # independent of lucky timing.
    cases = [(f"seed-{seed}", seed, "") for seed in args.seeds]
    cases += [(f"fault-{fault}", args.seeds[0], fault) for fault in FAULTS]
    records, failures = check_isolation_cases(args, cases)
    return write_results(args.output, results + records, errors + failures)


if __name__ == "__main__":
    sys.exit(main())
