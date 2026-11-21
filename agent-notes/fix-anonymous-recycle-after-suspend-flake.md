# Fix: anonymous recycle after suspend(0) flake

## Goal

Find and fix the cause of the intermittent failure of
`builtins/recycle_keeps_shared_anonymous.yaml::anonymous_held_only_by_recycled_object_is_destroyed`
(expected 1, got 0) seen once in the merge-queue run for PR 1035 (CI run
44891357917, job 136808099622). Saved log: `C:/Users/Q/code/barn/.tmp/pr1035-mq-conf.log`.

Done: cause stated with file:line evidence, fix committed locally in the
worktree(s) it belongs to, this note committed. Not included: push, PR, posting.

## State

Cause found (Barn ordering bug, class (a)). The hold fix is committed and
published as PR 1040, but it must not merge: it cuts Mongoose goodput about
15-20x and fails the Elle check (both below). A replacement design is an
open choice for the lead. No suite change (the test asserts what Toast
guarantees).

## Findings

### Toast guarantees the order (upstream `origin/master`)

- `src/utils.cc:245-275` (`complex_free_var`, TYPE_ANON): when an anonymous
  object's refcount reaches zero and it is neither RECYCLED nor INVALID, it is
  `queue_anonymous_object(v)` (not destroyed inline). `recycle(h)` frees h's
  property values, so the anonymous child of `p` is queued during `recycle(h)`.
- `src/server.cc:633-673` `recycle_anonymous_objects()`: drains the queue,
  sets FLAG_RECYCLED, calls `run_server_task(-1, v, "recycle", ...)`
  synchronously, then destroys the object.
- `src/server.cc:878-883` main loop: `recycle_anonymous_objects();
  recycle_waifs(); network_process_io(); run_ready_tasks();` every iteration.
- `src/tasks.cc:1629-1823` `run_ready_tasks()`: promotes due waiting tasks, then
  runs at most one task (`did_one`) and returns. A task that calls `suspend(0)`
  is put in the waiting queue during its run, so it can resume only in a later
  `run_ready_tasks()` call, and every main-loop iteration before that call has
  run `recycle_anonymous_objects()`. So on Toast the queued anonymous object's
  `recycle` verb has run to completion before the `suspend(0)` task resumes:
  `k.hits` is 1, deterministically.
- Suite commit "Pin that recycling an object keeps anonymous objects held
  elsewhere" (from barn#939) claims only that Toast's bf_recycle frees the
  property values and an anonymous object dies with its last reference; it
  makes no claim about timing. The ordering above is what makes `suspend(0)`
  sufficient on Toast.

Classification so far: not (b). The test asserts an order Toast guarantees.

### Barn: recycle of the orphan is deferred and the resume is not ordered after it

- `engine/task_runtime.go:878-891` `handOffSuspended`: at `suspend(0)` the
  slice's anonymous garbage is only queued (`deferAnonGC`), the VM is saved,
  and the task is requeued.
- `engine/waif_lifecycle.go:111-140` `deferAnonGC` appends to
  `lifecycle.PendingAnonGC`; the recycle verb runs only when a later flush
  drains that batch (`waif_lifecycle.go:297-382`,
  `vm.RecycleOrphanAnonymousBatch`).
- Flushes: `task_runtime.go:225` (every slice boundary),
  `runtime.go:614/623/633` (after a pass / batch / dispatched task), all
  `flushDeferredGC()` = `flushDeferredGCBarrier(0, false)`. That flush gives up
  without sweeping when (`waif_lifecycle.go:251-293`): SweepMu or VMStartMu is
  held (another flush, a checkpoint), or the last sweep cost >= 50 ms
  (`cheapGCSweep`) and the last sweep was < 2 s ago, or any other VM is
  executing (`collectAllGCRefs` not quiescent). The only other sweeper is the
  maintenance worker (`deferred_gc_maintenance.go:21-39,41-66`), whose deadline
  is `PendingSince + 2s` (or `LastGCSweep + 2s` when sweeps are expensive).
- Nothing in the scheduler (`engine/scheduler.go:337-424` `readyLocked`) holds a
  suspended task back while the batch its own suspend queued is still pending.
  A `suspend(0)` task is due immediately, so when the boundary flush gives up
  (another task executing, or an expensive earlier sweep under CPU load), the
  task resumes, reads `k.hits` (0), and the orphan's `recycle` runs up to 2 s
  later.

Classification: (a) Barn ordering bug. Toast runs the queued recycle before
any later task slice; Barn's boundary flush is best-effort and the resume is
not ordered after it.

### Reproducer (Go, deterministic)

`engine/suspend_finalization_order_test.go` runs the conformance statement
(with `#-1`/`#0` for `$nothing`/`player`) on a runtime with real builtins:
- `TestSuspendResumesAfterQueuedOrphanRecycleWhenSweepThrottled`: last sweep
  marked expensive and recent. Unchanged master: `k.hits after suspend(0) = 0`.
- `TestSuspendResumesAfterQueuedOrphanRecycleBesideRunningTask`: another task
  spinning in a VM. Unchanged master: `= 0`.
- Control: the throttled test with `LastGCCost = 0` passes on master (the
  boundary flush sweeps), so the test itself is sound.

### Fix

A task whose suspend queued deferred finalization is held until the sweep
that drains that queue has run:
- `engine/internal/finalization/coordinator.go`: `QueuedSeq` (numbered per
  queued request), `SweptSeq` (atomic, highest drained), `WaitSeq` (highest a
  held task waits for).
- `engine/deferred_gc_maintenance.go`: `noteDeferredGCQueuedLocked` numbers
  the request; once a slice-boundary flush has left a held task unswept
  (`WaiterUnserved`), the maintenance deadline is now and a failed rendezvous
  retries after `maintenanceBarrierPatience` instead of 2 s.
- `engine/waif_lifecycle.go`: `deferPendingWaifs`/`deferAnonGC` return the
  number; `holdForFinalization`; a sweep records `SweptSeq` for what it drained
  and notifies the scheduler; the cheap-sweep throttle is skipped while a task
  waits; `flushDeferredGC` marks `WaiterUnserved` and wakes the worker when
  it could not sweep for a held task; the shutdown hand-off
  (`takeDeferredFinalizationRootsLocked`) also releases waiters.
- `engine/task_runtime.go` `handOffSuspended`: holds the task on the number of
  what it queued.
- `task/task.go`: `HoldForFinalization`; `readyDeadlineLocked` returns zero
  (not ready, no timer) until `SweptSeq` reaches it.

Tests: the two anonymous tests above plus
`TestSuspendResumesAfterQueuedWaifRecycleWhenSweepThrottled` and
`TestSuspendResumesAfterQueuedWaifRecycleBesideRunningTask` (WAIF dropped
before `suspend(0)`, Toast `recycle_waifs()`); all four fail on unchanged
master (stash of the production files) and pass with the fix.

Not changed (scope): Barn still lets a *different* task run before an earlier
task's queued finalization is swept (the deliberate deferral documented at
`waif_lifecycle.go` `deferPendingWaifs`). Toast orders that too. The fix covers
the task that queued the garbage, which is what the test and the same-task
waif case observe.

### Runs

Go (Windows workstation):
- `go test ./engine -run 'TestSuspendResumesAfterQueued' -count=1`: 4 FAIL on
  unchanged production files (`k.hits after suspend(0) = 0`), 4 PASS with fix.
- Final code: `go test ./engine/... ./task ./vm ./server/... ./builtins
  -count=1`: ok. `go test -race ./engine ./task -count=1`: ok (engine 473 s).
  `-race -count=30` of the suspend tests plus
  `TestSuspendFinalizesReleasedWaifBeforeResume`: all pass.
- `gofmt -l engine task`: clean. `go vet ./engine/... ./task`: clean.
  `golangci-lint run ./engine/... ./task`: 0 issues. `staticcheck`: clean.
- `go test ./server/... ./builtins ./internal/... -count=1`: ok.

Managed harness (WSL Debian, suite worktree copied to `/root/anon-flake-suite`
because collection over `/mnt/c` took 7 min; Test.db
sha256 `1a3f23eb...d408e`, 2035 bytes, the suite worktree's file;
`-k "capability_admission or recycle_keeps_shared_anonymous"`):
- Toast `/root/src/toaststunt/build-release/moo`, target manifest
  `profiles/toast/stock-wsl-testdb.json`: 5 passed (load 16.9).
- Barn with fix (`bin/barn-linux-amd64`, sha256 `226133a0...392d`): 5 passed
  (load 14.3).
- Barn unchanged master (`bin/barn-master-linux-amd64`, sha256
  `39b2b9ec...782e`) x10: 10/10 passed (load 12.4-24.3). In isolation the
  boundary flush is quiescent and cheap, so the window does not open; in the
  full suite it needs another VM executing or an expensive recent sweep, which
  the Go tests force.

- Full managed suite, final Barn (`bin/barn-linux-amd64`, sha256
  `8e1ad416...d193`), same command without `-k`: 13768 passed, 325 skipped,
  0 failed, 610 s (load 14.2 at start, 6.0 at end).

Scripts: scratchpad `run_conf.sh` / `loop_conf.sh` (not committed).

### Second failure: PR 1032 merge queue, same class

`audit/divergence_waif_statement_semantics.yaml::waif_recycle_verb_called_on_release`
got `[]` for `[1]` (log `C:/Users/Q/code/barn/.tmp/pr1032-mq-conf.log`, line
680). Its statement: `w = c:new(); w = 0; suspend(0); r = #0.dwf_log;`. The
released WAIF is queued by `deferPendingWaifs` in `handOffSuspended`
(`engine/task_runtime.go`), the same deferred batch and the same unordered
resume as the anonymous case; Toast's `recycle_waifs()` runs in the same
main-loop spot as `recycle_anonymous_objects()` (`server.cc:878-883`). Same
class, same fix, no separate commit. The two WAIF Go tests run that statement
shape; the existing `TestSuspendFinalizesReleasedWaifBeforeResume`
(`engine/task_runtime_test.go`) is the same statement on a quiescent runtime,
which is why it never caught the window.

### PR 1035

Its diff (`git diff origin/master...origin/fix/callers-call-function-frame`)
touches frame naming for builtin callbacks (`vm/registry.go`, `vm/stack.go`,
`vm/op_misc.go`, `vm/op_verb.go`, `vm/persistence.go`, `builtins/tasks.go`,
`task/traceback.go`). It does not touch `engine/waif_lifecycle.go`,
`engine/deferred_gc_maintenance.go`, `engine/scheduler.go`,
`engine/task_runtime.go` or `vm/anonymous_gc.go`, i.e. nothing that decides
when the deferred batch is swept or when the suspended task resumes. Not (c);
the window above exists on master.

### How long a held task can wait

The hold ends only when `SweptSeq` reaches the task's number
(`task/task.go` `readyDeadlineLocked`), i.e. when a sweep drains its batch
(`engine/waif_lifecycle.go` `markFinalizationSweptLocked(drained)` in the sweep's
deferred cleanup) or shutdown takes the batches
(`takeDeferredFinalizationRootsLocked`). What keeps a sweep from running:
- `FinalizationHeld` (startup, until `#0:server_started` returns,
  `engine/runtime.go` `ReleaseStartupFinalization`): flush and worker both stand
  down (`waif_lifecycle.go:294`, `deferred_gc_maintenance.go:26`); the release
  flushes. Toast also runs startup finalization only after server_started.
- Another sweep running (`GCRunning`): it ends, wakes the worker.
- A lease that does not park. A sweep needs every execution lease parked
  (`engine/gc_handshake.go:55-77` `awaitGCParked` / `leasesParkedLocked`,
  `engine/runtime.go:703-730` `collectAllGCRefs`). A VM parks only at its
  1024-tick checkpoint (`vm/vm.go:439-472`, `engine/task_runtime.go:463-471`),
  never while it holds the commit gate, while inside a builtin (an outer VM with
  a nested lease, `count != 1`), or while `TaskRunning` without a lease. The
  worker retries every `maintenanceBarrierPatience` = 20 ms
  (`waif_lifecycle.go:267`, `deferred_gc_maintenance.go:85-88`).

So the wait has no fixed bound: it lasts until one 20 ms window finds every
running VM parked or finished. One unparked VM delays it by that VM's stretch
without a checkpoint: at most its slice's seconds limit for MOO code, and
unbounded by Barn for a builtin that blocks inline. Under constant load the
windows can keep failing, and the held task waits for all of them. Before
the fix the sweep was delayed the same way, but the task did not wait for it.

### Throughput: the fix is a large regression (TestMongooseRealWorkload)

Interleaved A/B in WSL, base = merge base with origin/master (detached worktree
`C:/Users/Q/code/barn-anon-perf-base`), fix = this branch's commit; goodput-series
shape (fixture `mongoose.db.new.2026-11-12T0055` sha256 `626bd284...25e7`,
sound sqlite `0fcdc8d7...9318`, players 1,16, warmup 5 s, measure 30 s,
GOGC 400, linux `go test -c` binaries, CGO off). Script: scratchpad
`perf_ab.sh`. 1-minute load before each run in the last column.

| pair | base 1p | fix 1p | base 16p | fix 16p | load before base / fix |
|---|---|---|---|---|---|
| 1 | 62/s | 3/s | 140/s | 19/s | 12.1 / 18.7 |
| 2 | 32/s | 3/s | 227/s | 19/s | 20.5 / 25.9 |
| 3 | 52/s | 1/s | 80/s | 14/s | 19.3 / 27.7 |
| 4 | 39/s | 2/s | 208/s | 14/s | 22.1 / 17.8 |
| 5 | 58/s | 3/s | 306/s | 24/s | 14.3 / 11.3 |

Base spread is wide (1p 32-62, 16p 80-306) because the machine was at load
11-28, but every fix run is far below the slowest base run (1p max 3 vs min
32; 16p max 24 vs min 80), so the data does distinguish the builds: the fix
cuts goodput about 15-20x. p50 latency 7-16 ms base vs 148-220 ms fix (1p);
allocs/op about 30k vs 600k and bytes/op about 4 MB vs 150 MB (1p), i.e.
every command now waits on and pays for a full sweep. Every run of both builds
ended `--- FAIL: TestMongooseRealWorkload` (failed 1-4 or uncaught 1 at some
level, in base as well), so the harness verdict does not separate them.

Why (inference from the counters, not traced per command): on Mongoose nearly
every command suspends after its slice queued a WAIF or anonymous request, so
each command is held for a sweep, and a Mongoose sweep costs on the order of
the 150-200 ms p50 added. That cost is why sweeps are batched on a 2 s cadence
once they exceed 50 ms.

### Elle check failure on PR 1040: caused by the hold

`TestElleIsolationHistories` (`-tags elle`, seeds 412/395/396, 40 rounds,
GOMAXPROCS 4, the `scripts/check_elle.py` command) fails on the fix with
`elle_workload_test.go:757: unsettled task 1001xx: queued`, and passes on the
base worktree for the same three seeds. `elleDrain`
(`engine/elle_workload_test.go:195-214`) treats `ProcessReadyTasks() == 0` as
"everything settled"; with the hold, a suspended task whose slice-boundary and
end-of-pass flushes could not sweep (parallel batch siblings executing, or the
worker holding SweepMu) is queued but not ready until the maintenance worker
sweeps asynchronously, so the pass returns 0 with the task unsettled. The same
assumption made `TestSuspendFinalizesReleasedWaifBeforeResume` flaky earlier.

## Decisions and why

- Hold only the task whose own suspend queued the garbage. Toast also orders
  other tasks after the queued recycle; Barn's batching (2 s cadence once a
  sweep costs >= 50 ms) is a deliberate departure for large databases, and
  removing it would force a sweep before every task start.
- The maintenance worker is not woken at hold time. First version did, and
  `TestSuspendFinalizesReleasedWaifBeforeResume` failed 7/20 under `-race`
  (`task made no progress in state queued`): the worker took SweepMu and waited
  for the still-leased suspending VM to park, so the slice-boundary flush lost
  TryLock and the task stayed held for one more pass. Now the boundary flush
  goes first and only its failure arms the worker (30/30 under `-race` after).

## Surprises

- In isolation the managed test passes 10/10 on unchanged master: the window
  needs another VM executing or an expensive recent sweep at the moment the
  test task suspends, which happens in the full suite under CI load.

## Next step

- Do not merge PR 1040 as is (throughput table and Elle section above).
- Choice pending with the lead: (1) hold only while sweeps are cheap
  (`LastGCCost < cheapGCSweep`), so small databases such as Test.db get Toast's
  order and Mongoose keeps the batched deferral, plus make the end-of-pass flush
  in `ProcessReadyTasks` wait (patience, handshake) while a task is held so
  drivers that read 0 as settled (Elle, `TestSuspendFinalizesReleasedWaifBeforeResume`)
  stay correct; or (2) withdraw the hold and leave the flake until finding an
  orphan costs O(released values) instead of a full sweep (Toast does it by
  refcount).
