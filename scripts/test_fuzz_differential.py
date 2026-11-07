import contextlib
import io
import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from fuzz_differential import decode_corpus_file, exclusion, main, outcome, probe, results

# Excerpts of real lane output from the first local run (2026-11-06).
TOAST_STDOUT = """
LambdaMOO Emergency Holographic Wizard Mode
-------------------------------------------
"Please state the nature of the wizardly emergency..."
(Type `help' for assistance.)

** Now running emergency commands as #3 ...

=> 0
=> {"F00001", "value", {0, {"Line 1:  syntax error"}}}
#3 <- VERB:
=> {"F00002", "raised", E_VARNF}
=> *Aborted*
Bye.  (saving database)

"""

BARN_STDOUT = """=> 0
=> {"F00001", "value", {1, {0}}}
=> {"F00002", "value", {1, "a
b"}}
Error: internal error: boom
-- skipped
"""


class CorpusFileTests(unittest.TestCase):
    def test_go_quoted_string(self):
        data = b'go test fuzz v1\nstring("return \\"a\\\\b\\";\\n\\treturn 1;")\n'
        self.assertEqual(decode_corpus_file(data), 'return "a\\b";\n\treturn 1;')

    def test_non_ascii_escape_is_decoded_then_excluded(self):
        source = decode_corpus_file(b'go test fuzz v1\nstring("A0000000\\xd0()")\n')
        self.assertEqual(exclusion(source), "non-ASCII or control character")

    def test_other_files_are_not_programs(self):
        self.assertIsNone(decode_corpus_file(b"go test fuzz v1\n[]byte(\"x\")\n"))
        self.assertIsNone(decode_corpus_file(b"not a corpus file\n"))


class ExclusionTests(unittest.TestCase):
    def test_nondeterministic_calls_and_words(self):
        self.assertEqual(exclusion("return ticks_left();"), "ticks_left(): tick budget")
        self.assertEqual(exclusion("fork (0) endfork"), "fork")
        self.assertEqual(exclusion("$server_options.fg_ticks = 1;"), "server_options")

    def test_plain_program_and_lookalike_names(self):
        self.assertIsNone(exclusion("x = {0..0}; return x[1];"))
        self.assertIsNone(exclusion("return times;"))


class ResultTests(unittest.TestCase):
    def test_toast_records_skip_banner_notify_and_bye(self):
        self.assertEqual(results(TOAST_STDOUT), [
            "=> 0",
            '=> {"F00001", "value", {0, {"Line 1:  syntax error"}}}',
            '=> {"F00002", "raised", E_VARNF}',
            "=> *Aborted*",
        ])

    def test_barn_record_spans_a_printed_newline(self):
        got = results(BARN_STDOUT)
        self.assertEqual(len(got), 4)
        self.assertEqual(outcome("F00002", got[2]), 'value {1, "a\nb"}')
        self.assertEqual(got[3], "Error: internal error: boom")

    def test_outcome_checks_the_key(self):
        record = '=> {"F00001", "value", {1, {0}}}'
        self.assertEqual(outcome("F00001", record), "value {1, {0}}")
        self.assertIsNone(outcome("F00002", record))
        self.assertIsNone(outcome("F00001", "=> *Aborted*"))

    def test_probe_passes_each_line_to_eval(self):
        self.assertEqual(
            probe("F00007", 'return "a\nb";'),
            'try r = eval("return \\"a", "b\\";"); return {"F00007", r[1] ? "value" | "compile", '
            'r[1] ? r[2] | 0}; except e (ANY) return {"F00007", "raised", e[1]}; endtry')


class CampaignTests(unittest.TestCase):
    def run_campaign(self, toast, barn, excluded=None):
        # These are lane transcripts, including the production preamble result.
        programs = {f"return {i};": f"corpus/{i}" for i in range(len(toast))}
        lanes = {"toast": ("\n".join(["=> 0"] + toast), ""),
                 "barn": ("\n".join(["=> 0"] + barn), "")}
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            with (patch("sys.argv", ["fuzz_differential.py", "corpus", "--barn-linux", "barn",
                                     "--out", directory]),
                  patch("fuzz_differential.load_programs", return_value=(programs, excluded or {})),
                  patch("fuzz_differential.run_wsl", return_value=SimpleNamespace(
                      stdout="fixture-checksum\nToastStunt version fixture\n", stderr="", returncode=0)),
                  patch("fuzz_differential.run_lanes", return_value=lanes),
                  contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO())):
                code = main()
            return code, json.loads((output / "results.json").read_text()), (output / "report.md").read_text()

    def test_real_abort_output_cannot_pass_without_completed_comparisons(self):
        aborted = results(TOAST_STDOUT)[-1]
        code, data, report = self.run_campaign([aborted], [aborted])
        self.assertEqual(code, 2)
        self.assertEqual(data["summary"]["completed_pairs"], 0)
        self.assertEqual(data["summary"]["aborted_pairs"], 1)
        self.assertEqual(data["summary"]["outcome"], "inconclusive")
        self.assertEqual(data["rows"][0]["toast_raw"], aborted)
        self.assertIn("completed comparisons: 0", report)

    def test_real_completed_output_counts_and_both_aborted_is_partial(self):
        # A completed keyed value taken from the existing real Barn transcript.
        completed = results(BARN_STDOUT)[1]
        aborted = results(TOAST_STDOUT)[-1]
        code, data, _ = self.run_campaign([completed, aborted], [completed, aborted], {"fork": 2})
        self.assertEqual(code, 0)
        self.assertEqual(data["summary"]["completed_pairs"], 1)
        self.assertEqual(data["summary"]["loaded"], 4)
        self.assertEqual(data["summary"]["excluded"], 2)
        self.assertTrue(data["summary"]["partial"])

    def test_completed_match_and_raised_error_are_comparisons(self):
        completed = results(TOAST_STDOUT)[1:3]
        code, data, _ = self.run_campaign(completed, completed)
        self.assertEqual(code, 0)
        self.assertEqual(data["summary"]["completed_pairs"], 2)
        self.assertFalse(data["summary"]["partial"])

    def test_divergence_remains_a_hard_error(self):
        code, data, _ = self.run_campaign([results(TOAST_STDOUT)[1]], [results(BARN_STDOUT)[1]])
        self.assertEqual(code, 1)
        self.assertEqual(data["summary"]["completed_pairs"], 1)
        self.assertEqual(data["summary"]["outcome"], "divergence")

    def test_one_sided_abort_is_inconclusive(self):
        completed = results(BARN_STDOUT)[1]
        code, data, _ = self.run_campaign([completed], ["=> *Aborted*"])
        self.assertEqual(code, 2)
        self.assertEqual(data["rows"][0]["kind"], "aborted on barn")

    def test_malformed_and_wrong_key_output_remains_unknown(self):
        for malformed in ('=> {"F00002", "value", 1}', '=> {"F00001", "value"', "Error: native failure"):
            with self.subTest(malformed=malformed):
                completed = results(BARN_STDOUT)[1]
                code, data, _ = self.run_campaign([completed], [malformed])
                self.assertEqual(code, 2)
                self.assertEqual(data["summary"]["unknown_pairs"], 1)
                self.assertEqual(data["rows"][0]["barn_raw"], malformed)

    def test_incomplete_lane_retains_records_and_is_inconclusive(self):
        completed = results(BARN_STDOUT)[1]
        code, data, _ = self.run_campaign([completed], [])
        self.assertEqual(code, 2)
        self.assertEqual(data["summary"]["completed_pairs"], 0)
        self.assertTrue(data["summary"]["record_errors"])
        self.assertEqual(data["records"]["toast"], ["=> 0", completed])

    def test_recovered_panic_is_divergence_even_without_completed_pair(self):
        code, data, _ = self.run_campaign(["=> *Aborted*"], [results(BARN_STDOUT)[3]])
        self.assertEqual(code, 1)
        self.assertEqual(data["summary"]["completed_pairs"], 0)
        self.assertEqual(data["rows"][0]["barn_raw"], "Error: internal error: boom")

    def test_record_count_error_cannot_hide_recovered_panic(self):
        panic = results(BARN_STDOUT)[3]
        code, data, _ = self.run_campaign([results(BARN_STDOUT)[1]], [panic, "=> *Aborted*"])
        self.assertEqual(code, 1)
        self.assertTrue(data["summary"]["record_errors"])
        self.assertEqual(data["summary"]["recovered_panics"], [panic])


if __name__ == "__main__":
    unittest.main()
