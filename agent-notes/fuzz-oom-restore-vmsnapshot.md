# Fuzz OOM: FuzzRestoreVMSnapshotDoesNotPanic

## Goal

Diagnose (no fix) why FuzzRestoreVMSnapshotDoesNotPanic (./vm) ran the fuzz compute guest out of memory on 2026-11-10 06:43:13 UTC (run 42766490058, lane mongoose-barn-fuzz-2).

(This note lives in the worktree feat-fuzz-compute-ops because the session is worktree-isolated; a stub with the same name was created in the shared checkout's agent-notes before the isolation took effect and is stale.)

## State

Diagnosis done. OOM input identified and reproduced locally: retained-corpus record a67f1c049e52d97e18d49c1aee56e21459aa3602003d91681f7066b02d7f7a6c (also a saved "hang" finding of run 42750434626). No code changed; the three replay inputs I put in vm/testdata were removed afterwards.

## Evidence

E1. Job 129664137710 log (`gh api --allow-escape-sequences repos/MongooseMoo/barn/actions/jobs/129664137710/logs`). Step 7 loops campaigns, each `go test ./vm -run '^$' -fuzz ... -parallel=2`; after each failure it quarantines the failing input and restarts. Three campaigns ran:
  - Campaign 1 (06:41:01-06:42:04): baseline coverage stalled at 357/3120 for ~57 s, then `fuzzing process hung or terminated unexpectedly: exit status 2`, input 021b3e3abaa49095 (issue #695, kind hang). Quarantined.
  - Campaign 2 (06:42:07-06:42:14): `panic: stack underflow` at vm/stack.go:51 via executeCallVerbNamed vm/op_verb.go:79, input 23a3e856b403a399 (issue #694). Quarantined.
  - Campaign 3 (06:42:18-06:43:16): baseline coverage advanced ~200 per 3 s to 2018/3118 at 06:42:48, then froze; 06:43:13 runner shutdown signal (guest-journal OOM time); `fuzzing process terminated by unexpected signal; no crash will be recorded: signal: killed`.
  The OOM happened while replaying the retained corpus (baseline coverage), not on a mutated input. Go records nothing on SIGKILL, so the log does not name the input.
E2. Steps 8-13 skipped (step cancelled): run 42766490058 has no Restore failure or corpus artifact.
E3. Retained cache: key `fuzz-public-v1-FuzzRestoreVMSnapshotDoesNotPanic-42750434626-1`, manifest_sha256 38fc35833da19f896bff09d32f4d710aaa1f09ed7095dae1965c3f35c4695a9e (log line 221) = census.json manifest_sha256 of artifact fuzz-corpus-FuzzRestoreVMSnapshotDoesNotPanic of run 42750434626, so that artifact is exactly the corpus replayed. scripts/fuzz_retention.py restore copies each record into the native cache named by its sha256.
E4. Fuzz target vm/persistence_verify_test.go:183-224: ticks capped at 100, `task.NewState(1, player, 100, 1)`, constants up to 65535, locals up to 255, arbitrary code if the restore verifier accepts it. Fixture validRestoredSnapshot (vm/persistence_verify_test.go:15-20) leaves VerbDebug false.
E5. Run 42750434626 already hit this family: 5 "hung or terminated unexpectedly: exit status 2" findings (1c6e5faafb5575af, a67f1c049e52d97e, db7bf09e2599b51d, 021b3e3abaa49095, cccf71beaee136fa), all with code prefix `00!!!` and suffix `...\xc1!\xaf\x00\x10\xb30\xb3\xb3\xbb`.
E6. Baseline order: f.Add (4) and testdata (8, all also in the retained set) first, then the native cache sorted by name. In campaign 3, in-flight positions 2019-2020 map to sorted cache indices 2016-2017 = a6698923a2e4da2d... and a67f1c049e52d97e...
E7. Local replays (Windows, test binary built from worktree at dc0cf92b; vm/ and builtins/ differ from d2457f0f only in fuzz testdata and vm/world_history_test.go), `GOMEMLIMIT=1GiB GODEBUG=gctrace=1`, bounded by `-test.timeout` and `timeout`:
  - a6698923a2e4da2d: PASS immediately, 1 GC.
  - a67f1c049e52d97e: never returns; live heap (third gctrace number) 1 MB at 0.01 s, 56 MB at 0.28 s, 212 MB at 0.92 s, 517 MB at 2.13 s, 1100 MB at 9.9 s (growth continues past GOMEMLIMIT; GC then thrashes). Timeout stack: `(*VM).Execute` vm/vm.go:1368 (OP_FOR_LIST_LOAD `return newMooError(types.E_TYPE, "for loop iterator is not a list")`) and vm/vm.go:1153, under executeLoop vm/vm.go:968. This is the OOM input.
  - 021b3e3abaa49095 (sha256 verified): never returns, live heap flat ~5 MB over 15 s, stack in OP_LOOP case vm/vm.go:1298-1300. Pure CPU hang (issue #695), not memory.
E8. Disassembly of a67f1c049e52d97e via bytecode.Disassemble (scratchpad probe disasm/main.go):
  ```
   6: FOR_RANGE_CHECK 175 175 off 0     (falls through either way)
  11: DUP   12: PUSH 33   14: IMM 20   15: BITXOR   16: POP   17: BITSHR
  18: FOR_LIST_LOAD 169 169 33 33
  23: IMM 35   24: IMM 35
  25: FOR_RANGE_NEXT var 33, end 175, loopOffset 16   -> IP 30-16 = 14
  30..34: BITAND IMM BITAND BITAND RETURN
  ```
  Loop 14..25. Tick kinds (bytecode/opcodes.go:447-486): IMM, POP, FOR_LIST_LOAD, FOR_RANGE_NEXT are tick-free; BITXOR and BITSHR (OP_SHR) are tickUnchecked. No checked tick in the cycle.
E9. Why the loop never stops: chargeTicks vm/vm.go:437-450 tests TickLimit only for checked ticks; unchecked ticks only reach tickCheckpoint every 1024 ticks (vm/vm.go:446), which stops only when `vm.Task.SecondsLeft() <= 0` (vm/vm.go:488). SecondsLeft task/task.go:821-828 returns `SecondsLimit - SecondsUsed` (= 1, constant) when no execution deadline is set; the harness never calls SetExecutionDeadline (production does: engine/call_verb.go:399, engine/task_runtime.go:455).
E10. Why the verifier accepts it: verifyLoopsTick bytecode/verify.go:73-96 rejects only cycles where `checked|unchecked == 0`; for OP_TICKS it counts `operands[0] &^ TicksUnchecked`. An unchecked-only cycle passes. This is deliberate for compiled for-list loops, whose per-iteration test FOR_LIST_CHECK_WIDE is tickUnchecked (Toast EOP_FOR_LIST, bytecode/opcodes.go:470), and compiler/lower_control.go:135,352,369 emit `OP_TICKS 1|TicksUnchecked`; compiled for-list loops are bounded by the list length, arbitrary bytecode is not.
E11. Why memory grows: the verifier's stack heights treat FOR_LIST_LOAD as net 0, so the cycle is balanced on paper. At run time Locals[169] is not a list, FOR_LIST_LOAD returns E_TYPE (vm/vm.go:1367-1368), and because VerbDebug is false the generic path pushes the error as a value and continues (vm/vm.go:978-988, `vm.Push(types.NewErr(errCode))`). Net +1 stack slot per iteration; Push (vm/stack.go:13-20) appends with no depth limit, so vm.Stack grows without bound at interpreter speed.

## Hypotheses

H1 (strong, matches E7 timing): the guest OOM is a67f1c049e52d97e replayed in one of the two workers of campaign 3 (E6), growing at hundreds of MB/s (E7) until the 16 GB guest (shared with 3 other lanes) ran out ~25 s after the stall. Not proven that it was a67f1c rather than another family member in flight, but a67f1c is the in-flight record that reproduces the growth; a6698923 passes.
H2: production exposure: the same bytecode restored from a database suspended task would be stopped by the seconds deadline (E9), but during those seconds the stack would grow at the same rate (several GB in a 5 s budget). Not measured.

## Proposed smallest fix (not implemented)

Barn-owned, two places, pick per Q:
1. Harness (stops both the #695 hang and this OOM in the fuzzer): in vm/persistence_verify_test.go after creating machine.Task, set an execution deadline (`machine.Task.SetExecutionDeadline(time.Now().Add(time.Second))`) as production does. Bounds time; memory is still whatever ~1 s of growth costs (~200-500 MB per worker locally).
2. Production root (bounds memory): the restore verifier/VM must not let stack height drift under !VerbDebug error pushes. Options: make RestoreVMSnapshot reject cycles with no checked tick unless they pass through FOR_LIST_CHECK_WIDE (the only compiled unchecked-only loop test), or cap vm.Stack growth at the verifier's computed max height + per-frame slack. Option 1 of these is narrower and matches the verifyLoopsTick comment's stated intent ("Compiled loops always pass through a ticking test").

Toast comparison: not needed. The input is verifier-accepted bytecode no compiler emits; the defect is Barn's verifier/harness. The !VerbDebug error push itself (vm/vm.go:978-988) cites Toast PUSH_ERROR and is not proposed to change.

## Next step

Team lead decides fix; then add a67f1c049e52d97e and 021b3e3abaa49095 as checked-in regression inputs only together with the fix (they hang go test ./vm today).
