# fix/checkpoint-callback-flake

## Goal
Merge-queue CI run 42326068784 failed
`server/checkpoint_failure_callback_context.yaml::checkpoint_failed_open_calls_finished_zero_then_recovers`
once: the step `#0.cb_events = {}; return dump_database();` returned the traceback
`#2:eval (this == #2), line 1:  Invalid argument` instead of 0. The same binary passed the same
suite in another run, so the failure is nondeterministic. Find the cause and fix it in Barn.

## State
Fix and deterministic Go test written; PR open against master. Managed Toast/Barn repetitions are
recorded below.

## Cause
- `builtins/system.go` (before this change) builtinDumpDatabase called `host.Checkpoint`
  mid-task, and the builtin was registered `CommitGateReentrant`, whose wrapper
  (`builtins/registry.go`) sets `ctx.IrreversibleSideEffect = true`.
- `server/server.go` host.Checkpoint is `requestCheckpoint`, a non-blocking send; mainLoop runs
  `s.checkpoint()` at once, and `checkpointWith` calls `#0:checkpoint_started` through
  `RunServerVerbTask` before the checkpoint barrier. In the test, that hook appends to
  `#0.cb_events` and commits.
- If the hook commits first, the caller's commit fails read-set validation with E_INVARG.
  `engine/task_runtime.go` retries such a conflict only when `!ctx.IrreversibleSideEffect`; with
  dump_database's flag set it surfaces E_INVARG as the task result ("commit conflict not
  retryable; surfacing as task error"). One frame, line 1: the CI symptom.
- Toast (`server.cc` bf_dump_database) only sets `checkpoint_requested`; its main loop runs
  `checkpoint_started` and the dump after the requesting task returned. So in Toast the hook always
  follows the caller's writes.

## Decisions and why
- dump_database now queues the request as a commit-deferred effect (PendingEffectAsyncStart,
  flushed after the commit), requesting at once only on a direct transaction (dbtool eval), the same
  shape as `builtins/threaded.go`. It is Transactional: a lost commit is retried and the queued
  request is discarded with the attempt.
- Not `Irreversible`: that holds the exclusive commit gate for the rest of the caller's slice and
  the hook would still start early and lose.
- The `CommitGateReentrant` exemption (85b06d31) existed because the checkpoint then ran on the
  calling goroutine; 835b542c moved it to the server loop. shutdown keeps the exemption.
- The "CHECKPOINTING: dump_database() requested" log line (asserted by
  server/dump_database.yaml) is emitted when the request is sent, text unchanged.
- `builtins/catalog_contract_test.go` hash updated with a comment, as for kill_task/resume.

## Surprises
- The CI artifact keeps only the last 10 Barn server logs; the failing server's log was rotated
  away, so the conflict WARN line could not be read from CI.
- On the Windows workstation `go test ./internal/app` fails the two TestGitImplementationRef*
  tests with or without this change.

## Verification
- `engine/dump_database_order_test.go` TestDumpDatabaseRequestFollowsCallerCommit: on unfixed
  master it failed 3/3 (2x E_INVARG task result, 1x request seen before the caller's write);
  with the fix it passes 5/5.

## Next step
Watch PR checks; record managed repetition counts.
