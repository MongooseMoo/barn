# Task: Implement fork/endfork statement

## Background

See `reports/research-task-fork.md` for the complete research. Key points:

1. **Fork is NOT a function** - it's a control flow operation
2. VM evaluates fork → yields with `FlowFork` → scheduler creates child → parent continues
3. Child gets **deep copy** of variable environment
4. Named fork: `fork task_id (delay) ... endfork` - parent's `task_id` var gets child's ID
5. Anonymous fork: `fork (delay) ... endfork` - no ID stored
6. Even `fork (0)` is async - child is queued, parent continues immediately

## Implementation Steps

### 1. Parser Changes

**Add tokens to lexer (`parser/lexer.go`):**
- `TOKEN_FORK` for "fork"
- `TOKEN_ENDFORK` for "endfork"

**Add AST node (`parser/ast.go`):**
```go
type ForkStmt struct {
    Delay   Expr     // Delay expression (seconds)
    VarName string   // Variable name for task ID (empty = anonymous)
    Body    []Stmt   // Fork body statements
}
func (s *ForkStmt) stmtNode() {}
```

**Add parser (`parser/parser.go`):**
```
fork [varname] (delay)
    body
endfork
```

### 2. Types Changes

**Add FlowFork to `types/result.go`:**
```go
const (
    FlowNormal FlowType = iota
    FlowReturn
    FlowBreak
    FlowContinue
    FlowException
    FlowFork  // NEW
)
```

**Add ForkInfo structure:**
```go
type ForkInfo struct {
    Body      []parser.Stmt       // Fork body to execute
    Delay     time.Duration       // Delay before execution
    VarName   string              // Variable to store task ID (empty = anonymous)
    Variables map[string]Value    // Deep copy of variable environment
    ThisObj   ObjID               // this context
    Player    ObjID               // player context
    Caller    ObjID               // caller context
    Verb      string              // verb context
}
```

### 3. Evaluator Changes

**Add to `vm/eval_stmt.go`:**
```go
case *parser.ForkStmt:
    return e.forkStmt(s, ctx)
```

**Implement forkStmt:**
1. Evaluate delay expression
2. Validate delay is numeric and >= 0
3. Deep copy current variable environment from `e.env`
4. Return `Result{Flow: FlowFork, ForkInfo: ...}`

**Deep copy variables:**
- Copy each variable in the environment
- For lists/maps, do recursive deep copy
- Don't share references between parent and child

### 4. Task Package Changes

**Add to `task/task.go`:**
```go
type TaskKind int
const (
    TaskInput TaskKind = iota
    TaskForked
    TaskSuspended
)

// Add to Task struct:
Kind      TaskKind
ForkInfo  *types.ForkInfo  // For forked tasks
```

### 5. Scheduler Changes

**Add to `server/scheduler.go` or create new file:**

The scheduler needs to:
1. Accept new forked tasks
2. Queue them with their start time
3. Run them when start time arrives
4. Track task IDs

```go
func (s *Scheduler) CreateForkedTask(parent *Task, forkInfo *types.ForkInfo) int64 {
    childID := atomic.AddInt64(&s.nextTaskID, 1)

    child := &task.Task{
        ID:        childID,
        Kind:      task.TaskForked,
        Owner:     parent.Owner,
        State:     task.TaskQueued,
        StartTime: time.Now().Add(forkInfo.Delay),
        ForkInfo:  forkInfo,
        // ... other fields
    }

    s.QueueTask(child)
    return childID
}
```

### 6. Server Integration

**Modify where tasks are executed to handle FlowFork:**

When evaluator returns `FlowFork`:
1. Call scheduler to create child task
2. If named fork, store child ID in parent's variable
3. Continue parent execution (don't return, just continue to next statement)

### 7. Forked Task Execution

When scheduler runs a forked task:
1. Create new evaluator with fresh environment
2. Load variables from ForkInfo.Variables
3. Set up context (this, player, caller, verb)
4. Execute ForkInfo.Body statements
5. Mark task complete when done

## Files to Modify

1. `parser/lexer.go` - Add FORK, ENDFORK tokens
2. `parser/ast.go` - Add ForkStmt
3. `parser/parser.go` - Parse fork statement
4. `types/result.go` - Add FlowFork, ForkInfo
5. `vm/eval_stmt.go` - Implement forkStmt
6. `task/task.go` - Add TaskKind, ForkInfo field
7. `task/manager.go` - Methods for forked tasks
8. `server/scheduler.go` - Forked task scheduling
9. `server/connection.go` - Handle FlowFork in task execution

## Testing

After implementation, this should work:
```bash
barn.exe -db toastcore.db -eval "fork (0); endfork; 1"
# Should return 1 (parent continues)
```

And the connect verb should compile and run.

## Verification

1. Build succeeds: `go build -o barn.exe ./cmd/barn`
2. Server starts and accepts connections
3. `connect wizard` command progresses past fork
4. No E_VERBNF on connect verb

## Output

Write status updates to `./reports/implement-fork.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
