import unittest
from unittest.mock import patch

from repair_completion import BARN, SUITE, finish_companion, required_check_state


REQUIRED = f"repos/{SUITE}/branches/main/protection/required_status_checks"
ENDPOINT = f"repos/{SUITE}/pulls/301"
CHECKS = f"repos/{SUITE}/commits/refreshed/check-runs?per_page=100&filter=latest"
ALLOWED = {"filename": "src/moo_conformance/_tests/builtins/example.yaml", "status": "added"}


def check(conclusion, app=15368):
    return {"name": "Full suite", "app": {"id": app}, "conclusion": conclusion,
            "html_url": "https://github.com/MongooseMoo/moo-conformance-tests/runs/42"}


class CheckStateTest(unittest.TestCase):
    def test_pending_check_is_not_a_rejection(self):
        responses = {REQUIRED: {"checks": [{"context": "Full suite", "app_id": 15368}]},
                     CHECKS: {"check_runs": [check(None), check("failure", app=99)]}}
        self.assertEqual(required_check_state(responses.__getitem__, SUITE, "main", "refreshed"),
                         ([], True))

    def test_failure_on_exact_head_carries_the_check_link(self):
        failed = check("failure")
        responses = {REQUIRED: {"checks": [{"context": "Full suite", "app_id": 15368}]},
                     CHECKS: {"check_runs": [failed]}}
        self.assertEqual(required_check_state(responses.__getitem__, SUITE, "main", "refreshed"),
                         ([failed["html_url"]], True))


class CompanionTest(unittest.TestCase):
    def test_companion_does_not_land_before_barn(self):
        def api(path, *flags):
            self.assertEqual(path, f"repos/{BARN}/pulls/1215")
            return {"state": "open", "merged": False}

        with self.assertRaisesRegex(ValueError, "Barn #1215 is still open"):
            finish_companion(api, 1215)

    def test_already_green_companion_requires_a_protection_bound_credential(self):
        for admin in (False, True, None):
            with self.subTest(admin=admin):
                companion = {"number": 301, "changed_files": 1, "state": "open",
                             "merged": False, "mergeable_state": "clean",
                             "head": {"sha": "refreshed", "repo": {"full_name": SUITE}}}
                merges = []

                def api(path, *flags):
                    if path == f"{ENDPOINT}/merge":
                        merges.append(flags)
                        return {"merged": True, "sha": "landed"}
                    return {
                        f"repos/{BARN}/pulls/1219": {"state": "closed", "merged": True},
                        f"repos/{SUITE}/pulls?state=open&head=MongooseMoo:barn-fix/pr-1219&base=main": [companion],
                        ENDPOINT: companion,
                        f"{ENDPOINT}/files?per_page=100": [ALLOWED],
                        f"repos/{SUITE}/git/ref/heads/main": {"object": {"sha": "main"}},
                        f"repos/{SUITE}/compare/main...refreshed": {"behind_by": 0},
                        REQUIRED: {"checks": [{"context": "Full suite", "app_id": 15368}]},
                        CHECKS: {"check_runs": [check("success")]},
                        f"repos/{SUITE}/branches/main/protection/enforce_admins": {"enabled": False},
                        f"repos/{SUITE}": {"permissions": {"admin": admin}},
                    }[path]

                if admin is False:
                    self.assertEqual(finish_companion(api, 1219), "Suite #301 merged at landed.")
                    self.assertEqual(merges, [("-X", "PUT", "-f", "merge_method=merge", "-f", "sha=refreshed")])
                else:
                    with self.assertRaisesRegex(ValueError, "requires a non-bypassing merge credential"):
                        finish_companion(api, 1219)
                    self.assertEqual(merges, [])

    @patch("repair_completion.time.sleep")
    def test_checks_are_observed_after_refresh_before_completion(self, _sleep):
        companion = {"number": 301, "node_id": "PR301", "changed_files": 1, "state": "open",
                     "merged": False, "head": {"sha": "stale", "repo": {"full_name": SUITE}}}
        armed = dict(companion, auto_merge={"merge_method": "merge"})
        refreshed = dict(armed, head=dict(armed["head"], sha="refreshed"))
        merged = dict(refreshed, state="closed", merged=True, merge_commit_sha="landed")
        states = iter((companion, companion, companion, armed, refreshed, refreshed, merged))
        verdicts = iter(({"check_runs": [check(None)]}, {"check_runs": [check("success")]}))
        bases = iter(("main", "main", "advanced", "advanced"))
        writes = []

        def api(path, *flags):
            if flags:
                writes.append((path, flags))
                return {}
            if path == ENDPOINT:
                return next(states)
            if path == CHECKS:
                return next(verdicts)
            if path == f"repos/{SUITE}/git/ref/heads/main":
                return {"object": {"sha": next(bases)}}
            return {
                f"repos/{BARN}/pulls/1219": {"state": "closed", "merged": True},
                f"repos/{SUITE}/pulls?state=open&head=MongooseMoo:barn-fix/pr-1219&base=main": [companion],
                f"{ENDPOINT}/files?per_page=100": [ALLOWED],
                f"repos/{SUITE}/commits/refreshed": {"parents": [{"sha": "stale"}, {"sha": "advanced"}]},
                f"repos/{SUITE}/compare/main...stale": {"behind_by": 1},
                f"repos/{SUITE}/compare/main...advanced": {"behind_by": 0},
                f"repos/{SUITE}/compare/advanced...refreshed": {"behind_by": 0},
                REQUIRED: {"checks": [{"context": "Full suite", "app_id": 15368}]},
            }[path]

        self.assertEqual(finish_companion(api, 1219), "Suite #301 merged at landed.")
        self.assertEqual([path for path, _ in writes], ["graphql", f"{ENDPOINT}/update-branch"])
        self.assertIn("head=stale", writes[0][1])
        self.assertIn("expected_head_sha=stale", writes[1][1])
        self.assertEqual(list(verdicts), [])


if __name__ == "__main__":
    unittest.main()
