# Fix: anonymous recycle after suspend(0) flake

## Goal

Find and fix the cause of the intermittent failure of
`builtins/recycle_keeps_shared_anonymous.yaml::anonymous_held_only_by_recycled_object_is_destroyed`
(expected 1, got 0) seen once in the merge-queue run for PR 1035 (CI run
44891357917, job 136808099622). Saved log: `C:/Users/Q/code/barn/.tmp/pr1035-mq-conf.log`.

Done: cause stated with file:line evidence, fix committed locally in the
worktree(s) it belongs to, this note committed. Not included: push, PR, posting.

## State

Cause found (Barn ordering bug, class (a)); fix and Go tests in the working
tree of branch `fix/anonymous-recycle-after-suspend-flake`, committed locally.
No suite change (the test asserts what Toast guarantees). Not pushed, no PR.

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

- Not measured: Mongoose real-workload throughput (`TestMongooseRealWorkload`
  1p/16p). Every suspend that queues finalization now forces a prompt sweep
  instead of waiting for the 2 s cadence; on Mongoose that may cost throughput
  and must be measured on an idle machine before merge.
- Lead: push, PR, CI.
