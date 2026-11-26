"""Distinguish ordered-parent dispatch from nearest-ancestor dispatch."""

import json
import sys
from itertools import product
from pathlib import Path

import yaml

from vm_cases import capture, create, evaluate, finish, install, ref


RULES = [
    "path(S,X) :- start(S,X).",
    "path(T,P) :- path(S,X), edge(X,I,P), extend(S,I,T).",
    "hit(S,X) :- path(S,X), defines(X).",
    "beaten(S) :- hit(S,X), hit(T,Y), earlier(T,S).",
    "winner(X) :- hit(S,X), not beaten(S).",
]


def predict(parents, defines, models):
    # Path keys encode only parent positions. Prefixes precede descendants;
    # every path in the first parent's subtree precedes the second subtree.
    depth = {"leaf": 0}
    for obj in parents:
        depth[obj] = 1 + max((depth[p] for p in parents[obj]), default=-1)
    words = [p for n in range(depth["leaf"] + 1) for p in product((0, 1), repeat=n)]
    ranks = {word: i for i, word in enumerate(sorted(words))}
    facts = {
        "start": {(ranks[()], "leaf")},
        "edge": {(x, i, p) for x, ps in parents.items() for i, p in enumerate(ps)},
        "extend": {(ranks[w[:-1]], w[-1], ranks[w]) for w in words if w},
        "earlier": {(a, b) for a in ranks.values() for b in ranks.values() if a < b},
        "defines": {(obj,) for obj in defines},
    }
    [winner] = evaluate(facts, RULES, models)["winner"]
    return list(winner)


def case(shape, reverse, mode):
    parents = {"deep": [], "left": ["deep"], "right": ["deep"] if shape == "diamond" else []}
    if shape == "long":
        parents["bridge"] = ["left"]
    if shape == "cut":
        parents["left"] = []
    branch = "bridge" if shape == "long" else "left"
    parents["leaf"] = ["right", branch] if reverse else [branch, "right"]
    name = f"gunray_order_{shape}_{'reverse' if reverse else 'forward'}_{mode}"
    steps = []
    for obj, ps in parents.items():
        if len(ps) == 2:
            parent_list = "{" + ", ".join(map(ref, ps)) + "}"
            steps.append({"run": f"create({parent_list})", "capture": obj, "expect": {"type": "obj"}})
        else:
            steps.append(create(obj, ps[0] if ps else None))
        if obj in ("deep", "right"):
            steps.extend(install(obj, "pick", 'return {"' + obj + '"};'))
    if mode == "pass":
        steps.extend(install("leaf", "pick", "return pass();"))
    if mode == "local":
        steps.extend(install("leaf", "unrelated", "return 0;"))
    steps.append(capture("readback", 'try return {leaf}:pick(); except e (ANY) return {"error", toliteral(e[1])}; endtry'))
    models = []
    result = predict(parents, ["deep", "right"], models)
    return finish(name, steps, {"readback": result}, models, list(parents))


def main(run):
    trials = [case(shape, reverse, mode)
              for shape in ("fork", "diamond", "long", "cut")
              for reverse in (False, True) for mode in ("plain", "local", "pass")]
    suite = {"name": "gunray_order", "description": "Definition on an earlier parent's ancestor versus a later parent",
             "requires": {"config": ["managed_server"]}, "tests": [c for c, _ in trials]}
    (run / "candidate/cases/gunray_order.yaml").write_text(yaml.safe_dump(suite, sort_keys=False))
    (run / "evidence/predictions.json").write_text(json.dumps(
        {"cases": {c["name"]: p for c, p in trials}}, indent=2) + "\n")
    print(f"{len(trials)} ordered-parent histories")


if __name__ == "__main__":
    main(Path(sys.argv[1]))
