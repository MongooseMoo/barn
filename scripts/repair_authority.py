"""Authorize Barn repair requests using GitHub identities and repository roles."""

import argparse
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import quote


BARN = "MongooseMoo/barn"
ACTIONS_BOT = 41898282
WORKER_BOT = 359488567
CLAUDE_BOT = 209825114
MERGE_QUEUE_BOT = 118344674
ALARM_LABELS = {"goodput-alarm", "conformance-suite-alarm"}


def pages(api, path):
    separator = "&" if "?" in path else "?"
    page = 1
    while True:
        rows = api(f"{path}{separator}per_page=100&page={page}")
        yield from rows
        if len(rows) < 100:
            return
        page += 1


def bot(user, *ids):
    return user.get("type") == "Bot" and user.get("id") in ids


def writer(api, user):
    if user.get("type") != "User" or not user.get("login"):
        return False
    login = quote(user["login"], safe="")
    role = api(f"repos/{BARN}/collaborators/{login}/permission")
    return ((role.get("user") or {}).get("id") == user.get("id")
            and role.get("permission") in {"write", "admin"})


def current_label_actor(api, number, labels):
    active = {}
    for event in pages(api, f"repos/{BARN}/issues/{number}/timeline"):
        name = (event.get("label") or {}).get("name")
        if name in labels and event["event"] in {"labeled", "unlabeled"}:
            active.pop(name, None)
            if event["event"] == "labeled":
                active[name] = event.get("actor") or {}
    return next(reversed(active.values()), {})


def authorized_item(api, stage, item, *, allow_closed_pr=False):
    labels = {label["name"] for label in item["labels"]}
    user = item.get("user") or {}
    if stage == "conformance":
        if not item.get("pull_request"):
            return False
        pr = api(f"repos/{BARN}/pulls/{item['number']}")
        return (pr["base"]["ref"] == "master"
                and (pr["head"]["repo"] or {}).get("full_name") == BARN
                and (pr["state"] == "open" or pr["merged"] or allow_closed_pr)
                and (bot(user, CLAUDE_BOT, WORKER_BOT) or writer(api, user)))
    if item.get("pull_request") or item["state"] != "open":
        return False
    if stage == "investigation" and not labels & ALARM_LABELS:
        return False
    if bot(user, ACTIONS_BOT):
        report_labels = (labels & ALARM_LABELS if stage == "investigation"
                         else labels & {"claude-fix"})
        return bool(report_labels and bot(current_label_actor(api, item["number"], report_labels), ACTIONS_BOT))
    return writer(api, user)


def authorized_event(api, event_name, event, mode):
    sender = event.get("sender") or {}
    if event_name == "issues" and mode == "repair" and event.get("action") == "labeled":
        if event["label"]["name"] != "claude-fix" or not writer(api, sender):
            return False
        stage, number = "repair", event["issue"]["number"]
    elif event_name == "workflow_dispatch":
        inputs = event["inputs"]
        stage = inputs.get("stage", "repair") if mode == "repair" else mode
        if mode == "repair" and stage not in {"repair", "conformance"}:
            return False
        if not (bot(sender, ACTIONS_BOT, WORKER_BOT) or writer(api, sender)):
            return False
        number = inputs["issue_number"]
    elif event_name == "pull_request" and mode == "repair" and event.get("action") == "closed":
        if not (bot(sender, CLAUDE_BOT, WORKER_BOT, MERGE_QUEUE_BOT) or writer(api, sender)):
            return False
        stage, number = "conformance", event["pull_request"]["number"]
    else:
        return False

    if not str(number).isascii() or not str(number).isdigit() or int(number) < 1:
        return False
    item = api(f"repos/{BARN}/issues/{int(number)}")
    if event_name == "issues" and not any(label["name"] == "claude-fix" for label in item["labels"]):
        return False
    if event_name == "pull_request" and not any(label["name"] == "automatic-fix" for label in item["labels"]):
        return False
    return authorized_item(api, stage, item, allow_closed_pr=event_name == "pull_request")


def approved_comments(api, number):
    permitted = {}
    for comment in pages(api, f"repos/{BARN}/issues/{number}/comments"):
        user = comment.get("user") or {}
        key = user.get("id")
        if key not in permitted:
            permitted[key] = bot(user, ACTIONS_BOT, WORKER_BOT, CLAUDE_BOT) or writer(api, user)
        if permitted[key]:
            yield comment


def github_api(path, payload=None, *, token=None):
    command = ["gh", "api", path]
    if payload is not None:
        command.extend(["--method", "POST", "--input", "-"])
    env = None if token is None else {**os.environ, "GH_TOKEN": token}
    result = subprocess.run(command, input=None if payload is None else json.dumps(payload),
                            env=env, check=True, stdout=subprocess.PIPE, encoding="utf-8", timeout=60)
    return json.loads(result.stdout) if result.stdout else None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("repair", "investigation"))
    parser.add_argument("--snapshot", type=Path)
    args = parser.parse_args()
    event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text(encoding="utf-8"))
    event_name = os.environ["GITHUB_EVENT_NAME"]
    authorized = (os.environ["GITHUB_REPOSITORY"] == BARN
                  and (event_name == "pull_request" or os.environ["GITHUB_REF"] == "refs/heads/master")
                  and authorized_event(github_api, event_name, event, args.mode))
    if args.snapshot:
        if not authorized:
            raise SystemExit("The repair request is no longer authorized")
        number = int(event["issue"]["number"] if event_name == "issues" else event["inputs"]["issue_number"])
        item = github_api(f"repos/{BARN}/issues/{number}")
        report = {"item": item, "comments": list(approved_comments(github_api, number))}
        args.snapshot.write_text(json.dumps(report, ensure_ascii=False), encoding="utf-8")
    else:
        print(f"Repair request authorized: {str(authorized).lower()}")
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
            print(f"authorized={str(authorized).lower()}", file=output)


if __name__ == "__main__":
    main()
