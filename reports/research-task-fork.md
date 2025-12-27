# Research Report: MOO Task and Fork Implementation

**Date:** 2025-12-26
**Author:** Research Agent
**Purpose:** Study fork/task semantics from cow_py and ToastStunt for barn implementation

---

## Executive Summary

Fork is a fundamental MOO primitive that creates **asynchronous child tasks** from a running parent task. The child task inherits execution context but runs independently after a delay. This report documents the complete fork/task model from both Python (cow_py/moo_interp) and C++ (ToastStunt) implementations.

**Key Finding:** Fork is NOT a function call - it's a **bytecode-level control flow operation** that suspends the parent, creates a child task in the scheduler, stores the child's task ID (if requested), then continues parent execution. The child runs later as a separate scheduled task.

---

## 1. Fork Statement Semantics

### 1.1 Basic Syntax

```moo
fork (delay)
    // body
endfork

fork task_id (delay)
    // body
endfork
```

### 1.2 Behavior

**When fork executes:**
1. Parent evaluates `delay` expression → number of seconds
2. VM emits `OUTCOME_FORKED` and saves fork information
3. TaskRunner creates child task with copied environment
4. Child task is queued with `start_time = now + delay`
5. Parent continues immediately after fork statement
6. If `task_id` given, parent's variable gets child's task ID

**Key Properties:**
- Parent and child run **independently** (no blocking)
- Parent receives child's task ID (if using named fork)
- Child receives `0` as its task ID (special return value)
- Delay can be `0` (executes "soon" but not immediately)
- Fork body has **access to parent's variable environment** (copied at fork time)

### 1.3 Context Inheritance

Child task inherits from parent at fork time:
- All local variables (copied, not shared)
- `this` - the object context
- `player` - the player who initiated the task
- `caller` - the calling object
- Tick budget (shared limit concept, not exhausted parent budget)
- Permissions (`programmer`)

Child does NOT inherit:
- Parent's task ID
- Parent's stack state
- Parent's loop iterators

---

## 2. Task Model

### 2.1 What is a Task?

A task is the **unit of MOO code execution**. It represents:
- A piece of code to execute
- An execution context (player, this, variables)
- Resource limits (ticks, time)
- Scheduling information (when to run, priority)

**Task Types:**
1. **Input Task** - Created from user command
2. **Forked Task** - Created by `fork` statement
3. **Suspended Task** - Paused by `suspend()` builtin

### 2.2 Task States

**cow_py States:**
```python
class TaskState(Enum):
    PENDING = auto()       # Waiting to start
    RUNNING = auto()       # Currently executing
    SUSPENDED = auto()     # Paused (suspend() called)
    WAITING_INPUT = auto() # Blocked on read()
    DONE = auto()          # Completed successfully
    ABORTED = auto()       # Error or killed
```

**ToastStunt States:**
```c
typedef enum {
    TASK_INBAND,    // Input tasks (user commands)
    TASK_OOB,       // Out-of-band
    TASK_QUOTED,    // Quoted input
    TASK_BINARY,    // Binary mode
    TASK_FORKED,    // Background fork
    TASK_SUSPENDED, // Suspended by suspend()
} task_kind;
```

### 2.3 Task Scheduling

**Scheduler Responsibilities:**
1. Assign unique task IDs
2. Track tasks by state (waiting, running, suspended)
3. Execute ready tasks when `start_time` <= now
4. Enforce tick and time limits
5. Handle task suspension/resumption
6. Kill tasks on request or timeout

**Priority Queue:**
- Tasks ordered by `start_time` (earliest first)
- Fork delay determines `start_time = now + delay`
- Suspended tasks have `resume_time = now + suspend_seconds`

### 2.4 Tick and Second Limits

**Ticks:**
- Each VM operation consumes ticks
- Foreground tasks: ~60,000 ticks (default)
- Background (forked) tasks: ~30,000 ticks
- Exceeding limit → `E_MAXREC` error, task aborted

**Seconds:**
- Wall-clock time limit
- Foreground: ~5 seconds
- Background: ~3 seconds
- Exceeding limit → task killed by scheduler

**Key Design:** Ticks prevent infinite loops; seconds prevent slow operations from hogging CPU.

---

## 3. Implementation Details

### 3.1 cow_py/moo_interp Implementation

#### Fork Bytecode Compilation (moo_ast.py)

```python
class _ForkStatement(_Statement):
    delay: _Expression
    body: _Body
    var_id: str = None  # Optional task ID variable

    def to_bytecode(self, state: CompilerState, program: Program):
        # 1. Compile delay expression
        delay_bc = self.delay.to_bytecode(state, program)

        # 2. Compile fork body separately
        body_bc = self.body.to_bytecode(state, program)

        # 3. Store body in program's fork_vectors
        f_index = len(program.fork_vectors)
        program.fork_vectors.append(body_bc)

        # 4. Create fork instruction
        if self.var_id:
            # Named fork: OP_FORK_WITH_ID (f_index, var_index)
            fork_instr = Instruction(
                opcode=Opcode.OP_FORK_WITH_ID,
                operand=(f_index, var_index)
            )
        else:
            # Anonymous fork: OP_FORK (f_index)
            fork_instr = Instruction(
                opcode=Opcode.OP_FORK,
                operand=f_index
            )

        return delay_bc + [fork_instr]
```

**Key Insight:** Fork body is compiled into a separate "fork vector" stored in the program. The fork instruction references this vector by index.

#### Fork Execution (vm.py)

```python
@operator(Opcode.OP_FORK)
def exec_fork(self, f_index: int):
    delay = self.pop()  # Get delay from stack
    frame = self.current_frame

    # Store fork info for TaskRunner
    self.fork_info = {
        'f_index': f_index,
        'delay': float(delay),
        'fork_vector': frame.prog.fork_vectors[f_index],
        'rt_env': list(frame.rt_env),  # Copy environment
        'var_names': frame.prog.var_names,
        'this': frame.this,
        'player': frame.player,
    }
    self.state = VMOutcome.OUTCOME_FORKED
    return None

@operator(Opcode.OP_FORK_WITH_ID)
def exec_fork_with_id(self, operand: tuple):
    f_index, var_index = operand
    delay = self.pop()

    self.fork_info = {
        # ... same as OP_FORK ...
        'var_index': var_index,  # Store child ID here
    }
    self.state = VMOutcome.OUTCOME_FORKED
    return None
```

**Critical Detail:** VM doesn't create the child task itself - it sets `OUTCOME_FORKED` state and returns control to TaskRunner.

#### Task Creation (scheduler.py)

```python
def fork_from_info(self, parent_task: Task, fork_info: dict) -> int:
    delay = fork_info.get('delay', 0)

    # Create child context (deep copy parent context)
    child_ctx = deepcopy(parent_task.context)
    child_ctx.this = fork_info.get('this', parent_task.context.this)
    child_ctx.player = fork_info.get('player', parent_task.context.player)

    child = Task(
        id=0,  # Will be assigned by scheduler
        state=TaskState.PENDING,
        kind=TaskKind.FORKED,
        context=child_ctx,
        ticks_remaining=parent_task.ticks_remaining,
        fork_info=fork_info,  # Store for execution
    )

    if delay > 0:
        child.resume_time = time.time() + delay

    return self.submit(child)
```

#### Parent Continuation (task_runner.py)

```python
def continue_after_fork(self, task: Task, child_task_id: int) -> TaskOutcome:
    vm = task.vm_state  # Saved VM from OUTCOME_FORKED

    # If named fork, store child ID in parent's variable
    fork_info = task.fork_info
    if fork_info and 'var_index' in fork_info:
        var_index = fork_info['var_index']
        # Adjust index for context vars prepended by execute_code
        adjusted_index = var_index + num_context_vars
        vm.current_frame.rt_env[adjusted_index] = child_task_id

    # Clear fork state and continue parent
    vm.state = None
    vm.fork_info = None
    task.vm_state = vm

    return self.run(task)
```

#### Child Execution (task_runner.py)

```python
def _create_forked_vm(self, task: Task) -> VM:
    fork_info = task.fork_info

    # Create program with fork vector as bytecode
    prog = Program(
        first_lineno=1,
        literals=[],
        fork_vectors=[],
        var_names=list(fork_info['var_names']),
    )

    # Create frame with fork_vector as bytecode
    frame = StackFrame(
        func_id=0,
        prog=prog,
        ip=0,
        stack=list(fork_info['fork_vector']),  # Fork body
        rt_env=list(fork_info['rt_env']),      # Copied env
        this=fork_info.get('this', 0),
        player=fork_info.get('player', 0),
        verb="<forked>",
    )

    vm = VM(db=self.db, bi_funcs=self.bi_funcs)
    vm.call_stack = [frame]
    return vm
```

**Pattern:**
1. VM hits fork → returns `OUTCOME_FORKED`
2. TaskRunner creates child task, gets child ID
3. TaskRunner stores child ID in parent's variable (if named fork)
4. TaskRunner resumes parent execution
5. Scheduler runs child when `resume_time` passes

### 3.2 ToastStunt Implementation

#### Data Structures (tasks.cc)

```c
typedef struct forked_task {
    int id;                  // Child task ID
    Program *program;        // Compiled program
    activation a;            // Activation frame (context)
    Var *rt_env;            // Runtime environment (variables)
    int f_index;            // Index into fork_vectors
    struct timeval start_tv; // When to start
} forked_task;

typedef struct task {
    struct task *next;
    task_kind kind;  // TASK_FORKED, TASK_SUSPENDED, etc.
    union {
        input_task input;
        forked_task forked;
        suspended_task suspended;
    } t;
} task;
```

#### Fork Opcode (execute.cc)

```c
case OP_FORK:
case OP_FORK_WITH_ID:
{
    Var time;
    unsigned id = 0, f_index;
    double when;

    time = POP();  // Get delay from stack
    f_index = READ_BYTES(bv, bc.numbytes_fork);
    if (op == OP_FORK_WITH_ID)
        id = READ_BYTES(bv, bc.numbytes_var_name);

    // Validate delay
    when = time.type == TYPE_INT ? time.v.num : time.v.fnum;
    if (when < 0) {
        RAISE_ERROR(E_INVARG);
    }

    // Enqueue forked task
    enum error e = enqueue_forked_task2(
        RUN_ACTIV,          // Current activation
        f_index,            // Fork vector index
        when,               // Delay in seconds
        op == OP_FORK_WITH_ID ? id : -1
    );
    if (e != E_NONE)
        RAISE_ERROR(e);
}
break;
```

#### Task Enqueueing (tasks.cc)

```c
enum error
enqueue_forked_task2(activation a, int f_index,
                     double after_seconds, int vid)
{
    struct timeval when;
    int id;
    Var *rt_env;

    // Check quota
    if (!check_user_task_limit(a.progr))
        return E_QUOTA;

    id = new_task_id();

    // Copy activation (increment refcounts)
    a._this = var_ref(a._this);
    a.vloc = var_ref(a.vloc);
    a.verb = str_ref(a.verb);
    a.verbname = str_ref(a.verbname);
    a.prog = program_ref(a.prog);
    a.threaded = DEFAULT_THREAD_MODE;

    // If named fork, store child ID in variable
    if (vid >= 0) {
        free_var(a.rt_env[vid]);
        a.rt_env[vid].type = TYPE_INT;
        a.rt_env[vid].v.num = id;
    }

    // Copy runtime environment
    rt_env = copy_rt_env(a.rt_env, a.prog->num_var_names);

    // Calculate start time
    when = double_to_start_tv(after_seconds);

    // Add to waiting queue
    enqueue_forked(a.prog, a, rt_env, f_index, when, id);
    return E_NONE;
}
```

#### Task Execution (execute.cc)

```c
enum outcome
do_forked_task(Program *prog, Var *rt_env, activation a, int f_id)
{
    check_activ_stack_size(current_max_stack_size());
    top_activ_stack = 0;

    // Set up activation
    RUN_ACTIV = a;
    RUN_ACTIV.rt_env = rt_env;

    // Execute (f_id points to fork vector in program)
    return do_task(prog, f_id, nullptr, 0/*bg*/, 1/*traceback*/);
}
```

**Key Differences from cow_py:**
- ToastStunt modifies parent's `rt_env` directly before copying (stores child ID)
- cow_py stores child ID after child creation during parent continuation
- Both achieve same result: parent sees child ID, child sees copied environment

### 3.3 Fork Vectors

**Concept:** Fork bodies are compiled into separate bytecode sequences called "fork vectors."

**Why?**
- Fork body might contain references to variables defined after the fork
- Fork body needs to be executable standalone
- Main program and fork bodies share the same literal pool and variable namespace

**Structure:**
```
Program:
  literals: [...]
  var_names: ["x", "y", "task_id"]
  fork_vectors: [
    [OP_PUSH "y", OP_IMM 1, OP_ADD, OP_PUT "y", ...],  # Fork 0
    [OP_PUSH "x", OP_CALL_BUILTIN "notify", ...],      # Fork 1
  ]
  main_vector: [
    OP_IMM 0, OP_PUT "y",
    OP_IMM 5, OP_FORK_WITH_ID (0, 2),  # Fork vector 0, store in var_index 2
    ...
  ]
```

**At fork execution:**
1. Push delay onto stack
2. Execute `OP_FORK_WITH_ID (0, 2)`
3. VM saves: `fork_vector[0]`, `rt_env`, `var_names`
4. Child task created with this saved state
5. When child runs, it executes `fork_vector[0]` as its bytecode

---

## 4. queued_tasks() Builtin

### 4.1 Purpose

Returns list of all queued (non-completed) tasks visible to the caller.

### 4.2 Return Format

```moo
{
  {task_id, start_time, x, y, z, programmer, vloc, verb, line, this},
  ...
}
```

Where:
- `task_id` - Unique task identifier
- `start_time` - When task started (or will start for delayed forks)
- `x, y, z` - Reserved (unused in modern MOO)
- `programmer` - Object ID with permissions
- `vloc` - Object where verb is defined
- `verb` - Verb name
- `line` - Current line number (for running tasks)
- `this` - Object context

### 4.3 Permission Model

**Non-wizard:** See only your own tasks (where `programmer == caller`)
**Wizard:** See all tasks

### 4.4 cow_py Implementation

```python
def queued_tasks(self) -> list[TaskInfo]:
    result: list[TaskInfo] = []
    for task in self.tasks.values():
        if task.state not in (TaskState.DONE, TaskState.ABORTED):
            result.append(TaskInfo(
                id=task.id,
                state=task.state.name,
                kind=task.kind.name,
                player=task.context.player,
                verb=task.context.verb,
                started_at=task.started_at,
            ))
    return result
```

### 4.5 ToastStunt Implementation

```c
static package
bf_queued_tasks(Var arglist, Byte next, void *vdata, Objid progr)
{
    bool show_all = is_wizard(progr);
    int count = 0;

    // Count tasks
    for (tq = active_tqueues; tq; tq = tq->next) {
        for (t = tq->first_bg; t; t = t->next)
            if (show_all ||
                (t->kind == TASK_FORKED
                    ? t->t.forked.a.progr == progr
                    : progr_of_cur_verb(t->t.suspended.the_vm) == progr))
                count++;
    }

    // Build result list
    tasks = new_list(count);
    for each task {
        list = list_for_vm(the_vm, progr, 0);
        tasks.v.list[i++] = list;
    }

    return make_var_pack(tasks);
}
```

---

## 5. Key Design Decisions and Tradeoffs

### 5.1 Why Fork is Not a Function

**Problem:** If fork were a function, it would need to:
- Return different values to parent (task ID) and child (0)
- Create child task in the middle of function call stack
- Clone entire VM state including all frames

**Solution:** Fork is a bytecode operation that:
- Suspends VM with special `OUTCOME_FORKED` state
- Lets TaskRunner orchestrate task creation
- Returns control to parent with child ID already stored

**Benefit:** Clean separation between VM (execution) and Scheduler (orchestration)

### 5.2 Fork Vector vs Code Cloning

**Alternative:** Clone parent's entire code and jump to fork body start

**Problem:**
- Wastes memory (duplicate code)
- Complicates variable scoping
- Makes it hard to reason about what child sees

**Chosen:** Compile fork body into separate vector

**Benefits:**
- Explicit boundaries (fork body is self-contained)
- Shared literal pool (efficient)
- Clear variable inheritance (copy rt_env)

### 5.3 Copy Environment, Not Share

**Choice:** Child gets a **deep copy** of parent's variable environment

**Why?**
- MOO semantics: child can't affect parent's variables
- Simplifies reasoning (no shared state bugs)
- Natural for async execution (parent may complete before child runs)

**Cost:** Memory overhead for large environments

**Mitigation:** Most MOO code has small variable sets; literals are shared

### 5.4 Delay Semantics

**Choice:** `fork (0)` doesn't run immediately - child is queued

**Why?**
- Prevents parent starvation (parent finishes before yielding)
- Allows scheduler to fairly distribute CPU
- Matches original LambdaMOO behavior

**Implication:** Even 0-delay forks are asynchronous - parent continues first

### 5.5 Task ID in Named Fork

**Two approaches:**

**ToastStunt:** Modify parent's `rt_env` before copying
```c
if (vid >= 0) {
    a.rt_env[vid].type = TYPE_INT;
    a.rt_env[vid].v.num = id;
}
rt_env = copy_rt_env(a.rt_env, ...);
```

**cow_py:** Store in parent after child creation during continuation
```python
if 'var_index' in fork_info:
    vm.current_frame.rt_env[adjusted_index] = child_task_id
```

**Why Different?**
- ToastStunt: Synchronous architecture (VM calls scheduler directly)
- cow_py: Async architecture (VM yields, TaskRunner mediates)

**Result:** Same semantics, different implementation points

---

## 6. Recommendations for barn Implementation

### 6.1 Architecture

**Follow cow_py's async model:**
1. Add `ForkStmt` AST node in `parser/ast.go`
2. Add `FlowFork` result type in `types/result.go`
3. Evaluator returns `FlowFork` with fork info
4. Scheduler creates child task, continues parent
5. Child executes when `StartTime` arrives

**Why?** Go's concurrency model aligns well with async task management. Goroutines + channels are natural for task scheduling.

### 6.2 Data Structures

**Add to Task:**
```go
type Task struct {
    // ... existing fields ...

    Kind TaskKind  // InputTask, ForkedTask, SuspendedTask
    ParentID int64 // For fork tracking
    ForkInfo *ForkInfo // Saved fork state
}

type TaskKind int
const (
    TaskInput TaskKind = iota
    TaskForked
    TaskSuspended
)

type ForkInfo struct {
    ForkBody     []parser.Stmt  // Compiled fork body
    VarEnv       map[string]types.Value // Copied variables
    This         types.ObjID
    Player       types.ObjID
    VarName      string // Variable to store task ID (empty if anonymous)
}
```

**Add Result Flow:**
```go
const (
    FlowNormal FlowType = iota
    FlowReturn
    FlowBreak
    FlowContinue
    FlowException
    FlowFork  // New: Fork statement executed
)

type Result struct {
    Flow     FlowType
    Val      types.Value
    Error    types.Error
    ForkInfo *ForkInfo  // Set when Flow == FlowFork
}
```

### 6.3 Evaluation

**In `vm/eval_stmt.go`:**
```go
case *parser.ForkStmt:
    return e.forkStmt(s, ctx)

func (e *Evaluator) forkStmt(stmt *parser.ForkStmt, ctx *types.TaskContext) types.Result {
    // 1. Evaluate delay
    delayResult := e.Eval(stmt.Delay, ctx)
    if !delayResult.IsNormal() {
        return delayResult
    }

    // 2. Convert to duration
    delay, err := valueToDuration(delayResult.Val)
    if err != nil {
        return types.Err(types.E_TYPE)
    }

    // 3. Copy variable environment
    varEnv := ctx.CopyVariables()

    // 4. Build fork info
    forkInfo := &ForkInfo{
        ForkBody: stmt.Body,
        VarEnv:   varEnv,
        This:     ctx.ThisObj,
        Player:   ctx.Player,
        VarName:  stmt.VarName, // Empty for anonymous fork
    }

    // 5. Return fork flow
    return types.Result{
        Flow:     types.FlowFork,
        ForkInfo: forkInfo,
    }
}
```

**In `server/scheduler.go`:**
```go
func (s *Scheduler) HandleForkResult(parent *Task, forkInfo *ForkInfo) {
    // 1. Create child task
    childID := atomic.AddInt64(&s.nextTaskID, 1)
    child := &Task{
        ID:         childID,
        Kind:       TaskForked,
        Player:     forkInfo.Player,
        Programmer: parent.Programmer,
        StartTime:  time.Now().Add(delay),
        Code:       forkInfo.ForkBody,
        Context:    contextFromForkInfo(forkInfo),
        ParentID:   parent.ID,
    }
    s.QueueTask(child)

    // 2. Store child ID in parent's variable (if named fork)
    if forkInfo.VarName != "" {
        parent.Context.SetVariable(forkInfo.VarName, types.NewInt(childID))
    }

    // 3. Continue parent execution
    // (Return from HandleForkResult allows Task.Run to continue)
}
```

### 6.4 Scheduler Integration

**Modify `Task.Run()`:**
```go
func (t *Task) Run(ctx context.Context, evaluator *vm.Evaluator, scheduler *Scheduler) error {
    // ... existing setup ...

    for _, stmt := range t.Code {
        result := evaluator.EvalStmt(stmt, t.Context)

        if result.Flow == types.FlowFork {
            // Handle fork
            scheduler.HandleForkResult(t, result.ForkInfo)
            // Continue parent (don't return)
            continue
        }

        // ... existing control flow handling ...
    }
}
```

### 6.5 Parser Changes

**Add to `parser/ast.go`:**
```go
type ForkStmt struct {
    Delay   Expr       // Expression evaluating to delay seconds
    VarName string     // Variable name for task ID (empty = anonymous)
    Body    []Stmt     // Fork body statements
}

func (s *ForkStmt) stmtNode() {}
```

**Add to `parser/parser.go`:**
```go
func (p *Parser) parseForkStmt() (*ForkStmt, error) {
    // fork [varname] (delay) body endfork
    p.expect(FORK)

    var varName string
    if p.peek().Type == IDENT {
        varName = p.next().Literal
    }

    p.expect(LPAREN)
    delay := p.parseExpr()
    p.expect(RPAREN)

    body := p.parseStatements()

    p.expect(ENDFORK)

    return &ForkStmt{
        Delay:   delay,
        VarName: varName,
        Body:    body,
    }, nil
}
```

### 6.6 Testing Strategy

**Unit Tests:**
1. Fork with immediate delay (0 seconds)
2. Fork with 1-second delay
3. Named fork (verify parent gets child ID)
4. Anonymous fork (verify parent doesn't block)
5. Fork with variable inheritance
6. Nested forks (fork within fork)
7. Fork exceeding quota (E_QUOTA)

**Integration Tests:**
1. queued_tasks() with no tasks
2. queued_tasks() with forked tasks
3. queued_tasks() permission filtering
4. kill_task() on forked task
5. Forked task inherits permissions correctly
6. Forked task independent of parent (parent completion doesn't kill child)

**Conformance Tests:**
- cow_py conformance suite doesn't have explicit fork tests yet
- Consider adding based on ToastStunt behavior

### 6.7 Common Pitfalls to Avoid

1. **Don't block parent on fork** - Child ID must be available immediately
2. **Don't share variables** - Child needs deep copy of environment
3. **Don't run child synchronously** - Even 0-delay forks are queued
4. **Don't forget tick budgets** - Forked tasks get fresh budget (typically 30k ticks)
5. **Don't let child inherit parent's task-local** - Each task has separate task_local() storage
6. **Don't skip permission checks** - Fork child runs with parent's `programmer` permissions
7. **Don't execute fork body in parent context** - Fork body is separate code path

### 6.8 Performance Considerations

**Memory:**
- Each fork copies variable environment (can be large)
- Consider copy-on-write optimization for large lists/maps
- Reuse evaluator instance across tasks (don't create per-task)

**Scheduling:**
- Use heap-based priority queue (Go's container/heap)
- Batch-process ready tasks (don't wake scheduler for each task)
- Consider task quota per user (prevent fork bombs)

**Concurrency:**
- Each task runs in own goroutine (barn already does this)
- Protect scheduler state with mutex
- Use channels for suspend/resume signaling

---

## 7. Implementation Checklist

- [ ] Add `ForkStmt` to AST
- [ ] Add `FlowFork` to Result types
- [ ] Implement `forkStmt()` in evaluator
- [ ] Add `ForkInfo` and `TaskKind` to Task
- [ ] Implement `HandleForkResult()` in Scheduler
- [ ] Update `Task.Run()` to handle `FlowFork`
- [ ] Add parser support for `fork`/`endfork` keywords
- [ ] Implement `queued_tasks()` builtin
- [ ] Implement `kill_task()` builtin
- [ ] Add fork-related tests
- [ ] Document fork semantics in spec

---

## 8. References

### Source Files Examined

**cow_py:**
- `src/cow_py/task.py` - Task state machine
- `src/cow_py/scheduler.py` - Task scheduling
- `src/cow_py/task_runner.py` - Task execution and fork handling

**moo_interp:**
- `moo_interp/vm.py` - VM execution and fork opcodes
- `moo_interp/moo_ast.py` - AST and bytecode compilation
- `moo_interp/opcodes.py` - Opcode definitions

**ToastStunt:**
- `src/tasks.cc` - Task management and queuing
- `src/execute.cc` - VM execution loop and fork opcode
- `src/include/tasks.h` - Task interfaces
- `src/include/execute.h` - Activation and VM structures

**barn:**
- `server/task.go` - Current task implementation
- `server/scheduler.go` - Current scheduler
- `vm/eval_stmt.go` - Statement evaluation

### Key Insights

1. **Fork is not a function** - It's a control flow operation that yields from VM
2. **Child task creation is scheduler's job** - VM just signals intent
3. **Variable copying is essential** - No shared state between parent and child
4. **Fork vectors enable clean separation** - Fork body is standalone code unit
5. **Named vs anonymous fork** - Only difference is storing child ID in variable
6. **Delay is always asynchronous** - Even fork(0) queues child task
7. **Permissions follow programmer** - Child inherits parent's permission context

---

## Appendix A: Fork Execution Flow Diagram

```
[Parent Task Running]
        |
        v
[Execute fork (5)]
        |
        v
[Eval delay expression] -> 5 seconds
        |
        v
[VM: exec_fork()]
        |
        v
[VM sets state = OUTCOME_FORKED]
[VM saves fork_info]
        |
        v
[TaskRunner detects OUTCOME_FORKED]
        |
        +---------------------------+
        |                           |
        v                           v
[Create Child Task]        [Continue Parent]
ID = 1234                   Store 1234 in var
StartTime = now+5           Resume execution
ForkBody = ...              Parent continues
        |                           |
        v                           v
[Queue Child]              [Parent completes]
        |
        v
[Scheduler: wait 5 seconds]
        |
        v
[Child StartTime reached]
        |
        v
[Execute Child Task]
Fork body runs
Child ID = 1234
        |
        v
[Child completes]
```

---

## Appendix B: Variable Environment Copying

**Example Code:**
```moo
x = 10;
y = {1, 2, 3};

fork task_id (0)
    x = x + 1;  // x = 11 in child
    y = {@y, 4}; // y = {1,2,3,4} in child
endfork

x = x * 2;  // x = 20 in parent
y[1] = 99;  // y = {99,2,3} in parent

// Result:
// Parent: x=20, y={99,2,3}, task_id=1234
// Child:  x=11, y={1,2,3,4}
```

**Memory State at Fork:**
```
Parent rt_env:
  x -> 10
  y -> List[1,2,3]
  task_id -> (uninitialized)

Child rt_env (deep copy):
  x -> 10
  y -> List[1,2,3]  (separate list object)
  task_id -> (not present in child)

After Fork Setup:
Parent rt_env:
  x -> 10
  y -> List[1,2,3]
  task_id -> 1234

Child rt_env:
  x -> 10
  y -> List[1,2,3]
```

**Key:** Lists, maps, and waifs are deep-copied. Modifications in child don't affect parent.

---

## Appendix C: Comparison Matrix

| Feature | cow_py/moo_interp | ToastStunt | barn (recommended) |
|---------|-------------------|------------|---------------------|
| **Fork Storage** | Fork vectors in Program | Fork vectors in Program | Statements in ForkInfo |
| **Task State** | PENDING/RUNNING/SUSPENDED/DONE | TASK_FORKED/TASK_SUSPENDED | TaskCreated/TaskWaiting/TaskRunning |
| **ID Assignment** | TaskRunner stores in rt_env during continuation | enqueue_forked_task modifies rt_env before copy | Scheduler stores in Context after child creation |
| **Variable Copy** | Deep copy with `list()` | copy_rt_env (refcount) | Need deep copy for values |
| **Scheduler Loop** | gevent cooperative | select/poll-based | Goroutines + ticker |
| **Concurrency** | Single-threaded (gevent) | Single-threaded (C) | Multi-goroutine (Go) |
| **Child Start** | resume_time check | start_tv comparison | StartTime comparison |

---

**End of Report**
