"""Safety regressions for the multiplayer benchmark; no real servers/signals."""
import json
import os
import random
import signal
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock, patch

import bench_players as bench


class ToastCleanupTests(unittest.TestCase):
    def server(self, root):
        server = bench.ToastServer.__new__(bench.ToastServer)
        server.run_dir = root
        server.db_copy = root / "run.db"
        server.db_wsl = "/tmp/this-session/run.db"
        server.pid_file = root / "toast.pid"
        server.port = 7777
        server.proc = None
        server.log_file = None
        return server

    def test_no_launched_pid_never_searches_or_signals_existing_listener(self):
        with tempfile.TemporaryDirectory() as root:
            server = self.server(Path(root))
            calls = []

            def fake_wsl(*args, **kwargs):
                calls.append(args)
                if args[0] == "bash" and "ss " in args[-1]:
                    return 'LISTEN users:(("moo",pid=99999,fd=3))'
                if args[0] == "bash":
                    return bench.TOAST_MOO + " /tmp/another-session/run.db 7777"
                return ""

            with patch.object(bench, "wsl", side_effect=fake_wsl):
                server.stop()
            self.assertEqual(calls, [])

    def test_cleanup_uses_only_recorded_pid_and_exact_paths(self):
        with tempfile.TemporaryDirectory() as root:
            server = self.server(Path(root))
            server.pid_file.write_text("12345", encoding="ascii")
            with patch.object(bench, "wsl", return_value="") as call:
                server.stop()
            args = call.call_args.args
            self.assertEqual(args[:2], ("python3", "-c"))
            self.assertEqual(args[3:], ("12345", bench.TOAST_MOO, server.db_wsl))


class CleanupReportTests(unittest.TestCase):
    def test_cleanup_failure_preserves_original_error_and_database(self):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            source = root / "source.db"
            source.write_bytes(b"disposable test fixture")
            out = root / "run"
            server = Mock(identity={})
            server.start.side_effect = RuntimeError("startup failed")
            server.stop.side_effect = RuntimeError("ownership mismatch")
            with (patch.object(sys, "argv", ["bench", "--engine", "toast", "--db", str(source), "--out", str(out)]),
                  patch.object(bench, "load_login_script", return_value=("proxy", "user", "pass")),
                  patch.object(bench, "ToastServer", return_value=server),
                  patch.object(bench, "log")):
                self.assertEqual(bench.main(), 1)
            report = json.loads((out / "run.json").read_text())
            self.assertEqual(report["error"], "RuntimeError: startup failed")
            self.assertEqual(report["cleanup_error"], "RuntimeError: ownership mismatch")
            self.assertTrue((out / "run.db").is_file())


class LinuxOwnershipTests(unittest.TestCase):
    def check_identity(self, executable, database, expected_signal):
        argv = [bench.TOAST_MOO, database, database + ".new", "7777"]
        raw = b"\0".join(a.encode() for a in argv) + b"\0"
        with (patch.object(sys, "argv", ["cleanup", "12345", bench.TOAST_MOO, "/tmp/this-session/run.db"]),
              patch.object(os, "pidfd_open", return_value=42, create=True) as opened,
              patch.object(os, "readlink", return_value=executable),
              patch.object(os.path, "realpath", return_value=bench.TOAST_MOO),
              patch.object(Path, "read_bytes", return_value=raw),
              patch.object(signal, "pidfd_send_signal", create=True) as sent,
              patch.object(os, "close") as closed):
            if expected_signal:
                exec(bench.STOP_OWNED_TOAST, {})
                sent.assert_called_once_with(42, signal.SIGTERM)
            else:
                with self.assertRaisesRegex(RuntimeError, "refusing to signal"):
                    exec(bench.STOP_OWNED_TOAST, {})
                sent.assert_not_called()
            opened.assert_called_once_with(12345)
            closed.assert_called_once_with(42)

    def test_same_basename_in_another_run_is_not_owned(self):
        self.check_identity(bench.TOAST_MOO, "/tmp/another-session/run.db", False)

    def test_another_executable_is_not_owned(self):
        self.check_identity("/usr/bin/other", "/tmp/this-session/run.db", False)

    def test_exact_owned_process_is_signalled_through_pidfd(self):
        self.check_identity(bench.TOAST_MOO, "/tmp/this-session/run.db", True)


class ScriptedConnection:
    def __init__(self, lines):
        self.lines = iter(lines)
        self.sent = []

    def send_line(self, line):
        self.sent.append(line)

    def read_line(self, deadline):
        return next(self.lines, None)


class CommandCompletionTests(unittest.TestCase):
    def test_suffix_alone_is_not_terminal_completion(self):
        conn = ScriptedConnection([bench.PREFIX_TAG, bench.SUFFIX_TAG])
        stats = bench.PlayerStat([bench.ShapeStat()])
        with patch.object(bench, "SHAPES", [("look", "look", 1)]):
            bench.run_window([conn], [random.Random(0)], [stats], 0.01, True, 1)
        self.assertEqual(stats.shapes[0].ok, 0)
        self.assertEqual(stats.shapes[0].fail, 1)
        self.assertIsNotNone(stats.broken)

    def test_terminal_ack_collects_tail_and_ignores_stale_framing(self):
        conn = ScriptedConnection([
            bench.SUFFIX_TAG, "old output", bench.PREFIX_TAG, "before",
            bench.SUFFIX_TAG, "after", bench.SUFFIX_TAG,
            "===BP-DONE=== stale", "===BP-DONE=== nonce",
        ])
        with patch.object(bench.uuid, "uuid4", return_value=Mock(hex="nonce")):
            body = bench.completed_command(conn, "look", 1)
        self.assertEqual(body, ["before", "after"])
        self.assertEqual(conn.sent, ["look", "#$#bench-terminal nonce"])

    def test_warmup_timeout_stops_the_connection(self):
        conn = ScriptedConnection([bench.PREFIX_TAG, bench.SUFFIX_TAG])
        stats = bench.PlayerStat([bench.ShapeStat()])
        with patch.object(bench, "SHAPES", [("look", "look", 1)]):
            bench.run_window([conn], [random.Random(0)], [stats], 0.01, False, 1)
        self.assertIsNotNone(stats.broken)
        self.assertEqual(conn.sent.count("look"), 1)
        self.assertEqual(stats.shapes[0].ok, 0)


if __name__ == "__main__":
    unittest.main()
