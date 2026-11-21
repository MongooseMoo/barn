import json
import tempfile
import unittest
from pathlib import Path

from fuzz_cross_engine import (CHUNK, DIRECTIONS, TEST_PREFIX, classify, failure_class, junit_results,
                               load_programs, render_report, suite, summary_line, test_case)
from fuzz_report import report

FIXTURES = Path(__file__).resolve().parent / "fixtures" / "fuzz"
# Real harness results (--junitxml) of scripts/fuzz_cross_engine.py on four
# one-layer seeds (choices 0000010000, 0000010101, 0000010200, 0000000000):
# Barn from before PR 886 writing, Toast resuming; and Toast writing, master
# Barn resuming, where the call_function("eval", ...) seed does not boot.
PRE886_BARN_FIRST = FIXTURES / "cross-engine" / "pre886-barn-first.xml"
MASTER_TOAST_FIRST = FIXTURES / "cross-engine" / "master-toast-first.xml"
# Byte-for-byte records from Go's fuzz cache for FuzzSuspendedStackCheckpoint;
# two of them render the same program.
CORPUS = FIXTURES / "FuzzSuspendedStackCheckpoint"

EVAL_PROGRAM = "\n".join([
    "o = create(#-1);",
    'add_property(#0, "suspended_stack", o, {o.owner, "r"});',
    "results = {};",
    'r1 = call_function("eval", "suspend(); v = 0; return {1, v};");',
    "if (!r1[1]) raise(E_INVARG, tostr(1), r1[2]); endif",
    "v = r1;",
    "results = {@results, v};",
    "return results;",
])


def program(name, source=EVAL_PROGRAM):
    return {"name": name, "origin": f"/corpus/{name}", "lane": "/corpus", "choices": "0000010200",
            "source": source}


class GeneratedTestTests(unittest.TestCase):
    def test_suite_is_json_yaml_with_one_restart_test_per_program(self):
        text = json.dumps(suite([program("generated_a"), program("generated_b")]))
        loaded = json.loads(text)
        self.assertEqual([test["name"] for test in loaded["tests"]], ["generated_a", "generated_b"])
        steps = loaded["tests"][0]["steps"]
        self.assertEqual([next(iter(step)) for step in steps],
                         ["run", "run", "run", "run", "restart_server", "run", "run"])
        # The checkpoint is waited for, so the restart reads this run's dump.
        self.assertEqual(steps[4]["restart_server"]["checkpoint_timeout_ms"], 10000)
        # Every code step starts with the number failure_class reads.
        self.assertEqual([step["run"].split(";", 1)[0] for step in steps if "run" in step],
                         ['"1 install"', '"2 run A"', '"3 start B"', '"4 checkpoint"', '"6 resume B"',
                          '"7 compare"'])
        self.assertEqual(loaded["tests"][0]["permission"], "wizard")

    def test_program_lines_are_installed_as_moo_string_literals(self):
        setup = test_case("generated_a", EVAL_PROGRAM)["steps"][0]["run"]
        # The quote and the backslash-free eval() source survive as one
        # literal per line, in order.
        self.assertIn('"r1 = call_function(\\"eval\\", \\"suspend(); v = 0; return {1, v};\\");"', setup)
        line = next(line for line in setup.split("\n") if 'set_verb_code(h, "run"' in line)
        self.assertEqual(line.count('", "') + 1, len(EVAL_PROGRAM.split("\n")))
        backslash = test_case("generated_a", 'x = "a\\\\b";')["steps"][0]["run"]
        self.assertIn('{"x = \\"a\\\\\\\\b\\";"}', backslash)


class ResultTests(unittest.TestCase):
    def test_pre886_export_divergence_is_classified_as_a_changed_result(self):
        results = junit_results(PRE886_BARN_FIRST)
        names = [program(name.split("::")[1].rstrip("]")) for name in results if TEST_PREFIX in name]
        rows, problems = classify(names, [("barn-first", names, results)])
        self.assertEqual(problems, [])
        kinds = {row["name"]: (row["kind"], row.get("class")) for row in rows}
        self.assertEqual(kinds["generated_8855508aade16ec5"], ("match", None))
        changed = ("divergence", "barn-first: the result differs after the restart")
        for name in ("generated_00d4713429e4fd21", "generated_8e0c5acc0a2f5318", "generated_3c477a14bb6f1b4e"):
            self.assertEqual(kinds[name], changed)
        eval_0 = next(row for row in rows if row["name"] == "generated_00d4713429e4fd21")
        self.assertEqual(summary_line(eval_0["message"]),
                         "expected value 1, but got ['no restart', [['return', [[1, [1, 0]]]], []], "
                         "'restarted', [['return', [[1, 0]]], []]]")

    def test_master_import_failure_is_classified_as_a_server_that_did_not_boot(self):
        results = junit_results(MASTER_TOAST_FIRST)
        names = [program(name.split("::")[1].rstrip("]")) for name in results if TEST_PREFIX in name]
        rows, problems = classify(names, [("toast-first", names, results)])
        self.assertEqual(problems, [])
        failed = [row for row in rows if row["kind"] == "divergence"]
        self.assertEqual([row["name"] for row in failed], ["generated_3c477a14bb6f1b4e"])
        self.assertEqual(failed[0]["class"], "toast-first: the other engine did not boot on the checkpoint")
        line = summary_line(failed[0]["message"])
        self.assertTrue(line.startswith("moo_conformance.server.RestartCommandError: Managed server exited"), line)
        self.assertIn('level=ERROR msg="Barn failed" err="load database: restore suspended tasks: suspended task '
                      '963952496: native frame 3: native builtin continuation requires conversion"', line)

    def test_a_session_without_results_or_with_a_failed_admission_is_inconclusive(self):
        chunk = [program("generated_a")]
        rows, problems = classify(chunk, [("barn-first", chunk, None)])
        self.assertEqual([row["kind"] for row in rows], ["missing"])
        self.assertIn("barn-first: the harness wrote no results", problems)
        admission = {"test_capability_admission": {"outcome": "failed", "message": "probe failed"},
                     "test_yaml_conformance[x.yaml::generated_a]": {"outcome": "skipped", "message": "s"}}
        rows, problems = classify(chunk, [("toast-first", chunk, admission)])
        self.assertEqual([row["kind"] for row in rows], ["skipped"])
        self.assertIn("toast-first: harness test test_capability_admission failed: probe failed", problems)

    def test_failure_class_names_the_step_or_the_restart(self):
        self.assertEqual(failure_class("barn-first", "Test 'step '\"6 resume B\";\nreturn #0....'' expected"),
                         "barn-first: the restored tasks did not finish")
        self.assertEqual(failure_class("toast-first", "Test 'step '\"1 install\";\nh = cre...'' got"),
                         "toast-first: the program did not install")
        self.assertEqual(failure_class("barn-first", "something else"), "barn-first: the test failed")


class ReportTests(unittest.TestCase):
    def test_report_is_a_divergence_the_reporter_files_under_its_class(self):
        results = junit_results(MASTER_TOAST_FIRST)
        names = [program(name.split("::")[1].rstrip("]")) for name in results if TEST_PREFIX in name]
        rows, problems = classify(names, [("toast-first", names, results)])
        summary = {"outcome": "divergence", "programs": 4, "counts": {"match": 3, "divergence": 1},
                   "problems": problems, "lanes": {}, "toast": "toaststunt/build/moo", "toast_sha256": "t",
                   "barn": "bin/barn", "barn_sha256": "b"}
        text = render_report(summary, rows)
        result = report("go version go1.27.1 linux/amd64\n" + text, "./scripts", "ToastCrossEngineCheckpoint",
                        "commit")
        self.assertEqual(result["kind"], "divergence")
        self.assertEqual(result["input_path"], "generated_3c477a14bb6f1b4e")
        self.assertEqual(result["input_text"], EVAL_PROGRAM)
        first = result["first_error"].split("\n")
        self.assertEqual(first[0], "## divergence (1)")
        self.assertTrue(first[1].startswith(f"- {DIRECTIONS['toast-first']}: moo_conformance.server."
                                            "RestartCommandError"), first[1])
        self.assertEqual(first[2], "- test: `generated_3c477a14bb6f1b4e` in `toast-first`")
        self.assertEqual(first[3], "- toast binary: `toaststunt/build/moo` sha256 `t`")
        # The signature is the failure class, so another input with the same
        # failure in a later cycle is the same issue.
        other = report("go version go1.27.1 linux/amd64\n" + text.replace("generated_3c477a14bb6f1b4e", "generated_x"),
                       "./scripts", "ToastCrossEngineCheckpoint", "commit")
        self.assertEqual(other["signature"], result["signature"])
        changed = report("go version go1.27.1 linux/amd64\n" + text.replace(
            "signature: toast-first: the other engine did not boot",
            "signature: toast-first: the restored tasks did not finish"), "./scripts", "ToastCrossEngineCheckpoint",
            "commit")
        self.assertNotEqual(changed["signature"], result["signature"])


class CorpusTests(unittest.TestCase):
    def test_corpus_records_render_once_per_distinct_program(self):
        with tempfile.TemporaryDirectory() as directory:
            missing = Path(directory) / "missing"
            programs, lanes = load_programs([CORPUS, missing])
        self.assertEqual(lanes[str(CORPUS)], {"records": 3, "duplicates": 1, "programs": 2, "undecodable": []})
        self.assertEqual(lanes[str(missing)]["records"], 0)
        self.assertEqual(len(programs), 2)
        for record in programs:
            self.assertTrue(record["name"].startswith(TEST_PREFIX))
            self.assertIn("suspend()", record["source"])
            self.assertTrue(record["source"].endswith("return results;"))
        self.assertGreater(CHUNK, 0)


if __name__ == "__main__":
    unittest.main()
