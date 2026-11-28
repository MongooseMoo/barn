from pathlib import Path
import json
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from automatic_fix_scope import load_bound_evidence, out_of_scope  # noqa: E402

SEED = ("A", "vm/testdata/fuzz/FuzzRestore/0123abcd", 0)
FIX = ("M", "builtins/strings.go", 2)
ADMISSION = "test_capability_admission"
TEST = "test_yaml_conformance[strsub_case.yaml::strsub_preserves_case]"
GREEN = {"exit": 0, "passed": [ADMISSION, TEST], "failed": [], "skipped": []}
RED = {"exit": 1, "passed": [ADMISSION], "failed": [TEST], "skipped": []}


def evidence(toast=GREEN, base=RED, head=GREEN):
    """The record claude-fix.yml writes after running the conformance branch's added tests."""
    return {"files": ["src/moo_conformance/_tests/builtins/strsub_case.yaml"],
            "runs": {"toast": toast, "barn_base": base, "barn_head": head}}


class OutOfScopeTests(unittest.TestCase):
    def test_production_fix_with_its_fuzz_input_is_in_scope(self):
        self.assertEqual(out_of_scope([SEED, ("M", "vm/control.go", 3)]), [])

    def test_new_test_file_and_new_test_data_are_in_scope(self):
        self.assertEqual(out_of_scope([SEED, ("M", "vm/control.go", 0), ("A", "vm/control_unwind_test.go", 0),
                                       ("A", "db/format/testdata/fork.db", 0)]), [])

    def test_lines_added_to_an_existing_test_are_in_scope(self):
        # PR 758 added eight lines to vm/fork_source_lines_test.go and removed none.
        self.assertEqual(out_of_scope([SEED, ("M", "vm/fork_source_lines_test.go", 0)]), [])

    def test_fix_without_a_fuzz_input_is_out_of_scope(self):
        reasons = out_of_scope([("M", "vm/control.go", 1)])
        self.assertEqual(len(reasons), 1)
        self.assertIn("adds no fuzz input", reasons[0])

    def test_removing_lines_from_an_existing_test_is_out_of_scope(self):
        # PR 699 fixed a fuzz finding by changing the fuzz harness's comparison.
        self.assertEqual(out_of_scope([SEED, ("M", "vm/checkpoint_resume_test.go", 2)]),
                         ["removes 2 line(s) from Go test vm/checkpoint_resume_test.go"])

    def test_deleting_or_changing_test_data_is_out_of_scope(self):
        self.assertEqual(out_of_scope([SEED, ("D", "parser/testdata/fuzz/FuzzParse/old", 2),
                                       ("M", "db/format/testdata/a.db", 0)]),
                         ["deletes test data parser/testdata/fuzz/FuzzParse/old",
                          "changes test data db/format/testdata/a.db"])

    def test_checks_and_rules_are_out_of_scope_even_when_added(self):
        for path, what in ((".github/workflows/ci.yml", "workflow file"),
                           ("scripts/fuzz_red_green.py", "script"),
                           ("profiles/toast/stock-wsl-testdb.json", "runtime or oracle profile"),
                           ("CLAUDE.md", "agent rules file"),
                           ("vm/AGENTS.md", "agent rules file"),
                           ("tests/mongoose-conformance/root/x.yaml", "conformance regression"),
                           (".golangci.yml", "build or lint configuration")):
            with self.subTest(path=path):
                self.assertEqual(out_of_scope([SEED, ("A", path, 0)]), [f"adds {what} {path}"])

    def test_renamed_test_is_a_delete(self):
        # changes() runs git with --no-renames, so a rename arrives as D plus A.
        self.assertEqual(out_of_scope([SEED, ("D", "vm/a_test.go", 9), ("A", "vm/b_test.go", 0)]),
                         ["deletes Go test vm/a_test.go"])


class ConformanceEvidenceTests(unittest.TestCase):
    NO_PROOF = "adds no fuzz input under testdata/fuzz, so nothing proves it fixes a reproduced failure"

    def test_fuzz_input_alone_is_in_scope(self):
        self.assertEqual(out_of_scope([SEED, FIX]), [])

    def test_conformance_red_green_is_in_scope(self):
        self.assertEqual(out_of_scope([FIX], evidence()), [])

    def test_base_passing_is_out_of_scope(self):
        self.assertEqual(out_of_scope([FIX], evidence(base=GREEN)), [self.NO_PROOF])

    def test_toast_failing_is_out_of_scope(self):
        self.assertEqual(out_of_scope([FIX], evidence(toast=RED)), [self.NO_PROOF])

    def test_head_failing_is_out_of_scope(self):
        self.assertEqual(out_of_scope([FIX], evidence(head=RED)), [self.NO_PROOF])

    def test_no_evidence_and_no_input_is_out_of_scope(self):
        self.assertEqual(out_of_scope([FIX]), [self.NO_PROOF])

    def test_base_that_never_admitted_is_not_red(self):
        # A Barn that cannot start fails admission too; that is not the test failing.
        broken = {"exit": 1, "passed": [], "failed": [ADMISSION, TEST], "skipped": []}
        self.assertEqual(out_of_scope([FIX], evidence(base=broken)), [self.NO_PROOF])

    def test_harness_error_is_not_a_barn_base_test_failure(self):
        broken = dict(RED, errors=["test_fixture"])
        self.assertEqual(out_of_scope([FIX], evidence(base=broken)), [self.NO_PROOF])

    def test_evidence_is_bound_to_both_barn_commits_and_the_suite_head(self):
        proof = dict(evidence(), base="barn-base", head="barn-head", suite="suite-head")
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "evidence.json"
            path.write_text(json.dumps(proof), encoding="utf-8")
            self.assertEqual(load_bound_evidence(path, "barn-base", "barn-head", "suite-head"), proof)
            for refs in (("old-base", "barn-head", "suite-head"),
                         ("barn-base", "old-head", "suite-head"),
                         ("barn-base", "barn-head", "old-suite"),
                         ("barn-base", "barn-head", None)):
                with self.subTest(refs=refs):
                    self.assertIsNone(load_bound_evidence(path, *refs))

    def test_runs_must_select_the_same_non_empty_tests(self):
        other = {"exit": 0, "passed": [ADMISSION, TEST + "x"], "failed": [], "skipped": []}
        self.assertEqual(out_of_scope([FIX], evidence(head=other)), [self.NO_PROOF])
        only_admission = {"exit": 0, "passed": [ADMISSION], "failed": [], "skipped": []}
        self.assertEqual(out_of_scope([FIX], evidence(only_admission, only_admission, only_admission)),
                         [self.NO_PROOF])

    def test_evidence_does_not_excuse_touching_a_check(self):
        self.assertEqual(out_of_scope([FIX, ("M", "profiles/barn/outbound-on.conf", 1)], evidence()),
                         ["changes runtime or oracle profile profiles/barn/outbound-on.conf"])


if __name__ == "__main__":
    unittest.main()
