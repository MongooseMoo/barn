# Fix F6 — data race on `Task.BytecodeVM`

## The race
`runTask` wrote `t.BytecodeVM` with **no lock** (suspend save, completion clear,
deadline-kill clear, forked zero-delay re-yield). Meanwhile `liveTaskVMs` and
`ProcessReadyTasks`' readiness scan **read** other tasks' `BytecodeVM` while
holding only `s.mu`. `s.mu` does not serialize against the lock-free writes in
`runTask`, so when one goroutine runs a task (main/checkpoint `RunServerVerbTask`)
while another runs `ProcessReadyTasks`, the two genuinely race on the field. The
authority here is Go's race detector, not ToastStunt (single-threaded C++ does
not map to Go's memory model).

## Fix
`Task` already guards every other mutable field (`State`, `CallStack`, ticks,
`TaskLocal`, …) through `t.mu`-protected accessors. I followed that existing
pattern rather than adding a new lock: added two accessors on `task.Task`
(`task/task.go`):

- `BytecodeVMValue() interface{}` — `RLock`
- `SetBytecodeVM(machine interface{})` — `Lock`

and routed **every** read/write of the field through them.

### Every call site found (LSP/grep across `scheduler` + `task`) and how it is now synchronized

| File:line | Access | Now |
|---|---|---|
| `task_runtime.go:55` | read (resume detection) | `t.BytecodeVMValue()` |
| `task_runtime.go:88/91` | read (retrieve saved VM) | single `t.BytecodeVMValue()` |
| `task_runtime.go:216` | write nil (deadline kill) | `t.SetBytecodeVM(nil)` |
| `task_runtime.go:222` | write bcVM (forked zero-delay re-yield) | `t.SetBytecodeVM(bcVM)` |
| `task_runtime.go:239` | write bcVM (suspend save) | `t.SetBytecodeVM(bcVM)` |
| `task_runtime.go:314` | write nil (completion release) | `t.SetBytecodeVM(nil)` |
| `task_load.go:86` | write machine (checkpoint restore) | `t.SetBytecodeVM(machine)` |
| `task_factory.go:242` | write childVM (fork creation) | `t.SetBytecodeVM(childVM)` |
| `scheduler.go:146` | read (readiness scan, under `s.mu`) | `t.BytecodeVMValue()` |
| `scheduler.go:189` | read (`liveTaskVMs`, under `s.mu`) | `queued.BytecodeVMValue()` |

The two construction-time writes (`task_load`, `task_factory`) operate on a task
not yet visible to other goroutines, but are routed through the accessor anyway
for uniformity.

## Lock ordering
The accessors are **leaves**: they take only `t.mu` and never acquire `s.mu` (or
any other lock). The scheduler's two reads call them while holding `s.mu`, giving
a single consistent order **`s.mu` → `task.mu`**. No path holds `task.mu` and
then acquires `s.mu`, so no inversion / deadlock is possible. In `runTask`'s
suspend path the `SetBytecodeVM` at :239 fully releases `task.mu` before `s.mu`
is taken at :241 — the locks are never nested there.

## Test results

### `go test -race ./scheduler/ -run TestReview_BytecodeVMDataRaceLiveTaskVMsVsRunTask -count=30 -v`
All 30 iterations PASS, no `DATA RACE` reported:
```
=== RUN   TestReview_BytecodeVMDataRaceLiveTaskVMsVsRunTask
--- PASS: TestReview_BytecodeVMDataRaceLiveTaskVMsVsRunTask (0.00s)
... (30 iterations) ...
PASS
ok  	barn/scheduler	1.469s
```

### Whole packages under `-race` (`go test -race ./scheduler/ ./task/`)
No `DATA RACE` anywhere. `barn/task` = ok. `barn/scheduler` reports only the two
**pre-existing, intentionally-red review tests for OTHER findings** (not F6):
- `TestReview_DoneChannelClosedOnSuspend` (BUG-1: Done closed on suspend)
- `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent` (BUG-2)

### Functional before/after (`go test ./scheduler/... ./task/...`)
Identical failure set before and after this change: the same two red review tests
above. No NEW functional failures; my change touches only `BytecodeVM` access,
which is orthogonal to the Done-channel and ID-counter code those tests exercise.

## Commit
`COMMIT_HASH_PLACEHOLDER`
