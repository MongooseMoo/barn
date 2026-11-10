import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import textwrap
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from fuzz_red_green import verdict  # noqa: E402

SCRIPT = Path(__file__).resolve().parent / "fuzz_red_green.py"


class VerdictTests(unittest.TestCase):
    def test_red_then_green_passes(self):
        self.assertEqual(verdict((1, "--- FAIL: FuzzT/bad"), (0, "ok")),
                         (True, "fails on the base (exit 1), passes with the change"))

    def test_hang_on_the_base_counts_as_red(self):
        self.assertTrue(verdict((124, ""), (0, "ok"))[0])

    def test_input_that_passes_on_the_base_fails(self):
        ok, reason = verdict((0, "ok"), (0, "ok"))
        self.assertFalse(ok)
        self.assertIn("passes on the base", reason)

    def test_input_that_fails_with_the_change_fails(self):
        self.assertFalse(verdict((1, "FAIL"), (1, "FAIL"))[0])

    def test_new_target_is_exempt_from_the_base(self):
        self.assertTrue(verdict((0, "testing: warning: no tests to run\nok"), (0, "ok"))[0])

    def test_base_that_does_not_build_cannot_show_red(self):
        self.assertFalse(verdict((1, "FAIL\tex/m [build failed]"), (0, "ok"))[0])


FUZZ = """package m

import "testing"

func FuzzT(f *testing.F) {{
	f.Fuzz(func(t *testing.T, s string) {{
		{body}
	}})
}}
"""


def seed(value):
    return f'go test fuzz v1\nstring("{value}")\n'


@unittest.skipUnless(sys.platform != "win32" and shutil.which("go") and shutil.which("timeout"),
                     "needs go and GNU timeout")
class RepositoryTests(unittest.TestCase):
    """Runs the script in a scratch repository with a real Go fuzz target."""

    def setUp(self):
        self.repo = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.repo, True)
        self.git("init", "-q")
        self.write("go.mod", "module ex/m\n\ngo 1.21\n")
        self.write("m_test.go", FUZZ.format(body='if s == "bad" { t.Fatal("bug") }'))
        self.base = self.commit("base")

    def git(self, *args):
        return subprocess.run(["git", "-c", "user.name=t", "-c", "user.email=t@t", *args], cwd=self.repo,
                              check=True, capture_output=True, text=True).stdout.strip()

    def write(self, path, text):
        (self.repo / path).parent.mkdir(parents=True, exist_ok=True)
        (self.repo / path).write_text(textwrap.dedent(text), newline="\n")

    def commit(self, message):
        self.git("add", "-A")
        self.git("commit", "-q", "-m", message)
        return self.git("rev-parse", "HEAD")

    def check(self):
        env = {**os.environ, "GOFLAGS": "-mod=mod"}
        return subprocess.run([sys.executable, str(SCRIPT), "--base", self.base], cwd=self.repo, env=env,
                              capture_output=True, text=True, timeout=300)

    def test_fix_with_its_failing_input_passes(self):
        self.write("m_test.go", FUZZ.format(body="_ = s"))
        self.write("testdata/fuzz/FuzzT/bad", seed("bad"))
        self.commit("fix")
        run = self.check()
        self.assertEqual(run.returncode, 0, run.stdout + run.stderr)
        self.assertIn("ok   testdata/fuzz/FuzzT/bad: fails on the base", run.stdout)

    def test_input_that_never_failed_is_rejected(self):
        self.write("testdata/fuzz/FuzzT/fine", seed("fine"))
        self.commit("claimed fix")
        run = self.check()
        self.assertEqual(run.returncode, 1, run.stdout + run.stderr)
        self.assertIn("passes on the base commit", run.stdout)

    def test_input_still_failing_with_the_change_is_rejected(self):
        self.write("testdata/fuzz/FuzzT/bad", seed("bad"))
        self.commit("input without a fix")
        run = self.check()
        self.assertEqual(run.returncode, 1, run.stdout + run.stderr)
        self.assertIn("fails with the change", run.stdout)

    def test_seed_for_a_new_target_is_not_compared_with_the_base(self):
        self.write("n_test.go", FUZZ.replace("FuzzT", "FuzzN").format(body="_ = s"))
        self.write("testdata/fuzz/FuzzN/first", seed("first"))
        self.commit("new target")
        run = self.check()
        self.assertEqual(run.returncode, 0, run.stdout + run.stderr)
        self.assertIn("new fuzz target", run.stdout)

    def test_no_added_inputs(self):
        self.write("README", "x\n")
        self.commit("docs")
        run = self.check()
        self.assertEqual(run.returncode, 0, run.stdout + run.stderr)
        self.assertIn("No fuzz inputs added.", run.stdout)


if __name__ == "__main__":
    unittest.main()
