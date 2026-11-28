"""Dispatch Barn conformance for each newly observed suite main revision."""

import argparse
from functools import partial
import json
import os
from pathlib import Path
import re
import subprocess
from tempfile import NamedTemporaryFile

from repair_authority import WORKER_BOT, github_api
from repair_publisher import PublisherToken, dispatch, existing_dispatch


SUITE_URL = "https://github.com/MongooseMoo/moo-conformance-tests.git"
WORKFLOW = "conformance-upstream.yml"
SHA = re.compile(r"[0-9a-f]{40}")


def git(directory, *args):
    return subprocess.run(["git", "-c", "core.hooksPath=/dev/null", *args], cwd=directory,
                          env={**os.environ, "GIT_TERMINAL_PROMPT": "0"}, check=True,
                          stdout=subprocess.PIPE, encoding="utf-8", timeout=120).stdout.strip()


def pending_revisions(repository, cursor, tip):
    if cursor is None:
        return [tip]
    if not SHA.fullmatch(cursor):
        raise ValueError("Invalid stored conformance cursor")
    if cursor == tip:
        return []
    revisions = git(repository, "rev-list", "--first-parent", "--reverse", f"{cursor}..{tip}").splitlines()
    if not revisions or git(repository, "rev-parse", f"{revisions[0]}^1") != cursor:
        raise ValueError(f"Suite main no longer advances from {cursor} to {tip}")
    return revisions


def save_cursor(filename, revision):
    with NamedTemporaryFile(mode="w", encoding="ascii", dir=filename.parent, delete=False) as pending:
        temporary = Path(pending.name)
        pending.write(f"{revision}\n")
        pending.flush()
        os.fsync(pending.fileno())
    os.replace(temporary, filename)


def submit(revisions, cursor_path, api):
    for revision in revisions:
        title = f"Conformance suite {revision}"
        run_id = existing_dispatch(api, WORKFLOW, title, WORKER_BOT)
        if run_id is None:
            run_id = dispatch(api, WORKFLOW, {"suite_sha": revision})
        print(f"Conformance suite {revision}: Barn Actions run {run_id}", flush=True)
        save_cursor(cursor_path, revision)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--state", type=Path, required=True)
    parser.add_argument("--publisher", type=Path, default=Path("/etc/barn-repair/github-app.json"))
    args = parser.parse_args()
    args.state.mkdir(parents=True, exist_ok=True)
    repository = args.state / "suite.git"
    repository.mkdir(exist_ok=True)
    if not (repository / "HEAD").is_file():
        git(repository, "init", "--bare", "--quiet")
    git(repository, "fetch", "--quiet", "--no-tags", SUITE_URL, "+refs/heads/main:refs/heads/main")
    tip = git(repository, "rev-parse", "refs/heads/main^{commit}")
    cursor_path = args.state / "last-dispatch"
    cursor = cursor_path.read_text(encoding="ascii").strip() if cursor_path.is_file() else None
    revisions = pending_revisions(repository, cursor, tip)
    if revisions:
        publisher = PublisherToken(json.loads(args.publisher.read_text(encoding="utf-8")),
                                   repositories=("barn",), permissions={"actions": "write"})
        submit(revisions, cursor_path, partial(github_api, token=publisher.get()))


if __name__ == "__main__":
    main()
