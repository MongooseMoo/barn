# Barn MVCC Concurrent MOO Plan

## Control Rules

- This plan is the active control surface for the multi-core/MVCC workstream.
- Do not begin production implementation until the research and plan artifacts are committed and pushed.
- Commit and push frequently. Keep each commit scoped to one phase or one measurable implementation slice.
- Do not introduce a generic backend interface, adapter, sender, shim, compatibility layer, or fallback path. Extend the real owners: `scheduler`, `kernel`, `vm`, `builtins`, `task`, and `db/store`.
- Use only the documented managed conformance command for conformance work unless the user explicitly approves a different path.
- Treat `~/code/moo-conformance-tests` as read-only.
- Before declaring completion, run focused tests, `go test` coverage for touched packages, `git diff --check`, and full managed `moo-conformance`.

## Current Baseline

- Branch: `work/mvcc-concurrent-moo`.
- Dependency source is now GitHub-backed: `moo-conformance = { git = "https://github.com/mongoosemoo/moo-conformance-tests" }`.
- `scheduler.ProcessReadyTasks` gathers ready tasks, releases the scheduler mutex, then calls `s.runTask(t)` sequentially.
- Foreground command dispatch and `read()` resume paths still call `ExecuteVerbTaskSync` / `ResumeReadingTask`, which run `s.runTask(t)` synchronously.
- `vm.VM` is task-local and safe to keep single-owner per running task.
- `db/store.Store` owns object internals behind a single `sync.RWMutex`. It already exposes store-owned mutations, but those mutations apply directly to live state.
- `server.checkpoint()` writes `dbformat.NewWriter(tempFile, s.store.Snapshot())` plus immutable `scheduler.TaskSnapshots()`.
- Unrelated local dirt: `pyghidra_mcp_projects/` is untracked and must not be staged.

## Target Architecture

Barn task execution becomes multi-core by running independent task slices on worker goroutines. A task slice is the code executed from start/resume until return, exception, suspend, fork-yield drain, or forced serialization boundary.

Each task slice uses a transaction context:

- `ReadTS`: stable logical timestamp for reads.
- `ReadSet`: object/property/verb records and scans observed by the VM/builtins.
- `WriteSet`: object/property/verb/lifecycle mutations staged by the VM/builtins.
- `Effects`: output, traceback, task state transitions, fork creation, finalization, and connection side effects staged until commit.
- `Mode`: read-write, read-only, or serialized.

Commit:

1. Validate read stability and scan/range predicates.
2. Allocate a write timestamp.
3. Install versions atomically.
4. Publish staged effects in deterministic commit order.
5. Mark task terminal/suspended/queued and notify waiters.

Abort/retry:

- If validation fails before irreversible effects, rebuild the VM/task slice from the captured start state and retry with bounded backoff.
- If a slice touches an operation not yet staged safely, rerun or route it through serialized mode.

## Phase 0 - Plan and Baseline Proof

Status: in progress.

Work:

- Write `reports/research-mvcc-concurrent-moo.md`.
- Write this plan.
- Verify dependency source and CLI resolution:

```powershell
uv run moo-conformance --help
```

- Capture current scheduler/store bottleneck with static evidence.
- Commit and push the plan slice.

Gates:

```powershell
git diff --check -- reports/research-mvcc-concurrent-moo.md plans/mvcc-concurrent-moo-plan.md
git status --short --branch
```

Commit:

- `Plan MVCC concurrent MOO work`

## Phase 1 - Worker Pool Without Semantic Change

Status: pending.

Goal:

- Separate scheduler readiness from task execution mechanics without changing store semantics yet.

Work:

- Introduce a bounded scheduler worker pool owned by `Scheduler`.
- Replace the serial ready-task loop with task dispatch and completion collection.
- Preserve existing synchronous semantics for command dispatch and `read()` resume until transaction/effect staging exists.
- Add scheduler tests proving:
  - worker pool starts/stops cleanly;
  - tasks are not run twice;
  - completion closes `Done` once;
  - output flush remains ordered for one connection.

Expected result:

- No claimed multi-core semantics yet. This is plumbing only.

Gates:

```powershell
go test ./scheduler ./server ./task
go test -race ./scheduler ./server ./task
git diff --check
```

Commit and push.

## Phase 2 - Store Version Stamps and Read-Only Transactions

Status: pending.

Goal:

- Establish the real MVCC read boundary without changing mutating code.

Work:

- Add logical timestamp/version state to `db/store`.
- Stamp object records and key subrecords:
  - object scalar/relationship version;
  - property namespace/version;
  - verb namespace/version.
- Add `Store.BeginReadOnly(readTS)` and `StoreTxn` read APIs for:
  - `ObjectExists`, `ObjectName`, `ObjectOwner`, `ObjectFlags`, `HasObjectFlag`;
  - `FindProperty`, `PropertyValue`, `LocalProperty`;
  - `FindVerb`, `FindParentVerb`, `FindVerbOnObject`;
  - relationship and scan reads used by command matching.
- Keep existing direct store methods as delegating non-transactional callers for code not yet migrated.
- Add unit tests that a read-only transaction sees a stable view while later live writes advance versions.

Gates:

```powershell
go test ./db/store ./vm ./builtins ./scheduler ./server
go test -race ./db/store
git diff --check
```

Commit and push.

## Phase 3 - Transaction Context Wiring Through VM and Builtins

Status: pending.

Goal:

- Make normal VM reads go through the transaction view.

Work:

- Extend `kernel.TaskContext` with the current store transaction.
- Update `scheduler.runTask` to begin a transaction for each task slice.
- Route VM property/verb lookup through transaction-aware reads.
- Route builtin read-only store access through transaction-aware reads where available.
- Add tests proving a task slice has stable reads across concurrent live mutations.

Gates:

```powershell
go test ./kernel ./vm ./builtins ./scheduler ./server ./db/store
go test -race ./kernel ./vm ./builtins ./scheduler ./server ./db/store
git diff --check
```

Commit and push.

## Phase 4 - Write Sets for Core Object/Property/Verb Mutations

Status: pending.

Goal:

- Stage store writes inside a task transaction instead of mutating live state immediately.

Work:

- Implement `StoreTxn` write-set operations for:
  - `SetObjectName`, `SetObjectOwner`, `SetObjectLocationRaw`, `SetObjectFlag`;
  - `SetPropertyValue`, `SetPropertyInfo`, `DefineProperty`, `DeleteDefinedProperty`, `ClearPropertyOverride`;
  - `AddVerb`, `DeleteVerb`, `SetVerbInfo`, `SetVerbArgs`, `SetVerbCode`, `SetVerbCodeByIndex`;
  - `CreateObject`, `MoveObject`, `ChangeParents`, `Recycle`, `Recreate`.
- Preserve store-owned policy and invariants. Do not duplicate relationship/property/verb logic outside `db/store`.
- Add validation at commit for every read record and written record.
- Conflict policy: bounded retry with backoff, then serialized fallback only for the conflicting task slice.
- Add focused concurrent tests:
  - disjoint property writes both commit;
  - same property write conflicts and retries;
  - verb code update invalidates a concurrent caller's read if needed;
  - create/recycle/move conflicts preserve object graph invariants.

Gates:

```powershell
go test ./db/store ./vm ./builtins ./scheduler ./server
go test -race ./db/store ./vm ./builtins ./scheduler ./server
git diff --check
```

Commit and push.

## Phase 5 - Effect Buffering and Deterministic Publication

Status: pending.

Goal:

- Prevent speculative execution from leaking output or task side effects before commit.

Work:

- Add a task effect buffer for:
  - player output and command output suffix flushes;
  - traceback delivery;
  - fork task creation;
  - task manager registration/removal;
  - task state transitions;
  - pending finalization values;
  - connection/login completion callbacks.
- Publish effects only after transaction commit.
- Keep `read()` and `suspend()` as transaction boundaries.
- Initially classify unsafe external side effects as serialized:
  - `exec()`;
  - file I/O;
  - network/listener/HTTP held-input state;
  - sqlite handles;
  - server checkpoint/shutdown hooks.

Gates:

```powershell
go test ./task ./scheduler ./server ./builtins ./vm
go test -race ./task ./scheduler ./server ./builtins ./vm
git diff --check
```

Commit and push.

## Phase 6 - Enable Concurrent Ready-Task Execution

Status: pending.

Goal:

- Run independent ready task slices concurrently with MVCC commit validation.

Work:

- Remove the remaining serial `ProcessReadyTasks` execution loop for transaction-safe task slices.
- Use worker goroutines for ready tasks.
- Keep per-connection command/read ordering where required by protocol semantics.
- Add runtime metrics:
  - worker count;
  - committed task slices;
  - retries;
  - serialized fallbacks;
  - validation failures by cause.
- Add a stress test that proves overlapping CPU-bound tasks execute concurrently and commit correctly.

Gates:

```powershell
go test ./scheduler ./server ./db/store ./vm ./builtins
go test -race ./scheduler ./server ./db/store ./vm ./builtins
git diff --check
```

Commit and push.

## Phase 7 - Checkpoint, Version GC, and Restart Semantics

Status: pending.

Goal:

- Make persistence observe a committed, stable world without blocking all execution longer than necessary.

Work:

- Make `Store.Snapshot()` choose a committed timestamp and retain needed versions until the snapshot completes.
- Combine snapshot timestamp with immutable `Scheduler.TaskSnapshots()`.
- Add version garbage collection once no active transaction or checkpoint can see old versions.
- Add tests for checkpoint while workers are running.

Gates:

```powershell
go test ./db/... ./scheduler ./server ./task
go test -race ./db/... ./scheduler ./server ./task
git diff --check
```

Commit and push.

## Phase 8 - Benchmarks and Full Conformance

Status: pending.

Goal:

- Prove Barn is both conformant and actually concurrent.

Work:

- Add or extend benchmarks that compare:
  - serial baseline;
  - concurrent disjoint property writes;
  - concurrent read-heavy verb calls;
  - conflict-heavy same-object writes;
  - checkpoint during workload.
- Run managed conformance from this checkout:

```powershell
go build -o barn.exe ./cmd/barn/
uv run moo-conformance --server-command "C:/Users/Q/code/working/barn/barn.exe -db {db} -port {port}"
```

- If the managed command fails to start/connect, report exact command, expected result, and actual failure before improvising.

Final gates:

```powershell
go test ./...
go test -race ./db/store ./scheduler ./server ./task ./vm ./builtins
go build -o barn.exe ./cmd/barn/
uv run moo-conformance --server-command "C:/Users/Q/code/working/barn/barn.exe -db {db} -port {port}"
git diff --check
git status --short --branch
```

Commit and push.

## Completion Criteria

- Every phase is complete or explicitly deferred by the user.
- Ready task slices can run on multiple goroutines.
- Store reads and writes for normal MOO execution use transaction boundaries.
- Conflicting writes validate, retry, or serialize without corrupting store state.
- Speculative output and task side effects do not leak before commit.
- Checkpoint/restart observes committed state.
- Full managed `moo-conformance` passes from this checkout.
- The branch contains pushed commits for the completed slices.
