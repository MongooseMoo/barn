"""Independent splice and exception-handler oracles for nested MOO writes."""

import csv
import json
import sys
from itertools import combinations, product
from pathlib import Path

import yaml
from gunray import DefeasibleTheory, GunrayEvaluator, Rule, evaluate_program_file

from vm_cases import capture, finish


BOUNDS = {"replace": (2, 2), "clamp": (-2, 7), "prepend": (1, 0), "append": (4, 3),
          "overlap": (3, 1), "past": (5, 3), "negative": (1, -1)}
FILTERS = {"typed": ("E_TYPE",), "group": ("E_TYPE", "E_RANGE"), "fallback": ("ANY",)}
ORDERS = [("typed", "group", "fallback"), ("group", "typed", "fallback"),
          ("typed", "fallback")]


def predict_writes():
    facts = {name: [] for name in ("base", "replacement", "bounds", "limit", "item")}
    inputs = {}
    for kind, (label, (low, high)), variant in product(
        ("list", "string"), BOUNDS.items(), ("replace", "delete", "wrong")
    ):
        key = f"{kind}_{label}_{variant}"
        same = variant != "wrong"
        original = list("abc") if kind == "list" else "abc"
        other = "string" if kind == "list" else "list"
        replacement_kind = kind if same else other
        tokens = "XY" if variant != "delete" else ""
        value = list(tokens) if replacement_kind == "list" else tokens
        inputs[key] = (original, low, high, value)
        for relation, row in [("base", (key, kind)), ("replacement", (key, replacement_kind)),
                              ("bounds", (key, low, high)), ("limit", (key, len(original) + 1))]:
            facts[relation].append(row)
        facts["item"].extend((key, i, v) for i, v in enumerate(original, 1))
    model = evaluate_program_file(Path(__file__).with_name("range-write.gnr"), facts=facts).facts
    predictions = {}
    for key, (original, _, _, replacement) in inputs.items():
        if (key,) in model["applies"]:
            def segment(name):
                return [v for c, _, v in sorted(model[name]) if c == key]
            parts = segment("prefix") + list(replacement) + segment("suffix")
            value = "".join(parts) if isinstance(original, str) else parts
            predictions[key] = ("ok", value)
        else:
            [error] = [code for c, code in model["error"] if c == key]
            predictions[key] = (error, original)
    return inputs, facts, predictions


HANDLER_CODES = ("E_TYPE", "E_RANGE", "E_DIV")


def handler_theory(order, priorities, present=tuple(FILTERS)):
    # Keep eligibility facts fixed across additions, removals and reorderings.
    facts = {f"matches_{name}": [(code,) for code in HANDLER_CODES
                                  if "ANY" in FILTERS[name] or code in FILTERS[name]]
             for name in FILTERS}
    facts["present"] = [(name,) for name in present]
    return DefeasibleTheory(facts=facts,
        defeasible_rules=[Rule(id=name, head=f"{name}(X)", body=[f'matches_{name}(X)', f'present("{name}")']) for name in order],
        superiority=list(priorities), conflicts=list(combinations(order, 2)))


def handler_model(order):
    theory = handler_theory(order, combinations(order, 2))
    warranted = GunrayEvaluator().evaluate(theory).sections["yes"]
    winners = {}
    for code in HANDLER_CODES:
        [winners[code]] = [name for name in order if (code,) in warranted.get(name, set())]
    return winners


def literal(value):
    if isinstance(value, list):
        return "{" + ", ".join(map(literal, value)) + "}"
    return json.dumps(value)


def trial(key, initial, low, high, replacement, status, expected, order, winners, reraise):
    body = ('try before = {"left", ' + literal(initial) + ', "right"}; '
            'work = before; seen = {}; try ')
    if status == "E_DIV":
        body += "ignore = 1 / 0; "
    else:
        body += f"work[2][{low}..{high}] = {literal(replacement)}; "
    body += 'seen = {"ok"}; '
    for name in order:
        body += f'except e ({", ".join(FILTERS[name])}) seen = {{"{name}", toliteral(e[1])}}; '
        if reraise:
            body += 'raise(E_INVARG, "from handler"); '
    body += ('endtry except outer (ANY) seen = {@seen, toliteral(outer[1]), outer[2]}; '
             'endtry return {seen, work, before};')
    trace = ["ok"] if status == "ok" else [winners[status], status]
    if reraise and status != "ok":
        trace += ["E_INVARG", "from handler"]
    observation = [trace, ["left", expected, "right"], ["left", initial, "right"]]
    return capture(key, body), observation


def main(run):
    inputs, facts, writes = predict_writes()
    # One managed history per ordering and catch-body behavior. Each probe
    # starts with fresh local values and records its own complete observation.
    histories = []
    for order in ORDERS:
        winners = handler_model(order)
        for reraise in (False, True):
            steps, expected = [], {}
            for key, (initial, lo, hi, repl) in inputs.items():
                status, value = writes[key]
                step, expected[key] = trial(key, initial, lo, hi, repl, status, value, order, winners, reraise)
                steps.append(step)
            key = "fallback_control"
            step, expected[key] = trial(key, "abc", 1, 1, "", "E_DIV", "abc", order, winners, reraise)
            steps.append(step)
            name = "gunray_range_handlers_" + "_".join(order) + ("_escape" if reraise else "_handle")
            histories.append(finish(name, steps, expected, [{"order": order, "winners": winners}]))
    suite = {"name": "gunray_range_handlers", "description": "Nested range writes and exception routing",
             "requires": {"config": ["managed_server"]}, "tests": [c for c, _ in histories]}
    (run / "candidate/cases/gunray_range_handlers.yaml").write_text(yaml.safe_dump(suite, sort_keys=False))
    (run / "evidence/predictions.json").write_text(json.dumps(
        {"cases": {c["name"]: pred for c, pred in histories}, "range_facts": facts}, indent=2) + "\n")
    print(f"{len(histories)} histories; {len(inputs)} writes and one control each")


def compare_pairs(evidence):
    cases = json.loads((evidence / "predictions.json").read_text())["cases"]
    with (evidence / "relations.csv").open("w", newline="") as out:
        writer = csv.writer(out)
        writer.writerow(["engine", "history", "probe", "relation", "holds"])
        for engine in ("toast", "current"):
            raw = (evidence / f"model-{engine}" / "observations.jsonl").read_text().splitlines()
            runs = {}
            for line in raw:
                r = json.loads(line)
                runs[r["case"]] = dict(zip(cases[r["case"]]["labels"], r["result"]["value"], strict=True))
            verdicts = []
            for name, probes in runs.items():
                for key, (trace, work, original) in probes.items():
                    if key.startswith("list_"):
                        other = probes["string_" + key.removeprefix("list_")]
                        # The two representations use the same ASCII tokens.
                        verdicts.append((name, key, "list-string agreement",
                                         [trace, [work[0], "".join(work[1]), work[2]],
                                          [original[0], "".join(original[1]), original[2]]] == other))
                    if name.endswith("_handle"):
                        esc_trace, esc_work, esc_original = runs[name.removesuffix("_handle") + "_escape"][key]
                        suffix = [] if trace == ["ok"] else ["E_INVARG", "from handler"]
                        verdicts.append((name, key, "raise in handler",
                                         esc_trace == trace + suffix and esc_work == work and esc_original == original))
            for row in verdicts:
                writer.writerow([engine, *row])
            print(f"{engine}: {sum(row[-1] for row in verdicts)}/{len(verdicts)} paired checks")
            if not all(row[-1] for row in verdicts):
                raise ValueError(f"{engine}: inspect relations.csv")


if __name__ == "__main__":
    if sys.argv[1] == "--compare":
        compare_pairs(Path(sys.argv[2]))
    else:
        main(Path(sys.argv[1]))
