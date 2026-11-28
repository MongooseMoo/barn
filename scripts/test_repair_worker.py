from pathlib import Path
import json
import subprocess
import tempfile
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


class ManagedProofTest(unittest.TestCase):
    DISPATCH = f"repos/{repair_worker.BARN}/actions/workflows/repair-proof.yml/dispatches"
    RUN = f"repos/{repair_worker.BARN}/actions/runs/42"

    def test_polling_uses_a_fresh_token_without_dispatching_again(self):
        requests = []
        publisher = mock.Mock()
        publisher.get.side_effect = ("first", "first", "renewed", "renewed")
        with tempfile.TemporaryDirectory() as directory, \
                mock.patch.object(repair_worker, "PUBLISHER_TOKEN", publisher), \
                mock.patch.object(repair_worker.time, "sleep"), \
                mock.patch.object(repair_worker.subprocess, "run") as subprocess_run:
            destination = Path(directory)
            evidence = destination / "conformance-evidence.json"
            replies = iter(({"workflow_run_id": 42}, {"status": "queued"}, {"status": "completed", "conclusion": "success"}))

            def reply(command, **kwargs):
                requests.append((command[1:3], kwargs["env"]["GH_TOKEN"], kwargs["timeout"]))
                if command[1:3] == ("run", "download"):
                    self.assertEqual(command[3], "42")
                    evidence.write_text("{}", encoding="utf-8")
                    return types.SimpleNamespace(stdout="")
                self.assertEqual(command[1], "api")
                return types.SimpleNamespace(stdout=json.dumps(next(replies)))

            subprocess_run.side_effect = reply
            self.assertEqual(repair_worker.managed_proof(1216, "barn-head", "suite-head", destination), evidence)
        self.assertEqual(requests, [
            (("api", self.DISPATCH), "first", 120),
            (("api", self.RUN), "first", 120),
            (("api", self.RUN), "renewed", 120),
            (("run", "download"), "renewed", 120),
        ])

    def test_unsuccessful_terminal_run_preserves_the_reason_without_downloading(self):
        with mock.patch.object(repair_worker, "api", side_effect=(
                {"workflow_run_id": 42}, {"status": "completed", "conclusion": "cancelled"},
        )) as api, mock.patch.object(repair_worker, "run") as run:
            with self.assertRaisesRegex(ValueError, "run 42 completed with cancelled"):
                repair_worker.managed_proof(1216, "barn-head", "suite-head", Path("."))
        self.assertEqual([call.args[0] for call in api.call_args_list], [self.DISPATCH, self.RUN])
        run.assert_not_called()

    def test_poll_deadline_preserves_the_pending_state_without_dispatching_again(self):
        with mock.patch.object(repair_worker, "api", side_effect=(
                {"workflow_run_id": 42}, {"status": "queued"},
        )) as api, mock.patch.object(repair_worker.time, "monotonic", side_effect=(0, 7200)), \
                mock.patch.object(repair_worker, "run") as run:
            with self.assertRaisesRegex(ValueError, "run 42 was still queued after two hours"):
                repair_worker.managed_proof(1216, "barn-head", "suite-head", Path("."))
        self.assertEqual([call.args[0] for call in api.call_args_list], [self.DISPATCH, self.RUN])
        run.assert_not_called()


class QueueRepairTest(unittest.TestCase):
    PR = {"number": 1216, "head": {"sha": "barn-head"}, "node_id": "PR1216"}

    def test_managed_proof_is_used_before_required_checks_enable_exact_enqueue(self):
        commands = []

        def run(*args, **kwargs):
            commands.append(args)
            if "--conformance-evidence" not in args:
                raise subprocess.CalledProcessError(1, args)

        with mock.patch.object(repair_worker, "clone"), \
                mock.patch.object(repair_worker, "git", side_effect=("", "barn-head", "barn-base")), \
                mock.patch.object(repair_worker, "run", run), \
                mock.patch.object(repair_worker, "api", side_effect=(
                    {"head": {"sha": "suite-head"}},
                    {"data": {"enqueuePullRequest": {"mergeQueueEntry": {"id": "queue-entry"}}}},
                )) as api, \
                mock.patch.object(repair_worker, "managed_proof", return_value=Path("evidence.json")) as proof, \
                mock.patch.object(repair_worker, "wait_for_repair_checks", return_value="ready") as wait:
            self.assertTrue(repair_worker.queue_repair(Path("."), self.PR, 294))
            self.assertEqual(proof.call_args.args[:3], (1216, "barn-head", "suite-head"))
            self.assertEqual(commands[1][2:], ("--base", "barn-base", "--head", "barn-head",
                                               "--suite-head", "suite-head", "--conformance-evidence", "evidence.json"))
            wait.assert_called_once_with(api, 1216, "barn-head", "suite-head")
            enqueue = api.call_args.args
            self.assertIn("head=barn-head", enqueue)
            self.assertIn("enqueuePullRequest", enqueue[2])

    def test_a_moved_head_is_not_proved_or_enqueued(self):
        with mock.patch.object(repair_worker, "clone"), \
                mock.patch.object(repair_worker, "git", side_effect=("", "different-head")), \
                mock.patch.object(repair_worker, "managed_proof") as proof, \
                mock.patch.object(repair_worker, "api") as api:
            self.assertFalse(repair_worker.queue_repair(Path("."), self.PR, 294))
            proof.assert_not_called()
            api.assert_not_called()


if __name__ == "__main__":
    unittest.main()
