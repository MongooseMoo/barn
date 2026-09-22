"""Check benchmark acceptance rules, especially fast-but-wrong results."""
import unittest

from bench_mongoose_matrix import measure_case, parallel_case
from report_mongoose_matrix import report


class ControlStub:
    def __init__(self, result):
        self.result = result

    def eval(self, code, **kwargs):
        return self.result, []


class AcceptanceTests(unittest.TestCase):
    def test_successful_eval(self):
        result = measure_case(ControlStub([0.25, [1, 123]]), "return 123;", 123)
        self.assertEqual(result["value"], 123)
        self.assertEqual(result["server_s"], 0.25)

    def test_failed_or_wrong_eval_never_counts_as_speed(self):
        for response in ([0.01, [0, "E_QUOTA"]], [0.01, [1, 122]], "broken", None):
            with self.subTest(response=response), self.assertRaises(AssertionError):
                measure_case(ControlStub(response), "return 123;", 123)

    def test_lost_updates_rejected_even_if_requests_succeeded(self):
        class LostUpdate:
            def eval(self, code, **kwargs):
                if "counter=0" in code:
                    return 0, []
                if "for i" in code:
                    return 1, []
                return 1, []  # should be 2 clients * 3 operations * 5 increments

        with self.assertRaisesRegex(AssertionError, "committed counter mismatch"):
            parallel_case([LostUpdate(), LostUpdate()], [2, 3], 1, "hot_write", 3, 5)

    def test_invalid_server_clock_is_not_a_speedup(self):
        for duration in (-1, float("nan"), float("inf"), "zero"):
            with self.subTest(duration=duration), self.assertRaisesRegex(AssertionError, "invalid elapsed"):
                measure_case(ControlStub([duration, [1, 123]]), "return 123;", 123)


class ReportTests(unittest.TestCase):
    def test_unrelated_samples_from_an_errored_world_are_excluded(self):
        data = {"platform": "test", "artifacts": {}, "runs": [
            {"lane": "barn-g4", "round": 0, "errors": [{"case": "other", "error": "timeout"}],
             "parallel": [], "samples": [{"case": "x", "server_s": 0.001}]},
            {"lane": "barn-g4", "round": 1, "errors": [], "parallel": [],
             "samples": [{"case": "x", "server_s": 0.009}]},
        ]}
        self.assertIn("9.000 (9.000–9.000)", report(data))
        self.assertIn("Accepted worlds by lane: barn-g4=1", report(data))

    def test_a_failed_repeat_invalidates_its_speed_cell(self):
        data = {"platform": "test", "artifacts": {}, "runs": [{
            "lane": "barn-g4", "round": 0, "errors": [{"case": "map", "error": "wrong"}],
            "parallel": [], "samples": [
                {"case": "map", "server_s": 0.001},
                {"case": "map", "error": "wrong"},
            ],
        }]}
        self.assertIn("| map / server | INVALID |", report(data))

    def test_worlds_have_equal_weight_and_warmup_is_excluded(self):
        data = {"platform": "test", "artifacts": {}, "runs": []}
        for index, values in enumerate(([0.001] * 10, [0.009])):
            data["runs"].append({"lane": "toast", "round": index, "errors": [], "parallel": [],
                                 "samples": [{"case": "x", "server_s": 10, "warmup": True}] +
                                            [{"case": "x", "server_s": value} for value in values]})
        self.assertIn("5.000 (1.000–9.000)", report(data))


if __name__ == "__main__":
    unittest.main()
