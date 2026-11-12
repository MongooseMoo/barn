import contextlib
import hashlib
import io
import json
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import bench_differ
from bench_differ import REPO
from fuzz_differential import (TIMED_OUT, decode_corpus_file, engine_runner, exclusion, load_programs, main,
                               outcome, probe, render_choices, results, run_bounded)

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


HANG = object()


class FakeEngine:
    """Stands in for engine_runner's run(lane, lines, bound). outputs[lane][n]
    is the record the engine prints for input n (0 is the preamble, n is
    probe Fnnnnn). On HANG it stops printing and returns as coreutils
    timeout does after killing the engine (exit 124); the records printed
    before that are kept, as both engines flush them."""

    def __init__(self, outputs):
        self.outputs = outputs
        self.calls = []

    def __call__(self, lane, lines, bound):
        self.calls.append((lane, list(lines), bound))
        printed = []
        for line in lines:
            m = re.search(r'"F(\d{5})"', line)
            record = self.outputs[lane][int(m.group(1)) if m else 0]
            if record is HANG:
                return SimpleNamespace(returncode=124, stdout="\n".join(printed) + "\n", stderr="")
            printed.append(record)
        return SimpleNamespace(returncode=0, stdout="\n".join(printed) + "\n", stderr="")


class RawEngine(FakeEngine):
    """Prints a lane's whole transcript verbatim, whatever its record count,
    unless it holds a HANG."""

    def __call__(self, lane, lines, bound):
        if HANG in self.outputs[lane]:
            return super().__call__(lane, lines, bound)
        self.calls.append((lane, list(lines), bound))
        return SimpleNamespace(returncode=0, stdout="\n".join(self.outputs[lane]) + "\n", stderr="")


def value(key, v=1):
    return f'=> {{"{key}", "value", {{1, {v}}}}}'


class BoundedRunTests(unittest.TestCase):
    def lines(self, n):
        return [";;preamble"] + [";;" + probe("F%05d" % i, f"return {i};") for i in range(1, n + 1)]

    def test_hanging_probe_times_out_and_later_probes_still_run(self):
        outputs = {"toast": ["=> 0", value("F00001"), HANG, value("F00003"), value("F00004")]}
        engine = FakeEngine(outputs)
        with tempfile.TemporaryDirectory() as directory:
            records, errors, timed_out = run_bounded("toast", engine, self.lines(4), Path(directory),
                                                     probe_timeout=10, startup=30, chunk=20)
            transcript = Path(directory, "toast_raw.txt").read_text(encoding="utf-8")
        self.assertEqual(records, ["=> 0", value("F00001"), TIMED_OUT, value("F00003"), value("F00004")])
        self.assertEqual((errors, timed_out), ([], [2]))
        # The partial transcript of the killed process is kept as evidence.
        self.assertIn(value("F00001"), transcript)
        self.assertIn("exit 124", transcript)
        first, second = engine.calls
        # Each process is bounded by its own inputs, not by Toast's limits.
        self.assertEqual(first[2], 30 + 10 * 5)
        # The restart replays the probes before the stuck one, so later
        # probes see the same database state as in one process, and skips it.
        self.assertEqual(second[1], [self.lines(4)[i] for i in (0, 1, 3, 4)])
        self.assertEqual(second[2], 30 + 10 * 4)

    def test_chunks_bound_processes_and_each_starts_with_the_preamble(self):
        outputs = {"barn": ["=> 0"] + [value("F%05d" % i) for i in range(1, 6)]}
        engine = FakeEngine(outputs)
        with tempfile.TemporaryDirectory() as directory:
            records, errors, timed_out = run_bounded("barn", engine, self.lines(5), Path(directory),
                                                     probe_timeout=10, startup=30, chunk=2)
        self.assertEqual(records, outputs["barn"])
        self.assertEqual((errors, timed_out), ([], []))
        self.assertEqual([len(lines) for _, lines, _ in engine.calls], [3, 3, 2])
        self.assertTrue(all(lines[0] == ";;preamble" for _, lines, _ in engine.calls))

    def test_two_hanging_probes_in_one_chunk(self):
        outputs = {"toast": ["=> 0", HANG, value("F00002"), HANG]}
        with tempfile.TemporaryDirectory() as directory:
            records, errors, timed_out = run_bounded("toast", FakeEngine(outputs), self.lines(3), Path(directory),
                                                     probe_timeout=1, startup=1, chunk=20)
        self.assertEqual(records, ["=> 0", TIMED_OUT, value("F00002"), TIMED_OUT])
        self.assertEqual((errors, timed_out), ([], [1, 3]))

    def test_record_count_mismatch_is_an_error(self):
        engine = lambda lane, lines, bound: SimpleNamespace(returncode=0, stdout="=> 0\n", stderr="")  # noqa: E731
        with tempfile.TemporaryDirectory() as directory:
            _, errors, _ = run_bounded("barn", engine, self.lines(1), Path(directory),
                                       probe_timeout=1, startup=1, chunk=20)
        self.assertEqual(len(errors), 1)

    def test_other_engine_failure_still_raises_with_raw_output_kept(self):
        engine = lambda lane, lines, bound: SimpleNamespace(returncode=7, stdout="raw", stderr="err")  # noqa: E731
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(RuntimeError, "toast.*7"):
                run_bounded("toast", engine, self.lines(1), Path(directory), probe_timeout=1, startup=1, chunk=20)
            self.assertIn("raw", Path(directory, "toast_raw.txt").read_text(encoding="utf-8"))

    def test_real_shell_kills_an_engine_that_ignores_sigterm(self):
        # A stand-in Toast that, like the pinned Toast in an empty named
        # while loop, never returns and ignores SIGTERM: the lane's coreutils
        # timeout must kill it and keep what it printed before.
        with tempfile.TemporaryDirectory() as directory:
            out = Path(directory)
            fake = out / "fake_toast.sh"
            fake.write_text("trap '' TERM\n"
                            "while IFS= read -r line; do\n"
                            "  case \"$line\" in\n"
                            "    quit) echo 'Bye.'; exit 0 ;;\n"
                            "    *HANG*) while :; do sleep 1; done ;;\n"
                            "    *) echo \"=> $line\" ;;\n"
                            "  esac\n"
                            "done\n", encoding="utf-8", newline="\n")
            db = out / "in.db"
            db.write_text("db", encoding="utf-8")
            lines = [";;preamble", ";;one", ";;HANG", ";;three"]
            run = engine_runner(db, out / "barn", "bash " + bench_differ.to_wsl(fake), out)
            records, errors, timed_out = run_bounded("toast", run, lines, out, probe_timeout=1, startup=1,
                                                     chunk=20)
        self.assertEqual(records, ["=> ;;preamble", "=> ;;one", TIMED_OUT, "=> ;;three"])
        self.assertEqual((errors, timed_out), ([], [2]))


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
            engine = FakeEngine({lane: ["=> 0", completed] for lane in ("toast", "barn")})
            with (patch("sys.argv", ["fuzz_differential.py", str(lane), "--barn-linux", "barn", "--out", str(out)]),
                  patch("fuzz_differential.run_wsl", return_value=SimpleNamespace(
                      stdout="fixture-checksum\nToastStunt version fixture\n", stderr="", returncode=0)),
                  patch("fuzz_differential.engine_runner", return_value=engine),
                  contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO())):
                code = main()
            data = json.loads((out / "results.json").read_text(encoding="utf-8"))
        self.assertEqual(code, 0)
        # The preamble and the one decodable program; nothing for the bad record.
        self.assertEqual([len(lines) for _, lines, _ in engine.calls], [2, 2])
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
        engine = RawEngine({"toast": ["=> 0"] + toast, "barn": ["=> 0"] + barn})
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            with (patch("sys.argv", ["fuzz_differential.py", "corpus", "--barn-linux", "barn",
                                     "--out", directory]),
                  patch("fuzz_differential.load_programs", return_value=(programs, excluded or {}, lanes_counts)),
                  patch("fuzz_differential.run_wsl", return_value=SimpleNamespace(
                      stdout="fixture-checksum\nToastStunt version fixture\n", stderr="", returncode=0)),
                  patch("fuzz_differential.engine_runner", return_value=engine),
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

    def test_toast_timeout_with_barn_abort_is_recorded_and_others_compared(self):
        # Issue #668: `n = 1; while o (n) endwhile` never returns on the
        # pinned Toast and Barn aborts it on ticks.
        completed = value("F00001")
        code, data, report = self.run_campaign([completed, HANG, value("F00003")],
                                               [completed, "=> *Aborted*", value("F00003")])
        self.assertEqual(code, 0)
        summary = data["summary"]
        self.assertEqual(summary["outcome"], "success")
        self.assertEqual(summary["completed_pairs"], 2)
        self.assertEqual(summary["timeouts"], {"toast": ["F00002"], "barn": []})
        self.assertTrue(summary["partial"])
        row = data["rows"][1]
        self.assertEqual(row["kind"], "aborted on both")
        self.assertEqual((row["toast_timed_out"], row["barn_timed_out"]), (True, False))
        self.assertEqual(row["toast_raw"], TIMED_OUT)
        self.assertIn("## timed out on toast (1)", report)

    def test_toast_timeout_with_barn_value_is_inconclusive(self):
        code, data, _ = self.run_campaign([HANG], [value("F00001")])
        self.assertEqual(code, 2)
        self.assertEqual(data["rows"][0]["kind"], "aborted on toast")
        self.assertEqual(data["summary"]["timeouts"], {"toast": ["F00001"], "barn": []})

    def test_timeout_on_both_engines(self):
        code, data, _ = self.run_campaign([value("F00001"), HANG], [value("F00001"), HANG])
        self.assertEqual(code, 0)
        self.assertEqual(data["rows"][1]["kind"], "aborted on both")
        self.assertEqual(data["summary"]["timeouts"], {"toast": ["F00002"], "barn": ["F00002"]})

    def test_record_count_error_cannot_hide_recovered_panic(self):
        panic = results(BARN_STDOUT)[3]
        code, data, _ = self.run_campaign([results(BARN_STDOUT)[1]], [panic, "=> *Aborted*"])
        self.assertEqual(code, 1)
        self.assertTrue(data["summary"]["record_errors"])
        self.assertEqual(data["summary"]["recovered_panics"], [panic])


if __name__ == "__main__":
    unittest.main()
