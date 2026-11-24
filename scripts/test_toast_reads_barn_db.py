import json
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import fuzz_report
from fuzz_differential import MAX_LINE, results
from toast_reads_barn_db import (BARN_FORM, DATA_READ_MARKER, failure_line, REPAIR_RE, RECORD_RE, SEEDS, TOAST_TASKS_RE, barn_form_tasks,
                                 divergence_section, first_difference, go_formats, load_worlds,
                                 object_count, parse_literal, public_reason, same_record, walk_inputs)

# Excerpt of the toast lane of a real local run (seed action-0, 2026-11-20).
TOAST_STDOUT = """
LambdaMOO Emergency Holographic Wizard Mode
-------------------------------------------
"Please state the nature of the wizardly emergency..."
(Type `help' for assistance.)

** Now running emergency commands as #3 ...

=> 0
=> {"M0", #10}
=> {"O0", "System Object", #3, {0, 0, 0, 1, 0, 0}, {#1}, #-1, {}}
Bye.  (saving database)

TOAST TASKS 1 queued tasks
TOAST TASKS 1 suspended tasks
TOAST TASKS 0 interrupted tasks
"""


class WalkTest(unittest.TestCase):
    def test_every_input_fits_one_emergency_line(self):
        lines = walk_inputs(99999)
        self.assertEqual(len(lines), 2 + 3 * 100000)
        self.assertLessEqual(max(len(";;" + line) for line in lines), MAX_LINE)

    def test_walk_lifts_the_world_value_size_limits(self):
        # Mongoose #42804's property record is 82 MB: under the world's own
        # max_list_value_bytes Toast aborted and Barn raised E_MAXREC.
        first = walk_inputs(0)[0]
        for name in ("max_string_concat", "max_list_value_bytes", "max_map_value_bytes"):
            self.assertIn(f'"{name}"', first)
            self.assertIn(f"$server_options.{name} = 0;", first)
        self.assertTrue(first.endswith("return load_server_options();"))

    def test_inputs_name_each_object_and_kind(self):
        lines = walk_inputs(1)
        self.assertEqual(lines[1], 'return {"M0", max_object()};')
        for line, key in zip(lines[2:], ["O0", "P0", "V0", "O1", "P1", "V1"]):
            self.assertIn(f'o = #{key[1:]};', line)
            self.assertIn(f'return {{"{key}", ', line)
            # An error in one engine is a printed result, not a lost record.
            self.assertIn(f'except e (ANY) return {{"{key}", "raised", e[1], e[2]}}; endtry', line)

    def test_toast_records_and_task_counts_parse(self):
        records = results(TOAST_STDOUT)
        self.assertEqual(records[1], '=> {"M0", #10}')
        self.assertEqual(RECORD_RE.fullmatch(records[2]).groups()[:2], ("O", "0"))
        self.assertEqual(TOAST_TASKS_RE.findall(TOAST_STDOUT),
                         [("1", "queued"), ("1", "suspended"), ("0", "interrupted")])

    def test_repair_lines_exclude_phase_banners(self):
        self.assertIsNone(REPAIR_RE.search("Nov 20 22:45:12: VALIDATE: Phase 1: Check for invalid objects ..."))
        self.assertIsNone(REPAIR_RE.search("Nov 20 22:45:12: VALIDATE: Check for self-referential waifs ..."))
        self.assertIsNone(REPAIR_RE.search("Nov 20 23:01:02: VALIDATE: Done through #10000 ..."))
        self.assertIsNotNone(REPAIR_RE.search("VALIDATE: #3 not in it's location's (#2) contents."))


class RecordTest(unittest.TestCase):
    # A P record of the walk: {definer, name, info, clear, toliteral(value)}.
    @staticmethod
    def p_record(literal: str) -> str:
        quoted = literal.replace("\\", "\\\\").replace('"', '\\"')
        return '=> {"P4", {{#4, "m", {#3, "r"}, 0, "' + quoted + '"}}}'

    def test_waif_keys_in_any_order_are_the_same(self):
        a, b = "[[class = #5, owner = #3]]", "[[class = #9, owner = #3]]"
        toast = self.p_record(f'[1 -> 2, "k" -> {{1.5}}, {a} -> "x", {b} -> ["s" -> 1]]')
        barn = self.p_record(f'[1 -> 2, "k" -> {{1.5}}, {b} -> ["s" -> 1], {a} -> "x"]')
        self.assertNotEqual(toast, barn)
        self.assertTrue(same_record(toast, barn))
        anon = self.p_record('[*anonymous* -> 1, *anonymous* -> 2]')
        self.assertTrue(same_record(anon, self.p_record('[*anonymous* -> 2, *anonymous* -> 1]')))

    def test_other_differences_stay(self):
        a, b = "[[class = #5, owner = #3]]", "[[class = #9, owner = #3]]"
        # Ordinary keys keep their order; a waif's value still counts.
        self.assertFalse(same_record(self.p_record('[1 -> 2, 3 -> 4]'), self.p_record('[3 -> 4, 1 -> 2]')))
        self.assertFalse(same_record(self.p_record(f'[{a} -> 1, {b} -> 2]'),
                                     self.p_record(f'[{a} -> 2, {b} -> 1]')))
        # Not a P record, or not parseable: compared as text.
        self.assertFalse(same_record('=> {"O4", "x"}', '=> {"O4", "y"}'))
        self.assertFalse(same_record(self.p_record("[" + a + " -> 1"), self.p_record("[" + a + " -> 1 ")))
        self.assertFalse(same_record('=> {"P4", "raised", E_PROPNF, "x"}', self.p_record("1")))

    def test_parse_literal_reads_toliteral_spellings(self):
        value = parse_literal('{1, -2, 1.5e+30, "a\\"b", #-1, E_PERM, *anonymous*, true, [], {}, '
                              '[[class = #1, owner = #3]]}')
        self.assertEqual(value[0], "list")
        self.assertEqual(len(value[1]), 11)
        self.assertEqual(value[1][10], ("waif", ("atom", "#1"), ("atom", "#3")))


class FailureTest(unittest.TestCase):
    SCRIPT = Path(__file__).resolve().parent / "toast_reads_barn_db.py"

    def test_failure_line_hides_the_message_only_after_data_is_read(self):
        error = FileNotFoundError(2, "No such file", "/tmp/x/barn.db.new")
        self.assertEqual(failure_line(error, False),
                         "- harness failure before the database was read: FileNotFoundError: "
                         "[Errno 2] No such file: '/tmp/x/barn.db.new'")
        line = failure_line(KeyError("a player's property"), True)
        self.assertEqual(line, "- harness failure after the engines began reading the database: "
                               "KeyError (message not shown)")
        self.assertNotIn("player", line)

    def test_missing_inputs_print_fixed_reasons_on_stdout(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "out"
            proc = subprocess.run([sys.executable, str(self.SCRIPT), "database", str(Path(tmp) / "none.db"),
                                   "--toast", str(Path(tmp) / "moo"), "--barn-linux", str(Path(tmp) / "barn"),
                                   "--out", str(out)], capture_output=True, text=True)
            self.assertEqual(proc.returncode, 2)
            self.assertIn(f"- harness failure before the database was read: input database "
                          f"{Path(tmp) / 'none.db'} is not a file", proc.stdout)
            self.assertIn(f"--barn-linux {Path(tmp) / 'barn'} is not a file", proc.stdout)
            self.assertFalse((out / DATA_READ_MARKER).exists())

    def test_workflow_shows_error_output_only_before_the_marker(self):
        workflow = (self.SCRIPT.parent.parent / ".github/workflows/toast-reads-mongoose-checkpoint.yml").read_text(
            encoding="utf-8")
        self.assertIn(f'if [ -e "$RUNNER_TEMP/trbd/out/{DATA_READ_MARKER}" ]; then', workflow)
        self.assertIn('--out "$RUNNER_TEMP/trbd/out"', workflow)


class FileTest(unittest.TestCase):
    def test_object_count_follows_headers_in_order(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "x.db"
            # "#7" and "#2" in a task's strings before the objects do not count;
            # a recycled slot does.
            path.write_bytes(b"header\n#7\n#0\n#0\nname\n# 1 recycled\n#2\n#0:0\n#3\r\n#5\n")
            self.assertEqual(object_count(path), 3)

    def test_object_count_includes_anonymous_batches(self):
        # write_db_file writes the permanent objects ("2", #0, #1), then each
        # batch of anonymous objects numbered on from them ("1", #2), then "0".
        # The Mongoose dump's Barn checkpoint has such a batch of 2072.
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "x.db"
            path.write_bytes(b"2\n#0\nname\n#1\nname\n1\n#2\nname\n0\n0\n")
            self.assertEqual(object_count(path), 2)

    def test_barn_form_tasks_read_from_the_json_log(self):
        with tempfile.TemporaryDirectory() as tmp:
            log = Path(tmp) / "latest.jsonl"
            log.write_text("\n".join([
                json.dumps({"level": "WARN", "msg": "#0 not in it's parent's (#1) children"}),
                "not json",
                json.dumps({"level": "WARN", "msg": BARN_FORM, "task_id": 2, "reason": "frame 0 (#0:): x"}),
            ]), encoding="utf-8")
            self.assertEqual(barn_form_tasks(Path(tmp)), [{"task_id": 2, "reason": "frame 0 (#0:): x"}])

    def test_public_reason_shows_only_go_format_strings(self):
        formats = go_formats()
        reason = ("frame 0 (#123:secret_verb): which variables Toast holds unbound at the call "
                  "depends on the path into its block")
        shown = public_reason(reason, formats)
        self.assertNotIn("123", shown)
        self.assertNotIn("secret_verb", shown)
        self.assertTrue(shown.endswith("depends on the path into its block"), shown)
        self.assertEqual(public_reason("player Alice's own words", formats),
                         "(not a fixed string in the Go source; not shown)")

    def test_first_difference_starts_near_it(self):
        toast, barn = first_difference("x" * 100 + "toast", "x" * 100 + "barn!")
        self.assertTrue(toast.startswith("...") and toast.endswith("toast"))
        self.assertTrue(barn.endswith("barn!"))


class ReportTest(unittest.TestCase):
    def test_fuzz_report_reads_a_failed_world_as_a_divergence(self):
        case = {"name": "W00001", "origin": "seed:action-0", "choices": "007f0000",
                "source": "ancestor = create(#2);\nreturn 1;\n",
                "file": {"size": 4409, "sha256": "ab" * 32},
                "findings": [{"kind": "walk", "records": [{"key": "O0", "kind": "O",
                                                           "toast": '=> {"O0", "Planted Object"}',
                                                           "barn": '=> {"O0", "System Object"}'}]}]}
        text = "\n".join(["# Toast reads Barn-written databases", ""] + divergence_section([case]))
        result = fuzz_report.report(text, "./scripts", "ToastReadsBarnDatabase", "c0ffee")
        self.assertEqual(result["kind"], "divergence")
        self.assertEqual(result["input_path"], "seed:action-0")
        self.assertIn("ancestor = create(#2);", result["input_text"])
        self.assertIn("Planted", result["first_error"])

    def test_each_kind_of_finding_reports(self):
        base = {"name": "W00001", "origin": "o", "source": "return 1;", "file": {"size": 1, "sha256": "0"},
                "tasks": {"barn wrote": [1, 1], "toast read": [1, 0]}}
        for finding in ({"kind": "barn-form task", "tasks": [{"task_id": 2, "reason": "r"}]},
                        {"kind": "toast refused the file", "exit": 1, "log": ["*** DB_LOAD: Cannot load database!"]},
                        {"kind": "toast repaired the file", "log": ["VALIDATE: x"]},
                        {"kind": "task counts", "counts": base["tasks"]}):
            text = "\n".join(divergence_section([{**base, "findings": [finding]}]))
            self.assertEqual(fuzz_report.report(text, "./scripts", "T", "c")["kind"], "divergence", finding)


class SeedTest(unittest.TestCase):
    def test_seeds_render_as_toast_worlds(self):
        worlds = load_worlds([])
        self.assertEqual([w["origin"] for w in worlds], [f"seed:{p.name}" for p in sorted(SEEDS.iterdir())])
        for world in worlds:
            self.assertIn("fork (3600)", world["source"], world["origin"])
            self.assertTrue(world["source"].rstrip().endswith("suspend();\nreturn observations;"))
        text = next(w for w in worlds if w["origin"] == "seed:text-utf8")["source"]
        # One to four UTF-8 bytes per character, in a value, a name and verb code.
        self.assertIn('child.text = "ü€😀é";', text)
        self.assertIn('ancestor.name = "ü€😀é";', text)
        self.assertTrue(re.search(r'set_verb_code\(child, "read_value", .*ü€😀é', text))


if __name__ == "__main__":
    unittest.main()
