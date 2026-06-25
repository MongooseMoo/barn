# Concurrency / Task-Lifecycle Review

Packages reviewed: `scheduler/`, `task/`, `kernel/`

Pre-existing baseline: all tests pass, `go test -race` clean.

---

## Architecture Summary — Dual Task Ownership

### Who is authoritative?

There are two completely separate task-tracking systems with independent ID counters:

**`scheduler.Scheduler`** (`scheduler/scheduler.go:22`)
- `tasks map[int64]*task.Task` — the set of tasks the scheduler actively runs
- `nextTaskID int64` — incremented via `atomic.AddInt64` per task creation
- Protected by `s.mu sync.Mutex`
- Source of truth for: `ProcessReadyTasks`, `liveTaskVMs`, `TaskSnapshots` (checkpoint), `runTask`

**`task.Manager`** (global singleton, `task/manager.go:11`)
- `tasks map[int64]*Task` — the set the builtins see
- `nextTaskID int64` — a completely separate counter, also starts at 1
- Protected by `m.mu sync.RWMutex`
- Source of truth for: `kill_task`, `resume_task`, `queued_tasks`, `task_id`, `suspend`/`resume`

### How they sync

`Scheduler.QueueTask` calls `task.GetManager().RegisterTask(t)` — every task created through the normal scheduler path (foreground, background, fork, server-verb) appears in both maps under the same ID.

### The gap

`scheduler/eval.go:EvalCommandOutput` creates tasks via `manager.CreateTask()` (manager counter) and **never** puts them in `s.tasks`. Those eval tasks are:
- Invisible to `ProcessReadyTasks` (scheduler can't run them autonomously)
- Invisible to `TaskSnapshots` (checkpoint will not persist them)
- Invisible to `liveTaskVMs` (GC boundary won't include them)

### ID divergence

Both counters start at 1 and grow independently. After a checkpoint restore, `task_load.go` advances `s.nextTaskID` to `max(restored IDs)` via a CAS loop, but **`m.nextTaskID` is never updated**. If restored tasks had IDs up to N, the manager will issue IDs 2..N to subsequent eval tasks — all of which collide with restored scheduler-task IDs 2..N that the manager already has (via RegisterTask). The second registration (`QueueTask → RegisterTask`) silently overwrites the eval task's manager entry, making the eval task unreachable by builtins for the duration of its execution.

### Can IDs and task sets diverge in production?

Yes, in two scenarios:
1. **Always, after checkpoint restore**: manager IDs start lower than scheduler IDs, collision window is guaranteed.
2. **Within a session if eval commands and scheduler tasks both run**: since both counters start at 1 and grow monotonically, eventually they produce the same value. The only mitigation is that `EvalCommandOutput` creates and removes its task synchronously, so the window is narrow.

---

## Findings

### CRITICAL

#### CONFIRMED-1: Data race on `Task.BytecodeVM` between `runTask` and `liveTaskVMs`

**Files:** `scheduler/task_runtime.go:239,314` (write) / `scheduler/scheduler.go:189` (read)

`runTask` writes `t.BytecodeVM = bcVM` (line 239, FlowSuspend path) and `t.BytecodeVM = nil` (line 314, completion path) with **no lock held** — neither `s.mu` nor `task.mu`.

`liveTaskVMs` reads `queued.BytecodeVM.(*vm.VM)` at scheduler.go:189 while **holding `s.mu`**, but not `task.mu`.

Holding `s.mu` for the read does not protect against the unguarded write. When the main/checkpoint goroutine calls `RunServerVerbTask` (→ `runTask`) concurrently with the InputProcessor goroutine calling `ProcessReadyTasks` (→ `runTask` → `liveTaskVMs`), the two goroutines race on `BytecodeVM` of any task that is in `s.tasks` during both executions.

**Test:** `TestReview_BytecodeVMDataRaceLiveTaskVMsVsRunTask` in `scheduler/review_concurrency_test.go`

**Confirmed output (`go test -race ./scheduler/ -run TestReview_BytecodeVMDataRaceLiveTaskVMsVsRunTask -count=5`):**
```
WARNING: DATA RACE
Read at 0x00c0004ae100 by goroutine 19:
  barn/scheduler.(*Scheduler).liveTaskVMs()
      barn/scheduler/scheduler.go:189
  barn/scheduler.(*Scheduler).runTask()
      barn/scheduler/task_runtime.go:307

Previous write at 0x00c0004ae100 by goroutine 18:
  barn/scheduler.(*Scheduler).runTask()
      barn/scheduler/task_runtime.go:239
```

**Fix direction:** protect `BytecodeVM` accesses with `task.mu` uniformly, or ensure that only the goroutine running a task can read/write its `BytecodeVM` (no external reader).

---

### HIGH

#### CONFIRMED-2: `ProcessReadyTasks` closes `t.Done` on suspended tasks

**File:** `scheduler/scheduler.go:164-168`

```go
for _, t := range readyTasks {
    err := s.runTask(t)
    // ...
    if t.Done != nil {
        close(t.Done)  // unconditional
    }
}
```

`runTask` returns `nil` for **both** `FlowSuspend` (task alive, will resume) and terminal completion. The `close(t.Done)` fires after every `runTask` return regardless of whether the task actually finished. Any goroutine waiting on `<-t.Done` as a "task completed" signal wakes spuriously on suspension.

`task.Task.Done` is currently never set by any barn code path (it is dead code at this moment), so no production defect fires today. The bug is latent and will activate the moment `Done` is wired up.

**Test:** `TestReview_DoneChannelClosedOnSuspend`

**Red output:**
```
review_concurrency_test.go:72: BUG: ProcessReadyTasks closed t.Done on a suspended
(not completed/killed) task; waiters will falsely believe the task has terminated
--- FAIL: TestReview_DoneChannelClosedOnSuspend (0.00s)
```

**Fix direction:** only `close(t.Done)` when task state is `TaskCompleted` or `TaskKilled`.

---

#### CONFIRMED-3: Independent `nextTaskID` counters produce colliding task IDs

**Files:** `scheduler/task_factory.go` / `task/manager.go:34`

`scheduler.nextTaskID` and `manager.nextTaskID` are independent `int64` counters, both starting at 1. `EvalCommandOutput` (`scheduler/eval.go:63`) calls `manager.CreateTask()` to obtain an ID from the manager's counter. All other creation paths use `atomic.AddInt64(&s.nextTaskID, 1)`.

When the manager counter and the scheduler counter reach the same value simultaneously, both systems issue the same task ID to different `*task.Task` objects. `QueueTask` then calls `RegisterTask`, overwriting the manager's entry for that ID — the eval task becomes unreachable by `kill_task` / `resume_task` / `queued_tasks`. The eval task's actual kill/resume can no longer be addressed by builtins.

After checkpoint restore this collision is **guaranteed**: `task_load.go` advances `s.nextTaskID` to the max restored ID but leaves `m.nextTaskID` at its pre-restore value, so manager IDs 2..max_restored collide with restored scheduler-task IDs 2..max_restored.

**Test:** `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`

**Red output:**
```
review_concurrency_test.go:134: BUG: ID collision at 3 — manager.CreateTask and
scheduler.QueueTask produced the same task ID from independent counters; the eval
task was overwritten in manager.tasks and is no longer reachable by kill_task/
resume_task/queued_tasks
--- FAIL: TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent (0.00s)
```

**Fix direction:** eliminate the dual counter. `EvalCommandOutput` should allocate its task ID from `s.nextTaskID` (or any single authoritative counter), not from the manager's own counter.

---

### MEDIUM

#### SUSPECTED-4: `EvalCommandOutput` blocks the scheduler goroutine for up to 10 seconds on indefinite `suspend()`

**File:** `scheduler/eval.go:140-178`

When MOO code inside `EvalCommandOutput` calls `suspend()` (indefinite), eval.go enters a busy-wait spin loop calling `s.ProcessReadyTasks()` every 10ms for up to 10 seconds, then gives up with `E_INVARG`. This is not a deadlock (s.mu is not held during the spin), but it blocks the InputProcessor goroutine (which calls EvalCommandOutput) for the entire wait. All other task execution, input processing, and ticker-driven work stalls for up to 10 seconds.

**Impact:** The design also causes `ProcessReadyTasks` to be called re-entrantly — the ticker would fire on the InputProcessor goroutine which is already running `ProcessReadyTasks` via `EvalCommandOutput`. Since the goroutine is blocked in the spin loop, the ticker's `ProcessReadyTasks` call is actually safe (it won't re-enter). But the 10-second stall is a server responsiveness bug.

---

#### SUSPECTED-5: `CancelLoginTasksFor` writes `t.ReadingPlayer` without `task.mu`

**File:** `scheduler/task_factory.go:352`

```go
t.ReadingPlayer = types.ObjNothing  // no task.mu held
t.Kill()
```

`task.Task.ReadingPlayer` is read by `Manager.FindReadingTask` while holding `m.mu.RLock` but without `task.mu`. The write at CancelLoginTasksFor is also without `task.mu`. Similarly, `ResumeReadingTask` writes `t.ReadingPlayer = types.ObjNothing` without `task.mu`.

In practice, all paths run on the same InputProcessor goroutine, so the race never actually interleaves. The field discipline is still inconsistent: some accesses treat it as protected by `task.mu`, others don't.

---

### LOW / ARCHITECTURAL

#### ARCH-1: `kernel.TaskContext` stores `Task`, `CallerVM`, `Registry` as `interface{}`

**File:** `kernel/context.go:46-66`

Three fields use `interface{}` to break import cycles. Every builtin that needs these must type-assert, with no compile-time guarantee of the correct type. A wrong value stored silently produces a nil pointer or runtime panic at the assertion site. The comment documents the intent ("Import cycle prevention"), but the risk is real as the codebase grows.

#### ARCH-2: Func-pointer wiring on `Scheduler` (`taskLineSender`, `tracebackSender`, etc.)

**File:** `scheduler/scheduler.go:28-32`

Four `func` fields are set once during server startup via setters, then treated as immutable. There are no locks on the reads. This is safe only because barn has a single-writer/multiple-reader startup contract. If any setter is called after task execution begins, the write races with concurrent reads. Low current risk; latent.

#### ARCH-3: `CallVerbWithArgstr` creates an unregistered throwaway task with ID=0

**File:** `scheduler/call_verb.go:36-39`

```go
t := &task.Task{
    Owner: player,
    ...
}
```

No ID is assigned, no registration with manager or scheduler. `task_id()` called from within a server hook (do_login_command, user_connected, etc.) returns 0 — which is the system object, not a valid task ID. This differs from Toast's behavior where every activation has a real task ID.

#### ARCH-4: `task.Task.Done` field is dead code

**File:** `task/task.go:159`

The `Done chan struct{}` field is declared but never assigned by any barn code. The `close(t.Done)` guard in `ProcessReadyTasks` always short-circuits on `nil`. This is why CONFIRMED-2 is latent rather than immediately active.

#### ARCH-5: `liveTaskVMs` iterates `s.tasks` while `s.mu` is held; `GetState()` acquires `task.mu` inside

**File:** `scheduler/scheduler.go:180`

Lock order: `s.mu → task.mu` (via `queued.GetState()`). This ordering is consistent across the codebase (manager similarly takes `m.mu → task.mu`). No deadlock risk detected — but any new code must respect this ordering.

---

## Test File

`C:/Users/Q/code/working/barn/scheduler/review_concurrency_test.go`

All three `TestReview_*` tests are new. The two deterministic tests fail immediately; the race test requires `-race -count=5` or more to reliably trigger.

Pre-existing tests unchanged; baseline still passes after adding the test file.
