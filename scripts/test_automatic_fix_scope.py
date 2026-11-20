from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from automatic_fix_scope import out_of_scope  # noqa: E402

SEED = ("A", "vm/testdata/fuzz/FuzzRestore/0123abcd", 0)


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


if __name__ == "__main__":
    unittest.main()
