"""Check warrant revision on fixed eligibility facts and emit the ordered MOO cases."""

import argparse
from dataclasses import replace
from itertools import combinations
from pathlib import Path

import yaml
from gunray import GunrayEvaluator

from range_handlers import FILTERS, handler_theory


def check(destination=None):
    # Expected conclusions are independent of both the evaluator and the VM.
    scenarios = [
        ("default", ("group",), "E_TYPE", ("group",), ()),
        ("exception", ("typed", "group"), "E_TYPE", ("typed",), ("group",)),
        ("nonmatching", ("typed", "group"), "E_RANGE", ("group",), ()),
        ("reversed", ("group", "typed"), "E_TYPE", ("group",), ("typed",)),
        ("unranked", ("typed", "group"), "E_TYPE", (), ("typed", "group")),
    ]
    tests = []
    original_facts = None
    fixed_policy = handler_theory(("typed", "group"), [("typed", "group")], present=("group",))
    expanded = replace(fixed_policy, facts={**fixed_policy.facts, "present": [("group",), ("typed",)]})
    before = {(name, *row) for name, rows in fixed_policy.facts.items() for row in rows}
    after = {(name, *row) for name, rows in expanded.facts.items() for row in rows}
    assert before < after and after - before == {("present", "typed")}
    assert replace(expanded, facts=fixed_policy.facts) == fixed_policy
    by_fact = {"fact_before": fixed_policy, "fact_after": expanded}
    scenarios.extend([
        ("fact_before", ("group",), "E_TYPE", ("group",), ()),
        ("fact_after", ("typed", "group"), "E_TYPE", ("typed",), ("group",)),
    ])
    for label, order, error, expected_yes, expected_undecided in scenarios:
        priorities = [] if label == "unranked" else list(combinations(order, 2))
        if label in by_fact:
            theory = by_fact[label]
        else:
            theory = handler_theory(order, priorities)
            if original_facts is None:
                original_facts = theory.facts
            assert theory.facts == original_facts
        sections = GunrayEvaluator().evaluate(theory).sections
        claims = tuple(rule.id for rule in theory.defeasible_rules)
        yes = tuple(name for name in claims if (error,) in sections["yes"].get(name, set()))
        undecided = tuple(name for name in claims if (error,) in sections["undecided"].get(name, set()))
        assert (yes, undecided) == (expected_yes, expected_undecided), (label, yes, undecided)
        print(f"{label:12} {error:7} warranted={','.join(yes) or '-'} undecided={','.join(undecided) or '-'}")
        if label == "unranked":
            continue  # Every real MOO except clause has a source position.
        name = "gunray_handler_revision_" + label
        body = f'case_id = "{name}"; try raise({error}); '
        for handler in order:
            body += f'except caught ({", ".join(FILTERS[handler])}) return {{"{handler}", toliteral(caught[1])}}; '
        tests.append({"name": name, "permission": "wizard", "steps": [
            {"run": body + "endtry", "expect": {"value": [yes[0], error]}}]})
    if destination is not None:
        suite = {"name": "gunray_handler_revision", "description": "Ordered exception dispatch",
                 "requires": {"builtins": ["raise", "toliteral"], "config": ["managed_server"]},
                 "tests": tests}
        destination.write_text(yaml.safe_dump(suite, sort_keys=False))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--suite", type=Path, help="Write managed MOO cases to this YAML file.")
    check(parser.parse_args().suite)
