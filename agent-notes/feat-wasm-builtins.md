# Make PR 464 (WASM component builtins) fast and mergeable

## Goal

Q 2026-11-20: "see if you can make the wasm pr actually worth merging, actually
make it fast? worktree, up to date, etc."

The PR is barn #464, branch `feat/wasm-builtins`, worktree
`C:/Users/Q/code/barn-wasm-builtins`.

## State (2026-11-20)

- Plan step 1 is done: the PR branch has master merged in and is pushed.
- Plan step 2 is done: `internal/wasmbuiltins/call_bench_test.go`
  (`BenchmarkWASMCall`: Full, InstantiateClose, ExecuteExisting for
  `example.wasm` and `scalar.wasm`) is committed on its own. Local, not
  pushed.
- wazy is upgraded from v0.3.0 to v0.8.0, committed with the record
  `experiments/2026-11-20-wasm-wazy-upgrade/README.md` and its gate output
  `gates.txt`. Local, not pushed.
- The upgrade does not change full-call cost: about 100 us for the Rust
  fixture (`example.wasm`) and about 26 us for the 2-page fixture
  (`scalar.wasm`) on Windows and Linux. Running an export on an existing
  guest went from about 3 us to about 1 us. Native `BenchmarkVM` rows: no
  slowdown.
- v0.8.0 dropped the call-entry context check, so a call with an
  already-cancelled context ran to completion and failed
  `TestAdmissionAndGuestBudgets`. `Manager.Call` now checks `ctx.Err()`
  before `component.Instantiate`; the test is unchanged and passes. A
  deadline passing during a pure compiled loop still ends the call with
  `E_QUOTA` (loop preemption checks). A call that completes after its
  deadline returns its result on bare v0.8.0 where v0.3.0 could return
  `E_QUOTA`; `Manager.Call` now checks `ctx.Err()` after the call too and
  returns `E_QUOTA` (read from source; no test reaches that line).
  Details and source lines in the record.
- `withGraphInstantiation` is still needed on v0.8.0 (the one-instance path
  still bypasses `CompileCache`); kept, comment corrected.
- On v0.8.0, 94% of a Full call is `component.Instantiate`, and two sinks
  account for most of it: clearing the guest's linear memory on every
  instantiation (about a third of the call) and binding every export, one
  new call engine per export, on every instantiation (about a quarter).
  v0.8.0 has no instance reset, snapshot or clone API.
- An instance reset is being added to a fork of wazy in
  `C:/Users/Q/code/wazy`, branch `instance-reset`, by another agent.
- Three local branches stacked on the PR were never pushed:
  `feat/wasm-values-host` (richer values, host imports, and a copy of the
  wazy v0.3.0 runtime in `third_party/wazy`, about 150,000 lines in 233
  files), `bench/wasm-call-costs` (benchmarks and profiles),
  `perf/wasm-instantiation` (three setup-cost patches to the copied runtime).
  Worktrees: barn-wasm-values, barn-wasm-costs, barn-wasm-fast.

## Why it is slow (October records, not rerun)

Source: `experiments/2026-10-11-wasm-instantiation/README.md` and
`experiments/2026-10-11-wasm-instantiation-fix/README.md` on
`perf/wasm-instantiation`. Windows amd64, one pinned CPU, tiny fixtures. No
Linux numbers exist.

- Full call of `example.add(20, 22)`: about 100 us on the PR, about 79 us with
  the perf commit.
- Running the export on a guest that already exists: about 3 us.
- SunRay's `group_number` verb in MOO on ordinary inputs: about 9 to 12 us.
- Every call builds a guest and closes it:
  `internal/wasmbuiltins/runtime.go` calls `component.Instantiate` and
  `inst.Close` inside `CallWithHost` (lines 461 and 466 on
  `perf/wasm-instantiation`; the PR has the same shape in `Call`).
- Profile of the pure call: 94.56% under `component.Instantiate`; 30.90% flat
  in `memclrNoHeapPointers`, 83.80% of that from `getPooledMemoryBuffer`
  (clearing the guest's linear memory); 44.61% under `bindFuncExportGraph`.
- The runtime has no reset or snapshot API (searched `component`,
  `internal/component/instance`, `internal/wasm`, `experimental`).

## Plan

Target: a pure wasm call costs less than the same work as a MOO verb (under
10 us for the `add` fixture), measured on Linux as well as Windows.

1. Merge master into `feat/wasm-builtins` in this worktree and resolve the 8
   conflicts. Push; CI on the PR is the check.
2. Bring the benchmark file from `bench/wasm-call-costs`
   (`internal/wasmbuiltins/instantiation_bench_test.go`, pure-call rows only)
   onto the PR so before and after are measured by the same code.
3. Candidate A, keeps "fresh guest state on every call": each loaded
   generation keeps warm guests and restores one to its just-instantiated
   state after each call (memory image, globals, tables, dropped segments). A
   guest that trapped, timed out or failed to restore is discarded. This needs
   a reset entry point added to the runtime. Measure the restore cost; memory
   restore is the unknown (the clear alone is about 30% of today's call).
4. Candidate B, same guarantee: build fresh guests ahead of time off the call
   path and consume each once. Call latency drops to about the 3 us floor
   while the pool keeps up; total CPU per call is unchanged.
5. Measure A, B and A+B against the PR head with ten alternating pinned
   processes per side, plus the native VM holdouts (`BenchmarkVM`), since a
   slowdown of native calls is a defect. Keep what wins.
6. Only if neither reaches the target: put the remaining option to Q with the
   numbers. That option is reusing a guest with no reset (about 3 us
   measured). It drops the PR's "fresh guest state" guarantee: guest statics
   would persist across calls and players, and a task retried after a commit
   conflict would see state left by its aborted attempt.
7. Record results under `experiments/`, update the PR description (remove the
   stale "Stacked on #462" line, new numbers), queue the merge.

## Decisions and why

- The runtime patch goes in a fork of wazy referenced from go.mod, not a
  150,000-line copy in `third_party/`. Same arrangement as the ToastStunt
  fork. Creating the fork repository is a GitHub write and needs Q's order.
- Host imports and richer values (`feat/wasm-values-host`) stay out of #464.
  They are a separate feature and get their own PR against master later. The
  three setup-cost patches from `perf/wasm-instantiation` move to the fork
  only if step 5 shows they still matter once guests are reused.
- wazy moved to v0.8.0 before the fork, so the reset patch is written
  against current upstream. The upgrade itself bought no full-call speed;
  the two setup sinks are unchanged, so candidate A (reset) is still the
  path.
- `Manager.Call` owns the call-entry and post-call context checks rather
  than relying on the runtime, because v0.8.0 removed
  `WithCloseOnContextDone`, checks the context only once a call yields to Go,
  and returns success for a call that finishes after its deadline. Keeping
  v0.3.0's `E_QUOTA` for a late call was the team lead's decision.

## Unverified

- The "Why it is slow" section is from the October records; the
  2026-11-20 measurements on v0.3.0 and v0.8.0 are in the upgrade record.
- Linux timings in the upgrade record were taken under CI load (WSL load
  average 9.7-20.5 on 8 CPUs).
- Restore cost of candidate A.
- CI has not run the upgrade (not pushed).

## Next step

Candidate A: once the wazy fork's `instance-reset` branch has a reset entry
point, point go.mod at the fork and give each generation a pool of reset
guests in Barn (a guest that trapped, timed out or failed to reset is
discarded), then measure as in plan step 5.
