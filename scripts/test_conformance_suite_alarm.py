from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
import datetime  # noqa: E402
from unittest import mock  # noqa: E402

import conformance_suite_alarm  # noqa: E402
from conformance_suite_alarm import (WEEK, Sample, investigate, issue, parse_report, run_report,  # noqa: E402
                                     runtime_report, spread, timed_sample, verdict, weeks)

NOW = datetime.datetime(2026, 11, 15, 21, 0, tzinfo=datetime.timezone.utc)

# The spread CI measured on 2026-11-11: 662-724 s around 680.
STEADY = [662, 670, 675, 680, 683, 690, 701, 724]


def job(conclusion="success", start="2026-11-11T08:00:00Z", end="2026-11-11T08:11:20Z"):
    return {"name": "MOO conformance suite", "run_id": 17, "html_url": "job/23", "runner_name": "barn-06", "steps": [
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


class WeekTests(unittest.TestCase):
    def test_without_a_closed_alarm_the_weeks_are_whole(self):
        self.assertEqual(weeks(NOW, None), [(NOW - WEEK, NOW), (NOW - 2 * WEEK, NOW - WEEK)])

    def test_a_close_inside_the_week_before_trims_it(self):
        closed = NOW - WEEK - datetime.timedelta(days=2)
        self.assertEqual(weeks(NOW, closed), [(NOW - WEEK, NOW), (closed, NOW - WEEK)])

    def test_a_close_inside_the_newest_week_leaves_the_week_before_empty(self):
        # Closing #828 on 2026-11-15: no run-time verdict until a full week has passed.
        closed = NOW - datetime.timedelta(hours=1)
        (recent_start, recent_end), (earlier_start, earlier_end) = weeks(NOW, closed)
        self.assertEqual((recent_start, recent_end), (closed, NOW))
        self.assertGreaterEqual(earlier_start, earlier_end)


class SampleTests(unittest.TestCase):
    def test_step_seconds_reads_the_suite_step(self):
        self.assertEqual(timed_sample([{"name": "Build & unit tests", "steps": []}, job()]),
                         Sample(17, "job/23", "barn-06", 680))

    def test_skipped_or_failed_suite_has_no_time(self):
        self.assertIsNone(timed_sample([job(conclusion="skipped")]))
        self.assertIsNone(timed_sample([job(conclusion="failure")]))
        self.assertIsNone(timed_sample([{"name": "MOO conformance suite", "steps": []}]))
        self.assertIsNone(timed_sample([]))

    def test_test_count_counts_cases_in_nested_suites(self):
        report = ('<testsuites><testsuite tests="3"><testcase name="a"/><testcase name="b">'
                  '<failure/></testcase><testcase name="c"/></testsuite></testsuites>')
        parsed = parse_report(report)
        self.assertEqual(parsed.count, 3)
        self.assertIsNone(parsed.times)

    def test_weekly_attribution_uses_all_runs_and_includes_removed_cases(self):
        earlier = [Sample(1, "job/1", "one", 10), Sample(2, "job/2", "two", 14)]
        recent = [Sample(3, "job/3", "two", 20)]
        reports = {
            1: parse_report('<testsuite><testcase name="removed" time="4"/></testsuite>'),
            2: parse_report('<testsuite><testcase name="stayed" time="2"/></testsuite>'),
            3: parse_report('<testsuite><testcase name="stayed" time="2"/><testcase name="test_yaml[a.yaml::b]" time="3"/>'
                            '<testcase name="test_yaml[a.yaml::b]" time="4"/></testsuite>')}
        body = issue(["runtime changed"], "alarm", runtime_report(earlier, recent, reports))["body"]
        for line in ("3/3", "**+8.00 s**", "earlier week: +7.00 s", "including removals: -1.00 s",
                     "outside JUnit case timers: +2.00 s", "`a.yaml`: +7.00 s", "[2](job/2) | `two` | 14 s"):
            self.assertIn(line, body)

    def test_missing_artifacts_are_named_and_do_not_become_zero_time(self):
        earlier = [Sample(1, "job/1", "one", 10)]
        recent = [Sample(2, "job/2", "two", 20)]
        for artifacts, problem in (([], "artifact missing"), ([{"name": "conformance-results", "expired": True}],
                                                              "artifact expired")):
            with self.subTest(problem=problem), mock.patch.object(conformance_suite_alarm, "api",
                                                                  return_value={"artifacts": artifacts}):
                body = runtime_report(earlier, recent, {1: run_report("r", 1), 2: parse_report("<testsuite/>")})
                self.assertIn("1/2", body)
                self.assertIn(f"[1](job/1) | `one` | 10 s | {problem}", body)
                self.assertNotIn("outside JUnit case timers:", body)

    def test_damaged_zip_is_reported_as_unavailable(self):
        artifact = {"name": "conformance-results", "expired": False, "id": 1}
        with mock.patch.object(conformance_suite_alarm, "api", side_effect=[{"artifacts": [artifact]}, b"damaged"]):
            report = run_report("r", 17)
        self.assertIsNone(report.count)
        self.assertIsNone(report.times)
        self.assertEqual(report.problem, "unreadable JUnit report")

    def test_issue_carries_the_reasons_and_the_label(self):
        made = issue(["The suite ran 13790 tests; the run before it ran 13804."], "https://example.test/run/1")
        self.assertEqual(made["labels"], ["conformance-suite-alarm"])
        self.assertIn("13790 tests", made["body"])
        self.assertIn("https://example.test/run/1", made["body"])
        self.assertIn("Nothing has been stopped", made["body"])

    def test_investigate_dispatches_the_investigation_on_master(self):
        with mock.patch.object(conformance_suite_alarm, "api") as api:
            investigate("MongooseMoo/barn", 829)
        api.assert_called_once_with("repos/MongooseMoo/barn/actions/workflows/alarm-investigate.yml/dispatches",
                                    raw=True, post={"ref": "master", "inputs": {"issue_number": "829"}})

    def test_spread_keeps_both_ends(self):
        self.assertEqual(spread(list(range(100)), 12)[0], 0)
        self.assertEqual(spread(list(range(100)), 12)[-1], 99)
        self.assertEqual(len(spread(list(range(100)), 12)), 12)
        self.assertEqual(spread([1, 2, 3], 12), [1, 2, 3])


if __name__ == "__main__":
    unittest.main()
