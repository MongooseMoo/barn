# Task-row shared gate wait accounting

## Goal

Correct the omitted shared commit-gate counters and wait observer on task-row-only
commits, preserving gate ordering and publication semantics.

## State

Read AGENTS.md, docs/elle-isolation.md and both commit paths. New focused real
task-row regression fails before implementation:

```text
task_rows_only: count=0 total=0s callbacks=0; want one positive wait
```

The first probe's immediate grant release also produced zero duration on the
ordinary path because of Windows clock resolution. The regression now retains
the proven queued waiter for 5ms before release. Parent owns engine/metrics changes.

## Next step

Implementation complete. Both commit paths call acquireSharedCommitGate, which
contains the existing acquisition, counters and callback once. Gate exemption,
error handling, lock order, validation and publication code are unchanged.

Focused GREEN:

```text
go test ./db/store -run '^(TestTaskRowSharedGateWaitAccounting|TestTaskRowExclusiveGrantDoesNotAccountSharedWait)$' -count=1 -v
PASS
ok github.com/MongooseMoo/barn/db/store 1.529s
```

The real task-row-only and object-writing paths each account one positive wait
after the held exclusive grant releases; the observer matches the counter.
Exclusive-owner commits account no shared acquisition. git diff --check emitted
no output. No full suite, CI, performance, isolation or publication verdict.
The same focused selector with the race detector also completed:

```text
go test -race ./db/store -run '^(TestTaskRowSharedGateWaitAccounting|TestTaskRowExclusiveGrantDoesNotAccountSharedWait)$' -count=1 -v
PASS
ok github.com/MongooseMoo/barn/db/store 2.722s
```

Next: parent engine integration and scoped review. No commits or publishing.
