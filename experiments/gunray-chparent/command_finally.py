"""Bounded parsed-command and nested finalizer experiments."""

import csv
import json
import sys
from itertools import product
from pathlib import Path

import yaml

from parent_order import predict
from vm_cases import capture, create, evaluate, finish, install, mutation, ref


MATCH = [
    "eligible(X) :- spec(X,P), typed(P).",
    'eligible(X) :- spec(X,"any").',
]


def command_case(deep_prep, right_prep, typed):
    graph = {"deep": [], "left": ["deep"], "right": [], "leaf": ["left", "right"]}
    steps = [create("deep"), create("left", "deep"), create("right"),
             {"run": "create({{left}, {right}})", "capture": "leaf", "expect": {"type": "obj"}},
             mutation('add_property({leaf}, "seen", {}, {player, "rw"})')]
    specs = {"deep": deep_prep, "right": right_prep}
    for obj, prep in specs.items():
        steps.append({"verb_setup": {"object": ref(obj), "name": "gunraycmdprobe",
                      "args": ["this", prep, "any"],
                      "code": f'this.seen = {{@this.seen, {{"{obj}", this == dobj}}}}; notify(player, "command done");'}})
    command = "gunraycmdprobe {leaf}" + (" to {leaf}" if typed == "to" else "")
    results, models = {}, []
    eligible = evaluate({"spec": set(specs.items()), "typed": {(typed,)}}, MATCH, models)["eligible"]
    for phase, order in [("forward", ["left", "right"]), ("reversed", ["right", "left"])]:
        if phase == "reversed":
            steps.append(mutation("chparents({leaf}, {{right}, {left}})"))
        graph["leaf"] = order
        winner = predict(graph, [x for x, in eligible], models)
        steps.extend([{"command": command}, capture(phase, "return {leaf}.seen;")])
        prior = results.get("forward", [])
        results[phase] = [*prior, [*winner, 1]]
    return finish(f"gunray_cf_cmd_{deep_prep}_{right_prep}_{typed}", steps, results, models, graph)


UNWIND = [
    "exits(X) :- first(X).",
    "exits(Y) :- exits(X), next(X,Y).",
    "pending(X,K,V) :- first(X), action(X,K,V).",
    "pending(Y,K,V) :- exits(X), next(X,Y), action(Y,K,V).",
    "pending(Y,K,V) :- pending(X,K,V), next(X,Y), normal(Y).",
    "result(K,V) :- last(X), pending(X,K,V).",
]


def transfer(kind, label):
    if kind == "raise":
        return f'raise(E_INVARG, "{label}");'
    if kind == "return":
        return f'return "{label}";'
    return ""


def finally_case(initial, inner, outer, pause):
    actions = {"body": initial, "inner": inner, "outer": outer}
    facts = {"first": {("body",)}, "last": {("outer",)},
             "next": {("body", "inner"), ("inner", "outer")},
             "normal": {(s,) for s, k in actions.items() if k == "normal"},
             "action": {(s, k, s) for s, k in actions.items() if k != "normal"}}
    models = []
    rows = evaluate(facts, UNWIND, models)
    [(kind, label)] = rows["result"]
    trace = []
    body = ""
    for site, action in actions.items():
        mark = f'this.seen = {{@this.seen, "{site}"}};'
        code = mark
        if (site,) in rows["exits"]:
            trace.append(site)
        if pause == site:
            code += f' suspend(0); this.seen = {{@this.seen, "{site}_resumed"}};'
            trace.append(site + "_resumed")
        code += " " + transfer(action, site)
        body = code if site == "body" else f"try {body} finally {code} endtry"
    steps = [create("driver"), mutation('add_property({driver}, "seen", {}, {player, "rw"})'),
             *install("driver", "probe", body),
             capture("outcome", 'try v = {driver}:probe(); result = {"return", v}; '
                     'except e (ANY) result = {"raise", toliteral(e[1]), e[2]}; endtry '
                     'return {result, {driver}.seen};')]
    result = ["return", label] if kind == "return" else ["raise", "E_INVARG", label]
    return finish(f"gunray_cf_finally_{initial}_{inner}_{outer}_{pause}", steps,
                  {"outcome": [result, trace]}, models, ["driver"])


def main(run):
    cases = [command_case(a, b, typed) for a, b in [("any", "any"), ("none", "to"), ("to", "none")]
             for typed in ["none", "to"]]
    cases += [finally_case(*args) for args in product(
        ("return", "raise"), ("normal", "return", "raise"), ("normal", "return", "raise"),
        ("uninterrupted", "inner", "outer"))]
    suite = {"name": "gunray_cf", "description": "Parsed dispatch and nested cleanup transfers",
             "requires": {"config": ["managed_server"]}, "tests": [c for c, _ in cases]}
    (run / "candidate/cases/gunray_cf.yaml").write_text(yaml.safe_dump(suite, sort_keys=False))
    (run / "evidence/predictions.json").write_text(json.dumps(
        {"cases": {c["name"]: p for c, p in cases}}, indent=2) + "\n")
    print(f"{len(cases)} histories")


def compare_histories(evidence):
    rows = []
    for engine in ("toast", "current"):
        path = evidence / f"model-{engine}" / "observations.jsonl"
        records = [json.loads(line) for line in path.read_text().splitlines()]
        actual = {r["case"]: r["result"]["value"] for r in records}
        for key, value in actual.items():
            if key.startswith("gunray_cf_cmd_"):
                first, both = value
                same_winner = not key.startswith("gunray_cf_cmd_any_any_")
                holds = (len(first) == 1 and len(both) == 2 and both[:1] == first
                         and (first[0] == both[1]) == same_winner)
                rows.append((engine, key, "reverse parents", holds))
            elif key.endswith("_uninterrupted"):
                [(outcome, trace)] = value
                for site in ("inner", "outer"):
                    [(other_outcome, other_trace)] = actual[key.removesuffix("uninterrupted") + site]
                    insertion = trace.index(site) + 1
                    expected_trace = [*trace[:insertion], site + "_resumed", *trace[insertion:]]
                    rows.append((engine, key, "suspend in " + site,
                                 outcome == other_outcome and other_trace == expected_trace))
    with (evidence / "relations.csv").open("w", newline="") as stream:
        writer = csv.writer(stream)
        writer.writerow(["engine", "case", "transformation", "holds"])
        writer.writerows(rows)
    for engine in ("toast", "current"):
        for family in ("cmd", "finally"):
            checks = [ok for e, key, _, ok in rows if e == engine and key.startswith(f"gunray_cf_{family}_")]
            print(f"{engine} {family}: {sum(checks)}/{len(checks)} paired checks")


if __name__ == "__main__":
    if sys.argv[1:2] == ["--compare"]:
        compare_histories(Path(sys.argv[2]))
    else:
        main(Path(sys.argv[1]))
