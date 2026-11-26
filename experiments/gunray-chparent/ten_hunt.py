"""Generate bounded semantic probes and inventory every managed observation."""
import csv
import json
import re
import sys
from collections import defaultdict
from pathlib import Path
from xml.etree import ElementTree

import yaml
from gunray import evaluate_program_file


def catalog():
    probes = {}
    facts = defaultdict(list)

    def add(name, expression, relation, *terms):
        assert name not in probes
        probes[name] = expression
        facts[relation].append((name, *terms))

    add("ansi_raw", 'length(remove_ansi(parse_ansi("[red]")))', "ansi_bytes", "strip_raw", "red")
    add("ansi_strip_tag", 'length(remove_ansi("[red]"))', "ansi_bytes", "strip_tag", "red")
    for color in ["grey", "red"]:
        add("ansi_" + color, f'length(parse_ansi("[{color}]"))', "ansi_bytes", "emit", color)
    add("random_palette", 'parse_ansi("[random]") == parse_ansi("[white]")', "palette_probe", "white")
    for name, expr, relation, terms in [
        ("word_begin", 'match("a!", "a%<")', "boundary", ("begin", 1, 0)),
        ("word_begin_yes", 'match("!a", "%<a")', "boundary", ("begin", 0, 1)),
        ("word_end", 'match("!a", "%>a")', "boundary", ("end", 0, 1)),
        ("word_end_yes", 'match("a!", "a%>")', "boundary", ("end", 1, 0)),
        ("capture_same", 'match("aa", "%(a%)%1", 1)', "backreference", ("a", "a")),
        ("capture_diff", 'match("ab", "%(a%)%1", 1)', "backreference", ("a", "b")),
        ("reverse_anchor", 'rmatch("ba", "^a")', "anchored", (2,)),
        ("reverse_anchor_yes", 'rmatch("ab", "^a")', "anchored", (1,)),
    ]:
        add(name, "length(" + expr + ") > 0", relation, *terms)
        facts["regex_probe"].append((name,))

    for name, text in [("hex", "0x1p2"), ("hex_fraction", "0x1.8"),
                       ("underscore", "1_2"), ("decimal", "1.5")]:
        if "_" in text:
            add("float_" + name, f'tofloat({json.dumps(text)})', "invalid_float", text)
        else:
            number = float.fromhex(text) if text.startswith("0x") else float(text)
            add("float_" + name, f'tofloat({json.dumps(text)})', "valid_float", number)
    mode = '"embedded-types"'
    for label, value in [
        ("text", '"42|int"'), ("key", '["2|int" -> 19]'),
        ("ordinary", '"forty-two"'), ("boolkey", '[parse_json("true") -> 1]'),
        ("backslash_t", 'tostr(chr(92), "t")'), ("backslash_u", 'tostr(chr(92), "u00ab")'),
        ("backslash_q", 'tostr(chr(92), "q")'),
    ]:
        add("json_roundtrip_" + label, f'parse_json(generate_json({value},{mode}),{mode}) == {value}',
            "required", "roundtrip")
    for label, token, moo_value in [
        ("bool", "true|bool", 'parse_json("true")'), ("false", "maybe|bool", 'parse_json("false")'),
        ("invalid_int", "abc|int", "0"), ("invalid_float", "abc|float", "0.0"),
        ("invalid_err", "E_FICTION|err", "E_NONE"), ("valid_err", "E_TYPE|err", "E_TYPE"),
        ("valid_int", "12|int", "12"),
    ]:
        arg = json.dumps(json.dumps(token))
        add("embedded_" + label, f'parse_json({arg},{mode}) == {moo_value}', "required", "typed_match")
    for label, expr, law in [
        ("json_tiny", 'parse_json("1e-400")', "invalid_argument"),
        ("json_html", 'generate_json("<") == tostr(chr(34),"<",chr(34))', "literal_match"),
        ("plain_escape", 'length(match("q","%q")) > 0', "literal_match"),
        ("ordinary_percent", 'length(match("%","%%")) > 0', "literal_match"),
        ("bad_anchor", 'length(match("abc","a^b"))', "invalid_argument"),
        ("nested_ansi", 'length(remove_ansi("[gr[red]een]"))', "zero_length"),
        ("extra_heading", 'relative_heading({0.0,0.0,0.0,4.0},{1.0,0.0,0.0,5.0}) == {0,0}', "typed_match"),
        ("heading_control", 'relative_heading({0.0,0.0,0.0},{1.0,0.0,0.0}) == {0,0}', "typed_match"),
        ("all_members_arity", 'all_members(2,{2},0)', "wrong_arity"),
        ("all_members_normal", 'all_members(2,{2}) == {1}', "typed_match"),
    ]:
        add(label, expr, "required", law)
    for label, spell in [
        ("object_gap", "# 7"), ("object_solid", "#7"), ("object_negative", "# -3"),
    ]:
        add(label, f'toint(toobj({json.dumps(spell)}))', "object_id", int(spell.removeprefix("#")))
    for label, expr, law in [
        ("int_underscore", 'toint("1_2")', "invalid_integer"),
        ("int_hex", 'toint("0x1p2")', "invalid_integer"),
        ("int_tab_tail", 'toint(tostr("7",chr(9)))', "invalid_integer"),
        ("int_tab_head", 'toint(tostr(chr(9),"7"))', "seven"),
        ("int_space_tail", 'toint("7 ")', "seven"),
        ("bol_after_newline", 'length(match(tostr("a",chr(10),"b"),"^b")) > 0', "literal_match"),
        ("eol_before_newline", 'length(match(tostr("a",chr(10),"b"),"a$")) > 0', "literal_match"),
        ("punct_boundary", 'length(match("!","%b!")) > 0', "literal_match"),
        ("punct_nonboundary", 'length(match("!","%B!")) > 0', "no_match"),
        ("chr_zero_middle", 'equal(chr(65,0,66),"A")', "literal_match"),
        ("chr_zero_control", 'equal(chr(65,66),"AB")', "literal_match"),
        ("json_norm_generate", 'equal(generate_json(tostr(chr(92),"t")),tostr(chr(34),chr(92),chr(92),"t",chr(34)))', "literal_match"),
        ("json_norm_parse", 'equal(parse_json(tostr(chr(34),chr(92),chr(92),"t",chr(34))),tostr(chr(92),"t"))', "literal_match"),
        ("json_finite", 'parse_json("2.5")', "two_point_five"),
        ("json_bool_key_generated", f'generate_json([parse_json("true") -> 1],{mode}) == generate_json(["true|bool" -> 1])', "literal_match"),
        ("json_invalid_value_type", f'typeof(parse_json({json.dumps(json.dumps("bad|float"))},{mode})) == typeof(1.0)', "typed_match"),
    ]:
        add(label, expr, "required", law)
    return probes, facts


def generate(run, family, chosen):
    probes, facts = catalog()
    rows = evaluate_program_file(Path(__file__).with_name("ten-laws.gnr"), facts=facts,
                                 query="prediction").facts["prediction"]
    predictions = dict(rows)
    if len(predictions) != len(rows):
        raise ValueError("Conflicting predictions for a probe")
    if set(predictions) != set(probes):
        raise ValueError(set(probes) ^ set(predictions))
    tests = []
    for label in chosen or probes:
        expr = probes[label]
        name = family + "_" + label
        statement = f'case_id = "{name}"; try return {{({expr})}}; except e (ANY) return {{toliteral(e[1])}}; endtry'
        tests.append({"name": name, "permission": "wizard", "steps": [
            {"run": statement, "expect": {"value": [predictions[label]]}}]})
    called = {"toliteral"}
    for label in chosen or probes:
        called.update(re.findall(r"\b([a-z][a-z_0-9]*)\(", probes[label]))
    suite = {"name": family, "description": "Gunray semantic law discriminators",
             "requires": {"builtins": sorted(called), "config": ["managed_server"]}, "tests": tests}
    (run / "candidate/cases" / (family + ".yaml")).write_text(yaml.safe_dump(suite, sort_keys=False))
    (run / "evidence" / (family + "-catalog.json")).write_text(json.dumps(
        {t["name"]: {"expected": t["steps"][0]["expect"]["value"], "expr": probes[t["name"][len(family)+1:]]}
         for t in tests}, indent=2) + "\n")
    print(f"{len(tests)} semantic probes")


def compare(run, phase, family):
    evidence = run / "evidence"
    cases = json.loads((evidence / (family + "-catalog.json")).read_text())
    for engine in ("toast", "current"):
        d = evidence / (phase + "-" + engine)
        xml = ElementTree.parse(d / "results.xml").findall(".//testcase")
        admission, = [item for item in xml if item.get("name") == "test_capability_admission"]
        assert not any(admission.find(t) is not None for t in ("failure", "skipped", "error"))
        records = [json.loads(s) for s in (d / "observations.jsonl").read_text().splitlines()]
        assert len(records) == len(cases) and {r["case"] for r in records} == set(cases)
        for r in records:
            assert r["result"]["success"], r
            name = r["case"]
            v = r["result"]["value"]
            case = cases[name]
            test, = [t for t in xml if t.get("name", "").endswith("::" + name + "]")]
            assert test.find("error") is None and test.find("skipped") is None
            assert (test.find("failure") is not None) == (case["expected"] != v)
            case[engine] = v
    with (evidence / (family + "-observations.csv")).open("w", newline="") as f:
        writer = csv.writer(f, lineterminator="\n")
        writer.writerow(["probe", "expr", "gunray", "toast", "barn"])
        for name, case in cases.items():
            writer.writerow([name, case["expr"], *[json.dumps(case[k]) for k in ("expected", "toast", "current")]])
            if case["toast"] != case["current"]:
                print(name, "Gunray", case["expected"], "Toast", case["toast"], "Barn", case["current"])


if __name__ == "__main__":
    command, run, family, *args = sys.argv[1:]
    if command == "generate":
        generate(Path(run), family, args)
    else:
        compare(Path(run), args[0], family)
