# MOO Task Model Specification

## Overview

MOO uses cooperative multitasking with tick-based resource limits. Tasks are units of execution that can be suspended, resumed, and killed.

---

## 1. Task Types

### 1.1 Foreground Tasks

Created by player input:
- Higher tick limits (default 60,000)
- Higher time limits (default 5 seconds)
- Interactive response expected

### 1.2 Background Tasks

Created by `fork` statements:
- Lower tick limits (default 30,000)
- Lower time limits (default 3 seconds)
- Non-interactive processing

---

## 2. Task Lifecycle

### 2.1 States

```
Created → Waiting → Running → (Suspended) → Completed/Aborted
```

| State | Description |
|-------|-------------|
| Created | Task exists but not yet runnable |
| Waiting | Queued for execution (time-delayed) |
| Running | Currently executing |
| Suspended | Blocked on I/O or explicit suspend |
| Completed | Finished normally |
| Aborted | Killed or timed out |

### 2.2 Lifecycle Diagram

```
Player Input
     │
     ▼
┌─────────┐     fork      ┌─────────┐
│Foreground├──────────────►│Background│
│  Task   │               │  Task   │
└────┬────┘               └────┬────┘
     │                         │
     ▼                         ▼
┌─────────┐               ┌─────────┐
│ Running │               │ Waiting │ (delay period)
└────┬────┘               └────┬────┘
     │                         │
     ├───── suspend() ────────►│
     │                         │
     │◄──── resume()  ─────────┤
     │                         │
     ▼                         ▼
┌─────────┐               ┌─────────┐
│Complete │               │Complete │
│or Abort │               │or Abort │
└─────────┘               └─────────┘
```

---

## 3. Task Creation

### 3.1 Foreground Task

Created automatically when player enters command:

```
Command parsing → Verb lookup → Task creation → Execution
```

### 3.2 Fork Statement

```moo
fork (delay)
  // Background task body
endfork

fork task_id (delay)
  // task_id receives new task ID
endfork
```

**Semantics:**
1. Create new task with copy of current environment
2. Schedule to run after `delay` seconds
3. Parent continues immediately
4. Background task executes independently

**Environment copy:** Fork uses shallow copy with reference-counted copy-on-write
semantics. Primitives (INT, FLOAT) are copied by value. Lists, maps, and strings
are shallow copied with reference count increment; mutations trigger COW.

**Delay of 0:** Even with `delay = 0`, the forked task is queued and runs on the
next scheduler cycle, NOT before the parent continues. Parent resumes immediately
after the fork statement completes.

**Task ID binding:** When using `fork task_id (delay)`, the task_id variable is
bound synchronously when the fork statement executes, before the parent continues.
The parent can use the ID immediately after the fork statement.

### 3.3 Task ID

Every task has a unique integer ID:

```moo
id = task_id();  // Get current task's ID
```

---

## 4. Tick System

### 4.1 Purpose

Prevent infinite loops and resource hogging:
- Each operation costs "ticks"
- Task aborted when ticks exhausted
- Configurable limits

### 4.2 Tick Costs

| Operation | Cost |
|-----------|------|
| Most opcodes | 1 tick |
| List creation | 0 ticks |
| Map creation | 0 ticks |
| Variable access | 0-1 ticks |
| Builtin call | Varies |

### 4.3 Limits

| Context | Default Ticks | Default Seconds |
|---------|---------------|-----------------|
| Foreground | 60,000 | 5 |
| Background | 30,000 | 3 |

### 4.4 Checking Limits

```moo
ticks = ticks_left();    // Remaining ticks
secs = seconds_left();   // Remaining seconds
```

### 4.5 Yielding

```moo
yin(ticks);  // Yield if fewer than N ticks remain
```

**Semantics:**
- If `ticks_left() < ticks`, suspend and resume later
- Refreshes tick count when resumed

---

## 5. Task Suspension

### 5.1 Explicit Suspension

```moo
suspend();           // Suspend indefinitely
suspend(seconds);    // Suspend for duration
```

**Semantics:**
- Task state saved (stack, variables, PC)
- Task moved to waiting queue
- Resumes when time elapses or `resume()` called

### 5.2 I/O Suspension

Tasks automatically suspend during:
- `read()` - Waiting for player input
- Network operations
- `exec()` - External process execution

### 5.3 Suspension Limits

Maximum suspended tasks per player (configurable).

---

## 6. Task Resumption

### 6.1 Automatic Resume

Suspended tasks resume when:
- Delay period expires
- I/O completes
- Input received

### 6.2 Manual Resume

```moo
resume(task_id, value);
```

**Semantics:**
- Wake up suspended task
- `value` becomes return value of `suspend()`

---

## 7. Task Killing

### 7.1 Kill Task

```moo
kill_task(task_id);
```

**Permissions:**
- Can kill own tasks
- Wizards can kill any task
- Killing current task aborts it

### 7.2 Abort Reasons

| Reason | Cause |
|--------|-------|
| ABORT_TICKS | Tick limit exceeded |
| ABORT_SECONDS | Time limit exceeded |
| ABORT_KILL | Explicit kill_task() |
| ABORT_ERROR | Unhandled error |

---

## 8. Task Inspection

### 8.1 Current Task

```moo
task_id()       // Current task ID
caller_perms()  // Permission object
task_stack()    // Call stack
```

### 8.2 Queued Tasks

```moo
queued_tasks()
queued_tasks(include_variables)
```

**Returns:** List of task info:
```moo
{task_id, start_time, x, y, programmer, verb_loc, verb_name, line, this, [variables]}
```

### 8.3 Task Stack

```moo
callers()
callers(include_line_numbers)
```

**Returns:** List of stack frames:
```moo
{this, verb_name, programmer, verb_loc, player, [line_no]}
```

---

## 9. Task-Local Storage

### 9.1 Setting Values

```moo
set_task_local(key, value);
```

### 9.2 Getting Values

```moo
value = task_local(key);
```

### 9.3 Semantics

- Persists for lifetime of task
- Not inherited by forked tasks
- Keys are arbitrary values

---

## 10. Task Context

### 10.1 Context Variables

Available in every verb:

| Variable | Description |
|----------|-------------|
| `this` | Object verb is on |
| `player` | Player who initiated |
| `caller` | Calling object |
| `verb` | Verb name |
| `args` | Argument list |

### 10.2 Permission Context

```moo
caller_perms()     // Who we're running as
set_task_perms(obj)  // Change permission context
```

**Wizard-only:** Changing task permissions

---

## 11. Scheduling

### 11.1 Task Queue

Tasks are scheduled in time order:
- Earliest start time first
- FIFO for same-time tasks

### 11.2 Execution Model

Single-threaded cooperative:
1. Pick next runnable task
2. Execute until suspend/complete/abort
3. Process any resulting tasks
4. Repeat

### 11.3 Time Slicing

No preemption:
- Task runs until it yields
- Tick limits enforce fairness
- Long-running tasks should use `yin()`

---

## 12. Error Handling in Tasks

### 12.1 Unhandled Errors

If error propagates to top level:
- Task aborts
- Error logged
- Player notified (if foreground)

### 12.2 Error in Fork

Background task errors don't affect parent:
- Parent continues normally
- Background task aborts independently

---

## 13. Go Implementation

### 13.1 Task Structure

```go
type Task struct {
    ID          int64
    State       TaskState
    VM          *VM
    Player      int64
    StartTime   time.Time
    TicksUsed   int
    TickLimit   int
    Deadline    time.Time
    TaskLocal   map[Value]Value
    WakeChannel chan Value  // For suspension
}

type TaskState int

const (
    TaskCreated TaskState = iota
    TaskWaiting
    TaskRunning
    TaskSuspended
    TaskCompleted
    TaskAborted
)
```

### 13.2 Goroutine Mapping

```go
// Fork creates new goroutine with delay
func (vm *VM) Fork(delay time.Duration, body func()) int64 {
    task := &Task{
        ID:        nextTaskID(),
        State:     TaskWaiting,
        StartTime: time.Now().Add(delay),
    }

    go func() {
        time.Sleep(delay)
        task.State = TaskRunning
        body()
        task.State = TaskCompleted
    }()

    return task.ID
}
```

### 13.3 Suspension with Channels

```go
// Suspend current task
func (task *Task) Suspend(timeout time.Duration) Value {
    task.State = TaskSuspended

    if timeout > 0 {
        select {
        case value := <-task.WakeChannel:
            return value
        case <-time.After(timeout):
            return Int(0)
        }
    } else {
        return <-task.WakeChannel  // Wait forever
    }
}

// Resume suspended task
func ResumeTask(taskID int64, value Value) error {
    task := findTask(taskID)
    if task == nil || task.State != TaskSuspended {
        return ErrInvarg
    }
    task.WakeChannel <- value
    return nil
}
```

### 13.4 Tick Counting

```go
func (vm *VM) Step() error {
    op := vm.fetchOpcode()

    if countsTick(op) {
        vm.Task.TicksUsed++
        if vm.Task.TicksUsed >= vm.Task.TickLimit {
            return ErrTicksExceeded
        }
    }

    if time.Now().After(vm.Task.Deadline) {
        return ErrTimeout
    }

    return vm.execute(op)
}
```

### 13.5 Context Cancellation

```go
// Using context for task lifecycle
func (task *Task) Run(ctx context.Context) error {
    ctx, cancel := context.WithDeadline(ctx, task.Deadline)
    defer cancel()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            if err := task.VM.Step(); err != nil {
                return err
            }
        }
    }
}
```

### 13.6 Task Killing

```go
func KillTask(taskID int64, killerID int64) error {
    task := findTask(taskID)
    if task == nil {
        return ErrInvarg
    }

    // Permission check
    if killerID != task.Player && !isWizard(killerID) {
        return ErrPerm
    }

    // Cancel context (triggers abort)
    task.Cancel()
    return nil
}
```

---

## 14. Go Concurrency Advantages

### 14.1 Natural Mapping

| MOO Concept | Go Concept |
|-------------|------------|
| Task | Goroutine |
| Suspend | Channel receive |
| Resume | Channel send |
| Fork | go + time.After |
| Kill | Context cancellation |
| Task queue | Priority queue + scheduler goroutine |

### 14.2 Benefits Over Python

- Real parallelism (not GIL-limited)
- Lightweight goroutines (not greenlets)
- Built-in channels (not gevent.Queue)
- Context cancellation (not manual state tracking)
- select for timeouts (not gevent.Timeout)

### 14.3 Scheduler Design

```go
type Scheduler struct {
    waiting  *PriorityQueue  // Time-ordered
    running  *Task           // Currently executing
    mutex    sync.Mutex
}

func (s *Scheduler) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            task := s.nextRunnable()
            if task != nil {
                s.running = task
                task.Run(ctx)
                s.running = nil
            } else {
                time.Sleep(10 * time.Millisecond)
            }
        }
    }
}
```
