import json
from pathlib import Path
from tempfile import TemporaryDirectory
import unittest
import xml.etree.ElementTree as ET

from conformance_failure import HARNESS, failed_cases, report_failure
from conformance_watch import git, pending_revisions, submit
from repair_authority import ACTIONS_BOT, WORKER_BOT


class ConformanceWatchTests(unittest.TestCase):
    def test_catches_up_in_first_parent_order(self):
        with TemporaryDirectory() as directory:
            git(directory, "init", "--quiet", "-b", "main")

            def commit(message):
                git(directory, "-c", "user.name=Test", "-c", "user.email=test@example.invalid",
                    "-c", "commit.gpgSign=false", "commit", "--allow-empty", "-m", message)
                return git(directory, "rev-parse", "HEAD")

            first = commit("first")
            second = commit("second")
            third = commit("third")
            self.assertEqual(pending_revisions(directory, None, third), [third])
            self.assertEqual(pending_revisions(directory, first, third), [second, third])
            self.assertEqual(pending_revisions(directory, third, third), [])
            git(directory, "switch", "--quiet", "--detach", first)
            rewrite = commit("rewrite")
            with self.assertRaisesRegex(ValueError, "no longer advances"):
                pending_revisions(directory, second, rewrite)

    def test_advances_only_after_acknowledgement_and_reuses_run(self):
        first = "a" * 40
        second = "b" * 40
        requests = []
        runs = [{"id": 12, "actor": {"id": WORKER_BOT}, "display_title": f"Conformance suite {first}"}]

        def api(path, payload=None):
            if payload is None:
                return {"workflow_runs": runs}
            requests.append(payload["inputs"]["suite_sha"])
            if len(requests) == 1:
                return None
            return {"workflow_run_id": 13}

        with TemporaryDirectory() as directory:
            cursor = Path(directory, "last-dispatch")
            with self.assertRaisesRegex(RuntimeError, "did not return a run ID"):
                submit([first, second], cursor, api)
            self.assertEqual(cursor.read_text().strip(), first)
            submit([second], cursor, api)
            self.assertEqual(cursor.read_text().strip(), second)
        self.assertEqual(requests, [second, second])


class ConformanceFailureTests(unittest.TestCase):
    def evidence(self, directory, outcome=None, *, admission="passed", include_packaged=True):
        context = "barn-upstream:12:1:barn:suite"
        data = {"context": context, "schema_version": 2, "phase": "admission", "probes": [{"status": admission}]}
        (directory / "conformance-admission.json").write_text(json.dumps(data))
        root = ET.Element("testsuite")
        test = ET.SubElement(root, "testcase", classname=HARNESS, name="test_capability_admission")
        ET.SubElement(ET.SubElement(test, "properties"), "property", name="admission_context", value=context)
        if include_packaged:
            case = ET.SubElement(root, "testcase", classname=HARNESS, name="test_yaml_conformance[vm/test.yaml::case]")
            if outcome:
                ET.SubElement(case, outcome)
        ET.ElementTree(root).write(directory / "conformance-results.xml")
        return context

    def test_passing_and_confirmed_failure_evidence(self):
        with TemporaryDirectory() as directory:
            path = Path(directory)
            self.assertEqual(failed_cases(path, self.evidence(path), 0), [])
            self.assertEqual(failed_cases(path, self.evidence(path, "failure"), 1), [("vm/test.yaml", "case")])

    def test_does_not_call_missing_admission_setup_errors_or_no_tests_a_regression(self):
        with TemporaryDirectory() as directory:
            path = Path(directory)
            with self.assertRaises(FileNotFoundError):
                failed_cases(path, "missing", 1)
            for settings in [{"admission": "failed"}, {"outcome": "error"}, {"include_packaged": False}]:
                with self.subTest(settings=settings), self.assertRaises(ValueError):
                    failed_cases(path, self.evidence(path, **settings), 1)

    def test_retry_uses_existing_issue_and_dispatch(self):
        issues = []
        runs = []
        attempts = []

        def api(path, payload=None):
            if "/issues?" in path:
                return issues
            if path.endswith("/issues"):
                issue = {**payload, "state": "open", "user": {"type": "Bot", "id": ACTIONS_BOT},
                         "number": 123, "html_url": "https://github.com/MongooseMoo/barn/issues/123"}
                issues.append(issue)
                return issue
            if "/runs?" in path:
                return {"workflow_runs": runs}
            attempts.append(payload)
            if len(attempts) == 1:
                return None
            runs.append({"id": 456, "display_title": "Repair #123 / auto", "actor": {"id": ACTIONS_BOT}})
            return {"workflow_run_id": 456}

        args = (api, "a" * 40, "b" * 40, [("vm/test.yaml", "case")], "https://github.com/MongooseMoo/barn/actions/runs/12")
        with self.assertRaisesRegex(RuntimeError, "did not return a run ID"):
            report_failure(*args)
        report_failure(*args)
        report_failure(*args)
        self.assertEqual(len(issues), 1)
        self.assertEqual(issues[0]["labels"], ["claude-fix"])
        self.assertEqual(len(attempts), 2)
        self.assertEqual(attempts[-1]["inputs"], {"issue_number": "123"})


if __name__ == "__main__":
    unittest.main()
