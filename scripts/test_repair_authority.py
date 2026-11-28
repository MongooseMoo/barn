import subprocess
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import repair_authority as policy
import repair_worker


WRITER = {"type": "User", "id": 236552, "login": "ctoth"}
READER = {"type": "User", "id": 1024025, "login": "torvalds"}
ACTIONS = {"type": "Bot", "id": policy.ACTIONS_BOT, "login": "github-actions[bot]"}
WORKER = {"type": "Bot", "id": policy.WORKER_BOT, "login": "mongoosemoo-barn-repair-worker[bot]"}
FOREIGN_BOT = {"type": "Bot", "id": 99, "login": WORKER["login"]}
REPO = f"repos/{policy.BARN}"


def item(opener, *labels, **fields):
    return {"number": 1201, "state": "open", "user": opener,
            "labels": [{"name": label} for label in labels], **fields}


def label_event(name, actor, action="labeled"):
    return {"label": {"name": name}, "actor": actor, "event": action}


class RepairAuthorityTests(unittest.TestCase):
    def api(self, issue, timeline=(), **extra):
        records = {f"{REPO}/issues/1201": issue,
                   f"{REPO}/issues/1201/timeline?per_page=100&page=1": list(timeline),
                   f"{REPO}/collaborators/ctoth/permission": {"permission": "admin", "user": WRITER},
                   f"{REPO}/collaborators/torvalds/permission": {"permission": "read", "user": READER},
                   **extra}
        return records.__getitem__

    def test_label_requires_writer_opener_and_writer_sender(self):
        label = {"action": "labeled", "label": {"name": "claude-fix"}, "issue": {"number": 1201}}
        for opener, sender, result in [(WRITER, WRITER, True), (READER, WRITER, False), (WRITER, READER, False)]:
            with self.subTest(opener=opener, sender=sender):
                self.assertEqual(policy.authorized_event(self.api(item(opener, "claude-fix")), "issues",
                                                          {**label, "sender": sender}, "repair"), result)
        self.assertFalse(policy.authorized_event(self.api(item(WRITER)), "issues",
                                                 {**label, "sender": WRITER}, "repair"))

    def test_bot_handoff_requires_exact_identity_and_machine_report(self):
        events = [label_event("claude-fix", ACTIONS)]
        for opener, sender, result in [(ACTIONS, ACTIONS, True), (ACTIONS, WORKER, True),
                                       (ACTIONS, FOREIGN_BOT, False), (FOREIGN_BOT, WORKER, False),
                                       (READER, WORKER, False)]:
            with self.subTest(opener=opener, sender=sender):
                event = {"inputs": {"issue_number": "1201"}, "sender": sender}
                self.assertEqual(policy.authorized_event(self.api(item(opener, "claude-fix"), events),
                                                          "workflow_dispatch", event, "repair"), result)
        event = {"inputs": {"issue_number": "1201"}, "sender": ACTIONS}
        self.assertFalse(policy.authorized_event(self.api(item(ACTIONS, "claude-fix")),
                                                 "workflow_dispatch", event, "repair"))

    def test_alarm_bot_preserves_investigation_dispatch(self):
        alarm = item(ACTIONS, "conformance-suite-alarm")
        events = [label_event("conformance-suite-alarm", ACTIONS)]
        dispatch = {"inputs": {"issue_number": "1201"}, "sender": ACTIONS}
        self.assertTrue(policy.authorized_event(self.api(alarm, events), "workflow_dispatch", dispatch, "investigation"))
        self.assertFalse(policy.authorized_event(self.api(alarm, events), "workflow_dispatch", dispatch, "repair"))
        invalid = {**dispatch, "inputs": {**dispatch["inputs"], "stage": "investigation"}}
        self.assertFalse(policy.authorized_event(self.api(alarm, events), "workflow_dispatch", invalid, "repair"))

    def test_conformance_dispatch_preserves_same_repository_repair_pr(self):
        issue = item(WORKER, "automatic-fix", pull_request={"url": f"{REPO}/pulls/1201"})
        dispatch = {"inputs": {"issue_number": "1201", "stage": "conformance"}, "sender": WORKER}
        pr = {"state": "open", "merged": False, "base": {"ref": "master"},
              "head": {"repo": {"full_name": policy.BARN}}}
        api = self.api(issue, **{f"{REPO}/pulls/1201": pr})
        self.assertTrue(policy.authorized_event(api, "workflow_dispatch", dispatch, "repair"))
        pr["head"]["repo"]["full_name"] = "outside/barn"
        self.assertFalse(policy.authorized_event(api, "workflow_dispatch", dispatch, "repair"))

    def test_removed_label_and_unapproved_comments(self):
        timeline = [label_event("codex-repair", ACTIONS), label_event("codex-repair", WRITER, "unlabeled")]
        comments = [{"user": READER, "id": 1}, {"user": WRITER, "id": 2}, {"user": ACTIONS, "id": 3}]
        api = self.api(item(WRITER), timeline, **{f"{REPO}/issues/1201/comments?per_page=100&page=1": comments})
        self.assertEqual(policy.current_label_actor(api, 1201, {"codex-repair"}), {})
        self.assertEqual([comment["id"] for comment in policy.approved_comments(api, 1201)], [2, 3])

    def test_permission_lookup_errors_stop_authorization(self):
        label = {"action": "labeled", "label": {"name": "claude-fix"}, "issue": {"number": 1201},
                 "sender": WRITER}

        def unavailable(path):
            raise subprocess.CalledProcessError(1, ["gh", "api", path])

        with self.assertRaises(subprocess.CalledProcessError):
            policy.authorized_event(unavailable, "issues", label, "repair")

    def test_codex_skips_denied_items_without_starting_provider(self):
        queue = f"{REPO}/issues?state=all&labels=codex-repair&sort=created&direction=asc&per_page=100&page=1"
        api = self.api(item(WRITER, "codex-repair"), [label_event("codex-repair", READER)], **{
            queue: [item(WRITER, "codex-repair")],
            queue.replace("codex-repair", "codex-conformance"): [],
            queue.replace("codex-repair", "codex-investigation"): [],
        })
        with patch.object(repair_worker, "api", api), patch.object(repair_worker, "process") as launch:
            self.assertTrue(repair_worker.cycle(SimpleNamespace(number=None)))
        launch.assert_not_called()


if __name__ == "__main__":
    unittest.main()
