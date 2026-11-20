"""Safety contracts for the managed offline profiler controller."""
import json
from contextlib import closing
import sqlite3
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

import profile_offline_mongoose as profiler


class OfflineProfilerTests(unittest.TestCase):
    def test_sound_catalog_copy_preserves_source_and_counts_real_rows(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'catalog.sqlite'
            with closing(sqlite3.connect(source)) as db:
                with db:
                    db.execute('CREATE TABLE Sounds(sound_id INTEGER)')
                    db.execute('INSERT INTO Sounds VALUES(44028)')
            digest = profiler.bench.sha256_file(source)
            run_dir = root / 'run'
            (run_dir / 'files').mkdir(parents=True)
            result = profiler.prepare_sound_catalog(source, digest, run_dir)
            self.assertEqual(result['sounds_rows'], 1)
            self.assertEqual(result['source_sha256'], result['copy_initial_sha256'])
            self.assertEqual(profiler.bench.sha256_file(source), digest)
            self.assertEqual(result['sqlite_quick_check'], 'ok')

    def test_pending_wal_or_wrong_hash_cannot_be_silently_ignored(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'catalog.sqlite'
            source.write_bytes(b'incomplete')
            run_dir = root / 'run'
            (run_dir / 'files').mkdir(parents=True)
            with self.assertRaisesRegex(ValueError, 'hash mismatch'):
                profiler.prepare_sound_catalog(source, '0' * 64, run_dir)
            Path(str(source) + '-wal').write_bytes(b'pending')
            with self.assertRaisesRegex(ValueError, 'nonempty WAL'):
                profiler.prepare_sound_catalog(source, profiler.bench.sha256_file(source), run_dir)
            self.assertFalse((run_dir / 'files' / 'sqlite').exists())

    def test_metadata_mode_is_explicit_and_detailed_remains_legacy_compatible(self):
        self.assertEqual('return moo_cpu_profile("start", 100, 65536);',
                         profiler.profile_start_request(100, "detailed"))
        self.assertEqual('return moo_cpu_profile("start", 100, 65536, "definitions");',
                         profiler.profile_start_request(100, "definitions"))
        for rate, mode in ((0, "detailed"), (2000, "definitions"), (True, "detailed"), (100, "unknown")):
            with self.assertRaises(ValueError):
                profiler.profile_start_request(rate, mode)

    def test_emergency_bootstrap_propagates_create_error_then_finishes_account(self):
        original = (';set_thread_mode(0)\n;;for p in (players()) w = p; endfor '
                    'try $account_manager:create_account("", "benchboot", "abc"); except e (ANY) endtry '
                    'a = $account_manager.accounts["benchboot"]; return {"bootstrap", 1};\nquit\n')
        adapted = profiler.split_emergency_bootstrap(original)
        self.assertNotIn("except", adapted)
        lines = adapted.splitlines()
        self.assertEqual(lines[1], ';;$account_manager:create_account("", "benchboot", "abc");')
        self.assertTrue(lines[2].startswith(";;for p in (players())"))
        self.assertIn('return {"bootstrap", 1}', lines[2])

    def test_bootstrap_contract_drift_is_rejected(self):
        with self.assertRaises(RuntimeError):
            profiler.split_emergency_bootstrap(";;unfamiliar code;\nquit\n")

    def test_same_network_namespace_is_rejected(self):
        output = type("Result", (), {"stdout": json.dumps([{"ifname": "lo", "flags": ["UP"]}])})
        empty = type("Result", (), {"stdout": "[]"})
        with patch.object(profiler.os, "readlink", return_value="net:[1]"), \
                patch.object(profiler.subprocess, "run", side_effect=[output, empty, empty]):
            with self.assertRaisesRegex(RuntimeError, "separate network"):
                profiler.check_network_isolation()

    def test_namespace_route_outside_loopback_is_rejected(self):
        output = type("Result", (), {"stdout": json.dumps([{"ifname": "lo", "flags": ["UP"]}])})
        route = type("Result", (), {"stdout": '[{"dev": "eth0", "dst": "default"}]'})
        empty = type("Result", (), {"stdout": "[]"})
        with patch.object(profiler.os, "readlink", side_effect=["net:[2]", "net:[1]"]), \
                patch.object(profiler.subprocess, "run", side_effect=[output, route, empty]):
            with self.assertRaisesRegex(RuntimeError, "outside loopback"):
                profiler.check_network_isolation()

    def test_immutable_manifest_refuses_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "manifest.json"
            profiler.write_json(target, {"first": True})
            with self.assertRaises(FileExistsError):
                profiler.write_json(target, {"second": True})
            self.assertEqual(json.loads(target.read_text()), {"first": True})

    def test_stop_failure_preserves_process_and_control_windows(self):
        server = SimpleNamespace(proc=SimpleNamespace(pid=123))
        control = Mock()
        control.eval.side_effect = [("started", []), ConnectionError("server closed connection")]
        with tempfile.TemporaryDirectory() as directory, \
                patch.object(profiler, "snapshot", return_value={"current_cycle": 1}), \
                patch.object(profiler, "process_state", return_value={"main_cpu_ns": 100}), \
                patch.object(profiler.time, "monotonic", side_effect=[0, 2]), \
                patch.object(profiler.time, "monotonic_ns", side_effect=[100, 110, 200, 230]):
            output = Path(directory)
            with self.assertRaisesRegex(ConnectionError, "server closed"):
                profiler.capture(control, server, output, 1, 100, "failed", "definitions")
            process = json.loads((output / "failed-process-window.json").read_text())
            self.assertEqual(process["before_stop_control"]["main_cpu_ns"], 100)
            timing = json.loads((output / "failed-control-window.json").read_text())
            self.assertEqual(timing["start_rpc_wall_ns"], 10)
            self.assertEqual(timing["stop_export_rpc_wall_ns"], 30)
            self.assertFalse((output / "failed-capture.json").exists())


if __name__ == "__main__":
    unittest.main()
