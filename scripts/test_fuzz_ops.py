"""fuzz_ops parsing and verdicts against raw output captured on 2026-11-10."""

from datetime import datetime, timezone
import json
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
import fuzz_ops  # noqa: E402

FIXTURES = Path(__file__).resolve().parent / "fixtures/fuzz/ops"
NOW = datetime(2026, 11, 10, 8, 30, tzinfo=timezone.utc)


def fixture(name):
    return (FIXTURES / name).read_text()


class FuzzOpsTests(unittest.TestCase):
    def setUp(self):
        self.runs = json.loads(fixture("runs.json"))
        self.jobs = json.loads(fixture("jobs-42766490058.json"))["jobs"]
        self.runners = json.loads(fixture("runners.json"))["runners"]
        self.current = fuzz_ops.summarize_cycle(self.runs[0], self.jobs, NOW)

    def cycle(self, **overrides):
        return {**self.current, **overrides}

    def test_real_running_cycle_summary(self):
        c = self.current
        self.assertEqual((c["id"], c["status"], c["conclusion"]), (42766490058, "in_progress", None))
        self.assertEqual((c["lanes_total"], c["lanes_done"]), (17, 14))
        self.assertEqual(c["red"], ["FuzzCheckpointResumeEquivalence", "FuzzParseFormatRoundTrip",
                                    "FuzzSourceExecution", "FuzzRestoreVMSnapshotDoesNotPanic",
                                    "FuzzReadDatabase", "FuzzStructuredSourceExecution"])
        self.assertEqual(c["running"][0], {"name": "FuzzWorldCheckpointEffects", "minutes": 18})
        self.assertIsNone(c["accept"])
        self.assertIsNone(c["idle_minutes"])

    def test_real_guest_and_host_probe(self):
        # Slice installed, listeners not yet restarted into it (2026-11-10).
        outside = fuzz_ops.parse_probe(fixture("guest-probe-outside-slice.txt"))
        self.assertEqual(outside["listeners"][2], {
            "state": "active", "restarts": 0,
            "cgroup": "/system.slice/actions.runner.MongooseMoo.mongoose-barn-fuzz-2.service"})
        self.assertEqual(outside["slice"], {"memory_max": "15032385536", "memory_current": "0"})
        # One global kill (06:43) and four pairs of 3500M cgroup kills.
        self.assertEqual(len(outside["oom"]), 9)
        self.assertEqual(sum("task=format.test" in line for line in outside["oom"]), 4)
        self.assertEqual((outside["disk"], outside["hostdisk"]), (8, 9))
        self.assertEqual(outside["host"], {"state": "active", "restarts": 0})
        # After each listener restarted into the slice.
        inside = fuzz_ops.parse_probe(fixture("guest-probe.txt"))
        for slot in fuzz_ops.SLOTS:
            self.assertTrue(inside["listeners"][slot]["cgroup"].startswith("/barn-fuzz.slice/"))

    def test_probe_counts_only_kernel_kill_lines(self):
        # 08:30:17 systemd repeated "system.slice: A process of this unit has
        # been killed by the OOM killer" on daemon-reload; the kernel logged
        # one kill in 24h (06:43), and system.slice memory.events oom_kill 1.
        self.assertIn('-k --since -24h', fuzz_ops.GUEST_PROBE)
        self.assertIn('--grep "oom-kill:"', fuzz_ops.GUEST_PROBE)
        self.assertNotIn("OOM killer", fuzz_ops.GUEST_PROBE.split("journalctl")[1].split(";")[0])

    def test_real_controller_decisions(self):
        lines = fuzz_ops.decisions(fixture("controller-logs.txt"))
        self.assertEqual(lines, ["Submitted one dedicated successor", "continuous discovery stops here"])

    def test_real_state_reports_every_seen_problem(self):
        outside = fuzz_ops.parse_probe(fixture("guest-probe-outside-slice.txt"))
        state, problems = fuzz_ops.verdict(True, [self.current], self.runners, outside)
        self.assertEqual(state, "RUNNING: cycle 42766490058 is in_progress")
        self.assertIn("runner mongoose-barn-fuzz-2 is offline", problems)
        self.assertEqual(sum("runs outside barn-fuzz.slice" in p for p in problems), 4)
        self.assertTrue(any(p.startswith("9 kernel OOM kills in the last 24h") for p in problems))
        inside = fuzz_ops.parse_probe(fixture("guest-probe.txt"))
        online = [{**r, "status": "online"} for r in self.runners]
        _, problems = fuzz_ops.verdict(True, [self.current], online, inside)
        self.assertFalse(any("barn-fuzz.slice" in p for p in problems), problems)

    def test_failed_listener_and_unbounded_slice_are_problems(self):
        probe = fuzz_ops.parse_probe("listener 2 failed 0 /barn-fuzz.slice/x.service\nslice infinity 0\n")
        _, problems = fuzz_ops.verdict(True, [self.current], [], probe)
        self.assertIn("guest listener 2 is failed (python scripts/fuzz_ops.py runner restart 2)", problems)
        self.assertIn("barn-fuzz.slice has no memory bound: install .github/runner/barn-fuzz.slice", problems)

    def test_chain_states(self):
        done = dict(status="completed", conclusion="failure", accept="success")
        cases = [
            ((False, [self.current]), "STOPPED: FUZZ_CONTINUOUS_ENABLED"),
            ((True, []), "BROKEN: continuous mode is on but no continuous cycle"),
            ((True, [self.cycle(**done, idle_minutes=10)]), "BETWEEN CYCLES"),
            ((True, [self.cycle(**done, idle_minutes=95)]), "BROKEN: nothing queued or running 1h 35m"),
            ((True, [self.cycle(**{**done, "accept": "failure"}, idle_minutes=10)]), "WAITING FOR WATCHDOG"),
            ((True, [self.cycle(**{**done, "accept": "failure"}, idle_minutes=95)]),
             "BROKEN: nothing queued or running 1h 35m after cycle 42766490058 (acceptance failure)"),
            ((True, [self.cycle(**done, idle_minutes=95, title="Nightly fuzz / dedicated-once / native-only")]),
             "BROKEN: continuous mode is on but no continuous cycle"),
        ]
        for (enabled, cycles), expected in cases:
            with self.subTest(expected):
                state, _ = fuzz_ops.verdict(enabled, cycles, [])
                self.assertTrue(state.startswith(expected), state)

    def test_grace_matches_the_watchdog(self):
        source = (Path(__file__).resolve().parent / "fuzz_compute.cjs").read_text()
        self.assertIn(f"const WATCHDOG_GRACE_MS = {fuzz_ops.GRACE_MINUTES} * 60 * 1000;", source)

    def test_render_is_plain_lines(self):
        status = {"state": "RUNNING: x", "problems": [], "enabled": True, "max_parallel": "4",
                  "cycles": [self.current], "controllers": [], "runners": self.runners, "issues": [], "probe": None}
        text = fuzz_ops.render(status)
        self.assertTrue(text.startswith("RUNNING: x\nNo problems seen.\n"))
        self.assertNotRegex(text, r"[|─-╿]")


if __name__ == "__main__":
    unittest.main()
