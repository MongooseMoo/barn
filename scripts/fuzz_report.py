"""Summarize Go fuzz output without interpreting or executing corpus contents."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import shlex


INLINE_LIMIT = 8000
TAIL_LIMIT = 12000
MODULE = "github.com/MongooseMoo/barn/"


def report(log, package, target, commit, campaign_outcome="failure"):
    # gh --log-failed fixtures contain multiple jobs; a live tee log has no prefix.
    lines = []
    for line in log.splitlines():
        match = re.match(r"([^\t]+)\t[^\t]+\t\d{4}-\d\d-\d\dT\S+ (.*)", line)
        if match:
            if match[1] != target:
                continue
            line = match[2]
        lines.append(line)
    text = "\n".join(lines)
    signal = re.search(r"^.*(?:signal:|terminated by unexpected signal|Killed\s+).*?$", text, re.M)
    panic = re.search(r"^.*(?:panic:|fatal error:).*?$", text, re.M)
    hang = re.search(r"^.*fuzzing process hung or terminated unexpectedly:.*$", text, re.M)
    check = re.search(r"^\s*\S+\.go:\d+: .+$", text, re.M)
    saved = re.search(r"Failing input written to (testdata/fuzz/[^\s]+)", text)
    setup = re.search(r"^.*\[(?:build|setup) failed\].*$", text, re.M)
    # scripts/fuzz_differential.py's report: the first diverging program.
    divergence = re.search(r"^(## divergence \(\d+\))\n\n### \S+ `([^`]+)`\n\n```moo\n(.*?)\n```\n\n"
                           r"(- toast: [^\n]*)\n(- barn: [^\n]*)", text, re.M | re.S)
    # A recorded input is a finding, even when its worker died from a signal.
    # Program sources in a differential report are not Go output, so it is
    # classified before anything that could match their text.
    if divergence and campaign_outcome == "failure":
        kind, error = "divergence", None
    elif not saved and (campaign_outcome != "failure" or signal or setup):
        kind, error = "infrastructure", signal
    elif panic:
        kind, error = "panic", panic
    elif saved and signal:
        kind, error = "crash signal", signal
    elif hang:
        kind, error = "hang", hang
    elif saved or check:
        kind, error = "failed check", check
    else:
        kind, error = "infrastructure", None
    if error:
        first_error = error[0].strip()
    elif kind == "divergence":
        # The oracle is built in the job, so the issue names that binary.
        toast = re.search(r"^- toast binary: .*$", text, re.M)
        first_error = "\n".join([divergence[1], divergence[4], divergence[5]] + ([toast[0]] if toast else []))
    else:
        first_error = check[0].strip() if setup and check else next((line.strip() for line in lines if re.search(
            r"(?:error:|Error:|FAIL|build failed|setup failed|cannot |no required module)", line)),
            "The job failed outside the fuzz command; inspect the failed step.")

    frames = []
    for index, line in enumerate(lines):
        if line.strip().startswith(MODULE):
            # Strip argument values (addresses change between runs), keeping receivers.
            function = re.sub(r"\([^()]*\)$", "", line.strip())
            location = lines[index + 1].strip() if index + 1 < len(lines) else ""
            location = re.sub(r"^.*?/barn/barn/", "", location).split(" +0x")[0]
            frames.append({"function": function, "location": location})
            if len(frames) == 5:
                break
    top = frames[0]["function"] if frames else "unavailable"
    input_path, input_text, repro = None, None, None
    if kind == "divergence":
        # The first diverging corpus file stands in for the top frame.
        top, input_path, input_text = divergence[2], divergence[2], divergence[3][:INLINE_LIMIT]
    signature = hashlib.sha256(f"{target}|{kind}|{top}".encode()).hexdigest()
    if saved:
        relative = Path(saved[1])
        # Only read the corpus file named by Go, inside this target's directory.
        if relative.parent == Path("testdata/fuzz") / target:
            input_path = (Path(package) / relative).as_posix()
            try:
                with Path(input_path).open("rb") as stream:
                    data = stream.read(INLINE_LIMIT + 1)
                input_text = data[:INLINE_LIMIT].decode("utf-8", errors="replace")
                if len(data) > INLINE_LIMIT:
                    input_text += "\n[truncated]"
            except OSError as exc:
                input_text = f"Input unavailable: {exc.strerror}"
            repro = (f"timeout --kill-after=5s 60s go test {shlex.quote(package)} "
                     f"-run '{target}/{relative.name}'")
    version = re.search(r"^go version .+$", text, re.M)
    return {
        "kind": kind, "first_error": first_error, "barn_frames": frames,
        "signature": signature, "input_path": input_path, "input_text": input_text,
        "repro": repro, "commit": commit,
        "go_version": version[0] if version else "unavailable",
        "log_tail": "\n".join(lines[-80:])[-TAIL_LIMIT:],
    }


def summarize(logs, package, target, commit, campaign_outcome="failure"):
    """One report per distinct signature, in first-seen order, with its count."""
    distinct = {}
    for log in logs or [""]:
        result = report(log, package, target, commit, campaign_outcome)
        first = distinct.setdefault(result["signature"], {**result, "count": 0})
        first["count"] += 1
    return list(distinct.values())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    # One log per failed campaign of the night, in run order.
    parser.add_argument("--log", action="append", default=[], type=Path)
    parser.add_argument("--package", required=True)
    parser.add_argument("--target", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--campaign-outcome", required=True)
    args = parser.parse_args()
    logs = [log.read_text(errors="replace") for log in args.log if log.exists()]
    print(json.dumps(summarize(logs, args.package, args.target, args.commit, args.campaign_outcome)))


if __name__ == "__main__":
    main()
