"""Import exact verb bytes through lambdamoo-db-py's existing database reader."""

import argparse
import hashlib
import json
from pathlib import Path

from lambdamoo_db.inspection import code_fingerprint
from lambdamoo_db.reader import load_bytes


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--db", type=Path, action="append", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    manifest = {}
    # Extract before writing, so an unreadable source never becomes a partial
    # successful import. Property data and object/verb names are not exported.
    for path in args.db:
        raw = path.read_bytes()
        database = load_bytes(raw, str(path))
        source_digest = hashlib.sha256(raw).hexdigest()
        count = 0
        for obj in sorted(database.objects.values(), key=lambda obj: obj.id):
            for index, verb in enumerate(obj.verbs):
                if verb.code is None:
                    continue
                count += 1
                data = "".join(line + "\n" for line in verb.code).encode("latin-1")
                digest = code_fingerprint(verb)["sha256"]
                if hashlib.sha256(data).hexdigest() != digest:
                    raise ValueError("source bytes disagree with reader fingerprint")
                entry = manifest.setdefault(digest, {"source": data, "origins": []})
                entry["origins"].append({"database": str(path.resolve()),
                                         "database_sha256": source_digest,
                                         "object": obj.id, "anonymous": obj.anon,
                                         "verb_index": index})
        if count != database.total_verbs:
            raise ValueError(f"{path}: extracted {count} programs, declared {database.total_verbs}")
        print(json.dumps({"database": str(path), "programs": count}), flush=True)
    args.output.mkdir(parents=True, exist_ok=True)
    for digest, entry in sorted(manifest.items()):
        (args.output / (digest + ".moo")).write_bytes(entry.pop("source"))
    # Local provenance stays beside the private corpus, outside tracked code.
    (args.output / "manifest.json").write_text(
        json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"unique_sources": len(manifest), "output": str(args.output)}))


if __name__ == "__main__":
    main()
