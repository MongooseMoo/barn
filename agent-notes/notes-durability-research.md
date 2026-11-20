# Durability research notes (2026-09-23)

## Question
Should Barn use SQLite (or its B-tree/pager) as a storage engine? If not, what is the right
way to get finer-grained durability than periodic full checkpoints — and how do suspended
tasks (VM continuations saved in the checkpoint) stay consistent with any journal replay?

## Current position (from reasoning, before literature came back)
- SQLite as primary engine: no. The hot path is ns-scale in-memory property/verb reads.
  SQLite allows one writer at a time, which would serialize Barn's per-object optimistic
  commits. MOO values are opaque blobs to SQL. The Toast format must stay regardless.
- Candidate: in-memory MVCC stays authoritative, plus a write-behind commit journal, with
  checkpoint as compaction. Open problem: suspended tasks restored from the checkpoint vs
  object changes replayed from the journal.

## Observed in Barn/Toast (scout survey; full report in session scratchpad
## barn-toast-persistence-survey.md)
- suspend()/read() commit the slice's writes. Resume starts a fresh snapshot
  (engine/task_runtime.go:533-644). Suspended tasks hold no uncommitted writes.
- Suspended state is plain Go data: task.Task with *vm.VM; objects referenced by id.
  Serialized via task/snapshot.go.
- Checkpoint = stop-the-world barrier for the whole snapshot, write, fsync and rename
  (engine/admission.go:38-49, server/server.go:385-420, db/format/checkpoint.go).
  Mongoose dump_database() send until .new exists: Barn 2,341 ms, Toast 9,429 ms (Toast
  forks, so it doesn't pause). The barrier hold time alone has not been measured.
- Commit write set = unordered Go maps, cleared after apply (store_txn_commit.go,
  store_cow.go). No serialized record. Timestamps come from a global atomic clock, but
  commits touching disjoint objects run in parallel.
- Durable mutations OUTSIDE transactions:
  - direct-store builtins: create, recycle, chparent, renumber, move, add_verb, set_verb_*
    (builtins/store_reads.go)
  - reset_max_object
  - the pending-finalization queue
  - the task queue and set_task_local
  - connections
  - the WAIF identity sidecar
  A journal must capture all of these.
- Toast: forked checkpoint, no journaling. Tasks blocked in read()/exec are restored with
  E_INTRPT. Old connections get user_disconnected.
- Nothing in plans/, notes/ or docs/ covers journaling/WAL/crash recovery.

## MMDB recovery literature (done; scratchpad/research-mmdb-recovery.md)
- Consensus design: a redo-only log of committed write VALUES, group commit, periodic
  checkpoints, and truncation of the log at the checkpoint.
  Sources: SiloR OSDI'14 (40 ms epochs, fuzzy checkpoint), Hekaton SIGMOD'13, Redis AOF.
- Command logging (VoltDB) requires deterministic transactions, so it is ruled out for MOO.
  PACMAN SIGMOD'17 says the same for ad-hoc transactions.
- With MVCC, a consistent checkpoint at timestamp T comes nearly free (CALC SIGMOD'16).
  That means no global barrier is needed for the data.
- No DB engine persists in-flight computations. KeyKOS journals data between checkpoints
  but forbids journaling process state, and leaves the desync to the application.
- Researcher recommendation: put task-table changes (add/delete a suspended/queued task)
  in the SAME log record as the slice-ending commit. Replay then keeps objects and tasks
  consistent.
- Open questions:
  - Mongoose suspend frequency, which sets the cost of logging continuations.
  - A total commit order: Barn's parallel commit path only orders per object.

## Persistent-processes literature (done; scratchpad/research-persistent-processes.md)
- Snapshotting processes and data together at one instant is correct, but history puts
  the practical floor at minutes: KeyKOS 5 min; EROS; Eumel/L3 "fixpoints" 3 min;
  Napier88 stabilize. Toast is this design too.
- Journaling data between checkpoints without process state forces the application to
  deal with the inconsistency: KeyKOS, Grasshopper.
- Argus: processes are never durable. A crash aborts in-flight actions; only committed
  stable data survives.
- Temporal, DBOS and Restate commit the continuation state and the data in ONE atomic
  record/transaction.
- mooR: suspended tasks go to a separate tasks.db, saved after the world commit; world
  flush is async without fsync; read() tasks are not persisted. Likely inconsistent after
  a crash (inferred, not tested). Not a model to copy.
- DGD dumps only between tasks and saves only call-outs, never stacks.

## Synthesis
Single redo log of values. Each record = one MOO commit: object writes plus that task's
transition (suspended+VM / forked / finished / killed / reading, where reading is
restored as E_INTRPT like Toast). Checkpoints come from the MVCC snapshot at timestamp T,
tagged with T; truncate the log at T.
Open items:
- total commit order for the logger (SiloR epoch approach: replay by timestamp within
  an epoch)
- all the paths that change durable state outside a transaction
- suspend(0) cost: a decision for Q
- does the crash-loss window matter enough to justify this?

## Previously in flight
- Researcher: persistent processes (EROS/KeyKOS, PJama, Argus, Temporal/DBOS, DGD,
  ColdMUD, mooR) -> scratchpad/research-persistent-processes.md

## Next step
When both reports land, synthesize a recommendation for Q.
Likely shape to test against the evidence: treat task suspend/resume/fork/kill as logged
events in the same journal as object writes, so replay reconstructs a consistent world.
Alternatively, cheaper consistent snapshots (fork-style / COW snapshot off the MVCC store,
no stop-the-world) taken more often.
No code changes yet; no blockers.
