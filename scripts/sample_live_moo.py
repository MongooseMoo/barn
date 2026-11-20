#!/usr/bin/env python3
"""Observe an executing MOO stack without attaching or stopping the server.

Pinned to an explicitly identified ELF64 ToastStunt build. Metadata reads are
best-effort, not atomic snapshots or CPU profiles. See docs/live-moo-sampling.md.
"""
from __future__ import annotations

import argparse
from collections import Counter
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import statistics
import struct
import subprocess
import sys
import tempfile
import time


PINNED_SHA = "1d9864c792b9ce3ba92f23f24b53c8c6985cd36ad1bb928f4aa8be121f322c2c"
PINNED_REVISION = "d80a17ca1d51f91a1938a9e4c1e88d52457deac7"
DEFAULT_SOURCE = "/home/mongoose/toaststunt-d80a17c"
SYMBOLS = {
    "stack": "_ZL11activ_stack", "top": "_ZL15top_activ_stack",
    "running": "_ZL22interpreter_is_running", "task": "current_task_id",
}
# Replaced only when this complete module is sent over SSH stdin.
REMOTE_REQUEST = None


def service_state():
    result = subprocess.run(
        ["systemctl", "show", "mongoose.service",
         "--property=MainPID,NRestarts,ActiveState,SubState"],
        check=True, capture_output=True, text=True, timeout=5)
    return dict(line.split("=", 1) for line in result.stdout.splitlines() if "=" in line)


def process_identity(pid):
    proc = Path("/proc") / str(pid)
    fields = (proc / "stat").read_text().rsplit(")", 1)[1].split()
    binary = (proc / "exe").stat()
    return (int(fields[19]), binary.st_dev, binary.st_ino), {
        "main_cpu_ticks": int(fields[11]) + int(fields[12]), "native_state": fields[0],
    }


def binary_sha(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1048576), b""):
            digest.update(chunk)
    return digest.hexdigest()


def elf_bias(binary, maps, device, inode):
    with binary.open("rb") as source:
        header = source.read(64)
        if header[:6] != b"\x7fELF\x02\x01" or len(header) != 64:
            raise ValueError("only little-endian ELF64 executables are supported")
        kind, machine = struct.unpack_from("<HH", header, 16)
        if kind not in (2, 3) or machine != 62:
            raise ValueError("only x86-64 ELF executables are supported")
        if kind == 2:
            return 0
        phoff = struct.unpack_from("<Q", header, 32)[0]
        entry_size, count = struct.unpack_from("<HH", header, 54)
        if entry_size != 56 or not 1 <= count <= 256:
            raise ValueError("unexpected ELF program headers")
        load_vaddr = None
        for index in range(count):
            source.seek(phoff + index * entry_size)
            entry = source.read(56)
            if len(entry) != 56:
                raise ValueError("truncated ELF program headers")
            ptype, _, offset, vaddr = struct.unpack_from("<IIQQ", entry)
            if ptype == 1 and offset == 0:
                load_vaddr = vaddr
                break
        if load_vaddr is None:
            raise ValueError("ELF has no offset-zero load segment")
    page = os.sysconf("SC_PAGE_SIZE")
    for line in maps.splitlines():
        fields = line.split(None, 5)
        major, minor = (int(part, 16) for part in fields[3].split(":"))
        if (int(fields[2], 16) == 0 and int(fields[4]) == inode
                and os.makedev(major, minor) == device):
            return int(fields[0].split("-")[0], 16) - (load_vaddr // page * page)
    raise ValueError("cannot locate executable load mapping")


def symbol_addresses(binary, bias, layout):
    output = subprocess.run(["readelf", "--wide", "--syms", str(binary)], check=True,
                            capture_output=True, text=True, timeout=5).stdout
    found = {}
    sizes = {"stack": layout["pointer_size"], "top": 4,
             "running": layout["int_size"], "task": layout["int_size"]}
    for line in output.splitlines():
        parts = line.split()
        if len(parts) != 8 or parts[3] != "OBJECT" or parts[6] == "UND":
            continue
        for key, name in SYMBOLS.items():
            if parts[7] == name:
                if int(parts[2]) != sizes[key]:
                    raise ValueError("interpreter symbol size differs from compiled layout")
                value = int(parts[1], 16) + bias
                if key in found and found[key] != value:
                    raise ValueError("ambiguous interpreter symbol")
                found[key] = value
    if found.keys() != SYMBOLS.keys():
        raise ValueError("required interpreter symbols are missing")
    return found


def compile_layout(source_dir, revision, layout_source):
    source = Path(source_dir)
    # Root may read a service-user-owned checkout. Trust this exact explicitly
    # supplied path for these commands only; do not change global Git settings.
    git = ["git", "-c", "safe.directory=" + str(source), "-C", str(source)]
    actual = subprocess.run(git + ["rev-parse", "HEAD"],
                            check=True, capture_output=True, text=True, timeout=5).stdout.strip()
    dirty = subprocess.run(git + ["status", "--porcelain",
                            "--untracked-files=no"], check=True, capture_output=True,
                           text=True, timeout=5).stdout
    if actual != revision or dirty:
        raise ValueError("deployed source must be clean and match the pinned revision")
    build = source / "build"
    config = build / "config.h"
    cache = (build / "CMakeCache.txt").read_text()
    if not re.search(r"^CMAKE_CXX_FLAGS:STRING=\s*$", cache, re.MULTILINE):
        raise ValueError("custom compiler ABI flags require an explicit layout implementation")
    with tempfile.TemporaryDirectory(prefix="moo-layout-") as temporary:
        helper = Path(temporary) / "layout"
        subprocess.run(["g++", "-std=c++17", "-I", str(build), "-I",
                        str(source / "src/include"), "-x", "c++", "-", "-o", str(helper)],
                       input=layout_source, check=True, capture_output=True, text=True, timeout=30)
        layout = json.loads(subprocess.run([str(helper)], check=True, capture_output=True,
                                          text=True, timeout=5).stdout)
    if (layout["version"] != 1 or layout["pointer_size"] != 8
            or layout["int_size"] != 4 or layout["num_size"] not in (4, 8)
            or not 64 <= layout["activation_size"] <= 512):
        raise ValueError("unsupported activation ABI")
    headers = {name: binary_sha(source / "src/include" / name)
               for name in ("execute.h", "structures.h", "options.h")}
    return layout, {"source_revision": actual, "config_sha256": binary_sha(config),
                    "cmake_cache_sha256": binary_sha(build / "CMakeCache.txt"),
                    "header_sha256": headers,
                    "layout_source_sha256": hashlib.sha256(layout_source.encode()).hexdigest()}


class Changed(RuntimeError):
    pass


class StackReader:
    def __init__(self, read, layout, addresses, max_frames=16):
        self.read, self.layout, self.addresses = read, layout, addresses
        self.max_frames = max_frames

    def number(self, address, size, signed=False):
        data = self.read(address, size)
        if len(data) != size:
            raise OSError("short process-memory read")
        return int.from_bytes(data, "little", signed=signed)

    def context(self):
        a, layout = self.addresses, self.layout
        running = self.number(a["running"], layout["int_size"], True)
        if running not in (0, 1):
            raise Changed("invalid interpreter state")
        if not running:
            return (0,)
        return (1, self.number(a["stack"], layout["pointer_size"]),
                self.number(a["top"], 4), self.number(a["task"], layout["int_size"], True))

    def frame(self, data):
        layout, offsets = self.layout, self.layout["offsets"]
        def field(name, size, signed=False, extra=0):
            offset = offsets[name] + extra
            part = data[offset:offset + size]
            if len(part) != size:
                raise ValueError("activation layout exceeds frame")
            return int.from_bytes(part, "little", signed=signed)
        kind = field("vloc", 4, extra=layout["var_type"])
        if kind not in (layout["type_obj"], layout["type_anon"]):
            raise Changed("invalid definition location")
        return {
            "program": field("prog", layout["pointer_size"]),
            "definer_kind": kind,
            "definer_value": field("vloc", layout["num_size"], True, layout["var_value"]),
            "receiver": field("recv", layout["num_size"], True),
            "player": field("player", layout["num_size"], True),
            "programmer": field("progr", layout["num_size"], True),
            "verb_pointer": field("verb", layout["pointer_size"]),
            "names_pointer": field("verbname", layout["pointer_size"]),
            "saved_error_pc": field("error_pc", 4),
            "builtin_continuation": field("bi_func_pc", 1),
        }

    def name(self, address):
        if address < 4096:
            raise Changed("invalid verb name pointer")
        # Respect page boundaries; never demand bytes past a valid terminator.
        raw = bytearray()
        page = 4096
        while len(raw) < 256:
            count = min(256 - len(raw), page - ((address + len(raw)) % page))
            part = self.read(address + len(raw), count)
            if not part:
                raise OSError("empty verb name read")
            stop = part.find(b"\0")
            raw.extend(part if stop < 0 else part[:stop])
            if stop >= 0:
                break
        else:
            raise Changed("verb name exceeds metadata bound")
        if not raw or any(byte < 32 or byte == 127 for byte in raw):
            raise Changed("invalid verb name metadata")
        return raw.decode("utf-8", errors="replace")

    def frames(self, context):
        _, address, top, _ = context
        if address < 4096 or top > 1023:
            raise Changed("invalid activation stack bounds")
        first = max(0, top + 1 - self.max_frames)
        size = self.layout["activation_size"]
        data = self.read(address + first * size, (top + 1 - first) * size)
        if len(data) != (top + 1 - first) * size:
            raise OSError("short activation stack read")
        return [self.frame(data[i:i + size]) for i in range(0, len(data), size)][::-1]

    def observe(self):
        # Repeated identity/name checks reject observed changes, not all possible
        # races. The target never stops; there is no atomic snapshot guarantee.
        start = time.monotonic()
        try:
            before = self.context()
            if not before[0]:
                if before != self.context():
                    raise Changed("interpreter started during observation")
                return {"state": "outside_interpreter"}
            frames = self.frames(before)
            names = [(self.name(f["verb_pointer"]), self.name(f["names_pointer"])) for f in frames]
            if before != self.context() or frames != self.frames(before):
                raise Changed("active stack changed during observation")
            if names != [(self.name(f["verb_pointer"]), self.name(f["names_pointer"])) for f in frames]:
                raise Changed("verb name storage changed during observation")
            if before != self.context():
                raise Changed("active task changed during observation")
            result = []
            for frame, (called, defined) in zip(frames, names):
                if not frame["program"]:
                    raise Changed("activation has no program")
                definer = ("#" + str(frame["definer_value"])) if frame["definer_kind"] == self.layout["type_obj"] else "anonymous"
                result.append({"definer": definer, "called_verb": called, "defined_names": defined,
                               "receiver": "#" + str(frame["receiver"]),
                               "player": "#" + str(frame["player"]),
                               "programmer": "#" + str(frame["programmer"]),
                               "saved_error_pc": frame["saved_error_pc"], "source_line": None})
            return {"state": "active_observation", "consistency": "best_effort_rechecked",
                    "task_id": before[3], "stack_depth": before[2] + 1,
                    "frames_omitted": max(0, before[2] + 1 - len(result)), "frames": result,
                    "read_duration_s": time.monotonic() - start}
        except Changed as exc:
            return {"state": "unstable", "reason": str(exc)}
        except OSError as exc:
            return {"state": "unreadable", "error_type": type(exc).__name__, "errno": exc.errno}


def summarize(samples):
    states = Counter(row["state"] for row in samples)
    tops = Counter()
    durations = []
    for row in samples:
        if row["state"] == "active_observation":
            frame = row["frames"][0]
            tops[(frame["definer"], frame["called_verb"], frame["receiver"])] += 1
            durations.append(row["read_duration_s"])
    return {"sample_states": dict(states),
            "top_observed_verbs": [{"definer": d, "verb": v, "receiver": r, "observations": n}
                                   for (d, v, r), n in tops.most_common(20)],
            "median_active_read_s": statistics.median(durations) if durations else None,
            "scope": "periodic best-effort executing-stack observations; counts are not CPU percentages",
            "source_line": "unknown: interpreter-local PC is not continuously published"}


def collect(request):
    service = service_state()
    pid = int(service["MainPID"])
    if pid <= 0:
        raise RuntimeError("service has no live main process")
    identity, _ = process_identity(pid)
    proc = Path("/proc") / str(pid)
    binary = proc / "exe"
    actual_sha = binary_sha(binary)
    if actual_sha != request["expected_sha"]:
        raise ValueError("live executable differs from the explicitly pinned SHA-256")
    layout, provenance = compile_layout(request["source_dir"], request["revision"], request["layout_source"])
    bias = elf_bias(binary, (proc / "maps").read_text(), identity[1], identity[2])
    addresses = symbol_addresses(binary, bias, layout)
    samples = []
    # O_RDONLY plus pread: no ptrace attachment, signal, target write, or call.
    fd = os.open(proc / "mem", os.O_RDONLY)
    byte_count = 0
    def read(address, size):
        nonlocal byte_count
        result = os.pread(fd, size, address)
        byte_count += len(result)
        return result
    reader = StackReader(read, layout, addresses, request["max_frames"])
    count = int(request["duration"] / request["interval"])
    started = time.monotonic()
    cpu_start = time.process_time()
    terminal_error = None
    try:
        for index in range(count):
            expected = started + index * request["interval"]
            delay = expected - time.monotonic()
            if delay > 0:
                time.sleep(delay)
            current, cpu = process_identity(pid)
            if current != identity:
                samples.append({"state": "process_changed"})
                break
            row = reader.observe()
            if process_identity(pid)[0] != identity:
                row = {"state": "process_changed"}
            row.update(cpu)
            row["utc"] = datetime.datetime.now(datetime.timezone.utc).isoformat()
            samples.append(row)
            if row["state"] == "process_changed":
                break
    except OSError as exc:
        # Preserve observations already obtained if the target exits or a
        # procfs identity check fails during collection.
        terminal_error = {"error_type": type(exc).__name__, "errno": exc.errno}
    finally:
        os.close(fd)
    reader_cpu = time.process_time() - cpu_start
    sample_window = time.monotonic() - started
    try:
        after = service_state()
        stable_process = (int(after["MainPID"]) == pid and process_identity(pid)[0] == identity
                          and after["NRestarts"] == service["NRestarts"])
    except (OSError, subprocess.SubprocessError) as exc:
        after = {"error_type": type(exc).__name__}
        stable_process = False
    summary = summarize(samples)
    unknown_only = not summary["top_observed_verbs"] and any(
        state in summary["sample_states"] for state in ("unstable", "unreadable"))
    state = "capture_complete"
    if not stable_process:
        state = "process_changed"
    elif terminal_error or unknown_only or "unreadable" in summary["sample_states"]:
        state = "capture_partial"
    return {"protocol_version": 1, "state": state,
            "pid": pid, "start_ticks": identity[0], "service": service, "service_after": after,
            "executable_sha256": actual_sha,
            "layout": layout, "provenance": provenance, "samples": samples,
            "summary": summary, "clock_ticks": os.sysconf("SC_CLK_TCK"),
            "reader_cpu_s": reader_cpu, "memory_bytes_read": byte_count,
            "sample_window_s": sample_window, "terminal_error": terminal_error,
            "requested_duration_s": request["duration"], "interval_s": request["interval"],
            "max_frames": request["max_frames"],
            "limitations": ["reads are not atomic; rechecks cannot exclude ABA races",
                            "top saved PC can be stale; no executing source-line claim",
                            "native builtin CPU is not distinguished from its enclosing MOO verb",
                            "anonymous definers are not uniquely identified"]}


def main():
    if REMOTE_REQUEST is not None:
        try:
            record = collect(REMOTE_REQUEST)
        except Exception as exc:
            record = {"state": "capture_failed", "error_type": type(exc).__name__, "reason": str(exc)}
            if isinstance(exc, subprocess.CalledProcessError):
                record["native_returncode"] = exc.returncode
                record["native_stderr"] = (exc.stderr or "")[-3000:]
        print(json.dumps(record))
        return 0 if record["state"] == "capture_complete" else 1
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="root@mongoose.world")
    parser.add_argument("--duration", type=float, default=5.0)
    parser.add_argument("--interval", type=float, default=0.05)
    parser.add_argument("--max-frames", type=int, default=16)
    parser.add_argument("--source-dir", default=DEFAULT_SOURCE)
    parser.add_argument("--source-revision", default=PINNED_REVISION)
    parser.add_argument("--expected-exe-sha256", default=PINNED_SHA)
    parser.add_argument("--ssh-exe")
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9_.-]+@[A-Za-z0-9_.-]+", args.host):
        parser.error("host must be user@hostname")
    if (not 0.01 <= args.interval <= 1 or not args.interval <= args.duration <= 30
            or not 1 <= args.max_frames <= 32):
        parser.error("require 0.01..1s interval, interval..30s duration, and 1..32 frames")
    if not re.fullmatch(r"[a-fA-F0-9]{64}", args.expected_exe_sha256):
        parser.error("expected executable SHA-256 must have 64 hexadecimal digits")
    if not re.fullmatch(r"[a-fA-F0-9]{40}", args.source_revision):
        parser.error("source revision must be a full 40-digit commit")
    ssh = args.ssh_exe or shutil.which("ssh")
    if ssh is None and sys.platform == "win32" and sys.maxsize < 2**32:
        candidate = Path(os.environ.get("WINDIR", "C:/Windows")) / "Sysnative/OpenSSH/ssh.exe"
        if candidate.is_file():
            ssh = str(candidate)
    if ssh is None:
        parser.error("SSH unavailable; supply --ssh-exe")
    request = {"duration": args.duration, "interval": args.interval, "max_frames": args.max_frames,
               "source_dir": args.source_dir, "revision": args.source_revision.lower(),
               "expected_sha": args.expected_exe_sha256.lower(),
               "layout_source": Path(__file__).with_name("live_moo_layout.cc").read_text(encoding="utf-8")}
    source = Path(__file__).read_text(encoding="utf-8").replace("REMOTE_REQUEST = None", "REMOTE_REQUEST = " + repr(request), 1)
    args.out.parent.mkdir(parents=True, exist_ok=True)
    with args.out.open("x", encoding="utf-8") as output:
        json.dump({"state": "capture_started", "host": args.host}, output)
    record = {"host": args.host, "collector_sha256": hashlib.sha256(source.encode()).hexdigest()}
    try:
        result = subprocess.run([ssh, "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
                                 args.host, "python3 -"], input=source.encode(), capture_output=True,
                                timeout=args.duration + 65)
        record["remote_exit"] = result.returncode
        record["raw_stdout"] = result.stdout.decode(errors="replace")
        record["raw_stderr"] = result.stderr.decode(errors="replace")
        record["capture"] = json.loads(record["raw_stdout"])
        record["state"] = record["capture"]["state"]
        if record["state"] == "capture_complete" and (
                result.returncode != 0 or record["capture"].get("protocol_version") != 1):
            record["state"] = "capture_partial"
    except Exception as exc:
        record["state"] = "capture_failed"
        record["error_type"] = type(exc).__name__
    args.out.write_text(json.dumps(record, indent=2) + "\n", encoding="utf-8")
    brief = {"state": record["state"], "evidence": str(args.out.resolve())}
    capture = record.get("capture", {})
    brief.update(capture.get("summary", {}))
    if record["state"] != "capture_complete":
        brief.update({key: capture[key] for key in ("error_type", "reason", "native_stderr") if key in capture})
    print(json.dumps(brief, indent=2))
    return 0 if record["state"] == "capture_complete" else 1


if __name__ == "__main__":
    raise SystemExit(main())
