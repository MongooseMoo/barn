from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from conformance_suite_alarm import issue, spread, step_seconds, test_count, verdict  # noqa: E402

# The spread CI measured on 2026-11-11: 662-724 s around 680.
STEADY = [662, 670, 675, 680, 683, 690, 701, 724]


def job(conclusion="success", start="2026-11-11T08:00:00Z", end="2026-11-11T08:11:20Z"):
    return {"name": "MOO conformance suite", "steps": [
        {"name": "Build barn", "conclusion": "success",
         "started_at": "2026-11-11T07:59:00Z", "completed_at": "2026-11-11T08:00:00Z"},
        {"name": "Run conformance suite", "conclusion": conclusion, "started_at": start, "completed_at": end}]}


class VerdictTests(unittest.TestCase):
    def test_steady_weeks_and_equal_counts_raise_nothing(self):
        self.assertEqual(verdict(STEADY, list(reversed(STEADY)), 13804, 13804), [])

    def test_more_tests_raise_nothing(self):
        self.assertEqual(verdict(STEADY, STEADY, 13805, 13804), [])

    def test_fewer_tests_alarm(self):
        self.assertEqual(verdict(STEADY, STEADY, 13790, 13804),
                         ["The suite ran 13790 tests; the run before it ran 13804."])

    def test_eleven_percent_slower_alarms(self):
        reasons = verdict([s * 1.11 for s in STEADY], STEADY, 13804, 13804)
        self.assertEqual(len(reasons), 1)
        self.assertIn("+11.0% in a week", reasons[0])

    def test_eleven_percent_faster_alarms_too(self):
        self.assertIn("-11.0% in a week", verdict([s * 0.89 for s in STEADY], STEADY, 13804, 13804)[0])

    def test_nine_percent_raises_nothing(self):
        self.assertEqual(verdict([s * 1.09 for s in STEADY], STEADY, 13804, 13804), [])

    def test_a_week_with_too_few_runs_gives_no_time_verdict(self):
        self.assertEqual(verdict([2000, 2000, 2000, 2000], STEADY, 13804, 13804), [])
        self.assertEqual(verdict(STEADY, [], 13804, 13804), [])

    def test_missing_counts_give_no_count_verdict(self):
        self.assertEqual(verdict(STEADY, STEADY, None, 13804), [])
        self.assertEqual(verdict(STEADY, STEADY, 13804, None), [])


class SampleTests(unittest.TestCase):
    def test_step_seconds_reads_the_suite_step(self):
        self.assertEqual(step_seconds([{"name": "Build & unit tests", "steps": []}, job()]), 680)

    def test_skipped_or_failed_suite_has_no_time(self):
        self.assertIsNone(step_seconds([job(conclusion="skipped")]))
        self.assertIsNone(step_seconds([job(conclusion="failure")]))
        self.assertIsNone(step_seconds([{"name": "MOO conformance suite", "steps": []}]))
        self.assertIsNone(step_seconds([]))

    def test_test_count_counts_cases_in_nested_suites(self):
        report = ('<testsuites><testsuite tests="3"><testcase name="a"/><testcase name="b">'
                  '<failure/></testcase><testcase name="c"/></testsuite></testsuites>')
        self.assertEqual(test_count(report), 3)

    def test_issue_carries_the_reasons_and_the_label(self):
        made = issue(["The suite ran 13790 tests; the run before it ran 13804."], "https://example.test/run/1")
        self.assertEqual(made["labels"], ["conformance-suite-alarm"])
        self.assertIn("13790 tests", made["body"])
        self.assertIn("https://example.test/run/1", made["body"])
        self.assertIn("Nothing has been stopped", made["body"])

    def test_spread_keeps_both_ends(self):
        self.assertEqual(spread(list(range(100)), 12)[0], 0)
        self.assertEqual(spread(list(range(100)), 12)[-1], 99)
        self.assertEqual(len(spread(list(range(100)), 12)), 12)
        self.assertEqual(spread([1, 2, 3], 12), [1, 2, 3])


if __name__ == "__main__":
    unittest.main()
