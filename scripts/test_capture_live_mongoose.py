"""Checks for attribution and evidence boundaries in the live collector."""
import contextlib
import io
import json
from pathlib import Path
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import capture_live_mongoose as collector
from capture_live_mongoose import cpu_delta, decode_moo_state, summarize


class LiveCaptureTests(unittest.TestCase):
    def test_pid_reuse_is_not_a_cpu_interval(self):
        before = {"pid": 7, "start_ticks": 100, "cpu_ticks": 1000, "monotonic": 1.0}
        after = {"pid": 7, "start_ticks": 200, "cpu_ticks": 2000, "monotonic": 2.0}
        self.assertIsNone(cpu_delta(before, after, 100))

    def test_cpu_is_one_core_percent_not_host_percent(self):
        before = {"pid": 7, "start_ticks": 100, "cpu_ticks": 1000, "monotonic": 1.0}
        after = {"pid": 7, "start_ticks": 100, "cpu_ticks": 1200, "monotonic": 3.0}
        self.assertEqual(cpu_delta(before, after, 100), 100.0)

    def test_missing_moo_response_cannot_claim_task_attribution(self):
        result = summarize({"clock_ticks": 100, "samples": [], "moo_state": {"ok": False}})
        self.assertEqual(result["running_moo_verb"], "unknown")
        self.assertIsNone(result["mean_main_cpu_percent"])

    def test_decode_rejects_compiler_or_missing_state(self):
        for payload in ({"ok": 0, "value": "compiler failure"}, {"ok": 1, "value": '"{}"'}):
            with self.subTest(payload=payload), self.assertRaises(ValueError):
                decode_moo_state(payload)

    def test_decode_preserves_queue_coverage_and_does_not_call_it_running(self):
        import json
        state = {"queued_count": 17, "queued_shown": 3, "groups": {}, "current_cycle": 123}
        result = decode_moo_state({"ok": 1, "value": json.dumps(json.dumps(state))})
        self.assertEqual(result["queued_count"], 17)
        self.assertEqual(result["queued_shown"], 3)
        self.assertNotIn("running_moo_verb", result)


class LiveCaptureVerdictTests(unittest.TestCase):
    def stable(self):
        service = {"MainPID": "7", "NRestarts": "0", "ActiveState": "active", "SubState": "running"}
        return {"protocol_version": 1, "pid": 7, "clock_ticks": 100,
                "service": service, "service_after": dict(service),
                "samples": [{"pid": 7, "start_ticks": 100, "rss_bytes": 4096,
                             "threads": [{"pid": 7, "start_ticks": 100, "cpu_ticks": index * 20,
                                          "monotonic": index + 1.0}]} for index in range(3)],
                "moo_state": {"requested": False, "ok": False}}

    def cli(self, capture, moo=False):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "capture.json"
            argv = ["capture", "--ssh-exe", "mocked-ssh", "--samples", "2", "--out", str(output)]
            if moo:
                argv.append("--moo-state")
            stdout = json.dumps(capture).encode()
            with patch.object(sys, "argv", argv), contextlib.redirect_stdout(io.StringIO()), \
                    patch.object(collector.subprocess, "run", return_value=SimpleNamespace(
                        returncode=0, stdout=stdout, stderr=b"")) as remote:
                code = collector.main()
            remote.assert_called_once()
            record = json.loads(output.read_text())
            self.assertEqual(record["raw_stdout"], stdout.decode())
            self.assertEqual(record["capture"], capture)
            return code, record

    def test_vanished_process_actual_cli_repro_never_returns_complete(self):
        capture = self.stable()
        capture.update(samples=[], sampling_error="main process disappeared")
        capture["service_after"].update(MainPID="8", NRestarts="1")
        code, record = self.cli(capture)
        self.assertEqual(code, 1)
        self.assertEqual(record["state"], "process_changed")
        self.assertEqual(record["capture"]["sampling_error"], "main process disappeared")

    def test_complete_stable_process_has_every_interval_without_optional_moo(self):
        code, record = self.cli(self.stable())
        self.assertEqual(code, 0)
        self.assertEqual(record["state"], "capture_complete")
        self.assertEqual(record["summary"]["main_cpu_intervals"], 2)
        self.assertTrue(all(record["capture_checks"].values()))
        self.assertEqual(record["summary"]["running_moo_verb"], "unknown")

    def test_pid_restart_or_sample_incarnation_changes_are_nonzero(self):
        for variant in ("pid", "restart", "sample_start"):
            with self.subTest(variant=variant):
                capture = self.stable()
                if variant == "pid":
                    capture["service_after"]["MainPID"] = "8"
                elif variant == "restart":
                    capture["service_after"]["NRestarts"] = "1"
                else:
                    capture["samples"][-1]["start_ticks"] = 200
                code, record = self.cli(capture)
                self.assertEqual((code, record["state"]), (1, "process_changed"))

    def test_unknown_service_identity_or_nonrunning_service_is_partial(self):
        for variant in ("missing_after", "missing_restart", "zero_pid", "stopped"):
            with self.subTest(variant=variant):
                capture = self.stable()
                if variant == "missing_after":
                    del capture["service_after"]
                elif variant == "missing_restart":
                    del capture["service_after"]["NRestarts"]
                elif variant == "zero_pid":
                    capture["service"]["MainPID"] = capture["service_after"]["MainPID"] = "0"
                else:
                    capture["service_after"]["ActiveState"] = "inactive"
                code, record = self.cli(capture)
                self.assertEqual((code, record["state"]), (1, "capture_partial"))

    def test_partial_samples_or_cpu_intervals_and_sampling_errors_are_partial(self):
        for variant in ("short", "missing_main_thread", "sampling_error", "cpu_backwards", "wrong_main_incarnation"):
            with self.subTest(variant=variant):
                capture = self.stable()
                if variant == "short":
                    capture["samples"].pop()
                elif variant == "missing_main_thread":
                    capture["samples"][1]["threads"] = []
                elif variant == "sampling_error":
                    capture["sampling_error"] = "main process disappeared"
                elif variant == "wrong_main_incarnation":
                    for row in capture["samples"]:
                        row["threads"][0]["start_ticks"] = 200
                else:
                    capture["samples"][-1]["threads"][0]["cpu_ticks"] = 0
                code, record = self.cli(capture)
                self.assertEqual((code, record["state"]), (1, "capture_partial"))

    def test_requested_moo_metadata_cannot_be_silently_absent(self):
        code, record = self.cli(self.stable(), moo=True)
        self.assertEqual((code, record["state"]), (1, "capture_partial"))
        self.assertFalse(record["capture_checks"]["requested_moo_metadata"])

    def test_valid_optional_moo_metadata_keeps_coverage_and_running_unknown(self):
        capture = self.stable()
        metadata = {"queued_count": 17, "queued_shown": 3, "groups": {}, "current_cycle": 123}
        capture["moo_state"] = {"requested": True, "ok": True,
                                "response": {"ok": 1, "value": json.dumps(json.dumps(metadata))}}
        # Brief CLI output requires these additional bounded diagnostic fields.
        metadata.update(scheduled_count=0, scheduled_overdue=0, oldest_scheduled_lag_s=0,
                        universe_cycling=1)
        capture["moo_state"]["response"]["value"] = json.dumps(json.dumps(metadata))
        code, record = self.cli(capture, moo=True)
        self.assertEqual((code, record["state"]), (0, "capture_complete"))
        self.assertEqual(record["summary"]["moo_state"]["queued_shown"], 3)
        self.assertEqual(record["summary"]["running_moo_verb"], "unknown")


if __name__ == "__main__":
    unittest.main()
