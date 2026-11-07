"""Real nightly logs and offline execution of the issue/dispatch script."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import textwrap
import unittest
from unittest.mock import mock_open, patch

from fuzz_report import INLINE_LIMIT, TAIL_LIMIT, report


ROOT = Path(__file__).resolve().parents[1]
FIXTURES = Path(__file__).with_name("fixtures") / "fuzz"
TARGET = "FuzzRestoreVMSnapshotDoesNotPanic"


class FuzzReportTests(unittest.TestCase):
    def test_raw_fixtures_are_not_tool_truncations(self):
        for fixture in FIXTURES.glob("*.log"):
            text = fixture.read_text()
            self.assertNotIn("Warning: truncated output", text, str(fixture))
            self.assertTrue(text.startswith("Fuzz"), str(fixture))

    def real_report(self, run, target=TARGET):
        return report((FIXTURES / f"{run}.log").read_text(), "./vm", target, "test-commit")

    def test_real_panic_and_corpus(self):
        result = self.real_report(41882671144)
        self.assertEqual(result["kind"], "panic")
        self.assertIn("invalid memory address", result["first_error"])
        self.assertEqual(result["barn_frames"][0], {
            "function": "github.com/MongooseMoo/barn/db/store.(*StoreTxn).live",
            "location": "db/store/store_txn.go:260"})
        self.assertEqual(result["input_text"], (ROOT / result["input_path"]).read_text())
        self.assertEqual(result["repro"],
                         f"timeout --kill-after=5s 60s go test ./vm -run '{TARGET}/3fd49d1954a707ba'")

    def test_real_hang_wording(self):
        for run, name in [(42201471629, "1aa755cc0c129f9b"),
                          (42009421620, "f10bcff0e9a20440"),
                          (41680879951, "9b691aff745e0426")]:
            with self.subTest(run=run):
                result = self.real_report(run)
                self.assertEqual(result["kind"], "hang")
                self.assertIn("hung or terminated unexpectedly: exit status 2", result["first_error"])
                self.assertTrue(result["input_path"].endswith(name))
                self.assertEqual(result["barn_frames"], [])

    def test_real_sibling_signal_is_infrastructure(self):
        result = self.real_report(42009421620, "FuzzSourceExecution")
        self.assertEqual(result["kind"], "infrastructure")
        self.assertEqual(result["first_error"], "signal: terminated")
        self.assertIsNone(result["repro"])

    def test_failed_check_and_metadata(self):
        result = report("go version go1.27.1 linux/amd64\n--- FAIL: FuzzCheck\n"
                        "    check_test.go:42: got 1, want 2\n", "./vm", "FuzzCheck", "commit")
        self.assertEqual(result["kind"], "failed check")
        self.assertEqual(result["first_error"], "check_test.go:42: got 1, want 2")
        self.assertEqual(result["commit"], "commit")
        self.assertEqual(result["go_version"], "go version go1.27.1 linux/amd64")

    def test_real_toast_differential_divergence(self):
        # Real output of scripts/fuzz_differential.py over the runner's fuzz
        # cache, run inside WSL Debian (2026-11-06).
        log = (FIXTURES / "toast-differential-2026-11-06.txt").read_text()
        result = report(log, "./scripts", "ToastDifferential", "commit")
        self.assertEqual(result["kind"], "divergence")
        self.assertEqual(result["first_error"],
                         "## divergence (16)\n- toast: `compile 0`\n- barn: `raised E_TYPE`")
        self.assertEqual(result["input_path"], "0ede3170ad3ee915")
        self.assertEqual(result["input_text"], "{\"\"/@`A!ANY=>{}'};")
        self.assertIsNone(result["repro"])
        self.assertEqual(result["go_version"], "go version go1.27.1 linux/amd64")
        # Program text in the report never reclassifies it, and the same
        # first divergence keeps the same signature.
        self.assertEqual(report(log + "\npanic: in a program\n", "./scripts", "ToastDifferential",
                                "other")["signature"], result["signature"])
        # A run that compared nothing (exit 2) has no divergence section.
        self.assertEqual(report(log.split("## divergence")[0], "./scripts", "ToastDifferential",
                                "commit")["kind"], "infrastructure")

    def test_saved_crash_signal_is_a_finding(self):
        # Synthetic: installed Go 1.27.1 src/internal/fuzz/worker.go:186
        # formats "fuzzing process hung or terminated unexpectedly: %v".
        # sys_posix.go:116-128 identifies SIGSEGV as a crash signal.
        log = ("fuzzing process hung or terminated unexpectedly: signal: segmentation fault\n"
               f"Failing input written to testdata/fuzz/{TARGET}/synthetic-crash\n")
        result = report(log, "./vm", TARGET, "commit")
        self.assertEqual(result["kind"], "crash signal")
        self.assertIn("signal: segmentation fault", result["first_error"])
        self.assertEqual(result["input_path"], f"vm/testdata/fuzz/{TARGET}/synthetic-crash")
        self.assertEqual(result["repro"],
                         f"timeout --kill-after=5s 60s go test ./vm -run '{TARGET}/synthetic-crash'")

    def test_non_crash_signal_without_saved_input_is_infrastructure(self):
        # Synthetic: installed Go 1.27.1 src/internal/fuzz/worker.go:170-176
        # formats "fuzzing process terminated by unexpected signal; no crash will be recorded: %v".
        log = "fuzzing process terminated by unexpected signal; no crash will be recorded: signal: killed\n"
        result = report(log, "./vm", TARGET, "commit")
        self.assertEqual(result["kind"], "infrastructure")
        self.assertEqual(result["first_error"], log.strip())
        self.assertIsNone(result["input_path"])
        self.assertIsNone(result["repro"])

    def test_saved_input_overrides_infrastructure_markers(self):
        for marker in ["signal: terminated", "FAIL ./vm [setup failed]", "FAIL ./vm [build failed]", ""]:
            for outcome in ["failure", "skipped"]:
                with self.subTest(marker=marker, outcome=outcome):
                    log = marker + f"\nFailing input written to testdata/fuzz/{TARGET}/saved\n"
                    self.assertNotEqual(report(log, "./vm", TARGET, "commit", outcome)["kind"],
                                        "infrastructure")

    def test_setup_and_signals(self):
        for log in ["", "FAIL ./vm [setup failed]", "vm.go:42: undefined: missing\nFAIL ./vm [build failed]", "signal: killed",
                    "fuzzing process terminated by unexpected signal; no crash will be recorded: signal: terminated"]:
            self.assertEqual(report(log, "./vm", TARGET, "commit")["kind"], "infrastructure")
        self.assertEqual(report("panic: tool setup", "./vm", TARGET, "commit", "skipped")["kind"],
                         "infrastructure")

    def test_caps_and_missing_input(self):
        log = f"Failing input written to testdata/fuzz/{TARGET}/missing\n" + "x" * 20000
        result = report(log, "./vm", TARGET, "commit")
        self.assertIn("Input unavailable:", result["input_text"])
        self.assertLessEqual(len(result["log_tail"]), TAIL_LIMIT)
        with patch("pathlib.Path.open", mock_open(read_data=b"x" * (INLINE_LIMIT + 1))):
            self.assertTrue(report(log, "./vm", TARGET, "commit")["input_text"].endswith("[truncated]"))

    def test_signature_ignores_addresses_but_distinguishes_frames_and_kinds(self):
        def summary(function, address="0x123", kind="panic: error"):
            return report(f"{kind}\n{function}({address})\n /src/vm.go:1 +0x123", "./vm", TARGET, "c")
        frame = "github.com/MongooseMoo/barn/vm.(*VM).Resume"
        a = summary(frame)
        self.assertEqual(a["signature"], summary(frame, "0x456")["signature"])
        self.assertNotEqual(a["signature"], summary(frame + "Other")["signature"])
        self.assertNotEqual(a["signature"], summary(frame, kind="signal: terminated")["signature"])


class IssueScriptTests(unittest.TestCase):
    def execute(self, result, issues=(), checkout=True):
        workflow = (ROOT / ".github/workflows/fuzz.yml").read_text()
        script = textwrap.dedent(workflow.split("          script: |\n", 1)[1])
        harness = r"""
const fs = require('node:fs');
const vm = require('node:vm');
const input = JSON.parse(fs.readFileSync(0, 'utf8'));
const calls = [];
const record = name => async args => {calls.push({name, ...args}); return {data: {number: 999}};};
const github = {
  paginate: async () => input.issues,
  rest: {issues: {listForRepo: {}, create: record('create'), createComment: record('comment')},
         actions: {createWorkflowDispatch: record('dispatch')}}
};
const context = {repo: {owner: 'test', repo: 'barn'}, sha: 'commit', serverUrl: 'https://github.com', runId: 1};
const mockedRequire = name => name === 'node:fs' ? {existsSync: () => input.checkout}
  : name === 'node:child_process' ? {execFileSync: () => JSON.stringify(input.result)} : require(name);
vm.runInNewContext('(async () => {' + input.script + '})()', {
  require: mockedRequire, github, context,
  process: {env: {FUZZ_TARGET: 'FuzzTarget', FUZZ_PACKAGE: './vm', CAMPAIGN_OUTCOME: 'failure', GITHUB_RUN_ATTEMPT: '1'}}
}).then(() => process.stdout.write(JSON.stringify(calls))).catch(e => {console.error(e); process.exit(1);});
"""
        node = shutil.which("node")
        if node is None:
            # Self-hosted Actions runs JS actions with its bundled Node, without
            # adding it to PATH for shell steps.
            node = str(Path(os.environ["RUNNER_TEMP"]).parents[1] / "externals/node24/bin/node")
        completed = subprocess.run([node, "-e", harness], input=json.dumps({
            "script": script, "result": result, "issues": list(issues), "checkout": checkout}),
            text=True, capture_output=True, check=True, cwd=ROOT)
        return json.loads(completed.stdout)

    def test_new_signature_on_same_target_dispatches_and_repeat_comments(self):
        result = report("panic: failure", "./vm", "FuzzTarget", "commit")
        calls = self.execute(result, [{"title": "Nightly fuzz failure: FuzzTarget", "body": "old signature"}])
        self.assertEqual([c["name"] for c in calls], ["create", "dispatch"])
        self.assertEqual(calls[1]["inputs"], {"issue_number": "999"})
        calls = self.execute(result, [{"number": 12, "body": calls[0]["body"]}])
        self.assertEqual([c["name"] for c in calls], ["comment"])
        self.assertEqual(calls[0]["issue_number"], 12)

    def test_infrastructure_never_dispatches_or_labels(self):
        for checkout in [True, False]:
            calls = self.execute(report("signal: terminated", "./vm", "FuzzTarget", "commit"), checkout=checkout)
            self.assertEqual([c["name"] for c in calls], ["create"])
            self.assertIn("infrastructure failure", calls[0]["title"])
            self.assertEqual(calls[0]["labels"], [])

    def test_saved_crash_signal_dispatches_repair(self):
        log = ("fuzzing process hung or terminated unexpectedly: signal: segmentation fault\n"
               "Failing input written to testdata/fuzz/FuzzTarget/synthetic-crash\n")
        calls = self.execute(report(log, "./vm", "FuzzTarget", "commit"))
        self.assertEqual([c["name"] for c in calls], ["create", "dispatch"])
        self.assertEqual(calls[0]["labels"], ["claude-fix"])

    def test_body_keeps_corpus_fences_inside_code_block(self):
        result = report("panic: failure", "./vm", "FuzzTarget", "commit")
        result.update(input_text="go test fuzz v1\n```\n@someone", input_path="input", repro="bounded command")
        body = self.execute(result)[0]["body"]
        self.assertIn("````text\ngo test fuzz v1\n```\n@someone\n````", body)
        self.assertIn("bounded command", body)


if __name__ == "__main__":
    unittest.main()
