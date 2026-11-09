# Command settlement consolidation

## Goal

Use the existing Runtime.settleTaskResult owner for the three duplicated
command-output settlement blocks, with task/error inputs rather than a
scheduler result carrier. Preserve all current lifecycle, logging and error
semantics.

## State

Implementation is complete within the assigned source scope. No local builds
or tests were run; behavior and compilation remain for the parent PR CI flow.

## Ownership

Only engine/runtime.go, engine/task_runtime.go, engine/task_factory.go and
this report are assigned to this coder. The living plan belongs to the parent.

## Changes

- Runtime.settleTaskResult now receives the task and error directly. The two
  scheduler callers unpack their schedulerResult at that boundary.
- ExecuteVerbTaskSyncWithStart calls that existing owner instead of repeating
  its error log, output flush and terminal-only Done closure.
- runServerVerbTask retains its early error return and calls settlement only
  after a successful invocation.
- ResumeReadingTask keeps its distinct task-resume-error log and delegates
  the subsequent flush and terminal close with a nil error argument.

No admission, execution lease, callback, retry or effect ordering code moved.
StartEval, CreateLoginHookTask and the sweep-owned hook path were not changed.
No new abstraction, adapter, test, type or state enum was added.

## Inspection evidence

Commands run from the worktree:

```text
gofmt -w engine/runtime.go engine/task_runtime.go engine/task_factory.go
gofmt exit: 0
git diff --check -- engine/runtime.go engine/task_runtime.go engine/task_factory.go
diff check exit: 0
```

The exact source diff was inspected after editing and formatting:

```text
engine/runtime.go      | 11 +++++------
engine/task_factory.go | 10 ++--------
engine/task_runtime.go | 14 +-------------
3 files changed, 8 insertions(+), 27 deletions(-)
```

The caller search reports the owner and five callers:

```text
engine/runtime.go:610: s.settleTaskResult(result.Task, result.Err)
engine/runtime.go:646: s.settleTaskResult(result.Task, result.Err)
engine/runtime.go:650: func (s *Runtime) settleTaskResult(t *Task, err error)
engine/task_factory.go:144: s.settleTaskResult(t, nil)
engine/task_factory.go:412: s.settleTaskResult(t, nil)
engine/task_runtime.go:1358: s.settleTaskResult(t, err)
```

Searching engine for s.flushTaskOutput(t) reports only the existing owner at
runtime.go:659. No commit, push, PR, review, comment or merge was performed.

## Next step

Parent review, analyst/verifier stages and exact-head PR CI. No architectural
objection or implementation deviation was found in this bounded reduction.
