"""The live fuzz page's classification, run in Node on records shaped like
GitHub's API output (values copied from barn's runs, issues and PRs on
2026-11-11/12)."""

import json
import os
from pathlib import Path
import subprocess
import unittest

from test_fuzz_compute import node_binary


ROOT = Path(__file__).resolve().parents[1]
MODULE = ROOT / "docs/live/fuzz_live.js"
NOW = "2026-11-12T01:25:00Z"
RUN = 43109992532


def call(function, *args):
    harness = r"""
const input = JSON.parse(require('node:fs').readFileSync(0, 'utf8'));
const live = require(input.module);
console.log(JSON.stringify(live[input.function](...input.args)));
"""
    result = subprocess.run([node_binary(), "-e", harness], input=json.dumps(
        {"module": str(MODULE), "function": function, "args": list(args)}),
        text=True, capture_output=True, check=True, env={**os.environ, "TZ": "America/New_York"})
    return json.loads(result.stdout)


def issue(number, title, run, state="open", created="2026-11-10T12:07:45Z", updated="2026-11-10T12:07:45Z",
          closed=None):
    return {"number": number, "title": title, "state": state, "created_at": created, "updated_at": updated,
            "closed_at": closed, "html_url": f"https://github.com/MongooseMoo/barn/issues/{number}",
            "body": f"<!-- fuzz-signature: x -->\n\nTarget: ./x\n\n"
                    f"Run: https://github.com/MongooseMoo/barn/actions/runs/{run}/attempts/1\n\nKind: panic"}


def job(name, conclusion="success", failed=(), status="completed",
        started="2026-11-11T23:17:07Z", completed="2026-11-11T23:28:08Z"):
    steps = [{"name": "Check /dev/null is a device", "conclusion": "success"},
             {"name": f"Fuzz {name}", "conclusion": "success"},
             {"name": "Open failure issues", "conclusion": "skipped"}]
    steps = [{**s, "conclusion": "failure"} if s["name"] in failed else s for s in steps]
    steps += [{"name": n, "conclusion": "failure"} for n in failed if n not in {s["name"] for s in steps}]
    return {"name": name, "status": status, "conclusion": conclusion if status == "completed" else None,
            "started_at": started, "completed_at": completed if status == "completed" else None,
            "steps": steps, "html_url": "https://github.com/MongooseMoo/barn/actions/runs/1/job/2"}


ISSUES = [
    issue(722, "Nightly fuzz failure: FuzzReadDatabase", 42808491320, updated="2026-11-11T23:17:46Z"),
    issue(717, "Nightly fuzz infrastructure failure: FuzzReadDatabase", 42790517087,
          created="2026-11-10T10:06:37Z", updated="2026-11-11T23:28:05Z"),
    issue(761, "Nightly fuzz failure: FuzzReadDatabase", 42993955078,
          created="2026-11-11T10:39:49Z", updated="2026-11-11T20:56:32Z"),
    issue(765, "Nightly fuzz failure: FuzzCommandLine", 43062378082,
          created="2026-11-11T18:08:32Z", updated="2026-11-11T21:07:21Z"),
    issue(771, "Nightly fuzz infrastructure failure: FuzzStructuredSourceExecution", 43087028220,
          created="2026-11-11T20:34:11Z", updated="2026-11-11T20:34:11Z"),
    issue(757, "Nightly fuzz failure: FuzzCheckpointResumeEquivalence", 42972636456, state="closed",
          created="2026-11-11T08:06:07Z", updated="2026-11-11T21:19:31Z", closed="2026-11-11T21:19:31Z"),
    issue(742, "Nightly fuzz failure: FuzzCheckpointResumeEquivalence", 42937999782, state="closed",
          created="2026-11-11T03:17:35Z", updated="2026-11-11T05:28:37Z", closed="2026-11-11T05:07:25Z"),
    issue(735, "Nightly fuzz failure: FuzzCheckpointResumeEquivalence", 42871469786, state="closed",
          created="2026-11-10T19:40:33Z", updated="2026-11-10T22:14:10Z", closed="2026-11-10T21:28:44Z"),
    {"number": 774, "title": "Share property resolution across transactions", "state": "closed",
     "created_at": "2026-11-12T00:19:50Z", "updated_at": "2026-11-12T00:34:15Z", "body": "",
     "pull_request": {"merged_at": "2026-11-12T00:33:34Z"}},
]


def pull(number, ref, merged, created="2026-11-11T08:10:47Z", state="closed"):
    return {"number": number, "head": {"ref": ref}, "state": state, "merged_at": merged,
            "created_at": created, "title": f"PR {number}",
            "html_url": f"https://github.com/MongooseMoo/barn/pull/{number}"}


PULLS = [
    pull(774, "perf/shared-prop-resolve-memo", "2026-11-12T00:33:34Z"),
    pull(758, "claude/issue-757", "2026-11-11T21:19:29Z"),
    pull(743, "claude/issue-742", "2026-11-11T05:07:23Z"),
    pull(736, "claude/issue-735", "2026-11-10T21:28:42Z"),
    pull(730, "claude/issue-729", None),
]


class LaneTests(unittest.TestCase):
    def kinds(self, lane, run=RUN, now=NOW):
        return [(o["kind"], o.get("issue", {}).get("number")) for o in call("classifyJob", lane, run, ISSUES, now)]

    def test_repeat_finding_then_in_step_machine_problem_in_one_lane(self):
        # Run 43109992532: #722 got the finding at 23:17:46, #717 the
        # infrastructure report at 23:28:05; only the campaign step failed.
        lane = job("FuzzReadDatabase", "failure", ["Fuzz FuzzReadDatabase"])
        self.assertEqual(self.kinds(lane), [("repeat", 722), ("machine", 717)])

    def test_new_issue_from_this_run_is_a_bug_found(self):
        lane = job("FuzzCommandLine", "failure", ["Fuzz FuzzCommandLine"],
                   started="2026-11-11T17:50:00Z", completed="2026-11-11T18:20:00Z")
        self.assertEqual(self.kinds(lane, 43062378082), [("bug", 765)])

    def test_new_infrastructure_issue_from_this_run_is_a_machine_problem(self):
        lane = job("FuzzStructuredSourceExecution", "failure", ["Fuzz FuzzStructuredSourceExecution"],
                   started="2026-11-11T20:10:00Z", completed="2026-11-11T20:35:00Z")
        self.assertEqual(self.kinds(lane, 43087028220), [("machine", 771)])

    def test_job_level_failures_are_machine_problems(self):
        cancelled = job("FuzzReadDatabase", "cancelled")
        self.assertEqual(self.kinds(cancelled), [("machine", None)])
        lost = job("FuzzVerifyProgram", "failure")
        self.assertEqual(self.kinds(lost), [("machine", None)])
        setup = job("FuzzVerifyProgram", "failure", ["Check /dev/null is a device"])
        out = call("classifyJob", setup, RUN, ISSUES, NOW)
        self.assertEqual([o["kind"] for o in out], ["machine"])
        self.assertIn("Check /dev/null is a device", out[0]["why"])

    def test_a_clean_lane_ignores_issue_activity_in_its_window(self):
        self.assertEqual(self.kinds(job("FuzzReadDatabase")), [("clean", None)])

    def test_red_campaign_with_no_matching_report_is_unmatched(self):
        lane = job("FuzzTCPReadInput", "failure", ["Fuzz FuzzTCPReadInput"])
        self.assertEqual(self.kinds(lane), [("unmatched", None)])

    def test_running_lane_shows_what_it_found_so_far(self):
        lane = job("FuzzCommandLine", status="in_progress", started="2026-11-11T18:00:00Z")
        self.assertEqual(self.kinds(lane, 43062378082, "2026-11-11T18:10:00Z"),
                         [("running", None), ("bug", 765)])
        queued = job("FuzzCommandLine", status="queued")
        self.assertEqual(self.kinds(queued), [("waiting", None)])

    def test_repeat_of_a_closed_issue_says_it_was_fixed(self):
        # #742 closed at 05:07:25 and was commented on at 05:28:37.
        lane = job("FuzzCheckpointResumeEquivalence", "failure", ["Fuzz FuzzCheckpointResumeEquivalence"],
                   started="2026-11-11T05:10:00Z", completed="2026-11-11T05:40:00Z")
        out = call("classifyJob", lane, RUN, ISSUES, NOW)
        self.assertEqual([(o["kind"], o["issue"]["number"], o["issue"]["state"]) for o in out],
                         [("repeat", 742, "closed")])

    def test_closing_an_issue_is_not_a_repeat(self):
        # #757's last update is its closing by repair PR #758 at 21:19:31.
        lane = job("FuzzCheckpointResumeEquivalence", "failure", ["Fuzz FuzzCheckpointResumeEquivalence"],
                   started="2026-11-11T21:00:00Z", completed="2026-11-11T21:30:00Z")
        self.assertEqual(self.kinds(lane), [("unmatched", None)])

    def test_toast_differential_divergence_is_a_bug_found(self):
        diff = issue(780, "Nightly fuzz failure: ToastDifferential", RUN,
                     created="2026-11-12T00:20:00Z", updated="2026-11-12T00:20:00Z")
        lane = job("ToastDifferential", "failure", ["Compare retained public source on Barn and Toast"],
                   started="2026-11-12T00:00:00Z", completed="2026-11-12T00:25:00Z")
        out = call("classifyJob", lane, RUN, ISSUES + [diff], NOW)
        self.assertEqual([(o["kind"], o["issue"]["number"]) for o in out], [("bug", 780)])


class CycleTests(unittest.TestCase):
    JOBS = ([{"name": "Validate compute routing", "status": "completed", "conclusion": "success"}] +
            [{"name": f"Done{i}", "status": "completed", "conclusion": "success"} for i in range(4)] +
            [{"name": f"Run{i}", "status": "in_progress", "conclusion": None} for i in range(4)] +
            [{"name": f"Wait{i}", "status": "queued", "conclusion": None} for i in range(9)])

    def test_progress_counts_jobs_not_minutes(self):
        self.assertEqual(call("progress", self.JOBS), {"total": 17, "done": 4, "running": 4, "waiting": 9})

    def test_health_comes_from_the_accept_job(self):
        accept = lambda c: [{"name": "Accept discovery cycle", "status": "completed", "conclusion": c}]
        self.assertEqual(call("health", accept("success")), "healthy")
        self.assertEqual(call("health", accept("failure")), "not healthy")
        self.assertEqual(call("health", accept("skipped")), "not checked")
        self.assertEqual(call("health", self.JOBS), "pending")

    def test_last_day_counts_new_bug_issues_and_merged_repairs(self):
        day = call("lastDay", ISSUES, PULLS, NOW)
        self.assertEqual([(b["number"], b["pr"] and b["pr"]["number"]) for b in day["bugs"]],
                         [(765, None), (761, None), (757, 758), (742, 743)])
        self.assertEqual([p["number"] for p in day["fixes"]], [758, 743])

    def test_summary_leads_with_state_progress_and_the_last_day(self):
        text = call("summary", {"current": {"id": 1}, "currentJobs": self.JOBS, "previous": None,
                                "issues": ISSUES, "pulls": PULLS, "now": NOW, "locale": "en-US"})
        self.assertEqual(text, "Running now: 4 of 17 lanes done, 4 running, 9 waiting. "
                               "Last 24 hours: 4 new bugs found, 2 fixes merged.")

    def test_summary_when_waiting_finishing_or_stopped(self):
        waiting = [j if j["status"] != "in_progress" else {**j, "status": "queued"} for j in self.JOBS]
        base = {"current": {"id": 1}, "previous": None, "issues": [], "pulls": [], "now": NOW, "locale": "en-US"}
        self.assertEqual(call("summary", {**base, "currentJobs": waiting}),
                         "Waiting for a runner: 4 of 17 lanes done, 13 waiting. "
                         "Last 24 hours: 0 new bugs found, 0 fixes merged.")
        finishing = [{**j, "status": "completed", "conclusion": "success"} for j in self.JOBS] + [
            {"name": "Accept discovery cycle", "status": "in_progress", "conclusion": None}]
        self.assertTrue(call("summary", {**base, "currentJobs": finishing}).startswith(
            "Running now: all 17 lanes done, finishing the cycle. "))
        starting = self.JOBS[:1]
        self.assertTrue(call("summary", {**base, "currentJobs": starting}).startswith("Starting a cycle. "))
        stopped = call("summary", {**base, "current": None, "currentJobs": [],
                                   "previous": {"id": 2, "updated_at": "2026-11-12T00:30:28Z"},
                                   "issues": ISSUES[3:4], "pulls": PULLS[1:2]})
        self.assertEqual(stopped, "Stopped: no cycle is running or queued; the last one ended at 7:30 PM EST. "
                                  "Last 24 hours: 1 new bug found, 1 fix merged.")


class TimeTests(unittest.TestCase):
    def test_relative_times(self):
        cases = {"2026-11-12T01:24:40Z": "just now", "2026-11-12T01:24:00Z": "1 minute ago",
                 "2026-11-12T01:11:00Z": "14 minutes ago", "2026-11-12T00:25:00Z": "1 hour ago",
                 "2026-11-11T23:20:00Z": "2 hours 5 minutes ago", "2026-11-09T01:25:00Z": "3 days ago"}
        for iso, text in cases.items():
            self.assertEqual(call("ago", iso, NOW), text, iso)

    def test_local_times_name_the_zone(self):
        self.assertEqual(call("local", "2026-11-12T01:12:00Z", NOW, "en-US"), "8:12 PM EST")
        self.assertEqual(call("local", "2026-11-11T19:25:00Z", "2026-11-12T15:00:00Z", "en-US"),
                         "Wed, Nov 11, 2:25 PM EST")
        self.assertEqual(call("when", "2026-11-12T01:11:00Z", NOW, "en-US"), "14 minutes ago (8:11 PM EST)")
        self.assertEqual(call("duration", 2 * 3600000 + 16 * 60000), "2 hours 16 minutes")
        self.assertEqual(call("duration", 30000), "less than a minute")


if __name__ == "__main__":
    unittest.main()
