"""Native collector safety/failure contracts; never launch or sample Mongoose."""
from contextlib import ExitStack
import json
import os
from pathlib import Path
import sys
from types import SimpleNamespace
import tempfile
import unittest
from unittest.mock import Mock, patch

import mongoose_native_profile as native


@unittest.skipUnless(sys.platform == "linux", "Linux native perf/pidfd fixtures require Linux")
class NativeCollectorTests(unittest.TestCase):
    def test_default_off_does_not_probe_or_inspect_target_and_finish_is_idempotent(self):
        with tempfile.TemporaryDirectory() as temporary, \
                patch.object(native, "capability") as probe, patch.object(native, "_target") as target:
            directory = Path(temporary) / "native"
            collector = native.begin(None, directory, SimpleNamespace())
            first = collector.finish()
            self.assertIs(first, collector.finish())
            self.assertEqual(first["status"], "disabled")
            self.assertFalse(first["instrumented_diagnostic"])
            probe.assert_not_called(); target.assert_not_called()
            self.assertEqual(json.loads((directory / "native.json").read_text()), first)

    def test_preexisting_destination_is_preserved_before_probe(self):
        with tempfile.TemporaryDirectory() as temporary, patch.object(native, "capability") as probe:
            directory = Path(temporary)
            (directory / "perf.data").write_bytes(b"user data")
            with self.assertRaises(FileExistsError):
                native.begin(None, directory, SimpleNamespace(native_mode="auto"))
            self.assertEqual((directory / "perf.data").read_bytes(), b"user data")
            probe.assert_not_called()

    def test_auto_unavailable_does_not_touch_target_or_spawn(self):
        with tempfile.TemporaryDirectory() as temporary, patch.object(native.platform, "system", return_value="Windows"), \
                patch.object(native, "_target") as target, patch.object(native.subprocess, "Popen") as spawn:
            collector = native.begin(None, Path(temporary) / "native", SimpleNamespace(native_mode="auto"))
            self.assertEqual(collector.finish()["status"], "unavailable")
            target.assert_not_called(); spawn.assert_not_called()

    def test_required_unavailable_retains_manifest_before_raising(self):
        with tempfile.TemporaryDirectory() as temporary, patch.object(native, "capability", return_value={
                "available": False, "reason": "software event permission denied"}):
            directory = Path(temporary) / "native"
            with self.assertRaises(native.NativeProfileError) as raised:
                native.begin(None, directory, SimpleNamespace(native_mode="required"))
            saved = json.loads((directory / "native.json").read_text())
            self.assertEqual(saved, raised.exception.manifest)
            self.assertIs(raised.exception.evidence, raised.exception.manifest)
            self.assertEqual(saved["status"], "unavailable")
            self.assertIn("permission denied", saved["reason"])

    def test_capability_io_failure_is_explicitly_unavailable(self):
        with tempfile.TemporaryDirectory() as temporary, patch.object(native, "capability", side_effect=OSError("broken perf launcher")):
            result = native.begin(None, Path(temporary) / "native", SimpleNamespace(native_mode="auto")).finish()
            self.assertEqual(result["status"], "unavailable")
            self.assertIn("broken perf launcher", result["reason"])

    def test_invalid_frequency_cannot_create_artifacts_or_start_probe(self):
        with tempfile.TemporaryDirectory() as temporary, patch.object(native, "capability") as probe:
            for hz in (True, 0, 1001, "99"):
                path = Path(temporary) / str(hz)
                with self.assertRaises(ValueError):
                    native.begin(None, path, SimpleNamespace(native_mode="required", native_hz=hz))
                self.assertFalse(path.exists())
            probe.assert_not_called()

    def test_owned_target_identity_failure_prevents_attachment(self):
        with tempfile.TemporaryDirectory() as temporary, patch.object(native, "capability", return_value={"available": True}), \
                patch.object(native, "_target", side_effect=RuntimeError("start ticks changed")), \
                patch.object(native.subprocess, "Popen") as spawn:
            result = native.begin(None, Path(temporary) / "native", SimpleNamespace(native_mode="auto")).finish()
            self.assertEqual(result["status"], "failed")
            self.assertIn("start ticks changed", result["reason"])
            spawn.assert_not_called()

    def configured(self, root, stack):
        executable = root / "perf"
        executable.write_bytes(b"\x7fELF owned fixture perf")
        workspace = root / "linux-local"
        workspace.mkdir()
        capabilities = {"available": True, "perf": str(executable), "perf_sha256": native._sha(executable),
                        "workspace": str(workspace)}
        process = Mock(pid=222, returncode=None)
        process.poll.side_effect = lambda: process.returncode
        process.wait.side_effect = lambda timeout: setattr(process, "returncode", 0)
        server = SimpleNamespace(proc=Mock(pid=111), pidfd=999, verify_owned=Mock())
        identity = {"pid": 222, "parent_pid": os.getpid(), "start_ticks": 333, "executable": str(executable)}
        target = {"pid": 111, "main_tid": 111, "start_ticks": 444, "sha256": "a" * 64}
        stack.enter_context(patch.object(native, "capability", return_value=capabilities))
        stack.enter_context(patch.object(native, "_target", return_value=target))
        stack.enter_context(patch.object(native, "_state", return_value=identity))
        spawn = stack.enter_context(patch.object(native.subprocess, "Popen", return_value=process))
        stack.enter_context(patch.object(native.os, "pidfd_open", side_effect=lambda _: os.open(os.devnull, os.O_RDONLY)))
        control = stack.enter_context(patch.object(native.Collector, "_control"))
        send = stack.enter_context(patch.object(native.signal, "pidfd_send_signal"))
        collector = native.begin(server, root / "native", SimpleNamespace(native_mode="required", native_hz=99))
        return collector, workspace, spawn, control, send, identity

    def report_commands(self, arguments, directory, name, **kwargs):
        (directory / (name + ".stdout")).write_bytes(b"moo 111 1.234: cpu-clock:u: 123 moo_profile::refresh_stack\n 456 caller\n")
        (directory / (name + ".stderr")).write_bytes(b"")
        return {"argv": arguments, "exit_code": 0}

    def test_collects_main_tid_and_signals_only_owned_perf_pidfd(self):
        with tempfile.TemporaryDirectory() as temporary, ExitStack() as stack:
            collector, workspace, spawn, control, send, _ = self.configured(Path(temporary), stack)
            argv = spawn.call_args.args[0]
            self.assertIn("--tid=111", argv); self.assertIn("--no-inherit", argv)
            self.assertIn("--event=cpu-clock:u", argv); self.assertNotIn("--pid=111", argv)
            self.assertIn("--delay=-1", argv)
            self.assertNotIn("--call-graph=dwarf,8192", argv)
            owned_fd = collector.pidfd
            (workspace / "perf.data").write_bytes(b"raw native data")
            stack.enter_context(patch.object(native, "_command", side_effect=self.report_commands))
            result = collector.finish()
            self.assertEqual(result["status"], "collected")
            self.assertEqual(result["sample_count"], 1)
            self.assertEqual((collector.directory / "perf.data").read_bytes(), b"raw native data")
            self.assertFalse((collector.directory / "perf-report-inclusive.stdout").exists())
            send.assert_called_once_with(owned_fd, native.signal.SIGINT)
            self.assertNotEqual(owned_fd, collector.server.pidfd)
            self.assertIsNone(collector.pidfd)
            control.assert_any_call("enable"); control.assert_any_call("disable")
            self.assertIs(result, collector.finish())

    def test_changed_collector_identity_refuses_signal_and_retains_failure(self):
        with tempfile.TemporaryDirectory() as temporary, ExitStack() as stack:
            collector, _, _, _, send, identity = self.configured(Path(temporary), stack)
            changed = dict(identity, start_ticks=9999)
            stack.enter_context(patch.object(native, "_state", return_value=changed))
            result = collector.finish()
            self.assertEqual(result["status"], "failed")
            self.assertIn("refusing to signal", result["reason"])
            send.assert_not_called()
            self.assertEqual(json.loads((collector.directory / "native.json").read_text())["status"], "failed")

    def test_zero_samples_are_not_reported_as_collected(self):
        with tempfile.TemporaryDirectory() as temporary, ExitStack() as stack:
            collector, workspace, _, _, _, _ = self.configured(Path(temporary), stack)
            (workspace / "perf.data").write_bytes(b"empty event header")
            def empty(arguments, directory, name, **kwargs):
                (directory / (name + ".stdout")).write_bytes(b"")
                return {"argv": arguments, "exit_code": 0}
            stack.enter_context(patch.object(native, "_command", side_effect=empty))
            self.assertEqual(collector.finish()["status"], "empty")

    def test_report_failure_is_retained_without_raising_over_domain_exception(self):
        with tempfile.TemporaryDirectory() as temporary, ExitStack() as stack:
            collector, workspace, _, _, _, _ = self.configured(Path(temporary), stack)
            (workspace / "perf.data").write_bytes(b"failed report")
            stack.enter_context(patch.object(native, "_command", side_effect=RuntimeError("native decoder failed")))
            result = collector.finish()
            self.assertEqual(result["status"], "failed")
            self.assertIn("native decoder failed", result["reason"])
            self.assertEqual((collector.directory / "perf.data").read_bytes(), b"failed report")

    def test_sample_count_excludes_callchains_comments_and_lost_records(self):
        with tempfile.TemporaryDirectory() as temporary:
            source = Path(temporary) / "script"
            source.write_text("# cpu-clock:u: header\nmoo 111 1.234: cpu-clock:u: leaf\n 123 caller\nLOST cpu-clock:u: 4\n")
            self.assertEqual(native._samples(source), 1)

    def test_missing_unwinder_selects_explicit_flat_fallback_with_nonzero_proof(self):
        with tempfile.TemporaryDirectory() as temporary, ExitStack() as stack:
            root = Path(temporary)
            perf = root / "perf"
            perf.write_bytes(b"\x7fELF test")
            workspace = root / "staging"
            workspace.mkdir()
            artifacts = root / "artifacts"
            artifacts.mkdir()
            stack.enter_context(patch.object(native.shutil, "which", side_effect=lambda value: str(perf) if value == "perf" else None))
            stack.enter_context(patch.object(native.tempfile, "mkdtemp", return_value=str(workspace)))
            commands = []
            def command(arguments, directory, name, **kwargs):
                commands.append(arguments)
                if "record" in arguments:
                    output = next(item.split("=", 1)[1] for item in arguments if item.startswith("--output="))
                    Path(output).write_bytes(b"perf data")
                (directory / (name + ".stdout")).write_bytes(b"python 555 1.234: cpu-clock:u: symbol\n")
                warning = b"Please install libunwind or libdw development packages during the perf build.\n" if name == "perf-probe-report" else b""
                (directory / (name + ".stderr")).write_bytes(warning)
                return {"argv": arguments, "exit_code": 0}
            stack.enter_context(patch.object(native, "_command", side_effect=command))
            result = native.capability(artifacts, 99)
            self.assertTrue(result["available"])
            self.assertEqual(result["callgraph"], "none")
            self.assertEqual(result["user_stack_dump_bytes"], 0)
            self.assertEqual(result["sample_count"], 1)
            self.assertIn("flat instruction-pointer", result["callgraph_unavailable_reason"])
            records = [argv for argv in commands if "record" in argv]
            self.assertEqual(len(records), 2)
            self.assertIn("--call-graph=dwarf,8192", records[0])
            self.assertNotIn("--call-graph=dwarf,8192", records[1])
            self.assertEqual((artifacts / "probe-flat.data").read_bytes(), b"perf data")

    def test_exclusive_native_data_copy_preserves_existing_artifact(self):
        with tempfile.TemporaryDirectory() as temporary:
            source, destination = Path(temporary) / "source", Path(temporary) / "destination"
            source.write_bytes(b"new perf data"); destination.write_bytes(b"existing evidence")
            with self.assertRaises(FileExistsError):
                native._copy(source, destination)
            self.assertEqual(destination.read_bytes(), b"existing evidence")

    def test_requested_owned_perf_sigint_keeps_raw_exit_and_valid_capture(self):
        with tempfile.TemporaryDirectory() as temporary, ExitStack() as stack:
            collector, workspace, _, _, send, _ = self.configured(Path(temporary), stack)
            (workspace / "perf.data").write_bytes(b"native data")
            collector.process.wait.side_effect = lambda timeout: setattr(collector.process, "returncode", -native.signal.SIGINT)
            stack.enter_context(patch.object(native, "_command", side_effect=self.report_commands))
            result = collector.finish()
            self.assertEqual(result["status"], "collected")
            self.assertEqual(result["record_exit_code"], -2)
            self.assertEqual(result["collector_stop_signal"], 2)
            send.assert_called_once_with(send.call_args.args[0], native.signal.SIGINT)

    def test_unrequested_sigint_exit_is_failed_even_with_valid_reports(self):
        with tempfile.TemporaryDirectory() as temporary, ExitStack() as stack:
            collector, workspace, _, _, send, _ = self.configured(Path(temporary), stack)
            (workspace / "perf.data").write_bytes(b"native data")
            collector.process.returncode = -native.signal.SIGINT
            stack.enter_context(patch.object(native, "_command", side_effect=self.report_commands))
            result = collector.finish()
            self.assertEqual(result["status"], "failed")
            self.assertNotIn("collector_stop_signal", result)
            send.assert_not_called()

    def test_forced_stop_remains_failed_despite_requested_sigint(self):
        with tempfile.TemporaryDirectory() as temporary, ExitStack() as stack:
            collector, workspace, _, _, _, _ = self.configured(Path(temporary), stack)
            (workspace / "perf.data").write_bytes(b"native data")
            collector.process.wait.side_effect = lambda timeout: setattr(collector.process, "returncode", -native.signal.SIGINT)
            collector.manifest["collector_forced_shutdown"] = True
            stack.enter_context(patch.object(native, "_command", side_effect=self.report_commands))
            self.assertEqual(collector.finish()["status"], "failed")


@unittest.skipUnless(sys.platform == "linux", "Linux native perf/pidfd fixtures require Linux")
class PerfControlTests(unittest.TestCase):
    def collector(self):
        collector = native.Collector(None, Path("unused"), "required", 99)
        collector.process = Mock()
        collector.process.poll.return_value = None
        collector.control_fd, collector.ack_fd = 81, 82
        return collector

    def scripted(self, chunks, stack):
        collector = self.collector()
        stack.enter_context(patch.object(native.os, "write", return_value=7))
        reads = stack.enter_context(patch.object(native.os, "read", side_effect=chunks))
        selects = stack.enter_context(patch.object(native.select, "select",
            side_effect=[([82], [], [])] * len(chunks) + [([], [], [])]))
        return collector, reads, selects

    def test_canonical_full_frame_retains_all_ack_bytes(self):
        with ExitStack() as stack:
            collector, reads, _ = self.scripted([b"ack\n\x00"], stack)
            collector._control("enable")
            self.assertEqual(collector.manifest["controls"][0]["ack_hex"], "61636b0a00")
            reads.assert_called_once_with(82, 6)

    def test_fragmented_nul_consumed_before_next_command(self):
        with ExitStack() as stack:
            collector, reads, _ = self.scripted([b"a", b"ck", b"\n", b"\x00"], stack)
            collector._control("enable")
            self.assertEqual(reads.call_count, 4)
            self.assertEqual(reads.call_args.args, (82, 2))
            stack.enter_context(patch.object(native.select, "select", side_effect=[([82], [], []), ([], [], [])]))
            stack.enter_context(patch.object(native.os, "read", return_value=b"ack\n\x00"))
            collector._control("disable")
            self.assertEqual([row["command"] for row in collector.manifest["controls"]], ["enable", "disable"])

    def test_wrong_prefix_or_same_read_junk_is_rejected(self):
        for frame in (b"no\n\x00", b"ack\r\x00", b"ack\n\x00x", b"ack\n\x00\x00"):
            with self.subTest(frame=frame), ExitStack() as stack:
                collector, _, _ = self.scripted([frame], stack)
                with self.assertRaisesRegex(RuntimeError, "unexpected perf control"):
                    collector._control("enable")
                self.assertNotIn("controls", collector.manifest)

    def test_separate_buffered_junk_is_rejected(self):
        with ExitStack() as stack:
            collector, _, _ = self.scripted([b"ack\n\x00", b"x"], stack)
            with self.assertRaisesRegex(RuntimeError, "unexpected data after"):
                collector._control("enable")
            self.assertNotIn("controls", collector.manifest)

    def test_missing_nul_closed_pipe_is_not_acknowledgement(self):
        with ExitStack() as stack:
            collector, _, _ = self.scripted([b"ack\n", b""], stack)
            with self.assertRaisesRegex(RuntimeError, "pipe closed"):
                collector._control("enable")
            self.assertNotIn("controls", collector.manifest)

    def test_partial_frame_cannot_extend_five_second_deadline(self):
        with ExitStack() as stack:
            collector, reads, _ = self.scripted([b"ack\n"], stack)
            stack.enter_context(patch.object(native.time, "monotonic", side_effect=[100, 100, 105]))
            with self.assertRaisesRegex(TimeoutError, "timed out"):
                collector._control("enable")
            self.assertEqual(reads.call_count, 1)
            self.assertNotIn("controls", collector.manifest)

    def test_select_timeout_retains_no_success(self):
        with ExitStack() as stack:
            collector, reads, _ = self.scripted([], stack)
            with self.assertRaisesRegex(TimeoutError, "timed out"):
                collector._control("enable")
            reads.assert_not_called()

    def test_exited_collector_before_or_after_read_is_rejected(self):
        for polls in ([1], [None, 1], [None, None, 1]):
            with self.subTest(polls=polls), ExitStack() as stack:
                collector, _, _ = self.scripted([b"ack\n\x00"], stack)
                collector.process.poll.side_effect = polls
                with self.assertRaisesRegex(RuntimeError, "collector exited"):
                    collector._control("enable")
                self.assertNotIn("controls", collector.manifest)


if __name__ == "__main__":
    unittest.main()
