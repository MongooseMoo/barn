"""Resolve saved main-thread perf IP samples against one SHA-pinned MOO ELF.

Offline only: no process attachment or signals. Libraries retain unresolved
weights. Addresses follow perf's ip-start+pgoff and ELF PT_LOAD conversion;
captured mmap2 GNU build IDs must match the pinned executable. Flat IP shares
include observation work and are not callgraph or exclusive per-verb timings.
"""
from __future__ import annotations

import argparse
from collections import defaultdict
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess


MMAP = re.compile(r"PERF_RECORD_MMAP2 (\d+)/(\d+): \[(0x[0-9a-f]+)\((0x[0-9a-f]+)\) @ (0x[0-9a-f]+|0) (.*?)\]: ([rwxps-]+) (.+)$")
SAMPLE = re.compile(r"^\s*(\d+)/(\d+)\s+(\d+\.\d+):\s+(\d+) cpu-clock:u:\s+([0-9a-f]+) \((.+)\)$")


def sha(path):
    with Path(path).open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def write_json(path, value):
    with Path(path).open("x", encoding="utf-8") as stream:
        json.dump(value, stream, indent=2)
        stream.write("\n")


def command(argv, output, name, commands, input_text=None):
    row = {"argv": argv}
    commands.append(row)
    environment = dict(os.environ, LC_ALL="C", DEBUGINFOD_URLS="")
    with (output / (name + ".stdout")).open("xb") as stdout, (output / (name + ".stderr")).open("xb") as stderr:
        result = subprocess.run(argv, input=None if input_text is None else input_text.encode(),
                                stdout=stdout, stderr=stderr, env=environment, timeout=60)
    row["exit_code"] = result.returncode
    write_json(output / (name + ".command.json"), row)
    if result.returncode != 0:
        raise ValueError(name + " failed; raw stdout/stderr retained")
    return (output / (name + ".stdout")).read_text()


def load_segments(text):
    segments = []
    for line in text.splitlines():
        words = line.split()
        if words and words[0] == "LOAD":
            segments.append({"offset": int(words[1], 16), "vaddr": int(words[2], 16),
                             "filesz": int(words[4], 16), "executable": "E" in words[6:-1]})
    if not segments:
        raise ValueError("readelf provided no PT_LOAD segments")
    return segments


def build_id(text):
    found = re.findall(r"Build ID:\s*([0-9a-fA-F]+)", text)
    if len(found) != 1:
        raise ValueError("one GNU ELF build ID is required")
    return found[0].lower()


def parse_samples(text):
    """Process mapping records in stream order; never borrow a future mapping."""
    active, mappings, samples = defaultdict(list), [], []
    for line_number, line in enumerate(text.splitlines(), 1):
        if not line.strip() or line.startswith("#"):
            continue
        if "PERF_RECORD_MMAP2" in line:
            match = MMAP.search(line)
            if match is None:
                raise ValueError(f"unsupported mmap2 format at line {line_number}")
            pid, tid, start, length, offset, identity, permissions, dso = match.groups()
            identifier = re.fullmatch(r"<([0-9a-fA-F]+)>", identity)
            mapping = {"index": len(mappings), "pid": int(pid), "tid": int(tid),
                       "start": int(start, 16), "length": int(length, 16),
                       "file_offset": int(offset, 16), "identity": identity,
                       "build_id": identifier[1].lower() if identifier else None,
                       "permissions": permissions, "dso": dso, "line": line_number}
            if mapping["length"] <= 0:
                raise ValueError("empty executable mapping")
            mappings.append(mapping)
            begin, end = mapping["start"], mapping["start"] + mapping["length"]
            # Conservatively discard an overlapping old range, even if partly
            # unaffected. Such samples remain unresolved rather than guessed.
            active[int(pid)] = [old for old in active[int(pid)] if
                                old["start"] + old["length"] <= begin or old["start"] >= end]
            active[int(pid)].append(mapping)
            continue
        match = SAMPLE.fullmatch(line)
        if match is None:
            raise ValueError(f"unsupported perf record/sample at line {line_number}")
        pid, tid, timestamp, period, ip, dso = match.groups()
        sample = {"pid": int(pid), "tid": int(tid), "time": timestamp,
                  "period": int(period), "ip": int(ip, 16), "dso": dso, "line": line_number}
        if sample["period"] <= 0:
            raise ValueError("nonpositive CPU sampling period")
        candidates = [item for item in active[sample["pid"]] if item["dso"] == dso and
                      item["start"] <= sample["ip"] < item["start"] + item["length"]]
        sample["mapping_index"] = candidates[0]["index"] if len(candidates) == 1 else None
        samples.append(sample)
    return mappings, samples


def address(sample, mappings, segments, executable, expected_build_id, main_pid):
    if sample["pid"] != main_pid or sample["tid"] != main_pid:
        return None, "different_thread_or_process"
    if sample["dso"] != executable:
        return None, "library_or_other_dso_not_sha_pinned"
    index = sample["mapping_index"]
    if index is None:
        return None, "no_prior_matching_mapping"
    mapping = mappings[index]
    if mapping["build_id"] != expected_build_id:
        return None, "captured_build_id_missing_or_mismatched"
    if "x" not in mapping["permissions"]:
        return None, "mapping_not_executable"
    file_offset = sample["ip"] - mapping["start"] + mapping["file_offset"]
    matches = [segment for segment in segments if segment["executable"] and
               segment["offset"] <= file_offset < segment["offset"] + segment["filesz"]]
    if len(matches) != 1:
        return None, "file_offset_not_in_unique_executable_pt_load"
    segment = matches[0]
    sample["file_offset"] = file_offset
    return file_offset - segment["offset"] + segment["vaddr"], None


def parse_addr2line(text, addresses):
    lines = text.splitlines()
    if len(lines) != 3 * len(addresses):
        raise ValueError("addr2line result does not match requested address count")
    result = {}
    for index, expected in enumerate(addresses):
        actual, function, location = lines[3 * index:3 * index + 3]
        if int(actual, 16) != expected:
            raise ValueError("addr2line address ordering mismatch")
        result[expected] = {"function": None if function == "??" else function,
                            "source_location": location,
                            "source_line_available": bool(re.search(r":([1-9]\d*)(?: \(discriminator \d+\))?$", location)),
                            "resolution": "GNU addr2line"}
    return result


def nm_functions(text):
    result = []
    for line in text.splitlines():
        match = re.fullmatch(r"([0-9a-fA-F]+) ([0-9a-fA-F]+) ([TtWw]) (.+)", line.strip())
        if match and int(match[2], 16) > 0:
            result.append({"start": int(match[1], 16), "size": int(match[2], 16), "function": match[4]})
    return result


def bounded_symbol(value, symbol, table):
    candidates = [row for row in table if row["start"] <= value < row["start"] + row["size"]]
    names = {row["function"] for row in candidates}
    result = dict(symbol)
    if symbol["function"] is None and len(names) == 1:
        result.update(function=next(iter(names)), resolution="GNU nm bounded symbol size",
                      source_location=None, source_line_available=False)
    if result["function"] not in names:
        result.update(addr2line_function=symbol["function"], function=None,
                      unresolved_reason="no_matching_bounded_function_symbol")
    else:
        result["nm_symbol_bounds"] = [row for row in candidates if row["function"] == result["function"]]
        result["unresolved_reason"] = None
    return result


def summarize(samples):
    denominator = sum(row["period"] for row in samples)
    functions, unresolved = {}, {}
    for sample in samples:
        resolved = sample.get("function") is not None
        destination = functions if resolved else unresolved
        key = sample["function"] if resolved else (sample["dso"], sample["unresolved_reason"])
        if key not in destination:
            destination[key] = ({"function": key} if resolved else {"dso": key[0], "reason": key[1]})
            destination[key].update(samples=0, period=0)
        destination[key]["samples"] += 1
        destination[key]["period"] += sample["period"]
    for rows in (functions, unresolved):
        for row in rows.values():
            row["period_share_percent"] = 100 * row["period"] / denominator if denominator else 0
            row["sample_share_percent"] = 100 * row["samples"] / len(samples) if samples else 0
    return {"sample_count": len(samples), "sample_period_sum": denominator,
            "resolved_samples": sum(row["samples"] for row in functions.values()),
            "unresolved_samples": sum(row["samples"] for row in unresolved.values()),
            "functions": sorted(functions.values(), key=lambda row: (-row["period"], row["function"])),
            "unresolved": sorted(unresolved.values(), key=lambda row: (-row["period"], row["dso"], row["reason"]))}


def run(native_directory, output, executable=None):
    native_directory, output = Path(native_directory).resolve(), Path(output).absolute()
    output.mkdir(exist_ok=False)
    report = {"schema_version": 1, "status": "failed", "commands": [],
              "scope": "offline flat main-thread userspace IP samples; observer work included",
              "interpretation": "sample period weighted function estimates, not exact or exclusive per-verb CPU durations",
              "callgraph": "unavailable", "shared_library_symbolization": "not attempted: capture SHA identities not supplied"}
    try:
        manifest_path, data = native_directory / "native.json", native_directory / "perf.data"
        original_manifest = json.loads(manifest_path.read_text())
        expected = original_manifest["target"]
        elf = Path(executable or expected["executable"]).resolve(strict=True)
        before = {"native_manifest": sha(manifest_path), "perf_data": sha(data), "executable": sha(elf)}
        report.update(input_directory=str(native_directory), executable=str(elf), captured_executable=expected,
                      input_hashes=before, postprocessor_sha256=sha(__file__))
        if original_manifest["status"] != "collected" or original_manifest.get("callgraph") != "none":
            raise ValueError("a valid collected flat native capture is required")
        if before["executable"] != expected["sha256"]:
            raise ValueError("actual ELF SHA differs from capture target SHA")
        if before["perf_data"] != original_manifest["data_sha256"]:
            raise ValueError("saved perf data SHA differs from capture manifest")
        perf = Path(original_manifest["capability"]["perf"]).resolve(strict=True)
        if sha(perf) != original_manifest["capability"]["perf_sha256"]:
            raise ValueError("perf executable SHA changed since capture")
        tools = {name: str(Path(shutil.which(name)).resolve(strict=True)) for name in ("readelf", "addr2line", "nm")}
        report["tools"] = {name: {"path": value, "sha256": sha(value)} for name, value in tools.items()}
        for name, tool in tools.items():
            command([tool, "--version"], output, name + "-version", report["commands"])
        text = command([str(perf), "script", "--ns", "--show-mmap-events", "-F", "pid,tid,time,event,ip,dso,period",
                        "--input=" + str(data)], output, "perf-mappings-samples", report["commands"])
        mappings, samples = parse_samples(text)
        if len(samples) != original_manifest["sample_count"] or not samples:
            raise ValueError("sample denominator differs from saved capture manifest")
        segments = load_segments(command([tools["readelf"], "-lW", str(elf)], output, "elf-program-headers", report["commands"]))
        identifier = build_id(command([tools["readelf"], "-nW", str(elf)], output, "elf-notes", report["commands"]))
        report.update(elf_build_id=identifier, elf_load_segments=segments)
        for sample in samples:
            value, reason = address(sample, mappings, segments, expected["executable"], identifier, expected["main_tid"])
            sample.update(elf_address=value, function=None, unresolved_reason=reason)
        addresses = sorted({row["elf_address"] for row in samples if row["elf_address"] is not None})
        symbols = {}
        if addresses:
            requests = "".join(hex(value) + "\n" for value in addresses)
            with (output / "addr2line-addresses.txt").open("x") as stream:
                stream.write(requests)
            symbols = parse_addr2line(command([tools["addr2line"], "-a", "-C", "-f", "-e", str(elf)],
                                       output, "addr2line", report["commands"], requests), addresses)
        table = nm_functions(command([tools["nm"], "-n", "-S", "-C", str(elf)], output, "nm-functions", report["commands"]))
        for sample in samples:
            value = sample["elf_address"]
            if value is None:
                continue
            sample.update(bounded_symbol(value, symbols[value], table))
        report.update(summarize(samples))
        report["nm_bounded_samples"] = sum(bool(row.get("nm_symbol_bounds")) for row in samples)
        report["known_source_line_samples"] = sum(bool(row.get("source_line_available")) for row in samples)
        report["captured_dso_mappings"] = mappings
        after = {"native_manifest": sha(manifest_path), "perf_data": sha(data), "executable": sha(elf)}
        report["final_input_hashes"] = after
        if before != after or sha(perf) != original_manifest["capability"]["perf_sha256"] or any(
                sha(tools[name]) != row["sha256"] for name, row in report["tools"].items()):
            raise ValueError("offline input or tool changed during symbolization")
        write_json(output / "samples.json", samples)
        report["status"] = "complete"
    except Exception as error:
        report.update(status="failed", error_type=type(error).__name__, reason=str(error))
    write_json(output / "symbols.json", report)
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("native_directory", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--executable", type=Path, help="optional relocated exact-SHA copy")
    options = parser.parse_args()
    result = run(options.native_directory, options.output, options.executable)
    print(json.dumps({name: result[name] for name in ("status", "sample_count", "resolved_samples", "unresolved_samples", "reason") if name in result}, indent=2))
    raise SystemExit(0 if result["status"] == "complete" else 1)
