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
            if "/pulls?" in path or "/comments?" in path:
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


if __name__ == "__main__":
    unittest.main()
