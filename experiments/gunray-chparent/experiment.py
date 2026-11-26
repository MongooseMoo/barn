"""Emit a finite #499 conformance suite and its independent Datalog predictions."""

import json
import sys
from itertools import product
from pathlib import Path

import yaml
from gunray import GunrayEvaluator, Program


RULES = [
    "reachable(T, X, X) :- snapshot(T), object(X).",
    "reachable(T, X, Z) :- edge(T, X, Y), reachable(T, Y, Z).",
    "kept(X, D) :- old(O), new(N), reachable(O, X, D), reachable(N, X, D), defines(D, V).",
    "explicit(X, V) :- kept(X, D), wrote(X, V).",
    "explicit(X, V) :- defines(X, V).",
    "has_explicit(X) :- explicit(X, V).",
    "prediction(X, V, C) :- explicit(X, V), nonclear(C).",
    "prediction(X, V, C) :- new(N), reachable(N, X, D), defines(D, V), clear(C), not has_explicit(X).",
]


def fixture(name, destination, descendant=False, second=False, same=False):
    # Labels also determine the read order. Include the spare parent in every read.
    parents = {"a": [], "b": ["a"], "d": [] if destination == "replace" else ["a"], "c": ["b"]}
    if descendant:
        parents["y"] = ["c", "b"] if second else ["c"]
    definitions = {"a": 1}
    if destination == "replace":
        definitions["d"] = 2
    writes = {"c": 42}
    if descendant:
        writes["y"] = 7
    final = dict(parents)
    if destination != "stay":
        final["c"] = ["d"]
    facts = {
        "snapshot": {("old",), ("new",)}, "old": {("old",)}, "new": {("new",)},
        "object": {(x,) for x in parents}, "defines": set(definitions.items()),
        "wrote": set(writes.items()), "nonclear": {(0,)}, "clear": {(1,)},
        "edge": {(t, x, p) for t, graph in [("old", parents), ("new", final)]
                 for x, edges in graph.items() for p in edges},
    }
    model = GunrayEvaluator().evaluate(Program(facts=facts, rules=RULES))
    rows = model.facts["prediction"]
    expected = []
    for label in parents:
        [(value, clear)] = [(v, c) for x, v, c in rows if x == label]
        expected.append([value, clear])

    def ref(label):
        return "{" + label + "}"

    steps = []
    for x, edges in parents.items():
        parent = "$nothing"
        if len(edges) == 1:
            parent = ref(edges[0])
        elif edges:
            parent = "{" + ", ".join(map(ref, edges)) + "}"
        steps.append({"run": f"create({parent})", "capture": x})
        if x in definitions:
            steps.append({"run": f'add_property({ref(x)}, "q", {definitions[x]}, {{player, "rc"}})'})
    actions = [f"{ref(x)}.q = {value}" for x, value in writes.items()]
    if destination != "stay":
        actions.append("chparent({c}, {d})")
    pairs = ["{" + ref(x) + '.q, is_clear_property(' + ref(x) + ', "q")}' for x in parents]
    vector = "{" + ", ".join(pairs) + "}"
    read = f'case_id = "{name}"; return {vector};'
    if same:
        read = "; ".join(actions) + "; " + read
    else:
        steps.extend({"run": action} for action in actions)
    steps.append({"run": read, "expect": {"value": expected}})
    case = {"name": name, "permission": "wizard", "steps": steps,
            "cleanup": [{"run": f"recycle({ref(x)})"} for x in reversed(parents)]}
    prediction = {"labels": list(parents), "expected": expected,
                  "facts": {k: sorted(v) for k, v in facts.items()}}
    return case, prediction


def main(run):
    trials = [fixture("gunray_499_" + mode, mode) for mode in ("keep", "replace", "stay")]
    for second, mode, same in product((False, True), ("keep", "replace"), (False, True)):
        if second and mode == "replace":
            continue  # y would inherit q from two distinct definitions.
        name = f"gunray_499_{mode}_{'diamond' if second else 'chain'}_{'same' if same else 'separate'}"
        trials.append(fixture(name, mode, descendant=True, second=second, same=same))
    cases = [case for case, _ in trials]
    predictions = {case["name"]: result for case, result in trials}
    (run / "candidate/cases/gunray_499.yaml").write_text(
        yaml.safe_dump({"name": "gunray_499", "description": "Inherited q after reparenting",
                        "tests": cases}, sort_keys=False))
    (run / "evidence/predictions.json").write_text(json.dumps(
        {"rules": RULES, "cases": predictions}, indent=2) + "\n")
    print(f"{len(cases)} cases, {sum(len(v['labels']) for v in predictions.values())} slots")


if __name__ == "__main__":
    main(Path(sys.argv[1]))
