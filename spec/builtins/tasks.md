# MOO Task Management Built-ins

## Overview

Functions for managing tasks (execution threads).

---

## 1. Current Task

### 1.1 task_id

**Signature:** `task_id() → INT`

**Description:** Returns current task's unique ID.

**Examples:**
```moo
id = task_id();   => 12345 (varies)
```

---

### 1.2 caller

**Signature:** `caller() → OBJ`

**Description:** Returns the object that called current verb.

**Note:** Same as `caller` variable but as function.

---

### 1.3 caller_perms

**Signature:** `caller_perms() → OBJ`

**Description:** Returns the permission context object.

**Semantics:**
- Usually the player who initiated the task
- Can be changed by wizards

---

### 1.4 set_task_perms

**Signature:** `set_task_perms(who) → none`

**Description:** Changes permission context.

**Wizard only.**

**Examples:**
```moo
set_task_perms(#wizard);  // Run as wizard
```

---

## 2. Task Information

### 2.1 callers

**Signature:** `callers([include_line_numbers]) → LIST`

**Description:** Returns call stack.

**Returns:** List of frames, each:
```moo
{this, verb_name, programmer, verb_loc, player, [line_no]}
```

**Examples:**
```moo
callers()
  => {{#room, "look", #wizard, #room, #player},
      {#thing, "describe", #wizard, #thing, #player}}
```

---

### 2.2 task_stack (ToastStunt)

**Signature:** `task_stack([task_id [, include_vars]]) → LIST`

**Description:** Returns detailed stack for task.

**Returns:** List of frames with more detail than callers().

---

### 2.3 queued_tasks

**Signature:** `queued_tasks([include_vars]) → LIST`

**Description:** Returns list of queued (waiting) tasks.

**Returns:** List of task info:
```moo
{task_id, start_time, x, y, programmer, verb_loc, verb_name, line, this, [vars]}
```

**Examples:**
```moo
tasks = queued_tasks();
for task in (tasks)
    notify(player, "Task " + tostr(task[1]) + " at " + tostr(task[2]));
endfor
```

---

## 3. Task Control

### 3.1 suspend

**Signature:** `suspend([seconds]) → VALUE`

**Description:** Suspends current task.

**Behavior:**
- Without args: suspend indefinitely (until resumed)
- With seconds: suspend for that duration

**Returns:** Value passed to `resume()`, or 0 on timeout.

**Timeout limitation:** There is no way to distinguish a timeout (returns 0) from
an explicit `resume(task, 0)`. Both return integer 0. If distinction is needed,
use a non-zero sentinel value when resuming (e.g., `resume(task, 1)`).

**Examples:**
```moo
suspend(5);        // Sleep 5 seconds
val = suspend();   // Wait for resume
```

---

### 3.2 resume

**Signature:** `resume(task_id [, value]) → none`

**Description:** Resumes a suspended task.

**Parameters:**
- `task_id`: Task to resume
- `value`: Value returned by suspend() (default: 0)

**Examples:**
```moo
resume(other_task, "wake up!");
```

**Errors:**
- E_INVARG: Task not suspended
- E_PERM: Not owner or wizard

---

### 3.3 kill_task

**Signature:** `kill_task(task_id) → none`

**Description:** Terminates a task.

**Permissions:**
- Can kill own tasks
- Wizards can kill any task

**Examples:**
```moo
kill_task(runaway_task);
```

**Errors:**
- E_INVARG: Invalid task ID
- E_PERM: Not owner or wizard

---

## 4. Resource Limits

### 4.1 ticks_left

**Signature:** `ticks_left() → INT`

**Description:** Returns remaining ticks for current task.

---

### 4.2 seconds_left

**Signature:** `seconds_left() → INT`

**Description:** Returns remaining seconds for current task.

---

### 4.3 yin (ToastStunt: yield if needed)

**Signature:** `yin(threshold) → none`

**Description:** Yields if ticks remaining < threshold.

**Semantics:**
- If `ticks_left() < threshold`, suspend and resume later
- Refreshes tick count on resume

**Examples:**
```moo
for i in [1..10000]
    yin(1000);  // Yield if low on ticks
    process_item(i);
endfor
```

---

## 5. Task Limits

### 5.1 set_task_ticks (ToastStunt)

**Signature:** `set_task_ticks(task_id, ticks) → none`

**Description:** Sets tick limit for task.

**Wizard only.**

---

### 5.2 set_task_seconds (ToastStunt)

**Signature:** `set_task_seconds(task_id, seconds) → none`

**Description:** Sets time limit for task.

**Wizard only.**

---

## 6. Task-Local Storage

### 6.1 set_task_local

**Signature:** `set_task_local(key, value) → none`

**Description:** Stores value in task-local storage.

**Examples:**
```moo
set_task_local("request_id", 12345);
```

---

### 6.2 task_local

**Signature:** `task_local(key) → VALUE`

**Description:** Retrieves value from task-local storage.

**Examples:**
```moo
id = task_local("request_id");  => 12345
task_local("missing")           => 0
```

---

## 7. Fork Statement

The `fork` statement creates background tasks:

```moo
fork (delay)
    // Runs after delay seconds
endfork

fork task_var (delay)
    // task_var receives task ID
endfork
```

**Examples:**
```moo
fork (0)
    // Run as soon as possible
endfork

fork tid (10)
    expensive_work();
endfork
// tid now contains task ID
```

---

## 8. Server Control

### 8.1 server_version

**Signature:** `server_version() → STR`

**Description:** Returns server version string.

---

### 8.2 memory_usage (ToastStunt)

**Signature:** `memory_usage() → LIST`

**Description:** Returns memory statistics.

---

### 8.3 shutdown

**Signature:** `shutdown([message]) → none`

**Description:** Initiates server shutdown.

**Wizard only.**

---

### 8.4 dump_database

**Signature:** `dump_database() → none`

**Description:** Forces database checkpoint.

**Wizard only.**

---

## 9. Connection Management

### 9.1 connected_players

**Signature:** `connected_players([include_queued]) → LIST`

**Description:** Returns list of connected player objects.

---

### 9.2 connection_name

**Signature:** `connection_name(player) → STR`

**Description:** Returns connection identifier.

---

### 9.3 boot_player

**Signature:** `boot_player(player) → none`

**Description:** Disconnects a player.

**Wizard only.**

---

### 9.4 notify

**Signature:** `notify(player, message [, no_flush]) → none`

**Description:** Sends message to player's connection.

**Examples:**
```moo
notify(player, "Hello, world!");
```

---

## 10. Exception Raising

### 10.1 raise

**Signature:** `raise(error_code [, message [, value]]) → none`

**Description:** Raises an error that can be caught by try/except blocks or catch expressions, or propagates to the caller.

**Parameters:**
- `error_code` (ERR): Error code constant (E_TYPE, E_PERM, E_INVARG, etc.)
- `message` (STR, optional): Custom error message string (defaults to error code name if omitted)
- `value` (ANY, optional): Additional error data to include (defaults to 0 if omitted)

**Behavior:**
- Raises the specified error which propagates through the exception handler stack
- If caught by a try/except block or catch expression, normal error handling proceeds
- If not caught by any handler, the task aborts with that error code
- The error message defaults to the error code name if not provided

**Examples:**
```moo
raise(E_INVARG);                        // Simple raise with code only
raise(E_INVARG, "Invalid user object"); // With custom message
raise(E_PERM, "Access denied", #123);   // With message and value
```

**Common Patterns:**
```moo
// Validation
if (!valid(obj))
    raise(E_INVARG, "Object does not exist");
endif

// Permission checking
if (!is_wizard(player))
    raise(E_PERM, "Wizard access required");
endif

// Re-raise with context
try
    risky_operation();
except e (ANY)
    raise(e, "Failed during startup");  // Re-raise with more context
endtry
```

**Errors:**
- E_TYPE: error_code argument is not an ERR type

**Notes:**
- Can be called from anywhere (try blocks, except blocks, finally blocks, normal code)
- If called from a finally block, the new error replaces any pending error from the try block
- Errors raised in catch expression default values propagate normally

---

## 11. Error Handling

| Error | Condition |
|-------|-----------|
| E_INVARG | Invalid task ID |
| E_PERM | Permission denied |
| E_ARGS | Wrong argument count |

---

## 11. Go Implementation Notes

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
    TaskLocal   map[string]Value
    WakeChan    chan Value
    Ctx         context.Context
    Cancel      context.CancelFunc
}

func builtinSuspend(args []Value) (Value, error) {
    task := vm.CurrentTask()

    var timeout time.Duration
    if len(args) > 0 {
        secs, _ := toFloat(args[0])
        timeout = time.Duration(secs * float64(time.Second))
    }

    task.State = TaskSuspended

    if timeout > 0 {
        select {
        case val := <-task.WakeChan:
            return val, nil
        case <-time.After(timeout):
            return IntValue(0), nil
        case <-task.Ctx.Done():
            return nil, E_TICKS  // Killed
        }
    } else {
        select {
        case val := <-task.WakeChan:
            return val, nil
        case <-task.Ctx.Done():
            return nil, E_TICKS
        }
    }
}

func builtinResume(args []Value) (Value, error) {
    taskID := int64(args[0].(IntValue))
    task := scheduler.FindTask(taskID)
    if task == nil || task.State != TaskSuspended {
        return nil, E_INVARG
    }

    if !canControl(callerPerms(), task) {
        return nil, E_PERM
    }

    value := IntValue(0)
    if len(args) > 1 {
        value = args[1]
    }

    task.WakeChan <- value
    return nil, nil
}

func builtinKillTask(args []Value) (Value, error) {
    taskID := int64(args[0].(IntValue))
    task := scheduler.FindTask(taskID)
    if task == nil {
        return nil, E_INVARG
    }

    if !canControl(callerPerms(), task) {
        return nil, E_PERM
    }

    task.Cancel()  // Triggers context cancellation
    return nil, nil
}

func builtinCallers(args []Value) (Value, error) {
    includeLines := false
    if len(args) > 0 {
        includeLines = isTruthy(args[0])
    }

    frames := make([]Value, 0)
    for _, frame := range vm.Frames {
        entry := []Value{
            ObjValue(frame.This),
            StringValue(frame.Verb),
            ObjValue(frame.Programmer),
            ObjValue(frame.VerbLoc),
            ObjValue(frame.Player),
        }
        if includeLines {
            entry = append(entry, IntValue(frame.LineNo))
        }
        frames = append(frames, &MOOList{data: entry})
    }
    return &MOOList{data: frames}, nil
}
```
