"""Finite MOO correctness experiments for dispatch, movement and control transfer."""

import json
import sys
from pathlib import Path

import yaml
from gunray import GunrayEvaluator, Program

from paired import mutation
from roundtrip import ref


def evaluate(facts, rules, models):
    facts = {key: rows for key, rows in facts.items() if rows}
    models.append({"rules": rules, "facts": {key: sorted(rows) for key, rows in facts.items()}})
    return GunrayEvaluator().evaluate(Program(facts=facts, rules=rules)).facts


def create(label, parent=None):
    obj = ref(parent) if parent else "$nothing"
    return {"run": f"create({obj})", "capture": label, "expect": {"type": "obj"}}


def install(obj, name, body):
    return [mutation(f'add_verb({ref(obj)}, {{player, "rxd", "{name}"}}, {{"this", "none", "this"}})'),
            {"run": f"return set_verb_code({ref(obj)}, 1, {{{json.dumps(body)}}});", "expect": {"value": []}}]


def capture(label, code):
    return {"run": code, "capture": label, "expect": {"type": "list"}}


def finish(name, steps, results, models, objects=()):
    values = list(results.values())
    labels = list(results)
    code = "return {" + ", ".join(map(ref, labels)) + "};"
    steps.append({"run": f'case_id = "{name}"; {code}', "expect": {"value": values}})
    case = {"name": name, "permission": "wizard", "steps": [{"restart_server": {"reset_db": True}}, *steps]}
    if objects:
        case["cleanup"] = [{"run": f"recycle({ref(x)})"} for x in reversed(objects)]
    return case, {"labels": labels, "expected": values, "models": models}


def dispatch(warm, alias):
    objects = ["root", "parent", "child"]
    edges = set(zip(objects[1:], objects))
    steps = [create("root"), create("parent", "root"), create("child", "parent")]
    parent_names = "salute greet" if alias else "greet"
    for obj in objects:
        body = 'return {"child", this == args[1], verb, pass(@args)};' if obj == "child" else (
            'return {"' + obj + '", this == args[1], verb};')
        steps.extend(install(obj, parent_names if obj == "parent" else "greet", body))
    if warm:
        steps.append({"run": "{child}:greet({child})", "expect": {"value": [
            "child", 1, "greet", ["parent", 1, "greet"]]}})
    else:
        steps.append({"run": "1", "expect": {"value": 1}})
    results, models = {}, []
    rules = [
        "answer(X,Y) :- edge(X,Y), definition(Y,N), called(N).",
        "matches(Y) :- definition(Y,N), called(N).",
        "answer(X,Z) :- edge(X,Y), not matches(Y), answer(Y,Z).",
    ]
    for phase, new_name in [("renamed", "salute"), ("restored", parent_names)]:
        steps.append(mutation(f'set_verb_info({ref("parent")}, 1, {{player, "rxd", "{new_name}"}})'))
        definitions = {("root", "greet"), *(("parent", word) for word in new_name.split())}
        rows = evaluate({"edge": edges, "definition": definitions,
                         "called": {("greet",)}}, rules, models)["answer"]
        [selected] = [obj for x, obj in rows if x == "child"]
        results[phase] = ["child", 1, "greet", [selected, 1, "greet"]]
        steps.append(capture(phase, 'try return {child}:greet({child}); except e (ANY) return {"error", toliteral(e[1])}; endtry'))
    name = f"gunray_vm_dispatch_{'alias' if alias else 'exact'}_{'warm' if warm else 'cold'}"
    return finish(name, steps, results, models, objects)


def movement(attempt, live_write):
    objects = ["a", "b", "c", "spare", "room"]
    steps = []
    for obj in objects:
        steps.append(create(obj))
        steps.extend(install(obj, "accept", "return 1;"))
    # Indices in the fixed fixture list are cross-world labels; zero means $nothing.
    refs = "{" + ", ".join(map(ref, objects)) + "}"
    inspect = ("ids = " + refs + "; graph = {}; for x in (ids) kids = {}; "
               "for k in (x.contents) kids = {@kids, k in ids}; endfor "
               "p = x.location; index = p in ids; "
               "graph = {@graph, {p == $nothing || index ? index | tostr(p), kids}}; "
               "endfor return {status, graph};")
    location = {}
    contents = {x: [] for x in objects}
    results, models = {}, []
    rules = ["under(X,Y) :- inside(X,Y).", "under(X,Z) :- inside(X,Y), under(Y,Z)."]
    ops = [("place_a", "a", "room"), ("place_b", "b", "a"), ("place_c", "c", "b"),
           ("place_spare", "spare", "b"), ("challenge", "a", "c"),
           ("detach_c", "c", "room"), ("retry_a", "a", "c")]
    for phase, obj, dest in ops:
        ancestors = evaluate({"inside": set(location.items())}, rules, models).get("under", set())
        if phase == "challenge" and not attempt:
            code, status = 'status = {"noop"};', ["noop"]
        else:
            code = (f'try status = {{"ok", move({ref(obj)}, {ref(dest)})}}; '
                    'except e (ANY) status = {"error", toliteral(e[1])}; endtry')
            if (dest, obj) in ancestors:
                status = ["error", "E_RECMOVE"]
            else:
                status = ["ok", 0]
                if obj in location:
                    contents[location[obj]].remove(obj)
                location[obj] = dest
                contents[dest].append(obj)
        graph = [[objects.index(location[x]) + 1 if x in location else 0,
                  [objects.index(k) + 1 for k in contents[x]]] for x in objects]
        results[phase] = [status, graph]
        if live_write:
            code = 'set_verb_info({spare}, 1, {player, "rxd", "accept"}); ' + code
        steps.append(capture(phase, code + " " + inspect))
    name = f"gunray_vm_move_{'live' if live_write else 'ordinary'}_{'reject' if attempt else 'noop'}"
    return finish(name, steps, results, models, objects)


def cleanup_case(depth, target, pause):
    # Lexical scopes run from the transfer site outwards. Derive crossed cleanup
    # regions by intersecting ancestors of the site with descendants of the target.
    cleaners = [f"clean{k}" for k in range(depth)]
    scopes = ["site", *cleaners, "j", "outer_clean", "i"]
    facts = {"edge": set(zip(scopes, scopes[1:])), "origin": {("site",)}, "target": {(target,)},
             "cleanup": {(s,) for s in [*cleaners, "outer_clean"]}}
    rules = ["ancestor(X,Y) :- edge(X,Y).", "ancestor(X,Z) :- edge(X,Y), ancestor(Y,Z).",
             "crossed(S) :- origin(O), target(T), ancestor(O,S), ancestor(S,T), cleanup(S)."]
    models = []
    crossed = {s for s, in evaluate(facts, rules, models)["crossed"]}

    def moo_tag(tag, index):
        return 'seen = {@seen, {"' + tag + '", i, ' + index + '}};'

    def finally_block(region, index):
        suspend = "suspend(0);" if region == pause else ""
        return " ".join([moo_tag(region + "_in", index), suspend, moo_tag(region + "_out", index)])

    body = f"last = {{i,j}}; {moo_tag('visit', 'j')} continue {target};"
    for region in cleaners:
        body = f"try {body} finally {finally_block(region, 'j')} endtry"
    program = (f"seen = {{}}; last = {{}}; for i in [1..2] try for j in [1..2] {body} endfor "
               f"{moo_tag('after_inner', '0')} finally {finally_block('outer_clean', '0')} "
               "endtry endfor return {seen, last};")
    events = []
    for i in (1, 2):
        for j in (1, 2):
            last = [i, j]
            events.append(["visit", i, j])
            for region in cleaners:
                if region in crossed:
                    events.extend([[region + "_in", i, j], [region + "_out", i, j]])
            if "outer_clean" in crossed:
                break
        if "outer_clean" not in crossed:
            events.append(["after_inner", i, 0])
        events.extend([["outer_clean_in", i, 0], ["outer_clean_out", i, 0]])
    name = f"gunray_vm_cleanup_{depth}_{target}_{pause}"
    return finish(name, [capture("execution", program)], {"execution": [events, last]}, models)


def main(run):
    trials = [dispatch(warm, alias) for alias in (False, True) for warm in (False, True)]
    trials.extend(movement(attempt, live_write) for live_write in (False, True) for attempt in (False, True))
    for depth in (1, 2):
        for target in ("i", "j"):
            for pause in ["none", *(f"clean{k}" for k in range(depth)), "outer_clean"]:
                trials.append(cleanup_case(depth, target, pause))
    cases = [case for case, _ in trials]
    (run / "candidate/cases/gunray_vm.yaml").write_text(yaml.safe_dump(
        {"name": "gunray_vm", "description": "Dispatch, containment and finally transfers",
         "requires": {"config": ["managed_server"]}, "tests": cases}, sort_keys=False))
    predictions = {case["name"]: predicted for case, predicted in trials}
    (run / "evidence/predictions.json").write_text(json.dumps({"cases": predictions}, indent=2) + "\n")
    print(f"{len(trials)} histories: 4 dispatch, 4 movement, {len(trials)-8} cleanup")


if __name__ == "__main__":
    main(Path(sys.argv[1]))
