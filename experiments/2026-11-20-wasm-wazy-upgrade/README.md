# wazy v0.3.0 to v0.8.0 upgrade: wasm_call cost

This record says whether moving `internal/wasmbuiltins` from
github.com/samyfodil/wazy v0.3.0 to v0.8.0 makes `wasm_call` cheaper, what the
upgrade changed in Barn, and what a call still spends its time on.

Result in one line: the upgrade does not make a `wasm_call` measurably faster.
Full and InstantiateClose are unchanged within noise on Windows and Linux
(about 100 us for the Rust fixture); only running an export on an existing
guest got faster (about 3 us to about 1 us), and that is not a supported mode.
With the upgrade, `Manager.Call` checks for a done context itself before
instantiating and after the call returns, because v0.8.0 no longer does
either; with that change every existing test passes unchanged (see
"Cancelled-context call" and "Gates").

## Environment

- AMD Ryzen 9 5950X, 16 cores / 32 logical CPUs, Windows 11 Pro 10.0.26200.
- Go 1.27.1 windows/amd64. Linux binaries cross-built with
  `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` and run in WSL2 distro `Debian`
  (kernel 6.18.58.1-microsoft-standard-WSL2, 8 CPUs).
- Branch `feat/wasm-builtins`, worktree `C:/Users/Q/code/barn-wasm-builtins`.
  Baseline: the branch with the benchmark commit (wazy v0.3.0). Candidate: the
  same plus the upgrade in this commit.
- The workstation is shared with eight CI runner services (in the same WSL
  distro) and other sessions.

## Method

- Benchmark: `internal/wasmbuiltins/call_bench_test.go`, `BenchmarkWASMCall`,
  identical source in both binaries. Rows per fixture:
  - `Full`: `Manager.Call(ctx, 0, "<module>.add", [20, 22])`, result checked
    equal to 42 inside the loop.
  - `InstantiateClose`: `component.Instantiate` with the generation's
    `CompileCache`, then `Close`, under a 250 ms context.
  - `ExecuteExisting`: one guest instantiated once, the export called
    repeatedly (lower, call, lift, check). Diagnostic floor only; Barn never
    reuses a guest.
  - Fixtures: `testdata/example.wasm` (Rust, wit-bindgen) and
    `testdata/scalar.wasm` (hand-written WAT, 2-page memory).
- Test binaries built once per side into the git-ignored `.tmp/`
  (`go test -c`), baseline built before the upgrade and kept.
- Each process: run from the package directory so `testdata` resolves,
  `GOMAXPROCS=1`, pinned to logical CPU 6 (Windows: `ProcessorAffinity` set
  right after process start; WSL: `taskset -c 6`),
  `-test.run=^$ -test.benchtime=250ms -test.benchmem -test.count=1`.
- Ten processes per side, alternating baseline and candidate process by
  process.
- Load gate before each process: total host CPU load
  (`Win32_Processor.LoadPercentage`, averaged) must be under 50%; wait up to
  two minutes, then proceed and log it. No process needed to wait past the
  gate. Every figure is in each raw directory's `load.txt`; WSL runs also log
  WSL's `/proc/loadavg`.
- Comparison: `benchstat` (golang.org/x/perf), median with 95% confidence
  interval, Mann-Whitney U test, n=10 per side.
- The driver script was a scratch PowerShell file outside the repository; its
  behaviour is what the bullets above state.

## Results

Times are medians, +- is benchstat's 95% confidence interval, `~` means no
significant difference (p >= 0.05).

### Windows (raw: `raw-windows-wasm/`, host load 14-49% before each process)

| Row | v0.3.0 | v0.8.0 | Change |
|---|---|---|---|
| example Full | 101.0 us +-35% | 103.0 us +-15% | ~ (p=0.912) |
| example InstantiateClose | 94.48 us +-14% | 98.72 us +-22% | ~ (p=0.393) |
| example ExecuteExisting | 3.175 us +-8% | 1.026 us +-12% | -67.69% (p=0.000) |
| scalar Full | 30.52 us +-15% | 26.48 us +-15% | -13.23% (p=0.023) |
| scalar InstantiateClose | 24.93 us +-12% | 25.25 us +-11% | ~ (p=0.739) |
| scalar ExecuteExisting | 2.955 us +-13% | 0.987 us +-74% | -66.59% (p=0.000) |

| Row | B/op v0.3.0 -> v0.8.0 | allocs/op v0.3.0 -> v0.8.0 |
|---|---|---|
| example Full | 41.74 Ki -> 46.48 Ki (+11.35%) | 144 -> 150 |
| example InstantiateClose | 41.32 Ki -> 46.43 Ki (+12.37%) | 138 -> 149 |
| example ExecuteExisting | 665.5 -> 288 | 9 -> 5 |
| scalar Full | 17.30 Ki -> 21.49 Ki (+24.26%) | 130 -> 123 |
| scalar InstantiateClose | 16.97 Ki -> 21.45 Ki (+26.38%) | 124 -> 121 |
| scalar ExecuteExisting | 589 -> 288 | 9 -> 5 |

An earlier v0.3.0-only run (raw: `raw-windows-baseline/`, load 29-43%) gave
example Full 102.1 us +-6% and InstantiateClose 96.02 us +-5%, consistent
with the alternating run and with the October figure of about 100 us.

### Linux, WSL Debian (raw: `raw-linux-wasm/`)

Host load before each process was 9-48%, but WSL's own load average was
9.7-20.5 on its 8 CPUs throughout (the CI runners), so pinned CPU 6 was
shared with other work. Treat these spreads as contaminated.

| Row | v0.3.0 | v0.8.0 | Change |
|---|---|---|---|
| example Full | 105.15 us +-24% | 98.55 us +-57% | ~ (p=0.631) |
| example InstantiateClose | 97.00 us +-43% | 95.29 us +-26% | ~ (p=0.971) |
| example ExecuteExisting | 2.851 us +-20% | 0.995 us +-22% | -65.09% (p=0.000) |
| scalar Full | 28.52 us +-45% | 27.02 us +-110% | ~ (p=0.218) |
| scalar InstantiateClose | 23.84 us +-38% | 24.61 us +-61% | ~ (p=0.353) |
| scalar ExecuteExisting | 2.731 us +-55% | 1.009 us +-93% | -63.05% (p=0.000) |

Bytes and allocations match the Windows table (same code paths).

### Native VM paths (raw: `raw-windows-vm/`, host load 2-45%)

`go test -c ./vm` before and after, alternating, ten processes per side,
`-test.bench='BenchmarkVM/(int_arith_1M|builtin_abs_200k|tostr_200k|prop_defined_1M)$'`:

| Row | v0.3.0 | v0.8.0 | Change |
|---|---|---|---|
| int_arith_1M | 49.30 ms +-1% | 49.40 ms +-3% | ~ (p=0.684) |
| builtin_abs_200k | 30.76 ms +-1% | 28.65 ms +-1% | -6.85% (p=0.000) |
| tostr_200k | 76.93 ms +-1% | 75.37 ms +-2% | -2.04% (p=0.000) |
| prop_defined_1M | 204.0 ms +-2% | 204.8 ms +-2% | ~ (p=0.436) |

No native slowdown. Allocations identical. No VM code changed; the `vm` test
binary links `wasmbuiltins` through `builtins` (40.9 MB -> 42.8 MB), so the
two faster rows are most likely code-layout effects; no source cause was
found or looked for.

## What changed in the code

- `go.mod` / `go.sum`: github.com/samyfodil/wazy v0.3.0 -> v0.8.0. `go mod
  tidy` changed nothing else.
- `internal/wasmbuiltins/runtime.go:326`: removed
  `.WithCloseOnContextDone(true)` from the runtime config. v0.6.0 removed the
  option; compiled code is always interruptible and a done context ends the
  call with an error, leaving the module usable (release notes v0.6.0). Barn
  closes the guest after every call anyway, so nothing replaces it. The
  250 ms deadline test (`TestAdmissionAndGuestBudgets/spin`) still returns
  `E_QUOTA`.
- `internal/wasmbuiltins/runtime.go:445-449`: `Manager.Call` returns
  `guestError(ctx)` when the call's context is already done, before
  instantiating. v0.8.0 no longer checks at call entry (see
  "Cancelled-context call").
- `internal/wasmbuiltins/runtime.go:463-466`: `Manager.Call` also returns
  `guestError(ctx)` when the context is done after `inst.Call` returned
  without an error, keeping v0.3.0's `E_QUOTA` for a call that finishes after
  its deadline (see "Deadline passing just before a call returns
  successfully"). Decided by the team lead.
- `internal/wasmbuiltins/compile.go:8-12`: comment on
  `withGraphInstantiation` corrected; the code is unchanged. See below.

### withGraphInstantiation: still needed, still valid

Source under
`C:/Users/Q/go/pkg/mod/github.com/samyfodil/wazy@v0.8.0/internal/component/instance/`:

- `instance.go:1473-1476` routes to `instantiateGraph` only if
  `needsGraphPath` (`instance.go:1487-1509`) or `needsImportPath`
  (`instance.go:1547-1575`) holds; `needsImportPath` returns true for
  `len(comp.CoreInstances) != 1` (`instance.go:1571`), which is what the
  appended inline core instance trips.
- The trivial path still bypasses `CompileCache`: `instantiateComponentWith`
  calls `r.InstantiateWithConfig` (`instance.go:1622`), which compiles on every
  call (`C:/Users/Q/go/pkg/mod/github.com/samyfodil/wazy@v0.8.0/runtime.go:391-397`),
  and binds exports with `finalizeBoundExport(be, resolve, nil, nil, 0) //
  trivial path: no CompileCache` (`instance.go:1675`). The graph path uses the
  cache (`graph.go:320-322`).
- The trivial path now instantiates anonymously (`WithName("")`,
  `instance.go:1622`), so the old comment's second reason, name registration,
  no longer holds. The comment now states only the cache reason.
- v0.8.0's stricter load-time validation accepts the rewritten bytes: the
  appended section is a standard inline (`0x01`) core instance exporting core
  func 0. `scalar.wasm` is rewritten (one core instance, no imports) and
  `TestNamedScalarComponentConcurrencyAndLimits` loads, calls and re-extracts
  it.
- For both fixtures the rewrite does not change the route. `scalar.wasm`
  aliases its core memory (`scalar.wat`), which alone makes `needsGraphPath`
  true (`instance.go:1503-1506`); `example.wasm` lifts strings and lists,
  which require a `memory` canonical option, and such an option alone makes
  `needsImportPath` true (`instance.go:1565-1568`). It matters for a component with no memory alias
  and only plain lifts; no such fixture exists, so that effect is unmeasured.

## Cancelled-context call: what failed and the fix

What failed: with only the version bump and the removed option, one existing
test failed (raw: `test-wasmbuiltins-v080.txt`); all other tests in
`./internal/wasmbuiltins` and `./builtins` passed. The failing assertion is
`internal/wasmbuiltins/runtime_test.go:147`: `Manager.Call` with an
already-cancelled context must return `E_QUOTA`. On v0.8.0 the call completed:

```
    runtime_test.go:147: cancel: {Val:3 Flow:0 Error:E_NONE Reason:unknown Unreported:false Label: ForkInfo:<nil> CallStack:[]}
--- FAIL: TestAdmissionAndGuestBudgets (0.33s)
```

Why (upstream source):

- v0.3.0 with `WithCloseOnContextDone(true)` checked the context at every call
  entry: `C:/Users/Q/go/pkg/mod/github.com/samyfodil/wazy@v0.3.0/internal/engine/native/call_engine.go:576-583`
  (`if ensureTermination { select { case <-ctx.Done(): m.CloseWithCtxErr(ctx); return m.FailIfClosed()`).
- v0.8.0 has no entry check. The context is first watched when a call yields
  to Go, so "a call that is over before then never pays for it":
  `C:/Users/Q/go/pkg/mod/github.com/samyfodil/wazy@v0.8.0/internal/engine/native/call_engine.go:484-510`
  (`watchContext`), registered from lines 443 and 535; `callWithStack`
  (line 576) does not look at the context. `example.add` never yields.
  `component.Instantiate` and `Instance.Call` do not check it either.

The production change that fixed it: Barn now owns the entry check.
`Manager.Call` returns `guestError(ctx)` (`E_QUOTA` for a done context) when
`ctx.Err() != nil`, after `context.WithTimeout` and before
`component.Instantiate` (`internal/wasmbuiltins/runtime.go:445-449`). The test
is unchanged (`git diff -- '*_test.go'` is empty) and passes (raw:
`test-wasmbuiltins-v080-fixed.txt`, and the fresh `go test` gate in
`gates.txt`).

### Deadline passing during a pure compiled loop: still ended with E_QUOTA

Source under
`C:/Users/Q/go/pkg/mod/github.com/samyfodil/wazy@v0.8.0/internal/`:

- Loop preemption checks are on in every build: `var LoopYield = true`
  (`engine/native/frontend/frontend.go:186-188`), copied into the compiler at
  `frontend.go:197`. Every `loop` header gets a check
  (`engine/native/frontend/lower.go:1515-1520`, `emitPreemptCheck` at
  `lower.go:4598`): it reads the word the Go runtime sets to ask a goroutine
  to stop and, when set, returns to Go. Functions that can recurse without
  crossing a loop check on entry (`lower.go:225-235`, `needsEntryCheck` at
  `lower.go:263-267`).
- The Go runtime asks any goroutine that has run for more than a few
  milliseconds to stop, so a looping guest returns to Go through `yieldToGo`
  (`engine/native/call_engine.go:439-450`), which registers the context
  watcher (`watchContext`, `call_engine.go:484-510`) and fails the call if the
  watcher saw the context done (`failIfCanceled`, which panics with the
  context's cause, `wasm/call_watch.go:62-65`). Once registered, a done
  context makes the watcher mark the call canceled and store the stop word
  itself (`wasm/call_watch.go:24-36`, `preempt` in `call_engine.go`), so the
  guest stops at its next loop check and the call returns an error.
- `Manager.Call` turns that error into `guestError(ctx)`, which is `E_QUOTA`
  because the context is done (`internal/wasmbuiltins/runtime.go:459-461,
  478-483`).
- The existing `spin` subtest is such a loop: `examples/wasm/src/lib.rs:21-22`
  is `fn spin() -> i64 { loop { std::hint::spin_loop(); } }`, with no host
  calls (admission refuses imports). `runtime_test.go:130` expects `E_QUOTA`;
  it passes on v0.8.0 in 0.25 s (`test-wasmbuiltins-v080-fixed.txt`).

### Deadline passing just before a call returns successfully: wazy differs from v0.3.0, Barn now checks

Before the fix below, `Manager.Call` did not look at the context after
`inst.Call` returned without an error; it lifted and returned the result. So
the answer depended on whether wazy reports the late finish as an error.

- v0.8.0: on a normal return the call's error is `FailIfClosed()`
  (`C:/Users/Q/go/pkg/mod/github.com/samyfodil/wazy@v0.8.0/internal/engine/native/call_engine.go:703-704`).
  A done context never closes the module (`wasm/call_watch.go:62-63`: "The
  module stays usable"); it only stops the guest at its next check. A guest
  that reaches its return after the deadline but before its next check (or
  before it has ever yielded, so before any watcher exists) returns success,
  and `Manager.Call` returns the value.
- v0.3.0 with `WithCloseOnContextDone(true)`: every call started a goroutine
  that closed the module as soon as the context was done
  (`C:/Users/Q/go/pkg/mod/github.com/samyfodil/wazy@v0.3.0/internal/engine/native/call_engine.go:651-654`,
  `CloseModuleOnCanceledOrTimeout` at
  `C:/Users/Q/go/pkg/mod/github.com/samyfodil/wazy@v0.3.0/internal/wasm/module_instance.go:43-61`),
  and a normal return still set the error to `FailIfClosed()` (`call_engine.go:639`
  in v0.3.0). So a call that finished after that goroutine had closed the
  module returned an error, and `Manager.Call` returned `E_QUOTA`.
- So on v0.3.0 a call that completes after its 250 ms deadline could return
  `E_QUOTA`; on v0.8.0 it returns its result. The window is the straight-line
  code after the guest's last loop check, plus the Go-side lowering and
  lifting inside `Instance.Call`.
- Fix (team lead's decision: keep v0.3.0's contract): after `inst.Call`
  returns without an error, `Manager.Call` returns `guestError(ctx)`, which is
  `E_QUOTA`, when `ctx.Err() != nil`
  (`internal/wasmbuiltins/runtime.go:463-466`). This is slightly stricter
  than v0.3.0, which gave `E_QUOTA` only if its watcher goroutine had already
  closed the module; Barn now gives it whenever the deadline has passed by
  the time the call returns.
- No test was added: making the deadline pass between the guest's last
  check and `inst.Call` returning needs either a fixture that runs for a
  controlled time or a hook inside wazy, and the brief allowed neither. The
  `spin` export never returns, so no existing test reaches this line; see
  "Not verified".

## Where a Full call's time goes on v0.8.0

CPU profile: `GOMAXPROCS=1 .tmp/wasm-v080.exe -test.bench='BenchmarkWASMCall/example/Full$' -test.benchtime=5s -test.cpuprofile=...`
(110.3 us/op in that run). Files: `cpu-example-full-v080.pprof`,
`cpu-example-full-v080-top-cum.txt` (whole process),
`cpu-example-full-v080-call-only-top-cum.txt` (`-focus=Manager.Call`),
`cpu-example-full-v080-top-flat.txt`. The whole-process profile also contains
session setup (the bundled wasm-tools parser compile, 23% of samples), so
shares below are of the 6.15 s under `Manager.Call`.

Top cumulative entries under `Manager.Call`:

| cum | share of Call | function |
|---|---|---|
| 6.15 s | 100% | `wasmbuiltins.(*Manager).Call` |
| 5.79 s | 94% | `component.Instantiate` / `instance.Instantiate` |
| 5.65 s | 92% | `instance.instantiateGraph` |
| 2.68 s | 44% | `instance.instantiateCoreModuleCacheable` -> `runtime.InstantiateModule` -> `wasm.(*Store).instantiate` |
| 2.49 s | 40% | `instance.bindImportExportsGraph` -> `bindFuncExportGraph` (2.45 s) |
| 2.34 s | 38% | `runtime.memclrNoHeapPointers` (flat) |
| 2.05 s | 33% | `wasm.(*ModuleInstance).buildMemory` -> `NewMemoryInstance` |
| 2.04 s | 33% | `instance.finalizeBoundExport` |
| 1.99 s | 32% | `wasm.getPooledMemoryBuffer` (all of it is `memclrNoHeapPointers`) |
| 1.58 s | 26% | `runtime.mallocgc` |
| 1.46 s | 24% | `wasm.(*ModuleInstance).ExportedFunction` -> `native.(*moduleEngine).NewFunction` (1.38 s) |

Two sinks account for most of it, both in setup, none in the export:

1. Clearing the guest's linear memory. Each instantiation takes a pooled
   buffer and zeroes it: `internal/wasm/memory_pool.go:74-91`
   (`clear(buf)` at line 89), from `NewMemoryInstance`
   (`internal/wasm/memory.go:451`) via `buildMemory`
   (`internal/wasm/module.go:959`). The clear covers the buffer's visible
   length, so it grows with the guest's memory; that is consistent with the
   2-page scalar fixture costing about a quarter of the Rust one (the Rust
   fixture's memory size was not checked).
2. Binding every export on every instantiation. `finalizeBoundExport`
   (`internal/component/instance/instance.go:1248`) calls
   `be.mod.ExportedFunction(be.funcName)` (line 1250), which allocates a new
   `callEngine` per export (`internal/wasm/module_instance.go:243-249`,
   `internal/engine/native/module_engine.go:190-250`). The example fixture has
   11 exports and all are bound though one is called.

(All paths in this section are under
`C:/Users/Q/go/pkg/mod/github.com/samyfodil/wazy@v0.8.0/`.)

## Does v0.8.0 expose reuse, reset, snapshot or pre-building of an instance?

No reset, snapshot or clone of an instance's state; pre-building stops short of
the instance.

- Public component API, `component/component.go:45-144`: `Instantiate`,
  `WithCompileCache`, `NewCompileCache`, import options. `Instance` methods
  (`internal/component/instance/instance.go:460, 466, 1915, 1942, 2553, 2588`;
  `callasync.go:117`; `public_host.go:84-128`; `stream_host.go:126-164`):
  `Call`, `CallAsync`, `CallExport`, `DropResource`, `Close`, `HostState`,
  `InstanceExports`, `Resources`, `ResourcesFor`, stream/future constructors,
  `CoreModuleCount`, `WASICalls`. None resets, snapshots or copies guest
  state, and the core `api.Module`s are not exposed.
- Runtime, `runtime.go:219-509`: `Module`, `DecodeModuleNoCompile`,
  `CompileModule`, `Instantiate`, `InstantiateWithConfig`,
  `InstantiateModule`, `Close`. Nothing to reset a module instance.
- Pre-building: `CompileCache` (`internal/component/instance/compilecache.go`)
  keeps compiled core modules, decoded components, graph plans and export ABI
  metadata across instantiations. That is everything except the instance
  itself; memory allocation and clearing and export binding still happen per
  instantiation (previous section).
- Snapshot: `api/checkpoint.go:9-36` (`WithSnapshotter`, `Snapshotter`,
  `Snapshot.Restore`) captures and restores the execution stack from inside a
  host function during a call (setjmp/longjmp style). It does not capture
  memory, globals or tables, and this package has no host functions.
- Reuse without reset: `Instance.Call` may be called repeatedly on one
  instance (what `ExecuteExisting` does), and v0.8.0 states that re-entering an
  instance is no longer a trap (release notes, "may_enter is gone"). Guest
  state then persists between calls, which Barn's "fresh guest state on every
  call" contract forbids.

## Note: component/wit could replace the bundled wasm-tools (not acted on)

v0.8.0 adds `component/wit.FromComponent(componentBytes)`
(`C:/Users/Q/go/pkg/mod/github.com/samyfodil/wazy@v0.8.0/component/wit/wit.go:360`)
and `component/binary.Decode` (`component/binary/binary.go:33`). The release
notes say `FromComponent` reads the WIT from a compiled component without
instantiating it. `internal/wasmbuiltins/wit.go` instead runs the embedded
wasm-tools parser (`wasm-tools.wasm.gz`) under a separate wazero runtime and
consumes `go.bytecodealliance.org/wit` types; `FromComponent` returns wazy's
own `*wit.Package`, so a switch would also change the type planning in
`types.go`. Out of scope; left as is.

## Gates

Raw: `gates.txt` (one run, started 2026-11-21T03:44:10Z, Go 1.27.1
windows/amd64, this commit's code including the post-call check, run before
committing). Each gate's exit status:

| Gate | Exit |
|---|---|
| `test -z "$(gofmt -l $(git ls-files '*.go'))"` | 0 |
| `go build ./...` | 0 |
| `go vet ./...` | 0 |
| `staticcheck ./...` | 0 |
| `golangci-lint run ./...` | 0 (`0 issues.`) |
| `go test ./... -count=1` | 0 (no `FAIL` line; `internal/wasmbuiltins` ok 7.157s, `builtins` ok 48.036s) |
| `go test -race ./internal/wasmbuiltins ./builtins -count=1` | 0 (ok 43.836s, ok 159.999s) |

The `gates.txt` left by the earlier, interrupted session (with a `FAIL` line
of unknown cause) was overwritten and not used. An intermediate run on the
code before the post-call check was added also exited 0 on every gate; its
output was overwritten by the run above.

## Not verified

- Linux timings were taken while WSL's load average was 9.7-20.5 on 8 CPUs;
  the Linux medians are not clean measurements.
- Windows CPU pinning is applied a few milliseconds after process start
  (affinity set on the running process), before the benchmark loop but not
  before runtime start.
- The load gate reads total host CPU load only; it does not see contention
  on the one pinned CPU.
- The cost effect of `withGraphInstantiation` on a component that would
  otherwise take the trivial path was not measured (no such fixture).
- No CPU profile was taken on v0.3.0 in this session; the October profile
  (30.90% in `memclrNoHeapPointers`, 44.61% under `bindFuncExportGraph`) is
  from the `perf/wasm-instantiation` branch records, not rerun.
- The native comparison ran on Windows only.
- Whether the two faster native rows have a cause other than code layout.
- CI was not run (nothing pushed).
- A call that completes after its deadline: from source, v0.3.0 could return
  `E_QUOTA` for it and bare v0.8.0 returns the result; Barn's post-call
  check (`runtime.go:463-466`) now returns `E_QUOTA`. No test exercises that
  line, and the behavior was not observed on either version.
- That the Go runtime's preemption request reaches a spinning guest within a
  few milliseconds on Linux; the `spin` subtest was run on Windows only.
- The benchmarks and profile were measured before the entry and post-call
  checks in `Manager.Call` were added; they are two `ctx.Err()` calls per
  `wasm_call` and were not re-measured.
