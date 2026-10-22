#!/usr/bin/env python3
"""Validate a Toast moo_cpu_profile JSON export; write CPU tables/folded stacks.

Input is generate_json(moo_cpu_profile("stop")) from the instrumented server,
or a fixture record with that object under "profile". Never infer percentages
from the passive sample_live_moo observation counts.
"""
from __future__ import annotations

import argparse
from collections import Counter
import html
import json
import math
from pathlib import Path
import sys


CATEGORIES = ("runtime_samples", "transition_samples", "profiler_samples",
              "unknown_samples", "moo_self_samples", "builtin_self_samples")
LOSSES = ("dropped", "overruns", "unobserved_periods")


def integer(record, key):
    value = record[key]
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise ValueError(f"{key} must be a nonnegative integer")
    return value


def validate(payload):
    profile = payload.get("profile", payload)
    if profile.get("schema_version") != 1:
        raise ValueError("unsupported CPU profile schema")
    if profile.get("clock") != "main-thread CPU" or "CPU timer" not in profile.get("sampling", ""):
        raise ValueError("not a native main-thread CPU sampler export")
    recorded = integer(profile, "recorded")
    denominator = recorded + sum(integer(profile, key) for key in LOSSES)
    if integer(profile, "denominator_samples") != denominator:
        raise ValueError("CPU denominator omits or duplicates missing periods")
    if sum(integer(profile, key) for key in CATEGORIES) != recorded:
        raise ValueError("recorded sample categories do not reconcile")
    if not denominator or profile.get("sampling_valid") != 1:
        raise ValueError("capture has no usable CPU periods or is marked invalid")
    seconds = profile["main_cpu_seconds"]
    if not isinstance(seconds, (int, float)) or not math.isfinite(seconds) or seconds < 0:
        raise ValueError("invalid main-thread CPU duration")
    start, stop = integer(profile, "start_cpu_ns"), integer(profile, "stop_cpu_ns")
    period, phase = integer(profile, "period_ns"), integer(profile, "initial_phase_ns")
    if stop < start or not period or not 0 < phase <= period:
        raise ValueError("invalid CPU clock boundaries or sampling phase")
    if integer(profile, "first_expiration_cpu_ns") != start + phase:
        raise ValueError("first expiration disagrees with CPU clock and phase")
    elapsed = stop - start
    expected = 0 if elapsed < phase else 1 + (elapsed - phase) // period
    if integer(profile, "expected_periods") != expected or denominator < expected:
        raise ValueError("CPU denominator does not cover clock-derived expected periods")
    if abs(seconds - elapsed / 1e9) > 1e-6:
        raise ValueError("CPU duration disagrees with clock boundaries")
    metadata_mode = profile.get("metadata_mode", "detailed")
    if metadata_mode not in ("detailed", "definitions"):
        raise ValueError("unsupported profiler metadata mode")
    if metadata_mode == "definitions":
        if profile.get("context_attribution") != "unavailable" or profile.get("tasks") != []:
            raise ValueError("definitions mode must expose unavailable task context")
        if any(any(key in row for key in ("receiver", "player", "programmer"))
               for row in profile["frame_dictionary"]) or any(
            "task_id" in row for row in profile["stack_dictionary"]
        ):
            raise ValueError("definitions mode contains misleading runtime context")
    for name, count_name in (("verbs", "moo_self_samples"), ("builtins", "builtin_self_samples")):
        rows = profile[name]
        if not isinstance(rows, list):
            raise ValueError(f"{name} must preserve JSON arrays")
        if sum(integer(row, "self_samples") for row in rows) != profile[count_name]:
            raise ValueError(f"{name} self counts do not reconcile")
        for row in rows:
            own, inclusive = integer(row, "self_samples"), integer(row, "inclusive_samples")
            if not own <= inclusive <= recorded:
                raise ValueError(f"invalid {name} inclusive sample count")
            for key, count in (("self_cpu_share", own), ("inclusive_cpu_share", inclusive)):
                value = row[key]
                if not isinstance(value, (int, float)) or not math.isfinite(value) or abs(value - count / denominator) > 1e-6:
                    raise ValueError(f"{name} {key} disagrees with complete denominator")
    return profile


def label(value):
    return html.escape(str(value)).replace("|", "\\|").replace("\n", "\\n").replace("\r", "\\r")


def table(rows, denominator, native=False, limit=20):
    result = ["| Builtin | Self CPU | Samples |" if native else
              "| MOO verb | Generation | Vector | Self CPU | Inclusive CPU | Self samples |",
              "|---|---:|---:|" if native else "|---|---:|---:|---:|---:|---:|"]
    ordered = sorted(rows, key=lambda r: (r["self_samples"], r["inclusive_samples"]), reverse=True)
    for row in ordered[:limit]:
        if native:
            result.append(f"| {label(row['builtin'])} | {100 * row['self_samples'] / denominator:.2f}% | {row['self_samples']} |")
        else:
            result.append(f"| {label(row['definer'])}:{label(row['verb'])} | {row['program_generation']} | "
                          f"{label(row.get('vector', 'unreported'))} | "
                          f"{100 * row['self_samples'] / denominator:.2f}% | "
                          f"{100 * row['inclusive_samples'] / denominator:.2f}% | {row['self_samples']} |")
    return "\n".join(result)


def root_table(profile, limit=20):
    frames = {row["frame_id"]: row for row in profile["frame_dictionary"]}
    counts = Counter()
    for state in profile["stack_dictionary"]:
        if not state["frame_ids"] or state["frame_ids"][0] not in frames:
            raise ValueError("sampled stack is missing its root frame")
        root = frames[state["frame_ids"][0]]
        counts[(root["definer"], root["verb"], root["program_generation"])] += integer(state, "samples")
    if sum(counts.values()) != profile["moo_self_samples"] + profile["builtin_self_samples"]:
        raise ValueError("root paths omit semantic sample weights")
    result = ["| Root verb | Generation | Attributed CPU | Samples |", "|---|---:|---:|---:|"]
    for (definer, verb, generation), count in counts.most_common(limit):
        result.append(f"| {label(definer)}:{label(verb)} | {generation} | "
                      f"{100 * count / profile['denominator_samples']:.2f}% | {count} |")
    return "\n".join(result)


def line_table(profile, limit=20):
    rows = sorted(profile.get("lines", []), key=lambda row: row["self_samples"], reverse=True)
    if not rows:
        return "No source-line observations exported."
    result = ["| MOO verb | Generation | Vector | Canonical line | Self CPU | Samples |",
              "|---|---:|---:|---:|---:|---:|"]
    for row in rows[:limit]:
        count = integer(row, "self_samples")
        if count:
            result.append(f"| {label(row['definer'])}:{label(row['verb'])} | {row['program_generation']} | "
                          f"{label(row.get('vector', 'unreported'))} | "
                          f"{row['line']} | {100 * count / profile['denominator_samples']:.2f}% | {count} |")
    return "\n".join(result)


def markdown(profile):
    denominator = profile["denominator_samples"]
    unknown = profile["unknown_samples"] + sum(profile[key] for key in LOSSES)
    text = ["# MOO main-thread CPU profile", "",
            f"Measured CPU: {profile['main_cpu_seconds']:.6f}s. "
            f"CPU sample period: {profile['period_ns'] / 1e6:.3f}ms. "
            f"Accounted periods: {denominator}.", "",
            "Shares are sampling estimates. Their denominator includes every recorded and missing period. "
            "Inclusive rows overlap; worker CPU and wall latency are separate metrics.", "",
            f"Metadata mode: {label(profile.get('metadata_mode', 'detailed (legacy)'))}. "
            f"Context attribution: {label(profile.get('context_attribution', 'available (legacy)'))}. "
            f"Identity scope: {label(profile.get('identity_scope', 'retained frame and task identities'))}.", "",
            "Unknown periods can contain omitted verbs. Selective losses can make rankings incomplete; "
            "reported fractions describe attributed observations of the instrumented run.", "",
            f"Unknown or missing: {unknown}/{denominator} ({100 * unknown / denominator:.2f}%). "
            f"Transition: {100 * profile['transition_samples'] / denominator:.2f}%. "
            f"Metadata/control: {100 * profile['profiler_samples'] / denominator:.2f}%. "
            f"Outside interpreter: {100 * profile['runtime_samples'] / denominator:.2f}%.", "",
            "## MOO verbs", "", table(profile["verbs"], denominator), "",
            "## Native builtins", "", table(profile["builtins"], denominator, native=True), "",
            "## Attributed root paths", "",
            "Each semantic sample contributes once to its outermost captured MOO verb, including builtin work. "
            "Contexts, aliases and vectors are aggregated within the same definition generation. "
            "Unknown, transition and profiler weights remain unassigned in the full CPU denominator.", "",
            root_table(profile), "",
            "## Source-line self CPU", "",
            "Canonical unparsed lines belong to retained Program generations. Vector -1 is main; nonnegative vectors are forks. "
            "Line 0 means unresolved; "
            "builtin self CPU is separate. Resolve source from the matching checkpoint.", "",
            line_table(profile), "",
            "## Collection limits", "",
            f"Recorded={profile['recorded']}; buffer drops={profile['dropped']}; "
            f"timer overruns={profile['overruns']}; unobserved expirations={profile['unobserved_periods']}.", "",
            f"Late deliveries={profile.get('late_deliveries', 'unknown')}; "
            f"lateness threshold={profile.get('lateness_threshold_ns', 'unknown')}ns; "
            f"maximum delivery lag={profile.get('max_delivery_lag_ns', 'unknown')}ns. "
            "Late observations remain in the unknown bucket.", "",
            f"CPU prefix={profile.get('prefix_cpu_ns', 'unknown')}ns; "
            f"CPU tail={profile.get('tail_cpu_ns', 'unknown')}ns. "
            "These partial intervals are reported separately from full-period sample weights.", "",
            "The metadata/control percentage measures published profiler states. "
            "It excludes the sampler handler and export; total observer overhead needs a paired benchmark. "
            "Structural dictionary budgets exclude transient export peak memory.", "",
            f"State slots={profile.get('states', 'unknown')}/{profile.get('max_states', 'unknown')}; "
            f"active states={profile.get('active_states', 'not reported')}; "
            f"frames={profile.get('frames', 'unknown')}/{profile.get('max_frames', 'unknown')}. "
            f"Budgeted metadata={profile.get('metadata_bytes_budgeted', 'unknown')}/"
            f"{profile.get('max_metadata_bytes', 'unknown')} bytes; "
            f"retained Program estimate={profile.get('retained_program_bytes', 'unknown')} bytes.", "",
            f"State retirements={profile.get('state_retirements', 'not reported')}; "
            f"capacity rejections={profile.get('state_capacity_rejections', 'not reported')}; "
            f"dictionary rejections={profile.get('dictionary_rejections', 'not reported')}; "
            f"eviction probes={profile.get('state_eviction_probes', 'not reported')}.", "",
            f"State retention: {label(profile.get('state_retention', 'not reported'))}. "
            "Slot occupancy alone does not establish quota loss.", "",
            f"User-signal dispatch: {label(profile.get('user_signal_dispatch', 'not reported'))}. "
            f"Deferred events={profile.get('deferred_user_signal_events', 'not reported')}.", "",
            f"Export-budget omissions={profile.get('export_omitted_samples', 'unknown')}; "
            f"retained semantic sites={profile.get('retained_semantic_location_buckets', 'unknown')}; "
            f"semantic frame work={profile.get('semantic_frame_observations_processed', 'unknown')}. "
            "Omitted weights remain unknown in the full CPU denominator.", "",
            "Program generation IDs are local to this session. Native builtin self CPU remains separate "
            "from MOO self CPU and contributes to the enclosing MOO verb's inclusive CPU.", "",
            f"Source resolution: {label(profile.get('source_lines', 'unknown'))}", "",
            f"Export wall time: {profile.get('export_wall_seconds', 'unknown')}s. "
            "This is additional collection work after the sampling interval.", ""]
    return "\n".join(text)


def folded(profile):
    frames = {row["frame_id"]: row for row in profile["frame_dictionary"]}
    builtins = {row["function_id"]: row["builtin"] for row in profile["builtins"]}
    counts = Counter()
    for state in profile["stack_dictionary"]:
        path = []
        if "caller_pcs" in state:
            pcs = state["caller_pcs"]
            if not isinstance(pcs, list) or len(pcs) != len(state["frame_ids"]) or any(
                isinstance(pc, bool) or not isinstance(pc, int) or pc < 0 for pc in pcs
            ):
                raise ValueError("stack caller PCs do not match semantic frames")
        for frame_id in state["frame_ids"]:
            row = frames[frame_id]
            # Folded-stack separators/newlines are escaped for a text renderer.
            name = f"{row['definer']}:{row['verb']} [g{row['program_generation']}]"
            path.append(name.replace(";", "；").replace("\n", "\\n").replace("\r", "\\r"))
        if state["builtin_id"] >= 0:
            name = "builtin:" + builtins[state["builtin_id"]]
            path.append(name.replace(";", "；").replace("\n", "\\n").replace("\r", "\\r"))
        if not path:
            raise ValueError("sampled stack is missing its semantic frames")
        counts[";".join(path)] += integer(state, "samples")
    expected = profile["moo_self_samples"] + profile["builtin_self_samples"]
    if sum(counts.values()) != expected:
        raise ValueError("stack dictionary omits semantic sample weights")
    for name, key in (("[outside interpreter]", "runtime_samples"),
                      ("[transition]", "transition_samples"), ("[profiler]", "profiler_samples")):
        counts[name] += profile[key]
    counts["[unknown or missing]"] += profile["unknown_samples"] + sum(profile[key] for key in LOSSES)
    if sum(counts.values()) != profile["denominator_samples"]:
        raise ValueError("folded stacks do not account for all CPU periods")
    return "\n".join(f"{name} {count}" for name, count in sorted(counts.items()) if count) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--folded", type=Path)
    args = parser.parse_args()
    try:
        profile = validate(json.loads(args.input.read_text(encoding="utf-8")))
        report = markdown(profile)
        stacks = folded(profile) if args.folded else None
        for target in (args.out, args.folded):
            if target is not None and target.exists():
                raise FileExistsError(f"refusing to overwrite {target}")
        with args.out.open("x", encoding="utf-8") as output:
            output.write(report)
        if args.folded:
            with args.folded.open("x", encoding="utf-8") as output:
                output.write(stacks)
        print(json.dumps({"report": str(args.out.resolve()), "accounted_periods": profile["denominator_samples"],
                          "unknown_or_missing_periods": profile["unknown_samples"] + sum(profile[k] for k in LOSSES)}))
        return 0
    except (OSError, ValueError, KeyError, TypeError) as exc:
        print(f"cannot report CPU shares: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
