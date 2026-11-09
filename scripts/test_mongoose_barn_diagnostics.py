"""Pure HTTP/process fixtures. No server, socket, subprocess or world is launched."""
import gzip
import hashlib
import json
import os
from pathlib import Path
import socket
import tempfile
import threading
import time
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

import mongoose_barn_diagnostics as diagnostics

PROFILE = gzip.compress(b"transport fixture; NOT a validated protobuf/sample profile")
VARS = json.dumps({"memstats": {"NumGC": 2}, "barn.runtime_phases": {},
                   "barn.gc_sweeps": 4}).encode()


class Response:
    def __init__(self, body, content_type, status=200, *, on_end=None, headers=None):
        self.body, self.status = body, status
        self.headers = {"Content-Type": content_type, **(headers or {})}
        self.offset = 0
        self.on_end = on_end
        self.closed = False

    def getheader(self, key):
        return self.headers.get(key)

    def read(self, count):
        chunk = self.body[self.offset:self.offset + count]
        self.offset += len(chunk)
        if not chunk and self.on_end:
            self.on_end()
            self.on_end = None
        return chunk

    def close(self):
        self.closed = True


class DiagnosticTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.server = SimpleNamespace(engine="barn", proc=SimpleNamespace(pid=123), start_ticks=456,
            run_dir=self.root, identity={"endpoints": {"debug": "127.0.0.1:8123"},
                                          "executable_sha256": "a" * 64}, verify_owned=Mock())
        self.responses = []
        self.connections = []
        self.http = patch.object(diagnostics.http.client, "HTTPConnection", side_effect=self.connection)
        self.http.start()
        self.addCleanup(self.http.stop)

    def connection(self, *args, **kwargs):
        # An unplanned request fails instead of ever contacting a real endpoint.
        if not self.responses:
            raise AssertionError("unexpected HTTP request")
        response = self.responses.pop(0)
        conn = Mock()
        conn.sock = Mock()
        conn.getresponse.side_effect = response if callable(response) or isinstance(response, Exception) else None
        if not callable(response) and not isinstance(response, Exception):
            conn.getresponse.return_value = response
        self.connections.append((args, kwargs, conn))
        return conn

    def collector(self, name="diagnostics"):
        return diagnostics.Collector(self.server, self.root / name)

    def snapshots(self):
        self.responses.extend((Response(VARS, "application/json; charset=utf-8"),
                               Response(PROFILE, "application/octet-stream")))

    def test_snapshot_is_sequential_natural_heap_and_retains_hashes(self):
        collector = self.collector()
        self.snapshots()
        result = collector.snapshot("before")
        self.assertEqual([conn.request.call_args.args for _, _, conn in self.connections],
                         [("GET", "/debug/vars"), ("GET", "/debug/pprof/heap?gc=0&debug=0")])
        for kind, body in (("vars", VARS), ("heap", PROFILE)):
            row = result[kind]
            self.assertEqual(row["status"], "captured")
            self.assertEqual(row["sha256"], hashlib.sha256(body).hexdigest())
            self.assertEqual((collector.directory / row["artifact"]).read_bytes(), body)
            self.assertTrue(row["artifact_final"])
            self.assertFalse(row["activation_acknowledged"])
            self.assertLessEqual(row["scheduled_monotonic_ns"], row["finished_monotonic_ns"])
        self.assertEqual(result["heap"]["pprof_semantic_validation"], "pending offline validation")
        self.assertFalse(result["vars"]["metric_availability"]["barn.tasks_live"])

    def test_preexisting_output_directory_is_never_reused(self):
        directory = self.root / "preserve"
        directory.mkdir()
        (directory / "owned-by-user").write_bytes(b"preserve")
        with self.assertRaises(FileExistsError):
            diagnostics.Collector(self.server, directory)
        self.assertEqual((directory / "owned-by-user").read_bytes(), b"preserve")
        self.assertFalse(self.connections)

    def test_bad_endpoint_engine_and_external_output_fail_before_http(self):
        for address in ("http://127.0.0.1:8123", "localhost:8123", "192.0.2.1:8123",
                        "127.0.0.1:0", "127.0.0.1:65536"):
            with self.subTest(address=address):
                self.server.identity["endpoints"]["debug"] = address
                with self.assertRaises(ValueError):
                    self.collector()
        self.server.identity["endpoints"]["debug"] = "127.0.0.1:8123"
        self.server.engine = "toast"
        with self.assertRaises(ValueError):
            self.collector()
        self.server.engine = "barn"
        with tempfile.TemporaryDirectory() as elsewhere, self.assertRaises(ValueError):
            diagnostics.Collector(self.server, Path(elsewhere) / "external")
        self.assertFalse(self.connections)

    def test_environment_proxies_are_unused_and_redirects_never_followed(self):
        collector = self.collector()
        self.responses.extend((Response(b"redirect", "text/plain", 302,
                                        headers={"Location": "http://192.0.2.1/forbidden"}),
                               Response(PROFILE, "application/octet-stream")))
        with patch.dict(os.environ, {"http_proxy": "http://192.0.2.2:99", "HTTP_PROXY": "http://192.0.2.2:99"}), \
                self.assertRaises(diagnostics.DiagnosticError) as raised:
            collector.snapshot("redirect")
        self.assertEqual(raised.exception.evidence["captures"]["vars"]["http_status"], 302)
        self.assertEqual(len(self.connections), 2)  # vars plus heap, no redirect request
        self.assertTrue(all(args == ("127.0.0.1", 8123) for args, _, _ in self.connections))
        self.assertNotIn("Location", raised.exception.evidence["captures"]["vars"]["headers"])

    def test_http_error_body_is_private_and_does_not_become_metadata(self):
        collector = self.collector()
        body = b"private synthetic credential-bearing payload"
        self.responses.extend((Response(body, "text/plain", 500, headers={"Set-Cookie": "private-cookie"}),
                               Response(PROFILE, "application/octet-stream")))
        with self.assertRaises(diagnostics.DiagnosticError) as raised:
            collector.snapshot("failed")
        row = raised.exception.evidence["captures"]["vars"]
        self.assertEqual((collector.directory / row["artifact"]).read_bytes(), body)
        self.assertNotIn("private", json.dumps(row))
        self.assertEqual(collector.finish()["status"], "failed")

    def test_wrong_content_types_cannot_be_captures(self):
        collector = self.collector()
        self.responses.extend((Response(VARS, "text/plain"), Response(PROFILE, "application/json")))
        with self.assertRaises(diagnostics.DiagnosticError) as raised:
            collector.snapshot("types")
        self.assertTrue(all(row["status"] == "failed" for row in raised.exception.evidence["captures"].values()))

    def test_malformed_or_missing_native_vars_structure_is_failure(self):
        for index, body in enumerate((b"invalid JSON", b"[]", b"{}")):
            with self.subTest(body=body):
                collector = self.collector("vars" + str(index))
                self.responses.extend((Response(body, "application/json"), Response(PROFILE, "application/octet-stream")))
                with self.assertRaises(diagnostics.DiagnosticError):
                    collector.snapshot("bad")

    def test_body_cap_keeps_bounded_partial_bytes_and_marks_failure(self):
        collector = self.collector()
        self.snapshots()
        with patch.dict(diagnostics.BODY_LIMITS, {"vars": 8}), self.assertRaises(diagnostics.DiagnosticError) as raised:
            collector.snapshot("capped")
        row = raised.exception.evidence["captures"]["vars"]
        self.assertEqual((collector.directory / row["artifact"]).read_bytes(), VARS[:8])
        self.assertEqual(row["bytes"], 8)
        self.assertEqual(row["sha256"], hashlib.sha256(VARS[:8]).hexdigest())
        self.assertIn("byte limit", row["failure_reason"])

    def test_truncated_or_expansion_capped_profile_is_not_successful(self):
        for index, body in enumerate((PROFILE[:-4], gzip.compress(b"x" * 20))):
            with self.subTest(index=index):
                collector = self.collector("gzip" + str(index))
                self.responses.extend((Response(VARS, "application/json"), Response(body, "application/octet-stream")))
                cap = diagnostics.EXPANDED_LIMIT if index == 0 else 10
                with patch.object(diagnostics, "EXPANDED_LIMIT", cap), self.assertRaises(diagnostics.DiagnosticError) as raised:
                    collector.snapshot("bad")
                row = raised.exception.evidence["captures"]["heap"]
                if index == 0:
                    self.assertEqual(row["failure_type"], "EOFError")
                else:
                    self.assertIn("expanded profile", row["failure_reason"])
                self.assertEqual((collector.directory / "bad-heap.pprof").read_bytes(), body)

    def test_identity_changes_before_or_after_requests_are_not_accepted(self):
        collector = self.collector()
        self.server.start_ticks += 1
        with self.assertRaises(diagnostics.DiagnosticError):
            collector.snapshot("before")
        self.assertFalse(self.connections)
        self.server.start_ticks -= 1
        collector = self.collector("after")
        self.responses.append(Response(VARS, "application/json", on_end=lambda: setattr(self.server, "start_ticks", 999)))
        with self.assertRaises(diagnostics.DiagnosticError) as raised:
            collector.snapshot("after")
        self.assertEqual(raised.exception.evidence["captures"]["vars"]["status"], "failed")
        self.assertEqual((collector.directory / "after-vars.json").read_bytes(), VARS)

    def test_error_response_also_records_after_identity_failure(self):
        collector = self.collector()
        self.responses.append(Response(b"failed", "text/plain", 500,
                                       on_end=lambda: setattr(self.server, "start_ticks", 999)))
        with self.assertRaises(diagnostics.DiagnosticError) as raised:
            collector.snapshot("error")
        row = raised.exception.evidence["captures"]["vars"]
        self.assertFalse(row["identity_after_verified"])

    def test_cpu_is_async_request_issued_is_not_an_activation_ack(self):
        collector = self.collector()
        blocked, release = threading.Event(), threading.Event()
        def response():
            blocked.set()
            if not release.wait(2):
                raise AssertionError("fixture was not released")
            return Response(PROFILE, "application/octet-stream")
        self.responses.append(response)
        handle = collector.start_cpu("flight", 10)
        try:
            self.assertTrue(handle.request_issued.wait(1))
            self.assertTrue(blocked.wait(1))
            self.assertFalse(handle.done.is_set())
            self.assertFalse(handle.row["activation_acknowledged"])
            self.assertNotIn("headers_received_monotonic_ns", handle.row)
            with self.assertRaises(RuntimeError):
                collector.start_cpu("overlap", 1)
        finally:
            release.set()
        row = collector.finish_cpu(handle)
        self.assertEqual(row["status"], "captured")
        self.assertEqual(self.connections[0][2].request.call_args.args,
                         ("GET", "/debug/pprof/profile?seconds=10"))

    def test_invalid_cpu_window_fails_before_artifact_or_request(self):
        collector = self.collector()
        for seconds in (True, 0, 31, 1.5, "10"):
            with self.subTest(seconds=seconds), self.assertRaises(ValueError):
                collector.start_cpu("cpu", seconds)
        self.assertFalse(list(collector.directory.iterdir()))
        self.assertFalse(self.connections)

    def test_deadline_aborts_owned_socket_and_preserves_failed_capture(self):
        collector = self.collector()
        entered, aborted = threading.Event(), threading.Event()
        def response():
            entered.set()
            if not aborted.wait(2):
                raise AssertionError("owned socket was not shut down")
            raise OSError("fixture interrupted")
        self.responses.append(response)
        handle = collector.start_cpu("deadline", 1)
        self.assertTrue(entered.wait(1))
        self.connections[0][2].sock.shutdown.side_effect = lambda _: aborted.set()
        handle.deadline = time.monotonic() - 1
        with self.assertRaises(diagnostics.DiagnosticError):
            collector.finish_cpu(handle)
        self.connections[0][2].sock.shutdown.assert_called_once_with(socket.SHUT_RDWR)
        self.assertTrue(handle.done.is_set())
        self.assertEqual(collector.finish()["status"], "failed")

    def test_worker_error_propagates_safe_evidence_and_finish_is_idempotent(self):
        collector = self.collector()
        self.responses.append(RuntimeError("private exception payload"))
        handle = collector.start_cpu("failed", 1)
        with self.assertRaises(diagnostics.DiagnosticError) as raised:
            collector.finish_cpu(handle)
        self.assertEqual(raised.exception.evidence["failure_type"], "RuntimeError")
        self.assertNotIn("private", json.dumps(raised.exception.evidence))
        first = collector.finish()
        self.assertIs(first, collector.finish())
        self.assertEqual(first["status"], "failed")
        self.assertEqual(json.loads((collector.directory / "manifest.json").read_text()), first)

    def test_unowned_leaf_is_preserved_and_not_hashed_as_capture(self):
        collector = self.collector()
        saved = collector.directory / "same-vars.json"
        saved.write_bytes(b"existing user bytes")
        self.responses.append(Response(PROFILE, "application/octet-stream"))
        with self.assertRaises(diagnostics.DiagnosticError) as raised:
            collector.snapshot("same")
        row = raised.exception.evidence["captures"]["vars"]
        self.assertFalse(row["artifact_owned"])
        self.assertNotIn("sha256", row)
        self.assertEqual(saved.read_bytes(), b"existing user bytes")
        self.assertEqual(collector.finish()["status"], "failed")

    def test_hash_failure_does_not_leave_an_unset_completion_event(self):
        collector = self.collector()
        self.responses.append(Response(VARS, "application/json"))
        with patch.object(hashlib, "file_digest", side_effect=OSError("hash unavailable")), \
                patch.object(diagnostics, "SNAPSHOT_TIMEOUT", 0.05):
            handle = collector._launch("hash", "vars", "/debug/vars", 0.05)
            self.assertTrue(handle.done.wait(0.5))
            with self.assertRaises(diagnostics.DiagnosticError) as raised:
                collector._wait(handle)
            self.assertEqual(raised.exception.evidence["artifact_failure_type"], "OSError")

    def test_replaced_artifact_path_cannot_become_successful_capture(self):
        collector = self.collector()
        self.responses.append(Response(VARS, "application/json"))
        original = Path.stat
        def replaced(path, *args, **kwargs):
            found = original(path, *args, **kwargs)
            if path.name == "replaced-vars.json":
                return SimpleNamespace(st_dev=found.st_dev, st_ino=found.st_ino + 1)
            return found
        with patch.object(Path, "stat", replaced):
            handle = collector._launch("replaced", "vars", "/debug/vars", 1)
            with self.assertRaises(diagnostics.DiagnosticError) as raised:
                collector._wait(handle)
        self.assertFalse(raised.exception.evidence["artifact_final"])
        self.assertIn("pathname was replaced", raised.exception.evidence["failure_reason"])
        self.assertEqual(raised.exception.evidence["sha256"], hashlib.sha256(VARS).hexdigest())

    def test_foreign_handle_and_unsafe_label_never_issue_requests(self):
        collector, other = self.collector(), self.collector("other")
        foreign = diagnostics.Capture(other, "foreign", "cpu", "/debug/pprof/profile?seconds=1", 1)
        with self.assertRaises(ValueError):
            collector.finish_cpu(foreign)
        for label in ("../escape", "x" * 49, 1, "line\nfeed"):
            with self.subTest(label=label), self.assertRaises(ValueError):
                collector.snapshot(label)
        self.assertFalse(self.connections)

    def test_failed_thread_start_still_has_a_failed_finish_manifest(self):
        collector = self.collector()
        with patch.object(threading.Thread, "start", side_effect=RuntimeError("thread unavailable")), \
                self.assertRaises(diagnostics.DiagnosticError):
            collector.snapshot("thread")
        self.assertEqual(collector.finish()["status"], "failed")

    def test_manifest_conflict_does_not_overwrite_and_finished_collector_rejects_work(self):
        collector = self.collector()
        (collector.directory / "manifest.json").write_bytes(b"existing")
        result = collector.finish()
        self.assertEqual(result["status"], "failed")
        self.assertEqual((collector.directory / "manifest.json").read_bytes(), b"existing")
        with self.assertRaises(RuntimeError):
            collector.snapshot("late")


if __name__ == "__main__":
    unittest.main()
