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
        probe = fuzz_ops.parse_probe(fixture("guest-probe.txt"))
        self.assertEqual(probe["listeners"][2], {"state": "failed", "restarts": 0, "memory_max": "infinity"})
        self.assertEqual(len(probe["oom"]), 4)
        self.assertIn("global_oom", probe["oom"][1])
        self.assertEqual((probe["disk"], probe["hostdisk"]), (8, 8))
        self.assertEqual(probe["memory"], {"used_mb": 2829, "total_mb": 16004})
        self.assertEqual(probe["host"], {"state": "active", "restarts": 0})

    def test_real_controller_decisions(self):
        lines = fuzz_ops.decisions(fixture("controller-logs.txt"))
        self.assertEqual(lines, ["Submitted one dedicated successor", "continuous discovery stops here"])

    def test_real_state_reports_every_seen_problem(self):
        probe = fuzz_ops.parse_probe(fixture("guest-probe.txt"))
        state, problems = fuzz_ops.verdict(True, [self.current], self.runners, probe)
        self.assertEqual(state, "RUNNING: cycle 42766490058 is in_progress")
        self.assertIn("runner mongoose-barn-fuzz-2 is offline", problems)
        self.assertIn("guest listener 2 is failed (python scripts/fuzz_ops.py runner restart 2)", problems)
        self.assertEqual(sum("no memory bound" in p for p in problems), 4)
        self.assertTrue(any(p.startswith("4 OOM journal lines") for p in problems))

    def test_chain_states(self):
        done = dict(status="completed", conclusion="failure", accept="success")
        cases = [
            ((False, [self.current]), "STOPPED: FUZZ_CONTINUOUS_ENABLED"),
            ((True, []), "BROKEN: continuous mode is on but no continuous cycle"),
            ((True, [self.cycle(**done, idle_minutes=10)]), "BETWEEN CYCLES"),
            ((True, [self.cycle(**done, idle_minutes=95)]), "BROKEN: nothing queued or running 1h 35m"),
            ((True, [self.cycle(**{**done, "accept": "failure"}, idle_minutes=95)]), "STOPPED BY ACCEPTANCE"),
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
