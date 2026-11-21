"""Read provider diagnostics without placing model transcripts in public logs."""

import argparse
import json
import re
from pathlib import Path


def classify(text, provider):
    try:
        records = json.loads(text)
        if isinstance(records, dict):
            records = [records]
    except ValueError:
        records = []
        for line in text.splitlines():
            try:
                records.append(json.loads(line))
            except ValueError:
                continue

    if not isinstance(records, list):
        records = []
    errors = []
    for record in records:
        if not isinstance(record, dict):
            continue
        kind = record.get("type")
        if provider == "claude" and kind == "rate_limit_event":
            info = record.get("rate_limit_info") or {}
            if info.get("status") == "rejected":
                reset = info.get("resetsAt")
                return "usage-limit", str(reset) if isinstance(reset, int) else ""
        if kind in {"error", "turn.failed"}:
            errors.append(str(record.get("error", record.get("message", ""))))
        if kind == "result" and record.get("is_error"):
            errors.extend((str(record.get("result", "")), str(record.get("errors", ""))))
        if kind == "assistant" and record.get("error"):
            errors.extend((str(record["error"]), str(record.get("message", ""))))

    error = "\n".join(errors).lower()
    if re.search(r"rate.limit|usage.limit|usage has been exceeded|hit your .*limit|insufficient.quota|credit.balance", error):
        return "usage-limit", ""
    if re.search(r"authentication.failed|invalid.api.key|unauthorized|not logged in|oauth.token.*expired", error):
        return "authentication", ""
    if re.search(r"overloaded|service.unavailable|internal.server.error", error):
        return "provider-unavailable", ""
    return "unknown", ""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("provider", choices=("claude", "codex"))
    parser.add_argument("execution_file", nargs="?", type=Path)
    args = parser.parse_args()
    text = ""
    if args.execution_file and args.execution_file.is_file():
        text = args.execution_file.read_text(encoding="utf-8")
    reason, reset = classify(text, args.provider)
    print(f"reason={reason}")
    print(f"retry_at={reset}")


if __name__ == "__main__":
    main()
