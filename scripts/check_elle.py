#!/usr/bin/env python3
"""Generate Barn histories and require explicit Elle verdicts, retaining evidence."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys

# Each BARN_ELLE_FAULT removes one protection; Elle must reject every one of
# them. Kept equal to elleFaults in engine/elle_workload_test.go by
# test_check_elle.py.
FAULTS = ["property-reads", "scalar-reads", "verb-reads", "relationship-reads",
          "waif-reads", "map-key-reads", "map-key-rebase", "map-key-capture",
          "commit-exclusion", "valid-reads"]


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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--jar", type=Path, required=True)
    parser.add_argument("--java", default="java")
    parser.add_argument("--output", type=Path, required=True,
                        help="new evidence directory; existing directories are refused")
    parser.add_argument("--seeds", nargs="+", type=int, default=[412, 395, 396])
    parser.add_argument("--rounds", type=int, default=40)
    args = parser.parse_args()
    if not args.jar.is_file():
        parser.error("--jar must name the built pinned elle-cli jar")
    args.output = args.output.resolve()
    args.jar = args.jar.resolve()
    args.output.mkdir(parents=True, exist_ok=False)
    results, errors = [], []
    # Same workload and seed under healthy validation and under each fault. The
    # synchronized waves and the held write-skew pair make every negative control
    # independent of lucky timing.
    cases = [(f"seed-{seed}", seed, "") for seed in args.seeds]
    cases += [(f"fault-{fault}", args.seeds[0], fault) for fault in FAULTS]
    for label, seed, fault in cases:
        case = args.output / label
        env = os.environ.copy()
        env.update(BARN_ELLE_OUTPUT=str(case), BARN_ELLE_SEED=str(seed),
                   BARN_ELLE_ROUNDS=str(args.rounds), GOMAXPROCS="4",
                   BARN_ELLE_PATHS="",
                   BARN_ELLE_FAULT=fault)
        command = ["go", "test", "-tags", "elle", "./engine", "-run",
                   "^TestElleIsolationHistories$", "-count=1", "-v", "-timeout=5m"]
        code, _ = run(command, args.output / f"{label}-go.log", env)
        if code:
            errors.append(f"{label}: Go workload exited {code}")
        for model in ["list-append", "rw-register"]:
            history = case / f"{model}.edn"
            # Every fault targets append read-modify-write isolation; blind-write
            # register checking is required for healthy cases only.
            if fault and model == "rw-register":
                continue
            if not history.is_file() or history.stat().st_size == 0:
                errors.append(f"{label}/{model}: missing or empty history")
                continue
            graphs = case / f"graphs-{model}"
            cli = [args.java, "-jar", str(args.jar), "-m", model, "-c", "serializable",
                   "--directory", str(graphs), "--max-plot-bytes", "2000000", str(history)]
            exit_code, output = run(cli, case / f"{model}-elle.log")
            record = {"case": label, "seed": seed, "model": model, "fault": fault,
                      "go_command": command, "elle_command": cli, "exit_code": exit_code}
            try:
                actual = verdict(output, history)
                record["verdict"] = actual
                expected = "false" if fault else "true"
                expected_exit = 1 if fault else 0
                if exit_code != expected_exit or actual != expected:
                    errors.append(f"{label}/{model}: exit={exit_code}, verdict={actual}, expected={expected}/{expected_exit}")
                if fault and actual == "false" and not list(graphs.rglob("*.txt")):
                    errors.append(f"{label}/{model}: rejected without an anomaly witness")
                print(f"{label}/{model}: {actual} (expected {expected})", flush=True)
            except ValueError as error:
                errors.append(f"{label}/{model}: {error}")
            results.append(record)
    (args.output / "results.json").write_text(json.dumps(
        {"results": results, "errors": errors}, indent=2) + "\n", encoding="utf-8")
    for error in errors:
        print(error, file=sys.stderr)
    return bool(errors)


if __name__ == "__main__":
    sys.exit(main())
