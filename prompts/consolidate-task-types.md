# Task: Consolidate Task Types - Delete server.Task, Use Only task.Task

## Context

There are two Task types representing the same concept:
- `task.Task` in `task/task.go` - Used by builtins
- `server.Task` in `server/task.go` - Used by scheduler

This is a design smell. We need ONE Task type.

## Objective

Delete `server.Task` entirely. Consolidate everything into `task.Task`. The compiler will guide you - fix all errors until it builds.

## Strategy

1. **Add missing fields to `task.Task`** that `server.Task` has
2. **Add `Run()` method to `task.Task`**
3. **Use interface for fork creation** to avoid import cycle
4. **Delete `server/task.go`**
5. **Update scheduler to use `task.Task` directly**
6. **Fix all compiler errors**

## Step-by-Step Implementation

### Step 1: Add ForkCreator interface to task/task.go

```go
// ForkCreator interface allows tasks to create forked children without importing server
type ForkCreator interface {
    CreateForkedTask(parent *Task, info *types.ForkInfo) int64
}
```

### Step 2: Add missing fields to task.Task

Look at `server/task.go` Task struct and add what's missing to `task/task.go` Task struct:

```go
type Task struct {
    // Existing fields (keep all of these)
    ID           int64
    Owner        types.ObjID
    Kind         TaskKind
    State        TaskState
    StartTime    time.Time
    QueueTime    time.Time
    TicksUsed    int64
    TicksLimit   int64
    SecondsUsed  float64
    SecondsLimit float64
    CallStack    []ActivationFrame
    TaskLocal    types.Value
    WakeTime     time.Time
    WakeValue    types.Value
    ForkInfo     *types.ForkInfo
    mu           sync.RWMutex

    // NEW: Execution fields (from server.Task)
    Code         interface{}    // []parser.Stmt - use interface{} to avoid importing parser
    Evaluator    interface{}    // *vm.Evaluator - use interface{} to avoid importing vm
    Context      *types.TaskContext
    Result       types.Result
    ForkCreator  ForkCreator    // For creating forked tasks
    cancelFunc   context.CancelFunc

    // NEW: Verb context (from server.Task)
    VerbName     string
    This         types.ObjID
    Caller       types.ObjID
    Argstr       string
    Args         []string
    Dobjstr      string
    Dobj         types.ObjID
    Prepstr      string
    Iobjstr      string
    Iobj         types.ObjID

    // NEW: Fork tracking
    IsForked     bool
}
```

Note: Use `interface{}` for Code and Evaluator to avoid importing parser/vm packages.

### Step 3: Add Run() method to task/task.go

```go
import (
    "context"
    // ... existing imports
)

// Run executes the task. Evaluator must be set before calling.
// Returns error if execution fails or is aborted.
func (t *Task) Run(ctx context.Context) error {
    t.SetState(TaskRunning)

    // Type assert to get actual evaluator and code
    // The server package will set these as the correct types
    evaluator := t.Evaluator
    code := t.Code

    if evaluator == nil || code == nil {
        t.SetState(TaskKilled)
        return fmt.Errorf("task has no evaluator or code")
    }

    // The actual execution is done by calling methods on the evaluator
    // This will be called from the scheduler which has the concrete types
    // For now, just mark as needing external execution
    return nil
}
```

Actually, the Run() logic is complex and uses vm.Evaluator methods. Better approach:

**Keep Run() in scheduler, but operate on task.Task:**

```go
// In server/scheduler.go
func (s *Scheduler) runTask(t *task.Task) error {
    t.SetState(task.TaskRunning)

    code := t.Code.([]parser.Stmt)
    ctx := t.Context

    for _, stmt := range code {
        // ... execution logic (copy from old server/task.go Run method)
    }

    t.SetState(task.TaskCompleted)
    return nil
}
```

### Step 4: Update NewTask in task/task.go

Expand the existing NewTask or add a new constructor:

```go
func NewTaskFull(id int64, owner types.ObjID, tickLimit int64, secondsLimit float64, code interface{}, ctx *types.TaskContext) *Task {
    now := time.Now()
    t := &Task{
        ID:           id,
        Owner:        owner,
        Kind:         TaskInput,
        State:        TaskCreated,
        StartTime:    now,
        QueueTime:    now,
        TicksUsed:    0,
        TicksLimit:   tickLimit,
        SecondsUsed:  0,
        SecondsLimit: secondsLimit,
        CallStack:    make([]ActivationFrame, 0),
        TaskLocal:    types.NewInt(0),
        WakeValue:    types.NewInt(0),
        Code:         code,
        Context:      ctx,
    }
    // Set ctx.Task to this task so builtins can access it
    if ctx != nil {
        ctx.Task = t
    }
    return t
}
```

### Step 5: Delete server/task.go

Once task.Task has everything needed, delete the file entirely:
```bash
rm server/task.go
```

### Step 6: Update server/scheduler.go

Change all references from `*Task` to `*task.Task`:

```go
import "barn/task"

type Scheduler struct {
    tasks       map[int64]*task.Task  // Changed
    waiting     *TaskQueue            // Update TaskQueue to use *task.Task
    // ...
}
```

Update all methods:
- `CreateForegroundTask` - create task.Task instead of server.Task
- `CreateVerbTask` - same
- `CreateBackgroundTask` - same
- `CreateForkedTask` - same
- `processReadyTasks` - call runTask with *task.Task

### Step 7: Update TaskQueue in server/scheduler.go

The TaskQueue heap needs to work with `*task.Task`:

```go
type TaskQueue []*task.Task

func (pq TaskQueue) Len() int { return len(pq) }
func (pq TaskQueue) Less(i, j int) bool {
    return pq[i].StartTime.Before(pq[j].StartTime)
}
// ... etc
```

### Step 8: Make Scheduler implement ForkCreator

```go
// Scheduler implements task.ForkCreator
func (s *Scheduler) CreateForkedTask(parent *task.Task, forkInfo *types.ForkInfo) int64 {
    // ... existing CreateForkedTask logic
}
```

### Step 9: Fix all compiler errors

Run `go build ./...` and fix each error. The compiler will tell you exactly what's wrong.

Common fixes:
- `t.Player` → `t.Owner`
- `t.State = X` → `t.SetState(X)`
- `TaskWaiting` → `task.TaskQueued`
- `TaskAborted` → `task.TaskKilled`
- Import `barn/task` where needed

## Test Commands

```bash
# Build and fix errors iteratively
cd /c/Users/Q/code/barn
go build ./cmd/barn/ 2>&1 | head -30

# Once it builds, test
./barn_test.exe -db Test.db -port 9900 > server_9900.log 2>&1 &
sleep 2

# Quick sanity check
printf 'connect wizard\n; return 1 + 1;\n' | nc -w 3 localhost 9900

# Test task builtins work (callers, etc)
printf 'connect wizard\n; return callers();\n' | nc -w 3 localhost 9900

# Run conformance tests
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9900 --tb=no -q 2>&1 | tail -5

# Clean up
taskkill //F //IM barn_test.exe
```

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./task/task.go`, `C:/Users/Q/code/barn/task/task.go`
4. NEVER use cat, sed, echo - always Read/Edit/Write

## Output

Write your report to `./reports/consolidate-task-types.md` when done, including:
- Summary of changes made
- Files modified/deleted
- Any issues encountered and how resolved
- Final build status
- Test results
