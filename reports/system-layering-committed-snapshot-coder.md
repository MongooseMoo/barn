# Committed snapshot lifetime implementation

## Goal and state

Implement the selected one-owner earlier transaction release, with a meaningful
blocking-output regression. The worktree started clean; source inspection is complete.
Only the main final attempt replay is in scope. Implementation and the focused
RED/GREEN comparison are complete. No publication or merge.

## Evidence

Read the selected scout report, isolation runbook, workload diagnostics, final
runtime path, dispatcher and current native-start closures. Replay consumes
captured IDs/strings/options/operations and host/session owners, not old reads.
The existing physical lease continues protecting VM/root mutations.

The first focused compile probe caught an unused test import, removed before
behavioral execution. It is not the RED witness. The actual RED command under
Debian was:

```text
go test ./engine -run='^TestRunTaskReleasesSnapshotBeforeBlockedCommittedOutput$' -count=1 -v
snapshot retained during committed output: readers=1 history={Objects:1 Entries:1 MostObject:0 MostEntries:1 Floor:2 Clock:3 Readers:1}
FAIL github.com/MongooseMoo/barn/engine 0.036s
```

The real VM publishes write_value=42, calls notify and blocks in the connection's
SendNotification. Publication and incomplete execution are checked before
the failing reader/floor assertion. The test then releases/joins output even
on failure. It reuses existing connection/runtime fixture methods.

## Next step

Parent reviews the scoped diff and sends the independent change through exact-head
PR CI, including the pinned Elle workflow and its raw evidence. No local broad
test or isolation coverage is claimed.

## GREEN and refactor

Moved the same Release call to immediately after ReleaseRetryCheckpoint, before
the final committed/discarded branch. Fresh cleanup-snapshot creation remains
after replay, and gate/physical/admission releases remain where they were.
No other replay site changed and no queue, flag or protocol was added.

```text
go test ./engine -run='^TestRunTaskReleasesSnapshotBeforeBlockedCommittedOutput$' -count=1 -v
--- PASS: TestRunTaskReleasesSnapshotBeforeBlockedCommittedOutput (0.03s)
PASS
ok github.com/MongooseMoo/barn/engine 0.043s
```

The same real-producer test now observes zero active readers and floor equal to
clock while notification output remains blocked. It also verifies publication,
incomplete task execution during the block, exactly one delivered notification,
the unchanged return value and zero readers after completion.

Refactor keeps runtime cleanup registered immediately after construction and
reuses existing connection fixture methods through embedding. The production
comment describes only timestamp/history-floor retention: Release marks the
transaction released and deregisters it, but does not clear its cached object
references while the old pointer remains on the context. No private-root
reclamation, CPU speedup or global commit-progress claim follows.

The gate, VM physical lease and admission remain held across output. Their later
release has independent connection-ordering, suspension-generation and GC-root
requirements, documented by the scout. The focused test does not establish
those broader contracts; the existing required CI coverage remains necessary.
