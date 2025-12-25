# Blind Implementor Audit: Tasks Feature

## Feature Overview
Auditing MOO's task system including:
- Task lifecycle and states
- Fork delay semantics
- Suspend/resume value passing
- Task killing and cleanup
- Tick counting and limits

## Specification Sources
- `spec/tasks.md` - Core task model
- `spec/builtins/tasks.md` - Task built-in functions

---

## Gaps Identified

### Task Lifecycle & States

- id: GAP-TASK-001
  feature: "Task States"
  spec_file: "spec/tasks.md"
  spec_section: "2.1 States"
  gap_type: guess
  question: |
    Can a task transition from Completed or Aborted back to any other state?
    The state diagram shows linear progression but doesn't explicitly forbid
    state transitions like Aborted → Running (if someone tries to resume it).
  impact: |
    An implementor must decide whether to:
    (a) Silently ignore operations on terminal states
    (b) Raise E_INVARG when trying to manipulate completed/aborted tasks
    (c) Allow resurrection of tasks
    The spec doesn't specify which is correct.
  suggested_addition: |
    Add: "Completed and Aborted are terminal states. Operations like resume()
    or kill_task() on tasks in these states raise E_INVARG."

- id: GAP-TASK-002
  feature: "Task States"
  spec_file: "spec/tasks.md"
  spec_section: "2.1 States"
  gap_type: ask
  question: |
    What is the difference between "Waiting" and "Suspended"? Both appear to
    be non-running states. Is Waiting specifically for time-delayed tasks and
    Suspended for explicitly suspended ones? Can a Suspended task have a delay?
  impact: |
    The distinction affects how queued_tasks() reports tasks and whether
    certain operations are valid. If the states are not mutually exclusive,
    the implementation becomes complex.
  suggested_addition: |
    Add: "Waiting applies to tasks scheduled with a delay (fork with delay > 0)
    that have not yet reached their start time. Suspended applies to tasks
    that called suspend() or are blocked on I/O. A task cannot be both."

- id: GAP-TASK-003
  feature: "Task State Transitions"
  spec_file: "spec/tasks.md"
  spec_section: "2.2 Lifecycle Diagram"
  gap_type: guess
  question: |
    The diagram shows suspend() moving from Running to some waiting state,
    but which one? Does suspend() move to Waiting or Suspended state?
    The text says "Suspended" but the diagram's arrows are unclear.
  impact: |
    Affects state machine implementation and which tasks queued_tasks() returns.
  suggested_addition: |
    Clarify: "suspend() transitions from Running → Suspended. Fork with delay
    creates tasks in Waiting state."

### Fork Statement Semantics

- id: GAP-TASK-004
  feature: "fork delay"
  spec_file: "spec/tasks.md"
  spec_section: "3.2 Fork Statement"
  gap_type: test
  question: |
    When delay is 0, does the forked task run:
    (a) Immediately before the parent continues?
    (b) After the parent's current statement?
    (c) After the parent completes?
    (d) At some unspecified future time when the scheduler picks it?

    The spec says "schedule to run after delay seconds" but delay=0 is ambiguous.
  impact: |
    Critical for reasoning about execution order. Code like:
    ```
    x = 1;
    fork (0)
      notify(player, tostr(x));  // What does this see?
    endfork
    x = 2;
    ```
    Could print 1 or 2 depending on scheduling.
  suggested_addition: |
    Add: "A delay of 0 schedules the task to run as soon as possible, but not
    before the parent task yields control. The parent continues executing
    immediately after the fork statement. The forked task will run on the next
    scheduler cycle."

- id: GAP-TASK-005
  feature: "fork environment copy"
  spec_file: "spec/tasks.md"
  spec_section: "3.2 Fork Statement"
  gap_type: guess
  question: |
    "Copy of current environment" - does this mean:
    (a) Deep copy of all local variables?
    (b) Shallow copy (references to same objects)?
    (c) Copy-on-write?
    (d) Snapshot of variable bindings but not values?

    For objects/lists/maps that are mutable, what does the fork see?
  impact: |
    Affects memory usage and mutation visibility. Critical for correctness:
    ```
    x = {1, 2, 3};
    fork (0)
      x[1] = 99;  // Does this mutate parent's list?
    endfork
    ```
  suggested_addition: |
    Add: "The forked task receives a snapshot of all local variable bindings.
    Values are shallow-copied with MOO's standard copy-on-write semantics.
    Mutations in the forked task do not affect the parent's variables."

- id: GAP-TASK-006
  feature: "fork task_id binding"
  spec_file: "spec/tasks.md"
  spec_section: "3.2 Fork Statement"
  gap_type: ask
  question: |
    When using `fork task_id (delay)`, when is task_id bound?
    (a) Immediately before the fork statement completes?
    (b) When the forked task starts running?
    (c) Asynchronously at some point after fork?

    Can the parent read task_id immediately after the fork statement?
  impact: |
    Affects whether you can do:
    ```
    fork tid (10)
      // ...
    endfork
    notify(player, "Started task " + tostr(tid));  // Is tid bound yet?
    ```
  suggested_addition: |
    Add: "The task_id variable (if specified) is bound to the new task's ID
    immediately when the fork statement completes, before the parent continues
    execution. The parent can use this ID immediately."

- id: GAP-TASK-007
  feature: "fork from suspended task"
  spec_file: "spec/tasks.md"
  spec_section: "3.2 Fork Statement"
  gap_type: guess
  question: |
    Can a suspended task execute a fork statement when it resumes?
    Does the delay start from when the fork executes, or from when the parent
    was originally scheduled?
  impact: |
    Matters for tasks that suspend, then fork:
    ```
    suspend(10);
    fork (5)  // Runs 5 seconds from now, or 15 seconds from original start?
      // ...
    endfork
    ```
  suggested_addition: |
    Add: "Fork delays are measured from the time the fork statement executes,
    not from any earlier suspension point."

### Suspend/Resume Semantics

- id: GAP-TASK-008
  feature: "suspend return value"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "3.1 suspend"
  gap_type: test
  question: |
    If suspend(5) times out (no resume() called), it returns 0.
    If resume(task, value) is called, suspend() returns value.
    But what if resume() is called with 0 as the value?
    How does the suspended task distinguish timeout from explicit resume(task, 0)?
  impact: |
    Ambiguity in protocol. Code cannot reliably detect timeout vs explicit wake:
    ```
    result = suspend(10);
    if (result == 0)
      // Did we timeout, or did someone call resume(task, 0)?
    endif
    ```
  suggested_addition: |
    Add: "On timeout, suspend() returns the integer 0. If this conflicts with
    the protocol, use non-zero values. There is no way to distinguish timeout
    from resume(task, 0)."

    OR better: "On timeout, suspend() returns ERR value indicating timeout,
    allowing distinction from resume(task, 0)."

- id: GAP-TASK-009
  feature: "suspend(0) semantics"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "3.1 suspend"
  gap_type: guess
  question: |
    What does suspend(0) mean?
    (a) Don't suspend at all, return immediately?
    (b) Suspend for 0 seconds (yield and resume on next tick)?
    (c) Same as suspend() with no args (indefinite)?
    (d) Invalid argument?
  impact: |
    Common pattern for "yield to scheduler" might be suspend(0).
    Different interpretations produce very different behavior.
  suggested_addition: |
    Add: "suspend(0) suspends the task and resumes it immediately on the next
    scheduler cycle, effectively yielding control. This is NOT the same as
    suspend() with no arguments (which waits indefinitely)."

- id: GAP-TASK-010
  feature: "suspend negative duration"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "3.1 suspend"
  gap_type: guess
  question: |
    What happens with suspend(-5)?
    (a) E_INVARG?
    (b) Treat as suspend() indefinite?
    (c) Treat as suspend(0)?
    (d) Undefined behavior?
  impact: |
    Validation requirement. Need to know whether to error-check.
  suggested_addition: |
    Add: "Negative durations raise E_INVARG."

- id: GAP-TASK-011
  feature: "resume on non-suspended task"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "3.2 resume"
  gap_type: assume
  question: |
    The spec says resume() raises E_INVARG if "Task not suspended".
    Does this mean:
    (a) Only tasks in Suspended state can be resumed?
    (b) Tasks in Waiting state can also be resumed (waking early)?
    (c) Tasks in Running state raise E_INVARG?
    (d) Tasks in Completed/Aborted state raise E_INVARG?
  impact: |
    Determines valid usage patterns. Can you wake a delayed fork early?
    ```
    fork tid (100)
      // ...
    endfork
    resume(tid);  // Valid? Does this wake the fork early?
    ```
  suggested_addition: |
    Add: "resume() can only wake tasks in Suspended state (blocked on suspend()
    or I/O). Tasks in Waiting state (scheduled with fork delay) cannot be
    resumed early. Tasks in Running, Completed, or Aborted states raise E_INVARG."

- id: GAP-TASK-012
  feature: "multiple resume calls"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "3.2 resume"
  gap_type: test
  question: |
    If resume(task, val1) is called, then resume(task, val2) is called before
    the task runs, what happens?
    (a) First resume wins, second raises E_INVARG?
    (b) Second resume overwrites first, task sees val2?
    (c) Task receives both values somehow?
    (d) Undefined behavior?
  impact: |
    Race condition behavior. Need to know how to handle:
    ```
    // Task A suspended
    resume(task_a, 1);
    resume(task_a, 2);  // What happens here?
    ```
  suggested_addition: |
    Add: "Once resume() successfully wakes a task, subsequent resume() calls
    on that task ID raise E_INVARG (task no longer suspended). If resume()
    is called while the task is transitioning from Suspended to Running,
    the behavior is undefined."

- id: GAP-TASK-013
  feature: "suspend during I/O"
  spec_file: "spec/tasks.md"
  spec_section: "5.2 I/O Suspension"
  gap_type: ask
  question: |
    Tasks "automatically suspend during" read(), network ops, exec().
    Can you call suspend() explicitly while already suspended on I/O?
    What happens? Does this nest, or raise an error?
  impact: |
    Need to know if suspend() is reentrant or blocked during I/O.
  suggested_addition: |
    Add: "Tasks cannot explicitly suspend while already suspended. Calling
    suspend() while the task is in Suspended state (including I/O suspension)
    raises E_INVARG."

### Task Killing

- id: GAP-TASK-014
  feature: "kill_task cleanup"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "3.3 kill_task"
  gap_type: guess
  question: |
    When kill_task() is called:
    (a) Does the task abort immediately mid-instruction?
    (b) Does it finish the current statement then abort?
    (c) Does it run finally blocks before aborting?
    (d) Does it clean up resources (close files, network connections)?
  impact: |
    Critical for resource management and exception safety.
    ```
    try
      f = open_file("data.txt", "w");
      suspend(60);  // Killed here
    finally
      close_file(f);  // Does this run?
    endtry
    ```
  suggested_addition: |
    Add: "When kill_task() is called, the target task aborts at the next
    safe point (typically the next instruction boundary). All finally blocks
    in the current stack are executed before the task terminates, allowing
    resource cleanup."

- id: GAP-TASK-015
  feature: "kill_task on self"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "3.3 kill_task"
  gap_type: test
  question: |
    The spec says "Killing current task aborts it" but doesn't specify:
    (a) Does kill_task(task_id()) return normally then abort?
    (b) Does it abort immediately without returning?
    (c) Do finally blocks run?
    (d) What does the caller see?
  impact: |
    Control flow behavior:
    ```
    try
      kill_task(task_id());
      notify(player, "Still alive?");  // Does this run?
    finally
      // Does this run?
    endtry
    ```
  suggested_addition: |
    Add: "kill_task() on the current task (task_id()) does not return normally.
    The task transitions to Aborted immediately after executing any pending
    finally blocks. Code after kill_task() does not execute."

- id: GAP-TASK-016
  feature: "kill_task on completed task"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "3.3 kill_task"
  gap_type: assume
  question: |
    What happens if you call kill_task() on a task that already completed?
    (a) E_INVARG (no such task)?
    (b) Silently succeed (no-op)?
    (c) E_INVARG (task exists but can't be killed)?
  impact: |
    Error handling for stale task IDs.
  suggested_addition: |
    Add: "kill_task() on a Completed or Aborted task raises E_INVARG."

- id: GAP-TASK-017
  feature: "kill_task permission check"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "3.3 kill_task"
  gap_type: ask
  question: |
    "Can kill own tasks" - does this mean:
    (a) Tasks owned by caller_perms()?
    (b) Tasks owned by player()?
    (c) Tasks created by the current task?
    (d) Tasks with the same owner as the current task?

    Also, if set_task_perms() was used, which identity matters?
  impact: |
    Permission model ambiguity. In a forked task where set_task_perms() was called:
    ```
    fork (0)
      set_task_perms(#wizard);
      kill_task(parent_task);  // Allowed?
    endfork
    ```
  suggested_addition: |
    Add: "A task can kill another task if caller_perms() matches the target
    task's owner (the player that initiated it), or if caller_perms() is a
    wizard. set_task_perms() affects this check."

### Tick System

- id: GAP-TASK-018
  feature: "tick costs"
  spec_file: "spec/tasks.md"
  spec_section: "4.2 Tick Costs"
  gap_type: guess
  question: |
    Table says "Most opcodes: 1 tick" but which opcodes DON'T cost 1 tick?
    List creation is 0, but what about:
    - List indexing: list[1]?
    - Map indexing: map[key]?
    - Arithmetic: a + b?
    - Comparison: a == b?
    - Property access: obj.prop?
    - Verb calls?
    - Builtin calls (marked "varies")?

    "Variable access: 0-1 ticks" - when is it 0, when is it 1?
  impact: |
    Cannot accurately implement tick counting without exhaustive cost table.
    Makes performance prediction impossible.
  suggested_addition: |
    Add comprehensive tick cost table for ALL operations, or clarify:
    "All operations cost 1 tick unless otherwise specified. Zero-cost operations:
    variable read, literal constants, list/map literals. Builtin calls cost
    varies by function (see builtin docs)."

- id: GAP-TASK-019
  feature: "tick limit enforcement"
  spec_file: "spec/tasks.md"
  spec_section: "4.3 Limits"
  gap_type: test
  question: |
    When ticks are exhausted:
    (a) Is the check done before or after executing the instruction?
    (b) Does the task abort immediately or finish the current statement?
    (c) Can the task catch this with try/except?
    (d) What error code is raised?
  impact: |
    Exception handling behavior:
    ```
    try
      while (1)  // Infinite loop
      endwhile
    except e (ANY)
      notify(player, "Caught: " + tostr(e));  // Can this catch tick exhaustion?
    endtry
    ```
  suggested_addition: |
    Add: "Tick limit is checked before each instruction. When exceeded, the task
    aborts with ABORT_TICKS. This cannot be caught by try/except - the task
    terminates immediately after running finally blocks."

- id: GAP-TASK-020
  feature: "seconds limit enforcement"
  spec_file: "spec/tasks.md"
  spec_section: "4.3 Limits"
  gap_type: guess
  question: |
    How frequently is the time limit checked?
    (a) Every instruction (expensive)?
    (b) Every N ticks?
    (c) At suspension points only?
    (d) Implementation-defined?

    Can a task exceed its time limit if it doesn't suspend?
  impact: |
    Real-world tasks might run slightly over or way over the limit depending
    on check frequency. Performance implications for frequent time checks.
  suggested_addition: |
    Add: "Time limit is checked at the same frequency as tick limit (before
    each instruction). Tasks abort with ABORT_SECONDS as soon as the deadline
    passes."

- id: GAP-TASK-021
  feature: "ticks_left precision"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "4.1 ticks_left"
  gap_type: test
  question: |
    Does ticks_left() return:
    (a) Ticks remaining before the NEXT instruction?
    (b) Ticks remaining after executing ticks_left() itself?
    (c) Ticks remaining at the start of the current instruction?

    Does calling ticks_left() consume a tick?
  impact: |
    Precision matters for yin() and tight loops:
    ```
    while (ticks_left() > 100)  // Does this call cost a tick?
      // ...
    endwhile
    ```
  suggested_addition: |
    Add: "ticks_left() returns the count AFTER executing ticks_left() itself
    (which costs 1 tick). The returned value is the number of ticks remaining
    before the next instruction after ticks_left() returns."

- id: GAP-TASK-022
  feature: "yin threshold semantics"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "4.3 yin"
  gap_type: ask
  question: |
    yin(1000) "yields if ticks_left() < threshold".
    After yielding and resuming, how many ticks does the task have?
    (a) Full limit refreshed (30,000/60,000)?
    (b) Full limit minus ticks consumed so far?
    (c) Just enough to continue (threshold amount)?
    (d) Implementation-defined?

    Can a task do infinite work by calling yin() in a loop?
  impact: |
    Determines if yin() enables unbounded computation or just smooths execution:
    ```
    for i in [1..1000000]
      yin(1000);  // Can this loop run forever?
      expensive_work();
    endfor
    ```
  suggested_addition: |
    Add: "When yin() suspends, the task is rescheduled with its tick count
    fully refreshed to the original limit. This allows long-running tasks to
    make progress. The time limit is NOT refreshed - it continues counting
    from the original start time."

- id: GAP-TASK-023
  feature: "yin vs suspend"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "4.3 yin"
  gap_type: guess
  question: |
    yin() "suspends and resumes later" but how much later?
    (a) Next scheduler tick (essentially immediate)?
    (b) Delay proportional to ticks used?
    (c) End of queue (all other waiting tasks run first)?
    (d) Implementation-defined?

    Is yin(1000) equivalent to `if (ticks_left() < 1000) suspend(0); endif`?
  impact: |
    Fairness and scheduling behavior. Determines if yin() is cooperative or
    could starve other tasks.
  suggested_addition: |
    Add: "yin() that triggers suspension reschedules the task to run immediately
    after the current scheduler cycle completes. This is equivalent to
    suspend(0) but only executes if the tick threshold is reached."

### Task Inspection

- id: GAP-TASK-024
  feature: "queued_tasks visibility"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "2.3 queued_tasks"
  gap_type: ask
  question: |
    Does queued_tasks() return:
    (a) All tasks in Waiting state (fork delayed)?
    (b) All tasks in Suspended state (suspended)?
    (c) Both Waiting and Suspended?
    (d) All non-running tasks?
    (e) All tasks period, including Running?

    Can a non-wizard see other players' tasks?
  impact: |
    Determines API semantics and permission model:
    ```
    tasks = queued_tasks();  // What do I see?
    ```
  suggested_addition: |
    Add: "queued_tasks() returns all tasks in Waiting or Suspended states
    (not Running, Completed, or Aborted). Non-wizards see only their own tasks.
    Wizards see all tasks."

- id: GAP-TASK-025
  feature: "queued_tasks return format"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "2.3 queued_tasks"
  gap_type: guess
  question: |
    Return value documented as:
    `{task_id, start_time, x, y, programmer, verb_loc, verb_name, line, this, [vars]}`

    What are `x` and `y`? The spec doesn't define them.
    Are they placeholders? Legacy fields? Coordinates?
  impact: |
    Cannot implement without knowing what these fields contain.
  suggested_addition: |
    Add: "Fields x and y are reserved for future use and currently always 0."
    OR define their actual purpose.

- id: GAP-TASK-026
  feature: "queued_tasks variables"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "2.3 queued_tasks"
  gap_type: ask
  question: |
    When include_vars is true, what is included?
    (a) All local variables in the task's current frame?
    (b) All variables across all frames (entire stack)?
    (c) Only task-local storage (set_task_local)?
    (d) Some combination?

    What format are variables returned in?
  impact: |
    Security and privacy concerns. Can one task inspect another's variables?
    ```
    tasks = queued_tasks(1);
    // Can I see other_task's local variables?
    ```
  suggested_addition: |
    Add: "When include_vars is true, the final element of each task entry is
    a map of {variable_name: value} for all local variables in the task's
    top stack frame. Wizards can see all tasks' variables. Non-wizards can
    only see their own tasks."

- id: GAP-TASK-027
  feature: "callers vs task_stack"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "2.2 task_stack"
  gap_type: ask
  question: |
    task_stack() is described as "more detail than callers()" but exactly what
    additional detail? The spec doesn't show the return format for task_stack().

    Also, task_stack() takes optional task_id - can you inspect other tasks'
    stacks, or only your own?
  impact: |
    Cannot implement without knowing return format and permission model.
  suggested_addition: |
    Add complete documentation for task_stack() including:
    - Full return value format
    - Permission model (can you inspect other tasks?)
    - What "more detail" means compared to callers()

### Task-Local Storage

- id: GAP-TASK-028
  feature: "task_local missing key"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "6.2 task_local"
  gap_type: assume
  question: |
    Example shows `task_local("missing") => 0` but is this:
    (a) Always 0 for missing keys?
    (b) Could be any default value?
    (c) Could raise an error?
    (d) Undefined?
  impact: |
    API contract. Need to know if you must check return value:
    ```
    val = task_local("maybe_set");
    if (val == 0)
      // Missing key, or actually set to 0?
    endif
    ```
  suggested_addition: |
    Add: "task_local() returns 0 for keys that were never set via
    set_task_local(). There is no way to distinguish an unset key from
    a key explicitly set to 0."

- id: GAP-TASK-029
  feature: "task_local key types"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "6.1 set_task_local"
  gap_type: ask
  question: |
    "Keys are arbitrary values" - does this mean:
    (a) Any MOO value (int, str, obj, list, etc.)?
    (b) Only strings?
    (c) Only hashable values?
    (d) Some restricted set?

    Can you use an object or list as a key?
  impact: |
    Storage implementation (hashmap vs. other structure):
    ```
    set_task_local({1, 2, 3}, "value");  // Valid?
    set_task_local(#123, "value");       // Valid?
    ```
  suggested_addition: |
    Add: "Keys can be any MOO value. Lists and maps are compared by value,
    not reference, for key lookup."

- id: GAP-TASK-030
  feature: "task_local inheritance"
  spec_file: "spec/tasks.md"
  spec_section: "9.3 Semantics"
  gap_type: test
  question: |
    "Not inherited by forked tasks" - does this mean:
    (a) Forked tasks start with empty task-local storage?
    (b) Forked tasks get a copy of parent's task-local storage?
    (c) Forked tasks share reference to parent's storage (until COW)?

    Spec says "not inherited" which suggests (a), but example would clarify.
  impact: |
    Behavior in forked tasks:
    ```
    set_task_local("key", "parent_value");
    fork (0)
      val = task_local("key");  // What is val?
    endfork
    ```
  suggested_addition: |
    Add: "Forked tasks begin with empty task-local storage. They do not inherit
    the parent's task-local values. Use regular variables if you need to pass
    data to forked tasks."

### Task Context

- id: GAP-TASK-031
  feature: "context variable 'verb'"
  spec_file: "spec/tasks.md"
  spec_section: "10.1 Context Variables"
  gap_type: test
  question: |
    Table lists `verb` as a context variable containing "Verb name".
    But in fork'd tasks or after multiple verb calls, which verb name?
    (a) Original verb that started the task?
    (b) Currently executing verb?
    (c) Most recently called verb?
  impact: |
    Which value `verb` holds in nested calls:
    ```
    // In #obj:foo
    #other:bar();
    notify(player, verb);  // "foo" or "bar"?
    ```
  suggested_addition: |
    Add: "The `verb` variable contains the name of the currently executing verb.
    It changes with each verb call and is restored when the verb returns."

- id: GAP-TASK-032
  feature: "set_task_perms scope"
  spec_file: "spec/tasks.md"
  spec_section: "10.2 Permission Context"
  gap_type: ask
  question: |
    When set_task_perms(obj) is called:
    (a) Does it affect only the current verb?
    (b) Does it affect all subsequent verbs called from this task?
    (c) Does it affect the entire task permanently?
    (d) Is it scoped to try/finally blocks?

    When a verb returns, is the permission context restored?
  impact: |
    Scope and restoration behavior:
    ```
    orig = caller_perms();
    set_task_perms(#wizard);
    do_privileged_thing();
    // Is caller_perms() back to orig, or still #wizard?
    ```
  suggested_addition: |
    Add: "set_task_perms() changes the permission context for the entire task
    permanently. It is not automatically restored when verbs return. To restore,
    explicitly call set_task_perms(original_perms) again."

### Scheduling

- id: GAP-TASK-033
  feature: "same-time task ordering"
  spec_file: "spec/tasks.md"
  spec_section: "11.1 Task Queue"
  gap_type: test
  question: |
    "FIFO for same-time tasks" - if multiple forks have the same delay:
    ```
    fork (5) notify(player, "A"); endfork
    fork (5) notify(player, "B"); endfork
    fork (5) notify(player, "C"); endfork
    ```
    Does this guarantee output order A, B, C?

    What if they're created from different parent tasks that happen to schedule
    at the same time?
  impact: |
    Determinism and debugging. Need to know if task order is predictable.
  suggested_addition: |
    Add: "Tasks scheduled for the same start time execute in the order they
    were created (FIFO). This ordering is global across all tasks, not per-parent."

- id: GAP-TASK-034
  feature: "task queue persistence"
  spec_file: "spec/tasks.md"
  spec_section: "11.1 Task Queue"
  gap_type: ask
  question: |
    Are queued tasks persisted across server shutdown?
    If the server shuts down with tasks in Waiting or Suspended state:
    (a) Tasks are lost?
    (b) Tasks are saved and resume when server restarts?
    (c) Tasks abort with specific error?
    (d) Configurable behavior?
  impact: |
    Critical for long-running or delayed tasks. Affects reliability:
    ```
    fork (86400)  // Run in 24 hours
      send_reminder();
    endfork
    // If server restarts in 12 hours, does this task survive?
    ```
  suggested_addition: |
    Add: "Task queue state is not persisted. When the server shuts down, all
    queued tasks are lost. Only the object database is persisted. For durable
    scheduling, use object properties to track pending work and recreate tasks
    on server startup."

### Error Handling

- id: GAP-TASK-035
  feature: "abort reasons exposure"
  spec_file: "spec/tasks.md"
  spec_section: "7.2 Abort Reasons"
  gap_type: ask
  question: |
    The table lists abort reasons (ABORT_TICKS, ABORT_SECONDS, etc.) but:
    (a) Are these accessible as constants/values in MOO code?
    (b) Are they only internal implementation details?
    (c) How does queued_tasks() or logs indicate abort reason?
    (d) Can verbs query why a task aborted?
  impact: |
    Observability and debugging. Need to distinguish timeout from kill:
    ```
    // How do I know if task X was killed or timed out?
    ```
  suggested_addition: |
    Add: "Abort reasons are internal implementation details not directly exposed
    to MOO code. Task abortion is logged by the server. There is no builtin to
    query a task's abort reason."

- id: GAP-TASK-036
  feature: "error in fork error propagation"
  spec_file: "spec/tasks.md"
  spec_section: "12.2 Error in Fork"
  gap_type: test
  question: |
    "Background task errors don't affect parent" - but:
    (a) Is the error logged?
    (b) Can the parent detect that the forked task failed?
    (c) Is there a completion callback or notification?
    (d) Does the task stay in some "Aborted" state that's queryable?
  impact: |
    Error visibility and monitoring:
    ```
    fork tid (0)
      some_operation();  // Raises E_INVARG
    endfork
    // How do I know if this failed?
    ```
  suggested_addition: |
    Add: "When a forked task aborts due to error, the error is logged to the
    server log. The parent task is not notified. To detect forked task errors,
    use task_local() or explicit notification within the forked task's
    try/except blocks."

### Go Implementation Questions

- id: GAP-TASK-037
  feature: "goroutine per task"
  spec_file: "spec/tasks.md"
  spec_section: "13.2 Goroutine Mapping"
  gap_type: guess
  question: |
    The Go example shows "Fork creates new goroutine" but this is just an example.
    Does the spec REQUIRE one goroutine per task, or is this implementation detail?

    Could an implementation use:
    (a) One goroutine per task (as shown)?
    (b) A pool of goroutines running tasks?
    (c) Single-threaded event loop (like LambdaMOO)?
    (d) Any approach that meets behavioral requirements?
  impact: |
    Implementation flexibility vs. specification constraint. If the spec requires
    goroutine-per-task, that's a strong Go-specific requirement. If it's just
    example, implementations have more freedom.
  suggested_addition: |
    Add: "The Go implementation sections are informative examples, not normative
    requirements. Implementations may use any concurrency model that provides
    the specified task semantics."

- id: GAP-TASK-038
  feature: "suspension channel blocking"
  spec_file: "spec/tasks.md"
  spec_section: "13.3 Suspension with Channels"
  gap_type: ask
  question: |
    The example uses unbuffered channels for WakeChannel. What if:
    (a) resume() is called before suspend() blocks on the channel?
    (b) Multiple resumes() happen in quick succession?
    (c) The channel is never read from (task killed while suspended)?

    Does this require buffered channels? What size?
  impact: |
    Correct channel sizing and avoiding deadlocks. The example might have a race:
    ```
    // Task A calls suspend()
    // Task B calls resume() before A blocks on chan
    // Does the value get lost?
    ```
  suggested_addition: |
    Add: "WakeChannel should be buffered with size 1 to handle resume() calls
    that arrive before suspend() blocks. Additional resume() attempts should
    be rejected with E_INVARG."

### Cross-Feature Interactions

- id: GAP-TASK-039
  feature: "task limits vs try/finally"
  spec_file: "spec/tasks.md"
  spec_section: "4.3 Limits"
  gap_type: test
  question: |
    When a task exceeds tick or time limits:
    - Do finally blocks run before abort?
    - Do finally blocks run WITHIN the tick/time limits?
    - If a finally block itself exceeds limits, what happens?
  impact: |
    Resource cleanup guarantee:
    ```
    try
      infinite_loop();
    finally
      cleanup_resources();  // Does this run when ticks exhausted?
      another_infinite_loop();  // Does this get aborted?
    endtry
    ```
  suggested_addition: |
    Add: "When task limits are exceeded, all finally blocks in the stack are
    executed before abort. Finally blocks execute with a small additional tick
    allowance (e.g., 1000 ticks) to allow cleanup. If a finally block exceeds
    this allowance, it is aborted immediately without running remaining finally
    blocks."

- id: GAP-TASK-040
  feature: "suspend vs break/continue/return"
  spec_file: "spec/tasks.md"
  spec_section: "5.1 Explicit Suspension"
  gap_type: test
  question: |
    Can suspend() be used in loops? What happens with:
    ```
    while (1)
      result = suspend(5);
      if (result == "stop")
        break;  // Does this work after resuming from suspend?
      endif
    endwhile
    ```

    Does suspend() affect control flow statements?
  impact: |
    Interaction between suspension and structured control flow.
  suggested_addition: |
    Add: "suspend() can be used anywhere, including inside loops and conditional
    blocks. When the task resumes, execution continues from the point of
    suspension. Control flow statements (break, continue, return) work normally
    after suspension."

- id: GAP-TASK-041
  feature: "fork inside try/finally"
  spec_file: "spec/tasks.md"
  spec_section: "3.2 Fork Statement"
  gap_type: guess
  question: |
    What happens if fork is used inside a try or finally block?
    ```
    try
      fork (0)
        risky_thing();
      endfork
    except e (ANY)
      // Does this catch errors from the fork?
    endtry
    ```

    Does the forked task inherit the exception handler context?
  impact: |
    Exception handling scope and task isolation.
  suggested_addition: |
    Add: "Forked tasks do not inherit the parent's exception handler stack.
    Errors in forked tasks are handled independently. A try/except in the parent
    does not catch errors from forked tasks."

- id: GAP-TASK-042
  feature: "queued_tasks during iteration"
  spec_file: "spec/builtins/tasks.md"
  spec_section: "2.3 queued_tasks"
  gap_type: test
  question: |
    If you iterate over queued_tasks() and modify tasks during iteration:
    ```
    for task in (queued_tasks())
      kill_task(task[1]);  // Kill tasks while iterating
    endfor
    ```

    Is the list snapshot at the time of call, or live? Can iteration be invalidated?
  impact: |
    Safe iteration patterns.
  suggested_addition: |
    Add: "queued_tasks() returns a snapshot of the task queue at the time of
    the call. Modifications to tasks (killing, resuming) during iteration do
    not affect the returned list."

---

## Summary Statistics

- Total gaps identified: 42
- Gap types:
  - guess: 18 (implementor must guess behavior)
  - ask: 13 (implementor must ask someone)
  - test: 10 (implementor must test existing system)
  - assume: 1 (implementor must make assumption)

## Critical Gaps (Highest Impact)

1. **GAP-TASK-004**: Fork delay=0 semantics - affects execution order reasoning
2. **GAP-TASK-005**: Fork environment copy semantics - affects correctness of mutation
3. **GAP-TASK-008**: Suspend timeout vs explicit resume distinction - protocol ambiguity
4. **GAP-TASK-014**: Kill task cleanup and finally blocks - critical for resource safety
5. **GAP-TASK-018**: Tick cost table completeness - cannot implement without full costs
6. **GAP-TASK-022**: yin() tick refresh semantics - determines if unbounded work is possible
7. **GAP-TASK-034**: Task queue persistence across shutdown - reliability concern
8. **GAP-TASK-039**: Task limits vs finally blocks - resource cleanup guarantee

## Recommendations

### Immediate Priority
- Define complete tick cost table (GAP-TASK-018)
- Clarify fork delay=0 and environment copy (GAP-TASK-004, GAP-TASK-005)
- Specify task cleanup and finally block guarantees (GAP-TASK-014, GAP-TASK-039)

### High Priority
- Document suspend/resume value protocol precisely (GAP-TASK-008)
- Specify yin() semantics completely (GAP-TASK-022, GAP-TASK-023)
- Clarify task state transitions and terminal states (GAP-TASK-001, GAP-TASK-002)
- Define queued_tasks() fields x, y (GAP-TASK-025)

### Medium Priority
- Specify all edge cases (suspend(0), suspend(-5), multiple resume(), etc.)
- Document permission models for all operations
- Clarify Go implementation examples vs requirements (GAP-TASK-037)

### Process Improvement
The specification would benefit from:
1. Exhaustive tables (tick costs, state transitions, error conditions)
2. Edge case examples for every operation
3. Clear distinction between "must" requirements and "may" suggestions
4. Complete function signatures with all parameters and return formats
5. Explicit statement of what's undefined vs implementation-defined
