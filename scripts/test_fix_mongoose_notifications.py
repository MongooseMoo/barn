import importlib.util
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("fix-mongoose-notifications.py")
spec = importlib.util.spec_from_file_location("notifications", SCRIPT)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class NotificationMigrationTests(unittest.TestCase):
    def fixture(self, newline=b"\n"):
        before = b'database prefix\n#1:38\n'
        body = b'"Historical comment.";\n' + module.OLD_BODY + b'\n"Modification date.";'
        after = b'\n.\n#2:0\nreturn 9;\n.\nuntouched suffix\x00'
        return tuple(part.replace(b"\n", newline) for part in (before, body, after))

    def test_preserves_every_other_byte_and_line_endings(self):
        for newline in (b"\n", b"\r\n"):
            with self.subTest(newline=newline):
                before, old, after = self.fixture(newline)
                fixed = module.repair(before + old + after)
                source = module.SOURCE.read_bytes().replace(b"\r\n", b"\n").rstrip(b"\n")
                self.assertEqual(fixed, before + source.replace(b"\n", newline) + after)
                with self.assertRaises(ValueError):
                    module.repair(fixed)

    def test_rejects_unknown_duplicate_or_incomplete_program(self):
        data = b"".join(self.fixture())
        for bad in (data.replace(b"level + 1", b"level + 2"), data + data,
                    data.replace(b"#1:38", b"#2:38"), data.replace(b"\n.\n", b"\nx\n"),
                    data.replace(b"endfor", b"endfor\nraise(E_DIV);")):
            with self.subTest(data=bad), self.assertRaises(ValueError):
                module.repair(bad)

    def test_cli_keeps_input_and_refuses_existing_output(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "input.db"
            output = Path(directory) / "output.db"
            data = b"".join(self.fixture())
            source.write_bytes(data)
            result = subprocess.run([sys.executable, str(SCRIPT), str(source), str(output)],
                                    capture_output=True, check=True)
            self.assertIn(b'"verb": "#1:38"', result.stdout)
            self.assertEqual(source.read_bytes(), data)
            fixed = output.read_bytes()
            second = subprocess.run([sys.executable, str(SCRIPT), str(source), str(output)],
                                    capture_output=True)
            self.assertNotEqual(second.returncode, 0)
            self.assertEqual(output.read_bytes(), fixed)

    def test_conformance_compiles_the_delivered_source(self):
        suite = module.SOURCE.parents[2] / "tests/mongoose-conformance/root/empty_notifications.yaml"
        text = suite.read_text()
        block = text.split("      - &generic_notification\n", 1)[1].split("        expect:\n", 1)[0]
        code = block.split("          code: |\n", 1)[1]
        self.assertEqual("\n".join(line[12:] for line in code.splitlines()) + "\n",
                         module.SOURCE.read_text())
        self.assertIn('args: ["this", "none", "this"]', block)


if __name__ == "__main__":
    unittest.main()
