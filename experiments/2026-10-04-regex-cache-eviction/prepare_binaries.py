"""Freeze timed and separately instrumented compile-count binaries in WSL."""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    source_path = Path("builtins/regexcache.go").resolve()
    test_path = Path("builtins/regexcache_eviction_test.go").resolve()
    archive = Path(__file__).resolve().parent
    test_source = archive / "frozen-tests.go.txt"
    baseline = subprocess.run(
        ["git", "show", f"{args.baseline}:builtins/regexcache.go"],
        check=True, capture_output=True, text=True,
    ).stdout
    candidate = (archive / "clock-candidate.go.txt").read_text()
    anchor = "func compileMOOPattern(pattern string, caseSensitive, anchored bool) regexpCacheEntry {"
    for variant, source in (("baseline", baseline), ("candidate", candidate)):
        for instrumented in (False, True):
            name = variant + ("-counts" if instrumented else "")
            text = source
            if instrumented:
                if '"sync/atomic"' not in text:
                    text = text.replace('"sync"', '"sync"\n\t"sync/atomic"', 1)
                if text.count(anchor) != 1:
                    raise ValueError("compile-count injection anchor changed")
                declarations = ("var regexpProbeCounter atomic.Uint64\n"
                                "func init() { regexpCompileCountProbe = regexpProbeCounter.Load }\n\n")
                text = text.replace(anchor, declarations + anchor + "\n\tregexpProbeCounter.Add(1)", 1)
            frozen = output / f"{name}.go"
            frozen.write_text(text)
            overlay = output / f"{name}.json"
            overlay.write_text(json.dumps({"Replace": {
                str(source_path): str(frozen), str(test_path): str(test_source),
            }}))
            binary = output / f"{name}.test"
            subprocess.run(["go", "test", "-c", f"-overlay={overlay}", "./builtins", "-o", str(binary)], check=True)
            print(name, hashlib.sha256(binary.read_bytes()).hexdigest(), flush=True)
    print("test-source", hashlib.sha256(test_source.read_bytes()).hexdigest())


if __name__ == "__main__":
    main()
