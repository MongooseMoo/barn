"""Exact corpus handoff, provenance, storage bounds and native-work census."""

import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from fuzz_retention import quarantine, transfer


ROOT = Path(__file__).resolve().parents[1]
TARGET = "FuzzSourceExecution"
RECORD = b'go test fuzz v1\nstring("return 1;\\n")\n'


class RetentionTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        previous = Path.cwd()
        os.chdir(self.root)
        self.addCleanup(os.chdir, previous)
        self.retained = self.root / "retained"
        self.cache = self.root / "host-one-cache"
        self.native = self.cache / "fuzz/github.com/MongooseMoo/barn/vm" / TARGET
        self.native.mkdir(parents=True)
        self.lane = self.retained / "vm" / TARGET
        self.digest = hashlib.sha256(RECORD).hexdigest()

    def run_transfer(self, mode="capture", cache=None, logs=None, head="source-one", campaign=None):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            transfer(mode, self.retained, cache or self.cache, "./vm", TARGET,
                     head, "go version test", "profile-digest", logs, campaign)
        return json.loads(output.getvalue())

    def test_successful_interesting_and_failing_inputs_restore_on_another_host(self):
        (self.native / "native-name").write_bytes(RECORD)
        failing = Path("vm/testdata/fuzz") / TARGET
        failing.mkdir(parents=True)
        (failing / "go-finding-name").write_bytes(RECORD)
        summary = self.run_transfer()
        self.assertEqual(summary["retained_records"], 1)
        self.assertEqual(summary["per_source"], {"interesting": 1, "testdata": 1})
        manifest = json.loads((self.lane / "manifest.json").read_text())
        origins = manifest["records"][self.digest]
        self.assertEqual({entry["name"] for entry in origins}, {"native-name", "go-finding-name"})
        self.assertEqual({entry["profile_sha256"] for entry in origins}, {"profile-digest"})
        other = self.root / "host-two-cache"
        restored = self.run_transfer("restore", cache=other)
        self.assertEqual(restored["manifest_sha256"], summary["manifest_sha256"])
        self.assertEqual((other / "fuzz/github.com/MongooseMoo/barn/vm" / TARGET / self.digest).read_bytes(), RECORD)
        self.run_transfer(cache=other, head="source-two")
        updated = json.loads((self.lane / "manifest.json").read_text())
        self.assertEqual({origin["head"] for origin in updated["records"][self.digest]}, {"source-one", "source-two"})

    def test_corruption_fails_before_restoring_or_recapturing(self):
        (self.native / "input").write_bytes(RECORD)
        self.run_transfer()
        (self.lane / "inputs" / self.digest).write_bytes(b"corrupt")
        for mode in ("restore", "capture"):
            with self.subTest(mode=mode), self.assertRaisesRegex(ValueError, "digest mismatch"):
                self.run_transfer(mode)

    def test_baseline_finding_is_kept_without_replaying_its_cached_original(self):
        # Actual run 42692025227 found the same seed 75 times during baseline
        # replay. Go's saved input must be unminimized to identify the original.
        record = (ROOT / "db/format/testdata/fuzz/FuzzReadDatabase/6b8992e5372061cb").read_bytes()
        source = Path("vm/testdata/fuzz") / TARGET / "failed"
        source.parent.mkdir(parents=True)
        source.write_bytes(record)
        (self.native / "original").write_bytes(record)
        (self.native / "restored-sha256").write_bytes(record)
        (self.native / "other").write_bytes(RECORD)
        found = self.root / "found"
        quarantine(self.cache, "./vm", TARGET, source, found)
        self.assertEqual((found / "failed").read_bytes(), record)
        self.assertFalse(source.exists())
        self.assertEqual([p.name for p in self.native.iterdir()], ["other"])
        # The workflow's EXIT trap returns saved findings to testdata before
        # capture, so artifacts retain the finding even though replay excludes it.
        source.write_bytes((found / "failed").read_bytes())
        summary = self.run_transfer()
        self.assertEqual(summary["per_source"], {"interesting": 1, "testdata": 1})
        with self.assertRaisesRegex(ValueError, "repeated after quarantine"):
            quarantine(self.cache, "./vm", TARGET, source, found)
        self.assertEqual((found / "failed").read_bytes(), record)

    def test_quarantine_rejects_a_path_outside_the_target(self):
        source = self.root / "unrelated"
        source.write_bytes(RECORD)
        with self.assertRaisesRegex(ValueError, "this target"):
            quarantine(self.cache, "./vm", TARGET, source, self.root / "found")
        self.assertEqual(source.read_bytes(), RECORD)

    def test_native_census_sums_per_campaign_high_water_marks(self):
        logs = self.root / "logs"
        logs.mkdir()
        (logs / "run-001.log").write_text(
            "fuzz: elapsed: 3s, execs: 3 (1/sec), new interesting: 0 (total: 1)\n"
            "fuzz: elapsed: 6s, execs: 9 (2/sec), new interesting: 1 (total: 2)\n"
            "string(\"execs: 999999\")\n")
        (logs / "run-002.log").write_text(
            "fuzz: elapsed: 3s, execs: 4 (1/sec), new interesting: 0 (total: 2)\n")
        summary = self.run_transfer(logs=logs, campaign="finding")
        self.assertEqual(summary["completed_executions"], 13)
        self.assertEqual(summary["campaign"], "finding")
        self.assertEqual(json.loads((self.lane / "census.json").read_text()), summary)
        self.assertEqual((self.lane / "logs/run-001.log").read_bytes(), (logs / "run-001.log").read_bytes())
        # A campaign step that never finished leaves its output empty.
        self.assertIsNone(self.run_transfer(logs=logs, campaign="")["campaign"])

    def test_real_go_statistics_are_counted_without_summing_progress_lines(self):
        raw = (ROOT / "scripts/fixtures/fuzz/42201471629.log").read_text()
        logs = self.root / "logs"
        logs.mkdir()
        (logs / "run-001.log").write_text("\n".join(
            line.split("\t", 2)[-1].split(" ", 1)[-1] for line in raw.splitlines()))
        self.assertEqual(self.run_transfer(logs=logs)["completed_executions"], 6791511)

    def test_zero_measured_work_is_red_but_inputs_and_census_are_preserved(self):
        (self.native / "input").write_bytes(RECORD)
        logs = self.root / "logs"
        logs.mkdir()
        (logs / "run-001.log").write_text("setup failed before fuzzing\n")
        with self.assertRaisesRegex(ValueError, "zero measured"):
            self.run_transfer(logs=logs)
        self.assertEqual((self.lane / "inputs" / self.digest).read_bytes(), RECORD)
        self.assertEqual(json.loads((self.lane / "census.json").read_text())["completed_executions"], 0)

    def test_real_baseline_findings_do_not_masquerade_as_mutation_work(self):
        logs = self.root / "logs"
        logs.mkdir()
        raw = (ROOT / "scripts/fixtures/fuzz/42692025227-baseline.log").read_bytes()
        (logs / "run-001.log").write_bytes(raw)
        with self.assertRaisesRegex(ValueError, "zero measured"):
            self.run_transfer(logs=logs, campaign="finding")
        self.assertEqual((self.lane / "logs/run-001.log").read_bytes(), raw)
        census = json.loads((self.lane / "census.json").read_text())
        self.assertEqual(census["completed_executions"], 0)
        self.assertEqual(census["campaign"], "finding")

    def test_storage_exhaustion_does_not_delete_existing_inputs(self):
        (self.native / "input").write_bytes(RECORD)
        self.run_transfer()
        (self.native / "another").write_bytes(RECORD + b"\n")
        with patch("fuzz_retention.MAX_BYTES", len(RECORD)), self.assertRaisesRegex(ValueError, "exceed"):
            self.run_transfer()
        self.assertEqual((self.lane / "inputs" / self.digest).read_bytes(), RECORD)

    def test_restore_enforces_storage_bound_and_manifest_identity(self):
        (self.native / "input").write_bytes(RECORD)
        self.run_transfer()
        with patch("fuzz_retention.MAX_BYTES", 1), self.assertRaisesRegex(ValueError, "exceed"):
            self.run_transfer("restore")
        path = self.lane / "manifest.json"
        manifest = json.loads(path.read_text())
        manifest["target"] = "FuzzDifferent"
        path.write_text(json.dumps(manifest))
        with self.assertRaisesRegex(ValueError, "identity mismatch"):
            self.run_transfer("restore")

    def test_rejects_non_corpus_input_and_path_traversal(self):
        (self.native / "input").write_bytes(b"private arbitrary file")
        with self.assertRaisesRegex(ValueError, "not a Go fuzz record"):
            self.run_transfer()
        with self.assertRaisesRegex(ValueError, "relative Go package"):
            transfer("capture", self.retained, self.cache, "../vm", TARGET, "h", "g", "p")


if __name__ == "__main__":
    unittest.main()
