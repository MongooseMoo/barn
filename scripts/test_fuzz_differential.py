import unittest

from fuzz_differential import decode_corpus_file, exclusion, outcome, probe, results

# Excerpts of real lane output from the first local run (2026-11-06).
TOAST_STDOUT = """
LambdaMOO Emergency Holographic Wizard Mode
-------------------------------------------
"Please state the nature of the wizardly emergency..."
(Type `help' for assistance.)

** Now running emergency commands as #3 ...

=> 0
=> {"F00001", "value", {0, {"Line 1:  syntax error"}}}
#3 <- VERB:
=> {"F00002", "raised", E_VARNF}
=> *Aborted*
Bye.  (saving database)

"""

BARN_STDOUT = """=> 0
=> {"F00001", "value", {1, {0}}}
=> {"F00002", "value", {1, "a
b"}}
Error: internal error: boom
-- skipped
"""


class CorpusFileTests(unittest.TestCase):
    def test_go_quoted_string(self):
        data = b'go test fuzz v1\nstring("return \\"a\\\\b\\";\\n\\treturn 1;")\n'
        self.assertEqual(decode_corpus_file(data), 'return "a\\b";\n\treturn 1;')

    def test_non_ascii_escape_is_decoded_then_excluded(self):
        source = decode_corpus_file(b'go test fuzz v1\nstring("A0000000\\xd0()")\n')
        self.assertEqual(exclusion(source), "non-ASCII or control character")

    def test_other_files_are_not_programs(self):
        self.assertIsNone(decode_corpus_file(b"go test fuzz v1\n[]byte(\"x\")\n"))
        self.assertIsNone(decode_corpus_file(b"not a corpus file\n"))


class ExclusionTests(unittest.TestCase):
    def test_nondeterministic_calls_and_words(self):
        self.assertEqual(exclusion("return ticks_left();"), "ticks_left(): tick budget")
        self.assertEqual(exclusion("fork (0) endfork"), "fork")
        self.assertEqual(exclusion("$server_options.fg_ticks = 1;"), "server_options")

    def test_plain_program_and_lookalike_names(self):
        self.assertIsNone(exclusion("x = {0..0}; return x[1];"))
        self.assertIsNone(exclusion("return times;"))


class ResultTests(unittest.TestCase):
    def test_toast_records_skip_banner_notify_and_bye(self):
        self.assertEqual(results(TOAST_STDOUT), [
            "=> 0",
            '=> {"F00001", "value", {0, {"Line 1:  syntax error"}}}',
            '=> {"F00002", "raised", E_VARNF}',
            "=> *Aborted*",
        ])

    def test_barn_record_spans_a_printed_newline(self):
        got = results(BARN_STDOUT)
        self.assertEqual(len(got), 4)
        self.assertEqual(outcome("F00002", got[2]), 'value {1, "a\nb"}')
        self.assertEqual(got[3], "Error: internal error: boom")

    def test_outcome_checks_the_key(self):
        record = '=> {"F00001", "value", {1, {0}}}'
        self.assertEqual(outcome("F00001", record), "value {1, {0}}")
        self.assertIsNone(outcome("F00002", record))
        self.assertIsNone(outcome("F00001", "=> *Aborted*"))

    def test_probe_passes_each_line_to_eval(self):
        self.assertEqual(
            probe("F00007", 'return "a\nb";'),
            'try r = eval("return \\"a", "b\\";"); return {"F00007", r[1] ? "value" | "compile", '
            'r[1] ? r[2] | 0}; except e (ANY) return {"F00007", "raised", e[1]}; endtry')


if __name__ == "__main__":
    unittest.main()
