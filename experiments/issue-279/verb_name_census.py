"""Count canonical verb-name sizes in the real input; emit no world text."""

import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path

from lambdamoo_db.reader import load


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("database", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    with args.database.open("rb") as source:
        digest = hashlib.file_digest(source, "sha256").hexdigest()
    database = load(str(args.database))
    names = Counter(
        verb.name
        for obj in database.objects.values()
        if obj is not None
        for verb in obj.verbs
    )
    limits = [8, 16, 32, 64, 128]
    report = {"input_sha256": digest, "definitions": sum(names.values()), "distinct": len(names)}
    report["size_buckets"] = [
        {
            "max_bytes": limit,
            "definitions_at_most": sum(count for name, count in names.items() if len(name.encode("latin-1")) <= limit),
            "distinct_at_most": sum(len(name.encode("latin-1")) <= limit for name in names),
        }
        for limit in limits
    ]
    encoded = json.dumps(report, indent=2) + "\n"
    args.output.write_text(encoded, encoding="utf-8")
    print(encoded, end="")


if __name__ == "__main__":
    main()
