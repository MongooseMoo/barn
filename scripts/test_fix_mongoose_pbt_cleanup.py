import importlib.util
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location(
    "pbt_cleanup", Path(__file__).with_name("fix-mongoose-pbt-cleanup.py"))
repair_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(repair_module)


class RepairTests(unittest.TestCase):
    def fixture(self, newline=b"\n"):
        before = b'database prefix\n#21247:152\n"mongoose-pbt-cleanup";\n'
        after = b'"PBT generated object was not recycled";\n.\nuntouched suffix\n'
        return tuple(part.replace(b"\n", newline)
                     for part in (before, repair_module.WAIT, after))

    def test_preserves_every_other_byte(self):
        for newline in (b"\n", b"\r\n"):
            with self.subTest(newline=newline):
                before, old, after = self.fixture(newline)
                fixed, verb = repair_module.repair(before + old + after)
                self.assertEqual(verb, "#21247:152")
                self.assertEqual(fixed, before + repair_module.DIRECT.replace(b"\n", newline) + after)
                with self.assertRaises(ValueError):
                    repair_module.repair(fixed)

    def test_rejects_changed_or_duplicate_block(self):
        data = b"".join(self.fixture())
        for invalid in (data.replace(b"drains < 300", b"drains < 301"), data + data):
            with self.assertRaises(ValueError):
                repair_module.repair(invalid)

    def test_rejects_block_outside_expected_verb(self):
        data = b"".join(self.fixture())
        for invalid in (data.replace(b"#21247:152", b"not-a-verb"),
                        data.replace(b"mongoose-pbt-cleanup", b"other-helper")):
            with self.assertRaises(ValueError):
                repair_module.repair(invalid)


if __name__ == "__main__":
    unittest.main()
