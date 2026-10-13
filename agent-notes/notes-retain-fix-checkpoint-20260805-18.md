# Retain/fix checkpoint 18

## Current PR #96 head and blocker

- Exact remote head remains `cc436313d892cba6b6e2cb6385849e9a360f0392`, draft, literal `Closes #95`.
- Build/unit is green. Legacy conformance is back to the expected `82 failed, 11108 passed, 396 skipped`; the four waif lifecycle regressions from the prior head are gone.
- Final review BLOCKED a root-snapshot TOCTOU: `flushDeferredGC` proves quiescence under `s.mu`, releases it, then drains/sweeps. A new VM can start after the root snapshot and gain/create unaccounted anonymous or waif roots before candidate recycling.

## Accepted barrier invariant

- `gcSweepMu` serializes complete sweep attempts. Explicit `run_gc` uses TryLock and returns success/no-op if another sweep owns it, including reentrant `run_gc` from a recycle hook.
- `vmStartMu` is held by a sweep from before root capture through candidate processing/recycle hooks. Every VM execution start briefly takes `vmStartMu`, then publishes its refcounted physical execution lease under `s.mu` and task state under `task.mu`.
- Lock order: `gcSweepMu -> vmStartMu -> s.mu -> task.mu`.
- Deferred/global sweeps fail closed when disallowed physical leases exist. Inline floor GC also fails closed and defers if any non-caller physical lease exists; merely skipping an active sibling is unsafe because that sibling can create anonymous IDs at or above the caller's floor.

## VM ownership matrix

- `runTask`: physical execution lease.
- `EvalCommandOutput`: must gain a physical lease for its complete ExecuteLoop/Resume lifecycle; currently unleased.
- top-level `CallVerbWithArgstr`: must gain a physical lease without changing observable task IDs; currently unleased.
- `CallVerbInContext`: inherits its parent execution ownership.
- `callWaifRecycle` / recycle VM: sweep-owned while the start barrier is held.
- Nested verb execution must inherit an existing lease or sweep explicitly. Do not blindly reacquire from a recycle hook (deadlock), and do not treat TryLock failure as inherited ownership for an unrelated goroutine.
- Preserve Eval/CallVerbWithArgstr transaction semantics: use scheduler-private context ownership provenance or an internal inherited-ownership parameter, not a broad reroute of all no-transaction eval calls to `CallVerbInContext`.

## Required proof and next action

- Deterministic actual-sweep barrier test: block in recycle after roots are captured, start another VM/task, prove it cannot enter until sweep releases, then prove it enters and its live root is retained.
- Cover Eval and top-level server-hook direct VM leases, inherited nested calls, reentrant `run_gc` success/no-op, and per-floor-GC deferral around active siblings.
- Then focused red/green x20, race, full tests, build, vet, gofmt, diff-check; commit/push/body; final exact-head independent review before merging history into PR #115.

