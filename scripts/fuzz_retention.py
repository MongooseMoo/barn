"""Retain exact public Go fuzz records, independently of a runner's build cache."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import tempfile


MAX_BYTES = 256 * 1024 * 1024


def native_corpus(cache, package, target):
    if not re.fullmatch(r"\./[a-zA-Z0-9_-]+(?:/[a-zA-Z0-9_-]+)*", package):
        raise ValueError("package must be a relative Go package")
    if not re.fullmatch(r"Fuzz[a-zA-Z0-9_]+", target):
        raise ValueError("target must be a Go fuzz function")
    return cache / "fuzz/github.com/MongooseMoo/barn" / package.removeprefix("./") / target


def go_list_items(source, start):
    """Count the comma-separated items of the Go list opening just before start."""
    depth, items, quote, index = 0, 1, None, start
    while True:
        char = source[index]
        if quote:
            if char == "\\" and quote != "`":
                index += 1
            elif char == quote:
                quote = None
        elif char in "\"'`":
            quote = char
        elif char in "([{":
            depth += 1
        elif char in ")]}":
            if depth == 0:
                return items
            depth -= 1
        elif char == "," and depth == 0:
            items += 1
        index += 1


def target_values(package, target):
    """Count the values one corpus record of target must hold.

    Go reads a corpus record only when its value count matches the f.Fuzz
    callback (internal/fuzz CheckCorpus); others are skipped and never fuzzed.
    A literal callback's parameters after *testing.T are counted directly. A
    callback built by a call (rapid.MakeFuzz) is counted from the target's first
    f.Add seed, which Go checks against the callback the same way.
    """
    declaration = re.compile(r"^func " + target + r"\((\w+) \*testing\.F\) \{$", re.MULTILINE)
    for path in sorted(Path(package).glob("*_test.go")):
        source = path.read_text()
        match = declaration.search(source)
        if match is None:
            continue
        fuzz = source.find(match[1] + ".Fuzz(", match.end())
        if fuzz < 0:
            break
        callback = fuzz + len(match[1] + ".Fuzz(")
        if source.startswith("func(", callback):
            # A grouped "a, b int" still separates every parameter with a comma.
            return go_list_items(source, callback + len("func(")) - 1
        seed = source.find(match[1] + ".Add(", match.end(), fuzz)
        if seed < 0:
            break
        return go_list_items(source, seed + len(match[1] + ".Add("))
    raise ValueError(f"no f.Fuzz callback or f.Add seed found for {target} in {package}")


def record_values(data):
    """Count a record's values as Go's unmarshalCorpusFile does: non-blank lines after the header."""
    return sum(1 for line in data.split(b"\n")[1:] if line.strip())


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
        # the original evidence and stop instead of repeating it for the lane's minutes.
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


def capture_inputs(manifest, paths, native, package, target, head, go_version, profile, dropped, values):
    for source, kind in ((native, "interesting"), (Path(package) / "testdata/fuzz" / target, "testdata")):
        if not source.exists():
            continue
        for path in sorted(source.iterdir()):
            if path.is_symlink() or not path.is_file():
                raise ValueError(f"unexpected corpus entry: {path}")
            data = path.read_bytes()
            if not data.startswith(b"go test fuzz v1\n"):
                raise ValueError(f"not a Go fuzz record: {path}")
            digest = hashlib.sha256(data).hexdigest()
            if record_values(data) != values:
                dropped.add(digest)
                continue
            if kind == "interesting" and path.name == digest and digest in paths:
                # Restore writes a full-digest filename. Replaying it is not a
                # new discovery with a new origin on every continuous cycle.
                continue
            paths.setdefault(digest, path)
            origin = {"head": head, "go": go_version, "profile_sha256": profile,
                      "source": kind, "name": path.name}
            origins = manifest["records"].setdefault(digest, [])
            if origin not in origins:
                origins.append(origin)


def save_capture(lane, manifest, paths, overflow):
    contents = json.dumps(manifest, separators=(",", ":")) + "\n"
    sizes = {digest: path.stat().st_size for digest, path in paths.items()}
    total = len(contents.encode()) + sum(sizes.values())
    if total > MAX_BYTES:
        if overflow is None:
            raise ValueError("retained corpus and provenance exceed 256 MiB; preserve artifacts before resizing")
        if overflow.exists():
            if (overflow / "manifest.json").read_bytes() != contents.encode():
                raise ValueError("overflow archive already contains a different generation")
        else:
            overflow.parent.mkdir(parents=True, exist_ok=True)
            with tempfile.TemporaryDirectory(dir=overflow.parent) as temporary:
                archive = Path(temporary) / "archive"
                (archive / "inputs").mkdir(parents=True)
                for digest, path in paths.items():
                    shutil.copyfile(path, archive / "inputs" / digest)
                (archive / "manifest.json").write_bytes(contents.encode())
                archive.replace(overflow)
        # Keep the full generation in the overflow artifact and leave room
        # for new records in the next cache. Favor confirmed testdata.
        records = manifest["records"]
        empty = {**manifest, "records": {}}
        budget = MAX_BYTES // 2 - len(json.dumps(empty, separators=(",", ":")).encode()) - 1
        selected = {}
        ranked = sorted(reversed(records), key=lambda digest: not any(
            origin["source"] == "testdata" for origin in records[digest]))
        for digest in ranked:
            entry = len(json.dumps({digest: records[digest]}, separators=(",", ":")).encode())
            cost = sizes[digest] + entry
            if cost <= budget:
                selected[digest] = records[digest]
                budget -= cost
        manifest["records"] = selected
        contents = json.dumps(manifest, separators=(",", ":")) + "\n"

    records_dir = lane / "inputs"
    records_dir.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=lane) as temporary:
        staging = Path(temporary)
        for digest in manifest["records"]:
            destination = records_dir / digest
            path = paths[digest]
            if path != destination:
                shutil.copyfile(path, staging / digest)
                (staging / digest).replace(destination)
        (staging / "manifest.json").write_bytes(contents.encode())
        (staging / "manifest.json").replace(lane / "manifest.json")
    for path in records_dir.iterdir():
        if path.name not in manifest["records"]:
            path.unlink()
    return total > MAX_BYTES


def transfer(mode, root, cache, package, target, head, go_version, profile, logs=None, campaign=None, overflow=None):
    if mode not in ("capture", "restore"):
        raise ValueError("unknown corpus transfer mode")
    native = native_corpus(cache, package, target)
    values = target_values(package, target)
    lane = root / package.removeprefix("./") / target
    if mode == "capture" and logs is not None:
        completed = 0
        evidence = lane / "logs"
        evidence.mkdir(exist_ok=True, parents=True)
        for path in sorted(logs.glob("*.log")):
            executions = re.findall(r"^fuzz: elapsed: [^\n,]+, execs: (\d+) ",
                                    path.read_text(errors="replace"), re.MULTILINE)
            completed += max(map(int, executions), default=0)
            shutil.copyfile(path, evidence / path.name)
        census = {"target": target, "completed_executions": completed, "campaign": campaign or None}
        (lane / "census.json").write_text(json.dumps(census, indent=2) + "\n")
    manifest_path = lane / "manifest.json"
    if manifest_path.exists() and manifest_path.stat().st_size > MAX_BYTES:
        raise ValueError("retained corpus provenance exceeds 256 MiB")
    manifest = json.loads(manifest_path.read_text()) if manifest_path.exists() else {
        "version": 1, "package": package, "target": target, "records": {}}
    if (manifest["version"], manifest["package"], manifest["target"]) != (1, package, target):
        raise ValueError("retained corpus identity mismatch")
    records = lane / "inputs"
    paths = {}
    retained_size = manifest_path.stat().st_size if manifest_path.exists() else 0
    # Records the current target rejects for their value count; Go never fuzzes them.
    dropped = set()
    for digest in manifest["records"]:
        if not re.fullmatch(r"[0-9a-f]{64}", digest):
            raise ValueError("retained corpus has an invalid record identity")
        path = records / digest
        retained_size += path.stat().st_size
        if retained_size > MAX_BYTES:
            raise ValueError("retained corpus and provenance exceed 256 MiB")
        data = None if path.is_symlink() else path.read_bytes()
        if data is None or hashlib.sha256(data).hexdigest() != digest:
            raise ValueError(f"retained corpus digest mismatch: {digest}")
        if record_values(data) != values:
            dropped.add(digest)
        else:
            paths[digest] = path
    if mode == "capture":
        for digest in dropped:
            del manifest["records"][digest]
        capture_inputs(manifest, paths, native, package, target, head, go_version, profile, dropped, values)
        rotated = save_capture(lane, manifest, paths, overflow)
        if rotated and os.environ.get("GITHUB_OUTPUT"):
            with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
                output.write("overflow=true\n")
    else:
        native.mkdir(parents=True, exist_ok=True)
        for digest in manifest["records"]:
            if digest not in dropped:
                shutil.copyfile(records / digest, native / digest)
    counts = {}
    for origins in manifest["records"].values():
        for kind in {origin["source"] for origin in origins}:
            counts[kind] = counts.get(kind, 0) + 1
    summary = {"mode": mode, "package": package, "target": target,
               "head": head, "go": go_version, "profile_sha256": profile,
               "retained_records": len(manifest["records"]), "per_source": counts,
               "target_values": values, "dropped_wrong_value_count": len(dropped),
               "manifest_sha256": hashlib.sha256(manifest_path.read_bytes()).hexdigest()
               if manifest_path.exists() else None}
    if mode == "capture" and logs is not None:
        summary.update(census, corpus_overflow=rotated)
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
    parser.add_argument("--overflow", type=Path, help="Preserve a full generation before rotating the replay cache")
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
             args.campaign, args.overflow)


if __name__ == "__main__":
    main()
