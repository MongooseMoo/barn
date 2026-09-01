#!/usr/bin/env python3
"""Differential micro-benchmark: Barn vs Toast, both in-process, timed inside MOO.

Runs the same workload corpus through two batch lanes with no sockets, logins,
or scheduler round-trips:

  * Toast: WSL emergency mode (``moo -e in.db out.db < script``), one ``;EXPR``
    per line, run as the database wizard.
  * Barn:  ``barn -eval-file`` (Linux cross-build, run inside the same WSL
    distribution so both engines share an OS and allocator story).

Each workload is wrapped as::

    {"K0007", ftime(1), eval("<workload>"), ftime(1)}

so the duration is measured by the engine's own monotonic clock -- process
startup, database load, and output parsing fall out.  ``eval`` returns
``{1, value}``; the two engines' values are compared as a free correctness
check.  A preamble raises ``$server_options`` tick/second limits and calls
``load_server_options()`` on both engines before any timed line.

Usage:
    python scripts/bench_differ.py                     # built-in workloads
    python scripts/bench_differ.py --repeats 7 --out experiments/bench-YYYYMMDD
    python scripts/bench_differ.py --corpus my.txt     # one statement list per
                                                       # line; ``name: code``
                                                       # or bare code; ``##``
                                                       # comments

Output: a markdown table (stdout + ``<out>/report.md``) sorted by Barn/Toast
ratio, raw engine transcripts, and ``<out>/results.json``.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import statistics
import subprocess
import sys
import time
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
TESTDB = Path("C:/Users/Q/code/moo-conformance-tests/src/moo_conformance/_db/Test.db")
TOAST = "/root/src/toaststunt/build-release/moo"
WSL = ["wsl", "-d", "Debian", "-u", "root", "-e", "bash", "-c"]

# Same ten workloads as moo-conformance-tests/bench/bench.py so the two
# harnesses can be cross-checked.
BUILTIN_WORKLOADS = [
    ("noop", "return 0;"),
    ("int_arith_5M", "x = 0; for i in [1..5000000]; x = x + i; endfor; return x;"),
    ("float_arith_5M", "x = 0.0; for i in [1..5000000]; x = x + 1.5; endfor; return x > 0.0;"),
    ("string_concat_50k", 's = ""; for i in [1..50000]; s = s + "x"; endfor; return length(s);'),
    ("list_append_30k", "l = {}; for i in [1..30000]; l = {@l, i}; endfor; return length(l);"),
    (
        "list_index_1M",
        "l = {}; for i in [1..1000]; l = {@l, i}; endfor; "
        "x = 0; for i in [1..1000000]; x = l[1 + (i % 1000)]; endfor; return x;",
    ),
    ("builtin_tostr_1M", "n = 0; for i in [1..1000000]; n = n + length(tostr(i)); endfor; return n;"),
    ("prop_access_1M", "n = #0; x = 0; for i in [1..1000000]; x = typeof(n.name); endfor; return x;"),
    ("builtin_abs_200k", "x = 0; for i in [1..200000]; x = x + abs(-i); endfor; return x;"),
    ("nested_loop_2500x2500", "c = 0; for i in [1..2500]; for j in [1..2500]; c = c + 1; endfor; endfor; return c;"),
]

PREAMBLE = (
    'try add_property($server_options, "fg_ticks", 2000000000, {$server_options.owner, "r"}); '
    "except (ANY) endtry "
    'try add_property($server_options, "fg_seconds", 30000, {$server_options.owner, "r"}); '
    "except (ANY) endtry "
    "return load_server_options();"
)

RESULT_RE = re.compile(r'=> \{"(K\d{4})", ([0-9.eE+-]+), (.*), ([0-9.eE+-]+)\}$')


def moo_str(s: str) -> str:
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def probe(key: int, code: str) -> str:
    return '{"K%04d", ftime(1), eval(%s), ftime(1)}' % (key, moo_str(code))


def to_wsl(p: Path) -> str:
    p = p.resolve()
    return "/mnt/" + p.drive[0].lower() + p.as_posix()[2:]


def sha256(p: Path) -> str:
    h = hashlib.sha256()
    with open(p, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def load_corpus(path: Path) -> list[tuple[str, str]]:
    out = []
    for n, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        line = line.strip()
        if not line or line.startswith("##"):
            continue
        m = re.match(r"^([A-Za-z0-9_.-]+):\s+(.*)$", line)
        if m:
            out.append((m.group(1), m.group(2)))
        else:
            out.append(("L%04d" % n, line))
    return out


def build_barn_linux(dest: Path) -> None:
    env = dict(os.environ, GOOS="linux", GOARCH="amd64", CGO_ENABLED="0")
    subprocess.run(["go", "build", "-o", str(dest), "./cmd/barn"], cwd=REPO, env=env, check=True)


def run_wsl(script: str, timeout: int) -> subprocess.CompletedProcess:
    return subprocess.run(WSL + [script], capture_output=True, text=True, encoding="utf-8",
                          errors="replace", timeout=timeout)


def parse(stdout: str) -> dict[str, tuple[float, str]]:
    """key -> (seconds, value-string). Unparsed/aborted probes are absent."""
    res = {}
    for raw in stdout.splitlines():
        line = re.sub(r"^\(#-?\d+\): ", "", raw.strip())
        m = RESULT_RE.match(line)
        if m:
            res[m.group(1)] = (float(m.group(4)) - float(m.group(2)), m.group(3))
    return res


def git_head() -> str:
    try:
        return subprocess.run(["git", "rev-parse", "--short", "HEAD"], cwd=REPO, capture_output=True,
                              text=True, check=True).stdout.strip()
    except Exception:
        return "unknown"


def fmt_ms(med, mn) -> str:
    return f"{med:.2f} / {mn:.2f}" if med is not None else "-"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--corpus", type=Path, help="workload file (default: built-in bench.py set)")
    ap.add_argument("--repeats", type=int, default=5)
    ap.add_argument("--out", type=Path, default=REPO / ".tmp" / "bench_differ")
    ap.add_argument("--db", type=Path, default=TESTDB)
    ap.add_argument("--barn-linux", type=Path, help="prebuilt linux barn (default: cross-compile now)")
    ap.add_argument("--timeout", type=int, default=1800)
    args = ap.parse_args()

    workloads = load_corpus(args.corpus) if args.corpus else BUILTIN_WORKLOADS
    args.out.mkdir(parents=True, exist_ok=True)
    db_sha = sha256(args.db)

    barn_bin = args.barn_linux or (args.out / "barn_linux")
    if not args.barn_linux:
        print("==> cross-compiling barn for linux/amd64", file=sys.stderr)
        build_barn_linux(barn_bin)
    barn_sha = sha256(barn_bin)

    # Interleave repeats so drift/GC noise spreads across workloads.
    order: list[tuple[str, str, str]] = []  # (key, name, code)
    k = 0
    for _ in range(args.repeats):
        for name, code in workloads:
            order.append(("K%04d" % k, name, code))
            k += 1

    probes = [probe(int(key[1:]), code) for key, _, code in order]
    preamble_line = "eval(%s)" % moo_str(PREAMBLE)

    barn_in = args.out / "barn_in.txt"
    barn_in.write_text(preamble_line + "\n" + "\n".join(probes) + "\n", encoding="utf-8", newline="\n")
    toast_in = args.out / "toast_in.txt"
    toast_in.write_text(";" + preamble_line + "\n" + "".join(";" + p + "\n" for p in probes) + "quit\n",
                        encoding="utf-8", newline="\n")

    db_wsl, barn_wsl = to_wsl(args.db), to_wsl(barn_bin)
    lanes = {
        "toast": (
            f"set -uo pipefail; cp {db_wsl} /tmp/bd_toast.db && "
            f"{TOAST} -e /tmp/bd_toast.db /tmp/bd_toast_out.db < {to_wsl(toast_in)} 2>/tmp/bd_toast.err"
        ),
        "barn": (
            f"set -uo pipefail; cp {db_wsl} /tmp/bd_barn.db && cp {barn_wsl} /tmp/bd_barn && "
            f"chmod +x /tmp/bd_barn && cd /tmp && /tmp/bd_barn -db /tmp/bd_barn.db -log-dir= "
            f"-debug-addr off -eval-file {to_wsl(barn_in)} 2>/tmp/bd_barn.err"
        ),
    }
    results, wall = {}, {}
    for lane, script in lanes.items():
        print(f"==> running {lane} lane ({len(probes)} probes)", file=sys.stderr)
        t0 = time.perf_counter()
        proc = run_wsl(script, args.timeout)
        wall[lane] = time.perf_counter() - t0
        (args.out / f"{lane}_raw.txt").write_text(
            proc.stdout + "\n--- STDERR ---\n" + proc.stderr, encoding="utf-8")
        results[lane] = parse(proc.stdout)
        print(f"    {lane}: {len(results[lane])}/{len(probes)} probes parsed in {wall[lane]:.1f}s",
              file=sys.stderr)

    rows = []
    for name, code in workloads:
        keys = [key for key, n, _ in order if n == name]
        t = [results["toast"][k][0] for k in keys if k in results["toast"]]
        b = [results["barn"][k][0] for k in keys if k in results["barn"]]
        tv = {results["toast"][k][1] for k in keys if k in results["toast"]}
        bv = {results["barn"][k][1] for k in keys if k in results["barn"]}
        row = {
            "name": name, "code": code,
            "toast_ms_median": statistics.median(t) * 1e3 if t else None,
            "toast_ms_min": min(t) * 1e3 if t else None,
            "barn_ms_median": statistics.median(b) * 1e3 if b else None,
            "barn_ms_min": min(b) * 1e3 if b else None,
            "toast_n": len(t), "barn_n": len(b),
            "toast_value": sorted(tv), "barn_value": sorted(bv),
            "values_match": bool(tv) and tv == bv,
        }
        row["ratio"] = (row["barn_ms_median"] / row["toast_ms_median"]
                        if row["toast_ms_median"] and row["barn_ms_median"] else None)
        rows.append(row)
    rows.sort(key=lambda r: -(r["ratio"] or 0))

    hdr = ["workload", "toast ms (med/min)", "barn ms (med/min)", "barn/toast", "n", "values"]
    lines = ["| " + " | ".join(hdr) + " |", "|" + "|".join("---" for _ in hdr) + "|"]
    for r in rows:
        ratio = f"{r['ratio']:.2f}x" if r["ratio"] else "-"
        vals = "match" if r["values_match"] else f"MISMATCH toast={r['toast_value']} barn={r['barn_value']}"
        lines.append(
            f"| {r['name']} | {fmt_ms(r['toast_ms_median'], r['toast_ms_min'])} | "
            f"{fmt_ms(r['barn_ms_median'], r['barn_ms_min'])} | {ratio} | "
            f"{r['toast_n']}/{r['barn_n']} | {vals} |")
    table = "\n".join(lines)

    head = git_head()
    report = "\n".join([
        f"# bench_differ {time.strftime('%Y-%m-%d %H:%M:%S')}",
        "",
        f"- db: `{args.db}` sha256 `{db_sha}`",
        f"- toast: `{TOAST}` (WSL Debian)",
        f"- barn: linux/amd64 cross-build sha256 `{barn_sha}` from `{REPO}` @ {head}",
        f"- repeats: {args.repeats} (interleaved); timing = in-MOO ftime(1) bookends around eval()",
        f"- lane wall clock: toast {wall['toast']:.1f}s, barn {wall['barn']:.1f}s",
        "",
        table,
        "",
    ])
    (args.out / "report.md").write_text(report, encoding="utf-8")
    (args.out / "results.json").write_text(json.dumps({
        "db": str(args.db), "db_sha256": db_sha, "barn_sha256": barn_sha, "git_head": head,
        "repeats": args.repeats, "rows": rows}, indent=2), encoding="utf-8")
    print(report)
    missing = [r["name"] for r in rows if r["toast_n"] < args.repeats or r["barn_n"] < args.repeats]
    if missing:
        print(f"WARNING: incomplete samples for {missing}; see *_raw.txt", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
