"""Failure retention and ordinary actor contracts for gameplay runs."""
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

import mongoose_scenario_support as support
import run_mongoose_gameplay as runner


class GameplayRunnerTests(unittest.TestCase):
    def test_native_failure_before_background_noise_remains_in_durable_transcript(self):
        with tempfile.TemporaryDirectory() as directory:
            output = ["E_PROPNF in own request"] + ["unrelated background error"] * 12
            control = Mock()
            control.eval.side_effect = support.bench.ControlEvalError("missing result", output)
            recording = runner.RecordingControl(control, Path(directory) / "transcript.jsonl")
            try:
                with self.assertRaises(RuntimeError):
                    recording.eval("return 1;")
            finally:
                recording.close()
            self.assertEqual(json.loads((Path(directory) / "transcript.jsonl").read_text())["native_output"], output)

    def test_changed_or_unknown_input_cannot_leave_a_successful_manifest(self):
        manifest = {"status": "complete", "source_database_unchanged": True,
                    "executable_unchanged": True, "controller_sources_unchanged": True,
                    "sound_catalog": {"source_unchanged": True}, "shutdown": {"exit_code": 0}}
        runner.finalize_verdict(manifest)
        self.assertEqual(manifest["status"], "complete")
        for invalid in (False, None):
            with self.subTest(invalid=invalid):
                manifest.update(status="complete", executable_unchanged=invalid)
                runner.finalize_verdict(manifest)
                self.assertEqual(manifest["status"], "invalidated")

    def test_native_parsed_object_reference_has_exactly_one_prefix(self):
        self.assertEqual(support.object_ref(support.bench.MooObj(277801)), "#277801")

    def test_wizard_cannot_be_counted_as_an_ordinary_actor(self):
        control = Mock()
        control.eval.return_value = ([1, 1], [])
        with self.assertRaisesRegex(support.ScenarioFailure, "ordinary non-wizard"):
            support.actor_eval(control, 2, 'return 1;')
        self.assertEqual(control.eval.call_count, 1)

    def test_stalled_domain_is_failure_with_last_observed_state(self):
        clock = iter((0.0, 0.0, 0.2, 0.3))
        with patch.object(support.time, "monotonic", side_effect=lambda: next(clock)), \
                patch.object(support.time, "sleep"):
            with self.assertRaises(support.ScenarioFailure) as raised:
                support.wait_until(lambda: {"lit": False}, lambda state: state["lit"], 0.1)
        self.assertEqual(raised.exception.evidence["last_state"], {"lit": False})

    def test_command_timeout_retains_actual_attempt(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "commands.jsonl"
            path.touch()
            connection = SimpleNamespace(gameplay_transcript=path)
            with patch.object(support.bench, "bracketed", return_value=None):
                with self.assertRaises(support.ScenarioFailure):
                    support.actor_command(connection, "weave lumos", 0.1)
            attempt = json.loads(path.read_text())
            self.assertEqual(attempt["command"], "weave lumos")
            self.assertEqual(attempt["status"], "failed")

    def test_failed_control_request_is_durable_before_exception_propagates(self):
        with tempfile.TemporaryDirectory() as directory:
            original = Mock()
            original.eval.side_effect = RuntimeError("native E_PERM")
            record = runner.RecordingControl(original, Path(directory) / "transcript.jsonl")
            try:
                with self.assertRaisesRegex(RuntimeError, "native E_PERM"):
                    record.eval("return 1;")
            finally:
                record.close()
            self.assertEqual(json.loads((Path(directory) / "transcript.jsonl").read_text())["status"], "failed")

    def scenario(self, directory, action, *, cleanup_error=None, native_status="disabled", mode="off", telemetry_error=None,
                 profile="off", profile_ack=None):
        options = SimpleNamespace(run_dir=Path(directory), iterations=1, profile=profile, hz=100, native_mode=mode)
        module = Mock()
        module.setup.return_value = {"actor": "#100"}
        module.snapshot.return_value = {"position": [0, 0, 0]}
        if isinstance(action, Exception):
            module.run.side_effect = action
        else:
            module.run.return_value = action
        module.cleanup.return_value = {"done": True}
        if cleanup_error:
            module.cleanup.side_effect = cleanup_error
        native = Mock()
        native.finish.return_value = {"status": native_status}
        native_module = Mock()
        native_module.begin.return_value = native
        control, observer, server = Mock(), Mock(), Mock()
        control.eval.return_value = (profile_ack, [])
        server.proc.pid = 1
        def modules(name):
            return native_module if name == "mongoose_native_profile" else module
        with patch.object(runner.importlib, "import_module", side_effect=modules), \
                patch.object(runner, "save_profile", return_value={"exported": True}), \
                patch.object(runner.offline, "process_state", side_effect=[
                    {"main_cpu_ns": 10, "start_ticks": 1}, telemetry_error or {"main_cpu_ns": 20, "start_ticks": 1}]):
            try:
                result = runner.run_scenario("flight", control, server, observer, options)
                return result, module
            except support.ScenarioFailure:
                return json.loads((Path(directory) / "flight" / "result.json").read_text()), module

    def test_unfinished_and_boolean_success_are_not_productive_work(self):
        for action in ({"completed_iterations": 0}, {"completed_iterations": True}):
            with self.subTest(action=action), tempfile.TemporaryDirectory() as directory:
                result, module = self.scenario(directory, action)
                self.assertEqual(result["status"], "failed")
                module.cleanup.assert_called_once()

    def test_wrong_sampler_mode_fails_before_actions_and_still_exports(self):
        with tempfile.TemporaryDirectory() as directory:
            result, module = self.scenario(directory, {"completed_iterations": 1},
                                            profile="definitions", profile_ack=[1, "detailed"])
            self.assertEqual(result["status"], "failed")
            self.assertEqual(result["moo_profile"], {"exported": True})
            module.run.assert_not_called()
            module.cleanup.assert_called_once()

    def test_compact_sampler_ack_allows_actions_and_exports(self):
        with tempfile.TemporaryDirectory() as directory:
            result, module = self.scenario(directory, {"completed_iterations": 1},
                                            profile="definitions", profile_ack=[1, "definitions"])
            self.assertEqual(result["status"], "complete")
            self.assertEqual(result["moo_profile_start"], [1, "definitions"])
            self.assertEqual(result["moo_profile"], {"exported": True})
            module.run.assert_called_once()

    def test_native_and_cleanup_failures_do_not_erase_domain_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            result, _ = self.scenario(directory, support.ScenarioFailure("no movement", {"position": [0, 0, 0]}),
                                      cleanup_error=RuntimeError("cleanup unavailable"), native_status="failed", mode="required")
            self.assertEqual(result["failure"], "no movement")
            self.assertEqual(result["failure_evidence"], {"position": [0, 0, 0]})
            self.assertEqual(result["cleanup_failure"], "cleanup unavailable")
            self.assertIn("native_profile_failure", result)

    def test_missing_required_profile_invalidates_successful_domain_run(self):
        with tempfile.TemporaryDirectory() as directory:
            result, _ = self.scenario(directory, {"completed_iterations": 1}, native_status="failed", mode="required")
            self.assertEqual(result["status"], "failed")

    def test_process_exit_cannot_replace_a_failed_domain_outcome(self):
        with tempfile.TemporaryDirectory() as directory:
            result, _ = self.scenario(directory, support.ScenarioFailure("no movement", {"position": [0, 0, 0]}),
                                      telemetry_error=RuntimeError("process vanished"))
            self.assertEqual(result["failure"], "no movement")
            self.assertEqual(result["failure_evidence"], {"position": [0, 0, 0]})
            self.assertEqual(result["measurement_failure"], "process vanished")


if __name__ == "__main__":
    unittest.main()
