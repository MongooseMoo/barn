"""Replace Mongoose's known root notification program in a separate checkpoint.

Never modifies the input or overwrites an output. Preserves every byte outside
the #1:38 program. Live installation uses @edit; this tool is for offline runs.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path


SOURCE = Path(__file__).resolve().parents[1] / "moo/mongoose/root-loc-notify.moo"
OLD_BODY = b"""{what, where, ?level = 0} = args;
for c in (this.contents)
`c:(verb)(what, where, level + 1) ! ANY => 0';
suspend(0);
endfor"""
COMMENT = re.compile(rb'"(?:[^"\\]|\\.)*";')


def repair(data: bytes) -> bytes:
    newline = b"\r\n" if b"\r\n" in data[:1024] else b"\n"
    header = newline + b"#1:38" + newline
    if data.count(header) != 1:
        raise ValueError("expected exactly one root #1:38 program")
    start = data.index(header) + len(header)
    end = data.find(newline + b"." + newline, start)
    if end == -1:
        raise ValueError("root program terminator is missing")
    lines = data[start:end].split(newline)
    executable = b"\n".join(line.strip() for line in lines
                            if line.strip() and not COMMENT.fullmatch(line.strip()))
    if executable != OLD_BODY:
        raise ValueError("root notification source differs or is already fixed")
    new = SOURCE.read_bytes().replace(b"\r\n", b"\n").rstrip(b"\n")
    return data[:start] + new.replace(b"\n", newline) + data[end:]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    original = args.source.read_bytes()
    repaired = repair(original)
    with args.output.open("xb") as out:
        out.write(repaired)
    print(json.dumps({"source": str(args.source), "output": str(args.output),
                      "verb": "#1:38",
                      "source_sha256": hashlib.sha256(original).hexdigest(),
                      "output_sha256": hashlib.sha256(repaired).hexdigest()}, indent=2))


if __name__ == "__main__":
    main()
