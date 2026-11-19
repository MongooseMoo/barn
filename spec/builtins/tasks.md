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

**Description:** Returns the programmer of the calling activation, or `#-1`
in the task's root activation (execute.cc `bf_caller_perms`). In a server
callback's root activation it is therefore `#-1` (see
[tasks.md §10.3](../tasks.md)).

---

### 1.4 set_task_perms

**Signature:** `set_task_perms(who) → none`

**Description:** Sets the programmer of the current (top) activation
(execute.cc `bf_set_task_perms`).

**Permissions:** A wizard may set any object; a non-wizard only its current
programmer, otherwise `E_PERM`.

**Examples:**
```moo
set_task_perms(#wizard);  // Run as wizard
```

---

### 1.5 task_perms (ToastStunt)

**Signature:** `task_perms() → OBJ`

**Description:** Returns the programmer of the current activation
(execute.cc `bf_task_perms`, `RUN_ACTIV.progr`). This differs from
`caller_perms()`, which reports the calling activation's programmer.

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

**Signature:** `task_stack(task_id [, line_numbers [, include_vars]]) → LIST`

**Description:** Returns the stack of a task waiting in suspend() (including
one already resumed and not yet run), in read() or read_http(), or on exec() (Toast
execute.cc bf_task_stack, tasks.cc find_suspended_task).

**Parameters:**
- `task_id` (INT): Task ID to inspect (required)
- `line_numbers` (ANY, optional): Include each frame's line number if truthy
- `include_vars` (ANY, optional): Include each frame's variables if truthy

**Returns:** List of frames with more detail than callers().

**Errors:**
- E_INVARG: No such task among those above (including the current task and a
  forked task that has not yet started)
- E_PERM: Caller is neither a wizard nor the programmer of the task's current
  verb

---

### 2.3 queued_tasks

**Signature:** `queued_tasks([include_vars [, return_count]]) → LIST or INT`

**Description:** Returns list of queued (waiting) tasks, or count if requested.

**Parameters:**
- `include_vars` (INT or BOOL, optional): If truthy, include variable info in task records
- `return_count` (INT or BOOL, optional): If truthy, return count (INT) instead of list

Both are `TYPE_FLAG` arguments (tasks.cc `register_function("queued_tasks", 0,
2, ..., TYPE_FLAG, TYPE_FLAG)`): an INT or BOOL is accepted, any other type
raises `E_TYPE` (`builtins/flag_args_accept_bool.yaml`).

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

**Description:** Returns a record of each recent interpreter run, oldest first. Wizard only (`E_PERM` otherwise).

**Notes:**
- In ToastStunt, only available when compiled with `SAVE_FINISHED_TASKS`; stock builds don't have it. A build with it reports the configured limit as `SAVE_FINISHED_TASKS` in `server_version("options")` (15 in the build the conformance suite uses).
- One record is added per run: a new task, and each resumption after a suspend. Fields describe the activation on top of the stack when the run started.
- The newest `$server_options.finished_tasks_limit` records are kept (default 15, never fewer than 1).

**Returns:** List of maps with these keys:

| Key | Value |
|---|---|
| `this` | `this` of the top activation |
| `player` | player |
| `programmer` | effective programmer |
| `receiver` | object the verb was looked up on (`#-1` for an anonymous object) |
| `object` | object defining the verb |
| `verb` | verb name as called, or `"n/a"` if empty |
| `fullverb` | the verb's whole names string, or `"n/a"` if empty |
| `foreground` | 1 for a new command or server task, 0 for a forked task or a resumption |
| `suspended` | 1 if the run ended by suspending |
| `time` | wall seconds the run took (FLOAT) |

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

**Description:** Makes a task waiting in suspend() runnable now, or replaces
the pending value of one already resumed. See [tasks.md §6.2](../tasks.md).

**Parameters:**
- `task_id`: Task to resume
- `value`: Value returned by suspend() (default: 0)

**Examples:**
```moo
resume(other_task, "wake up!");
```

**Errors:**
- E_INVARG: No task waiting in suspend() or already resumed has this ID
  (including the current task, a task in read() or read_http(), and a forked
  task not yet
  started)
- E_PERM: Caller is neither a wizard nor the programmer of the verb the task
  is suspended in

---

### 3.3 kill_task

**Signature:** `kill_task(task_id) → none`

**Description:** Terminates a task. See [tasks.md §7.1](../tasks.md).

**Permissions:**
- Wizards can kill any task
- Otherwise the caller must be the programmer the task is checked against: the
  forking programmer for a fork not yet started, the programmer of the verb a
  suspended task is suspended in, the queue's programmer for a task waiting in
  a background queue, or the connection's player for a task in read() or
  read_http()

**Examples:**
```moo
kill_task(runaway_task);
```

**Errors:**
- E_INVARG: Invalid task ID
- E_PERM: Caller is neither a wizard nor the programmer the task is checked
  against

---

## 4. Resource Limits

### 4.1 ticks_left

**Signature:** `ticks_left() → INT`

**Description:** Returns the ticks remaining in the current interpreter run
(execute.cc `bf_ticks_left` returns `ticks_remaining`).

**Note:** Every run has a tick budget. `run_interpreter` sets it from
`fg_ticks` for a new foreground task (a command, including a player's `;`
eval command, or a server task) and from `bg_ticks` for a forked task or a
resumption, with a floor of 100 (`setup_task_execution_limits`, execute.cc).
`eval()` code runs inside the calling task (verbs.cc `bf_eval` pushes an
activation with `setup_activ_for_eval` and returns a call pack), so it reports
and consumes the caller's budget; it never reports 0 for lack of a limit.

---

### 4.2 seconds_left

**Signature:** `seconds_left() → INT`

**Description:** Returns the whole seconds remaining before the current run's
time limit (execute.cc `bf_seconds_left` returns
`timer_wakeup_interval(task_alarm_id)` as an INT).

**Note:** As with `ticks_left()`, every run has a limit, set from `fg_seconds`
or `bg_seconds` with a floor of 1 second (`setup_task_execution_limits`), and
`eval()` code shares the calling task's limit.

---

### 4.3 yin (ToastStunt: yield if needed)

**Signature:** `yin([seconds [, min_ticks [, min_seconds]]]) → VALUE`

**Description:** Suspends the task only if it is low on ticks or seconds
(execute.cc `bf_yield_if_needed`). See [tasks.md §4.5](../tasks.md).

**Parameters:**
- `seconds` (NUMERIC, optional): How long to suspend if it yields (default 0)
- `min_ticks` (INT, optional): Yield when `ticks_left() < min_ticks` (default 2000)
- `min_seconds` (INT, optional): Yield when the seconds left are below
  `min_seconds` (default 2)

**Semantics:**
- Otherwise returns `0` at once without suspending.
- When it yields, the task resumes as a background task with fresh background
  limits, and `yin()` returns the value passed to `resume()` (0 by default).
- With any argument supplied, a negative `seconds`, a nonpositive threshold, or
  a threshold at or above `fg_ticks` / `fg_seconds` raises `E_INVARG`.

**Examples:**
```moo
for i in [1..10000]
    yin();         // Yield only when fewer than 2000 ticks or 2 seconds remain
    process_item(i);
endfor
```

---

### 4.4 set_thread_mode (ToastStunt)

**Signature:** `set_thread_mode([mode]) → INT`

**Description:** Controls whether certain builtins run in background threads for the current task activation.

**Parameters:**
- `mode` (INT or BOOL, optional; `TYPE_FLAG`, any other type raises `E_TYPE`): If omitted, returns the current mode. If provided, sets the mode to its truth value.

**Returns:**
- No arguments: current mode (INT)
- With argument: returns 0

**Notes:**
- Default mode is `DEFAULT_THREAD_MODE` (true in the reference build, `options.h`).
- This affects builtins implemented via the background thread system, such as `sort`, `all_members`, `locate_by_name`, `occupants`, `connection_name_lookup`, `curl`, `argon2`, `argon2_verify`, `sqlite_query`, and `sqlite_execute`.

**Activation scope:** The mode belongs to the running activation, not the task
(`get_thread_mode`/`set_thread_mode` read and write `RUN_ACTIV.threaded`,
execute.cc). Every new activation starts with `DEFAULT_THREAD_MODE`: a verb
call and a `pass()` target (`call_verb2(..., DEFAULT_THREAD_MODE)`, execute.cc),
`eval()` code (`setup_activ_for_eval`), and a forked task (tasks.cc
`enqueue_forked_task2` and `run_ready_tasks`). Consequently:
- A callee's change never reaches its caller, on normal return in either
  direction and on error unwind.
- A verb called, or a `pass()` target reached, from an activation with the mode
  off starts with it on; so do `eval()` code and a forked child.
- An activation keeps its own mode across `suspend()` and resumption.

**Threaded calls** (background.cc `background_thread`): with the calling
activation's mode on, every such call suspends the task, even one with no work
to do (`sort({})`); other tasks run meanwhile. The task resumes as a background
task (`resume_from_previous_vm`, execute.cc), so `ticks_left()` and
`seconds_left()` restart at the background limits after the call. An error
value produced by the work is raised at the call site on resumption. Before
suspending, the call is refused with `E_QUOTA` when the thread table is full
(`max_background_threads`) or the programmer's queued-task limit is reached.
With the mode off the work runs inline: the task does not suspend, its budget
is not reset, and an error value from the work is returned as the call's value
rather than raised (for example `sort({1, "a"})` returns `E_TYPE`). Argument
type and count errors are raised in either mode, before any work starts.

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

**Returns:** The value previously stored with `set_task_local()`, or an empty
map if none. A forked child starts with its own empty map and does not inherit
its parent's task-local value; suspending and resuming one task preserves that
task's value.

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

`server_version()`, `memory_usage()`, `shutdown()` and `dump_database()` are
documented in [server.md](server.md) §1.1, §1.2, §2.1 and §2.2.

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
