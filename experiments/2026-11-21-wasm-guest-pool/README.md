# wasm_call with a pool of reset guests

Date: 2026-11-21 (local time 2026-11-20 evening, UTC-7).

## What was measured

`internal/wasmbuiltins` `BenchmarkWASMCall` for the two call fixtures, and
the native `BenchmarkVM` holdouts, before and after the guest pool:

- **base**: the branch tree with `go.mod`, `go.sum` and
  `internal/wasmbuiltins` as they were before the pool commit (wazy v0.8.0,
  one guest instantiated and closed per call). Everything else, including
  the merge of master, is identical to final.
- **final**: the branch tree as committed (wazy fork with `Instance.Reset`;
  each generation keeps up to two reset guests; a call that finds the pool
  empty waits for a reset in flight).

Rows:

- `Full`: a tight loop of `Manager.Call`. In final, each call's guest is reset
  on a goroutine after the call and the next call waits for that reset, so on
  one CPU each `Full` iteration pays call plus reset.
- `FullSettled` (final only): the timer is stopped until the previous call's
  reset has finished, so it is the latency of a call that finds a clean guest
  ready.
- `InstantiateClose`: `component.Instantiate` plus `Close`, no call.
- `ExecuteExisting`: the export on one guest with no reset; a floor, not a
  supported mode.

Fixtures: `example.wasm` (the Rust example; its core module declares 17
initial memory pages) and `scalar.wasm` (hand-written WAT, 2 pages). Page
counts were read from the binaries' memory sections.

## Method

- Test binaries built with `go test -c -trimpath` for windows/amd64 and
  linux/amd64 (`CGO_ENABLED=0`); SHA-256 in `binaries.sha256.txt`.
- Ten rounds; in each round base then final, each a separate process with
  `GOMAXPROCS=1`, pinned to CPU 6 (Windows process affinity `0x40`; WSL
  `taskset -c 6`). wasm: `-test.bench=BenchmarkWASMCall -test.benchtime=250ms
  -test.benchmem -test.count=1`. vm:
  `-test.bench='BenchmarkVM/(int_arith_1M|builtin_abs_200k|tostr_200k|typeof_1M|prop_defined_1M)$'
  -test.benchtime=1s`.
- Before each process the driver read Windows CPU load
  (`Win32_Processor.LoadPercentage`) and waited, up to 120 s, for it to drop
  under 50%; for WSL runs it also logged `/proc/loadavg`. Every reading is in
  the `load.txt` beside the raw output.
- `benchstat` (golang.org/x/perf) of base against final, run from this
  directory.

## Environment and load

AMD Ryzen 9 5950X, Windows 11 Pro, WSL 2 Debian with 8 CPUs, Go 1.27.1.
Windows host load at sample start ranged 17-49% (longest wait 32 s; no wait
hit the 120 s limit). The WSL distro also runs Barn's eight CI runner
services; its load average was 15-28 on 8 CPUs during the Linux runs, and the
Linux spreads below (up to 175%) show it. The Linux rows support the
direction and size of change, not precise absolute times.

## Results: wasm, Windows (`benchstat-windows-wasm.txt`)

| row | base | final | change |
|---|---|---|---|
| example Full | 100.75 us +-8% | 27.67 us +-1% | -72.54% (p=0.000) |
| example InstantiateClose | 97.56 us +-11% | 101.51 us +-27% | ~ (p=0.631) |
| example ExecuteExisting | 1006.9 ns +-6% | 984.5 ns +-7% | ~ (p=0.247) |
| scalar Full | 28.888 us +-17% | 5.105 us +-8% | -82.33% (p=0.000) |
| scalar InstantiateClose | 25.06 us +-17% | 24.81 us +-8% | ~ (p=0.579) |
| scalar ExecuteExisting | 1020.5 ns +-13% | 991.9 ns +-16% | ~ (p=0.149) |
| example FullSettled | | 3.471 us +-11% | |
| scalar FullSettled | | 2.423 us +-3% | |

Full allocations: example 47597 B / 150 allocs -> 361 B / 8 allocs; scalar
22010 B / 123 allocs -> 360 B / 8 allocs.

## Results: wasm, Linux (`benchstat-linux-wasm.txt`)

| row | base | final | change |
|---|---|---|---|
| example Full | 160.59 us +-50% | 46.74 us +-83% | -70.89% (p=0.000) |
| example InstantiateClose | 181.0 us +-127% | 168.8 us +-46% | ~ (p=0.796) |
| example ExecuteExisting | 1.494 us +-38% | 1.296 us +-51% | ~ (p=0.190) |
| scalar Full | 42.362 us +-33% | 8.198 us +-26% | -80.65% (p=0.000) |
| scalar InstantiateClose | 33.98 us +-175% | 42.16 us +-80% | ~ (p=0.739) |
| scalar ExecuteExisting | 1.215 us +-87% | 1.313 us +-109% | ~ (p=0.971) |
| example FullSettled | | 5.523 us +-22% | |
| scalar FullSettled | | 3.279 us +-81% | |

Allocations match Windows (example Full 361 B / 8 allocs, scalar 360 B / 8).

An earlier run of the pool before the review fixes, under WSL load average
about 20, measured Linux example Full 31.42 us +-4% and scalar Full
5.579 us +-3% (`earlier-run/benchstat-linux-wasm-background.txt`).

## Results: native VM holdouts

Windows (`benchstat-windows-vm.txt`), sec/op:

| row | base | final | change |
|---|---|---|---|
| int_arith_1M | 46.26 ms +-3% | 47.03 ms +-2% | ~ (p=0.165) |
| builtin_abs_200k | 28.73 ms +-2% | 29.35 ms +-3% | ~ (p=0.063) |
| tostr_200k | 81.11 ms +-8% | 83.23 ms +-6% | ~ (p=0.481) |
| typeof_1M | 122.2 ms +-11% | 134.0 ms +-9% | ~ (p=0.123) |
| prop_defined_1M | 227.8 ms +-9% | 232.0 ms +-14% | ~ (p=0.436) |

Linux (`benchstat-linux-vm.txt`), sec/op:

| row | base | final | change |
|---|---|---|---|
| int_arith_1M | 51.57 ms +-33% | 54.32 ms +-57% | ~ (p=1.000) |
| builtin_abs_200k | 33.74 ms +-21% | 33.23 ms +-60% | ~ (p=0.631) |
| tostr_200k | 104.19 ms +-38% | 93.48 ms +-57% | ~ (p=0.971) |
| typeof_1M | 147.9 ms +-18% | 131.2 ms +-30% | ~ (p=0.853) |
| prop_defined_1M | 285.8 ms +-69% | 224.3 ms +-19% | -21.50% (p=0.043) |

Bytes and allocations per op are unchanged on both OSes. No VM row is
significantly slower on either OS. The VM interpreter loops do not reach
`wasm_call`; the test binary links `internal/wasmbuiltins`, so any
difference here is layout or noise. The one Linux row marked faster has 69%
spread on base and is not evidence of a speedup.

## What the raw files show

- Back-to-back calls on one CPU cost call plus reset: example Full 27.67 us
  on Windows (31.42 us in the earlier Linux run, 46.74 us in this loaded one),
  scalar Full about 5 us (5.105 us Windows, 5.579 us earlier Linux).
- A call that finds a clean guest ready costs about 3 us: FullSettled 3.471 us
  (example) and 2.423 us (scalar) on Windows.
- Reset cost is Full minus FullSettled: about 24.2 us for example and about
  2.7 us for scalar on Windows, so it scales with guest memory. wazy's
  restore (`internal/wasm/snapshot.go`, `memorySnapshot.restore`) walks the
  memory up to its current size, so the cost follows the guest's size at reset
  time. Per 64 KiB page that is about 1.4 us at the 17 pages example.wasm
  declares, about 1.15 us if its memory has grown to 21 pages by the time it
  is reset, and about 1.3 us for scalar's 2 pages.
- The under-10 us target is met for small guests (scalar Full about 5 us)
  and for a call that finds a ready guest (both fixtures about 2.4-3.5 us),
  but not for back-to-back calls on a default-stack Rust guest (example Full
  about 27-31 us, the cost of resetting its memory).
- Instantiation cost is unchanged by the pool (InstantiateClose ~ on both
  OSes); the pool removes it from the call path.

## Earlier records

`earlier-run/` holds the first recorded run of the pool, before the review
fixes (a context recheck after waiting for a reset, and a recovered panic in
guest close) and before master was merged. Its binaries no longer match the
code. It is kept because it is the only record of the design comparison:
`switch.txt` is the variant in `switch-variant.diff` (reset inline on the
call path by default, with a `Manager.backgroundReset` switch for the
background reset with wait that final uses), measured from its own test
binary. On Windows in that run, example `Full` (inline) cost 25.89 us and
`FullBackground` 26.66 us; scalar 4.314 us and 4.896 us
(`earlier-run/benchstat-windows-wasm-inline.txt`). Background reset was kept
as the only mode because it gives the about-3 us latency when a clean guest is
ready at about 1 us more per back-to-back call. Its Linux vm holdout run
stopped after three rounds and is incomplete.

## Gates

`gates.txt`: gofmt, go build, go vet, staticcheck, golangci-lint,
`go test ./... -count=1`, and `go test -race ./internal/wasmbuiltins
./builtins -count=1` on the final tree, with each command's exit status.

## Not verified

- Whether example.wasm's memory grows past its 17 declared pages during the
  benchmarked call; wazy gives no accessor for a guest's memory size, so the
  per-page cost for example is given for both 17 and 21 pages.
- Linux timings in this run were taken under WSL load average 15-28 on 8
  CPUs; the absolute Linux times are not reliable.
- Behaviour with more than one CPU: every run used `GOMAXPROCS=1`, where the
  reset goroutine runs inside the next call's timed window. With idle CPUs a
  reset could overlap the gap between calls; not measured.
- Real Mongoose workload or end-to-end server latency: not measured.
- CI has not run this tree.
