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
selected. Because the child starts only after the forking slice has ended, it
observes every write the parent made after the `fork` statement in that slice,
even when the forking verb's programmer differs from the connected player
(`fork/fork_timing.yaml::fork_zero_delay_child_observes_rest_of_forking_slice`).

**Start time:** The delay, integer or float, is added to the current time as a
double (`double_to_start_tv`, tasks.cc), so very long delays are honoured
rather than overflowing: a `fork (10000000000)` or `fork (1.0e10)` task is
reported by `queued_tasks()` with a start time at least `time() + 9999999990`
(`fork/fork_timing.yaml::fork_ten_billion_second_*`).

**Task ID binding:** When using `fork task_id (delay)`, the task_id variable is
bound synchronously when the fork statement executes, before the parent continues.
The parent can use the ID immediately after the fork statement.

### 3.3 Task ID

Every running task has an opaque, nonzero integer ID:

```moo
id = task_id();  // Get current task's ID
```

Toast's `new_task_id()` (tasks.cc) draws a nonzero value from its PRNG
(`RANDOM`, normally `lrand48`, `include/random.h`) into a C `int`, so IDs lie in
1..2147483647. Task IDs are not a monotonic allocation sequence, and the
allocator itself does not scan existing tasks for collisions. Saved task IDs
are read back with `%d` into an `int` (`read_task_queue`).

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
onward. Both are tested against the budget before the opcode runs (extended
opcodes since ToastStunt 2.8, fdfa9ae). Consequently, a singleton-list opcode costs one tick while empty-list
construction and list-tail/append opcodes do not; variable reads cost no tick,
assignments cost one, and the builtin-call opcode costs one. Storing a caught
error into a named except variable (`except f (ANY)`) is an ordinary `OP_PUT`,
so it charges and tests a tick like any assignment; a loop built only from that
store and extended opcodes still ends with a ticks abort
(`vm/named_except_tick_budget.yaml`). The complete
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
- The wake time is computed like a fork's start time (`double_to_start_tv`), so
  `suspend(10000000000)` is scheduled at least `time() + 9999999990`
  (`fork/fork_timing.yaml::suspend_ten_billion_seconds_*`).
- `suspend(0)` queues the task in `waiting_tasks` behind every entry already due,
  so a task that forks a zero-delay child and then calls `suspend(0)` resumes
  only after the child's slice: it observes the child's writes, and a child that
  ran to its own `suspend()` is listed by `queued_tasks()`
  (`fork/fork_timing.yaml::suspend_zero_read_only_parent_observes_ready_child`,
  `::suspend_zero_queued_tasks_lists_child_suspended_first`).

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

**Semantics** (Toast `do_resume`, tasks.cc):
- `resume()` reaches exactly two kinds of task:
  - a task waiting in `suspend()`, which it makes runnable now with `value`,
    moving it at once to the background queue of the programmer it is
    checked against (`enqueue_bg_task`). A resumer that then calls
    `suspend(0)` therefore resumes only after the target's next slice
    (`fork/fork_timing.yaml::suspend_zero_after_resume_observes_resumed_slice`).
    A task stays "waiting in `suspend()`" until it actually runs again: one in
    `suspend(0)`, or one whose timed suspension expired while another task
    was running, can still be resumed and receives `value`
    (`fork/fork_timing.yaml::resume_reaches_task_*`);
  - a task already resumed and waiting in a runnable background queue, whose
    pending value it replaces, leaving its place in the queue.
- `value`, defaulting to `0`, becomes the result delivered at the suspension
  point.
- Permission: the caller must be a wizard or the programmer of the verb the
  target is suspended in (the programmer of its top activation,
  `progr_of_cur_verb`), which need not be the programmer that started the
  task. For an already-resumed task it is the programmer whose background
  queue holds it, which is that same programmer at the time of the resume.
  Otherwise `E_PERM`.
- Any other ID receives `E_INVARG`: no such task, the current task, a forked
  task that has not yet started, a task waiting in `read()` or `read_http()`,
  and a task
  waiting on `exec()`.

---

## 7. Task Killing

### 7.1 Kill Task

```moo
kill_task(task_id);
```

**Permissions** (Toast `kill_task`, tasks.cc): the caller must be a wizard or
the programmer the located task is checked against, which depends on where it
is:
- a forked task that has not yet started: the programmer that executed the
  `fork`;
- a task waiting in `suspend()`: the programmer of the verb it is suspended in
  (its top activation, `progr_of_cur_verb`), which need not be the programmer
  that started it;
- a task waiting in a background queue (resumed or forked, about to run): the
  programmer whose queue holds it;
- a task blocked in `read()` or `read_http()`: the player of the connection
  it reads from, not the programmer of the verb that called it.

Killing the current task aborts it with `ABORT_KILL`. A located task the
caller may not kill yields `E_PERM`; an unknown task ID yields `E_INVARG`.

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
`task_stack()` (execute.cc `bf_task_stack`, tasks.cc `find_suspended_task`)
finds a task waiting in `suspend()` (including one already resumed and not yet
run), in `read()` or `read_http()`, or in an external queue such as `exec()`
or a background thread. It includes the task's current activation and requires
the caller to be a wizard or the programmer of the verb the task is paused in
(`progr_of_cur_verb`). Any other ID raises `E_INVARG`, including the current
task and a forked task that has not yet started
(`builtins/task_management.yaml`); another programmer receives `E_PERM`.

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

A verb call or `pass()` refused with `E_MAXREC` at the activation-depth limit
leaves the calling activation unchanged: `call_verb2` (execute.cc) tests
`push_activation()` after verb lookup and before it sets the new activation's
programmer or thread mode. A programmer-owned verb that catches that
`E_MAXREC` from calling a wizard-owned verb keeps its own `task_perms()`,
`caller_perms()`, thread mode, and lack of wizard authority
(`vm/max_recursion_caller_context.yaml`).

### 10.3 Server-Initiated and Nested Callback Context

A server callback that starts a new task (`#0:server_started`,
`checkpoint_started`, `checkpoint_finished`, `do_start_script`,
`handle_signal`, listener input and connection hooks, deferred recycling, and
similar) runs through `run_server_task_setting_id` (tasks.cc), which allocates
a fresh task ID and an empty task-local map. When no callable verb is found
(absent, or present without the `x` flag) it simulates an empty verb returning
`0`. `handle_uncaught_error`, `handle_task_timeout` and `handle_lagging_task`
are instead run by `do_task` (execute.cc) through `do_server_verb_task` after
the failing or lagging run ends, with `player` set to that run's
root-activation player. Both paths build the root activation in
`do_server_program_task` (execute.cc):
- `this` is the supplied receiver; the programmer is the verb's owner;
- `player`, `args` and `argstr` are what the dispatch supplies;
- `caller` is `#-1`, so `caller_perms()` is `#-1` and `callers()` is `{}`;
- `dobj` and `iobj` are `#-1`; `dobjstr`, `iobjstr` and `prepstr` are `""`;
  `verb` is the supplied name;
- the task runs with foreground limits and default thread mode.

What the dispatches supply (each row is pinned by the suites cited below, which
assert every field above):

| Callback | `this` | `player` | `args` | `argstr` |
|---|---|---|---|---|
| `server_started` | `#0` | `#-1` | `{}` | `""` |
| `do_start_script` | `#0` | `#-1` | the script's lines | `""` |
| `checkpoint_started` | `#0` | `#-1` | `{}` | `""` |
| `checkpoint_finished` | `#0` | `#-1` | `{1}` on success, `{0}` on failure | `""` |
| `handle_uncaught_error` | `#0` | originating player | `{code, message, value, stack, traceback}` | `""` |
| `handle_task_timeout` | `#0` | originating player | `{"ticks" or "seconds", stack, traceback}` | `""` |
| `handle_lagging_task` | `#0` | originating player | `{stack or partial frame, elapsed FLOAT}` | `""` |
| `do_login_command`, `do_command`, `do_out_of_band_command`, `do_blank_command` | listener object | the connection (negative ID) before login, else the user | input words | the whole input line |
| `user_connected`, `user_created`, `user_reconnected`, `user_disconnected`, `user_client_disconnected` | listener object | affected user, or connection ID before login | `{that player}` | `""` |
| deferred `recycle` of an anonymous object, WAIF `:recycle` | the object | `#-1` | `{}` | `""` |

Sources: `server/server_started_callback_context.yaml`,
`server/startup_script_callback_context.yaml`,
`server/checkpoint_callback_context.yaml`,
`server/checkpoint_failure_callback_context.yaml`,
`server/checkpoint_io_failure_callback_context.yaml`,
`server/error_callback_context.yaml`, `server/error_handler_outcomes.yaml`,
`server/lag_callback_context.yaml`, `server/input_callback_context.yaml`,
`server/connection_callback_context.yaml`,
`server/deferred_callback_context.yaml`. Further pinned details:
- `do_start_script` receives a `--start-line` argument as a one-element list
  and a `--start-script` file as one element per line (blank lines kept); with
  several options it is called once per option in command-line order, and every
  call precedes `server_started`. With no callable `do_start_script` the
  argument is not executed as code.
- `server_started` runs once per boot whatever it returns, raises, or
  suspends. `checkpoint_started` and `checkpoint_finished` outcomes do not stop
  the checkpoint. After a failed open, write or publication,
  `checkpoint_finished` gets `{0}`, and a later checkpoint succeeds with `{1}`.
- `handle_uncaught_error` runs after the failing task's `finally` blocks have
  run, for foreground, forked and resumed failures; a caught error, and a
  `kill_task()`, invoke neither it nor `handle_task_timeout`. Its stack frames
  are `{this, verb, programmer, verb_loc, player, line}`. A true return or a
  suspension suppresses the traceback; a false return, or an absent or
  non-executable handler, prints the original traceback to the player; a
  handler that raises prints its own traceback and then the original one.
- `handle_lagging_task` fires only when `$server_options.task_lag_threshold` is
  at least `0.1` and a run's elapsed time reaches it. For a run that completed,
  its first argument is a six-element frame `{#-1, verb names, #-1,
  verb location, #-1, #-1}` (execute.cc `do_task` builds `lag_info`); for one that aborted or suspended it is the stack list.
- A listener's `do_command` that returns false lets native command parsing run;
  a true return, an error, or a suspension (even one that later returns false)
  stops it. Native dispatch, and `huh` when no verb matches (on the player or
  its location, per `$server_options.player_huh`, with no fallback to the
  other), get `verb` = the typed command word, `caller` = `player` = the user,
  the parsed `args`, `argstr`, `dobj`, `dobjstr`, `prepstr`, `iobj` and
  `iobjstr`, and still `caller_perms()` `#-1` and `callers()` `{}`.
- `user_created` is chosen only when the logged-in object's number is above the
  highest object number before login; a recreated object below it gets
  `user_connected`.
- A deferred anonymous or WAIF recycle hook runs once, after the last
  reference is dropped, and its `verb` is `"recycle"` or `":recycle"` (the WAIF
  colon is kept on this path).

A callback a builtin makes inside the running task (`:initialize`, `:accept`,
`:exitfunc`, `:enterfunc`, `:recycle` from `recycle()`, WAIF `:_index` and
`:_set_index`, `#0:bf_*` protected-builtin wrappers) is an ordinary nested
verb call (`call_verb2`, execute.cc):
- its programmer is the callback verb's owner, and `caller_perms()` is the
  calling verb's programmer;
- `caller` is the calling activation's `this`;
- `argstr`, `dobj`, `dobjstr`, `prepstr`, `iobj` and `iobjstr` are copied from
  the calling activation's variables, including values the caller assigned;
- `player` is copied from the caller's `player` variable when the caller's
  programmer is a wizard and that variable holds an object, and is otherwise
  the calling activation's player;
- for WAIF `:_index`/`:_set_index` the `verb` variable drops the leading `:`
  (`"_index"`), while `callers()` shows `":_index"`;
- these fields are unchanged after the callback suspends and resumes.

For a callback made from inside `create()`, `recreate()`, `move()` or
`recycle()`, `callers()[1]` is a builtin frame `{#-1, builtin name, #-1, #-1,
player}` and `callers()[2]` is the calling verb's frame. A `#0:bf_*` wrapper
and the WAIF index hooks see the calling verb's frame directly as
`callers()[1]`. Pinned specifics:
- `:initialize` gets `create()`'s initializer list (`{}` from `recreate()`); an
  absent or non-executable `:initialize` is skipped; an error it raises
  propagates out of `create()`.
- `:accept`, `:exitfunc` and `:enterfunc` run in that order and get
  `{moved object}`; `:exitfunc` and `:enterfunc` see the object already in its
  new location. `recycle()` of a located object or of a container calls
  `:exitfunc` after the contents have moved to `#-1`.
- An explicit `recycle()` calls `:recycle` once; it is not called again when
  the value is later dropped.
- A protected `#0:bf_<name>` wrapper receives the raw argument list before
  native argument-count and type checks.
- A WAIF `:_set_index` hook's return value replaces the WAIF in the variable,
  while the assignment expression's value is the assigned value. A missing,
  non-executable, or non-wizard-owned hook raises `E_TYPE`, and an invalid
  class `E_INVIND`, without calling it.

Sources: `builtins/builtin_callback_context.yaml`,
`builtins/move_callback_context.yaml`,
`builtins/recycle_exit_callback_context.yaml`,
`builtins/protected_callback_context.yaml`,
`builtins/waif_index_callback_context.yaml`, `builtins/waif_index_dispatch.yaml`.
For a requested checkpoint, `checkpoint_started` runs after the
`dump_database()` caller's run has finished
(`server/dump_database_hook_order.yaml`).

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

### 11.4 Tasks Across a Checkpoint Restart

`write_task_queue()` (tasks.cc) saves every queued fork and every suspended
task, whether still in `waiting_tasks` or already in a background queue.
A fork is saved as its activation, its runtime environment and the unparsed
code of its fork body (`write_forked_task`, `dbio_write_forked_program`); a
suspended task as its pending resume value and whole VM (`write_suspended_task`,
`write_vm`). Start times are rounded to whole seconds. After loading such a
checkpoint, each task is still queued with its task ID, has not run early, and
runs its remaining code when due. Managed conformance tests pin these shapes:

- A fork with an empty body (`fork t (3600) endfork`) stays queued across two
  successive checkpoint restarts, and does not stop the second checkpoint from
  being written (`server/empty_fork_checkpoint_restart.yaml`).
- A fork body ending in a compound statement (`if`, `while`, `try ... finally`,
  a nested `fork`) runs its whole body after restart
  (`server/fork_trailing_block_checkpoint_restart.yaml`).
- In a named fork, the child's copy of the fork variable holds the child's own
  task ID, not the parent's binding before the fork (`enqueue_forked_task2`
  stores the ID in the activation's environment before `copy_rt_env`), and
  that value survives a restart (`server/fork_variable_checkpoint_restart.yaml`).
- A fork body that is a `try` with computed except codes (`except (E_PERM,
  @codes)`) restores, and the codes still catch
  (`server/fork_try_computed_codes_checkpoint_restart.yaml`).
- `eval()` source with lines beginning with `.` restores both as a suspended
  eval frame and as a queued fork body
  (`server/dot_source_line_checkpoint_restart.yaml`).
- A task suspended inside guarded code whose handler codes were computed
  before the suspension (`` `{suspend(4), 1 / 0} ! c, E_RANGE => 5' `` and
  `try ... except e (c, E_PERM)`) keeps the evaluated codes, which catch after
  restart (`server/suspended_computed_codes_checkpoint_restart.yaml`).
- A task suspended indefinitely inside a catch expression's handler, nested
  under pending operands, resumes after restart at the catch's entry depth:
  `` {1 + `1/0 ! ANY => 5 + suspend()', {`1/0 ! E_DIV => "d"'}} `` resumed with 10
  yields `{16, {"d"}}` (`server/catch_handler_suspend_checkpoint_restart.yaml`).
- A task suspended with no timeout is still queued and unresumed after restart
  (`server/restart_task_semantics.yaml::indefinitely_suspended_task_across_checkpoint_restart`).
- A restored fork that is already overdue at load runs only after
  `#0:server_started` returns (`server.cc` runs `server_started` before
  `main_loop()`;
  `server/restart_task_semantics.yaml::restored_overdue_fork_runs_after_server_started_returns`).

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

Barn represents a MOO task as explicit heap data in `task/task.go`; the task
catalog builtins consult is `engine/task_manager.go`. A task is not a Go
goroutine.

Fork source is compiled by `compiler/` (`lower_control.go`). `vm/control.go`
captures the fork body and locals and returns `FlowFork`;
`engine/task_factory.go` (`CreateForkedTask`) allocates and queues the child;
and `engine/task_runtime.go` binds the child ID and resumes the parent in
`drainForks()` before returning from the parent's run.
`engine/scheduler.go` keeps ready tasks in one ready-time heap with
enqueue-sequence ties and dispatches retry-compatible batches to a pool of
workers, so several task slices can run at once. Each slice runs in an MVCC
transaction (`db/store`); a commit that read stale data aborts and the slice
is retried. `newTaskID()` (`engine/runtime.go`) counts up from the highest
loaded ID, wraps to 1 after 2147483647 (Toast's C `int` range), and skips IDs
the catalog still holds.

Task builtins are registered in `builtins/registry.go`. Queue inspection,
kill, suspend, resume, permissions, callers, task stacks, `yin()`,
`finished_tasks()` and `set_thread_mode()` are in `builtins/tasks.go`;
task-local state, task ID, and remaining-limit accessors are in
`builtins/system.go`; compiled and `$server_options` limit values are in
`builtins/limits.go`. Thread mode is saved and restored per activation in
`vm/op_verb.go`. Task control (`kill_task`, `resume`) stages task rows checked
against the owner Toast checks (`engine/task_rows.go`).

Queued, suspended and interrupted task records are parsed in
`db/format/reader_task.go`, restored in `engine/task_load.go`, and written by
`db/format/writer_task.go`. `Runtime.TaskSnapshots()` (`engine/task_factory.go`)
exports each suspended VM in Toast's frame form (`compiler/native_export.go`);
a frame that form cannot hold stays in Barn's own frame form, which Barn
restores but Toast cannot load, and the export logs a warning naming the task.

Go goroutines are launched in the server/connection layer, the scheduler's
workers, and selected host builtins. Their completions re-enter explicit task
state.

### 13.1 Known current Barn differences from Toast

- Barn's task IDs come from a counter, not Toast's PRNG `new_task_id()`.
- Barn uses one global ready-time heap with enqueue-sequence ties and runs
  several slices concurrently under MVCC. Toast first promotes a time-ordered
  waiting list into per-programmer FIFO background queues, then runs one task
  at a time from the lowest-usage active programmer queue, giving its input
  precedence over background work.
- Because slices run concurrently, `queued_tasks()` also lists forked or
  resumed tasks that are running in another slice (never the caller), with the
  start time and line Toast would show (`engine/task_manager.go`
  `GetQueuedTasks`). In Toast only the caller can be running.
- `yin()` in a non-forked task's root VM that has run at least 10000 ticks in
  its current slice yields for 0 seconds even when Toast's thresholds are not
  reached (`builtins/tasks.go` `builtinYin`), so the slice commits. The task
  then continues under fresh background limits where Toast would have returned
  `0` without suspending.
- Barn returns `E_INTRPT` directly when `kill_task()` targets the current task;
  Toast returns an `ABORT_KILL` package that aborts the interpreter.

The normative behavior in Sections 1-12 comes from the verified Toast source
and managed conformance rows. Barn's current code describes implementation
status and must not override observable Toast behavior.
