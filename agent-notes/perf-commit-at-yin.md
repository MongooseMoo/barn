# perf/commit-at-yin

## Goal
Q-approved plan ("Make it so!", 2026-11-03): a yin() on a task's root VM
(ctx.CanSuspend) after at least yinCommitTicks ticks in the current slice
yields for 0 seconds, so long yin()-calling commands (#1:forbids,
#2700:_handlers under say/look) commit there instead of staying one huge
optimistic transaction. Toast's yin() can always yield, so yielding more often
is Toast-legal. The low-budget branch of builtinYin is unchanged.

## State
On perf/commit-at-yin:
- builtins/tasks.go builtinYin: no-yield branch yields 0 seconds when
  ctx.CanSuspend && t.Kind != TaskForked && t.TicksLimit-ctx.TicksRemaining
  >= yinCommitTicks (const 10000).
- engine/yin_commit_test.go: yield past the threshold (fork runs first), none
  below it, none for a forked task, none in a nested VM, pre-yin write
  published while queued, conflict after the yin re-runs only from the yin.
  Mutations caught: CanSuspend, forked clause, >= threshold, slice-ticks
  measure, threshold raised to 1e8 (3 tests fail) and lowered to 1 (below-
  threshold test fails).
- The experiment knob (YIN_COMMIT_TICKS / BARN_MONGOOSE_YIN_COMMIT_TICKS) used
  for the gate is removed.

### Gate (TestMongooseRealWorkload, census off, 1p+16p, 3 rotated rounds)
- First build (forks yield too): 16p 347-390/s vs master 131-142, but 1p
  23-27 vs 60-68: the database's forked background jobs (#1007:s_run
  workers, #18714:power_followup_queue) split at every yin overlapped and
  lost to each other (1p census: 2139 retries, 91% of 259M ticks wasted;
  master 8 retries, 41M ticks).
- Forks excluded (this build): 16p K=10000 211, 239, 243/s vs master 133,
  138, 154; K=100000 168-177. 16p unsettled commands 0 (master 2-3 per run,
  2 s drain). 1p K=10000 50, 54, 53 vs master 58, 58, 63 (about -9%).
  p99 16p about master's (1.0-1.2 s). K=1000 was worse in the first build.
- Follow-up: recover the forked share of the gain without the cron thrash.

### queued_tasks() lists running background tasks (team lead's follow-up)
The 1p K=10000 run showed E_RANGE in #1007:reset_scheduler line 9 and
#13232:settle line 109: a task running in another slice vanished from
queued_tasks(). Toast (tasks.cc bf_queued_tasks ~2496-2590) lists reading
tasks, every first_bg task (forks and resumed suspended tasks ready to run),
waiting_tasks and external queues; not first_input, not the caller. Committed:
- task.State.RunningInBackground: Status running and (Kind TaskForked or
  resumed). resumed is set by NoteResumedSlice, called where a slice resumes a
  yielded VM (engine/task_runtime.go, the background-budget reset).
- GetQueuedTasks adds those; builtinQueuedTasks and countBackgroundTasksFor
  skip the calling task (isCallingTask).
- Reported start and sort key: WakeTime when set, else StartTime
  (queuedStartLocked). Toast start_tv: fork = now+delay (double_to_start_tv),
  suspend(n) = now+n (enqueue_suspended_task), resume() = gettimeofday
  (do_resume). Barn resets StartTime at each resumed slice, so StartTime was
  wrong for running and woken-queued tasks.
- A running task's verb/line/this come from startFrame, the top activation
  captured in setStateLocked on entry to TaskRunning: the VM pushes callee
  frames onto CallStack while running (vm/op_verb.go PushFrame), Toast lists
  the queued activation (list_for_forked_task / list_for_vm).
- engine/queued_tasks_running_test.go: running fork listed with its queued row
  and unlisted after completion; running resumed task listed with its queued
  row (1.1 s after the yield); command first slice unlisted; caller unlisted
  (list and count). Seven mutations, each caught.
Known divergences not fixed: resume() of a running resumed task returns
E_INVARG (Toast do_resume replaces the value of a bg TASK_SUSPENDED and
returns 0; the value cannot reach a VM already past suspend()); kill_task of a
running listed task cancels its slice but whether that slice's writes commit
is not verified (Toast's bg task never runs); unstarted queued command tasks
are still listed (Toast's first_input is not); resume_task (read completion)
gives Toast start 0, Barn reports StartTime; num_bg_tasks counting is not
tested.

## Decisions and why
- Ticks in slice = t.TicksLimit - ctx.TicksRemaining. The root VM's TickLimit
  is always set from t.TicksLimit (engine/task_runtime.go launchSlice) and
  vm.Ticks is 0 at a slice's start (reset at resume, task_runtime.go ~350);
  ctx.TicksRemaining = vm.TickLimit - vm.Ticks (vm/vm.go syncContextTicks),
  refreshed before every generic-path instruction, which includes builtin calls.
- Nested-VM test uses a test builtin calling ctx.Session.CallVerb, the real
  nested-VM path; create()'s :initialize now runs on the calling VM
  (vm.pushInitialize), so it no longer exercises a nested VM.

## Surprises
- The brief's fixtures newReadCheckpointFixture/directWrite/read_checkpoint_test.go
  do not exist; used bumpReadValueLiveOnce and testBuiltinSlot instead.
- Without the CanSuspend guard, a suspend from the nested VM does not just get
  dropped: the outer task ended its slice queued.

## Next step
PR; conformance on CI must confirm the queued_tasks() start-time change for
woken queued tasks (not run locally). Merge when green.
