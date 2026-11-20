"""Credential-bearing setup failures retain phases/types, never error payloads."""
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

import mongoose_scenario_support as support


class ActorCredentialFailureTests(unittest.TestCase):
    secret = "deadbeefcafe0123456789abcdef012345"
    partial = secret[:12]

    def sensitive_error(self):
        return support.bench.ControlEvalError(
            'eval produced no result: account_source("' + self.partial,
            ["raw native output containing " + self.secret])

    def captured(self, *, fail_login=False, close_failure=False):
        original = Mock()
        original.eval.return_value = ([1, 1, 0], [])
        control = SimpleNamespace(control=original, eval=Mock(return_value=([1, 0], [])))
        connection = Mock()
        if close_failure:
            connection.close.side_effect = RuntimeError(self.partial)
        identifiers = [SimpleNamespace(hex="b" * 32), SimpleNamespace(hex=self.secret)]
        with tempfile.TemporaryDirectory() as temporary, \
                patch.object(support.uuid, "uuid4", side_effect=identifiers), \
                patch.object(support.bench, "create_account") as account, \
                patch.object(support.bench, "LineConn", return_value=connection), \
                patch.object(support.bench, "mongoose_login") as login:
            error = self.sensitive_error()
            if fail_login:
                login.side_effect = error
            else:
                account.side_effect = error
            options = SimpleNamespace(host="127.0.0.1", port=17778, run_dir=Path(temporary))
            with self.assertRaises(support.ScenarioFailure) as caught:
                support.connect_actor(control, 123, options)
            return caught.exception, connection, login

    def assert_sanitized(self, error):
        retained = json.dumps({"failure": str(error), "evidence": error.evidence})
        self.assertNotIn(self.secret, retained)
        self.assertNotIn(self.partial, retained)
        self.assertNotIn("account_source", retained)
        self.assertNotIn("raw native output", retained)
        self.assertEqual(error.evidence["native_error_type"], "ControlEvalError")
        self.assertIn("ControlEvalError", str(error))
        self.assertFalse(hasattr(error, "native_output"))

    def test_clipped_account_password_prefix_is_never_retained(self):
        error, connection, login = self.captured()
        self.assert_sanitized(error)
        self.assertEqual(error.evidence["stage"], "account_creation")
        connection.close.assert_not_called()
        login.assert_not_called()

    def test_login_failure_retains_type_and_stage_without_credential_output(self):
        error, connection, _ = self.captured(fail_login=True)
        self.assert_sanitized(error)
        self.assertEqual(error.evidence["stage"], "login")
        connection.close.assert_called_once()

    def test_cleanup_failure_keeps_original_native_error_type_without_its_payload(self):
        error, connection, _ = self.captured(fail_login=True, close_failure=True)
        self.assert_sanitized(error)
        self.assertEqual(error.evidence["native_error_type"], "ControlEvalError")
        self.assertEqual(error.evidence["cleanup_error_type"], "RuntimeError")
        connection.close.assert_called_once()


if __name__ == "__main__":
    unittest.main()
