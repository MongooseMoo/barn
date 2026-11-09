import contextlib
import hashlib
import io
import json
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from bench_differ import REPO
from fuzz_differential import (decode_corpus_file, exclusion, load_programs, main, outcome, probe,
                               render_choices, results)

# Real records copied byte for byte from Go's fuzz cache of the structured
# targets, as Go's internal/fuzz marshalCorpusFile wrote them.
FIXTURES = Path(__file__).resolve().parent / "fixtures" / "fuzz"
VM_RECORD = FIXTURES / "FuzzStructuredSourceExecution" / "e58adbf989baa13f"
COMPILER_RECORD = FIXTURES / "FuzzStructuredCompileVerifiable" / "0c4d4bbfffc14e70"


def go_owner_source(choices: bytes) -> str:
    """The program cmd/moo-fuzz-source prints for raw choice bytes on stdin."""
    return subprocess.run(["go", "run", "./cmd/moo-fuzz-source"], cwd=REPO, input=choices,
                          capture_output=True, check=True).stdout.decode("utf-8").removesuffix("\n")

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


class StructuredCorpusTests(unittest.TestCase):
    def test_real_record_renders_as_the_go_owner_renders_its_choices(self):
        rendered = render_choices([str(COMPILER_RECORD), str(VM_RECORD)])
        compiler = rendered[str(COMPILER_RECORD)]
        # []byte("#_\"x\xa2"): '#', '_', '"', 'x', 0xa2.
        self.assertEqual(compiler["choices"], "235f2278a2")
        self.assertNotIn("error", compiler)
        for record in rendered.values():
            self.assertEqual(record["source"], go_owner_source(bytes.fromhex(record["choices"])))
        # Go's %q leaves printable UTF-8 runs raw among \x escapes.
        self.assertIn("de9a", rendered[str(VM_RECORD)]["choices"])

    def test_lanes_keep_choices_and_count_undecodable_records(self):
        with tempfile.TemporaryDirectory() as directory:
            structured, strings = Path(directory, "structured"), Path(directory, "strings")
            structured.mkdir()
            strings.mkdir()
            shutil.copy(COMPILER_RECORD, structured)
            shutil.copy(VM_RECORD, structured)
            Path(structured, "bad-escape").write_bytes(b'go test fuzz v1\n[]byte("\\q")\n')
            Path(structured, "two-values").write_bytes(b'go test fuzz v1\n[]byte("a")\n[]byte("b")\n')
            Path(strings, "program").write_bytes(b'go test fuzz v1\nstring("return 1;")\n')
            with contextlib.redirect_stderr(io.StringIO()):
                programs, excluded, lanes = load_programs([structured, strings])
        compiled = programs[go_owner_source(bytes.fromhex("235f2278a2"))]
        self.assertEqual(compiled["choices"], "235f2278a2")
        self.assertEqual(compiled["lane"], str(structured))
        self.assertTrue(compiled["origin"].endswith("0c4d4bbfffc14e70"))
        self.assertNotIn("choices", programs["return 1;"])
        # The vm record's program forks, which is never compared.
        self.assertEqual(excluded, {"undecodable corpus record": 2, "fork": 1})
        self.assertEqual({k: v for k, v in lanes[str(structured)].items() if k != "undecodable"},
                         {"records": 4, "duplicates": 0, "excluded": 3, "eligible": 1, "completed": 0})
        self.assertEqual(sorted(Path(u["path"]).name for u in lanes[str(structured)]["undecodable"]),
                         ["bad-escape", "two-values"])
        self.assertEqual(lanes[str(strings)]["eligible"], 1)

    def test_undecodable_record_is_never_a_completed_comparison(self):
        with tempfile.TemporaryDirectory() as directory:
            lane, out = Path(directory, "lane"), Path(directory, "out")
            lane.mkdir()
            shutil.copy(COMPILER_RECORD, lane)
            Path(lane, "bad-escape").write_bytes(b'go test fuzz v1\n[]byte("\\q")\n')
            completed = '=> {"F00001", "value", {1, 0}}'
            lanes = {engine: ("\n".join(["=> 0", completed]), "") for engine in ("toast", "barn")}
            with (patch("sys.argv", ["fuzz_differential.py", str(lane), "--barn-linux", "barn", "--out", str(out)]),
                  patch("fuzz_differential.run_wsl", return_value=SimpleNamespace(
                      stdout="fixture-checksum\nToastStunt version fixture\n", stderr="", returncode=0)),
                  patch("fuzz_differential.run_lanes", return_value=lanes) as run_lanes,
                  contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO())):
                code = main()
            data = json.loads((out / "results.json").read_text(encoding="utf-8"))
        self.assertEqual(code, 0)
        # The preamble and the one decodable program; nothing for the bad record.
        self.assertEqual(len(run_lanes.call_args.args[3]), 2)
        summary = data["summary"]
        self.assertEqual((summary["loaded"], summary["eligible"], summary["completed_pairs"]), (2, 1, 1))
        self.assertEqual(data["excluded"], {"undecodable corpus record": 1})
        self.assertEqual(summary["lanes"][str(lane)]["completed"], 1)
        self.assertEqual(len(summary["lanes"][str(lane)]["undecodable"]), 1)
        self.assertEqual(data["rows"][0]["choices"], "235f2278a2")
        self.assertEqual(data["rows"][0]["source"], go_owner_source(bytes.fromhex("235f2278a2")))


class ExclusionTests(unittest.TestCase):
    def test_nondeterministic_calls_and_words(self):
        self.assertEqual(exclusion("return ticks_left();"), "ticks_left(): tick budget")
        self.assertEqual(exclusion("fork (0) endfork"), "fork")
        self.assertEqual(exclusion("$server_options.fg_ticks = 1;"), "server_options")

    def test_plain_program_and_lookalike_names(self):
        self.assertIsNone(exclusion("x = {0..0}; return x[1];"))
        self.assertIsNone(exclusion("return times;"))

    def test_deferred_comparison_is_counted_and_other_sources_remain(self):
        fingerprint = hashlib.sha256(b"return 1;").hexdigest()
        with tempfile.TemporaryDirectory() as directory:
            lane = Path(directory)
            Path(lane, "deferred").write_bytes(b'go test fuzz v1\nstring("return 1;")\n')
            Path(lane, "ordinary").write_bytes(b'go test fuzz v1\nstring("return 2;")\n')
            with patch("fuzz_differential.DEFERRED_COMPARISONS", {fingerprint}):
                programs, excluded, lanes = load_programs([lane])
        self.assertEqual(list(programs), ["return 2;"])
        self.assertEqual(excluded, {"deferred comparison": 1})
        self.assertEqual(lanes[str(lane)]["excluded"], 1)
        self.assertEqual(lanes[str(lane)]["eligible"], 1)


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
        programs = {f"return {i};": {"origin": f"corpus/{i}", "lane": "corpus"} for i in range(len(toast))}
        lanes_counts = {"corpus": {"records": len(toast), "duplicates": 0, "excluded": 0,
                                   "eligible": len(toast), "completed": 0, "undecodable": []}}
        lanes = {"toast": ("\n".join(["=> 0"] + toast), ""),
                 "barn": ("\n".join(["=> 0"] + barn), "")}
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            with (patch("sys.argv", ["fuzz_differential.py", "corpus", "--barn-linux", "barn",
                                     "--out", directory]),
                  patch("fuzz_differential.load_programs", return_value=(programs, excluded or {}, lanes_counts)),
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
