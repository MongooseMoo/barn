"""Change ancestry while an inherited verb is active, then call pass()."""

import json
import sys
from itertools import product
from pathlib import Path

import yaml
from gunray import evaluate_program_file

from vm_cases import capture, create, finish, install, ref


def case(program, edit, pause):
    changes = {"steady": None, "receiver": ("leaf", "side"),
               "definer": ("wrapper", "beta"), "intermediate": ("bridge", "beta")}
    change = changes[edit]
    facts = {"reparent": [change]} if change else {}
    models = [{"program": program.name, "additional_facts": facts}]
    result = evaluate_program_file(program, facts=facts).facts
    [(winner,)] = result["winner"]
    [(fresh_entry,)] = result["fresh_entry"]
    steps = [create("alpha"), create("beta"), create("side"), create("bridge", "alpha"),
             create("wrapper", "bridge"),
             {"run": "create({{wrapper}, {side}})", "capture": "leaf", "expect": {"type": "obj"}}]
    for obj in ("alpha", "beta", "side"):
        steps.extend(install(obj, "route", f'return {{"{obj}", this == args[1], args[2]}};'))
    wrapper = ('if (args[3]) if (args[6] == "before") suspend(0); endif '
               'if (args[4] != $nothing) chparent(args[4], args[5]); endif '
               'if (args[6] == "after") suspend(0); endif endif '
               'return {"wrapper", pass(@args)};')
    steps.extend(install("wrapper", "route", wrapper))
    target, dest = (map(ref, change) if change else ("$nothing", "$nothing"))
    steps.append(capture("active", f'return {{leaf}}:route({{leaf}}, "token", 1, {target}, {dest}, "{pause}");'))
    steps.append(capture("fresh", 'return {leaf}:route({leaf}, "token", 0, $nothing, $nothing, "none");'))
    active = ["wrapper", [winner, 1, "token"]]
    fresh = active if fresh_entry == "wrapper" else [fresh_entry, 1, "token"]
    return finish(f"gunray_inherited_pass_{edit}_{pause}", steps,
                  {"active": active, "fresh": fresh}, models,
                  ["alpha", "beta", "side", "bridge", "wrapper", "leaf"])


def main(run):
    program = Path(__file__).with_name("inherited-pass.gnr")
    histories = [case(program, edit, pause) for edit, pause in product(
        ("steady", "receiver", "definer", "intermediate"), ("none", "before", "after"))]
    suite = {"name": "gunray_inherited_pass", "description": "Active definer ancestry under reparenting",
             "requires": {"config": ["managed_server"]}, "tests": [c for c, _ in histories]}
    (run / "candidate/cases/gunray_inherited_pass.yaml").write_text(yaml.safe_dump(suite, sort_keys=False))
    (run / "evidence/predictions.json").write_text(json.dumps(
        {"cases": {c["name"]: p for c, p in histories}}, indent=2) + "\n")
    print(f"{len(histories)} inherited-pass histories")


if __name__ == "__main__":
    main(Path(sys.argv[1]))
