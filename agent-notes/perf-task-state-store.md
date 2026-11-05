# perf/task-state-store

## Goal
Build Q's approved design "PLAN: TASK STATE IN THE STORE"
(agent-notes/mongoose-workload-gap.md on master): one versioned row per task
in the MVCC store (owner, status, pending resume value); kill_task and resume
become plain builtins that read the row and stage a write; two killers are an
ordinary MVCC conflict; the target's own transitions are row writes in its own
commits; committed rows are applied to the scheduler by version.

## State
Built and committed on perf/task-state-store (not pushed). Targeted tests,
race run, vet, gofmt and staticcheck on the changed packages pass. Not yet
measured: the perf gate (harness 1p x8 + 16p x3 vs master) and managed
conformance belong to the lead.

What exists:
- db/store/store_task_rows.go: TaskRow / TaskRowStatus (Finished = no row,
  Running, Suspended, Queued, Killed), the store's row table under a leaf
  lock, StoreTxn.ReadTaskRow / StageTaskRow, validation (ConflictTask in the
  census) and publication, Store.PutTaskRow / RemoveTaskRow / TaskRow, and a
  task-row observer called after a publishing commit releases its locks.
- engine/task_rows.go: Runtime.applyTaskRows (the observer: kill / ready /
  replace value, by version), readOwnRow, ownEndRow, ownRowKilled.
- Each slice attempt reads its own row (killed -> stop; queued -> its value
  is what suspend() returns) and stages its end row (suspended, running for
  exec(), or finished) in the slice's main commit. A validation loss with the
  own row killed stops the task.
- Registration (TaskManager.registerTask) publishes a row; a fork child's row
  is staged in the parent's transaction instead. Settlement removes a task's
  row (TaskManager.noteFinished).
- kill_task / resume are Transactional builtins; the catalog contract hash in
  builtins/catalog_contract_test.go was updated for that.

Deleted: TaskManager.KillTask / CheckKill / killable / ResumeTask /
SetResumeScheduler, builtins.TaskController, State.ResumeFrom /
ReplaceWakeValue (replaced by State.ResumeAtRow / KillAtRow), scheduler.Resume
(replaced by ResumeAtRow), the beginIrreversible use in kill_task, and the
interface stubs in builtins/session_test.go and vm/session_test.go.

## Decisions and why
- Row versions come from a per-store counter, not the commit clock, so a
  direct insert (task registration) never collides with a reserved commit
  timestamp; only per-row monotonicity matters.
- "Finished" is the row's removal. The final commit stages removal; a task that
  settles without a successful commit (panic, terminal commit failure, kill)
  has its row removed when it settles, so rows never outlive their tasks.
- Rows are read at latest and validated by version at commit.
- A commit whose only writes are task rows validates only its task-row reads
  (still under the shared commit gate). A slice that wrote nothing used to
  commit nothing and so never validated its object reads; validating them now
  would make every read-only command able to lose, which the plan did not
  ask for. A commit that also writes objects validates everything.
- Renewal (CommitAndRenew, the hot-read renewal) carries task-row reads at the
  versions its own commit published, so a kill that lands after a mid-slice
  boundary still fails the slice's final commit.
- The scheduler applies a published row only when its version is newer than
  the version the task last saw (State.rowVersion); the task's own slice
  records the version it read, so a late application of an older row is
  skipped.
- resume follows Toast do_resume (tasks.cc:2783-2826): suspended -> queued
  with value; queued -> replace value; anything else (running, fork waiting,
  exec, self) E_INVARG, and E_PERM only for a row it would act on.
- Fork: the child's row goes in the parent's commit, but the eager catalog
  registration and the CreatedForks / discardCreatedForks undo stay. queued_tasks()
  reads the catalog (out of scope) and must list the child within the parent's
  own slice (conformance fork/fork_observation.yaml fork_creates_task; Toast
  enqueue_waiting at fork, tasks.cc:1258-1337), so the child must be in the
  catalog before the parent commits, and a discarded attempt must take it out.
- The GC-sweep task in callVerbWithArgstr (registered, has a row) reads its row
  at registration and stages its end row before its commit, like a slice.

## Surprises
- The fork-row test passed before the parent-commit change, because eager
  registration already published a row; it needed a probe from another task
  during the parent's slice to tell the two apart.
- TestRenewalAccountsForEveryTxnField pins every StoreTxn field's renewal
  disposition; taskReads carried, taskWrites declines.

## Next step
Lead: perf gate (harness 1p x8 + 16p x3 vs master) and managed conformance.
Known gaps: Runtime.KillTask / Runtime.ResumeTask (task_factory.go) were
already uncalled before this branch and still act on memory only; left as is.
