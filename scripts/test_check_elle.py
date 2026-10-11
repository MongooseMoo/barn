import unittest
from pathlib import Path

from check_elle import verdict


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
