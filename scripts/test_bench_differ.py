"""Exercise the benchmark's verdict using raw engine result shapes."""

import contextlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import bench_differ


class BenchmarkVerdictTests(unittest.TestCase):
    def run_report(self, toast_value="{1, 7}", barn_value="{1, 7}", missing=False):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            corpus = output / "corpus.txt"
            corpus.write_text("map: return 7;\n", encoding="utf-8")
            lanes = {
                "toast": (f'=> {{"K0000", 1.0, {toast_value}, 1.1}}\n', 0.1),
                "barn": ("" if missing else f'=> {{"K0000", 1.0, {barn_value}, 1.1}}\n',
                         0.1),
            }
            arguments = ["bench_differ.py", "--corpus", str(corpus), "--repeats", "1",
                         "--out", str(output), "--barn-linux", str(output / "barn")]
            with (patch.object(sys, "argv", arguments),
                  patch.object(bench_differ, "sha256", return_value="test-input"),
                  patch.object(bench_differ, "git_head", return_value="test-tree"),
                  patch.object(bench_differ, "run_lanes", return_value=lanes),
                  contextlib.redirect_stdout(io.StringIO()),
                  contextlib.redirect_stderr(io.StringIO())):
                status = bench_differ.main()
            report = json.loads((output / "results.json").read_text(encoding="utf-8"))
            return status, report

    def test_matching_successful_evals_pass(self):
        status, report = self.run_report()
        self.assertEqual(status, 0)
        self.assertTrue(report["rows"][0]["values_match"])

    def test_mismatch_fails_and_preserves_results(self):
        status, report = self.run_report(barn_value="{1, 8}")
        self.assertEqual(status, 1)
        self.assertFalse(report["rows"][0]["values_match"])
        self.assertEqual(report["rows"][0]["barn_value"], ["{1, 8}"])

    def test_matching_failed_evals_do_not_pass(self):
        status, _ = self.run_report("{0, {\"parse failed\"}}", "{0, {\"parse failed\"}}")
        self.assertEqual(status, 1)

    def test_missing_sample_fails(self):
        status, _ = self.run_report(missing=True)
        self.assertEqual(status, 1)

    def test_run_lanes_rejects_nonzero_status_and_preserves_raw_output(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            failed = subprocess.CompletedProcess([], 7, "raw stdout", "raw stderr")
            with patch.object(bench_differ, "run_wsl", return_value=failed):
                with self.assertRaisesRegex(RuntimeError, "toast.*7"):
                    bench_differ.run_lanes(output / "in.db", output / "barn",
                                          "/oracle/moo", [("input", ";input")], output, 10)
            self.assertIn("raw stdout", (output / "toast_raw.txt").read_text(encoding="utf-8"))
            self.assertIn("raw stderr", (output / "toast_raw.txt").read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
