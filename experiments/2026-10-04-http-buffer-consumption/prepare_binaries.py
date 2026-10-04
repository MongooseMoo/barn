"""Freeze before/after HTTP buffer-consumption benchmark binaries in WSL."""

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
    source = Path("builtins/network.go").resolve()
    baseline = subprocess.run(
        ["git", "show", f"{args.baseline}:builtins/network.go"],
        check=True, capture_output=True, text=True,
    ).stdout
    for variant, content in (("baseline", baseline), ("candidate", source.read_text())):
        frozen = output / f"{variant}.go"
        frozen.write_text(content)
        overlay = output / f"{variant}.json"
        overlay.write_text(json.dumps({"Replace": {str(source): str(frozen)}}))
        binary = output / f"{variant}.test"
        subprocess.run(["go", "test", "-c", f"-overlay={overlay}", "./builtins", "-o", str(binary)], check=True)
        print(variant, hashlib.sha256(binary.read_bytes()).hexdigest(), flush=True)
    print("test-source", hashlib.sha256(Path("builtins/http_buffer_consumption_test.go").read_bytes()).hexdigest())


if __name__ == "__main__":
    main()
