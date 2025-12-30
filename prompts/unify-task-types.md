# Task: Unify task.Task and server.Task Types

## Context

There are two separate Task types in the codebase that need to be unified:

1. `task.Task` in `task/task.go` - Used by builtins for MOO task API (queued_tasks, callers, suspend)
2. `server.Task` in `server/task.go` - Used by server scheduler for actual execution

This causes a type mismatch: builtins expect `ctx.Task.(*task.Task)` but server sets `ctx.Task` to `*server.Task`.

## Objective

Unify these types by having `server.Task` embed `*task.Task`, so:
- Builtins can access `ctx.Task.(*task.Task)` successfully
- Server scheduler still has full execution capabilities
- Task manager tracks all tasks centrally

## Implementation Steps

### 1. Modify `server/task.go`

Change `server.Task` to embed `*task.Task`:

```go
type Task struct {
    *task.Task  // Embed the task package's Task

    // Server-specific execution fields (keep these)
    Code          []parser.Stmt
    Evaluator     *vm.Evaluator
    Context       *types.TaskContext
    Scheduler     *Scheduler
    cancelFunc    context.CancelFunc
    mu            sync.Mutex

    // Verb context fields (keep these)
    VerbName string
    This     types.ObjID
    Caller   types.ObjID
    Argstr   string
    Args     []string
    Dobjstr  string
    Dobj     types.ObjID
    Prepstr  string
    Iobjstr  string
    Iobj     types.ObjID
}
```

Remove duplicate fields that are now in `task.Task`:
- ID (use task.Task.ID)
- State (use task.Task.State, but note different enum - may need mapping)
- Player (use task.Task.Owner)
- StartTime (use task.Task.StartTime)
- TicksUsed, TickLimit (use task.Task.TicksUsed, TicksLimit)
- TimeLimit, Deadline (use task.Task.SecondsLimit + calculation)
- TaskLocal (use task.Task.TaskLocal)
- WakeChannel (task.Task doesn't have this - keep in server.Task OR add to task.Task)
- Result (keep in server.Task)
- ForkInfo, IsForked (task.Task has ForkInfo, server.Task has IsForked - reconcile)

### 2. Update `NewTask` in `server/task.go`

```go
func NewTask(id int64, player types.ObjID, code []parser.Stmt, tickLimit int, timeLimit time.Duration) *Task {
    // Create the inner task via task manager
    mgr := task.GetManager()
    innerTask := mgr.CreateTask(player, int64(tickLimit), timeLimit.Seconds())

    // Create context
    ctx := types.NewTaskContext()
    ctx.Player = player
    ctx.Programmer = player
    ctx.TicksRemaining = int64(tickLimit)
    ctx.Task = innerTask  // Set to the *task.Task so builtins work

    t := &Task{
        Task:      innerTask,
        Code:      code,
        Context:   ctx,
        // ... other fields
    }

    return t
}
```

### 3. Handle State enum differences

`task.Task` has: TaskCreated, TaskQueued, TaskRunning, TaskSuspended, TaskCompleted, TaskKilled
`server.Task` has: TaskCreated, TaskWaiting, TaskRunning, TaskSuspended, TaskCompleted, TaskAborted

Options:
- Use task.Task's states everywhere (TaskQueued ≈ TaskWaiting, TaskKilled ≈ TaskAborted)
- Or keep server.Task's State field separate if needed for scheduler-specific states

Recommend: Use task.Task's states, map TaskWaiting→TaskQueued, TaskAborted→TaskKilled

### 4. Update scheduler references

In `server/scheduler.go`, update any references to Task fields that moved:
- `task.ID` → `task.Task.ID` (or just `task.ID` if embedding works)
- `task.Player` → `task.Task.Owner`
- `task.State` → use task.Task.GetState()/SetState()

### 5. Update `server/task.go` Run() method

The Run() method should update `task.Task` state appropriately:
```go
func (t *Task) Run(ctx context.Context, evaluator *vm.Evaluator) error {
    t.Task.SetState(task.TaskRunning)
    // ... execution ...
    t.Task.SetState(task.TaskCompleted)
}
```

### 6. Test the changes

Build and run:
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9900 > server_9900.log 2>&1 &
sleep 2

# Test suspend works now
printf 'connect wizard\n; suspend(0.1); return 42;\n' | nc -w 5 localhost 9900

# Run conformance tests
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9900 --tb=no -q -k "not limits and not waif and not anonymous" 2>&1 | tail -10
```

## Files to Modify

- `server/task.go` - Main changes (embed task.Task, update NewTask, update Run)
- `server/scheduler.go` - Update field references if needed
- Possibly `task/task.go` - Add WakeChannel if needed for suspend

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./server/task.go`, `C:/Users/Q/code/barn/server/task.go`
4. NEVER use cat, sed, echo - always Read/Edit/Write

## Output

Write your report to `./reports/unify-task-types.md` when done, including:
- What was changed
- Any issues encountered
- Test results
