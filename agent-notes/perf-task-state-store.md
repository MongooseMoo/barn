# perf/task-state-store

## Goal
Build Q's approved design "PLAN: TASK STATE IN THE STORE"
(agent-notes/mongoose-workload-gap.md on master): one versioned row per task
in the MVCC store (owner, status, pending resume value); kill_task and resume
become plain builtins that read the row and stage a write; two killers are an
ordinary MVCC conflict; the target's own transitions are row writes in its own
commits; committed rows are applied to the scheduler by version.

## State
Built on perf/task-state-store. Targeted tests, race run, vet, gofmt and
staticcheck on the changed packages pass. Perf gate passed (below). Managed
conformance runs in the PR's CI.

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
- The hot-read renewal (same transaction, new snapshot, no commit) carries
  task-row reads. CommitAndRenew does NOT: a commit boundary ends them, like
  object reads. The slice re-reads its own row just before staging its end
  row, so a kill that landed after a mid-slice boundary still stops it.
  (The first version carried them; PR #604 CI failed 3 conformance tests:
  fork_timing suspend_zero_queued_tasks_lists_child_suspended_first and two
  waif.yaml queued_tasks/task_stack tests. recycle()'s beginIrreversible
  boundary published the parent's kill of its child, the child settled and
  its row was removed outside any transaction, and the parent's final
  commit, holding the exclusive gate, lost on the carried read -> E_INVARG.
  Regression test engine/task_rows_conformance_test.go.)
- tasks.mu is taken only from row validation (after the commit/validated
  and commit/timestamped probe points, before any publication) through row
  publication. The first version took it before object validation, which
  re-serialized every commit and made Elle's fault-commit-exclusion negative
  control come out serializable (CI "Serializable histories" red).
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

## Perf gate (2026-11-04, vs master at the #603 merge, idle-gated, interleaved)
Measured on the first version, before the two CI fixes above (they add one
row lookup per slice end and narrow the tasks.mu span); a 3-round re-check
on the fixed branch follows CI.
- 1p x8 goodput/s: base 50 50 50 48 50 52 52 52 (mean 50.5); cand 51 50 50
  47 52 51 53 49 (mean 50.4). Equal.
- 16p x3 goodput/s: base 329 342 338 (mean 336); cand 346 344 364 (mean
  351). p99 736-749 ms base vs 747-793 ms cand, p999 1.28-1.44 s vs
  1.24-1.32 s: within run-to-run spread.
- Exclusive gate holds, 1p: count 109-120 -> 9-13, total ~330-400 ms ->
  ~295-350 ms (the remaining holds have other causes).
- The harness abort% fell 28.6% -> 11.6% at 16p, but that is the
  denominator: abort% = retries / store.CommitAttempts, and every slice now
  commits its end row. Absolute 16p retries: base 4796/4843/5056, cand
  4961/5088/5195, in proportion to goodput. Not an abort reduction.
- Pre-existing Toast divergences seen while checking, not changed here:
  resume() reaches a read() task (Toast E_INVARG, it is not on
  waiting_tasks), and kill/resume perms use the task owner, not
  progr_of_cur_verb (tasks.cc:2783-2826).

## Next step
Managed conformance via the PR's CI; merge when green.
Known gaps: Runtime.KillTask / Runtime.ResumeTask (task_factory.go) were
already uncalled before this branch and still act on memory only; left as is.
