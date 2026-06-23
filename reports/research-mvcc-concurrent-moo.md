# Research: MVCC Concurrent MOO

## Summary

Barn can become multi-core without changing MOO language semantics by treating one MOO task slice as an optimistic transaction over the object store. The store supplies consistent versioned reads and records a task-local write set; worker goroutines execute VM work speculatively; commit validates read stability and applies writes at one logical write timestamp. This is closer to optimistic MVCC databases and STM refs than to ad hoc goroutine fan-out. The important Barn-specific constraint is that side effects and task scheduling must remain ordered at commit/yield boundaries, not emitted during speculative execution.

## Approaches Found

### Optimistic MVCC with serializable validation

**Source:** https://www.microsoft.com/en-us/research/wp-content/uploads/2013/06/Hekaton-Sigmod2013-final.pdf

**Description:** Hekaton uses optimistic MVCC to give snapshot, repeatable-read, and serializable isolation. It distinguishes snapshot isolation from serializability and validates read stability before commit; scans need phantom checks.

**Pros:** High concurrency, nonblocking reads, clear serializability story, good fit for object-version records.

**Cons:** Requires read-set tracking, range/scan tracking, abort/retry handling, and version garbage collection.

**Complexity:** High.

### Main-memory MVCC/OCC for OLTP

**Source:** https://arxiv.org/abs/1201.0228

**Description:** Larson et al. compare main-memory concurrency control schemes and conclude multiversion schemes isolate read-only work from updates and are less sensitive to hotspots and long-running transactions than single-version locking.

**Pros:** Directly applicable to Barn's in-memory store; supports long-running read-heavy tasks better than global serialization.

**Cons:** More bookkeeping than single-version locking; hotspots still need contention control.

**Complexity:** Medium to high.

### Cicada-style optimistic multi-version multi-core transactions

**Source:** https://faculty.cc.gatech.edu/~jarulraj/courses/4420-s19/papers/06-mvcc2/lim-sigmod2017.pdf

**Description:** Cicada worker threads run transactions speculatively without eagerly writing shared memory, choose from multiple record versions, then commit with validation and timestamps. It adds multi-clock timestamp allocation, inlining, garbage collection, and contention regulation.

**Pros:** Strong model for actual multi-core scaling; emphasizes avoiding global timestamp bottlenecks and managing abort storms.

**Cons:** Too complex for Barn's first production slice; Barn should borrow the shape, not the full implementation.

**Complexity:** Very high.

### Silo-style OCC and epoch discipline

**Source:** https://wzheng.github.io/silo.pdf

**Description:** Silo uses optimistic transactions with read/write sets and an epoch system. The paper explicitly notes similarity to STM but argues database transactions can avoid tracking irrelevant memory.

**Pros:** Good warning for Barn: transaction tracking should live at store object/property/verb granularity, not arbitrary Go memory.

**Cons:** Silo is single-version OCC, so long read transactions and object scans are weaker fits than MVCC.

**Complexity:** High.

### Clojure STM refs

**Source:** https://clojure.org/reference/refs

**Description:** Clojure refs provide atomic, consistent, isolated transactions, retry conflicts automatically, use MVCC with adaptive history for snapshot isolation, and make all writes appear at one write point.

**Pros:** Useful analogy for interpreter-visible mutable cells; retries are transparent when the transaction has not emitted irreversible effects.

**Cons:** Barn has external side effects, player output, task IDs, fork scheduling, network/file/sqlite builtins, and checkpointing that must be staged or marked non-transactional.

**Complexity:** Medium.

## Recommended Barn Direction

Barn should not start by adding a generic backend or broad adapter. The first production design should extend `db/store` directly with versioned object records and a `StoreTxn` execution context used by the VM. The scheduler should run ready tasks on a worker pool, but every task slice must enter through a transaction boundary.

Use three task modes:

- `read-write tx`: normal MOO VM work, property/verb/object mutations staged in a write set and committed atomically.
- `read-only tx`: lookup-heavy work and command matching, no writes, no commit validation beyond snapshot lifetime.
- `serialized tx`: operations with irreversible effects or global process state until they are explicitly staged, including raw network/file/sqlite side effects, process execution completion, checkpoint finalization, and login/read ordering.

## Barn-Specific Requirements

- `scheduler.ProcessReadyTasks` must stop directly looping over ready tasks and calling `runTask` serially. It should dispatch runnable tasks to bounded workers.
- `runTask` must run a task slice under a transaction context and return an outcome whose side effects are applied after commit.
- `kernel.TaskContext` needs a transaction/store view field so builtins and VM opcodes do not mutate the live store directly during speculative execution.
- Store mutation methods such as `SetPropertyValue`, `SetObjectFlag`, `CreateObject`, `MoveObject`, `ChangeParents`, `Recycle`, `AddVerb`, and `SetVerbCode` need transaction-aware variants or must route through `StoreTxn`.
- Commit validation must begin with object/property/verb version checks. Ancestry scans, command matching, `find_verb`, `FindProperty`, object scans, and max/recycled-object queries need explicit read-set/range-set entries or must stay serialized until covered.
- Output lines, traceback delivery, fork task creation, task manager mutations, finalization, and connection state changes must be buffered as task effects and published only after a successful commit.
- `read()`/`suspend()` boundaries are transaction boundaries: a task must commit or abort before it becomes suspended.
- If a conflict occurs before an irreversible effect, retry the task slice from its saved starting VM/task state. If a conflict occurs after an unstaged irreversible operation, the operation must force serialized mode until it is staged.
- Checkpoint must read a stable store snapshot at a committed timestamp and combine it with immutable task snapshots.

## Complexity vs Quality Tradeoffs

- Minimal worker pool with the current locked store would prove scheduling mechanics but would not remove the serialized store bottleneck.
- Store-level optimistic transactions with coarse object versions can unlock real parallel read and disjoint-write execution while staying understandable.
- Full property/verb subrecord MVCC, scan predicate validation, multi-clock timestamps, and contention regulation can come later if benchmarks show conflicts or timestamp allocation as bottlenecks.

## Estimated Implementation Effort

- **Spike:** 1-2 commits. Add a benchmark/conformance-neutral worker-pool harness and prove current serialization points with metrics.
- **Minimal production:** 6-10 focused commits. Store version stamps, transaction read/write sets, worker pool, effect buffering for output/fork/task state, conflict retry, focused race tests, full managed conformance.
- **Full production:** Multiple workstreams. Subrecord MVCC, scan/range validation, version GC, contention regulation, concurrent checkpoint snapshots, side-effect staging for network/file/sqlite, and benchmark tuning.

## Open Questions

- [ ] What command/output ordering must be byte-for-byte stable when two players issue commands concurrently?
- [ ] Which builtins are safe to stage as pure task effects, and which must initially force serialized execution?
- [ ] Should first commit validation be object-level only, or split property/verb versions immediately?
- [ ] What is the target conflict policy for high-contention verbs: retry, backoff, or fallback to serialized execution?

## References

- Hekaton: SQL Server's Memory-Optimized OLTP Engine, Microsoft Research. https://www.microsoft.com/en-us/research/wp-content/uploads/2013/06/Hekaton-Sigmod2013-final.pdf
- High-Performance Concurrency Control Mechanisms for Main-Memory Databases, Larson et al. https://arxiv.org/abs/1201.0228
- Cicada: Dependably Fast Multi-Core In-Memory Transactions, Lim/Kaminsky/Andersen. https://faculty.cc.gatech.edu/~jarulraj/courses/4420-s19/papers/06-mvcc2/lim-sigmod2017.pdf
- Speedy Transactions in Multicore In-Memory Databases, Tu et al. https://wzheng.github.io/silo.pdf
- Clojure Refs and Transactions. https://clojure.org/reference/refs
