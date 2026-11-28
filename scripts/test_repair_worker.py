from pathlib import Path
import subprocess
import types
import unittest
from unittest import mock

import repair_worker


class PolicyBlockTest(unittest.TestCase):
    """A Codex policy block hands the item to Claude unless Claude already
    failed on it (#1124: Codex's cyber-policy refusal left it with no repair)."""

    def process(self, labels, stage="repair"):
        calls = []

        def api(path, *args):
            calls.append((path, args))
            if path.endswith("/labels") and not args:
                return [{"name": name} for name in labels]
            if "/pulls?" in path or path.endswith("/comments"):
                return []
            return None

        result = {"outcome": "policy-block", "retry_at": "", "title": "", "evidence": ""}
        added = []
        with mock.patch.object(repair_worker, "api", api), \
                mock.patch.object(repair_worker, "label", lambda n, name, add: added.append((name, add))), \
                mock.patch.object(repair_worker, "proposal", lambda *a: (result, None, None)):
            done = repair_worker.process(types.SimpleNamespace(root=None, user=None, codex=None, retry_at=None),
                                         stage, {"number": 1124})
        dispatches = [args for path, args in calls if path.endswith("/dispatches")]
        return done, added, dispatches

    def test_policy_block_dispatches_claude(self):
        done, added, dispatches = self.process(["claude-fix", "codex-repair"])
        self.assertTrue(done)
        self.assertEqual(len(dispatches), 1)
        self.assertIn("inputs[provider]=claude", dispatches[0])
        self.assertIn(("codex-unavailable", True), added)
        self.assertIn(("codex-repair", False), added)
        self.assertNotIn(("repair-needs-attention", True), added)

    def test_policy_block_after_claude_failed_needs_attention(self):
        done, added, dispatches = self.process(["claude-fix", "codex-repair", "claude-unavailable"])
        self.assertTrue(done)
        self.assertEqual(dispatches, [])
        self.assertIn(("repair-needs-attention", True), added)

    def test_conformance_policy_block_dispatches_the_conformance_stage(self):
        _, _, dispatches = self.process(["codex-conformance"], stage="conformance")
        self.assertIn("inputs[stage]=conformance", dispatches[0])


class QueueRepairTest(unittest.TestCase):
    PR = {"number": 1216, "head": {"sha": "barn-head"}, "node_id": "PR1216"}

    def test_managed_proof_is_used_before_pending_checks_arm_the_exact_head(self):
        commands = []

        def run(*args, **kwargs):
            commands.append(args)
            if "--conformance-evidence" not in args:
                raise subprocess.CalledProcessError(1, args)

        with mock.patch.object(repair_worker, "clone"), \
                mock.patch.object(repair_worker, "git", side_effect=("", "barn-head", "barn-base")), \
                mock.patch.object(repair_worker, "run", run), \
                mock.patch.object(repair_worker, "api", return_value={"head": {"sha": "suite-head"}}), \
                mock.patch.object(repair_worker, "managed_proof", return_value=Path("evidence.json")) as proof, \
                mock.patch.object(repair_worker, "companion_pull", return_value={"head": {"sha": "suite-head"}}), \
                mock.patch.object(repair_worker, "required_check_state", return_value=([], True)), \
                mock.patch.object(repair_worker, "auto_merge") as arm:
            self.assertTrue(repair_worker.queue_repair(Path("."), self.PR, 294))
            self.assertEqual(proof.call_args.args[:3], (1216, "barn-head", "suite-head"))
            self.assertEqual(commands[1][2:], ("--base", "barn-base", "--head", "barn-head",
                                               "--suite-head", "suite-head", "--conformance-evidence", "evidence.json"))
            arm.assert_called_once_with(repair_worker.api, self.PR)

    def test_a_moved_head_is_not_proved_or_armed(self):
        with mock.patch.object(repair_worker, "clone"), \
                mock.patch.object(repair_worker, "git", side_effect=("", "different-head")), \
                mock.patch.object(repair_worker, "managed_proof") as proof, \
                mock.patch.object(repair_worker, "auto_merge") as arm:
            self.assertFalse(repair_worker.queue_repair(Path("."), self.PR, 294))
            proof.assert_not_called()
            arm.assert_not_called()


if __name__ == "__main__":
    unittest.main()
