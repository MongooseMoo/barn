#!/usr/bin/env python3
"""Bounded exploratory Jepsen batches on the dedicated discovery runners.

Plan section 6 (plans/jepsen-expansion.md). One batch, inside a 30-minute
budget, runs scripts/jepsen_checkpoint.py campaigns in this order:

1. re-executions of open findings from earlier batches (--replay of the
   recorded client transactions with the original parameters and seed);
2. one healthy baseline configuration;
3. exploratory configurations chosen greedily to cover the factor-level pairs
   (FACTORS) no earlier run of the current round has covered.

Every run writes a new evidence directory; nothing is overwritten. A run that
reports a correctness or recovery class is re-analysed offline (--analyze
into a new directory) and its client transactions are kept in the ledger so
later batches re-execute it and count how often it reproduces. The ledger
(coverage, findings, measured run times) is a directory carried between
batches by the workflow's cache. Each batch writes report.json, report.md and
draft finding records under drafts/; nothing is posted anywhere.

Exit status: 0 clean batch, 1 a defect class was reported (the batch is
still complete), 4 incomplete (a run kept no results, or reported only an
incomplete or invalid experiment, or no run fit the budget).
"""
from __future__ import annotations

import argparse
import hashlib
import itertools
import json
import os
import random
import re
import subprocess
import sys
import time
from pathlib import Path

import jepsen_checkpoint as jc

CAMPAIGN = Path(__file__).resolve().with_name("jepsen_checkpoint.py")

# The dimensions a batch rotates through. Levels are strings so a pair key
# is plain text. dwell only changes the authority/topology verbs; gomaxprocs
# "runner" leaves Barn's GOMAXPROCS to the Go runtime.
FACTORS = {
    "families": ("smoke,props,keyed", "effects", "authority", "topology",
                 "smoke,props,keyed,effects,authority,topology"),
    "lane": ("fixed", "live", "transport", "stages"),
    "tasks": ("off", "on"),
    "concurrency": ("4", "16"),
    "rate": ("20", "60"),
    "hot_keys": ("family", "2"),
    "dwell": ("0", "5"),
    "gomaxprocs": ("runner", "1", "2"),
}
# Healthy in the fixed jepsen.yml campaign's families and lane at a low load:
# a batch whose baseline is not healthy says more about the runner than Barn.
BASELINE = {"families": "smoke,props,keyed", "lane": "fixed", "tasks": "off", "concurrency": "4", "rate": "20",
            "hot_keys": "family", "dwell": "0", "gomaxprocs": "runner"}
# Phase seconds per fault lane. "stages" runs the elle build's deterministic
# stage schedule (jepsen_checkpoint.STAGE_SCHEDULE), which replaces the phases.
LANES = {
    "fixed": {"phase1": 20, "phase3": 30, "phase_live": 0, "phase_transport": 0},
    "live": {"phase1": 15, "phase3": 20, "phase_live": 40, "phase_transport": 0},
    "transport": {"phase1": 15, "phase3": 20, "phase_live": 0, "phase_transport": 40},
    "stages": {"phase1": 0, "phase3": 0, "phase_live": 0, "phase_transport": 0, "stages": "all",
               "stage_seconds": 8},
}
# Starting estimate of one run's wall time beyond its client phases: process
# launches, observations, Jepsen start-up and Elle analysis. Not measured; the
# ledger's measured run times replace it per lane once a batch has run.
OVERHEAD_SECONDS = 120
MEASURED_KEPT = 10
BUDGET_SECONDS = 1800
# At most this many re-executions per batch, and they use at most half the budget.
MAX_REEXECUTIONS = 3
# A finding is re-executed until it has this many attempts, then its
# reproduction frequency is final and it is no longer scheduled.
REEXECUTION_ATTEMPTS = 5
MAX_PLANNED = 40
DEFECT_CLASSES = (jc.CORRECTNESS, jc.RECOVERY)
INCOMPLETE_VERDICTS = (jc.INCOMPLETE, jc.INVALID)
LEDGER_VERSION = 1


# ---------------------------------------------------------------------------
# Covering schedule

def pair_key(a, level_a, b, level_b):
    (a, level_a), (b, level_b) = sorted(((a, level_a), (b, level_b)))
    return f"{a}={level_a}|{b}={level_b}"


ALL_PAIRS = frozenset(pair_key(a, x, b, y) for a, b in itertools.combinations(sorted(FACTORS), 2)
                      for x in FACTORS[a] for y in FACTORS[b])


def pairs_of(config):
    return {pair_key(a, config[a], b, config[b]) for a, b in itertools.combinations(sorted(FACTORS), 2)}


def next_config(covered, rng, candidates=30):
    """AETG-style greedy choice: of several candidates, each built factor by
    factor with the level covering the most uncovered pairs with the levels
    already chosen, keep the one covering the most new pairs."""
    uncovered = ALL_PAIRS - set(covered)
    best = None
    for _ in range(candidates):
        order = list(FACTORS)
        rng.shuffle(order)
        config = {}
        for factor in order:
            levels = list(FACTORS[factor])
            rng.shuffle(levels)

            def score(level):
                with_chosen = sum(pair_key(factor, level, other, config[other]) in uncovered for other in config)
                overall = sum(1 for pair in uncovered if f"{factor}={level}" in pair.split("|"))
                return with_chosen, overall
            config[factor] = max(levels, key=score)
        gain = len(pairs_of(config) & uncovered)
        if best is None or gain > best[0]:
            best = (gain, config)
    return best[1], best[0]


def estimate_seconds(config, ledger):
    measured = ledger.get("elapsed", {}).get(config["lane"])
    if measured:
        return max(max(measured), 1)
    lane = LANES[config["lane"]]
    if lane.get("stages"):
        client = (len(jc.STAGE_SCHEDULE) + 1) * lane["stage_seconds"]
    else:
        client = lane["phase1"] + lane["phase3"] + lane["phase_live"] + lane["phase_transport"] + 6
    return client + OVERHEAD_SECONDS


# ---------------------------------------------------------------------------
# Ledger

def empty_ledger():
    return {"version": LEDGER_VERSION, "round": 0, "covered": [], "batches": 0, "elapsed": {}, "findings": {}}


def load_ledger(directory):
    path = Path(directory) / "ledger.json"
    if not path.exists():
        return empty_ledger()
    ledger = json.loads(path.read_text(encoding="utf-8"))
    if ledger.get("version") != LEDGER_VERSION:
        raise ValueError(f"{path}: ledger version {ledger.get('version')} is not {LEDGER_VERSION}")
    return ledger


def save_ledger(directory, ledger):
    Path(directory).mkdir(parents=True, exist_ok=True)
    (Path(directory) / "ledger.json").write_text(json.dumps(ledger, indent=2, sort_keys=True) + "\n",
                                                 encoding="utf-8")


def finding_key(finding):
    """A defect's identity across runs: its class and witness, or its message
    with numbers (generations, ids, tokens) erased when it has no witness."""
    return f"{finding['class']}|{finding.get('witness') or re.sub(r'[0-9]+', 'N', finding['message'])}"


def finding_id(key):
    return hashlib.sha256(key.encode()).hexdigest()[:12]


def open_findings(ledger):
    """Findings still scheduled for re-execution: replayable and with fewer
    than REEXECUTION_ATTEMPTS attempts; ones that already reproduced first."""
    rows = [row for row in ledger["findings"].values()
            if row.get("replayable") and row["attempts"] < REEXECUTION_ATTEMPTS]
    return sorted(rows, key=lambda row: (-row["reproductions"], row["attempts"], row["first_seen"]["batch"],
                                         row["id"]))


# ---------------------------------------------------------------------------
# Plan

def parameters_of(config, seed, timeout):
    """jepsen_checkpoint.py parameters (provenance form) for a configuration."""
    lane = LANES[config["lane"]]
    return {"phase1": lane["phase1"], "phase3": lane["phase3"], "phase_live": lane["phase_live"],
            "phase_transport": lane["phase_transport"], "rate": int(config["rate"]),
            "concurrency": int(config["concurrency"]), "seed": seed, "jepsen_timeout": timeout,
            "workloads": config["families"].split(","),
            "stages": list(jc.STAGE_SCHEDULE) if lane.get("stages") == "all" else [],
            "stage_seconds": lane.get("stage_seconds", 10), "control": None, "tasks": config["tasks"] == "on",
            "dwell": int(config["dwell"]), "transport_kinds": list(jc.TRANSPORT_KINDS),
            "hot_keys": 0 if config["hot_keys"] == "family" else int(config["hot_keys"]),
            "gomaxprocs": None if config["gomaxprocs"] == "runner" else config["gomaxprocs"]}


def plan(ledger, budget, seed):
    """The batch's ordered entries, filling the estimated budget: open
    findings' re-executions (at most half of it), the baseline, then the
    configurations covering the most pairs not yet covered this round."""
    rng = random.Random(seed)
    entries, spent = [], 0
    for row in open_findings(ledger):
        estimate = row["estimate_seconds"]
        if len(entries) >= MAX_REEXECUTIONS or spent + estimate > budget / 2:
            break
        entries.append({"kind": "reexecute", "finding": row["id"], "parameters": row["parameters"],
                        "estimate_seconds": estimate})
        spent += estimate
    covered = set(ledger["covered"])
    index = 0
    while spent < budget and index < MAX_PLANNED:
        if index == 0:
            kind, config, gain = "baseline", dict(BASELINE), len(pairs_of(BASELINE) - covered)
        else:
            if covered >= ALL_PAIRS:
                covered = set()  # every pair covered: the next round starts
            kind, (config, gain) = "explore", next_config(covered, rng)
        covered |= pairs_of(config)
        estimate = estimate_seconds(config, ledger)
        entries.append({"kind": kind, "config": config, "new_pairs": gain, "estimate_seconds": estimate,
                        "parameters": parameters_of(config, seed * 1000 + index, 2 * estimate)})
        spent += estimate
        index += 1
    return entries


# ---------------------------------------------------------------------------
# Run

def campaign_command(parameters, paths, output, replay=None):
    """The jepsen_checkpoint.py command and environment for one run."""
    binary = paths["barn_elle"] if parameters["stages"] else paths["barn"]
    command = [sys.executable, str(CAMPAIGN), "--barn", str(binary), "--db", str(paths["db"]),
               "--jepsen-jar", str(paths["jepsen_jar"]), "--elle-jar", str(paths["elle_jar"]),
               "--java", str(paths["java"]), "--output", str(output),
               "--workloads", ",".join(parameters["workloads"]), "--rate", str(parameters["rate"]),
               "--concurrency", str(parameters["concurrency"]), "--seed", str(parameters["seed"]),
               "--jepsen-timeout", str(parameters["jepsen_timeout"]), "--dwell", str(parameters["dwell"])]
    command += ["--phase1", str(parameters["phase1"]), "--phase3", str(parameters["phase3"]),
                "--phase-live", str(parameters["phase_live"]),
                "--phase-transport", str(parameters["phase_transport"])]
    if parameters["phase_transport"]:
        command += ["--transport-kinds", ",".join(parameters["transport_kinds"])]
    if parameters["stages"]:
        command += ["--stages", ",".join(parameters["stages"]), "--stage-seconds", str(parameters["stage_seconds"])]
    if parameters["tasks"]:
        command.append("--tasks")
    if parameters.get("hot_keys"):
        command += ["--hot-keys", str(parameters["hot_keys"])]
    if parameters.get("control"):
        command += ["--control", parameters["control"]]
    if replay is not None:
        command += ["--replay", str(replay)]
    environment = dict(os.environ)
    environment.pop("GOMAXPROCS", None)
    if parameters.get("gomaxprocs"):
        environment["GOMAXPROCS"] = str(parameters["gomaxprocs"])
    return command, environment


def parameters_from_provenance(provenance, timeout=None):
    """A recorded run's parameters, for re-executing it."""
    parameters = dict(provenance["parameters"])
    parameters.setdefault("hot_keys", 0)
    parameters["gomaxprocs"] = ((provenance.get("runner") or {}).get("environment") or {}).get("GOMAXPROCS")
    if timeout is not None:
        parameters["jepsen_timeout"] = timeout
    return parameters


def read_results(directory):
    path = Path(directory) / "results.json"
    if not path.exists():
        return None
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except ValueError:
        return None


def defect_keys(results):
    return sorted({finding_key(finding) for finding in (results or {}).get("findings") or []
                   if finding["class"] in DEFECT_CLASSES})


def execute(parameters, paths, output, log, replay=None):
    """Run one campaign; returns its run row (never raises for a failed run)."""
    command, environment = campaign_command(parameters, paths, output, replay)
    started = time.monotonic()
    timed_out = False
    with open(log, "wb") as stream:
        try:
            code = subprocess.run(command, env=environment, stdout=stream, stderr=subprocess.STDOUT,
                                  timeout=parameters["jepsen_timeout"] + 600, check=False).returncode
        except subprocess.TimeoutExpired:
            code, timed_out = None, True
    results = read_results(output)
    return {"command": command, "gomaxprocs": parameters.get("gomaxprocs"), "exit": code, "timed_out": timed_out,
            "elapsed_seconds": round(time.monotonic() - started, 1), "evidence_retained": results is not None,
            "verdict": (results or {}).get("verdict"), "classes": (results or {}).get("classes"),
            "defects": defect_keys(results), "outcomes": (results or {}).get("outcomes") or {},
            "replay": (results or {}).get("replay")}


def offline_analysis(directory, output, paths, log):
    """--analyze a run's directory into a new one; the defects it reports."""
    command = [sys.executable, str(CAMPAIGN), "--analyze", str(directory), "--output", str(output),
               "--elle-jar", str(paths["elle_jar"]), "--java", str(paths["java"])]
    with open(log, "wb") as stream:
        code = subprocess.run(command, stdout=stream, stderr=subprocess.STDOUT, check=False).returncode
    results = read_results(output)
    return {"command": command, "exit": code, "defects": defect_keys(results) if results is not None else None}


def record_finding(ledger, ledger_dir, key, run, batch, parameters, history):
    """Add a defect seen in a run to the ledger, keeping what re-executes it."""
    identifier = finding_id(key)
    row = ledger["findings"].get(identifier)
    if row is None:
        replay = Path(ledger_dir) / "findings" / identifier / "replay.jsonl"
        try:
            invokes = jc.replay_invokes(jc.load_jsonl(history))
        except (OSError, ValueError, KeyError):
            invokes = None
        if invokes is not None:
            replay.parent.mkdir(parents=True, exist_ok=True)
            replay.write_text("".join(json.dumps(op) + "\n" for op in invokes), encoding="utf-8")
        row = ledger["findings"][identifier] = {
            "id": identifier, "key": key, "class": key.split("|", 1)[0], "first_seen": {"batch": batch, "run": run},
            "parameters": parameters, "estimate_seconds": None, "replayable": invokes is not None,
            "attempts": 0, "reproductions": 0, "sightings": 0}
    row["sightings"] += 1
    return row


def update_elapsed(ledger, lane, seconds):
    rows = ledger.setdefault("elapsed", {}).setdefault(lane, [])
    rows.append(seconds)
    del rows[:-MEASURED_KEPT]


def lane_of(parameters):
    if parameters["stages"]:
        return "stages"
    if parameters["phase_transport"]:
        return "transport"
    return "live" if parameters["phase_live"] else "fixed"


def batch(args):
    out = Path(args.out).resolve()
    out.mkdir(parents=True, exist_ok=False)
    (out / "runs").mkdir()
    ledger = load_ledger(args.ledger)
    ledger["batches"] += 1
    batch_id = args.batch or str(ledger["batches"])
    paths = {"barn": args.barn, "barn_elle": args.barn_elle, "db": args.db, "jepsen_jar": args.jepsen_jar,
             "elle_jar": args.elle_jar, "java": args.java}
    entries = plan(ledger, args.budget, args.seed)
    started = time.monotonic()
    runs, deferred, first_witness = [], [], None
    for index, entry in enumerate(entries):
        remaining = args.budget - (time.monotonic() - started)
        if remaining < entry["estimate_seconds"]:
            deferred.append(entry)
            continue
        name = f"{index:02d}-{entry['kind']}" + (f"-{entry['finding']}" if entry["kind"] == "reexecute" else "")
        output = out / "runs" / name
        replay = None
        parameters = entry["parameters"]
        if entry["kind"] == "reexecute":
            replay = Path(args.ledger) / "findings" / entry["finding"] / "replay.jsonl"
        row = {"name": name, "kind": entry["kind"], "config": entry.get("config"), "finding": entry.get("finding"),
               "new_pairs": entry.get("new_pairs"), "parameters": parameters,
               **execute(parameters, paths, output, out / "runs" / f"{name}.log", replay)}
        row["evidence"] = f"runs/{name}"
        if row["defects"]:
            row["offline"] = offline_analysis(output, out / "runs" / f"{name}-analysis", paths,
                                              out / "runs" / f"{name}-analysis.log")
            row["offline"]["evidence"] = f"runs/{name}-analysis"
            if first_witness is None:
                first_witness = round(time.monotonic() - started, 1)
        if entry["kind"] == "reexecute":
            # Every attempt counts, so a finding that never re-executes cleanly
            # still leaves the schedule after REEXECUTION_ATTEMPTS batches.
            target = ledger["findings"][entry["finding"]]
            row["reproduced"] = target["key"] in row["defects"]
            target["attempts"] += 1
            target["reproductions"] += row["reproduced"]
        if row["evidence_retained"]:
            if entry["kind"] != "reexecute":
                ledger["covered"] = sorted(set(ledger["covered"]) | pairs_of(entry["config"]))
                if set(ledger["covered"]) >= ALL_PAIRS:
                    ledger["covered"], ledger["round"] = [], ledger["round"] + 1
            update_elapsed(ledger, lane_of(parameters), row["elapsed_seconds"])
        for key in row["defects"]:
            finding = record_finding(ledger, args.ledger, key, name, batch_id, parameters, output / "history.jsonl")
            if finding["estimate_seconds"] is None:
                finding["estimate_seconds"] = max(row["elapsed_seconds"], 1)
        runs.append(row)
    report = batch_report(batch_id, args, ledger, runs, deferred, first_witness, time.monotonic() - started)
    drafts(out, report, ledger)
    save_ledger(args.ledger, ledger)
    (out / "report.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    (out / "report.md").write_text(report_markdown(report), encoding="utf-8")
    print(report_markdown(report), flush=True)
    return {"clean": 0, "finding": 1, "incomplete": 4}[report["outcome"]]


def batch_report(batch_id, args, ledger, runs, deferred, first_witness, elapsed):
    incomplete = [row["name"] for row in runs
                  if not row["evidence_retained"] or row["verdict"] in INCOMPLETE_VERDICTS]
    defects = sorted({key for row in runs for key in row["defects"]})
    classes = {}
    for row in runs:
        for kind in row["classes"] or []:
            classes[kind] = classes.get(kind, 0) + 1
    completions = {"ok": 0, "fail": 0, "info": 0}
    for row in runs:
        for label, count in row["outcomes"].items():
            kind = label.split(":", 1)[0]
            if kind in completions:
                completions[kind] += count
    total = sum(completions.values())
    outcome = "finding" if defects else ("incomplete" if incomplete or not runs else "clean")
    by_key = {row["key"]: row for row in ledger["findings"].values()}
    return {
        "batch": batch_id, "budget_seconds": args.budget, "elapsed_seconds": round(elapsed, 1), "seed": args.seed,
        "outcome": outcome, "complete": bool(runs) and all(row["evidence_retained"] for row in runs),
        "runs": runs, "deferred": deferred,
        "yield": {
            "distinct_defects": defects,
            "reproducible_defects": sorted(key for key, row in by_key.items() if row["reproductions"]),
            "reexecutions": [{"finding": row["finding"], "reproduced": row["reproduced"]}
                             for row in runs if row["kind"] == "reexecute"],
            "fault_classes": classes,
            "time_to_first_witness_seconds": first_witness,
            "time_to_minimal_reproduction_seconds": None,
            "minimal_reproduction": "not measured: plan section 6 defers an execution reducer until a finding "
                                    "resists witness and artifact replay",
            "combinations": {"round": ledger["round"], "covered_pairs": len(ledger["covered"]),
                             "total_pairs": len(ALL_PAIRS),
                             "exercised": [row["config"] for row in runs if row["config"]]},
            "unknown_outcome_fraction": round(completions["info"] / total, 4) if total else None,
            "client_completions": completions,
            "incomplete_runs": incomplete,
            "incomplete_run_fraction": round(len(incomplete) / len(runs), 4) if runs else None,
            # The exploration plans no negative control; one would not count as a defect.
            "negative_controls": 0,
        },
    }


def drafts(out, report, ledger):
    """One draft finding record per defect this batch reported. Never posted."""
    by_key = {row["key"]: row for row in ledger["findings"].values()}
    directory = out / "drafts"
    for key in report["yield"]["distinct_defects"]:
        finding = by_key[key]
        runs = [row for row in report["runs"] if key in row["defects"]]
        record = {"status": "draft (not posted)", "id": finding["id"], "key": key, "class": finding["class"],
                  "first_seen": finding["first_seen"], "runs": [row["evidence"] for row in runs],
                  "offline_reanalysis": [{"run": row["evidence"], "reproduced": key in (row["offline"]["defects"] or [])}
                                         for row in runs if row.get("offline")],
                  "reexecution": {"attempts": finding["attempts"], "reproductions": finding["reproductions"],
                                  "replayable": finding["replayable"]},
                  "parameters": finding["parameters"],
                  "commands": [row["command"] for row in runs]}
        directory.mkdir(exist_ok=True)
        (directory / f"{finding['id']}.json").write_text(json.dumps(record, indent=2) + "\n", encoding="utf-8")


def report_markdown(report):
    rows = report["yield"]
    lines = [f"# Jepsen exploration batch {report['batch']}", "",
             f"Outcome: **{report['outcome']}** ({len(report['runs'])} runs, {len(report['deferred'])} deferred, "
             f"{report['elapsed_seconds']} s of {report['budget_seconds']} s).", "",
             "| run | kind | verdict | exit | seconds | defects |", "|---|---|---|---|---|---|"]
    for row in report["runs"]:
        lines.append(f"| {row['name']} | {row['kind']} | {row['verdict']} | {row['exit']} | {row['elapsed_seconds']} "
                     f"| {', '.join(row['defects']) or '-'} |")
    lines += ["", f"- Distinct defects: {rows['distinct_defects'] or 'none'}",
              f"- Reproducible defects (ledger): {rows['reproducible_defects'] or 'none'}",
              f"- Fault classes: {rows['fault_classes'] or 'none'}",
              f"- Time to first witness: {rows['time_to_first_witness_seconds']}",
              f"- Pair coverage: {rows['combinations']['covered_pairs']}/{rows['combinations']['total_pairs']} "
              f"(round {rows['combinations']['round']})",
              f"- Unknown-outcome fraction: {rows['unknown_outcome_fraction']}",
              f"- Incomplete-run fraction: {rows['incomplete_run_fraction']}", ""]
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Replay of one retained bundle

def replay_bundle(args):
    """Offline re-analysis of a retained run directory, then N fresh
    executions of its recorded transactions with its parameters and seed;
    prints and writes the reproduction frequency of its defects."""
    bundle = Path(args.bundle).resolve(strict=True)
    out = Path(args.out).resolve()
    out.mkdir(parents=True, exist_ok=False)
    original = read_results(bundle) or {}
    wanted = defect_keys(original)
    paths = {"barn": args.barn, "barn_elle": args.barn_elle, "db": args.db, "jepsen_jar": args.jepsen_jar,
             "elle_jar": args.elle_jar, "java": args.java}
    provenance = json.loads((bundle / "provenance.json").read_text(encoding="utf-8"))
    parameters = parameters_from_provenance(provenance)
    offline = offline_analysis(bundle, out / "offline", paths, out / "offline.log")
    executions = []
    for attempt in range(args.times):
        row = execute(parameters, paths, out / f"execution-{attempt}", out / f"execution-{attempt}.log",
                      bundle / "history.jsonl")
        executions.append(row)
    summary = {"bundle": str(bundle), "defects": wanted, "offline": offline["defects"],
               "executions": executions,
               "frequency": {key: sum(key in row["defects"] for row in executions) for key in wanted},
               "times": args.times}
    (out / "replay.json").write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    for key in wanted:
        print(f"{key}: offline {'reproduced' if key in (offline['defects'] or []) else 'NOT reproduced'}; "
              f"re-executed {summary['frequency'][key]}/{args.times}", flush=True)
    return 0


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    commands = parser.add_subparsers(dest="command", required=True)
    plan_parser = commands.add_parser("plan", help="print the batch plan for a ledger without running anything")
    plan_parser.add_argument("--ledger", required=True)
    plan_parser.add_argument("--budget", type=int, default=BUDGET_SECONDS)
    plan_parser.add_argument("--seed", type=int, default=1)
    for name, help_text in (("batch", "plan and run one bounded batch, then write its report and drafts"),
                            ("replay", "re-analyse one retained run directory and re-execute it N times")):
        sub = commands.add_parser(name, help=help_text)
        sub.add_argument("--out", required=True, help="new directory; an existing one is refused")
        sub.add_argument("--barn", type=Path, required=True)
        sub.add_argument("--barn-elle", type=Path, required=True, help="elle-tagged Barn for the stage lane")
        sub.add_argument("--db", type=Path, required=True)
        sub.add_argument("--jepsen-jar", type=Path, required=True)
        sub.add_argument("--elle-jar", type=Path, required=True)
        sub.add_argument("--java", default="java")
        if name == "batch":
            sub.add_argument("--ledger", required=True, help="ledger directory carried between batches")
            sub.add_argument("--budget", type=int, default=BUDGET_SECONDS)
            sub.add_argument("--seed", type=int, default=1)
            sub.add_argument("--batch", help="batch name for the report (default: the ledger's batch count)")
        else:
            sub.add_argument("--bundle", required=True, help="a retained jepsen_checkpoint.py run directory")
            sub.add_argument("--times", type=int, default=3)
    args = parser.parse_args(argv)
    if args.command == "plan":
        print(json.dumps(plan(load_ledger(args.ledger), args.budget, args.seed), indent=2))
        return 0
    return batch(args) if args.command == "batch" else replay_bundle(args)


if __name__ == "__main__":
    sys.exit(main())
