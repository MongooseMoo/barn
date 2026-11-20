# Make PR 464 (WASM component builtins) fast and mergeable

## Goal

Q 2026-11-20: "see if you can make the wasm pr actually worth merging, actually
make it fast? worktree, up to date, etc."

The PR is barn #464, branch `feat/wasm-builtins`, worktree
`C:/Users/Q/code/barn-wasm-builtins`.

## State (2026-11-20)

Plan written, no code changed. Waiting for Q's "make it so".

- #464 is 1235 commits behind master, 2 ahead. Its green checks are from
  October. Its description still says "Stacked on #462"; #462 merged
  2026-10-11.
- A dry-run merge with master (`git merge-tree`) conflicts in 8 files:
  README.md, builtins/catalog.go, builtins/session_runtime.go,
  cmd/barn/main.go, config/options.go, go.mod, go.sum,
  internal/dbtool/dbtool.go.
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

## Unverified

- No measurement was rerun on 2026-11-20; every number above is from the
  October records.
- Whether master's changes since October alter the native holdout results.
- Restore cost of candidate A, and all Linux behavior.

## Next step

On Q's "make it so": step 1.
