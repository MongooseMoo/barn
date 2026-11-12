from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from goodput_series import LOAD_AFTER, change, findings, issue, milliseconds, point, run_results  # noqa: E402

# The line engine/mongoose_real_bench_test.go logs per player level, inside go test -v output.
OUTPUT = """=== RUN   TestMongooseRealWorkload
    mongoose_real_bench_test.go:965: players=1 goodput=209/s failed=0 uncaught=0 abort=0.00% elided=3 p50=4.12ms p99=31.50ms p999=40.01ms max=44.90ms allocs/op=43000 bytes/op=5500000 GCs=4
    mongoose_real_bench_test.go:965: players=16 goodput={goodput}/s failed={failed} uncaught=1 abort=21.50% elided=9 p50=950.5us p99=412.00ms p999=600.10ms max=801.00ms allocs/op=45000 bytes/op=6000000 GCs=31
--- PASS: TestMongooseRealWorkload (71.20s)
"""

# Sixteen-player commands/s with the run-to-run noise measured on one commit (707 to 892).
NOISY = [800, 760, 892, 707, 830, 790, 850, 745, 810, 880, 770, 820]


def output(goodput=800, failed=0):
    return OUTPUT.format(goodput=goodput, failed=failed)


def points(goodputs):
    return [{"commit": f"{index:012x}" + "0" * 28,"measured_at": f"2026-11-12T{index:02d}:00:00Z", "fixture": "f",
             "levels": {"16": {"goodput": value, "p99_ms": 400.0}, "1": {"p50_ms": 4.0}}}
            for index, value in enumerate(goodputs)]


class ParseTests(unittest.TestCase):
    def test_latency_units(self):
        self.assertEqual(milliseconds("31.50ms"), 31.5)
        self.assertAlmostEqual(milliseconds("950.5us"), 0.9505)
        with self.assertRaises(ValueError):
            milliseconds("1.20s")

    def test_run_results_reads_each_level(self):
        results = run_results(output(goodput=812))
        self.assertEqual(results["1"], {"goodput": 209.0, "p50_ms": 4.12, "p99_ms": 31.5, "failed": 0, "uncaught": 0})
        self.assertEqual(results["16"]["goodput"], 812.0)
        self.assertAlmostEqual(results["16"]["p50_ms"], 0.9505)

    def test_point_is_the_median_of_the_runs(self):
        made = point([output(707), output(892), output(800)], ["1", "16"])
        self.assertEqual(made["16"]["goodput"], 800.0)
        self.assertEqual(made["16"]["goodput_runs"], [707.0, 892.0, 800.0])
        self.assertEqual(made["1"]["p50_ms"], 4.12)

    def test_load_after_a_run_is_read_from_the_line_the_workflow_appends(self):
        self.assertEqual(LOAD_AFTER.findall(output() + "load_after=13.72\n"), ["13.72"])
        self.assertEqual(LOAD_AFTER.findall(output()), [])

    def test_point_records_uncaught_errors_without_refusing(self):
        self.assertEqual(point([output()], ["16"])["16"]["uncaught"], 1)

    def test_point_refuses_a_run_with_failed_commands(self):
        with self.assertRaisesRegex(ValueError, "run 2 at 16 players: 3 commands failed"):
            point([output(), output(failed=3)], ["1", "16"])

    def test_point_refuses_a_run_without_a_level(self):
        with self.assertRaisesRegex(ValueError, "run 1 has no result for 64 players"):
            point([output()], ["64"])


class ChangeTests(unittest.TestCase):
    def test_noise_alone_reports_nothing(self):
        self.assertIsNone(change(NOISY, -1))
        self.assertIsNone(change(NOISY + [707, 745, 760], -1))  # three low but ordinary points

    def test_a_lasting_drop_is_reported_once_three_points_show_it(self):
        dropped = [value * 0.75 for value in NOISY[:3]]
        self.assertIsNone(change(NOISY + dropped[:2], -1))
        count, was, now, shift = change(NOISY + dropped, -1)
        self.assertEqual(count, 3)
        self.assertEqual((was, now), (805.0, 600.0))
        self.assertAlmostEqual(shift, -0.2547, places=3)

    def test_the_whole_run_of_worse_points_is_found(self):
        self.assertEqual(change(NOISY + [value * 0.75 for value in NOISY[:6]], -1)[0], 6)

    def test_a_gain_reports_nothing(self):
        self.assertIsNone(change(NOISY + [value * 1.4 for value in NOISY[:4]], -1))

    def test_higher_latency_is_worse(self):
        steady = [400, 410, 395, 420, 405, 398, 415, 402, 408]
        self.assertEqual(change(steady + [520, 540, 515], +1)[0], 3)
        self.assertIsNone(change(steady + [300, 310, 305], +1))

    def test_too_little_history_reports_nothing(self):
        self.assertIsNone(change([800, 810, 790, 805, 600, 610, 590], -1))

    def test_a_lasting_five_percent_drop_reports_nothing(self):
        self.assertIsNone(change(NOISY + [value * 0.95 for value in NOISY[:8]], -1))


class FindingsTests(unittest.TestCase):
    def test_finding_names_the_commit_range(self):
        reasons = findings(points(NOISY + [600, 570, 669]))
        self.assertEqual(len(reasons), 1)
        self.assertIn("Commands per second at 16 players moved -25%", reasons[0])
        self.assertIn(f"{11:012x}...{12:012x}", reasons[0])

    def test_steady_series_finds_nothing(self):
        self.assertEqual(findings(points(NOISY)), [])

    def test_issue_carries_the_reasons_and_label(self):
        made = issue(["reason one"], "MongooseMoo/barn", "https://example.invalid/run/1")
        self.assertEqual(made["labels"], ["goodput-alarm"])
        self.assertIn("- reason one", made["body"])
        self.assertIn("Nothing has been stopped", made["body"])


if __name__ == "__main__":
    unittest.main()
