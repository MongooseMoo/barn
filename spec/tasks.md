# MOO Task Model Specification

## Overview

MOO uses cooperative multitasking with tick-based resource limits. Tasks are units of execution that can be suspended, resumed, and killed.

## Authority

When in doubt, match Toast behavior. The verified WSL source identity,
executable, profile, wrapper, and managed command are recorded in
`../../banteng/docs/reports/toast-oracle-identity-2026-07-14.md`; the managed
workflow itself is owned by
`plans/barn-toast-mongoose-convergence-workstreams.md`. In the verified Toast
source, `src/server.cc` drives the server loop; `src/tasks.cc` owns runnable-task
selection, task-ID allocation, task queues, persisted tasks, and task builtins;
`src/include/options.h` defines the compiled foreground and background limits;
and `src/execute.cc` applies the `$server_options` limit overrides. These paths
are relative to `/root/src/toaststunt` at the source identity recorded above.

---

## 1. Task Types

### 1.1 Foreground Tasks

Created by player input or by the server's initiative, provided the task has
never suspended:
- Higher tick limit (compiled default 60,000)
- Higher time limit (compiled default 5 seconds)

### 1.2 Background Tasks

Created by `fork` statements, or produced when any task resumes after
suspension:
- Lower tick limit (compiled default 30,000)
- Lower time limit (compiled default 3 seconds)

`$server_options.fg_ticks`, `.bg_ticks`, `.fg_seconds`, and `.bg_seconds`
override the compiled defaults.

---

## 2. Task Lifecycle

### 2.1 States

Toast does not expose a general task-state enum. It has input tasks on
per-player queues, the currently executing task, a `waiting_tasks` queue for
forked programs and timed or indefinite suspensions, per-player VMs blocked in
`read()` or HTTP parsing, and registered external task queues for host work. A
new server task runs with foreground limits. A fork waits until its start time.
A suspended VM waits for its wake condition and, when resumed, runs with
background limits. A task that returns or aborts is removed rather than
retained in a completed or aborted queue state.

### 2.2 Lifecycle Diagram

```
player input or server initiative
                |
                v
      run with foreground limits
          |            |
       return/abort    suspend/read/host wait
                       |
                       v
              waiting suspended VM
                       |
                       v
              resume with background limits

fork statement -> waiting forked program -> run with background limits
```

---

## 3. Task Creation

### 3.1 Foreground Task

Created when a player command or server-initiated verb call begins:

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
1. Allocate a nonzero task ID.
2. For a named fork, bind that ID in the parent's runtime environment.
3. Copy the resulting runtime environment for the child.
4. Queue the child for its calculated start time.
5. Continue the parent without running the child at the fork opcode.

**Environment copy:** In Toast, `copy_rt_env()` applies `var_ref()` to every
runtime slot. Observable value updates in one task do not assign a different
value to the corresponding variable in the other task; values with reference
identity retain the identity defined by their type. Reference counts and
copy-on-write are implementation mechanisms, not an additional MOO contract.

**Delay of 0:** Even with `delay = 0`, the forked task is queued and runs on the
server loop after the current interpreter run returns control. The fork opcode
does not yield to the child, so the child cannot run before the parent continues
past that statement. This does not promise that the child is the very next task
selected.

**Task ID binding:** When using `fork task_id (delay)`, the task_id variable is
bound synchronously when the fork statement executes, before the parent continues.
The parent can use the ID immediately after the fork statement.

### 3.3 Task ID

Every running task has an opaque, nonzero integer ID:

```moo
id = task_id();  // Get current task's ID
```

Toast's `new_task_id()` draws a nonzero value from its PRNG. Task IDs are not a
monotonic allocation sequence, and the allocator itself does not scan existing
tasks for collisions.

---

## 4. Tick System

### 4.1 Purpose

Prevent infinite loops and resource hogging:
- Selected opcodes consume ticks
- Task aborted when ticks exhausted
- Configurable limits

### 4.2 Tick Costs

Tick cost is an opcode property, not a general cost table for source-level
operations. In Toast, `COUNT_TICK(op)` charges one tick for every ordinary
opcode through `OP_G_PUT`; later ordinary opcodes do not charge at dispatch.
`COUNT_EOP_TICK(eop)` charges one tick for extended opcodes from `EOP_CATCH`
onward. Consequently, a singleton-list opcode costs one tick while empty-list
construction and list-tail/append opcodes do not; variable reads cost no tick,
assignments cost one, and the builtin-call opcode costs one. The complete
ordering is defined in `src/include/opcode.h` and enforced by `src/execute.cc`.

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
yin();
yin(seconds);
yin(seconds, min_ticks, min_seconds);
```

**Semantics:**
- Defaults are `seconds = 0`, `min_ticks = 2000`, and `min_seconds = 2`.
- Suspend for `seconds` when `ticks_left() < min_ticks` or
  `seconds_left() < min_seconds`; otherwise return `0` immediately.
- A supplied negative delay, nonpositive threshold, tick threshold at or above
  the foreground tick limit, or seconds threshold at or above the foreground
  seconds limit raises `E_INVARG`.
- Resumption uses fresh background tick and seconds limits.

---

## 5. Task Suspension

### 5.1 Explicit Suspension

```moo
suspend();           // Suspend indefinitely
suspend(seconds);    // Suspend for duration
```

**Semantics:**
- Toast captures the explicit VM, including activation stack, runtime
  environments, program counters, task ID, and task-local value.
- With no argument, the task waits indefinitely. With a nonnegative numeric
  argument, it is queued for that many seconds in the future; a negative delay
  raises `E_INVARG`.
- A timed suspension becomes runnable when its wake time arrives. `resume()`
  can make a suspended task runnable earlier.

### 5.2 I/O Suspension

Only builtins that explicitly return a Toast suspension package suspend the
VM. These include `read()` and HTTP request parsing while waiting for input,
`exec()` while waiting for a child process, extension stdin, and host work run
through the enabled background-thread path. “Network operation” by itself does
not imply suspension.

### 5.3 Suspension Limits

`check_user_task_limit()` applies the programmer's `queued_task_limit`
property, falling back to `$server_options.queued_task_limit`. The count covers
queued background tasks, including both forks and suspended tasks; it is not a
separate suspended-task limit.

Once a foreground task suspends, Toast resumes it under the background tick and
seconds limits. The original foreground limits do not survive suspension.

---

## 6. Task Resumption

### 6.1 Automatic Resume

Suspended tasks resume when:
- Delay period expires
- I/O completes
- Input received

### 6.2 Manual Resume

```moo
resume(task_id);
resume(task_id, value);
```

**Semantics:**
- The task's programmer or a wizard can make a suspended task runnable now.
- `value`, defaulting to `0`, becomes the result delivered at the suspension
  point.
- Resuming a suspended task that is already in a runnable background queue
  replaces its pending resume value.
- A different programmer receives `E_PERM`; an ID that does not identify a
  suspended task receives `E_INVARG`.

---

## 7. Task Killing

### 7.1 Kill Task

```moo
kill_task(task_id);
```

**Permissions:**
- The current programmer can kill a queued task owned by that programmer; a
  task blocked in `read()` is owned for this check by the connection player.
- Wizards can kill any located task.
- Killing the current task aborts it with `ABORT_KILL`.
- A located task owned by someone else yields `E_PERM`; an unknown task ID
  yields `E_INVARG`.

### 7.2 Abort Reasons

| Reason | Cause |
|--------|-------|
| ABORT_TICKS | Tick limit exceeded |
| ABORT_SECONDS | Time limit exceeded |
| ABORT_KILL | Explicit kill_task() |

Unhandled MOO errors also terminate a task, but they are not an
`abort_reason` enum value in Toast.

---

## 8. Task Inspection

### 8.1 Current Task

```moo
task_id()      // Current task ID
task_perms()   // Current activation's programmer
caller_perms() // Calling activation's programmer, or #-1 at the root
```

### 8.2 Queued Tasks

```moo
queued_tasks()
queued_tasks(include_variables)
queued_tasks(ignored, return_count)
```

With two arguments and a true `return_count`, this returns the number of tasks
visible to the caller. Otherwise it returns a list. Wizards see all queues;
other programmers see their own tasks. A true single argument appends a runtime
variable map to each entry.

Each ordinary entry has this layout:

```moo
{task_id, start_time, 0, 30000, programmer, verb_loc, verb_name,
 line, this, bytes, [variables]}
```

Fields 3 and 4 are obsolete clock fields retained for compatibility; field 4
is the compiled `DEFAULT_BG_TICKS`, not the active server override. A task
blocked in `read()` uses `-1` for `start_time`. External task queues may use a
status string in that field.

### 8.3 Task Stack

```moo
callers()
callers(include_line_numbers)

task_stack(task_id)
task_stack(task_id, include_line_numbers)
task_stack(task_id, include_line_numbers, include_variables)
```

`callers()` reports calling activations and excludes the current activation.
`task_stack()` finds a suspended task, includes its current activation, and
requires the caller to be that task's programmer or a wizard. An invalid or
non-suspended ID raises `E_INVARG`; another programmer receives `E_PERM`.

Each frame has this layout:

```moo
{this, verb_name, programmer, verb_loc, player, [line_no]}
```

When `task_stack()` includes variables, the runtime-variable map follows the
optional line number. A suspended builtin continuation may appear as a frame
whose object and programmer fields are `#-1` and whose verb name is the builtin
name.

---

## 9. Task-Local Storage

### 9.1 Setting Values

```moo
set_task_local(value);
```

### 9.2 Getting Values

```moo
value = task_local();
```

### 9.3 Semantics

- Both builtins require wizard permission and raise `E_PERM` otherwise.
- The value is one arbitrary MOO value, not a keyed store.
- A new foreground task and a forked child each start with an empty map.
- The value survives suspension and resumption with the same task.
- A forked child does not inherit its parent's task-local value.

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
task_perms()         // Current activation's programmer
caller_perms()       // Calling activation's programmer, or #-1 at the root
set_task_perms(obj)  // Change the current activation's programmer
```

A non-wizard may set permissions only to the current programmer; a wizard may
set them to another object. The check is against the running programmer, not
the connected `player`. The change affects the top activation.

---

## 11. Scheduling

### 11.1 Task Queue

Toast keeps forked and timed-suspended tasks in `waiting_tasks`, ordered by
start time; equal start times retain insertion order. At the start of
`run_ready_tasks()`, all due entries are moved in that order to FIFO background
queues keyed by programmer.

Runnable programmer queues are ordered by accumulated usage. Within the chosen
queue, runnable input is selected before background work unless input is held;
background work is FIFO. This is not one global earliest-time FIFO queue.

### 11.2 Execution Model

The main loop in `src/server.cc` calls `run_ready_tasks()` once per iteration.
That call chooses at most one runnable input, forked, or suspended task from the
lowest-usage active programmer queue and runs it until it returns, suspends, or
aborts. Forks created during that interpreter run are queued; they do not run
inside the parent opcode.

### 11.3 Time Slicing

No preemption:
- A task runs until it returns, suspends, or aborts.
- Tick-counting opcodes enforce tick and elapsed-time limits; exhaustion aborts
  rather than preempting and later resuming the same execution slice.
- Long-running code can use `yin()` to suspend conditionally and resume later
  with background limits.

---

## 12. Error Handling in Tasks

### 12.1 Unhandled Errors

If error propagates to top level:
- The interpreter records `handle_uncaught_error` handler data and aborts the
  task outcome.
- With database tracebacks enabled, Toast invokes `#0:handle_uncaught_error`
  when present. If that handler reports the event handled or suspends, default
  notification stops; otherwise traceback lines are sent to the root
  activation's `player`.
- Tick and seconds aborts similarly record `handle_task_timeout` data before
  the abort unwind.

### 12.2 Error in Fork

Background task errors don't affect parent:
- The parent has already continued after queueing the fork.
- The child aborts independently and uses the same handler/traceback path as
  other task entry points.

---

## 13. Current Barn implementation map (non-normative)

Barn represents a MOO task as explicit heap data in `task/task.go`; task lookup
for builtins is in `task/manager.go`. A task is not a Go goroutine.

Fork source is compiled by `bytecode/compiler.go`. `vm/control.go` captures the
fork body and locals and returns `FlowFork`; `scheduler/task_factory.go`
allocates and queues the child; and `scheduler/task_runtime.go` binds the child
ID and resumes the parent in `drainForks()` before returning from the parent's
run. `scheduler/task_queue.go` defines Barn's global ready-time heap, while
`scheduler/scheduler.go` selects and executes at most one ready task per call.
`server/input_processor.go` owns the outer serialized input/scheduler loop.

Task builtins are registered in `builtins/registry.go`. Queue inspection,
kill, suspend, resume, permissions, callers, task stacks, and `yin()` are in
`builtins/tasks.go`; task-local state, task ID, and remaining-limit accessors
are in `builtins/system.go`; compiled and `$server_options` limit values are in
`builtins/limits.go`.

Queued-task parsing is in `db/format/reader_task.go`, restoration is in
`scheduler/task_load.go`, and task output is in `db/format/writer_task.go`.
The current writer emits zero suspended and interrupted tasks, and the current
reader skips those records rather than restoring them. Only serializable queued
forks are restored today.

Go goroutines are launched in the server/connection layer and selected host
builtins. Their completions re-enter explicit task state; MOO VM selection and
execution are still driven by the serialized scheduler loop.

### 13.1 Known current Barn differences from Toast

- `scheduler.newTaskID()` allocates monotonically with an atomic counter;
  Toast's `new_task_id()` draws a nonzero PRNG value.
- Barn uses one global ready-time heap with enqueue-sequence ties. Toast first
  promotes a time-ordered waiting list into per-programmer FIFO background
  queues, then selects the lowest-usage active programmer queue and gives its
  input precedence over background work.
- Barn's `queued_tasks()` currently interprets its first argument as a player
  filter. Toast treats a true single argument as `include_variables`; with two
  arguments only the second, `return_count`, controls the result mode.
- Barn accepts but ignores `task_stack()`'s third `include_variables` flag.
- Barn returns `E_INTRPT` directly when `kill_task()` targets the current task;
  Toast returns an `ABORT_KILL` package that aborts the interpreter.
- Barn does not currently round-trip suspended or interrupted task VMs through
  the database codec.

The normative behavior in Sections 1-12 comes from the verified Toast source
and managed conformance rows. Barn's current code describes implementation
status and must not override observable Toast behavior.
