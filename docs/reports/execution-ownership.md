# Concrete execution ownership

Research assignment, 2026-10-26. This is a proposed implementation, not an
implemented or tested change. Sources below are the current worktree. No tests,
builds, commits, or publication were performed for this report.

## Smallest selected design

Make `engine.Task` the executable owner: an inline `task.State` and a private
`machine *vm.VM`. Rename the existing lower `task.Task` to `task.State`, retaining
its language-visible state and lifecycle operations. VM and builtin code receive
`&owner.State`; runtime, catalog, scheduler, persistence capture, and server
execution entry points receive `*engine.Task`. There is one task object and one
task mutex. Do not embed a separately allocated `*task.State`.

The one-allocation, one-lock and no-new-registry constraints are implementation
preferences in the research brief, not additional requirements attributed to Q.
The source permits retaining them. This necessarily changes more than the field
declaration: the existing catalog and scheduler need the executable task too.

Use a narrow live profiling-parent contract for `CallingVM`, because its only
consumer needs profiling operations, not interpreter execution. It must preserve
ancestor tick accounting, time baselines, and live frame collection; a copied
stack is insufficient. This is a consumer contract, not a general VM interface.

## Source facts determining the boundary

* `task/task.go:60-148` combines task identity, suspension, activations, execution
  budgets, program/context and an opaque VM field under one mutex.
  `vm/vm.go:11-14,32-34` imports both builtins and task and retains task context.
  `builtins/registry.go:16-36` also retains the task. Giving the existing lower
  task a concrete VM therefore creates both direct and transitive cycles.
* `task/task.go:804-815` serializes saved-machine loads and stores with `t.mu`.
  `task/snapshot.go:181-224` holds that same lock across task-field copying and
  the VM persistence snapshot. `vmSnapshotter` is solely the opaque bridge to
  remove; the persisted snapshot records remain useful lower-level data.
* `task/manager.go:61-96` takes catalog mutex, task mutex, then unfinished mutex
  on registration/removal. `task/task.go:313-338` notifies the unfinished index
  when either terminal state or physical execution changes. The index is not
  merely a list of nonterminal logical states.
* `engine/internal/scheduler/scheduler.go:314,404` uses saved-machine presence
  to distinguish continuations from caller-dispatched fresh foreground tasks.
  Its queue, workers, flights, and callback signatures own task pointers
  (`:17-64`). A scheduler of lower State pointers would require recovering the
  executable owner to run them.
* `engine/call_verb.go:249-268` creates a direct task and acquires execution
  ownership before lookup/compilation without first registering it. Meanwhile
  `engine/task_factory.go:302-304,326-348` creates a fork VM, assigns its task,
  and only then queues the child. Catalog lookup cannot replace ownership.
* `engine/runtime.go:650-750` uses the runtime lock and physical execution
  counts to protect GC walks, separately from locking machine-pointer reads.
  Moving the pointer does not make walking a running VM safe.
* `vm/retry.go:11-47` distinguishes retry state from disk persistence, retaining
  live execution state and rebinding context/task on restoration. Keep this
  representation and VM/frame reuse unchanged.
* `vm/op_misc.go:13-30` allocates and binds its builtin Execution and service
  closures once per VM. `engine/call_verb.go:129-135` consumes CallingVM only to
  set a nested VM's profile parent. `vm/profile.go:85-130` needs live parent
  baselines and frames, not a saved task root VM (nested VMs can share a task).

## Exact ownership and symbol moves

1. In `task/task.go`, rename `Task` to `State` and receivers/within-state links
   accordingly. Preserve context, program, fork metadata, `ForkCreator`,
   cancellation, call stack, task-local state, budgets, readiness, input receipts,
   execution/admission flags, and their existing methods here. VM/builtins use
   these today; moving them upward would require forwarding a large API back
   down. `ForkCreator.CreateForkedTask` takes `*State`. No compatibility alias.
   Rename the enum-valued State field to Status: otherwise embedding task.State
   makes engine.Task.State refer to the embedded struct and shadows the old
   status field. GetState can retain its current public spelling.
2. Add `engine/task.go` defining `Task` with value-embedded `task.State`, private
   `machine *vm.VM`, and private `catalog *TaskManager`. Move machine getter and
   setter here and make them concretely typed. Name them `VM()` and `SetVM`.
   Lower State has no saved-machine marker or replacement snapshot field.
3. Move constructors `NewTask` and `NewTaskFull` to engine. Initialize lower
   defaults in place with `(*State).Init(id int64, owner types.ObjID,
   tickLimit int64, secondsLimit float64)`; full construction initializes
   the existing Context allocation separately. Initialize State only before
   publication. Do not assign `State: *task.NewTask(...)` or copy an initialized
   mutex. Concretely, allocate `t := &Task{}`, call `t.State.Init(...)`, and bind
   its settlement observer to t before returning it. Update direct literals
   to nested `State: task.State{...}` literals or
   the constructor; preserve the direct path's deliberately different defaults.
4. Move `task/manager.go` and its manager tests to `engine/task_manager.go` and
   corresponding tests. Rename Manager to TaskManager. Keep the existing two
   maps (catalog and unfinished), now holding `*engine.Task`; add no third map.
   Catalog methods for runtime ownership return executable Tasks. Builtin-facing
   finder methods return State, using distinct names such as `FindTaskState`
   versus the executable `GetTask`. `GetAllTasks`/`GetQueuedTasks` can retain
   their existing builtin names and return `[]*task.State`; executable scans
   already have Snapshot/Unfinished. Return actual inline-state addresses.
5. Move `engine/internal/scheduler/scheduler.go` and its tests into package
   engine, with private scheduler/result names where appropriate. Otherwise an
   engine.Task import from the child package produces an import cycle. Keep the
   scheduling algorithm; change owner pointers. Its blocked-readier map at
   `scheduler.go:347-369` becomes keyed by `*task.State`, with lookups using
   `&t.State`. This preserves pointer identity without reverse lookup, unsafe
   casts, or assuming IDs identify the same object.
6. In `task/snapshot.go`, retain Snapshot/VMSnapshot/frame records and copying
   helpers. Remove vmSnapshotter. Expose the task-field capture as
   `State.PersistenceSnapshotLocked()` with an explicit caller-held read-lock
   contract. `engine.Task.PersistenceSnapshot` holds the single task lock,
   invokes that helper, and calls its concrete machine's persistence method.
   `engine/task_factory.go:488-493` remains the capture owner; db/format keeps
   accepting the existing `task.Snapshot` records.
7. Change VM and builtin concrete task types to State. Change engine/server
   executable types and constructors to engine.Task. Do not blindly replace
   every task reference: `CallVerbInContext` and `drainForks` can operate on
   State because the child VM is already a separate argument, while runTask,
   restore, queue, GC/catalog iteration and machine persistence need the owner.
   `CreateForkedTask` accepts lower parent State and constructs upper child Task.

## One shared lock and catalog settlement

Expose the existing lower mutex as a named `Mu sync.RWMutex`; all existing State
methods keep locking it. Engine VM access and combined persistence capture lock
the same field. This exposes synchronization to the actual containing owner;
it adds neither a mutex nor a callback-based locking facade. A named field also
avoids promoting arbitrary Lock methods on the whole task accidentally.

Replace lower `catalog *Manager` with a narrow settlement observer interface:
`TaskSettled(bool)`. `*engine.Task` implements it and forwards to its current
catalog's existing noteFinished operation. Bind the observer to the containing
Task while constructing it, or under State.Mu during registration for literal
tasks. The catalog pointer remains on engine.Task and is read/written only
under State.Mu. Registration sets it and publishes initial settled status;
forget clears it and removes the unfinished entry under the existing order.
The observer never acquires the catalog mutex or runtime mutex. No closure or
extra task allocation is needed, and lower standalone test States can omit it.
The exact lower registration operations are
`BindSettlementObserverLocked(observer SettlementObserver)` and
`SettledLocked() bool`, with `SettlementObserver` declaring
`TaskSettled(settled bool)`. Registration holds State.Mu, binds the containing
Task (also supporting constructor-free literals), installs t.catalog, and calls
noteFinished(t, t.State.SettledLocked()). Forget clears t.catalog and removes
the unfinished entry under that same lock. Binding during fresh construction
needs no lock before the object escapes. Do not expose setters for every
scheduling flag. Promote `readingOrder` to `ReadingOrder` for the moved manager;
keep its own existing lock, since FindReadingTask holds only the catalog lock.

Preserved order: runtime or scheduler/catalog mutex -> State.Mu -> unfinishedMu.
Unfinished takes only unfinishedMu to copy pointers and never takes State.Mu.
Machine getters remain leaf operations. Do not call a normal locking State
getter from inside the combined snapshot read lock: a waiting writer can make
recursive RWMutex reads deadlock. SnapshotLocked must copy fields directly.

Pointer publication is not frame synchronization. Retain physical execution
leases, runtime lock, and quiescence rules around GC and checkpoint capture.
Retain the live VM through suspend/retry exactly where engine/task_runtime.go
currently stores/clears it; only the destination field/type changes.

## Replacing CallingVM without inventing a machine API

Define a profiling-only parent interface in `internal/mooprof` and use it for
`builtins.Execution.ProfileParent` and `vm.profileState.parent`. Four operations
are sufficient to express the current loops without exposing VM:

* `ProfileTime(*mooprof.Session) (time.Time, bool)` reads the baseline only for
  the matching session, for profileEnter (`vm/profile.go:44-54`).
* `ResetProfileTime(time.Time)` updates this VM and all ancestors, matching the
  newly-active-session branch (`:87-94`).
* `ChargeProfileChild(*mooprof.Session, int64, time.Time)` binds a differing
  session, increments ticks, and updates time on this VM and ancestors (`:100-105`).
* `AppendProfileFrames([]mooprof.Frame) []mooprof.Frame` appends this VM's live
  frames then ancestors to one shared result slice (`:116-130`).

Implement these methods on *vm.VM. Bind `execution.ProfileParent = vm` once at
the existing lazy Execution creation. Engine passes that capability directly
to SetProfileParent, without assertion or VM lookup. Preserve nil handling.
Existing task/frame ownership stays concrete; only the pre-existing profiling
callback boundary is represented by its actual consumer operations. No copied
stack, per-builtin closure, new global profiler registry, or new VM host callback.
The interface does not own the machine or execute/suspend/reset it.

A concrete mooprof profile context could hold the session/ticks/time/parent
chain, but live frame access still belongs to vm.StackFrame and its Program.
It would need a bound frame-appender callback, or a second continuously updated
frame representation. The former introduces another closure and requires
careful rebinding in retry copying/pool reset; the latter duplicates state and
adds execution-path work. The four-method contract avoids both changes and
leaves current profileState and retry copying intact. These proposed methods
are new narrow operations extracted from the cited loops, not existing APIs.

## Dependency graph and tests affected

`server -> engine -> vm -> builtins -> task`; engine also imports task and
builtins. task imports bytecode/types/its existing lower dependencies, never vm
or engine. vm and builtins both import internal/mooprof for the profiling
contract. db/format continues importing task snapshot records. The scheduler is
part of engine, not a child package importing its parent.

`builtins/task_manager_test.go:7-23` currently constructs the production lower
Manager inside same-package builtin tests. After moving it up, those tests
cannot import engine without a cycle. Move behavior tests requiring the real
manager to engine, invoke builtins through the existing public Session/Registry
API, and use the production manager. Examples include tasks_authority_test,
task_completion_test and relevant review_io_test cases; compat_sqlite_test and
signatures_test also call the shared wiring helper and need individual review.
Do not copy Manager into test scaffolding or broadly export private builtin
functions. Pure builtin tests keep lower State and existing small host stubs.
The exact split of mixed test files remains implementation work.

## Alternatives, size, and limits

Moving only VM.Task downward does not solve vm -> builtins -> task. Moving all
task state to engine requires a large VM/builtin facade and still causes the
builtin test cycle. Retaining a lower State catalog plus an engine VM/Task map
duplicates ownership and misses direct work unless more lifecycle machinery is
added. Moving the executable owner to vm could avoid relocating the scheduler
into engine, but puts scheduler lifetime/catalog ownership beneath the engine
and still requires the lower State split and manager/test moves. It does not
meet the selected ownership direction more simply.

Estimate: roughly 1,000-1,300 existing manager/scheduler lines relocate; the
roughly 1,100-line task implementation mostly changes receiver/type names.
Expect 150-300 substantive production lines for typed ownership, shared-lock
capture, catalog settlement, and profile methods, plus several hundred
mechanical call-site/test changes across dozens of files. These are planning
estimates, not measured diff counts. This is not a safe ten-line field edit.

Unresolved evidence: no compilation, race run, profile-output comparison,
allocation measurement, or exact test-migration patch has been performed.
Interface dispatch in profiling and the added settlement receiver may have a
small cost; source inspection does not measure it. Struct size/escape behavior
must be measured after implementation despite the one inline task allocation
design. Existing CI, isolation evidence requirements in docs/elle-isolation.md,
and authorized performance comparison apply to the eventual patch; this report
claims no verification result. No host delegation failure was observed by this
agent, and no host activation action was attempted here.
