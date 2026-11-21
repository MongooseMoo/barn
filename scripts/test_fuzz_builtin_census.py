import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from fuzz_builtin_census import (CONFORMANCE_SCAN, ZERO_LIST, builtin_calls, census, compared_sources,
                                 read_zero_list, render_zero_list)
from fuzz_differential import TIMED_OUT


def row(key, toast, barn, source):
    return {"key": key, "toast_raw": toast, "barn_raw": barn, "source": source}


class BuiltinCensusTests(unittest.TestCase):
    def test_only_programs_compared_on_both_engines_count(self):
        rows = [row("F00001", '=> {"F00001", "value", 2}', '=> {"F00001", "value", 2}', "return length(\"ab\");"),
                row("F00002", '=> {"F00002", "raised", E_TYPE}', '=> {"F00002", "value", 0}', "return abs(\"x\");"),
                row("F00003", '=> {"F00003", "compile", 0}', '=> {"F00003", "compile", 0}', "return min(;"),
                row("F00004", '=> {"F00004", "compile", 0}', '=> {"F00004", "value", 1}', "return max(1);"),
                row("F00005", TIMED_OUT, '=> {"F00005", "value", 1}', "return tostr(1);"),
                row("F00006", "=> *Aborted*", "=> *Aborted*", "while (1) toint(1); endwhile"),
                row("F00007", '=> {"F00007", "value", 1}', "Error: internal error", "return typeof(1);")]
        self.assertEqual(compared_sources({"rows": rows}), ["return length(\"ab\");", "return abs(\"x\");"])

    def test_calls_are_read_by_barns_parser(self):
        self.assertEqual(builtin_calls(['return call_function("ABS", -1) + length("min(1)");', "return (;"]),
                         [{"builtins": ["abs", "call_function", "length"]},
                          {"builtins": [], "error": "syntax error"}])

    def test_removing_a_builtins_coverage_fails_the_check(self):
        names = ["abs", "curl", "length", "trunc"]
        calls = [{"builtins": ["abs", "length"]}, {"builtins": ["not_a_toast_builtin"]}, {"error": "syntax error"}]
        tests = {"length": ["_tests/a.yaml::t1"], "trunc": ["_tests/b.yaml::t2"]}
        listed = ["curl"]
        covered = census(names, calls, tests, listed)
        self.assertEqual(covered["builtins"], {"abs": [1, 0], "curl": [0, 0], "length": [1, 1], "trunc": [0, 1]})
        self.assertEqual((covered["zero"], covered["newly_zero"], covered["newly_covered"]), (["curl"], [], []))
        self.assertEqual(covered["unparsed_programs"], 1)
        # trunc's only conformance test is removed: it newly drops to zero.
        dropped = census(names, calls, {"length": tests["length"]}, listed)
        self.assertEqual((dropped["newly_zero"], dropped["newly_covered"]), (["trunc"], []))
        # abs's only compared program is removed.
        dropped = census(names, calls[1:], tests, listed)
        self.assertEqual(dropped["newly_zero"], ["abs"])
        # A listed builtin that gains coverage fails too, so the list stays honest.
        gained = census(names, calls + [{"builtins": ["curl"]}], tests, listed)
        self.assertEqual((gained["newly_zero"], gained["newly_covered"]), ([], ["curl"]))

    def test_committed_zero_list_is_in_canonical_form(self):
        text = ZERO_LIST.read_text(encoding="utf-8")
        self.assertEqual(render_zero_list(read_zero_list(text)), text)

    def test_conformance_scan_counts_each_test_once(self):
        calls = [("length", "t1"), ("length", "t1.setup[0]"), ("length", "t1.steps[2].expect.satisfies"),
                 ("length", "t2"), ("length", "suite.setup[0]"), ("abs", "suite.teardown[1]")]
        with tempfile.TemporaryDirectory() as directory:
            package = Path(directory) / "moo_conformance"
            package.mkdir()
            (package / "__init__.py").write_text("")
            (package / "builtin_coverage.py").write_text(
                "from types import SimpleNamespace\n"
                f"CALLS = {calls!r}\n"
                "def collect_builtin_calls(root, names):\n"
                "    return [SimpleNamespace(name=n, path='a.yaml', context=c) for n, c in CALLS if n in names]\n")
            proc = subprocess.run([sys.executable, "-c", CONFORMANCE_SCAN, json.dumps(["abs", "length"]), "_tests"],
                                  cwd=directory, capture_output=True, text=True, check=True)
        self.assertEqual(json.loads(proc.stdout), {
            "abs": ["_tests/a.yaml::suite"],
            "length": ["_tests/a.yaml::suite", "_tests/a.yaml::t1", "_tests/a.yaml::t2"]})


if __name__ == "__main__":
    unittest.main()
