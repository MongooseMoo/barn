"""Retain exact public Go fuzz records, independently of a runner's build cache."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil


MAX_BYTES = 256 * 1024 * 1024


def native_corpus(cache, package, target):
    if not re.fullmatch(r"\./[a-zA-Z0-9_-]+(?:/[a-zA-Z0-9_-]+)*", package):
        raise ValueError("package must be a relative Go package")
    if not re.fullmatch(r"Fuzz[a-zA-Z0-9_]+", target):
        raise ValueError("target must be a Go fuzz function")
    return cache / "fuzz/github.com/MongooseMoo/barn" / package.removeprefix("./") / target


def quarantine(cache, package, target, source, found):
    """Keep an unminimized finding, removing its exact disposable replay copies."""
    native = native_corpus(cache, package, target)
    if source.parent != Path(package) / "testdata/fuzz" / target or source.is_symlink():
        raise ValueError("finding must be a regular record in this target's testdata")
    data = source.read_bytes()
    if not data.startswith(b"go test fuzz v1\n"):
        raise ValueError("finding is not a Go fuzz record")
    found.mkdir(parents=True, exist_ok=True)
    destination = found / source.name
    if destination.exists():
        # A static f.Add seed cannot be removed from a disposable cache. Keep
        # the original evidence and stop instead of repeating it for 30 minutes.
        raise ValueError("finding repeated after quarantine; no new campaign work")
    shutil.copyfile(source, destination)
    removed = 0
    if native.exists():
        for path in native.iterdir():
            if path.is_symlink() or not path.is_file():
                raise ValueError(f"unexpected corpus entry: {path}")
            if path.read_bytes() == data:
                path.unlink()
                removed += 1
    source.unlink()
    print(json.dumps({"quarantined": str(destination), "removed_replay_copies": removed}))


def transfer(mode, root, cache, package, target, head, go_version, profile, logs=None, campaign=None):
    if mode not in ("capture", "restore"):
        raise ValueError("unknown corpus transfer mode")
    native = native_corpus(cache, package, target)
    lane = root / package.removeprefix("./") / target
    manifest_path = lane / "manifest.json"
    if manifest_path.exists() and manifest_path.stat().st_size > MAX_BYTES:
        raise ValueError("retained corpus provenance exceeds 256 MiB")
    manifest = json.loads(manifest_path.read_text()) if manifest_path.exists() else {
        "version": 1, "package": package, "target": target, "records": {}}
    if (manifest["version"], manifest["package"], manifest["target"]) != (1, package, target):
        raise ValueError("retained corpus identity mismatch")
    records = lane / "inputs"
    retained_size = manifest_path.stat().st_size if manifest_path.exists() else 0
    for digest in manifest["records"]:
        if not re.fullmatch(r"[0-9a-f]{64}", digest):
            raise ValueError("retained corpus has an invalid record identity")
        path = records / digest
        retained_size += path.stat().st_size
        if retained_size > MAX_BYTES:
            raise ValueError("retained corpus and provenance exceed 256 MiB")
        if path.is_symlink() or hashlib.sha256(path.read_bytes()).hexdigest() != digest:
            raise ValueError(f"retained corpus digest mismatch: {digest}")
    if mode == "capture":
        records.mkdir(parents=True, exist_ok=True)
        sources = [(native, "interesting"),
                   (Path(package) / "testdata/fuzz" / target, "testdata")]
        total = sum(path.stat().st_size for path in records.iterdir() if path.is_file())
        for source, kind in sources:
            if not source.exists():
                continue
            for path in sorted(source.iterdir()):
                if path.is_symlink() or not path.is_file():
                    raise ValueError(f"unexpected corpus entry: {path}")
                data = path.read_bytes()
                if not data.startswith(b"go test fuzz v1\n"):
                    raise ValueError(f"not a Go fuzz record: {path}")
                digest = hashlib.sha256(data).hexdigest()
                destination = records / digest
                if not destination.exists():
                    if total + len(data) > MAX_BYTES:
                        raise ValueError("retained corpus exceeds 256 MiB; preserve artifacts before resizing")
                    destination.write_bytes(data)
                    total += len(data)
                origin = {"head": head, "go": go_version, "profile_sha256": profile,
                          "source": kind, "name": path.name}
                origins = manifest["records"].setdefault(digest, [])
                if origin not in origins:
                    origins.append(origin)
        encoded_manifest = json.dumps(manifest, indent=2) + "\n"
        if total + len(encoded_manifest.encode()) > MAX_BYTES:
            raise ValueError("retained corpus and provenance exceed 256 MiB; preserve artifacts before resizing")
        manifest_path.write_text(encoded_manifest)
    else:
        native.mkdir(parents=True, exist_ok=True)
        for digest in manifest["records"]:
            path = records / digest
            shutil.copyfile(path, native / digest)
    counts = {}
    for origins in manifest["records"].values():
        for kind in {origin["source"] for origin in origins}:
            counts[kind] = counts.get(kind, 0) + 1
    summary = {"mode": mode, "package": package, "target": target,
               "head": head, "go": go_version, "profile_sha256": profile,
               "retained_records": len(manifest["records"]), "per_source": counts,
               "manifest_sha256": hashlib.sha256(manifest_path.read_bytes()).hexdigest()
               if manifest_path.exists() else None}
    if mode == "capture" and logs is not None:
        completed = 0
        evidence = lane / "logs"
        evidence.mkdir(exist_ok=True)
        for path in sorted(logs.glob("*.log")):
            executions = re.findall(r"^fuzz: elapsed: [^\n,]+, execs: (\d+) ",
                                    path.read_text(errors="replace"), re.MULTILINE)
            completed += max(map(int, executions), default=0)
            shutil.copyfile(path, evidence / path.name)
        summary["completed_executions"] = completed
        # clean, finding or infrastructure; empty when the campaign never ended.
        summary["campaign"] = campaign or None
        (lane / "census.json").write_text(json.dumps(summary, indent=2) + "\n")
    print(json.dumps(summary, sort_keys=True))
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as stream:
            stream.write("```json\n" + json.dumps(summary, indent=2) + "\n```\n")
    if mode == "capture" and logs is not None and not summary["completed_executions"]:
        raise ValueError("zero measured native fuzz executions; campaign is inconclusive")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("restore", "capture", "quarantine"))
    parser.add_argument("--root", type=Path, default=Path(".fuzz/retained"))
    parser.add_argument("--cache", type=Path, required=True)
    parser.add_argument("--package", required=True)
    parser.add_argument("--target", required=True)
    parser.add_argument("--head")
    parser.add_argument("--go-version")
    parser.add_argument("--input", type=Path)
    parser.add_argument("--found", type=Path)
    parser.add_argument("--profile", type=Path, default=Path("profiles/toast/stock-wsl-testdb.json"))
    parser.add_argument("--logs", type=Path)
    parser.add_argument("--campaign", choices=("", "clean", "finding", "infrastructure"))
    args = parser.parse_args()
    if args.mode == "quarantine":
        if args.input is None or args.found is None:
            parser.error("quarantine requires --input and --found")
        quarantine(args.cache, args.package, args.target, args.input, args.found)
        return
    if args.head is None or args.go_version is None:
        parser.error("capture and restore require --head and --go-version")
    transfer(args.mode, args.root, args.cache, args.package, args.target,
             args.head, args.go_version, hashlib.sha256(args.profile.read_bytes()).hexdigest(), args.logs,
             args.campaign)


if __name__ == "__main__":
    main()
