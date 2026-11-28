"""Leak/incomplete evidence and confirmation tests; no server is launched."""

import hashlib
import json
import os
from pathlib import Path
import shlex
import tempfile
import subprocess
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import memory_check as mc
from fuzz_report import issue_body, memory_reports, saved_record


class MemoryEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)

    def test_native_profile_requires_explicit_leak_only_header(self):
        self.assertEqual(mc.leak_count("goroutineleak profile: total 0\n"), 0)
        self.assertEqual(mc.leak_count("goroutineleak profile: total 3\n1 @ stack\n"), 3)
        for text in ("", "goroutine profile: total 3\n", "Error: unavailable"):
            with self.assertRaises(ValueError):
                mc.leak_count(text)

    def test_checkpoint_runner_memory_preserves_sample_before_child_failure(self):
        membership = self.root / "membership"
        membership.write_text("0::/runner\n")
        group = self.root / "cgroup/runner"
        group.mkdir(parents=True)
        output = self.root / "runner-memory-001.log"
        (group / "memory.max").write_text("100\n")
        (group / "memory.events").write_text("oom_kill 3\n")
        (group / "memory.current").write_text("20\n")
        (group / "memory.stat").write_text("anon 10\n")

        def wait_for_child(*, timeout):
            self.assertEqual(timeout, 5)
            current = json.loads(output.read_text())["highest_sample"]["memory.current"]
            if current == 20:
                (group / "memory.current").write_text("95\n")
                (group / "memory.stat").write_text("anon 80\n")
                raise subprocess.TimeoutExpired("child", timeout)
            self.assertEqual(current, 95)
            (group / "memory.current").write_text("15\n")
            (group / "memory.stat").write_text("anon 5\n")
            (group / "memory.events").write_text("oom_kill 4\n")
            return -9

        with patch.object(mc.subprocess, "Popen") as launch:
            launch.return_value.__enter__.return_value.wait.side_effect = wait_for_child
            status = mc.observe_runner_memory(["child"], output, membership, group.parent)
        launch.assert_called_once_with(["child"])
        evidence = json.loads(output.read_text())
        self.assertEqual((status, evidence["exit_code"]), (137, 137))
        self.assertIn("shared runner cgroup", evidence["scope"])
        self.assertNotIn("pid", evidence)
        self.assertEqual(evidence["before"]["memory.events"], "oom_kill 3")
        self.assertEqual(evidence["highest_sample"]["memory.stat"], "anon 80")
        self.assertEqual(evidence["after"]["memory.events"], "oom_kill 4")
        self.assertEqual(evidence["after"]["memory.max"], "100")

    def test_checkpoint_runner_memory_reports_unavailable_without_changing_exit(self):
        output = self.root / "runner-memory-001.log"
        with patch.object(mc.subprocess, "Popen") as launch:
            launch.return_value.__enter__.return_value.wait.return_value = 1
            status = mc.observe_runner_memory(["child"], output, self.root / "missing", self.root)
        evidence = json.loads(output.read_text())
        self.assertEqual((status, evidence["exit_code"]), (1, 1))
        self.assertIn("cgroup_error", evidence)
        self.assertNotIn("memory.events", evidence["before"])

    def test_negative_correctness_exit_cannot_hide_memory_failures(self):
        for boundary in ("before", "after"):
            directory = self.root / "process-42" / boundary
            directory.mkdir(parents=True)
            (directory / "sample.json").write_text(json.dumps({"leaked_goroutines": int(boundary == "after")}))
        self.assertEqual(mc.test_process_findings(self.root), ["process-42/after: 1 native goroutine leaks"])
        (self.root / "process-42/after/sample.json").unlink()
        self.assertIn("incomplete memory evidence", mc.test_process_findings(self.root)[0])

    def sample(self, *, paused=False, count=0):
        directory = self.root / "gen-000/memory-1"
        directory.mkdir(parents=True)
        sample = {"pid": 42, "start_ticks": 123, "paused": paused,
                  "errors": [], "leaked_goroutines": None if paused else count}
        (directory / "sample.json").write_text(json.dumps(sample))
        if not paused:
            (directory / "goroutineleak.txt").write_text(f"goroutineleak profile: total {count}\n")
        return {"event": "memory", "generation": 0, "boundary": "admitted",
                "path": "gen-000/memory-1", "pid": 42, "start_ticks": 123}

    def test_jepsen_records_leaks_separately_from_incomplete(self):
        row = self.sample(count=2)
        result = mc.generation_findings(self.root, [row], True)
        self.assertEqual(len(result["leaks"]), 1)
        self.assertEqual(result["errors"], [])
        row["start_ticks"] = 124
        result = mc.generation_findings(self.root, [row], True)
        self.assertEqual(result["leaks"], [])
        self.assertIn("identity mismatch", result["errors"][0])

    def test_paused_generation_is_process_evidence_not_a_missing_profile(self):
        result = mc.generation_findings(self.root, [self.sample(paused=True)], True)
        self.assertEqual(result, {"captures": 1, "leaks": [], "errors": []})

    def test_enabled_capture_cannot_silently_disappear(self):
        result = mc.generation_findings(self.root, [{"event": "admit", "generation": 4}], True)
        self.assertIn("missing admitted memory capture", result["errors"][0])

    def test_profile_count_and_saved_sample_must_agree(self):
        row = self.sample(count=2)
        (self.root / row["path"] / "goroutineleak.txt").write_text("goroutineleak profile: total 0\n")
        self.assertIn("counts differ", mc.generation_findings(self.root, [row], True)["errors"][0])

    def test_escaping_artifact_path_is_invalid(self):
        row = self.sample()
        row["path"] = "../outside"
        self.assertIn("escapes", mc.generation_findings(self.root, [row], True)["errors"][0])

    def test_replay_timeout_is_incomplete_not_a_leak(self):
        with patch.object(mc.subprocess, "run", side_effect=subprocess.TimeoutExpired("replay", 1)):
            result = mc.replay_attempt(self.root / "test", self.root / "input", self.root / "attempt", 1)
        self.assertEqual(result["verdict"], "incomplete")

    def test_real_driver_replays_a_public_input_in_its_own_process(self):
        cache = self.root / "cache"
        target = "FuzzSourceExecution"
        corpus = mc.native_corpus(cache, "./vm", target)
        corpus.mkdir(parents=True)
        raw = (mc.ROOT / "scripts/fixtures/memory/source").read_bytes()
        (corpus / "public-input").write_bytes(raw)
        command = [os.sys.executable, str(mc.ROOT / "scripts/memory_check.py"), "--cache", str(cache),
                   "--target", target, "--output", str(self.root / "evidence"), "--seconds", "60"]
        run = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                             timeout=75, check=False, text=True)
        self.assertEqual(run.returncode, 0, run.stdout)
        result = json.loads((self.root / "evidence/results.json").read_text())
        self.assertEqual(result["verdict"], "healthy")
        for boundary in ("runner_before", "runner_after"):
            self.assertIn("not attributable", result[boundary]["runner_scope"])
            self.assertTrue((self.root / "evidence" / boundary.replace("_", "-")).is_dir())
        record = result["records"][0]
        self.assertEqual(Path(record["input_path"]).read_bytes(), raw)
        self.assertEqual(len(record["attempts"][0]["result"]["samples"]), 7)

    def test_report_requires_two_matching_replays_and_preserves_exact_record(self):
        record = self.root / "input"
        record.write_bytes(b'go test fuzz v1\nstring("return 1;")\n')
        attempt = {"verdict": "heap-candidate", "result": {"reasons": ["retained heap growth"],
                    "samples": [{"go_version": "go1.27.1"}]}}
        row = {"input_path": str(record), "sha256": hashlib.sha256(record.read_bytes()).hexdigest(),
               "verdict": "retained-growth", "attempts": [attempt, attempt]}
        report = self.root / "results.json"
        report.write_text(json.dumps({"verdict": "finding", "records": [row]}))
        finding = memory_reports(report, "FuzzSourceExecution", "test-revision")[0]
        self.assertEqual(finding["kind"], "retained growth")
        self.assertEqual(saved_record(finding), record.read_bytes())
        body = issue_body(finding, "./vm", "FuzzSourceExecution", "run", "artifact")
        self.assertIn("into vm/testdata/memory/FuzzSourceExecution/", body)
        self.assertNotIn("into " + str(record), body)
        if os.name == "posix":
            # Execute the issue's command: go test changes to the package directory.
            portable = mc.ROOT / finding["repro_input_path"]
            if portable.exists():
                self.assertEqual(portable.read_bytes(), record.read_bytes())
            else:
                portable.parent.mkdir(parents=True, exist_ok=True)
                portable.write_bytes(record.read_bytes())
                self.addCleanup(portable.unlink)
            evidence = self.root / "reported evidence"
            command = finding["repro"].replace("BARN_LEAK_OUTPUT=/tmp/barn-memory-replay",
                                               "BARN_LEAK_OUTPUT=" + shlex.quote(str(evidence)))
            run = subprocess.run(["sh", "-c", command], cwd=mc.ROOT, text=True,
                                 stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                 timeout=60, check=False)
            self.assertEqual(run.returncode, 0, run.stdout)
            self.assertIn("--- PASS: TestMemorySourceReplay", run.stdout)
            self.assertEqual(json.loads((evidence / "replay.json").read_text())["verdict"], "healthy")
        row["attempts"] = [attempt]
        report.write_text(json.dumps({"verdict": "finding", "records": [row]}))
        with self.assertRaises(ValueError):
            memory_reports(report, "FuzzSourceExecution", "test-revision")

    def test_driver_requires_completed_test_marker(self):
        source = self.root / "input"
        source.write_bytes(b"input")

        def run(command, **kwargs):
            out = Path(kwargs["env"]["BARN_LEAK_OUTPUT"])
            (out / "replay.json").write_text(json.dumps({"verdict": "healthy"}))
            kwargs["stdout"].write("PASS\n")
            return SimpleNamespace(returncode=0)

        with patch.object(mc.subprocess, "run", side_effect=run):
            result = mc.replay_attempt(self.root / "test", source, self.root / "attempt", 1)
        self.assertEqual(result["verdict"], "incomplete")

    def test_incomplete_replay_report_keeps_the_actual_failure(self):
        report = self.root / "results.json"
        report.write_text(json.dumps({"verdict": "incomplete", "records": [],
                                      "error": "replay build exited 1"}))
        finding = memory_reports(report, "FuzzSourceExecution", "test-revision")[0]
        self.assertEqual(finding["kind"], "infrastructure")
        self.assertEqual(finding["first_error"], "memory replay incomplete: replay build exited 1")


if __name__ == "__main__":
    unittest.main()
