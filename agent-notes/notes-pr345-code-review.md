# PR #345 code review — 2026-09-30

## Target (verified)
- PR #345 `feat/threaded-builtins-yield` -> `master`, head `d3bb0fe`, 65 files.
- Reviewed from worktree `C:/Users/Q/code/barn-pr345-retry` (at d3bb0fe).
  `barn-pr345-perf-review` is stale at `b33a487`.

## Finding (verified by me against the code)
Unhandled `FlowAbortAttempt` from `FinishRecycleLifecycle` (low severity).
- `builtins/objects.go:447`: new `beginIrreversible(ctx)` gate returns
  `abortedAttempt()` (FlowAbortAttempt).
- `vm/stack.go:180-191`: recycle continuation switch has no case for it ->
  `fmt.Errorf("unexpected recycle continuation flow")`. `vm/vm.go:869-888`
  turns that into E_EXEC (pushed as a value in non-d verbs, raised and
  catchable in d verbs). MOO code keeps running on an abandoned attempt.
- `vm/vm.go:1530-1537`: exception-unwind path checks only FlowException; the
  abort is silently dropped.
- Mitigation that exists: `ConflictRetryRequested` stays set and
  `vm/op_misc.go:96` aborts at the next builtin call, so the retry still
  happens. Cost: spurious E_EXEC and wasted work until then (up to the tick
  limit if no builtin is called).
- Likely fix: handle FlowAbortAttempt at both sites the way op_misc.go:96
  does (set vm.yieldResult to the abort and unwind).

## Unverified (the subagent's claims only)
- Ruled out: retry tick budget, copyRetryVM stack clone, LoopState.Iterator
  shallow copy, sort() comparator, scheduler blockedReadiersLocked cycle,
  drainForks replay.
- Open question: passing taskCtx to Commit drops uncommitted writes on kill,
  possibly at shutdown too (Runtime.Stop cancels s.ctx).

## Fix in progress (Q: "fix it") in barn-pr345-retry
- New `vm.abortAttempt()` helper (vm/recycle_lifecycle.go); op_misc.go:96 uses it.
- vm/stack.go recycle switch: `case FlowAbortAttempt` -> abortAttempt.
- vm/vm.go HandleError unwind: abort stops unwinding, returns handled=true
  with vm.yielded set; executeLoop's yielded check returns it; Resume()
  now returns yieldResult if yielded after HandleError.
- Move continuation can't abort (ApplyMoveLifecycle returns an error code).
- Test: engine/recycle_retry_boundary_test.go (return + exception subtests;
  :recycle suspends, interfere() makes renew validation fail on attempt 1).
  Green with fix; red without (probe attempts [1 2] and [1 2 2]).
- Gates green locally: gofmt, vet, staticcheck, go test ./..., race on
  db/store, engine, vm. Conformance not run.
- Committed 327e626, pushed fast-forward d3bb0fe..327e626 to
  feat/threaded-builtins-yield (PR #345). Next: watch PR CI.

## State / next step (before fix)
- Nothing posted to the PR. Waiting for Q to decide whether to file the
  finding as an inline review via the reviews API, or to fix it.
- The subagent left `probe_ticks_test.go` in the session scratchpad (not in
  any worktree); gopls flags it. Harmless.
