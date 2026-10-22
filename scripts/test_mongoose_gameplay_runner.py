"""Failure retention and ordinary actor contracts for gameplay runs."""
import json
import contextlib
import io
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

import mongoose_scenario_support as support
import run_mongoose_gameplay as runner


class GameplayRunnerTests(unittest.TestCase):
    def test_barn_cli_requires_sealed_executable_and_rejects_toast_collectors(self):
        base = ["--db", "fixture.db", "--db-sha256", "source", "--moo", "bootstrap", "--moo-sha256", "bootstrap-sha",
                "--sound-db", "sound", "--sound-db-sha256", "sound-sha", "--out", "owned-output"]
        with patch.object(runner.sys, "platform", "linux"):
            self.assertEqual(runner.arguments(base).engine, "toast")
            valid = base + ["--engine", "barn", "--barn", "barn", "--barn-sha256", "barn-sha"]
            self.assertEqual(runner.arguments(valid).engine, "barn")
            for invalid in (base + ["--engine", "barn"], valid + ["--profile", "definitions"],
                            valid + ["--native-profile", "auto"], valid + ["--native-profile", "required"]):
                with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit) as failed:
                    runner.arguments(invalid)
                self.assertEqual(failed.exception.code, 2)

    def test_go_cpu_cli_is_barn_only_and_integer_bounded(self):
        base = ["--db", "fixture.db", "--db-sha256", "source", "--moo", "bootstrap", "--moo-sha256", "bootstrap-sha",
                "--sound-db", "sound", "--sound-db-sha256", "sound-sha", "--out", "owned-output"]
        barn = base + ["--engine", "barn", "--barn", "barn", "--barn-sha256", "barn-sha"]
        with patch.object(runner.sys, "platform", "linux"):
            self.assertEqual(runner.arguments(base).go_cpu_seconds, 0)
            for seconds in (0, 1, 30):
                self.assertEqual(runner.arguments(barn + ["--go-cpu-seconds", str(seconds)]).go_cpu_seconds, seconds)
            for invalid in (base + ["--go-cpu-seconds", "1"], barn + ["--go-cpu-seconds", "-1"],
                            barn + ["--go-cpu-seconds", "31"], barn + ["--go-cpu-seconds", "1.5"],
                            barn + ["--go-cpu-seconds", "1", "--profile", "definitions"]):
                with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                    runner.arguments(invalid)

    def test_captured_transport_cannot_make_requested_unvalidated_profile_complete(self):
        manifest = {"engine": "barn", "status": "complete", "source_database_unchanged": True,
                    "executable_unchanged": True, "controller_sources_unchanged": True,
                    "bootstrap_executable_unchanged": True,
                    "immutable_inputs": {name: {"unchanged": True} for name in
                        ("barn_config", "barn_registry", "barn_runtime_profile")},
                    "sound_catalog": {"source_unchanged": True}, "shutdown": {"exit_code": 0,
                        "final_checkpoint": {"status": "verified", "verified": True}},
                    "go_cpu_seconds": 1, "go_diagnostics": {"status": "captured",
                        "pprof_semantic_validation": "pending offline validation"}}
        runner.finalize_verdict(manifest)
        self.assertEqual(manifest["status"], "diagnostic-unvalidated")
        self.assertTrue(manifest["final_checks"]["go_diagnostics_transport"])
        self.assertFalse(manifest["final_checks"]["go_profiles_validated"])
        manifest.update(status="complete", go_diagnostics={"status": "failed"})
        runner.finalize_verdict(manifest)
        self.assertEqual(manifest["status"], "invalidated")

    def test_barn_manifest_requires_immutable_bootstrap_configuration_and_profile(self):
        good = {"engine": "barn", "status": "complete", "source_database_unchanged": True,
                "executable_unchanged": True, "controller_sources_unchanged": True,
                "bootstrap_executable_unchanged": True, "sound_catalog": {"source_unchanged": True},
                "shutdown": {"exit_code": 0, "forced": False,
                             "final_checkpoint": {"status": "verified", "verified": True}},
                "immutable_inputs": {name: {"unchanged": True} for name in
                    ("barn_config", "barn_registry", "barn_runtime_profile")}}
        runner.finalize_verdict(good)
        self.assertEqual(good["status"], "complete")
        for name in ("barn_config", "barn_registry", "barn_runtime_profile"):
            bad = json.loads(json.dumps(good))
            bad["immutable_inputs"][name]["unchanged"] = False
            runner.finalize_verdict(bad)
            self.assertEqual(bad["status"], "invalidated")
        good.update(status="complete", bootstrap_executable_unchanged=False)
        runner.finalize_verdict(good)
        self.assertEqual(good["status"], "invalidated")
        good.update(status="complete", bootstrap_executable_unchanged=True)
        good["shutdown"]["forced"] = True
        runner.finalize_verdict(good)
        self.assertEqual(good["status"], "invalidated")

    def test_failed_final_checkpoint_invalidates_outer_success_but_preserves_domain_result(self):
        for checkpoint in (None, {"status": "unknown", "verified": False},
                           {"status": "failed", "verified": False}, {"status": "failed", "verified": True}):
            with self.subTest(checkpoint=checkpoint):
                manifest = {"engine": "barn", "status": "complete", "source_database_unchanged": True,
                            "executable_unchanged": True, "controller_sources_unchanged": True,
                            "bootstrap_executable_unchanged": True, "sound_catalog": {"source_unchanged": True},
                            "shutdown": {"exit_code": 0, "forced": False, "final_checkpoint": checkpoint},
                            "immutable_inputs": {name: {"unchanged": True} for name in
                                ("barn_config", "barn_registry", "barn_runtime_profile")},
                            "scenarios": {"flight": {"status": "complete", "completed_iterations": 1}}}
                runner.finalize_verdict(manifest)
                self.assertEqual(manifest["status"], "invalidated")
                self.assertFalse(manifest["final_checks"]["owned_final_checkpoint"])
                self.assertEqual(manifest["scenarios"]["flight"]["completed_iterations"], 1)
                manifest.update(status="failed", failure="primary native error")
                runner.finalize_verdict(manifest)
                self.assertEqual(manifest["status"], "failed")
                self.assertEqual(manifest["failure"], "primary native error")

    def test_provenance_missing_or_changed_inputs_still_preserve_primary_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "input"
            path.write_text("sealed")
            digest = runner.offline.bench.sha256_file(path)
            manifest = {"status": "failed", "failure": "primary native error",
                        "immutable_inputs": {"owned": {"path": str(path), "sha256": digest}}}
            path.write_text("changed")
            runner.finish_input_provenance(manifest)
            self.assertFalse(manifest["immutable_inputs"]["owned"]["unchanged"])
            path.unlink()
            runner.finish_input_provenance(manifest)
            self.assertIn("verification_failure", manifest["immutable_inputs"]["owned"])
            self.assertEqual(manifest["failure"], "primary native error")

    def test_barn_cpu_measurement_cannot_alias_main_thread_or_accept_reused_pid(self):
        key, value = runner.cpu_delta({"process_total_cpu_ns": 100, "start_ticks": 2},
                                     {"process_total_cpu_ns": 500, "start_ticks": 2}, "barn")
        self.assertEqual((key, value), ("process_total_cpu_ns", 400))
        with self.assertRaises(RuntimeError):
            runner.cpu_delta({"process_total_cpu_ns": 100, "start_ticks": 2},
                             {"process_total_cpu_ns": 500, "start_ticks": 3}, "barn")
        with self.assertRaises(RuntimeError):
            runner.cpu_delta({"process_total_cpu_ns": 100, "start_ticks": 2},
                             {"process_total_cpu_ns": 50, "start_ticks": 2}, "barn")
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

    def test_metadata_rpc_never_records_native_payload_or_unknown_acknowledgement(self):
        for response in (([1, 200, 0], ["SECRET response"]), (["SECRET value"], ["SECRET output"]),
                         RuntimeError("SECRET native message/local variable")):
            with self.subTest(response_type=type(response).__name__), tempfile.TemporaryDirectory() as directory:
                original = Mock()
                if isinstance(response, Exception):
                    original.eval.side_effect = response
                else:
                    original.eval.return_value = response
                path = Path(directory) / "transcript.jsonl"
                record = runner.RecordingControl(original, path)
                try:
                    if isinstance(response, Exception) or len(response[0]) != 3:
                        with self.assertRaises(Exception):
                            record.eval_metadata("return {1,200,0};")
                    else:
                        self.assertEqual(record.eval_metadata("return {1,200,0};"), ([1,200,0], []))
                finally:
                    record.close()
                self.assertNotIn("SECRET", path.read_text())
                self.assertNotIn("native_output", path.read_text())

    def scenario(self, directory, action, *, cleanup_error=None, native_status="disabled", mode="off", telemetry_error=None,
                 profile="off", profile_ack=None, engine="toast", events=None, metadata_error=None,
                 go_seconds=0, go_collector=None):
        options = SimpleNamespace(run_dir=Path(directory), iterations=1, profile=profile, hz=100, native_mode=mode, engine=engine,
                                  go_cpu_seconds=go_seconds, go_collector=go_collector)
        module = Mock()
        module.setup.return_value = {"actor": "#100"}
        module.snapshot.return_value = {"position": [0, 0, 0]}
        if isinstance(action, Exception):
            module.run.side_effect = action
        else:
            module.run.return_value = action
        if go_seconds and events is not None:
            def productive(*args):
                events.append("actions")
                if isinstance(action, Exception):
                    raise action
                return action
            module.run.side_effect = productive
        module.cleanup.return_value = {"done": True}
        if cleanup_error:
            module.cleanup.side_effect = cleanup_error
        elif events is not None:
            module.cleanup.side_effect = lambda *args: events.append("cleanup") or {"done": True}
        def metadata(*args, **kwargs):
            if events is not None:
                events.append(args[4])
            if metadata_error:
                raise metadata_error
            return {"status": "collected", "phase": args[4]}
        native = Mock()
        native.finish.return_value = {"status": native_status}
        native_module = Mock()
        native_module.begin.return_value = native
        control, observer, server = Mock(), Mock(), Mock()
        control.eval.return_value = (profile_ack, [])
        server.proc.pid = 1
        def modules(name):
            return native_module if name == "mongoose_native_profile" else module
        measurements = iter([
            {"process_total_cpu_ns" if engine == "barn" else "main_cpu_ns": 10, "start_ticks": 1},
            telemetry_error or {"process_total_cpu_ns" if engine == "barn" else "main_cpu_ns": 20, "start_ticks": 1}])
        def measured(*args):
            row = next(measurements)
            if go_seconds and events is not None:
                events.append("measure-before" if row.get("process_total_cpu_ns") == 10 else "measure-after")
            if isinstance(row, Exception):
                raise row
            return row
        with patch.object(runner.importlib, "import_module", side_effect=modules), \
                patch.object(runner.error_capture, "capture", side_effect=metadata), \
                patch.object(runner, "save_profile", return_value={"exported": True}), \
                patch.object(runner.barn_process if engine == "barn" else runner.offline, "process_state", side_effect=measured):
            try:
                result = runner.run_scenario("flight", control, server, observer, options)
                return result, module
            except support.ScenarioFailure:
                return json.loads((Path(directory) / "flight" / "result.json").read_text()), module

    def diagnostic_collector(self, events, *, cpu_error=None, request_issued=True):
        collector, handle = Mock(), Mock()
        collector.snapshot.side_effect = lambda label: events.append(label) or {"transport": "captured"}
        collector.start_cpu.side_effect = lambda *args, **kwargs: events.append("start-cpu") or handle
        handle.request_issued.wait.side_effect = lambda **kwargs: events.append("request-issued") or request_issued
        def finished(*args):
            events.append("finish-cpu")
            if cpu_error:
                raise cpu_error
            return {"status": "captured", "request_issued_monotonic_ns": 10,
                    "finished_monotonic_ns": 30, "pprof_semantic_validation": "pending offline validation"}
        collector.finish_cpu.side_effect = finished
        return collector, handle

    def test_go_capture_order_is_outside_action_measurement_and_before_cleanup(self):
        with tempfile.TemporaryDirectory() as directory:
            events = []
            collector, handle = self.diagnostic_collector(events)
            result, _ = self.scenario(directory, {"completed_iterations": 1}, engine="barn",
                                      events=events, go_seconds=1, go_collector=collector)
            self.assertEqual(events, ["baseline", "flight-before", "start-cpu", "request-issued", "measure-before",
                                      "actions", "measure-after", "finish-cpu", "flight-after", "cleanup"])
            self.assertEqual(result["domain_status"], "complete")
            self.assertEqual(result["status"], "diagnostic-unvalidated")
            self.assertEqual(result["process_total_cpu_ns"], 10)
            self.assertFalse(result["go_diagnostics"]["request_issued"]["activation_acknowledged"])
            handle.request_issued.wait.assert_called_once_with(timeout=10)

    def test_go_cpu_failure_follows_error_metadata_and_preserves_primary(self):
        with tempfile.TemporaryDirectory() as directory:
            events = []
            collector, _ = self.diagnostic_collector(events, cpu_error=RuntimeError("CPU transport failed"))
            result, _ = self.scenario(directory, support.ScenarioFailure("primary ordinary failure"), engine="barn",
                                      events=events, go_seconds=1, go_collector=collector)
            self.assertEqual(result["failure"], "primary ordinary failure")
            self.assertEqual(events[-4:], ["failure_before_cleanup", "finish-cpu", "flight-after", "cleanup"])
            self.assertIn("cpu_failure", result["go_diagnostics"])

    def test_cpu_request_not_issued_prevents_actions_but_drains_and_cleans(self):
        with tempfile.TemporaryDirectory() as directory:
            events = []
            collector, _ = self.diagnostic_collector(events, request_issued=False)
            result, module = self.scenario(directory, {"completed_iterations": 1}, engine="barn",
                                           events=events, go_seconds=1, go_collector=collector)
            module.run.assert_not_called()
            self.assertNotIn("action_wall_ns", result)
            self.assertEqual(events[-4:], ["failure_before_cleanup", "finish-cpu", "flight-after", "cleanup"])
            self.assertEqual(result["status"], "failed")

    def test_main_finishes_all_diagnostic_handles_before_owned_shutdown_and_never_false_greens(self):
        for primary in (None, support.ScenarioFailure("primary ordinary failure")):
            with self.subTest(primary=primary is not None), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                for name in ("input.db", "toast", "barn", "sound.sqlite"):
                    (root / name).write_bytes(b"not executable; lifecycle must remain mocked")
                digest = runner.offline.bench.sha256_file(root / "input.db")
                options = SimpleNamespace(db=root / "input.db", db_sha256=digest, moo=root / "toast", moo_sha256=digest,
                    barn=root / "barn", barn_sha256=digest, sound_db=root / "sound.sqlite", sound_db_sha256=digest,
                    out=root / "owned-run", engine="barn", source_commit=None, scenario="flight", seed=1, iterations=1,
                    profile="off", native_mode="off", port=17778, warmup_seconds=0, go_cpu_seconds=1)
                events = []
                server = Mock()
                server.run_dir, server.proc.pid, server.start_ticks = options.out, 234, 555
                server.profile_path = options.out / "barn-profile.json"
                server.identity = {"executable_sha256": digest,
                    "config_sha256": runner.offline.bench.sha256_file(runner.barn_process.CONFIG),
                    "profile_registry_sha256": runner.offline.bench.sha256_file(runner.barn_process.REGISTRY),
                    "profile_manifest_sha256": digest}
                def started():
                    server.profile_path.write_bytes(b"not executable; lifecycle must remain mocked")
                    return 0
                server.start.side_effect = started
                server.stop.side_effect = lambda: events.append("shutdown") or {"exit_code": 0, "forced": False,
                    "final_checkpoint": {"status": "verified", "verified": True}}
                observer = Mock(error=None)
                observer.finish.return_value = {"samples": 0}
                control = Mock()
                control.eval.return_value = ([0, 1, 0, "mock Barn"], [])
                collector = Mock()
                collector.finish.side_effect = lambda: events.append("finish-all") or {
                    "status": "captured", "pprof_semantic_validation": "pending offline validation"}
                go_module = Mock()
                go_module.Collector.return_value = collector
                def scenario(*args):
                    events.append("scenario")
                    self.assertIs(args[-1].go_collector, collector)
                    if primary:
                        raise primary
                    return {"scenario": "flight", "status": "diagnostic-unvalidated", "domain_status": "complete"}
                with patch.object(runner, "arguments", return_value=options), \
                     patch.object(runner.offline, "check_network_isolation", return_value={"mocked": True}), \
                     patch.object(runner.offline, "prepare_sound_catalog", return_value={"source_sha256": digest}), \
                     patch.object(runner.offline, "adapt_bootstrap"), \
                     patch.object(runner.offline.bench, "bootstrap_control_account", return_value=(0, "unused", "unused")), \
                     patch.object(runner.barn_process, "OwnedBarnServer", return_value=server), \
                     patch.object(runner.offline.bench, "connect_control", return_value=(control, None)), \
                     patch.object(runner.offline, "snapshot", return_value={}), \
                     patch.object(runner, "ProcessObserver", return_value=observer), \
                     patch.object(runner.importlib, "import_module", return_value=go_module), \
                     patch.object(runner, "run_scenario", side_effect=scenario):
                    with self.assertRaises(Exception) as caught:
                        runner.main()
                self.assertEqual(events, ["scenario", "finish-all", "shutdown"])
                manifest = json.loads((options.out / "manifest-final.json").read_text())
                self.assertFalse(manifest["final_checks"]["go_profiles_validated"])
                self.assertTrue(manifest["final_checks"]["go_diagnostics_transport"])
                if primary:
                    self.assertIs(caught.exception, primary)
                    self.assertEqual(manifest["failure"], "primary ordinary failure")
                else:
                    self.assertEqual(manifest["status"], "diagnostic-unvalidated")
                    self.assertEqual(manifest["scenarios"][0]["domain_status"], "complete")
                self.assertIn("mongoose_barn_diagnostics.py", manifest["controller_sources"])
                self.assertTrue(manifest["controller_sources_unchanged"])

    def test_error_baseline_and_failure_capture_precede_cleanup_and_preserve_primary(self):
        for capture_error in (None, RuntimeError("SECRET diagnostic failure")):
            with self.subTest(capture_error=capture_error is not None), tempfile.TemporaryDirectory() as directory:
                events = []
                result, _ = self.scenario(directory, support.ScenarioFailure("primary ordinary refusal failed"),
                                          events=events, metadata_error=capture_error)
                self.assertEqual(events, ["baseline", "failure_before_cleanup", "cleanup"])
                self.assertEqual(result["failure"], "primary ordinary refusal failed")
                self.assertIn("error_metadata_failure", result)
                self.assertNotIn("SECRET", json.dumps(result))

    def test_unfinished_and_boolean_success_are_not_productive_work(self):
        for action in ({"completed_iterations": 0}, {"completed_iterations": True}):
            with self.subTest(action=action), tempfile.TemporaryDirectory() as directory:
                result, module = self.scenario(directory, action)
                self.assertEqual(result["status"], "failed")
                module.cleanup.assert_called_once()

    def test_barn_uses_same_domain_oracle_and_separately_named_cpu(self):
        with tempfile.TemporaryDirectory() as directory:
            result, module = self.scenario(directory, {"completed_iterations": 1}, engine="barn")
            self.assertEqual(result["status"], "complete")
            self.assertEqual(result["process_total_cpu_ns"], 10)
            self.assertNotIn("main_cpu_ns", result)
            module.run.assert_called_once()
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
