import json
import unittest

from repair_provider import classify


class RepairProviderTests(unittest.TestCase):
    def test_claude_usage_limit_with_reset(self):
        transcript = json.dumps([{"type": "rate_limit_event",
                                  "rate_limit_info": {"status": "rejected", "resetsAt": 1795248000}}])
        self.assertEqual(classify(transcript, "claude"), ("usage-limit", "1795248000"))

    def test_generic_claude_failure_stays_unknown(self):
        transcript = '{"type":"result","subtype":"success","is_error":true,"total_cost_usd":0,"modelUsage":{}}'
        self.assertEqual(classify(transcript, "claude"), ("unknown", ""))

    def test_codex_error_ndjson(self):
        transcript = '{"type":"thread.started"}\n{"type":"error","message":"You have hit your usage limit"}'
        self.assertEqual(classify(transcript, "codex"), ("usage-limit", ""))

    def test_codex_cyber_policy_error_is_terminal(self):
        transcript = json.dumps({"type": "turn.failed", "error": {
            "message": "This content was flagged for possible cybersecurity risk.",
            "codex_error_info": "cyber_policy"}})
        self.assertEqual(classify(transcript, "codex"), ("policy-block", ""))

    def test_normal_model_output_cannot_set_provider_state(self):
        transcript = '{"type":"result","is_error":false,"result":"The usage limit was hit"}'
        self.assertEqual(classify(transcript, "claude"), ("unknown", ""))
