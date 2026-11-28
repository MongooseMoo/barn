import unittest
from unittest.mock import patch

from repair_completion import BARN, SUITE, finish_companion, required_check_state, wait_for_repair_checks


REQUIRED = f"repos/{SUITE}/branches/main"
ENDPOINT = f"repos/{SUITE}/pulls/301"
CHECKS = f"repos/{SUITE}/commits/refreshed/check-runs?per_page=100&filter=latest"
ALLOWED = {"filename": "src/moo_conformance/_tests/builtins/example.yaml", "status": "added"}


def required(context="Full suite"):
    return {"protection": {"required_status_checks": {"checks": [{"context": context, "app_id": 15368}]}}}


def check(conclusion, app=15368, context="Full suite"):
    return {"name": context, "app": {"id": app}, "conclusion": conclusion,
            "html_url": "https://github.com/MongooseMoo/moo-conformance-tests/runs/42"}


class CheckStateTest(unittest.TestCase):
    def test_pending_check_is_not_a_rejection(self):
        responses = {REQUIRED: required(), CHECKS: {"check_runs": [check(None), check("failure", app=99)]}}
        self.assertEqual(required_check_state(responses.__getitem__, SUITE, "main", "refreshed"),
                         ([], True))

    def test_failure_on_exact_head_carries_the_check_link(self):
        failed = check("failure")
        responses = {REQUIRED: required(), CHECKS: {"check_runs": [failed]}}
        self.assertEqual(required_check_state(responses.__getitem__, SUITE, "main", "refreshed"),
                         ([failed["html_url"]], True))

    @patch("repair_completion.time.sleep")
    def test_barn_checks_can_finish_without_arming_any_head(self, _sleep):
        for moved, already_queued in ((False, False), (True, False), (False, True)):
            with self.subTest(moved=moved, already_queued=already_queued):
                pending = {"state": "open", "merged": False, "head": {"sha": "proof"}, "node_id": "PR1216"}
                changed = dict(pending, head={"sha": "new"})
                prs = iter((pending, changed if moved else pending))
                checks = iter((None, "success"))
                queries = []

                def api(path, *flags):
                    queries.append(path)
                    if flags:
                        self.assertEqual(path, "graphql")
                        self.assertIn("query=query", flags[3])
                        return {"data": {"node": {
                            "headRefOid": "proof", "state": "OPEN", "isInMergeQueue": already_queued,
                        }}}
                    if path == f"repos/{BARN}/pulls/1216":
                        return next(prs)
                    if path == f"repos/{BARN}/branches/master":
                        return required()
                    return {"check_runs": [check(next(checks))]}

                if moved:
                    with self.assertRaisesRegex(ValueError, "Barn head moved after validation"):
                        wait_for_repair_checks(api, 1216, "proof")
                    self.assertNotIn("graphql", queries)
                else:
                    self.assertEqual(wait_for_repair_checks(api, 1216, "proof"),
                                     "queued" if already_queued else "ready")
                self.assertEqual(queries.count(f"repos/{BARN}/pulls/1216"), 2)


class CompanionTest(unittest.TestCase):
    def test_companion_does_not_land_before_barn(self):
        def api(path, *flags):
            self.assertEqual(path, f"repos/{BARN}/pulls/1215")
            return {"state": "open", "merged": False}

        with self.assertRaisesRegex(ValueError, "Barn #1215 is still open"):
            finish_companion(api, 1215)

    @patch("repair_completion.time.sleep")
    def test_refresh_rechecks_files_and_waits_for_protected_exact_head_merge(self, _sleep):
        initial = {"number": 301, "node_id": "PR301", "changed_files": 1, "state": "open",
                   "merged": False, "mergeable_state": "clean",
                   "head": {"sha": "stale", "repo": {"full_name": SUITE}}}
        refreshed = dict(initial, head=dict(initial["head"], sha="refreshed"))
        merged = dict(refreshed, state="closed", merged=True, merge_commit_sha="landed")
        for forbidden, can_bypass, receipt_matches in (
            (False, False, True), (True, False, True), (False, True, True), (False, False, False),
        ):
            with self.subTest(forbidden=forbidden, can_bypass=can_bypass, receipt_matches=receipt_matches):
                states = iter((initial, initial, initial, initial, refreshed, refreshed, refreshed, merged))
                verdicts = iter(({"check_runs": [check(None)]}, {"check_runs": [check("success")]}))
                bases = iter(("main", "main", "advanced", "advanced"))
                disallowed = {"filename": "src/moo_conformance/cli.py", "status": "modified"}
                file_lists = iter((ALLOWED, disallowed if forbidden else ALLOWED))
                calls = []

                def api(path, *flags):
                    calls.append((path, flags))
                    if path == ENDPOINT:
                        return next(states)
                    if path == f"{ENDPOINT}/files?per_page=100":
                        return [next(file_lists)]
                    if path == CHECKS:
                        return next(verdicts)
                    if path == f"repos/{SUITE}/git/ref/heads/main":
                        return {"object": {"sha": next(bases)}}
                    return {
                        f"repos/{BARN}/pulls/1219": {"state": "closed", "merged": True},
                        f"repos/{SUITE}/pulls?state=open&head=MongooseMoo:barn-fix/pr-1219&base=main": [initial],
                        f"repos/{SUITE}/commits/refreshed": {"parents": [{"sha": "stale"}, {"sha": "advanced"}]},
                        f"repos/{SUITE}/compare/main...stale": {"behind_by": 1},
                        f"repos/{SUITE}/compare/main...advanced": {"behind_by": 0},
                        f"repos/{SUITE}/compare/advanced...refreshed": {"behind_by": 0},
                        REQUIRED: required(),
                        f"{ENDPOINT}/update-branch": None,
                        "graphql": {"data": {"node": {"headRefOid": "refreshed", "viewerCanMergeAsAdmin": can_bypass}}},
                        f"{ENDPOINT}/merge-async": {"status": "pending", "details": {
                            "uuid": "request-id", "expected_head_sha": "refreshed" if receipt_matches else "other",
                            "bypass_rules": False,
                        }},
                        f"{ENDPOINT}/merge-async/request-id": {"status": "merged", "details": {"sha": "landed"}},
                    }[path]

                if forbidden or can_bypass or not receipt_matches:
                    with self.assertRaisesRegex(ValueError, "outside the allowed|non-bypassing|does not bind"):
                        finish_companion(api, 1219)
                else:
                    self.assertEqual(finish_companion(api, 1219), "Suite #301 merged at landed.")
                    self.assertEqual(list(verdicts), [])
                self.assertEqual(list(file_lists), [])
                writes = {path: flags for path, flags in calls if "-X" in flags}
                self.assertIn("expected_head_sha=stale", writes[f"{ENDPOINT}/update-branch"])
                if forbidden or can_bypass:
                    self.assertEqual(set(writes), {f"{ENDPOINT}/update-branch"})
                else:
                    merge = writes[f"{ENDPOINT}/merge-async"]
                    self.assertIn("merge_method=merge", merge)
                    self.assertIn("sha=refreshed", merge)
                    self.assertIn("bypass_rules=false", merge)


if __name__ == "__main__":
    unittest.main()
