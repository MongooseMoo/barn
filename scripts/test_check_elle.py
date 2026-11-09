import re
import unittest
from pathlib import Path

from check_elle import FAULTS, lifecycle_verdict, verdict


class FaultListTests(unittest.TestCase):
    def test_driver_runs_every_fault_the_workload_knows(self):
        source = (Path(__file__).resolve().parent.parent / "engine" /
                  "elle_workload_test.go").read_text(encoding="utf-8")
        declared = re.search(r"^var elleFaults = \[\]string\{([^}]*)\}", source, re.MULTILINE)
        self.assertIsNotNone(declared)
        self.assertEqual(re.findall(r'"([^"]+)"', declared.group(1)), FAULTS)


class LifecycleVerdictTests(unittest.TestCase):
    def test_requires_the_specific_completed_failure(self):
        witness = "parent observed {194, 0}, want {194, 1}; dispatched while handler held=1\n"
        completed = ("--- FAIL: TestUncaughtHandlerBeforeYieldedParent (0.01s)\n"
                     "FAIL\tgithub.com/MongooseMoo/barn/engine\t0.01s\n")
        output = witness + completed
        self.assertEqual(lifecycle_verdict(1, output, "finished-task-index"), "rejected")
        for code, invalid in ((0, output), (2, output), (1, completed),
                              (1, witness), (1, output.replace("--- FAIL:", "--- PASS:")),
                              (1, output.replace("FAIL\tgithub.com", "panic:\tgithub.com"))):
            with self.assertRaises(ValueError):
                lifecycle_verdict(code, invalid, "finished-task-index")

    def test_healthy_requires_the_test_to_run_and_finish(self):
        output = ("--- PASS: TestUncaughtHandlerBeforeYieldedParent (0.01s)\n"
                  "ok\tgithub.com/MongooseMoo/barn/engine\t0.01s\n")
        self.assertEqual(lifecycle_verdict(0, output, ""), "passed")
        for invalid in ("", output.replace("PASS:", "SKIP:"), "ok [no tests to run]\n"):
            with self.assertRaises(ValueError):
                lifecycle_verdict(0, invalid, "")


class VerdictTests(unittest.TestCase):
    def test_explicit_verdicts(self):
        history = Path("/tmp/evidence/list-append.edn")
        for value in ("true", "false", "unknown"):
            self.assertEqual(verdict(f"INFO ignored\n{history} \t {value}\n", history), value)

    def test_does_not_accept_unrelated_true(self):
        history = Path("/tmp/evidence/list-append.edn")
        for output in ("true", "/other.edn true\n", f"ERROR {history} true\n",
                       f"{history} true\n{history} false\n", f"{history} maybe\n"):
            with self.assertRaises(ValueError):
                verdict(output, history)


if __name__ == "__main__":
    unittest.main()
