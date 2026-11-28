"""Validate Barn's packaged conformance results and file assertion failures."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import xml.etree.ElementTree as ET

from repair_authority import ACTIONS_BOT, BARN, bot, github_api, pages
from repair_publisher import dispatch, existing_dispatch


REPAIR = "claude-fix.yml"
CASE = re.compile(r"test_yaml_conformance\[([^\r\n]+?\.yaml)::([^\r\n]+)\]")
HARNESS = "moo-conformance-tests.src.moo_conformance.test_conformance"


def failed_cases(directory, context, exit_code):
    evidence = json.loads((directory / "conformance-admission.json").read_text(encoding="utf-8"))
    probes = evidence["probes"]
    if (evidence["schema_version"] != 2 or evidence["context"] != context
            or evidence["phase"] != "admission" or not probes
            or any(probe["status"] != "passed" for probe in probes)):
        raise ValueError("Conformance admission did not produce passing evidence for this run")

    results = ET.parse(directory / "conformance-results.xml")
    testcases = list(results.iter("testcase"))
    bad = ("failure", "error", "skipped")
    admission = [case for case in testcases if case.get("classname") == HARNESS
                 and case.get("name") == "test_capability_admission"]
    if (len(admission) != 1 or any(admission[0].find(tag) is not None for tag in bad)
            or not any(prop.get("name") == "admission_context" and prop.get("value") == context
                       for prop in admission[0].findall("./properties/property"))):
        raise ValueError("Conformance JUnit did not record passing admission for this run")
    if any(case.find("error") is not None for case in testcases):
        raise ValueError("Conformance JUnit recorded a test setup or teardown error")

    ran = 0
    failed = set()
    for case in testcases:
        match = CASE.fullmatch(case.get("name", ""))
        if not match or case.get("classname") != HARNESS or case.find("skipped") is not None:
            continue
        ran += 1
        if case.find("failure") is not None:
            failed.add(match.groups())
    if ran == 0 or bool(failed) != (exit_code == 1):
        raise ValueError(f"Conformance exit {exit_code}: {ran} packaged cases ran, {len(failed)} failed")
    print(f"Conformance evidence: {ran} packaged cases ran, {len(failed)} failed")
    return sorted(failed)


def report_failure(api, barn, suite, cases, run_url):
    digest = hashlib.sha256(json.dumps(cases, ensure_ascii=True).encode()).hexdigest()
    marker = f"<!-- barn-upstream-conformance:{barn}:{suite}:{digest} -->"
    issues = pages(api, f"repos/{BARN}/issues?state=all&labels=claude-fix")
    existing = next((item for item in issues if bot(item["user"] or {}, ACTIONS_BOT)
                     and (item.get("body") or "").startswith(marker + "\n")), None)
    if existing is None:
        paths = sorted({path for path, _ in cases})
        selectors = ""
        if len(paths) <= 20 and all("`" not in path and len(path) <= 400 for path in paths):
            selectors = " " + " ".join(f"--moo-suite-path={shlex.quote(path)}" for path in paths)
        lines = [marker, "The managed MOO conformance suite reported failed cases.", "",
                 f"Barn revision `{barn}`", f"Suite revision `{suite}`", "",
                 f"[Actions logs and conformance evidence]({run_url})", "", "Failed case IDs", "```text"]
        lines.extend(f"{path}::{name}".replace("`", "\\x60")[:500] for path, name in cases[:50])
        lines.extend(["```", f"The artifact contains all {len(cases)} failed cases.", "",
                      "Check out both revisions with suite submodules and build `bin/barn` from `./cmd/barn`.",
                      "From Barn, with the suite at `./moo-conformance-tests`, run", "```bash",
                      f"bash scripts/run-packaged-conformance.sh local-upstream-repro{selectors}", "```"])
        existing = api(f"repos/{BARN}/issues", {"title": f"MOO conformance failure at suite {suite[:12]}",
                                               "body": "\n".join(lines), "labels": ["claude-fix"]})
    number = existing["number"]
    print(f"Conformance issue #{number} {existing['html_url']}")
    if existing["state"] != "open":
        return
    run_id = existing_dispatch(api, REPAIR, f"Repair #{number} / auto", ACTIONS_BOT)
    if run_id is None:
        run_id = dispatch(api, REPAIR, {"issue_number": str(number)})
    print(f"Conformance issue #{number}: repair Actions run {run_id}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--exit-code", required=True, type=int, choices=(0, 1))
    args = parser.parse_args()
    barn = os.environ["GITHUB_SHA"]
    suite = os.environ["SUITE_SHA"]
    run = os.environ["GITHUB_RUN_ID"]
    attempt = os.environ["GITHUB_RUN_ATTEMPT"]
    context = f"barn-upstream:{run}:{attempt}:{barn}:{suite}"
    cases = failed_cases(args.directory, context, args.exit_code)
    if cases:
        report_failure(github_api, barn, suite, cases, f"https://github.com/{BARN}/actions/runs/{run}/attempts/{attempt}")


if __name__ == "__main__":
    main()
