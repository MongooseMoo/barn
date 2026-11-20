# Execution ownership source survey

## State

Read-only scout survey complete for the first bounded P2 slice.
Source subject: Merge pull request #628 from MongooseMoo/test/elle-definitions.
The scout role has no file-writing capability; the coordinator persists its
source findings here as messages arrive. Coordinator source inspection confirmed
the three duplicate blocks and the existing runtime owner.

## Finding: duplicated terminal settlement

Scout reports that engine/runtime.go:650-668 already owns error logging,
suffix/output flushing and terminal-only Done closure. The synchronous command
entry at task_runtime.go:1357-1370 duplicates that block. task_factory.go:141-147
and :415-418 duplicate its successful flush-and-close subset. Existing
settleTaskResult accepts schedulerResult, although settlement's domain inputs
are a Task and an error. The implementation uses that owner directly.

Existing contracts identified by the scout: ready-order flushes and one-time
suffix across suspension in engine/worker_pool_test.go:228-279; callback versus
terminal flush acknowledgement in engine/mongoose_completion_test.go:11-64.
ResumeReadingTask's distinct error log must be retained. No test execution is
claimed by this source survey.

## Bounded handoff

Keep Runtime.settleTaskResult and change its inputs to Task/error. Scheduler
callers unpack their carrier. ExecuteVerbTaskSyncWithStart delegates its exact
logging/flush/Done sequence; successful runServerVerbTask and ResumeReadingTask
delegate the successful sequence, with the reading-task error log preserved.
Keep StartEval, CreateLoginHookTask, sweep hooks, callback roots, lease release,
admission and transaction/effect order unchanged. The superseded surface is the
repeated flush-then-terminal-Done decision in these three entrypoints.
