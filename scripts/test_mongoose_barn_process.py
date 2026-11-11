"""Pure lifecycle/ownership fixtures: never launch a process or send a signal."""
from contextlib import contextmanager
import errno
import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import unittest
from unittest.mock import Mock, patch

import mongoose_barn_process as barn


class OwnerFixture:
    def owner(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        exe, db, config, registry = (root / name for name in
            ("fixture-not-an-executable", "input.db", "mongoose-outbound-on.conf", "profiles.json"))
        exe.write_bytes(b"not executable; subprocess must be mocked")
        db.write_bytes(b"prepared image")
        config.write_text("OUTBOUND_NETWORK = 1\nPROMOTE_NUMBERS = 1\n")
        registry.write_text(json.dumps({"profiles": [{"profile_id": barn.PROFILE_ID,
            "runtime_os": "linux", "config_file": config.name, "support_status": "supported",
            "expected_features": {"option.OUTBOUND_NETWORK": True, "option.PROMOTE_NUMBERS": True}}]}))
        owner = barn.OwnedBarnServer(exe, db, root, 17778, config=config, registry=registry)
        owner.proc = Mock(pid=123456, returncode=0)
        owner.proc.poll.return_value = None
        owner.pidfd, owner.start_ticks = 77, 444
        return owner, root

    def profile(self, owner):
        owner.identity["prepared_database_sha256"] = barn.bench.sha256_file(owner.db)
        row = {"profile_id": barn.PROFILE_ID, "implementation": "barn", "runtime_os": "linux",
               "database_checksum": owner.identity["prepared_database_sha256"],
               "config_checksum": owner.identity["config_sha256"],
               "features": {"option.OUTBOUND_NETWORK": True, "option.PROMOTE_NUMBERS": True}}
        owner.profile_path.write_text(json.dumps(row))
        return row

    def owned_log(self, owner):
        owner.log_file = owner.log_path.open("xb")
        row = os.fstat(owner.log_file.fileno())
        owner._log_identity = (row.st_dev, row.st_ino)
        owner._checkpoint_launch_verified = True

    def publish_pair(self, owner, *, bad_header=False):
        for path, payload in ((owner.checkpoint_path, b"identical database bytes\n"),
                              (owner.sidecar_path, b"pending")):
            temporary = Path(str(path) + ".test-tmp")
            temporary.write_bytes(payload)
            if path == owner.sidecar_path:
                digest = "0" * 64 if bad_header else barn.bench.sha256_file(owner.checkpoint_path)
                temporary.write_bytes(("barn-waif-identities-v1 " + digest + "\n").encode())
            temporary.replace(path)

    def append_shutdown(self, owner, *, failure=None, complete=True):
        with owner.log_path.open("ab") as stream:
            stream.write(b'time=now level=INFO msg="SHUTDOWN: shutdown signal received"\n')
            if failure:
                stream.write(json.dumps({"msg": failure, "err":
                    "write database: write queued tasks: write queued task 473852115: write rtenv: runtime environment has 3 names for 24 values"}).encode() + b"\n")
            if complete:
                stream.write(b'time=now level=INFO msg="shutdown complete"\n')

    def mocked_stop(self, owner, callback):
        owner.proc.wait.side_effect = lambda **kwargs: callback()
        with patch.object(owner, "verify_owned"), patch.object(barn.signal, "pidfd_send_signal"), \
             patch.object(barn.os, "close"):
            return owner.stop()

    @contextmanager
    def mocked_readiness(self, owner, reads, times, *, guard_failures=None):
        child = owner.proc
        owner.proc = owner.pidfd = owner.start_ticks = None
        text = ('msg="operator endpoint listening" addr=127.0.0.1:8123\n'
                'msg="debug endpoint listening" addr=127.0.0.1:8124\n')
        actual_read = Path.read_text
        sequence = iter(reads)

        def read(path, *args, **kwargs):
            if path == owner.log_path:
                value = next(sequence)
                if isinstance(value, Exception):
                    raise value
                return text
            return actual_read(path, *args, **kwargs)

        def spawn(*args, **kwargs):
            owner.log_path.write_text(text)
            self.profile(owner)
            return child

        response = Mock(status=200)
        response.read.return_value = b"ready\n"
        context = Mock()
        context.__enter__ = Mock(return_value=response)
        context.__exit__ = Mock(return_value=False)
        try:
            with patch.object(barn.socket, "socket"), patch.object(barn.subprocess, "Popen", side_effect=spawn), \
                 patch.object(barn.os, "pidfd_open", return_value=77), \
                 patch.object(barn, "process_state", return_value={"start_ticks": 444}), \
                 patch.object(owner, "verify_owned", side_effect=guard_failures) as guard, \
                 patch.object(barn.bench, "wait_port"), \
                 patch.object(barn.urllib.request, "urlopen", return_value=context) as http, \
                 patch.object(barn.time, "monotonic", side_effect=times), \
                 patch.object(barn.time, "monotonic_ns", side_effect=[100, 200]), \
                 patch.object(barn.time, "sleep") as sleep, \
                 patch.object(Path, "read_text", autospec=True, side_effect=read):
                yield guard, http, sleep
        finally:
            if owner.log_file:
                owner.log_file.close()


class BarnProcessTests(OwnerFixture, unittest.TestCase):
    def test_startup_log_enodata_recovers_with_bounded_manifest_metadata(self):
        owner, _ = self.owner()
        manifest = {"server": owner.identity}
        failures = [OSError(errno.ENODATA, "unavailable"), OSError(errno.ENODATA, "unavailable")]
        with self.mocked_readiness(owner, failures + [None], [0, 1, 2, 3, 4]) as (guard, http, sleep):
            self.assertEqual(owner.start(), 4)
        self.assertEqual(manifest["server"]["startup_log_read_errors"], {
            "errno": errno.ENODATA, "count": 2, "first_monotonic_ns": 100, "last_monotonic_ns": 200})
        self.assertEqual(owner.identity["readiness"], {"status": 200, "body": "ready\n"})
        self.assertEqual(guard.call_count, 4)  # initial, each retry, actual ready
        self.assertEqual(http.call_count, 1)
        self.assertEqual([call.args for call in sleep.call_args_list], [(0.1,), (0.1,)])

    def test_startup_log_persistent_enodata_uses_original_deadline(self):
        owner, _ = self.owner()
        manifest = {"server": owner.identity}
        failures = [OSError(errno.ENODATA, "unavailable"), OSError(errno.ENODATA, "unavailable")]
        with self.mocked_readiness(owner, failures, [0, 1, 2, 181]) as (guard, http, sleep):
            with self.assertRaisesRegex(TimeoutError, "readiness"):
                owner.start()
        self.assertEqual(manifest["server"]["startup_log_read_errors"], {
            "errno": errno.ENODATA, "count": 2, "first_monotonic_ns": 100, "last_monotonic_ns": 200})
        self.assertEqual(owner.identity["last_readiness"], {"failure_type": "startup_log_read", "errno": errno.ENODATA})
        self.assertNotIn("runtime_profile", owner.identity)
        self.assertEqual(guard.call_count, 3)
        http.assert_not_called()
        self.assertEqual(sleep.call_count, 2)

    def test_startup_log_unrelated_oserrors_remain_fatal(self):
        for number in (errno.EACCES, errno.ENOENT, errno.EIO):
            with self.subTest(errno=number):
                owner, _ = self.owner()
                failure = OSError(number, "unavailable")
                with self.mocked_readiness(owner, [failure], [0, 1]) as (guard, http, sleep):
                    with self.assertRaises(OSError) as raised:
                        owner.start()
                self.assertIs(raised.exception, failure)
                self.assertNotIn("startup_log_read_errors", owner.identity)
                self.assertEqual(guard.call_count, 1)
                http.assert_not_called()
                sleep.assert_not_called()

    def test_startup_log_retry_identity_change_fails_closed(self):
        owner, _ = self.owner()
        with self.mocked_readiness(owner, [OSError(errno.ENODATA, "unavailable")], [0, 1],
                                   guard_failures=[None, RuntimeError("identity changed")]) as (guard, http, sleep):
            with self.assertRaisesRegex(RuntimeError, "identity changed"):
                owner.start()
        self.assertEqual(guard.call_count, 2)
        self.assertEqual(owner.identity["startup_log_read_errors"]["count"], 1)
        self.assertNotIn("runtime_profile", owner.identity)
        http.assert_not_called()
        sleep.assert_not_called()

    def test_retained_failed_dump_log_rejects_zero_exit_and_missing_pair(self):
        owner, _ = self.owner()
        self.owned_log(owner)
        result = self.mocked_stop(owner, lambda: self.append_shutdown(owner, failure="final checkpoint failed"))
        self.assertEqual(result["exit_code"], 0)
        self.assertFalse(result["forced"])
        self.assertEqual(result["final_checkpoint"]["status"], "failed")
        self.assertEqual(result["final_checkpoint"]["reason"], "final checkpoint failed")
        self.assertFalse(result["final_checkpoint"]["verified"])
        self.assertFalse(owner.checkpoint_path.exists())

    def test_signal_publication_needs_no_invented_success_log_or_changed_bytes(self):
        owner, _ = self.owner()
        self.owned_log(owner)
        self.publish_pair(owner)  # An earlier background dump is not sufficient.
        old_sha = barn.bench.sha256_file(owner.checkpoint_path)
        def shutdown():
            self.publish_pair(owner)  # Production atomic replacement, same bytes.
            self.append_shutdown(owner)
        result = self.mocked_stop(owner, shutdown)["final_checkpoint"]
        self.assertEqual(result["status"], "verified")
        self.assertTrue(result["verified"])
        self.assertEqual(result["outputs"]["database"]["sha256"], old_sha)
        self.assertEqual(result["outputs"]["waif_identities"]["size_bytes"], 89)
        self.assertNotIn("checkpoint complete", result["reason"])

    def test_missing_stale_corrupt_or_unfinished_output_never_verifies(self):
        for case in ("missing", "stale", "bad-header", "journal", "no-complete", "symlink", "failure-with-pair", "skipped"):
            with self.subTest(case=case):
                owner, _ = self.owner()
                self.owned_log(owner)
                if case == "stale":
                    self.publish_pair(owner)
                def shutdown():
                    if case not in ("missing", "stale"):
                        self.publish_pair(owner, bad_header=case == "bad-header")
                    if case == "journal":
                        owner.journal_path.write_bytes(b"unfinished")
                    if case == "symlink":
                        owner.sidecar_path.unlink()
                        owner.sidecar_path.symlink_to(owner.db)
                    failure = ("final checkpoint failed" if case == "failure-with-pair" else
                               "final checkpoint skipped (checkpointing disabled)" if case == "skipped" else None)
                    self.append_shutdown(owner, failure=failure, complete=case != "no-complete")
                result = self.mocked_stop(owner, shutdown)["final_checkpoint"]
                self.assertFalse(result["verified"])
                self.assertIn(result["status"], ("unknown", "failed"))
                if case in ("bad-header", "journal", "failure-with-pair", "skipped"):
                    self.assertEqual(result["status"], "failed")

    def test_prior_shutdown_markers_or_replaced_log_cannot_prove_our_final_dump(self):
        for replaced in (False, True):
            with self.subTest(replaced=replaced):
                owner, _ = self.owner()
                self.owned_log(owner)
                self.append_shutdown(owner)
                def shutdown():
                    self.publish_pair(owner)
                    if replaced:
                        temporary = Path(str(owner.log_path) + ".replacement")
                        temporary.write_bytes(owner.log_path.read_bytes())
                        temporary.replace(owner.log_path)
                        self.append_shutdown(owner)
                result = self.mocked_stop(owner, shutdown)["final_checkpoint"]
                self.assertEqual(result["status"], "unknown")
                self.assertFalse(result["verified"])

    def test_preexisting_output_pair_or_journal_refuses_launch_including_broken_symlink(self):
        for name in ("checkpoint_path", "sidecar_path", "journal_path"):
            with self.subTest(name=name):
                owner, root = self.owner()
                getattr(owner, name).symlink_to(root / "missing-target")
                with patch.object(barn.subprocess, "Popen") as spawn:
                    with self.assertRaisesRegex(RuntimeError, "absent before launch"):
                        owner.start()
                spawn.assert_not_called()

    def test_cpu_includes_system_and_user_threads_without_a_main_thread_alias(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            process = root / "123"
            process.mkdir()
            fields = ["0"] * 21
            fields[0], fields[11], fields[12], fields[19] = "R", "250", "50", "444"
            (process / "stat").write_text("123 (fixture ) name) " + " ".join(fields))
            (process / "status").write_text("VmRSS:\t123 kB\nVmHWM:\t456 kB\n")
            state = barn.process_state(123, proc_root=root, clock_ticks=100)
            self.assertEqual(state["process_total_cpu_ns"], 3_000_000_000)
            self.assertEqual(state["start_ticks"], 444)
            self.assertEqual(state["rss_kib"], 123)
            self.assertNotIn("main_cpu_ns", state)
            with self.assertRaises(RuntimeError):
                barn.process_state(123, proc_root=root, clock_ticks=0)

    def test_guard_checks_exact_executable_cwd_arguments_and_birth_identity(self):
        owner, root = self.owner()
        proc = root / "fake-proc" / str(owner.proc.pid)
        proc.mkdir(parents=True)
        (proc / "exe").symlink_to(owner.executable)
        (proc / "cwd").symlink_to(owner.run_dir)
        (proc / "cmdline").write_bytes(b"\0".join(os.fsencode(value) for value in owner.command) + b"\0")
        with patch.object(barn, "Path", side_effect=lambda value: root / "fake-proc" if value == "/proc" else Path(value)), \
             patch.object(barn, "process_state", return_value={"start_ticks": 444}) as state:
            owner.verify_owned()
            state.return_value = {"start_ticks": 445}
            with self.assertRaisesRegex(RuntimeError, "identity changed"):
                owner.verify_owned()
            state.return_value = {"start_ticks": 444}
            (proc / "cmdline").write_bytes(b"unrelated\0")
            with self.assertRaisesRegex(RuntimeError, "identity changed"):
                owner.verify_owned()
            (proc / "cmdline").write_bytes(b"\0".join(os.fsencode(value) for value in owner.command) + b"\0")
            (proc / "exe").unlink()
            (proc / "exe").symlink_to(owner.db)
            with self.assertRaisesRegex(RuntimeError, "identity changed"):
                owner.verify_owned()
            (proc / "exe").unlink()
            (proc / "exe").symlink_to(owner.executable)
            (proc / "cwd").unlink()
            (proc / "cwd").symlink_to(proc)
            with self.assertRaisesRegex(RuntimeError, "identity changed"):
                owner.verify_owned()

    def test_startup_identity_is_checked_after_exec_finishes_writing_argv(self):
        # Linux releases the Popen parent inside exec before writing the new
        # argv, so /proc/<pid>/cmdline reads empty right after Popen returns
        # (merge-queue Jepsen run 42862358939, generation 4).
        owner, root = self.owner()
        child = owner.proc
        owner.proc = owner.pidfd = owner.start_ticks = None
        proc = root / "fake-proc" / str(child.pid)
        proc.mkdir(parents=True)
        (proc / "exe").symlink_to(owner.executable)
        (proc / "cwd").symlink_to(owner.run_dir)
        argv = b"\0".join(os.fsencode(value) for value in owner.command) + b"\0"
        text = ('msg="operator endpoint listening" addr=127.0.0.1:8123\n'
                'msg="debug endpoint listening" addr=127.0.0.1:8124\n')

        def spawn(*args, **kwargs):
            (proc / "cmdline").write_bytes(b"")  # mid-exec: mm->env_end not yet set
            owner.log_path.write_text(text)
            self.profile(owner)
            return child

        def listening(*args):
            (proc / "cmdline").write_bytes(argv)  # exec finished before Barn could listen

        response = Mock(status=200)
        response.read.return_value = b"ready\n"
        context = Mock()
        context.__enter__ = Mock(return_value=response)
        context.__exit__ = Mock(return_value=False)
        try:
            with patch.object(barn, "Path", side_effect=lambda value: root / "fake-proc" if value == "/proc" else Path(value)), \
                 patch.object(barn.socket, "socket"), patch.object(barn.subprocess, "Popen", side_effect=spawn), \
                 patch.object(barn.os, "pidfd_open", return_value=77), \
                 patch.object(barn, "process_state", return_value={"start_ticks": 444}), \
                 patch.object(barn.bench, "wait_port", side_effect=listening), \
                 patch.object(barn.urllib.request, "urlopen", return_value=context), \
                 patch.object(barn.time, "monotonic", side_effect=[0, 1, 2]), \
                 patch.object(barn.time, "sleep"), \
                 patch.object(barn.signal, "pidfd_send_signal") as send:
                self.assertEqual(owner.start(), 2)
                (proc / "cmdline").write_bytes(b"unrelated\0")
                with self.assertRaisesRegex(RuntimeError, "identity changed"):
                    owner.verify_owned()
        finally:
            if owner.log_file:
                owner.log_file.close()
        self.assertTrue(owner._ownership_established)
        send.assert_not_called()

    def test_established_identity_mismatch_never_signals(self):
        owner, _ = self.owner()
        with patch.object(owner, "verify_owned", side_effect=RuntimeError("identity changed")), \
             patch.object(barn.signal, "pidfd_send_signal") as send, patch.object(barn.os, "close"):
            with self.assertRaisesRegex(RuntimeError, "identity changed"):
                owner.stop()
        send.assert_not_called()
        owner.proc.terminate.assert_not_called()
        owner.proc.kill.assert_not_called()

    def test_shutdown_waits_for_final_dump_and_forced_stop_is_explicit(self):
        owner, _ = self.owner()
        with patch.object(owner, "verify_owned") as guard, \
             patch.object(barn.signal, "pidfd_send_signal") as send, patch.object(barn.os, "close"):
            result = owner.stop()
        send.assert_called_once_with(77, signal.SIGTERM)
        owner.proc.wait.assert_called_once_with(timeout=90)
        guard.assert_called_once()
        self.assertEqual(result["exit_code"], 0)
        self.assertFalse(result["forced"])
        owner, _ = self.owner()
        owner.proc.returncode = -9
        owner.proc.wait.side_effect = [subprocess.TimeoutExpired("owned", 90), None]
        with patch.object(owner, "verify_owned") as guard, \
             patch.object(barn.signal, "pidfd_send_signal") as send, patch.object(barn.os, "close"):
            result = owner.stop()
        self.assertEqual(guard.call_count, 2)
        self.assertEqual([call.args for call in send.call_args_list], [(77, signal.SIGTERM), (77, signal.SIGKILL)])
        self.assertTrue(result["forced"])
        self.assertEqual(result["exit_code"], -9)

    def test_profile_presence_does_not_accept_wrong_configuration_or_prepared_image(self):
        owner, _ = self.owner()
        good = self.profile(owner)
        owner._validate_profile()
        for field, value in (("database_checksum", "another image"), ("config_checksum", "another config"),
                             ("runtime_os", "windows"), ("profile_id", "another profile")):
            row = dict(good, **{field: value})
            owner.profile_path.write_text(json.dumps(row))
            with self.assertRaisesRegex(RuntimeError, "profile does not match"):
                owner._validate_profile()

    def test_registered_feature_mismatch_rejects_manifest(self):
        owner, _ = self.owner()
        good = self.profile(owner)
        for features in ({"option.OUTBOUND_NETWORK": True}, {"option.OUTBOUND_NETWORK": 1, "option.PROMOTE_NUMBERS": True},
                         {"option.OUTBOUND_NETWORK": False, "option.PROMOTE_NUMBERS": True}):
            owner.profile_path.write_text(json.dumps(dict(good, features=features)))
            with self.assertRaisesRegex(RuntimeError, "profile does not match"):
                owner._validate_profile()

    def test_public_testdb_profile_comes_from_the_real_registry(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "barn").write_bytes(b"fixture")
            owner = barn.OwnedBarnServer(root / "barn", root / "Test.db", root, 17779,
                                         config=barn.ROOT / "profiles" / "barn" / "outbound-on.conf",
                                         profile_id="barn-linux-testdb-outbound-on")
            self.assertEqual(owner.command[owner.command.index("--profile-id") + 1], "barn-linux-testdb-outbound-on")
            self.assertEqual(owner.identity["profile_id"], "barn-linux-testdb-outbound-on")
            self.assertEqual(owner.expected_features, {"option.OUTBOUND_NETWORK": True,
                                                       "builtin.open_network_connection": "present"})
            with self.assertRaisesRegex(RuntimeError, "unavailable"):  # config must be the registered one
                barn.OwnedBarnServer(root / "barn", root / "Test.db", root, 17779,
                                     profile_id="barn-linux-testdb-outbound-on")
            with self.assertRaisesRegex(RuntimeError, "unavailable"):  # Windows diagnostics are not owned here
                barn.OwnedBarnServer(root / "barn", root / "Test.db", root, 17779,
                                     config=barn.ROOT / "profiles" / "barn" / "outbound-on.conf",
                                     profile_id="barn-windows-testdb-outbound-on")

    def test_pause_resume_crash_signal_only_the_verified_owned_pidfd(self):
        owner, _ = self.owner()
        owner.log_file = owner.log_path.open("xb")
        owner.proc.returncode = -9
        with patch.object(owner, "verify_owned") as guard, patch.object(barn, "run_state", return_value="T"), \
             patch.object(barn.signal, "pidfd_send_signal") as send, patch.object(barn.os, "close") as close:
            self.assertEqual(owner.pause(), "T")
            self.assertTrue(owner.paused)
            owner.resume()
            self.assertFalse(owner.paused)
            result = owner.crash()
        self.assertEqual([call.args for call in send.call_args_list],
                         [(77, signal.SIGSTOP), (77, signal.SIGCONT), (77, signal.SIGKILL)])
        self.assertEqual(guard.call_count, 3)
        owner.proc.wait.assert_called_once_with(timeout=10)
        close.assert_called_once_with(77)
        self.assertIsNone(owner.pidfd)
        self.assertTrue(owner.log_file.closed)
        self.assertEqual(result["exit_code"], -9)
        self.assertTrue(result["forced"])
        self.assertFalse(result["final_checkpoint"]["verified"])

    def test_pause_requires_the_kernel_to_report_the_owned_process_stopped(self):
        owner, _ = self.owner()
        with patch.object(owner, "verify_owned"), patch.object(barn.signal, "pidfd_send_signal"), \
             patch.object(barn, "run_state", return_value="S"):
            with self.assertRaisesRegex(RuntimeError, "not observed stopped"):
                owner.pause(timeout=0)
        with tempfile.TemporaryDirectory() as directory:
            (Path(directory) / "77").mkdir()
            (Path(directory) / "77" / "stat").write_text("77 (barn (x)) T 1 77 77 0 -1\n")
            self.assertEqual(barn.run_state(77, proc_root=Path(directory)), "T")

    def test_fault_methods_refuse_changed_identity_or_exited_process(self):
        for method in ("pause", "resume", "crash"):
            with self.subTest(method=method):
                owner, _ = self.owner()
                with patch.object(owner, "verify_owned", side_effect=RuntimeError("identity changed")), \
                     patch.object(barn.signal, "pidfd_send_signal") as send, patch.object(barn.os, "close"):
                    with self.assertRaisesRegex(RuntimeError, "identity changed"):
                        getattr(owner, method)()
                send.assert_not_called()
                owner, _ = self.owner()
                owner.proc.poll.return_value = 0
                with patch.object(owner, "verify_owned") as guard, \
                     patch.object(barn.signal, "pidfd_send_signal") as send, patch.object(barn.os, "close"):
                    with self.assertRaisesRegex(RuntimeError, "not running"):
                        getattr(owner, method)()
                guard.assert_not_called()
                send.assert_not_called()

    def test_endpoints_accept_actual_text_and_json_only_on_private_loopback(self):
        text = 'time=now msg="operator endpoint listening" addr=127.0.0.1:8123\n'
        text += json.dumps({"msg": "debug endpoint listening", "addr": "127.0.0.1:8124"})
        self.assertEqual(barn.endpoint_addresses(text), {"operator": "127.0.0.1:8123", "debug": "127.0.0.1:8124"})
        with self.assertRaisesRegex(RuntimeError, "private loopback"):
            barn.endpoint_addresses('msg="operator endpoint listening" addr=192.0.2.1:8123')

    def test_failed_pidfd_acquisition_reaps_only_its_unreaped_popen_child(self):
        owner, _ = self.owner()
        child = owner.proc
        owner.proc = owner.pidfd = owner.start_ticks = None
        with patch.object(barn.socket, "socket"), patch.object(barn.subprocess, "Popen", return_value=child), \
             patch.object(barn.os, "pidfd_open", side_effect=OSError("pidfd unavailable")):
            with self.assertRaisesRegex(OSError, "pidfd unavailable"):
                owner.start()
        child.terminate.assert_called_once()
        child.wait.assert_called_once_with(timeout=90)

    def test_missing_initial_stat_reaps_pinned_child_without_masking_primary(self):
        owner, _ = self.owner()
        child = owner.proc
        owner.proc = owner.pidfd = owner.start_ticks = None
        with patch.object(barn.socket, "socket"), patch.object(barn.subprocess, "Popen", return_value=child), \
             patch.object(barn.os, "pidfd_open", return_value=77), \
             patch.object(barn, "process_state", side_effect=OSError("initial stat unavailable")), \
             patch.object(barn.signal, "pidfd_send_signal") as send, patch.object(barn.os, "close"):
            with self.assertRaisesRegex(OSError, "initial stat unavailable"):
                owner.start()
        send.assert_called_once_with(77, signal.SIGTERM)
        child.wait.assert_called_once_with(timeout=90)
        child.terminate.assert_not_called()

    def test_startup_cleanup_failure_is_visible_without_replacing_primary(self):
        owner, _ = self.owner()
        child = owner.proc
        child.terminate.side_effect = RuntimeError("cleanup unavailable")
        owner.proc = owner.pidfd = owner.start_ticks = None
        with patch.object(barn.socket, "socket"), patch.object(barn.subprocess, "Popen", return_value=child), \
             patch.object(barn.os, "pidfd_open", side_effect=OSError("primary acquisition failure")):
            with self.assertRaisesRegex(OSError, "primary acquisition failure"):
                owner.start()
        self.assertEqual(owner.identity["startup_cleanup_failure"], "cleanup unavailable")
        self.assertTrue(owner.log_file.closed)

    def test_operator_not_ready_does_not_accept_existing_profile(self):
        owner, _ = self.owner()
        child = owner.proc
        owner.proc = owner.pidfd = owner.start_ticks = None
        def spawn(*args, **kwargs):
            owner.log_path.write_text('msg="operator endpoint listening" addr=127.0.0.1:8123\n'
                                      'msg="debug endpoint listening" addr=127.0.0.1:8124\n')
            self.profile(owner)
            return child
        response = Mock(status=503)
        response.read.return_value = b"not ready\n"
        context = Mock()
        context.__enter__ = Mock(return_value=response)
        context.__exit__ = Mock(return_value=False)
        with patch.object(barn.socket, "socket"), patch.object(barn.subprocess, "Popen", side_effect=spawn), \
             patch.object(barn.os, "pidfd_open", return_value=77), \
             patch.object(barn, "process_state", return_value={"start_ticks": 444}), \
             patch.object(owner, "verify_owned"), patch.object(barn.bench, "wait_port"), \
             patch.object(barn.urllib.request, "urlopen", return_value=context), \
             patch.object(barn.time, "monotonic", side_effect=[0, 1, 181]), patch.object(barn.time, "sleep"):
            with self.assertRaisesRegex(TimeoutError, "readiness"):
                owner.start()
        self.assertEqual(owner.identity["last_readiness"]["status"], 503)
        with patch.object(owner, "verify_owned"), patch.object(barn.signal, "pidfd_send_signal"), patch.object(barn.os, "close"):
            owner.stop()


def digest_json(hex_digest):
    """A Go [32]byte as encoding/json writes it: a list of numbers."""
    return list(bytes.fromhex(hex_digest))


class CheckpointArtifactTests(unittest.TestCase):
    """Frozen publication files laid out as db/format/checkpoint_publication.go leaves them."""

    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.db = self.root / "input.db"
        self.db.write_bytes(b"input")
        self.out = Path(str(self.db) + ".new")
        self.tmp = Path(str(self.db) + ".tmp")

    def pair(self, path, body):
        path.write_bytes(body)
        Path(str(path) + ".waifids").write_bytes(b"sidecar of " + body)
        return {"database": barn.bench.sha256_file(path),
                "waif_identities": barn.bench.sha256_file(Path(str(path) + ".waifids"))}

    def journal(self, recovery, next_pair, previous_pair):
        row = {"Version": 1, "Generation": recovery.name,
               "Next": {"Database": digest_json(next_pair["database"]),
                        "Sidecar": digest_json(next_pair["waif_identities"]), "HasSidecar": True}}
        if previous_pair:
            row["Previous"] = {"Database": digest_json(previous_pair["database"]),
                               "Sidecar": digest_json(previous_pair["waif_identities"]), "HasSidecar": True}
        Path(str(self.out) + ".publication").write_text(json.dumps(row))

    def stage(self, name, previous=True):
        """Build the files of one stage; returns (previous pair A, staged pair F)."""
        a = self.pair(self.out, b"checkpoint A") if previous else None
        f = self.pair(self.tmp, b"checkpoint F")
        recovery = self.root / (self.out.name + ".recovery-x1")
        if name == "staging":
            Path(str(self.tmp) + ".waifids").unlink()
            return a, None
        if name == "sidecar-prepared":
            return a, f
        recovery.mkdir()
        self.pair(recovery / "next", b"checkpoint F")
        if previous:
            self.pair(recovery / "previous", b"checkpoint A")
        if name == "cleanup":
            self.tmp.rename(self.out)
            Path(str(self.tmp) + ".waifids").rename(Path(str(self.out) + ".waifids"))
            return a, f
        self.journal(recovery, f, a)
        if name in ("database-renamed", "both-renamed"):
            self.tmp.rename(self.out)
        if name == "both-renamed":
            Path(str(self.tmp) + ".waifids").rename(Path(str(self.out) + ".waifids"))
        return a, f

    def test_each_stage_is_classified_from_the_files_with_its_required_recovery(self):
        expected = {"staging": "A", "sidecar-prepared": "A", "journal-published": "A",
                    "database-renamed": "A", "both-renamed": "F", "cleanup": "output"}
        for name, wanted in expected.items():
            with self.subTest(stage=name):
                self.setUp()
                a, f = self.stage(name)
                artifacts = barn.checkpoint_artifacts(self.db)
                self.assertEqual(artifacts["stage"], name)
                recovery = barn.expected_recovery(artifacts)
                self.assertEqual(recovery, {"A": a, "F": f, "output": f}[wanted])
        self.setUp()
        artifacts = barn.checkpoint_artifacts(self.db)
        self.assertEqual((artifacts["stage"], barn.expected_recovery(artifacts)), ("not-staged", None))

    def test_first_publication_recovers_its_saved_next_pair_and_journal_digests_are_hex(self):
        for name in ("journal-published", "database-renamed"):
            with self.subTest(stage=name):
                self.setUp()
                _, f = self.stage(name, previous=False)
                artifacts = barn.checkpoint_artifacts(self.db)
                self.assertEqual(artifacts["stage"], name)
                self.assertIsNone(artifacts["journal"]["previous"])
                self.assertEqual(artifacts["journal"]["next"], f)
                self.assertEqual(barn.expected_recovery(artifacts), f)
                self.assertEqual(artifacts["recovery_directories"][self.out.name + ".recovery-x1"]["next"], f)
        self.setUp()
        self.stage("staging", previous=False)
        self.assertIsNone(barn.expected_recovery(barn.checkpoint_artifacts(self.db)))  # unchanged input

    def test_unreadable_journal_is_reported_not_guessed(self):
        self.stage("journal-published")
        Path(str(self.out) + ".publication").write_text("{not json")
        artifacts = barn.checkpoint_artifacts(self.db)
        self.assertIn("error", artifacts["journal"])
        with self.assertRaisesRegex(ValueError, "unreadable publication journal"):
            barn.expected_recovery(artifacts)

    def test_forensic_copy_keeps_every_artifact_and_never_moves_one(self):
        self.stage("journal-published")
        before = {path: path.read_bytes() for path in self.root.rglob("*") if path.is_file()}
        copied = barn.preserve_artifacts(self.db, self.root / "forensic")
        self.assertEqual({path: path.read_bytes() for path in before}, before)
        names = {Path(row["copy"]).relative_to(self.root / "forensic").as_posix() for row in copied}
        self.assertEqual(names, {"input.db", "input.db.new", "input.db.new.waifids", "input.db.tmp",
                                 "input.db.tmp.waifids", "input.db.new.publication",
                                 "input.db.new.recovery-x1/next", "input.db.new.recovery-x1/next.waifids",
                                 "input.db.new.recovery-x1/previous", "input.db.new.recovery-x1/previous.waifids"})
        for row in copied:
            self.assertEqual(row["sha256"], barn.bench.sha256_file(Path(row["source"])))


class LiveCheckpointOwnerTests(OwnerFixture, unittest.TestCase):
    def test_ordinary_launch_refuses_an_input_with_a_pending_journal_but_a_declared_recovery_proceeds(self):
        owner, root = self.owner()
        owner.input_journal_path.write_text("{}")
        with patch.object(barn.subprocess, "Popen") as spawn:
            with self.assertRaisesRegex(RuntimeError, "pending checkpoint publication journal"):
                owner.start()
        spawn.assert_not_called()
        owner.recovered_input = {"kind": "interrupted-checkpoint-output"}
        owner.proc = owner.pidfd = owner.start_ticks = None
        with patch.object(barn.socket, "socket"), \
             patch.object(barn.subprocess, "Popen", side_effect=RuntimeError("spawned")):
            with self.assertRaisesRegex(RuntimeError, "spawned"):
                owner.start()
        self.assertEqual(owner.identity["input_journal_sha256"], barn.bench.sha256_file(owner.input_journal_path))
        self.assertEqual(owner.identity["input_pair"], {"database": barn.bench.sha256_file(owner.db),
                                                        "waif_identities": None})

    def test_absent_input_is_refused_before_launch(self):
        owner, _ = self.owner()
        owner.db.unlink()
        with patch.object(barn.subprocess, "Popen") as spawn:
            with self.assertRaisesRegex(RuntimeError, "input database is absent"):
                owner.start()
        spawn.assert_not_called()

    def test_recovered_pair_is_the_intact_pair_left_after_recovery(self):
        owner, _ = self.owner()
        owner.db.write_bytes(b"recovered")
        sidecar = Path(str(owner.db) + ".waifids")
        sidecar.write_bytes(("barn-waif-identities-v1 " + barn.bench.sha256_file(owner.db) + "\n").encode())
        pair = owner._recovered_pair()
        self.assertEqual((pair["database"]["sha256"], pair["waif_identities"]["sha256"]),
                         (barn.bench.sha256_file(owner.db), barn.bench.sha256_file(sidecar)))
        owner.input_journal_path.write_text("{}")
        with self.assertRaisesRegex(RuntimeError, "still has its publication journal"):
            owner._recovered_pair()
        owner.input_journal_path.unlink()
        sidecar.write_bytes(b"barn-waif-identities-v1 " + b"0" * 64 + b"\n")
        with self.assertRaisesRegex(RuntimeError, "not intact"):
            owner._recovered_pair()

    def test_checkpoint_markers_read_text_and_json_records(self):
        data = (b'time=now level=INFO msg="checkpoint started"\n'
                b'time=now level=INFO msg="checkpoint complete" duration_ms=17\n'
                b'{"level":"INFO","msg":"checkpoint started"}\n'
                b'{"level":"ERROR","msg":"checkpoint failed","err":"disk"}\n'
                b'{"level":"INFO","msg":"checkpoint complete","duration_ms":4}\n')
        markers = barn.checkpoint_markers(data)
        self.assertEqual((markers["started"], markers["complete"]), (2, [17, 4]))
        self.assertEqual(markers["failed"], ['{"level":"ERROR","msg":"checkpoint failed","err":"disk"}'])

    def test_request_signals_sigalrm_and_completion_needs_the_log_record_and_a_fresh_pair(self):
        owner, _ = self.owner()
        self.owned_log(owner)
        with patch.object(owner, "verify_owned"), patch.object(barn.signal, "pidfd_send_signal") as send:
            boundary = owner.request_checkpoint()
        self.assertEqual([call.args for call in send.call_args_list], [(77, signal.SIGALRM)])
        self.assertLessEqual(boundary["signal_ns"], boundary["signalled_ns"])
        with owner.log_path.open("ab") as stream:
            stream.write(b'time=now level=INFO msg="checkpoint started"\n')
        with patch.object(barn.time, "monotonic", side_effect=[0, 1, 2]), patch.object(barn.time, "sleep"):
            with self.assertRaisesRegex(TimeoutError, "no checkpoint completion"):
                owner.wait_checkpoint(boundary, 1)
        self.publish_pair(owner)
        with owner.log_path.open("ab") as stream:
            stream.write(b'time=now level=INFO msg="checkpoint complete" duration_ms=9\n')
        result = owner.wait_checkpoint(boundary, 1)
        self.assertEqual((result["status"], result["duration_ms"], result["started"]), ("complete", 9, 1))
        self.assertEqual(result["outputs"]["database"]["sha256"], barn.bench.sha256_file(owner.checkpoint_path))
        owner.journal_path.write_text("{}")
        with self.assertRaisesRegex(RuntimeError, "journal remains"):
            owner.wait_checkpoint(boundary, 1)
        owner.log_file.close()

    def test_failed_checkpoint_and_exit_are_reported_not_acknowledged(self):
        owner, _ = self.owner()
        self.owned_log(owner)
        with patch.object(owner, "verify_owned"), patch.object(barn.signal, "pidfd_send_signal"):
            boundary = owner.request_checkpoint()
        owner.proc.poll.return_value = 1
        with self.assertRaisesRegex(RuntimeError, "exited"):
            owner.wait_checkpoint(boundary, 5)
        with owner.log_path.open("ab") as stream:
            stream.write(b'{"level":"ERROR","msg":"checkpoint failed","err":"disk full"}\n')
        result = owner.wait_checkpoint(boundary, 5)
        self.assertEqual(result["status"], "failed")
        self.assertNotIn("outputs", result)
        owner.log_file.close()


if __name__ == "__main__":
    unittest.main()
