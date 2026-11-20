# Barn task / suspend / limits survey (federation design grounding)

Facts only, with `file:line` citations. Paths relative to `C:\Users\Q\code\barn`.
Ignore `.claude/worktrees/**` and `.tmp/**` copies — all citations below are the
live tree.

---

## 1. Suspend semantics

### The signalling mechanism

`suspend()` is `builtins/tasks.go:120-163`. It parses the seconds argument
(`-1.0` is the internal sentinel for indefinite; a negative *user-supplied*
value is rejected with `E_INVARG` at `tasks.go:148-150`), calls
`mgr.SuspendTask(t, seconds)`, then returns:

```go
// builtins/tasks.go:162
return types.Suspend(seconds)
```

`types.Suspend` (`types/result.go:88-90`) builds `Result{Flow: FlowSuspend,
Val: NewFloat(seconds)}`. `FlowSuspend` is a control-flow enum member
(`types/result.go:17`).

The VM sees `FlowSuspend` from a builtin call and yields
(`vm/op_misc.go:71-76`); `vm/persistence.go:146` sets
`machine.yieldResult = types.Result{Flow: types.FlowSuspend}`. The VM pushes a
default `0` onto the operand stack at the yield point, which the resume path
overwrites (see §3).

`Manager.SuspendTask` (`task/manager.go:124-141`) has three arms:

| seconds | behavior |
|---|---|
| `< 0` | `task.SuspendIndefinite()` — no wake deadline, waits for `resume()` |
| `== 0` | `task.Suspend(0)` then immediately `task.Resume(NewInt(0))` — a pure scheduler yield, task goes straight back to `TaskQueued` |
| `> 0` | `task.Suspend(duration)` — sets `WakeTime = now+duration` |

State transitions live in `task/task.go:481-500`. `SuspendIndefinite` stamps
`StartTime = IndefiniteSuspendStartTime` (`task/task.go:478`, `time.Unix(1<<62,
0)`) so the task sorts last in `queued_tasks()`, mirroring Toast's
`INTNUM_MAX start_tv`; `WakeTime` deliberately stays zero so nothing auto-wakes
it.

### Does suspension commit the transaction? Yes.

`engine/task_runtime.go:404-434` is the suspend hand-off block. Inside it:

```go
// engine/task_runtime.go:417-434
if ctx.StoreTxn != nil && ctx.StoreTxn.HasWrites() {
    if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
        result = types.Err(errCode)
        t.Result = result
        t.SetState(task.TaskKilled)
        t.SetBytecodeVM(nil)
        s.discardCreatedForks(t)
        builtins.DiscardPendingEffects(...)
        return nil
    }
    t.CreatedForks = nil
    builtins.FlushPendingEffects(s.registry.NewExecution(ctx, t))
    ctx.StoreTxn.Release()
    ctx.StoreTxn = s.store.BeginReadOnly(0)
}
```

Three things to note for a federation design:

1. **A failed commit at a suspend boundary kills the task** (`TaskKilled`,
   `SetBytecodeVM(nil)`, line 421-425). There is no retry at this point — the
   conflict-retry loop is upstream (line 315) and has already been passed.
2. **Buffered external effects flush on the suspend commit**
   (`FlushPendingEffects`, line 431). `notify`, `switch_player`, `boot_player`,
   `load_server_options` are the buffered set (per the comment at
   `builtins/registry.go:439-440`).
3. **The task gets a brand-new read-only transaction after the commit**
   (`ctx.StoreTxn = s.store.BeginReadOnly(0)`, line 433).

### Atomicity contract on resume

A resumed task runs under a *fresh* MVCC snapshot, not the pre-suspend one.
`engine/task_runtime.go:433` installs `BeginReadOnly(0)` right after the
suspend commit, and each subsequent execution slice re-enters `runTask` with a
fresh `ctx` setup (`engine/task_runtime.go:147-148` resets `LiveStoreMutated`
and `IrreversibleSideEffect`). So: writes before the suspend are durable and
visible to everyone; anything the task reads after resuming reflects the world
as of the resume, and the pre-suspend read set is no longer validated. Suspend
is a hard transaction boundary in both directions.

Suspension is also an **anonymous-object GC boundary**
(`engine/task_runtime.go:404-414`): the sweep is deferred, and the suspended
task's saved VM is registered via `SetBytecodeVM` (line 442) so the flush-time
root scan still sees its locals.

The re-queue itself happens under `s.mu` (`engine/task_runtime.go:441-456`):
`SetBytecodeVM(bcVM)`, and if the task is already `TaskQueued` (the
`suspend(0)` case), it stamps `WakeTime = time.Now()`, bumps `s.queueSeq`, and
`heap.Push(s.waiting, t)`.

There is a `suspend(0)` fast path that avoids the full re-dispatch:
`engine/task_runtime.go:371-385` loops up to 16 zero-delay yields inline for
forked tasks before falling through.

### Enumerating the suspend flavors

All flavors are the same `TaskSuspended` state plus discriminator fields on the
task struct (`task/task.go:138-145`):

```go
WakeTime            time.Time
WakeValue           types.Value
WakeErrorAsValue    bool        // return an error-typed wake value instead of raising
IsExecSuspended     bool        // suspended by exec(); can't resume, only kill
ExecCommandName     string
IsHTTPReadSuspended bool        // suspended by read_http()
ReadingPlayer       types.ObjID // player this task is read()ing from (ObjNothing = not reading)
```

| Flavor | Set at | Woken by | Notes |
|---|---|---|---|
| Timed `suspend(n)` | `task/task.go:481-488` | scheduler, `WakeDue` (`task/task.go:568-572`) | `WakeTime` set |
| Indefinite `suspend()` | `task/task.go:495-500` | `resume()` only | far-future `StartTime` sentinel |
| `suspend(0)` yield | `task/manager.go:133-136` | immediate self-resume | never observably suspended |
| `read()` | `builtins/signatures.go:497-556`; `SetReadingPlayer` at :547, then `SuspendTask(t, -1)` at :554 | input arrival via `InputProcessor.deliverToReadingTask` (`server/input_processor.go:353`) → `Manager.FindReadingTask` (`task/manager.go:146-164`) | FIFO by `QueueSeq` then ID |
| `exec()` | `builtins/system.go:250-259` | background goroutine calling `t.CompleteExec(...)` (`builtins/system.go:262-278`, `task/task.go:550-565`) | `Resume()` explicitly refuses (`task/task.go:511-513`); 30s subprocess timeout at `system.go:246` |
| `read_http()` | `builtins/network.go:1376-1377` | `Registry` HTTP read plumbing; cancelled by `kill_task` via `CancelHTTPReadTask` (`builtins/tasks.go:110`) | cleared on any `Resume` (`task/task.go:516`) |
| `read_stdin()` | `builtins/signatures.go:879-895` | `stdin.ReadLineAsync(t)` | the only user of `WakeErrorAsValue = true` (line 890) |

**How a new flavor plugs in.** The minimal contract a new suspend flavor must
satisfy, based on the existing ones:

1. A builtin sets its discriminator field on `ctx.Task`, calls
   `mgr.SuspendTask(t, -1)`, and returns `types.Suspend(-1)`.
2. Some external agent later calls `t.Resume(value)` / `t.ResumeAndClaim(value)`
   / a bespoke completer like `CompleteExec`. `Resume` refuses when
   `IsExecSuspended` (`task/task.go:511`), so a flavor that must not be
   resumable by MOO `resume()` sets a similar guard.
3. `Kill()` must clear the flag and cancel any external work
   (`task/task.go:574-587` does this for exec and http).
4. The flag must be added to `task.Snapshot` (`task/snapshot.go:19-37`) and to
   `IsInterruptedForPersistence()` (`task/snapshot.go:82-84`) if the flavor
   should serialize into the *interrupted* rather than *suspended* checkpoint
   section.
5. If the builtin does external I/O, add it to
   `builtinHasIrreversibleSideEffect` (`builtins/registry.go:441-453`).

---

## 2. Task serialization into checkpoints

**Yes — suspended tasks are serialized with full VM state.**

`task/snapshot.go:39-77` defines the persistence shape:

```go
type VMSnapshot struct {
    MaxStackDepth int
    Frames        []VMFrameSnapshot
}

type VMFrameSnapshot struct {
    Program      bytecode.Program
    IP           int
    Locals       []types.Value
    Stack        []types.Value
    This/ThisValue/Player/Verb/StoredVerb/Caller/VerbLoc/Args ...
    ExceptStack  []bytecode.Handler
    PendingError VMErrorSnapshot
    ...
}
```

That is the whole resumable machine: per-frame program, instruction pointer,
locals, operand stack, exception handler stack, and an in-flight error held
across a `finally`.

- Snapshot capture: `task.PersistenceSnapshot()` (`task/snapshot.go:136-182`),
  under the task read lock; the VM is snapshotted through the `vmSnapshotter`
  interface (`task/snapshot.go:130-132`, `:178-180`).
- Collection for checkpoint: `Runtime.TaskSnapshots()`
  (`engine/task_factory.go:457-481`). Completed/killed tasks are skipped;
  anything with a non-nil VM goes to `suspended`, otherwise it's bucketed by
  state into `queued` vs `suspended`.
- Writing: `db/format/checkpoint.go:69` calls
  `writer.SetTaskSnapshots(queued, suspended)`.
  `db/format/writer_task.go:11-22` splits `suspended` further into
  **suspended** vs **interrupted** by `IsInterruptedForPersistence()`
  (read/exec/http flavors go to interrupted).
  Three sections are emitted: `writeQueuedTasks` (:25), `writeSuspendedTasks`
  (:93), `writeInterruptedTasks` (:111).
- Hard failure if unserializable: `writer_task.go:95-97` returns
  `"suspended task %d has no serializable VM state"`, and `:27-29` returns
  `"queued task %d has no serializable fork program"`. A checkpoint fails
  rather than silently dropping a task.
- Reachability: `Snapshot.TransformPersistenceValues`
  (`task/snapshot.go:90-128`) walks every value the writer emits (constants,
  locals, operand stack, args, pending error, task-local, wake value) — that is
  the single definition of the task persistence surface for GC-root seeding and
  reference rewriting.

### On restart

- Reading: `db/format/reader_task.go:195` (`readSuspendedTasks`) and `:260`
  (`readInterruptedTasks`). **Interrupted tasks are appended into the same
  `SuspendedTasks` slice** (`reader_task.go:279`), so read/exec/http-flavored
  tasks come back as ordinary suspended tasks — the flavor flags are not
  restored.
- Restoring: `engine/task_load.go:30-101` (`LoadSuspendedTasks` /
  `loadSuspendedTask`) and `:103-176` (`loadQueuedTask`).
  `loadSuspendedTask` rebuilds the VM via `vm.RestoreVMSnapshot`
  (`task_load.go:74`), sets `Kind = task.TaskSuspendedTask`, forces
  `IsForked = true` (`:56`), and sets `WakeTime = saved.StartTime` (`:85`) so a
  restored task is due immediately-ish rather than indefinitely parked.
  Restore failures are logged and skipped, not fatal (`task_load.go:36-39`).
- Task IDs are monotonically bumped past every restored ID via CAS
  (`task_load.go:91-99`).

---

## 3. Resume

`resume(task_id [, value])` is `builtins/tasks.go:169-196`. It requires an INT
task id, defaults `value` to `NewInt(0)`, and delegates to
`Manager.ResumeTask` (`task/manager.go:101-121`), which checks
owner-or-wizard (`manager.go:108-110`), requires `TaskSuspended`
(`:112-114`), then calls `task.Resume(value)`.

`task.Resume` (`task/task.go:504-527`) sets `State = TaskQueued`,
`WakeValue = value`, clears `IsHTTPReadSuspended`, and clears the
indefinite-suspend `StartTime` sentinel back to now.

`ResumeAndClaim` (`task/task.go:532-545`) is the same but goes straight to
`TaskRunning`, for synchronous resume paths where publishing `TaskQueued` would
let the ordinary scheduler grab the same task.

**Can arbitrary MOO values be injected as the suspend() return value? Yes**, with
one wrinkle around error values. The value flows
`WakeValue` → `bcVM.SetResumeValue(t.WakeValue, t.WakeErrorAsValue)`
(`engine/task_runtime.go:203`, also `:370` and `engine/eval.go:182`) → the VM:

```go
// vm/vm.go:266-275
func (vm *VM) SetResumeValue(val types.Value, errorAsValue bool) {
    if val.Type() == types.TYPE_ERR && !errorAsValue {
        vm.resumeError = val.ErrCode()
        return
    }
    vm.resumeError = types.E_NONE
    if vm.SP > 0 {
        vm.Stack[vm.SP-1] = val
    }
}
```

So any non-error value is written directly over the top-of-stack slot the yield
pushed. An **error value resumes as a raised exception**, unless
`WakeErrorAsValue` is set — only `read_stdin` does that
(`builtins/signatures.go:890`). The wake value is consumed after delivery
(`t.WakeValue = types.None` at `engine/task_runtime.go:204`) so it cannot leak
into a later suspend.

Note `resume` is itself classified as an irreversible side effect
(`builtins/registry.go:448`), so a task that calls `resume()` can no longer be
conflict-retried.

---

## 4. Resource limits that exist today

### Per-task ticks and seconds — these exist

`builtins/limits.go:100-113` defines the defaults snapshot and
`collectServerOptions` (`:135-226`) reads from `$server_options`:

- `max_string_concat` (`limits.go:159`)
- `max_list_value_bytes` (`:167`), `max_map_value_bytes` (`:175`)
- `fg_ticks` (`:182`), `bg_ticks` (`:188`)
- `fg_seconds` (`:194`), `bg_seconds` (`:200`)
- `max_stack_depth` (`:206`, also read directly at `limits.go:63-70`)
- `max_crypt_bcrypt_cost` (`:212`), `max_crypt_sha_rounds` (`:218`)

Accessors: `GetTaskLimits(background bool)` (`limits.go:53`),
`GetMaxStackDepth` (`:63`). Tick consumption is
`Task.ConsumeTick()` (`task/task.go:428-433`), remaining budget via
`TicksLeft()` / `SecondsLeft()` (`task/task.go:414-425`).
`E_QUOTA` enforcement points: `CheckListLimit`/`CheckMapLimit`/
`CheckStringLength` (`builtins/limits.go:362-420`), `kernel/context.go:180-191`.

### Per-player queued-task limit — **does not exist**

I searched the whole tree for `queued_task_limit`, `MAX_QUEUED_TASKS`,
`queuedTaskLimit`. **Zero hits in Go source.** `$server_options.queued_task_limit`
is not read by `collectServerOptions`, and nothing counts tasks per owner before
enqueueing. `fork` has no admission control: `CreateForkedTask`
(`engine/task_factory.go:198`) allocates an ID and queues unconditionally.

The only reclamation pressure is periodic:
`Manager.CleanupCompletedTasks` (`task/manager.go:168-180`) deletes
completed/killed tasks, driven by a 5-second ticker
(`server/input_processor.go:186-187`, `:199-202`) whose comment explicitly
frames it as preventing unbounded growth of the pre-auth login path.

### `MAX_QUEUED_OUTPUT` / per-connection output limit — **does not exist**

`Connection.outputBuffer` is an unbounded `[]string`
(`server/connection.go:23`, `:63`); `Buffer()` (`:78-82`) appends with no cap.
`BufferedOutputLength()` (`:202-206`) reports the length but nothing enforces a
ceiling. No `MAX_QUEUED_OUTPUT` or `max_queued_output` anywhere in the tree.

### Backpressure that does exist

- **Global input queue**: `inputQueue` is a buffered channel of 256
  (`server/input_processor.go:43`). `EnqueueInput` does a plain blocking send
  (`:67-69`, `p.inputQueue <- evt`), so a full queue blocks the connection
  reader goroutine. That is real backpressure, but it is *global*, not
  per-player — one flooding connection can stall all readers.
- **Per-connection serial lane**: `dispatch` (`input_processor.go:221-235`)
  fans out to a per-`ConnID` channel of capacity 64, one goroutine per
  connection (`connectionWorker`, `:238-259`). Send is
  `select { case ch <- input: case <-p.ctx.Done(): }` — blocking, so a slow
  lane backpressures the dispatcher, and via the dispatcher the global queue.
  Lanes are retired on disconnect (`:248-256`).
- **Starvation guard**: `processRuntimeTick` (`:205-215`) rechecks the input
  queue before running another task so a busy runtime can't starve socket input.
- **WebSocket frame cap**: `wsConn.SetReadLimit(1 << 20)` — 1 MiB
  (`server/connection_manager.go:386`).
- **Login/connect timeout**: default 5 minutes, overridable by
  `$server_options.connect_timeout` (`server/input_processor.go:87-95`).
- **exec subprocess timeout**: hard-coded 30 seconds
  (`builtins/system.go:246`).

### Explicitly absent

No per-connection rate limiting (messages/sec or bytes/sec), no max-connection
count, no per-IP limits, no accept throttling, no checkpoint size control. Plain
TCP transport uses an unbounded `bufio.NewReader(conn)`
(`server/transport.go:68`) with no line-length cap that I could find.

---

## 5. Transactions: irreversible side effects and the retry loop

Two flags on the task context (`kernel/context.go:61-70`):

```go
// LiveStoreMutated is set after a task mutates Store directly outside
// StoreTxn. Validation conflicts after this point must not retry the task
// body, because the direct mutation cannot be rolled back.
LiveStoreMutated bool

// IrreversibleSideEffect is set after a task begins an immediate external
// effect that cannot safely be replayed (logging, input injection, listeners,
// outbound I/O, file/SQLite/process operations, and task control).
IrreversibleSideEffect bool
```

**The classifier is a static name list**, applied by wrapping the builtin at
registration time (`builtins/registry.go:381-389`):

```go
invoke := fn
if builtinHasIrreversibleSideEffect(name) {
    invoke = func(ctx *Execution, args []types.Value) types.Result {
        if ctx != nil && ctx.TaskContext != nil {
            ctx.IrreversibleSideEffect = true
        }
        return fn(ctx, args)
    }
}
```

The list (`builtins/registry.go:441-453`) — note the flag is set *before* the
builtin runs, so even a failed call poisons retry:

`reseed_random`, `listen`, `unlisten`, `set_connection_option`,
`open_network_connection`, `read_http`, `flush_input`, `force_input`, `curl`,
all `file_*` (14 of them), all `sqlite_*` (6), `dump_database`, `read_stdin`,
`shutdown`, `exec`, `server_log`, `run_gc`, `reset_max_object`, `kill_task`,
`resume`.

`LiveStoreMutated` is the parallel mechanism for direct (non-transactional)
store mutation, set by `markLiveStoreMutated` (`builtins/store_reads.go:20-24`)
from object/property/verb builtins (`builtins/objects.go:268`, `:485`,
`objects_hierarchy.go:255`, `:408`, `objects_movement.go:97`,
`objects_misc.go:80`, `verbs.go:462`, `:602`, `:670`, `store_reads.go:51`,
`:58`) and by the runtime at `engine/runtime.go:154-155`.

**The retry gate** is one condition, `engine/task_runtime.go:315`:

```go
if (errCode == types.E_INVARG || errCode == types.E_INVIND) &&
   ctx.StoreTxn.ValidationFailed() &&
   !ctx.LiveStoreMutated &&
   !ctx.IrreversibleSideEffect &&
   retryState.canRetry && attempt < maxConflictRetryAttempts {
```

When it holds: `discardCreatedForks`, `DiscardPendingEffects`,
`NoteCommitRetry`, `attempt++`, `goto retryAttempt` (lines 316-329) — the whole
task body re-runs. When it does not hold, the commit error becomes the task's
result (`:331-333`) and everything is discarded (`:341-344`).

The `E_INVIND` arm is deliberate: a create-under-P racing a recycle-of-P would
otherwise surface a raw `E_INVIND` that no serial ordering can produce (comment
at `:306-314`).

Buffered-until-commit effects are the counterpart: `PendingEffects`
(`kernel/context.go:76-78`), flushed on success (`FlushPendingEffects`) and
dropped on failure (`DiscardPendingEffects`). `notify`, `switch_player`,
`boot_player`, `load_server_options` use this instead of the irreversible flag.

There is also an escalation path (`escalated` / `s.store.EscalationUnlock()` /
`ClearCommitGateExemption`, `engine/task_runtime.go:353-359`) with a `defer`
backstop at `:85-99` so the gate never outlives the invocation.

Outbound network is additionally gated by a build/config feature flag:
`config/options.go:11` `OutboundNetwork`, checked at `builtins/curl.go:22` and
`builtins/signatures.go:832`; reported via
`FeatureOutboundNetwork = "option.OUTBOUND_NETWORK"` (`config/options.go:4`).

---

## 6. Network / listeners / the `network.go:399` panic

### The referenced panic appears to be already fixed

`docs/reports/federation-barn-groundwork.md:149` describes
"`network.go:399` `int(size)+2` overflow with no `recover()` on the connection
path", and `plans/moo-federation.md:289` carries it forward as a
non-negotiable prerequisite. **The current code at
`builtins/network.go:337-346` has an explicit pre-conversion bounds check:**

```go
// Check the attacker-controlled size before converting it to int or
// adding the trailing CRLF length. Either operation can overflow for a
// syntactically valid, very large chunk size.
remaining := len(data) - pos
if remaining < 2 || size > int64(remaining-2) {
    return "", 0, false
}
chunkSize := int(size)
```

Line 399 today is unrelated (inside `parseHTTPResponse`). The only `int(size)`
in the tree is the guarded one above. **The plan's citation is stale** — either
the fix landed, or the reference always pointed at a moved line. Worth
re-confirming against the plan author's intent before treating it as open work.

### Panic handling on the inbound path

There is **no `recover()` in `server/`** at all. `grep "recover()"` across
non-test Go finds exactly three sites, all in `engine/`:

- `engine/task_runtime.go:73-84` — the task-level recover. Increments
  `metrics.PanicsRecovered`, logs `task_id`/`this`/`verb`/`panic`/`go_stack`,
  sets `TaskKilled`, returns an error.
- `engine/call_verb.go:185`
- `engine/eval.go:27`

So a panic inside verb execution is contained per-task. A panic in a connection
reader goroutine, the websocket HTTP handler, or the input-processor dispatch
lane is **not** contained and takes the process down. `net/http` recovers
handler panics for the websocket listener path, but the goroutine spawned at
`connection_manager.go:392` (`go cm.handleConnection(conn)`) is outside that.

### Listener / websocket code

- `ConnectionManager.BindListeners` (`server/connection_manager.go:93`),
  `registerListener` (`:221`), `acceptConnections` (`:286`),
  `handleNewConnection` (`:331`).
- WebSocket: `serveWebSocketListener` (`:353`) runs
  `record.httpServer.Serve(record.listener)`; `handleWebSocketRequest`
  (`:360-394`) validates path and Upgrade header, refuses during shutdown with
  503, upgrades via `websocket.Accept(w, r, &websocket.AcceptOptions{
  InsecureSkipVerify: true})` (`:381`) — **origin checking is disabled** — sets
  the 1 MiB read limit (`:386`), wraps in `NewWebSocketTransport`
  (`server/websocket_transport.go:37`), and spawns `handleConnection`.
- Runtime `listen()`/`unlisten()` go through `AddListener` (`:657`),
  `listenAndRegister` (`:694`), `RemoveListener` (`:713`).
- Outbound: `OpenNetworkConnection` (`:741`), `detachOutboundClient` (`:827`).
- TLS: config is threaded through `registerListener`; see
  `server/tls_listener_test.go`.

### Per-connection rate limiting / output limits

None, as covered in §4. The only per-connection caps are the 1 MiB websocket
frame limit, the 64-slot input lane, and the connect timeout.

---

## Summary of gaps relevant to a federation resource-exhaustion design

1. No per-player queued-task limit (`queued_task_limit` unimplemented) and no
   fork admission control.
2. No output buffer bound (`MAX_QUEUED_OUTPUT` unimplemented) — a remote peer
   generating notifies grows `[]string` without limit.
3. Backpressure is blocking-channel based and global (256) plus per-connection
   (64); a flooding peer can stall the global input queue.
4. No panic containment on the inbound network path; only per-task recovery.
5. WebSocket accepts any Origin (`InsecureSkipVerify: true`).
6. Interrupted (read/exec/http) suspend flavors lose their flavor across a
   checkpoint/restart round trip — they reload as plain suspended tasks with
   `WakeTime = StartTime`.
7. A commit failure at a suspend boundary silently kills the task, with no
   retry.
