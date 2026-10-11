# WASM session state and native builtin timing

The initial three-sample, unpinned native benchmarks showed median increases of
1.03% for `length` and 5.42% for `strsub` versus the pre-refactor source. Those
samples did not establish the cause or a statistically reliable regression.

This follow-up separates the descriptor refactor, the WASM addition, and a small
change to how the session owns WASM state. It measures the existing
`BenchmarkBuiltinAdmission`; no guest executes in these benchmarks.

## Revisions and environment

- `baseline`: `d6d396e2`, before either PR. The benchmark file from `8013f218`
  was copied unchanged into this worktree.
- `refactor`: `8013f218`, PR #462, without WASM.
- `current`: `6e4e641d`, the first published head of PR #464.
- `cold`: this change, with the WASM mutex, manager and closed flag behind a
  private pointer. Source blob IDs are
  `fb5eb91ef325e904d315be9cfe1db53ef1f43089` (`session_runtime.go`) and
  `2a4ff8fe0c56c9263282dfaa6fadc7092afff196` (`wasm.go`).

Host: Windows amd64, AMD Ryzen 9 5950X, Go 1.27.1. Every benchmark binary was
built before measurement. Each set uses ten rounds of separate test processes,
one one-second sample per row per process. Revision order reverses on alternate
rounds; binaries run sequentially. `GOGC=100`, `GOMEMLIMIT=off`. CPU frequency
was not locked and other host activity was not controlled.

`three-way-*` and `cold-layout-*` use process affinity mask 16 (logical CPU 4)
and `GOMAXPROCS=1`. `normal-holdout-*` is an independent run with
`GOMAXPROCS=32` and ordinary scheduling, matching the original parallelism.
Raw Go output is preserved in the `.txt` files; their whitespace is data.

## Diagnosis and change

The descriptor refactor alone did not show a slowdown in the three-way set.
The original WASM head showed `length` +1.04% (p=0.023); `strsub` had no
significant difference (p=0.128). See `three-way-*.txt`.

The layout probe found these amd64 sizes and offsets:

| Runtime | Bytes | Server options offset | String limit offset | Observed base address modulo 64 |
| --- | --- | --- | --- | --- |
| baseline | 248 | 72 | 96 | 0 |
| inline WASM state | 272 | 72 | 96 | 0 or 32 |
| separate WASM state | 256 | 72 | 96 | 0 |

Keeping the field offsets unchanged had not kept allocation alignment unchanged.
The change makes the hot runtime hold one pointer to the cold state and adds one
24-byte state allocation during session construction. Native dispatch, admission,
string processing, limits and per-call allocation counts are unchanged. Only
WASM operations follow the new pointer.

The measurements support layout as a contributor. They do not identify a
particular hardware cache miss or establish a production latency effect.

## Results

These are benchstat medians and comparisons, with ten samples per row. `~`
means no statistically significant difference at the default 0.05 threshold.

| Set and comparison | length ns/op | length change | strsub ns/op | strsub change |
| --- | --- | --- | --- | --- |
| pinned, published to cold | 53.80 to 52.32 | -2.75%, p=0.000 | 238.6 to 234.3 | -1.80%, p=0.041 |
| pinned, baseline to cold | 52.72 to 52.32 | -0.75%, p=0.000 | 234.4 to 234.3 | ~, p=0.839 |
| normal holdout, published to cold | 53.21 to 52.40 | -1.51%, p=0.000 | 272.4 to 268.6 | ~, p=0.592 |
| normal holdout, baseline to cold | 52.80 to 52.40 | -0.74%, p=0.001 | 258.8 to 268.6 | ~, p=0.052 |

`length` remains 0 B/op and 0 allocations/op. `strsub` remains 72 B/op and
2 allocations/op. The length improvement reproduced in the independent set.
The strsub holdout versus baseline is borderline, with a +3.8% point estimate;
it does not prove equivalence or rule out a small regression. Its uncertainty
was about 3% for the baseline and 5% for the candidate in that set; one pinned
candidate interval was 11%. Do not call the original 5.42% increase a confirmed
regression, or this change proof that every native workload has zero overhead.

Separate ten-second CPU profiles were diagnostic, not promotion measurements.
Both spent substantial sample time in background sweeping (`runtime.bgsweep`:
34.88% baseline, 31.44% cold). Their timings reversed the original ranking:
269.5 ns/op baseline, 247.8 ns/op cold. Continuous allocation and background
runtime work make this tiny string benchmark sensitive to scheduling. These
profiles do not prove that all of the original difference was noise.

## Reproduce

Use isolated checkouts of the revisions above, with the same benchmark file.
In each checkout run `go test -c -o <scratch>/builtins-<revision>.test.exe ./builtins`,
using the labels `baseline`, `refactor`, `current`, and `cold` for the filenames.
Copy `measure-native.ps1` beside the binaries; it refuses missing inputs.
Run from that scratch directory:

```powershell
./measure-native.ps1 -Label three-way -Versions baseline,refactor,current -Rounds 10 -CPU 1 -Affinity 16
./measure-native.ps1 -Label cold-layout -Versions baseline,current,cold -Rounds 10 -CPU 1 -Affinity 16
./measure-native.ps1 -Label normal-holdout -Versions baseline,current,cold -Rounds 10 -CPU 32 -Affinity 0
benchstat cold-layout-current.txt cold-layout-cold.txt
benchstat cold-layout-baseline.txt cold-layout-cold.txt
benchstat normal-holdout-current.txt normal-holdout-cold.txt
benchstat normal-holdout-baseline.txt normal-holdout-cold.txt
```

`layout-probe.go.txt` contains the reflection probe. Copy it to a scratch `.go`
file and run it from each checkout with `go run <scratch>/native-layout.go`.
It constructs builtin sessions only; it does not launch a server or open a database.
