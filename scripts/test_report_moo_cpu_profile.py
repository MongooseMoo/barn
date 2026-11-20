import copy
import unittest

from report_moo_cpu_profile import folded, markdown, validate


def profile():
    return {
        "schema_version": 1, "clock": "main-thread CPU", "sampling": "CPU timer",
        "sampling_valid": 1, "main_cpu_seconds": 1.0001, "period_ns": 1000000,
        "start_cpu_ns": 10000000, "stop_cpu_ns": 1010100000,
        "initial_phase_ns": 500000, "first_expiration_cpu_ns": 10500000, "expected_periods": 1000,
        "recorded": 900, "dropped": 50, "overruns": 20, "unobserved_periods": 30,
        "denominator_samples": 1000, "runtime_samples": 50, "transition_samples": 20,
        "profiler_samples": 20, "unknown_samples": 10, "moo_self_samples": 700,
        "builtin_self_samples": 100,
        "verbs": [{"definer": "#101", "verb": "work", "program_generation": 0,
                   "self_samples": 700, "inclusive_samples": 800,
                   "self_cpu_share": 0.7, "inclusive_cpu_share": 0.8}],
        "builtins": [{"function_id": 7, "builtin": "crypt", "self_samples": 100,
                      "inclusive_samples": 100, "self_cpu_share": 0.1, "inclusive_cpu_share": 0.1}],
        "frame_dictionary": [{"frame_id": 0, "definer": "#101", "verb": "work", "program_generation": 0}],
        "stack_dictionary": [{"state_id": 16, "frame_ids": [0], "builtin_id": -1, "samples": 700},
                             {"state_id": 17, "frame_ids": [0], "builtin_id": 7, "samples": 100}],
    }


class CpuReportTests(unittest.TestCase):
    def test_missing_ticks_stay_in_denominator_and_visible_report(self):
        text = markdown(validate(profile()))
        self.assertIn("70.00%", text)
        self.assertIn("80.00%", text)
        self.assertIn("Unknown or missing: 110/1000 (11.00%)", text)
        self.assertNotIn("77.78%", text)

    def test_denominator_omitting_pending_expirations_is_rejected(self):
        data = profile()
        data["denominator_samples"] -= data["unobserved_periods"]
        with self.assertRaisesRegex(ValueError, "denominator"):
            validate(data)

    def test_clock_reconciliation_catches_consistently_omitted_loss(self):
        data = profile()
        data['unobserved_periods'] = 0
        data['denominator_samples'] = 970
        for name in ('verbs', 'builtins'):
            for row in data[name]:
                row['self_cpu_share'] = row['self_samples'] / 970
                row['inclusive_cpu_share'] = row['inclusive_samples'] / 970
        with self.assertRaisesRegex(ValueError, 'clock-derived'):
            validate(data)

    def test_clock_duration_and_expiration_must_reconcile(self):
        for key in ('expected_periods', 'first_expiration_cpu_ns', 'stop_cpu_ns'):
            data = profile()
            data[key] += 1
            if key == 'stop_cpu_ns':
                data[key] += 10000
            with self.assertRaises(ValueError):
                validate(data)

    def test_bad_shares_cannot_hide_missing_evidence(self):
        data = profile()
        data["verbs"][0]["self_cpu_share"] = 700 / 900
        with self.assertRaisesRegex(ValueError, "complete denominator"):
            validate(data)

    def test_native_self_remains_distinct_and_folded_totals_include_every_period(self):
        data = validate(profile())
        text = folded(data)
        self.assertIn("#101:work [g0] 700", text)
        self.assertIn("#101:work [g0];builtin:crypt 100", text)
        self.assertIn("[unknown or missing] 110", text)
        self.assertEqual(sum(int(line.rsplit(" ", 1)[1]) for line in text.splitlines()), 1000)

    def test_incomplete_stack_weights_are_rejected(self):
        data = profile()
        data["stack_dictionary"][0]["samples"] -= 1
        with self.assertRaisesRegex(ValueError, "omits semantic"):
            folded(validate(data))

    def test_passive_or_invalid_capture_cannot_claim_cpu_shares(self):
        with self.assertRaisesRegex(ValueError, "not a native"):
            validate({"schema_version": 1, "clock": "wall", "observations": 100})
        data = profile()
        data["sampling_valid"] = 0
        with self.assertRaisesRegex(ValueError, "marked invalid"):
            validate(data)

    def test_fixture_wrapper_preserves_json_singleton_arrays(self):
        data = profile()
        self.assertEqual(validate({"profile": copy.deepcopy(data)})["verbs"], data["verbs"])

    def test_per_stack_caller_pcs_preserve_base_frame_identity(self):
        data = profile()
        data['stack_dictionary'][0]['caller_pcs'] = [12]
        data['stack_dictionary'][1]['caller_pcs'] = [28]
        self.assertIn('#101:work [g0] 700', folded(validate(data)))
        self.assertEqual(1, len(data['frame_dictionary']))

    def test_misaligned_or_invalid_caller_pcs_are_rejected(self):
        for pcs in ([], [True], [-1], [12, 28]):
            data = profile()
            data['stack_dictionary'][0]['caller_pcs'] = pcs
            with self.assertRaisesRegex(ValueError, 'caller PCs'):
                folded(validate(data))

    def test_definitions_mode_keeps_cpu_weights_and_declares_missing_context(self):
        data = profile()
        data.update(metadata_mode='definitions', context_attribution='unavailable',
                    identity_scope='verb definitions, aliases and caller PCs', tasks=[])
        text = markdown(validate(data))
        self.assertIn('Context attribution: unavailable', text)
        self.assertIn('70.00%', text)
        self.assertIn('#101:work [g0] 700', folded(data))
        data['frame_dictionary'][0]['receiver'] = '#123'
        with self.assertRaisesRegex(ValueError, 'misleading runtime context'):
            validate(data)

    def test_definitions_mode_cannot_claim_a_task_context(self):
        data = profile()
        data.update(metadata_mode='definitions', context_attribution='available', tasks=[])
        with self.assertRaisesRegex(ValueError, 'unavailable task context'):
            validate(data)

    def test_root_paths_count_each_sample_once_including_builtin_and_recursive_frames(self):
        data = profile()
        data['stack_dictionary'][0]['frame_ids'] = [0, 0]
        text = markdown(validate(data))
        self.assertIn('| #101:work | 0 | 80.00% | 800 |', text)
        self.assertNotIn('| #101:work | 0 | 150.00%', text)
        data['stack_dictionary'][1]['samples'] -= 1
        with self.assertRaisesRegex(ValueError, 'root paths omit'):
            markdown(validate(data))

    def test_source_line_view_preserves_full_cpu_denominator_and_unresolved_line(self):
        data = profile()
        data['lines'] = [dict(data['verbs'][0], line=18),
                         dict(data['verbs'][0], line=0, self_samples=5)]
        text = markdown(validate(data))
        self.assertIn('| #101:work | 0 | unreported | 18 | 70.00% | 700 |', text)
        self.assertIn('| #101:work | 0 | unreported | 0 | 0.50% | 5 |', text)
        self.assertIn('Line 0 means unresolved', text)

    def test_future_schema_cannot_silently_change_cpu_accounting(self):
        data = profile()
        data["schema_version"] = 2
        with self.assertRaisesRegex(ValueError, "unsupported"):
            validate(data)


if __name__ == "__main__":
    unittest.main()
