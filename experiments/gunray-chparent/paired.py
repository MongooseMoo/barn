"""Generate paired histories and their predictions before either server runs."""

import json
import sys
from pathlib import Path

import yaml
from gunray import GunrayEvaluator, Program

from roundtrip import CHILDREN, PARENTS, ref


RULES = [
    "reachable(T,X,X) :- state(T), object(X).",
    "reachable(T,X,D) :- edge(T,X,P), reachable(T,P,D).",
    "source(T,X,D) :- reachable(T,X,D), defines(D,V).",
    "retains(T,U,X) :- previous(T,U), source(T,X,D), source(U,X,D).",
    "changed(T,X) :- write(T,X,V).",
    "changed(T,X) :- cleared(T,X).",
    "slot(T,X,V) :- write(T,X,V).",
    "slot(T,X,V) :- retains(T,U,X), slot(U,X,V), not changed(T,X).",
    "explicit(T,X) :- slot(T,X,V).",
    "base(T,X,V) :- slot(T,X,V).",
    "base(T,X,V) :- state(T), defines(X,V), not explicit(T,X).",
    "has_base(T,X) :- base(T,X,V).",
    "reads(T,X,V) :- base(T,X,V).",
    "reads(T,X,V) :- edge(T,X,P), reads(T,P,V), not has_base(T,X).",
    "bit(T,X,C) :- has_base(T,X), zero(C).",
    "bit(T,X,C) :- source(T,X,D), one(C), not has_base(T,X).",
]
PAIRS = ("erasure", "retention", "detour", "mask", "checkpoint")


def mutation(code):
    return {"run": code + "; return 1;", "expect": {"value": 1}}


def probe(name, phase):
    # Report owner identity and the native error code, including for a missing q.
    return {"run": f'case_id = "{name}_{phase}"; ' + """
        result = {};
        for child in ({{inherited}, {assigned}})
          try
            value = child.q;
            info = property_info(child, "q");
            owner = info[1] == $system ? "system" | (info[1] == player ? "player" | tostr(info[1]));
            item = {"present", value, is_clear_property(child, "q"), owner, info[2]};
          except err (ANY)
            item = {"error", toliteral(err[1])};
          endtry
          result = {@result, item};
        endfor
        return result;
    """, "expect": {"type": "list"}}


def history(pair, arm):
    """One event stream drives both the MOO history and finite model."""
    other = arm == "right"
    yield "write", "a", 11
    yield "write", "b", 22
    if pair in ("erasure", "retention"):
        yield "write", "assigned", 9 if other else 7
        yield "read", "initial"
        yield "move", "b"
        yield "read", "away"
        yield "write", "b", 31
        yield "read", "changed"
        yield "move", "a"
        yield "read", "back"
        return

    if pair != "mask" or other:
        yield "write", "assigned", 11 if pair == "mask" else 73
    else:
        yield "noop",
    for x, owner, flags in [("inherited", "system", "r"), ("assigned", "system", "w")]:
        yield "info", x, owner, flags
    if pair == "checkpoint":
        yield "move", "b"
        yield "read", "before"
        yield "reload" if other else "noop",
        active = "b"
    else:
        yield "read", "before"
        for parent in ("b", "a"):
            yield ("move", parent) if pair == "detour" and other else ("noop",)
        active = "a"
    yield "read", "join"
    yield "write", active, 31
    yield "read", "changed"
    yield "clear", "assigned"
    yield "read", "cleared"


def compile_arm(pair, arm):
    name = f"gunray_pairs_{pair}_{arm}"
    parents = dict(PARENTS)
    definitions = {"root": 1}
    if pair == "erasure":
        parents["b"] = None
        definitions["b"] = 1
    steps = [{"restart_server": {"reset_db": True}}]
    for x, parent in parents.items():
        parent_ref = ref(parent) if parent else "$nothing"
        steps.append({"run": f"create({parent_ref})", "capture": x, "expect": {"type": "obj"}})
        if x in definitions:
            steps.append(mutation(f'add_property({ref(x)}, "q", {definitions[x]}, {{player, "rw"}})'))

    facts = {"object": {(x,) for x in parents}, "defines": set(definitions.items()),
             "zero": {(0,)}, "one": {(1,)}, "edge": set(), "state": set(),
             "previous": set(), "write": set(), "cleared": set()}
    phases = {}
    metadata = {x: ["player", "rw"] for x in CHILDREN}
    for t, (kind, *args) in enumerate(history(pair, arm)):
        if kind == "move":
            parents["moved"] = args[0]
            steps.append(mutation(f"chparent({ref('moved')}, {ref(args[0])})"))
        elif kind == "write":
            x, value = args
            facts["write"].add((t, x, value))
            steps.append(mutation(f"{ref(x)}.q = {value}"))
        elif kind == "clear":
            x, = args
            facts["cleared"].add((t, x))
            steps.append(mutation(f'clear_property({ref(x)}, "q")'))
        elif kind == "info":
            x, owner, flags = args
            metadata[x] = [owner, flags]
            steps.append(mutation(f'set_property_info({ref(x)}, "q", {{$system, "{flags}"}})'))
        elif kind == "reload":
            steps.extend([{"run": "dump_database()", "expect": {"value": 0}},
                          {"restart_server": {"checkpoint_timeout_ms": 10000}}])
        elif kind == "noop":
            steps.append({"run": "1", "expect": {"value": 1}})
        elif kind == "read":
            phase, = args
            phases[phase] = (t, metadata.copy())
            steps.append(probe(name, phase))
        else:
            raise ValueError(kind)
        facts["edge"].update((t, x, p) for x, p in parents.items() if p)
        facts["state"].add((t,))
        if t:
            facts["previous"].add((t, t - 1))
    facts = {predicate: rows for predicate, rows in facts.items() if rows}
    model = GunrayEvaluator().evaluate(Program(facts=facts, rules=RULES))
    expected = {}
    for phase, (t, info) in phases.items():
        expected[phase] = []
        for x in CHILDREN:
            [value] = [v for ts, obj, v in model.facts["reads"] if (ts, obj) == (t, x)]
            [bit] = [v for ts, obj, v in model.facts["bit"] if (ts, obj) == (t, x)]
            expected[phase].append(["present", value, bit, *info[x]])
    case = {"name": name, "permission": "wizard", "steps": steps,
            "cleanup": [{"run": f"recycle({ref(x)})"} for x in reversed(parents)]}
    return case, {"expected": expected, "facts": {k: sorted(v) for k, v in facts.items()}}


def main(run):
    cases, predictions = [], {}
    for pair in PAIRS:
        predictions[pair] = {}
        for arm in ("left", "right"):
            case, predictions[pair][arm] = compile_arm(pair, arm)
            cases.append(case)
    (run / "candidate/cases/gunray_pairs.yaml").write_text(yaml.safe_dump(
        {"name": "gunray_pairs", "description": "Relational property-slot histories",
         "requires": {"config": ["managed_server"]}, "tests": cases}, sort_keys=False))
    (run / "evidence/predictions.json").write_text(json.dumps(
        {"children": CHILDREN, "rules": RULES, "pairs": predictions}, indent=2) + "\n")
    for pair, arms in predictions.items():
        print(pair, {phase: left == arms["right"]["expected"][phase]
                     for phase, left in arms["left"]["expected"].items()})


if __name__ == "__main__":
    main(Path(sys.argv[1]))
