"""Pure privacy/bounds/correlation fixtures; no worlds, commands or signals."""
import copy
import json
from pathlib import Path
import re
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

import mongoose_error_capture as capture


def projected_fixture(*, structured=False, issue_time=123):
    frame = {"receiver": 777, "verb": "scale_focus", "programmer": 2 if structured else None,
             "definer": 16980, "player": -1 if structured else None, "line": 18}
    event = {"history_index": 1, "representation": "structured" if structured else "formatted",
             "frames": [frame], "counts": dict.fromkeys(capture.COUNTERS, 0)}
    issue = copy.deepcopy(event)
    issue.update(history_index=0, representation="formatted")
    issue["frames"][0].update(programmer=None, player=None)
    return {"schema": 1, "actor": 100, "history_total": 1, "history_omitted": 0,
            "history_malformed": 0, "events": [event], "issue_status": "available", "event_player": 100,
            "issue_time": issue_time, "history_matches": [] if structured else [1],
            "correlation": "no_match" if structured else "unique_bounded_exact_list_match",
            "issue_unknown_fields": 0, "output_truncated": 0, "issue": issue}


class ErrorCaptureTests(unittest.TestCase):
    def environment(self, rows):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        run = Path(temporary.name)
        (run / "files").mkdir()
        directory = run / "magic"
        directory.mkdir()
        control, server = Mock(), Mock()
        server.proc.pid, server.start_ticks = 234, 555
        def evaluate(code, **kwargs):
            match = re.search(r'file_open\("(error-[a-f0-9]{32}\.json)"', code)
            self.assertIsNotNone(match)
            path = run / "files" / match.group(1)
            self.assertTrue(path.exists())
            row = rows.pop(0) if len(rows) > 1 else rows[0]
            raw = json.dumps(row).encode()
            path.write_bytes(raw + b"\n")
            return [1, len(raw), row.get("output_truncated", 0)], ["SECRET output must never be retained"]
        control.eval_metadata.side_effect = evaluate
        return control, server, directory

    def test_formatted_metadata_does_not_infer_frame_player_or_programmer(self):
        row = projected_fixture()
        validated = capture.validate_projection(json.dumps(row).encode(), 100)
        self.assertEqual(validated["event_player"], 100)
        self.assertIsNone(validated["events"][0]["frames"][0]["player"])
        for field in ("programmer", "player"):
            bad = copy.deepcopy(row)
            bad["issue"]["frames"][0][field] = 100
            with self.assertRaises(ValueError):
                capture.validate_projection(json.dumps(bad).encode(), 100)

    def test_structured_six_slots_remain_distinct_from_actor_issue_context(self):
        row = capture.validate_projection(json.dumps(projected_fixture(structured=True)).encode(), 100)
        self.assertEqual(row["event_player"], 100)
        self.assertEqual(row["events"][0]["frames"][0]["player"], -1)
        self.assertEqual(row["events"][0]["frames"][0]["programmer"], 2)
        self.assertEqual(row["correlation"], "no_match")

    def test_future_payload_fields_fail_closed_without_persisting_secrets(self):
        for extra in ("msg", "value", "traceback", "source", "args", "runtime_variables", "future_slot"):
            with self.subTest(extra=extra):
                row = projected_fixture()
                row["events"][0]["frames"][0][extra] = "SECRET credential"
                control, server, directory = self.environment([row])
                result = capture.capture(control, server, "#100", directory, "baseline")
                self.assertEqual(result["status"], "failed")
                self.assertNotIn("SECRET", json.dumps(result))
                self.assertEqual(list((directory.parent / "files").iterdir()), [])
                self.assertEqual(list(directory.iterdir()), [])

    def test_event_frame_verb_and_byte_caps_reject_oversized_fixtures(self):
        base = projected_fixture()
        oversized = copy.deepcopy(base)
        oversized["events"] *= 16
        frames = copy.deepcopy(base)
        frames["issue"]["frames"] *= 33
        verb = copy.deepcopy(base)
        verb["issue"]["frames"][0]["verb"] = "a" * 257
        for row in (oversized, frames, verb):
            with self.assertRaises(ValueError):
                capture.validate_projection(json.dumps(row).encode(), 100)
        with self.assertRaises(ValueError):
            capture.validate_projection(b" " * (capture.BYTE_CAP + 2), 100)

    def test_malformed_or_anonymous_coverage_is_counted_without_raw_headers(self):
        row = projected_fixture()
        row["issue"]["frames"] = []
        row["issue"]["counts"].update(malformed=2, omitted=9, unsupported_anonymous=1,
                                       unsupported_receiver=1, unknown_fields=3, truncated_headers=1)
        valid = capture.validate_projection(json.dumps(row).encode(), 100)
        self.assertEqual(valid["issue"]["counts"]["omitted"], 9)
        self.assertNotIn('"header"', json.dumps(valid))

    def test_no_correlation_or_ambiguous_match_cannot_claim_new_event_count(self):
        row = projected_fixture()
        row.update(history_matches=[1, 2], correlation="ambiguous", history_total=2)
        control, server, directory = self.environment([row])
        result = capture.capture(control, server, "#100", directory, "failure_before_cleanup")
        self.assertEqual(result["status"], "collected")
        self.assertIsNone(result["comparison"]["new_event_count"])
        self.assertEqual(result["comparison"]["command_exclusive_causality"], "unknown")

    def test_bounded_retry_waits_for_actor_issue_append_and_retains_capture_chronology(self):
        prior = projected_fixture(issue_time=123)
        control, server, directory = self.environment([prior, projected_fixture(issue_time=124)])
        with patch.object(capture.time, "sleep") as sleep:
            result = capture.capture(control, server, "#100", directory, "failure_before_cleanup", baseline={"projection": prior})
        self.assertEqual(control.eval_metadata.call_count, 2)
        sleep.assert_called_once_with(0.1)
        self.assertEqual(result["pid"], 234)
        self.assertEqual(result["start_ticks"], 555)
        for attempt in result["attempts"]:
            self.assertGreaterEqual(attempt["finished_monotonic_ns"], attempt["started_monotonic_ns"])
            self.assertGreaterEqual(attempt["latency_ns"], 0)
        self.assertEqual(server.verify_owned.call_count, 4)
        self.assertNotIn("SECRET", Path(result["artifact"]).read_text())

    def test_unchanged_issue_does_not_retry_forever(self):
        row = projected_fixture()
        control, server, directory = self.environment([row])
        with patch.object(capture.time, "sleep"):
            result = capture.capture(control, server, "#100", directory, "failure_before_cleanup", baseline={"projection": row})
        self.assertEqual(len(result["attempts"]), 3)
        self.assertFalse(result["comparison"]["issue_time_changed"])

    def test_native_failure_records_type_without_message_traceback_or_output(self):
        control, server, directory = self.environment([projected_fixture()])
        control.eval_metadata.side_effect = RuntimeError("SECRET password and native local values")
        result = capture.capture(control, server, "#100", directory, "baseline")
        self.assertEqual(result["failure_type"], "RuntimeError")
        self.assertNotIn("SECRET", json.dumps(result))
        self.assertEqual(list((directory.parent / "files").iterdir()), [])

    def test_exclusive_reservation_collision_preserves_preexisting_file(self):
        control, server, directory = self.environment([projected_fixture()])
        filename = "error-" + "a" * 32 + ".json"
        existing = directory.parent / "files" / filename
        existing.write_bytes(b"SECRET preexisting unrelated evidence")
        with patch.object(capture.uuid, "uuid4", return_value=SimpleNamespace(hex="a" * 32)):
            result = capture.capture(control, server, "#100", directory, "baseline")
        self.assertEqual(result["status"], "failed")
        self.assertEqual(existing.read_bytes(), b"SECRET preexisting unrelated evidence")
        control.eval_metadata.assert_not_called()
        self.assertNotIn("SECRET", json.dumps(result))

    def test_replaced_incoming_identity_is_not_followed_or_removed(self):
        for symlink in (False, True):
            with self.subTest(symlink=symlink):
                control, server, directory = self.environment([projected_fixture()])
                evaluate = control.eval_metadata.side_effect
                foreign = directory / "foreign.json"
                foreign.write_bytes(json.dumps(projected_fixture()).encode() + b"\n")
                paths = []
                def replaced(code, **kwargs):
                    ack, lines = evaluate(code, **kwargs)
                    name = re.search(r'file_open\("(error-[a-f0-9]{32}\.json)"', code).group(1)
                    path = directory.parent / "files" / name
                    path.unlink()
                    if symlink:
                        path.symlink_to(foreign)
                    else:
                        foreign.replace(path)
                    paths.append(path)
                    return ack, lines
                control.eval_metadata.side_effect = replaced
                result = capture.capture(control, server, "#100", directory, "baseline")
                self.assertEqual(result["status"], "failed")
                self.assertTrue(paths[0].exists())
                if symlink:
                    self.assertTrue(foreign.exists())

    def test_issue_write_before_history_append_retries_until_bounded_unique_join(self):
        early = projected_fixture(issue_time=124)
        early.update(correlation="no_match", history_matches=[])
        control, server, directory = self.environment([early, projected_fixture(issue_time=124)])
        with patch.object(capture.time, "sleep") as sleep:
            result = capture.capture(control, server, "#100", directory, "failure_before_cleanup")
        self.assertEqual(len(result["attempts"]), 2)
        sleep.assert_called_once_with(0.1)
        self.assertEqual(result["projection"]["correlation"], "unique_bounded_exact_list_match")

    def test_request_projects_before_export_and_caps_comparison_before_equal(self):
        code = capture.projection_request("#100", "error-" + "a" * 32 + ".json")
        self.assertIn('length(r) == 4', code)
        self.assertIn('length(r) == 5', code)
        self.assertIn('for k in [1..6]', code)
        self.assertIn('issueok && candidateok && equal(il,source)', code)
        self.assertIn('text[1..min(length(text),1024)]', code)
        self.assertNotIn('generate_json(h)', code)
        self.assertNotIn('generate_json(issue)', code)
        self.assertNotIn('r[2]', code)
        self.assertNotIn('r[3]', code)
        self.assertNotIn('fr[7]', code)
        self.assertLess(len(code.split('return ')[-1]), 100)

    def test_native_moo_object_actor_and_explicit_output_truncation(self):
        import bench_players
        control, server, directory = self.environment([{"schema": 1, "actor": 100, "output_truncated": 1}])
        result = capture.capture(control, server, bench_players.MooObj(100), directory, "baseline")
        self.assertEqual(result["status"], "collected")
        self.assertEqual(result["projection"]["output_truncated"], 1)


if __name__ == "__main__":
    unittest.main()
