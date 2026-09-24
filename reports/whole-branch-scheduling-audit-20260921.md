# Whole-branch scheduling audit

Reviewed source baseline `83c3e8d7ef3546ce2ae82d550684790f77706975` through `5729e21c859fb4b6c044bb7ee4930126c7d0a598`, including original branch changes rather than only the later correctness repairs. This analyst reviewed runtime admission, commit gates, scheduling, input processing, task lifecycle, configuration and builtin notification integration. Persistent collections, compiler/store implementation, VM implementation beyond the scheduling boundary, scripts and documentation are assigned to the other reviewers.

## Finding: direct hook tasks have a zero seconds budget (P1)

`engine/call_verb.go:245-251` creates a lightweight `task.Task` without initializing `SecondsLimit` or an execution deadline. At lines 350-354 it attaches that task to the VM but discards the seconds component of `foregroundTaskLimits`. The new `vm/vm.go:344-348` checkpoint calls `Task.SecondsLeft()` every 1024 ticks. `task/task.go:579-585` returns zero for this task, so a direct hook reaching its first checkpoint exits with `E_MAXREC: seconds limit exceeded`, even if it only ran for milliseconds and remains below the configured foreground tick limit.

Affected entrypoints include `Runtime.CallVerb`, `Runtime.CallVerbWithArgstr` (used by out-of-band input), and independently created lifecycle hook VMs using `callVerbWithArgstr`. The regular `runTaskSliceAdmitted` path initializes a deadline and therefore does not exhibit this missing initialization. A deterministic regression should run a finite direct hook with more than 1024 charged ticks, verify its normal return, and assert a positive configured seconds budget. Parent has been notified to reproduce and repair; this analyst has not changed implementation or test files.

## Fresh verification

Command from the correctness worktree, Windows Go race build:

```text
go test -race ./internal/admission ./internal/commitgate ./engine/internal/scheduler ./task ./server ./engine -count=1 -timeout=5m
ok  github.com/MongooseMoo/barn/internal/admission 1.712s
ok  github.com/MongooseMoo/barn/internal/commitgate 1.558s
ok  github.com/MongooseMoo/barn/engine/internal/scheduler 1.719s
ok  github.com/MongooseMoo/barn/task 1.707s
ok  github.com/MongooseMoo/barn/server 2.763s
ok  github.com/MongooseMoo/barn/engine 197.850s
exit_code: 0
```

The passing tests do not cover the direct-hook budget initialization defect above. Reviewed failure-mode tests include cancellation racing admission/grants, checkpoint exclusion, preempted-owner drain, forced-input self-lane flooding and receipt release, physical VM handoff, ready sibling cancellation, asynchronous eval completion and callback reentry, retry boundaries and seconds exhaustion. No additional concrete defect was identified in those reviewed paths. This report is not whole-branch merge approval or remote CI evidence. No manual server, oracle or live-database command was run during this audit.

## Additional finding: binary notify bytes are sent as WebSocket text (P2)

`builtins/network.go:802-809` now decodes binary-mode notifications into arbitrary raw bytes, including invalid UTF-8. `builtinSetConnectionOption` at lines 1277-1282 accepts binary mode on every resolved connection without checking a transport capability. `server/connection.go:94` forwards those bytes to the transport; `server/websocket_transport.go:73-77` unconditionally sends `websocket.MessageText`. A binary notification containing `~FF` therefore reaches the WebSocket writer as text with byte `0xff`, which is not a valid UTF-8 text payload. The new notification regression covers TCP only. This is a transport framing issue, distinct from MOO oracle semantics. The existing WebSocket input path explicitly rejects binary messages, so the repair must preserve or deliberately revise that transport contract rather than infer binary support from TCP. Parent has been notified to validate and repair this boundary.

## Constructor follow-up

Searched executable task creation sites after identifying the budget omission. The only other production `&task.Task{}` in the runtime is `engine/runtime.go:146`, an ID-only GC exclusion marker that is never attached to an executing VM. Queued, resumed, forked, restored and intrinsic eval paths initialize limits through `task.NewTaskFull` and set their execution deadline in `runTaskSliceAdmitted`.

## Repair review: direct hook budget

Independently reviewed the worker repair in `engine/call_verb.go` and new `engine/call_verb_budget_test.go`. The standalone hook path now reads both foreground limits and initializes its budget/deadline using `ResetExecutionBudget` and `SetExecutionDeadline` immediately before execution. The shared-context nested hook path remains unchanged and inherits its parent's budget. Tests exercise direct, argstr, execution-owned and sweep-owned entry, a finite loop exceeding 1024 charged ticks, forced deadline exhaustion, and live/expired nested parent budgets. This resolves the P1 finding above in the reviewed working tree.

```text
go test -race ./engine -run 'TestCallVerbForegroundBudget|TestCallVerbExpiredDeadlineStillStopsExecution|TestCallVerbInContext|TestAdmissionCapOneErrorHookBorrowsReservation|TestAdmissionCapOneCompletionStartsIndependentHook|TestRunTaskRetriesStaleReadsAtIrreversibleEffectInsideNestedVM' -count=1 -timeout=2m
ok  github.com/MongooseMoo/barn/engine 1.246s
exit_code: 0
```

For P2, `spec/server.md:312-331` requires text-only WebSocket output even when the binary connection option is enabled. Parent selected UTF-8 validation before the WebSocket writer with ordinary output-error propagation, preserving that framing contract. That proposed boundary resolves the invalid-frame issue without expanding the protocol; implementation review remains pending.

## Repair review: WebSocket output validation

Independently reviewed the completed repair in `server/connection.go`, `server/transport.go`, `server/websocket_transport.go` and `server/websocket_output_test.go`. WebSocket output validates UTF-8 before calling the writer. `Connection.SendNotification` also uses the transport's optional `OutputValidator` before buffering, preventing an invalid deferred notification from poisoning the queue. TCP does not implement that validator and retains arbitrary-byte output. The WebSocket remains text-only; newline framing and valid Unicode remain unchanged. This resolves the P2 finding above in the reviewed working tree. Both reported source defects are now resolved; full final-source CI and the independent merge gate remain the parent's responsibility.

```text
go test -race ./server -run 'TestWebSocketOutput|TestNotificationBytesAndBufferedOrder' -count=1 -timeout=2m
ok  github.com/MongooseMoo/barn/server 1.145s
REVIEW_RACE_EXIT=0
```

The regressions assert no writer calls for invalid direct/immediate/buffered payloads, no invalid queued output, successful valid output after rejection, preserved buffered ordering/text framing, and unchanged TCP raw-byte output. A production-call search found no remaining `Connection.Buffer` callers in server, builtins or engine; current notifications all use the validated entrypoint.
