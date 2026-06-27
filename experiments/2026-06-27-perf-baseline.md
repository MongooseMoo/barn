# VM Micro-benchmark Baseline — 2026-06-27 (C0)

This is the **canonical performance baseline** for the Barn single-thread VM track.
Every later optimization chunk (C1, C2, …) compares against this file with
`benchstat` so that improvements are reported with statistical significance, not
eyeballed.

## Machine / toolchain

```
goos: windows
goarch: amd64
cpu: AMD Ryzen 9 5950X 16-Core Processor
go version go1.26.0 windows/amd64
GOMAXPROCS: 32 (suffix "-32" on each benchmark name)
```

## Baseline summary (`-count=10`)

benchstat median ± coefficient of variation over 10 runs of
`experiments/perf-baseline-vm-20260627.txt`. (B/op shown here in bytes, converted
from benchstat's Mi/Ki units; raw file has exact byte counts.)

| Workload (BenchmarkVM/…) | ns/op (median) | CV  | B/op       | allocs/op |
|--------------------------|---------------:|----:|-----------:|----------:|
| int_arith_1M             |     59,330,000 |  6% | 16,005,000 | 1,999,740 |
| float_arith_1M           |     68,670,000 |  3% | 16,005,000 | 1,999,762 |
| string_concat_10k        |      1,331,000 | 11% |    608,100 |    19,784 |
| list_append_10k          |      2,225,000 | 15% |  1,391,200 |    29,799 |
| list_index_1M            |    139,200,000 | 15% | 28,022,500 | 3,491,533 |
| builtin_abs_200k         |     30,150,000 |  1% |  6,402,830 |   799,484 |
| tostr_200k               |     57,570,000 |  1% | 16,003,330 |   999,541 |
| nested_1k                |     55,650,000 | 10% | 13,987,100 | 1,747,504 |
| list_iter_1M             |     95,150,000 | 10% | 40,010,200 | 2,999,490 |
| **geomean**              | **29,200,000** |     |  8,482 Ki  |   687,900 |

Note: the higher-CV rows (`list_append_10k` ±15%, `list_index_1M` ±15%) are the
noisiest workloads on this box; treat sub-5% claimed wins there with suspicion and
prefer benchstat's `p`-value verdict over the raw delta.

## Exact reproduction command

```
go test ./vm -run='^$' -bench=BenchmarkVM -benchmem -count=10 | tee experiments/perf-baseline-vm-20260627.txt
```

(Takes ~2 minutes; total `ok barn/vm 126.567s` on the baseline run.)

## How to compare a chunk against this baseline

Use the helper script (PowerShell). It runs the chosen workload(s) at `-count=10`
into a temp file and prints the benchstat delta with significance:

```
pwsh scripts/perf-compare.ps1 BenchmarkVM/int_arith_1M c1-after
# or re-run everything:
pwsh scripts/perf-compare.ps1 BenchmarkVM c1-after
```

The baseline file already contains all nine workloads, so a **filtered** new run
still compares correctly — benchstat matches rows by benchmark name and reports the
shared row(s). When the benchmark set differs (filtered run), benchstat footnotes
that the geomeans are not comparable; read the per-row `vs base` column, not the
geomean, in that case.

Interpretation: `~` in the `vs base` column means the change is indistinguishable
from noise at p < 0.05. A real win must show a negative percentage **with** a
`p=` value below 0.05.

## Scope note

Campaign scope is the **single-thread VM track only**. The scheduler concurrency
harnesses are NOT on `master` and are explicitly out of scope for this baseline
(confirmed in `reports/perf-c0-scout.md`). Concurrency work (the C3 / scheduler
track) is deferred; do not restore or benchmark those harnesses against this file.
