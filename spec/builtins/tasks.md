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

### 1.2 caller (variable)

**Type:** `OBJ`

**Description:** Builtin variable containing the object that called current verb.

**Note:** This is a **variable**, not a function. Use `caller` (no parentheses).

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

### 1.5 task_perms (ToastStunt)

**Signature:** `task_perms() → OBJ`

**Description:** Alias for `caller_perms()`. Returns the permission context object.

**Note:** This is identical to `caller_perms()` - ToastStunt provides both names for compatibility.

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

callers(1)
  => {{#room, "look", #wizard, #room, #player, 1},
      {#thing, "describe", #wizard, #thing, #player, 5}}
```

**Note:** By default, `callers()` returns 5-element frames (without line numbers). Pass `1` (or any true value) to include line numbers (6-element frames).

---

### 2.2 task_stack (ToastStunt)

**Signature:** `task_stack(task_id [, include_vars [, include_temp_vars]]) → LIST`

**Description:** Returns detailed stack for specified task.

**Parameters:**
- `task_id` (INT): Task ID to inspect (required)
- `include_vars` (ANY, optional): Include local variables if truthy
- `include_temp_vars` (ANY, optional): Include temporary variables if truthy

**Returns:** List of frames with more detail than callers().

---

### 2.3 queued_tasks

**Signature:** `queued_tasks([include_vars [, return_count]]) → LIST or INT`

**Description:** Returns list of queued (waiting) tasks, or count if requested.

**Parameters:**
- `include_vars` (INT, optional): If truthy, include variable info in task records
- `return_count` (INT, optional): If truthy, return count (INT) instead of list

**Returns:**
- Without `return_count`: List of task info:
  ```moo
  {task_id, start_time, x, y, programmer, verb_loc, verb_name, line, this, [vars]}
  ```
- With `return_count`: Integer count of queued tasks

**Examples:**
```moo
tasks = queued_tasks();              // Basic list
tasks = queued_tasks(1);            // Include variables
count = queued_tasks(0, 1);         // Just get the count
for task in (tasks)
    notify(player, "Task " + tostr(task[1]) + " at " + tostr(task[2]));
endfor
```

---

### 2.4 finished_tasks (ToastStunt)

**Signature:** `finished_tasks() → LIST`

**Description:** Returns list of recently completed tasks, if task history is enabled.

**Notes:**
- Only available when compiled with `SAVE_FINISHED_TASKS`. Otherwise, the builtin does not exist.
- Server must be configured to track finished tasks. Returns empty list if disabled.

**Returns:** List of completed task information, similar format to `queued_tasks()`.

---

### 2.5 queue_info (ToastStunt)

**Signature:** `queue_info([player]) → LIST`

**Description:** Returns task queue statistics.

**Parameters:**
- `player` (OBJ, optional): Get queue info for specific player. If omitted, returns server-wide stats.

**Returns:** List containing queue statistics (counts, limits, etc.).

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

**Note:** Returns 0 in eval context (non-forked tasks) where no tick limit is configured.

---

### 4.2 seconds_left

**Signature:** `seconds_left() → FLOAT`

**Description:** Returns remaining seconds for current task.

**Note:** Returns 0.0 in eval context (non-forked tasks) where no time limit is configured.

---

### 4.3 yin (ToastStunt: yield if needed)

**Signature:** `yin([threshold [, ticks [, seconds]]]) → none`

**Description:** Yields execution if resources are low.

**Parameters:**
- `threshold` (NUMERIC, optional): Tick threshold - yield if ticks_left() < threshold
- `ticks` (INT, optional): Additional tick threshold
- `seconds` (INT, optional): Additional time threshold

**Semantics:**
- If ticks remaining is below threshold, suspend and resume later
- Refreshes tick count on resume
- Can be called with no arguments to unconditionally yield

**Examples:**
```moo
yin();              // Unconditional yield
yin(1000);         // Yield if low on ticks
for i in [1..10000]
    yin(1000);     // Yield if low on ticks
    process_item(i);
endfor
```

---

### 4.4 set_thread_mode (ToastStunt)

**Signature:** `set_thread_mode([mode]) → INT`

**Description:** Controls whether certain builtins run in background threads for the current task activation.

**Parameters:**
- `mode` (INT, optional): If omitted, returns the current mode. If provided, sets the mode to `mode != 0`.

**Returns:**
- No arguments: current mode (INT)
- With argument: returns 0

**Notes:**
- Default mode is `DEFAULT_THREAD_MODE` (true in the reference build).
- This affects builtins implemented via the background thread system, such as `sort`, `all_members`, `locate_by_name`, `occupants`, `connection_name_lookup`, `curl`, `argon2`, `argon2_verify`, `sqlite_query`, and `sqlite_execute`.

---

### 4.5 threads (ToastStunt)

**Signature:** `threads() → LIST`

**Description:** Returns a list of background thread handles for queued/active background tasks.

**Notes:**
- Order is unspecified.
- Wizard only.

**Errors:**
- E_PERM: Not a wizard

---

### 4.6 thread_pool (ToastStunt)

**Signature:** `thread_pool(function, pool [, value]) → INT`

**Description:** Controls background thread pools.

**Parameters:**
- `function` (STR): Only `"INIT"` is accepted.
- `pool` (STR): Only `"MAIN"` is accepted in the reference implementation.
- `value` (INT, optional): Thread count for `"INIT"`. If omitted, uses 0.

**Semantics:**
- `value <= 0` disables the pool.
- `value > 0` recreates the pool with that many threads.

**Returns:** 1 on success.

**Errors:**
- E_PERM: Not a wizard
- E_INVARG (raise): Invalid function, pool, or thread count

---

### 4.7 background_test (ToastStunt, optional)

**Signature:** `background_test([message [, seconds]]) → STR`

**Description:** Demonstration builtin that sleeps in a background thread and returns a string.

**Notes:**
- Only available when compiled with `BACKGROUND_TEST`.
- Defaults: `message` is "Hello, world.", `seconds` is 5.
- Uses the background thread system and may suspend.

---

## 5. Task-Local Storage

### 5.1 set_task_local

**Signature:** `set_task_local(value) → none`

**Description:** Stores a single value in task-local storage.

**Permissions:** Wizard only.

**Note:** Task-local storage holds ONE value per task, not a key/value map. Each call to `set_task_local()` replaces the previous value.

**Examples:**
```moo
set_task_local({#123, "request-456", time()});  // Store tuple
```

---

### 5.2 task_local

**Signature:** `task_local() → VALUE`

**Description:** Retrieves the value from task-local storage.

**Permissions:** Wizard only.

**Returns:** The value previously stored with `set_task_local()`, or 0 if none.

**Examples:**
```moo
data = task_local();  // Get stored value
if (typeof(data) == LIST)
    {obj, request_id, timestamp} = data;
endif
```

---

## 6. Fork Statement

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

## 7. Server Control

### 7.1 server_version

**Signature:** `server_version() → STR`

**Description:** Returns server version string.

---

### 7.2 memory_usage (ToastStunt)

**Signature:** `memory_usage() → LIST`

**Description:** Returns memory statistics.

---

### 7.3 shutdown

**Signature:** `shutdown([message [, restart]]) → none`

**Description:** Initiates server shutdown or restart.

**Parameters:**
- `message` (STR, optional): Shutdown message to display
- `restart` (ANY, optional): If truthy, restart server instead of shutting down

**Wizard only.**

---

### 7.4 dump_database

**Signature:** `dump_database() → none`

**Description:** Forces database checkpoint.

**Wizard only.**

---

## 8. Connection Management

### 8.1 connected_players

**Signature:** `connected_players([include_queued]) → LIST`

**Description:** Returns list of connected player objects.

---

### 8.2 connection_name

**Signature:** `connection_name(player [, full]) → STR`

**Description:** Returns connection identifier.

**Parameters:**
- `player` (OBJ): Player object
- `full` (INT, optional): If truthy, return full connection details

---

### 8.3 boot_player

**Signature:** `boot_player(player) → none`

**Description:** Disconnects a player.

**Wizard only.**

---

### 8.4 notify

**Signature:** `notify(player, message [, no_flush [, binary]]) → none`

**Description:** Sends message to player's connection.

**Parameters:**
- `player` (OBJ): Target player
- `message` (STR): Message to send
- `no_flush` (ANY, optional): If truthy, don't flush output buffer immediately
- `binary` (ANY, optional): If truthy, send as binary data without line ending

**Examples:**
```moo
notify(player, "Hello, world!");
notify(player, "Buffered message", 1);  // No immediate flush
```

---

### 8.5 force_input (ToastStunt)

**Signature:** `force_input(player, text [, at_front]) → none`

**Description:** Injects input into player's command queue as if they typed it.

**Parameters:**
- `player` (OBJ): Target player
- `text` (STR): Text to inject
- `at_front` (ANY, optional): If truthy, insert at front of queue instead of back

**Wizard only.**

---

### 8.6 flush_input (ToastStunt)

**Signature:** `flush_input(player [, show_messages]) → none`

**Description:** Clears player's pending input queue.

**Parameters:**
- `player` (OBJ): Target player
- `show_messages` (ANY, optional): If truthy, show messages about flushed commands

**Wizard only.**

---

### 8.7 output_delimiters (ToastStunt)

**Signature:** `output_delimiters(player) → LIST`

**Description:** Returns the output delimiters set for the player's connection.

**Parameters:**
- `player` (OBJ): Target player

**Returns:** List containing the output prefix and suffix delimiters.

---

### 8.8 switch_player (ToastStunt)

**Signature:** `switch_player(old_player, new_player [, include_queued]) → none`

**Description:** Transfers connection from one player object to another.

**Parameters:**
- `old_player` (OBJ): Current player object
- `new_player` (OBJ): Target player object
- `include_queued` (INT, optional): If truthy, transfer queued input as well

**Wizard only.**

---

### 8.9 connected_seconds (ToastStunt)

**Signature:** `connected_seconds(player) → INT`

**Description:** Returns how long player has been connected in seconds.

**Parameters:**
- `player` (OBJ): Target player

**Returns:** Number of seconds since connection established.

---

### 8.10 idle_seconds (ToastStunt)

**Signature:** `idle_seconds(player) → INT`

**Description:** Returns how long player has been idle in seconds.

**Parameters:**
- `player` (OBJ): Target player

**Returns:** Number of seconds since last input from player.

---

### 8.11 connection_name_lookup (ToastStunt)

**Signature:** `connection_name_lookup(player [, include_all]) → STR or LIST`

**Description:** Reverse lookup of connection information by player object.

**Parameters:**
- `player` (OBJ): Target player
- `include_all` (ANY, optional): If truthy, return all connection details

**Returns:** Connection name string or list of connection details.

---

## 9. Exception Raising

### 9.1 raise

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

## 10. Error Handling

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
