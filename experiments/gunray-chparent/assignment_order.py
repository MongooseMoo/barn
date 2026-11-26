"""Trace nested index/range assignment and compare local and property destinations."""

import csv
import json
import sys
from itertools import product
from pathlib import Path

import yaml
from gunray import evaluate_program_file

from vm_cases import capture, create, finish, install, mutation


ORIGINAL = [[10, 20, 30], [99]]
VISIBLE = {"original": ORIGINAL, "updated": [[10, 77, 30], [99]], "intervening": [[91], [92]]}
MARK = (
    'tag = args[1]; this.events = {@this.events, tag}; '
    'if (args[4]) suspend(0); this.events = {@this.events, tag + "_resumed"}; endif '
    'if (tag == "rhs" && args[5]) this.box = {{91}, {92}}; endif '
    'if (tag == args[3]) raise(E_INVARG, tag); endif return args[2];'
)


def probe(target, form, pause, fault, stages, expressions, models):
    is_prop = target != "local"
    overwrite = target == "reentrant"
    wrong = {"missing_property": ("read_property", "E_PROPNF"),
             "missing_child": ("read_child", "E_RANGE"), "bad_leaf": ("apply", "E_RANGE")}
    facts = {"first": [[stages[0]]], "last": [["store"]],
             "next": list(zip(stages, stages[1:])), "expression": list(zip(expressions, range(len(expressions))))}
    if fault != "success":
        loc, error = wrong.get(fault, (fault, "E_INVARG"))
        facts.update(fault=[[loc]], throws=[[loc, error]])
    if overwrite:
        facts["writes_target"] = [["rhs"]]
    models[fault] = facts
    rows = evaluate_program_file(Path(__file__).with_name("assignment-order.gnr"), facts=facts).facts
    trace = []
    for _, tag in sorted(rows["event"]):
        trace.append(tag)
        if pause:
            trace.append(tag + "_resumed")
    [(visible,)] = rows["visible"]
    errors = rows.get("error", set())
    if errors:
        [(code,)] = errors
        status = ["error", code]
    else:
        status = ["ok", 77 if form == "index" else [77]]

    def mark(tag, value):
        return f'd:mark("{tag}", {value}, "{fault}", {int(pause)}, {int(overwrite)})'

    name = '"absent_slot"' if fault == "missing_property" else '"box"'
    root = mark("base", "d") + ".(" + mark("name", name) + ")" if is_prop else "localbox"
    child = mark("child", "9" if fault == "missing_child" else "1")
    start = mark("start", "9" if fault == "bad_leaf" else "2")
    end = ".." + mark("end", "2") if form == "range" else ""
    rhs = mark("rhs", "{77}" if form == "range" else "77")
    readback = "d.box" if is_prop else "localbox"
    body = ('d = {driver}; saved = {{10, 20, 30}, {99}}; localbox = saved; d.box = saved; '
            'd.events = {}; try ' + f'v = ({root}[{child}][{start}{end}] = {rhs}); '
            'result = {"ok", v}; except e (ANY) result = {"error", toliteral(e[1])}; endtry '
            + 'return {result, d.events, ' + readback + ', saved};')
    return capture(fault, body), [status, trace, VISIBLE[visible], ORIGINAL]


def history(target, form, pause):
    stages = [] if target == "local" else ["base", "name", "read_property"]
    stages += ["child", "read_child", "start"]
    if form == "range":
        stages.append("end")
    stages += ["rhs", "apply", "store"]
    expressions = [s for s in stages if s not in ("read_property", "read_child", "apply", "store")]
    faults = ["success", *expressions, "missing_child", "bad_leaf"]
    if target != "local":
        faults.append("missing_property")
    steps = [create("driver"), mutation('add_property({driver}, "events", {}, {player, "rw"})'),
             mutation('add_property({driver}, "box", {}, {player, "rw"})'),
             *install("driver", "mark", MARK)]
    expected, models = {}, {}
    for fault in faults:
        step, expected[fault] = probe(target, form, pause, fault, stages, expressions, models)
        steps.append(step)
    name = f"gunray_assignment_order_{target}_{form}_" + ("pause" if pause else "plain")
    return finish(name, steps, expected, models, ["driver"])


def main(run):
    histories = [history(*args) for args in product(("local", "property", "reentrant"),
                                                    ("index", "range"), (False, True))]
    suite = {"name": "gunray_assignment_order", "description": "Observable order of nested assignments",
             "requires": {"config": ["managed_server"]}, "tests": [c for c, _ in histories]}
    (run / "candidate/cases/gunray_assignment_order.yaml").write_text(yaml.safe_dump(suite, sort_keys=False))
    (run / "evidence/predictions.json").write_text(json.dumps(
        {"cases": {c["name"]: p for c, p in histories}}, indent=2) + "\n")
    print(f"{len(histories)} histories; {sum(len(p['labels']) for _, p in histories)} observations")


def compare_pauses(evidence):
    cases = json.loads((evidence / "predictions.json").read_text())["cases"]
    verdicts = []
    for engine in ("toast", "current"):
        lines = (evidence / f"model-{engine}" / "observations.jsonl").read_text().splitlines()
        actual = {r["case"]: r["result"]["value"] for r in map(json.loads, lines)}
        held = []
        for name, case in cases.items():
            if not name.endswith("_plain"):
                continue
            paused = actual[name.removesuffix("_plain") + "_pause"]
            for label, plain, resumed in zip(case["labels"], actual[name], paused, strict=True):
                status, events, value, saved = plain
                with_resumes = [tag for event in events for tag in (event, event + "_resumed")]
                same = resumed == [status, with_resumes, value, saved]
                held.append(same)
                verdicts.append((engine, name, label, same))
        print(f"{engine}: {sum(held)}/{len(held)} pause pairs")
    with (evidence / "pause-pairs.csv").open("w", newline="") as stream:
        writer = csv.writer(stream)
        writer.writerow(["engine", "plain_history", "probe", "holds"])
        writer.writerows(verdicts)
    if not all(row[-1] for row in verdicts):
        raise ValueError("See pause-pairs.csv")


if __name__ == "__main__":
    if sys.argv[1] == "--compare":
        compare_pauses(Path(sys.argv[2]))
    else:
        main(Path(sys.argv[1]))
