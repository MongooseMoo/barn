# Task System Gaps - Research & Spec Patches

**Date:** 2025-12-24
**Gaps Identified:** 42
**Gaps Resolved:** 6 (partial research - sufficient for initial spec patches)
**Status:** PARTIAL - Foundational research complete, detailed analysis needed for remaining gaps

## Research Summary

Conducted systematic investigation of ToastStunt C++ implementation (primary reference) to resolve critical task system specification gaps. Note: moo_interp (Python) currently lacks full task support, so ToastStunt serves as authoritative source.

### Key Source Files Examined
- `~/src/toaststunt/src/tasks.cc` (3058 lines) - Task queue management, fork, suspend/resume
- `~/src/toaststunt/src/execute.cc` - VM execution, fork opcode handling
- `~/src/toaststunt/src/eval_env.cc` - Runtime environment copy implementation
- `~/src/toaststunt/src/functions.cc` - Task builtin function implementations

---

## RESOLVED GAPS

### ✅ GAP-TASK-005: Fork environment copy semantics

**Question:** Deep copy, shallow copy, or copy-on-write?

**Research:**
- **File:** `eval_env.cc:65-73`
- **Function:** `copy_rt_env(Var * from, unsigned size)`
- **Implementation:**
```cpp
Var *copy_rt_env(Var * from, unsigned size) {
    unsigned i;
    Var *ret = new_rt_env(size);
    for (i = 0; i < size; i++)
        ret[i] = var_ref(from[i]);  // Reference counted copy
    return ret;
}
```

**Finding:** Shallow copy with reference counting (COW semantics). MOO values use reference counting - primitives (int, float) are copied by value, objects/lists/strings are shallow copied with ref count increment. Mutations trigger copy-on-write.

**Resolution:** Fork uses shallow copy with MOO's standard reference-counted copy-on-write semantics.

**Spec Patch Required:** YES - Add to spec/tasks.md section 3.2

---

### ✅ GAP-TASK-006: Fork task_id binding timing

**Question:** When is task_id variable bound - immediately or asynchronously?

**Research:**
- **File:** `tasks.cc:1284-1292`
- **Function:** `enqueue_forked_task2`
- **Implementation:**
```cpp
id = new_task_id();
// ... ref counting ...
if (vid >= 0) {
    free_var(a.rt_env[vid]);
    a.rt_env[vid].type = TYPE_INT;
    a.rt_env[vid].v.num = id;  // Bound BEFORE enqueue
}
rt_env = copy_rt_env(a.rt_env, a.prog->num_var_names);
enqueue_forked(a.prog, a, rt_env, f_index, when, id);
```

**Finding:** Task ID is generated and bound to the variable IMMEDIATELY, before the fork statement completes. The parent can use the ID right after the fork statement.

**Resolution:** task_id variable is bound synchronously when fork statement executes, before parent continues.

**Spec Patch Required:** YES - Clarify in spec/tasks.md section 3.2

---

### ✅ GAP-TASK-004: Fork delay=0 semantics

**Question:** Does delay=0 run immediately or schedule for later?

**Research:**
- **File:** `tasks.cc:1290-1291`
- **Function:** `enqueue_forked_task2`
```cpp
when = double_to_start_tv(after_seconds);  // Converts to absolute time
enqueue_forked(a.prog, a, rt_env, f_index, when, id);  // Adds to waiting queue
```
- Forked tasks go to `waiting_tasks` queue, not immediate execution
- Parent continues immediately after fork statement
- Scheduler processes waiting queue in FIFO order

**Finding:** Even with delay=0, the forked task is queued and runs on the next scheduler cycle, NOT before the parent continues. Parent resumes immediately after the fork statement completes.

**Resolution:** fork (0) schedules task to run "as soon as possible" meaning next scheduler cycle, but parent continues first.

**Spec Patch Required:** YES - Add explicit note in spec/tasks.md section 3.2

---

### ✅ GAP-TASK-008: Suspend return value - timeout vs resume(task, 0)

**Question:** How to distinguish suspend() timeout from resume(task, 0)?

**Research:**
- **File:** `tasks.cc:1296-1321` (enqueue_suspended_task)
```cpp
t->t.suspended.value = zero;  // Default value is integer 0
```
- **File:** `tasks.cc:1324-1338` (resume_task)
```cpp
t->t.suspended.value = value;  // Resume sets arbitrary value
```
- When timeout occurs, suspended task resumes with the `zero` value (integer 0)
- When resume() is called, it can pass any value including integer 0

**Finding:** **THERE IS NO WAY TO DISTINGUISH TIMEOUT FROM resume(task, 0)**. Both return integer 0. This is a protocol ambiguity in the original design.

**Resolution:** Document that timeout returns 0, and there's no distinction from resume(task, 0). Recommend using non-zero resume values if distinction needed.

**Spec Patch Required:** YES - Document limitation in spec/builtins/tasks.md section 3.1

---

### ✅ GAP-TASK-034: Task queue persistence

**Question:** Are queued tasks saved across server shutdown?

**Research:**
- **File:** `tasks.cc` - Functions `write_task_queue()` and `read_task_queue()` exist
- **File:** `tasks.h:111-112` - Exported functions for persistence
- ToastStunt DOES persist task queue to database

**Finding:** Task queue is persisted. Functions `write_task_queue()` and `read_task_queue()` handle serialization/deserialization of waiting and suspended tasks across server restarts.

**Resolution:** Task queue state IS persisted in ToastStunt (and likely LambdaMOO). Tasks survive server restart.

**Spec Patch Required:** YES - Clarify in spec/tasks.md section 11.1 (or mark as implementation-defined)

---

### ✅ GAP-TASK-033: Same-time task ordering

**Question:** Are same-delay tasks guaranteed FIFO order?

**Research:**
- **File:** `tasks.cc:1207-1244` (enqueue_forked and enqueue_waiting)
- Tasks added to end of `waiting_tasks` linked list
- Scheduler processes in order from head of list
- No reordering based on priority (usage-based scheduling only applies to tqueues for interactive tasks)

**Finding:** Tasks are processed strictly FIFO from the waiting queue. Same-delay tasks execute in creation order.

**Resolution:** FIFO ordering guaranteed for same-delay tasks.

**Spec Patch Required:** YES - Confirm in spec/tasks.md section 11.1

---

## NEEDS FURTHER RESEARCH (36 gaps remaining)

The following gaps require deeper code analysis or are deferred:

### High Priority (Needs Code Trace)
- **GAP-TASK-014:** kill_task cleanup and finally blocks (need to trace VM abort path)
- **GAP-TASK-018:** Complete tick cost table (need comprehensive execute.cc opcode review)
- **GAP-TASK-022:** yin() tick refresh (need to find bf_yin implementation)
- **GAP-TASK-039:** Task limits vs finally blocks (need abort path analysis)

### Task State Model (8 gaps)
- GAP-TASK-001, 002, 003: State transitions and terminal states
- Requires: Trace all state transition points in task lifecycle

### Suspend/Resume Edge Cases (6 gaps)
- GAP-TASK-009, 010, 011, 012, 013: suspend(0), negative duration, multiple resume, reentrancy
- Requires: Read bf_suspend and bf_resume implementations in detail

### Task Killing (4 gaps)
- GAP-TASK-015, 016, 017: kill on self, completed task, permission model
- Requires: Read bf_kill_task implementation

### Tick System (6 gaps)
- GAP-TASK-019, 020, 021, 023: Limit enforcement, ticks_left precision, yin vs suspend
- Requires: Tick counting and limit check code in VM loop

### Task Inspection (4 gaps)
- GAP-TASK-024, 025, 026, 027: queued_tasks visibility, return format, variables, task_stack
- Requires: Read bf_queued_tasks implementation

### Task-Local Storage (3 gaps)
- GAP-TASK-028, 029, 030: Missing key behavior, key types, inheritance
- Requires: Find task_local implementation

### Context & Scheduling (4 gaps)
- GAP-TASK-031, 032, 036, 037, 038: Context variables, set_task_perms, abort reasons, Go examples
- Various sources needed

### Cross-Feature (3 gaps)
- GAP-TASK-040, 041, 042: suspend+control flow, fork+exceptions, queued_tasks iteration
- Complex interaction testing needed

---

## Summary for Q

**Completed:** Initial research on 6 critical gaps from ToastStunt source code. Found authoritative answers for:
1. Fork environment is shallow-copy with reference counting (COW)
2. Fork task_id is bound synchronously before parent continues
3. Fork delay=0 schedules for next cycle (parent runs first)
4. Suspend timeout cannot be distinguished from resume(task, 0) - protocol limitation
5. Task queue IS persisted across server shutdown
6. Same-delay tasks are strictly FIFO

**Spec Files Needing Patches:**
- `spec/tasks.md` - Sections 3.2 (fork), 11.1 (scheduling)
- `spec/builtins/tasks.md` - Section 3.1 (suspend)

**Remaining Work:** 36 gaps require deeper code tracing (bf_* function implementations, VM execution loop, tick counting, state transitions). Estimated 4-6 more hours of systematic code reading needed for complete resolution.

**Recommendation:** Should I:
(a) Continue researching all 42 gaps comprehensively, OR
(b) Apply the 6 resolved patches now and defer remaining gaps to future iterations?

