"""Real nightly logs, issue reporting, and the workflow's own campaign step."""

import base64
import hashlib
import http.server
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import textwrap
import threading
import unittest
from unittest.mock import mock_open, patch

from fuzz_report import DEFERRED, INLINE_LIMIT, RECORD_LIMIT, TAIL_LIMIT, publish, report, summarize


ROOT = Path(__file__).resolve().parents[1]
FIXTURES = Path(__file__).with_name("fixtures") / "fuzz"
TARGET = "FuzzRestoreVMSnapshotDoesNotPanic"

# Exact summary lines produced by the differential count-mismatch regression,
# using its existing real Barn `Error: internal error: boom` transcript record.
UNATTRIBUTED_DIFFERENTIAL_PANIC = '''# Barn vs Toast fuzz differential

- outcome: divergence; partial coverage: True
- programs loaded: 1; eligible: 1; completed comparisons: 0
- results: {}
- record errors: ["barn: 3 result records for 2 inputs; see barn_raw.txt"]
- recovered Barn panics: ["Error: internal error: boom"]
'''


class FuzzReportTests(unittest.TestCase):
    def test_raw_fixtures_are_not_tool_truncations(self):
        for fixture in FIXTURES.rglob("*.log"):
            text = fixture.read_text()
            self.assertNotIn("Warning: truncated output", text, str(fixture))
            if fixture.parent == FIXTURES / "native":
                self.assertTrue(text.startswith("go version go"), str(fixture))
                self.assertIn("\nfuzz: elapsed:", text, str(fixture))
            else:
                self.assertEqual(fixture.parent, FIXTURES)
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
                         "## divergence (16)\n- toast: `compile 0`\n- barn: `raised E_TYPE`\n"
                         "- toast binary: `/root/src/toaststunt/build-release/moo` sha256 "
                         "`72fb1cf96cb303647a8ee72808e7c1ff62a491ecf44f547e6757e71ba2402bde` "
                         "version `ToastStunt version 2.7.3_5`")
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

    def test_unattributed_differential_panic_is_a_finding_with_native_evidence(self):
        result = report(UNATTRIBUTED_DIFFERENTIAL_PANIC, "./scripts", "ToastDifferential", "commit")
        self.assertEqual(result["kind"], "panic")
        self.assertEqual(result["first_error"], "Error: internal error: boom")
        self.assertIn('recovered Barn panics: ["Error: internal error: boom"]', result["log_tail"])
        self.assertEqual(result["barn_frames"], [])
        self.assertIsNone(result["input_path"])
        self.assertIsNone(result["input_text"])
        self.assertIsNone(result["repro"])
        other = report(UNATTRIBUTED_DIFFERENTIAL_PANIC.replace("boom", "other native error"),
                       "./scripts", "ToastDifferential", "commit")
        self.assertNotEqual(result["signature"], other["signature"])

    def test_empty_or_malformed_panic_summary_does_not_invent_a_finding(self):
        for value in ('[]', 'not JSON', '{}', '["other error"]'):
            with self.subTest(value=value):
                log = "- recovered Barn panics: " + value
                self.assertEqual(report(log, "./scripts", "ToastDifferential", "commit")["kind"],
                                 "infrastructure")
        self.assertEqual(report(UNATTRIBUTED_DIFFERENTIAL_PANIC, "./scripts", "ToastDifferential",
                                "commit", "skipped")["kind"], "infrastructure")

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

    def test_summary_keeps_each_distinct_signature_once_with_its_count(self):
        # Synthetic night: the loop kept one log per failed campaign. A panic
        # was found, then a hang, then the same panic again from another input.
        panic = ("panic: runtime error: index out of range\n"
                 "github.com/MongooseMoo/barn/vm.(*VM).Resume(0x{})\n /src/vm.go:1 +0x12\n"
                 f"Failing input written to testdata/fuzz/{TARGET}/{{}}\n")
        hang = ("fuzzing process hung or terminated unexpectedly: exit status 2\n"
                f"Failing input written to testdata/fuzz/{TARGET}/bbbb\n")
        logs = [panic.format("1", "aaaa"), hang, panic.format("2", "cccc")]
        summary = summarize(logs, "./vm", TARGET, "commit")
        self.assertEqual([(r["kind"], r["count"]) for r in summary], [("panic", 2), ("hang", 1)])
        self.assertEqual(summary[0]["input_path"], f"vm/testdata/fuzz/{TARGET}/aaaa")
        self.assertEqual(summary[1]["input_path"], f"vm/testdata/fuzz/{TARGET}/bbbb")

    def test_summary_without_logs_reports_the_failed_job(self):
        for outcome in ["failure", "skipped"]:
            summary = summarize([], "./vm", TARGET, "commit", outcome)
            self.assertEqual([(r["kind"], r["count"]) for r in summary], [("infrastructure", 1)])

    def test_real_fixtures_summarize_like_single_reports(self):
        logs = [(FIXTURES / f"{run}.log").read_text() for run in (41882671144, 42201471629)]
        summary = summarize(logs, "./vm", TARGET, "commit")
        self.assertEqual([r["kind"] for r in summary], ["panic", "hang"])
        self.assertEqual(summary[0]["signature"], report(logs[0], "./vm", TARGET, "commit")["signature"])


def decoded_record(body):
    """The bytes a repair agent recreates from an issue body, and the sha256 it checks."""
    match = re.search(r"sha256 ([0-9a-f]{64})\. .*?```base64\n(.*?)\n```", body, re.S)
    return (base64.b64decode(match[2]), match[1]) if match else (None, None)


class FakeGitHub:
    """Records REST calls in order; returns what publish() reads back."""

    def __init__(self, issues=(), fail=()):
        self.issues, self.fail, self.calls = list(issues), set(fail), []

    def __call__(self, method, path, payload=None):
        self.calls.append((method, path, payload))
        if (method, path) in self.fail:
            raise RuntimeError(f"{method} {path}: HTTP 500")
        if method == "GET":
            return self.issues
        if path == "/issues":
            return {"number": 999, "body": payload["body"]}
        return None

    def names(self):
        return [path.split("?")[0] for _, path, _ in self.calls if not path.startswith("/issues?")]


class PublishTests(unittest.TestCase):
    def run_publish(self, reports, issues=(), **kwargs):
        github = FakeGitHub(issues)
        status = publish(reports, github, "./vm", "FuzzTarget", "https://run", **kwargs)
        return status, github

    def test_new_signature_dispatches_and_repeat_comments(self):
        result = summarize(["panic: failure"], "./vm", "FuzzTarget", "commit")
        _, github = self.run_publish(result, [{"number": 5, "body": "old signature"}])
        self.assertEqual(github.names(), ["/issues", "/actions/workflows/claude-fix.yml/dispatches"])
        self.assertEqual(github.calls[1][2]["labels"], ["claude-fix"])
        self.assertEqual(github.calls[2][2], {"ref": "master", "inputs": {"issue_number": "999"}})
        body = github.calls[1][2]["body"]
        _, github = self.run_publish(result, [{"number": 12, "body": body}])
        self.assertEqual(github.names(), ["/issues/12/comments"])

    def test_each_distinct_signature_is_reported_once(self):
        first = ("panic: first\ngithub.com/MongooseMoo/barn/vm.(*VM).Resume()\n /src/vm.go:1 +0x1\n"
                 "Failing input written to testdata/fuzz/FuzzTarget/aaaa\n")
        second = ("panic: second\ngithub.com/MongooseMoo/barn/vm.(*VM).Step()\n /src/vm.go:2 +0x1\n"
                  "Failing input written to testdata/fuzz/FuzzTarget/bbbb\n")
        result = summarize([first, second, first], "./vm", "FuzzTarget", "commit")
        _, github = self.run_publish(result)
        dispatch = "/actions/workflows/claude-fix.yml/dispatches"
        self.assertEqual(github.names(), ["/issues", dispatch, "/issues", dispatch])
        self.assertIn("Campaigns in this report that found it: 2", github.calls[1][2]["body"])
        self.assertIn("Campaigns in this report that found it: 1", github.calls[3][2]["body"])

    def test_infrastructure_never_dispatches_or_labels(self):
        _, github = self.run_publish(summarize(["signal: terminated"], "./vm", "FuzzTarget", "commit"))
        self.assertEqual(github.names(), ["/issues"])
        self.assertIn("infrastructure failure", github.calls[1][2]["title"])
        self.assertEqual(github.calls[1][2]["labels"], [])

    def test_unattributed_differential_panic_dispatches_with_native_evidence(self):
        result = summarize([UNATTRIBUTED_DIFFERENTIAL_PANIC], "./scripts", "ToastDifferential", "commit")
        github = FakeGitHub()
        publish(result, github, "./scripts", "ToastDifferential", "https://run")
        self.assertEqual(github.calls[1][2]["labels"], ["claude-fix"])
        self.assertIn("Error: internal error: boom", github.calls[1][2]["body"])

    def test_body_keeps_corpus_fences_inside_code_block(self):
        result = summarize(["panic: failure"], "./vm", "FuzzTarget", "commit")
        result[0].update(input_text="go test fuzz v1\n```\n@someone", input_path="input", repro="bounded command")
        _, github = self.run_publish(result)
        body = github.calls[1][2]["body"]
        self.assertIn("````text\ngo test fuzz v1\n```\n@someone\n````", body)
        self.assertIn("bounded command", body)

    def test_reporting_error_is_raised(self):
        github = FakeGitHub(fail={("POST", "/issues")})
        with self.assertRaisesRegex(RuntimeError, "HTTP 500"):
            publish(summarize(["panic: failure"], "./vm", "FuzzTarget", "commit"), github,
                    "./vm", "FuzzTarget", "https://run")


def step_script(workflow, name):
    """The run: script of the named step, with this test's matrix values."""
    step = workflow.split(f"name: {name}\n", 1)[1]
    lines = step.split("        run: |\n", 1)[1].splitlines()
    body = []
    for line in lines:
        if line.strip() and not line.startswith("          "):
            break
        body.append(line)
    return (textwrap.dedent("\n".join(body))
            .replace("${{ matrix.package }}", "./vm").replace("${{ matrix.target }}", "FuzzT"))


class CampaignStepTests(unittest.TestCase):
    """Runs fuzz.yml's own campaign and reporter steps with a fake go and GitHub API."""

    FAKE_GO = r"""#!/usr/bin/env bash
if [ "$1" = version ]; then echo "go version go1.27.1 linux/amd64"; exit 0; fi
n=$(( $(cat "$WORK/attempts" 2>/dev/null || echo 0) + 1 )); echo "$n" > "$WORK/attempts"
echo "attempt $n" >> "$WORK/sequence"
[ -n "${GITHUB_TOKEN:-}" ] && echo "token reached go test" >> "$WORK/sequence"
if [ "$n" = 1 ]; then
  mkdir -p vm/testdata/fuzz/FuzzT
  cp "$WORK/record" vm/testdata/fuzz/FuzzT/abcd
  echo "--- FAIL: FuzzT"
  echo "    x_test.go:1: boom"
  echo "    Failing input written to testdata/fuzz/FuzzT/abcd"
  exit 1
fi
echo "fuzz: elapsed: 1s, execs: 10 (10/sec), new interesting: 0 (total: 1)"
"""

    def setUp(self):
        self.bash = shutil.which("bash")
        if self.bash is None:
            self.skipTest("bash is required to run the workflow step")
        self.work = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.work, True)
        (self.work / "scripts").mkdir()
        for name in ("fuzz_report.py", "fuzz_retention.py"):
            shutil.copy(ROOT / "scripts" / name, self.work / "scripts" / name)
        (self.work / "vm").mkdir()
        bin_dir = self.work / "bin"
        bin_dir.mkdir()
        (bin_dir / "go").write_text(self.FAKE_GO, newline="\n")
        (bin_dir / "python3").write_text(f'#!/bin/sh\nexec "{Path(sys.executable).as_posix()}" "$@"\n',
                                         newline="\n")
        for tool in ("go", "python3"):
            (bin_dir / tool).chmod(0o755)
        self.api_calls, self.failing, self.created = [], set(), []
        test = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def reply(self, method):
                length = int(self.headers.get("Content-Length") or 0)
                payload = json.loads(self.rfile.read(length)) if length else None
                path = self.path.removeprefix("/repos/test/barn")
                with open(test.work / "sequence", "a") as sequence:
                    sequence.write(f"{method} {path.split('?')[0]}\n")
                test.api_calls.append((method, path, payload))
                if (method, path) in test.failing:
                    status, data = 500, {"message": "boom"}
                elif method == "GET":
                    status, data = 200, test.created
                elif path == "/issues":
                    test.created.append({"number": 7, "body": payload["body"]})
                    status, data = 201, {"number": 7}
                else:
                    status, data = 204, None
                self.send_response(status)
                self.end_headers()
                if data is not None:
                    self.wfile.write(json.dumps(data).encode())

            def do_GET(self):
                self.reply("GET")

            def do_POST(self):
                self.reply("POST")

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.addCleanup(self.server.server_close)
        self.addCleanup(self.server.shutdown)
        temp = self.work / "runner-temp"
        temp.mkdir()
        self.env = {**os.environ, "PATH": f"{bin_dir.as_posix()}{os.pathsep}{os.environ['PATH']}",
                    "WORK": self.work.as_posix(), "RUNNER_TEMP": temp.as_posix(),
                    "GITHUB_OUTPUT": (self.work / "output").as_posix(), "GOCACHE": (self.work / "cache").as_posix(),
                    "GITHUB_SHA": "commit", "GITHUB_TOKEN": "token", "GITHUB_REPOSITORY": "test/barn",
                    "GITHUB_API_URL": f"http://127.0.0.1:{self.server.server_port}",
                    "GITHUB_SERVER_URL": "https://github.com", "GITHUB_RUN_ID": "1", "GITHUB_RUN_ATTEMPT": "1",
                    "FUZZ_PACKAGE": "./vm", "FUZZ_TARGET": "FuzzT", "CAMPAIGN_OUTCOME": "failure",
                    "ARTIFACT_URL": "https://artifact"}
        workflow = (ROOT / ".github/workflows/fuzz.yml").read_text()
        self.campaign = step_script(workflow, "Fuzz ${{ matrix.target }}")
        self.reporter = step_script(workflow, "Open failure issues")

    def run_step(self, script):
        # Actions runs `shell: bash` as bash --noprofile --norc -eo pipefail.
        path = self.work / "step.sh"
        path.write_text(script, newline="\n")
        return subprocess.run([self.bash, "--noprofile", "--norc", "-eo", "pipefail", path.as_posix()],
                              cwd=self.work, env=self.env, text=True, capture_output=True, timeout=120)

    def sequence(self):
        return (self.work / "sequence").read_text().splitlines()

    def logs(self):
        return sorted(p.name for p in (self.work / "runner-temp/fuzz-logs").iterdir())

    def test_finding_is_reported_with_its_exact_record_before_the_next_attempt(self):
        record = b'go test fuzz v1\nstring("caf\xc3\xa9 ``` \\x00")\n'
        (self.work / "record").write_bytes(record)
        result = self.run_step(self.campaign)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        dispatch = "POST /actions/workflows/claude-fix.yml/dispatches"
        self.assertEqual(self.sequence(), ["attempt 1", "GET /issues", "POST /issues", dispatch, "attempt 2"])
        issue = self.api_calls[1][2]
        self.assertEqual(issue["labels"], ["claude-fix"])
        self.assertEqual(self.api_calls[2][2], {"ref": "master", "inputs": {"issue_number": "7"}})
        data, digest = decoded_record(issue["body"])
        self.assertEqual(data, record)
        self.assertEqual(digest, hashlib.sha256(record).hexdigest())
        self.assertIn("uploaded as artifact fuzz-failure-FuzzT when this job ends", issue["body"])
        self.assertEqual(self.logs(), ["reported-001.log", "success-002.log"])
        self.assertIn("cycle=finding", (self.work / "output").read_text())
        # The finding is kept for the artifact step, and nothing is reported twice.
        self.assertEqual((self.work / "vm/testdata/fuzz/FuzzT/abcd").read_bytes(), record)
        result = self.run_step(self.reporter)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(len(self.api_calls), 3)

    def test_failed_report_is_visible_and_retried_when_the_job_ends(self):
        (self.work / "record").write_bytes(b'go test fuzz v1\nstring("x")\n')
        self.failing = {("POST", "/issues")}
        result = self.run_step(self.campaign)
        self.assertEqual(result.returncode, 1)
        self.assertIn("::error::Reporting vm/testdata/fuzz/FuzzT/abcd failed", result.stdout)
        self.assertEqual(self.sequence(), ["attempt 1", "GET /issues", "POST /issues", "attempt 2"])
        self.assertEqual(self.logs(), ["run-001.log", "success-002.log"])
        self.failing = set()
        result = self.run_step(self.reporter)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual([f"{m} {p.split('?')[0]}" for m, p, _ in self.api_calls[2:]],
                         ["GET /issues", "POST /issues", "POST /actions/workflows/claude-fix.yml/dispatches"])
        self.assertIn("Failing inputs: https://artifact", self.api_calls[3][2]["body"])

    def test_record_too_large_for_an_issue_is_reported_with_its_artifact(self):
        (self.work / "record").write_bytes(b'go test fuzz v1\nstring("' + b"x" * RECORD_LIMIT + b'")\n')
        result = self.run_step(self.campaign)
        self.assertEqual(result.returncode, 1)
        self.assertIn("::notice::", result.stdout)
        self.assertEqual(self.sequence(), ["attempt 1", "attempt 2"])
        self.assertEqual(self.logs(), ["run-001.log", "success-002.log"])
        result = self.run_step(self.reporter)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        body = self.api_calls[1][2]["body"]
        self.assertIn("too large for an issue; use the failing-input artifact", body)
        self.assertEqual(decoded_record(body), (None, None))
        self.assertIn("Failing inputs: https://artifact", body)
        self.assertEqual(len(self.api_calls), 3)
        self.assertIn(f"{DEFERRED}) echo \"::notice::", self.campaign)


if __name__ == "__main__":
    unittest.main()
