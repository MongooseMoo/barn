"""Predict inherited reads across A -> B -> A and vary only task boundaries."""

import json
import sys
from itertools import product
from pathlib import Path

import yaml
from gunray import GunrayEvaluator, Program


PARENTS = {"root": None, "a": "root", "b": "root", "moved": "a",
           "inherited": "moved", "assigned": "moved"}
WRITES = {"a": 11, "b": 22, "assigned": 73}
CHILDREN = ("inherited", "assigned")
STATES = (("initial", "a"), ("away", "b"), ("back", "a"))
RULES = [
    "has_write(X) :- stored(X, V).",
    "resolved(T, X, V) :- state(T), stored(X, V).",
    "resolved(T, X, V) :- parent(T, X, P), resolved(T, P, V), not has_write(X).",
    "pair(T, X, V, C) :- resolved(T, X, V), has_write(X), nonclear(C).",
    "pair(T, X, V, C) :- resolved(T, X, V), not has_write(X), clear(C).",
]


def predict():
    facts = {
        "state": {(state,) for state, _ in STATES}, "clear": {(1,)}, "nonclear": {(0,)},
        "stored": {("root", 1), *WRITES.items()},
        "parent": {(state, x, parent if x == "moved" else p)
                   for state, parent in STATES for x, p in PARENTS.items() if p is not None},
    }
    result = GunrayEvaluator().evaluate(Program(facts=facts, rules=RULES))
    expected, labels = [], []
    for state, _ in STATES:
        for child in CHILDREN:
            [pair] = [[v, c] for t, x, v, c in result.facts["pair"] if (t, x) == (state, child)]
            expected.append(pair)
            labels.append(f"{state}/{child}")
    return {"expected": expected, "labels": labels}, {k: sorted(v) for k, v in facts.items()}


def ref(x):
    return "{" + x + "}"


def make_case(cuts, prediction):
    name = "gunray_roundtrip_" + "".join(map(str, cuts))
    steps = []
    for x, parent in PARENTS.items():
        parent_ref = ref(parent) if parent else "$nothing"
        steps.append({"run": f"create({parent_ref})", "capture": x})
        if parent is None:
            steps.append({"run": f'add_property({ref(x)}, "q", 1, {{player, "rc"}})'})

    payload = ["{" + ref(x) + f'.q, is_clear_property({ref(x)}, "q")' + "}" for x in CHILDREN]
    read = "seen = {@seen, " + ", ".join(payload) + "};"
    writes = " ".join(f"{ref(x)}.q = {v};" for x, v in WRITES.items())
    ops = [writes, read]
    for _, target in STATES[1:]:
        ops.extend([f"chparent({ref('moved')}, {ref(target)});", read])

    # Each 1 commits this task. Carry only the previously observed pairs into the next task.
    group = ["seen = {};"]
    for i, op in enumerate(ops):
        group.append(op)
        if i < len(cuts) and cuts[i]:
            steps.append({"run": " ".join([*group, "return seen;"]), "capture": "prior"})
            group = ["seen = {prior};"]
    steps.append({"run": " ".join([*group, f'case_id = "{name}"; return seen;']),
                  "expect": {"value": prediction["expected"]}})
    return {"name": name, "permission": "wizard", "steps": steps,
            "cleanup": [{"run": f"recycle({ref(x)})"} for x in reversed(PARENTS)]}


def main(run):
    prediction, facts = predict()
    cases = [make_case(cuts, prediction) for cuts in product((0, 1), repeat=5)]
    (run / "candidate/cases/gunray_roundtrip.yaml").write_text(yaml.safe_dump(
        {"name": "gunray_roundtrip", "description": "Read both children across two reparents",
         "tests": cases}, sort_keys=False))
    (run / "evidence/predictions.json").write_text(json.dumps(
        {"rules": RULES, "facts": facts, "cases": {c["name"]: prediction for c in cases}},
        indent=2) + "\n")
    print(f"{len(cases)} task partitions; predicted reads: {prediction['expected']}")


if __name__ == "__main__":
    main(Path(sys.argv[1]))
