"""Remove the obsolete global-queue drain from a Mongoose text checkpoint.

Writes a separate output, never edits the source. Refuses unknown source shapes
and existing outputs. All bytes outside the exact verb-source block are retained.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path


WAIT = b'''drains = 0;
while ((length(queued_tasks()) > 10) && (drains < 300))
suspend(0);
drains = drains + 1;
endwhile
if (length(queued_tasks()) > 10)
if (cleanup_error == {})
cleanup_error = {E_QUOTA, "PBT recycle queue did not drain", object, {}};
endif
else
try
recycle(object);
except recycle_error (ANY)
if (cleanup_error == {})
cleanup_error = recycle_error;
endif
endtry
endif
'''
DIRECT = b'''"Recycle this fixture without waiting for unrelated queued tasks.";
try
recycle(object);
except recycle_error (ANY)
if (cleanup_error == {})
cleanup_error = recycle_error;
endif
endtry
'''


def repair(data: bytes) -> tuple[bytes, str]:
    newline = b"\r\n" if b"\r\n" in data[:1024] else b"\n"
    old = WAIT.replace(b"\n", newline)
    new = DIRECT.replace(b"\n", newline)
    if data.count(old) != 1:
        raise ValueError("expected exactly one known PBT polling block; source differs or is already fixed")
    start = data.index(old)
    header = data.rfind(newline + b"#", 0, start) + len(newline)
    end = data.find(newline + b"." + newline, start)
    section = data[header:end]
    identity = section.split(newline, 1)[0]
    if end < start or not re.fullmatch(rb"#[0-9]+:[0-9]+", identity):
        raise ValueError("polling block is not inside a text-database verb program")
    if b'"mongoose-pbt-cleanup"' not in section or b'"PBT generated object was not recycled"' not in section:
        raise ValueError("verb does not have the expected PBT cleanup contract")
    return data[:start] + new + data[start + len(old):], identity.decode("ascii")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    original = args.source.read_bytes()
    repaired, verb = repair(original)
    with args.output.open("xb") as out:
        out.write(repaired)
    print(json.dumps({"source": str(args.source), "output": str(args.output), "verb": verb,
                      "source_sha256": hashlib.sha256(original).hexdigest(),
                      "output_sha256": hashlib.sha256(repaired).hexdigest()}, indent=2))


if __name__ == "__main__":
    main()
