# Checkpoint capture and encoding separation

## Goal

Keep consistent checkpoint capture under the existing runtime barrier while
moving running-checkpoint file encoding outside it. Preserve terminal checkpoint
barrier ownership, frozen identities and publication/recovery behavior.

## State

The implemented six-file candidate was transferred unchanged to the independent
checkpoint-capture worktree; no additional tests ran during that move.

Read the persistence scout, isolation runbook, existing checkpoint stage, store
WAIF freezer, task snapshot transforms, writer ownership and barrier regressions.
The first focused server regression reproduced the encoding barrier wait but
also had a fixture omission: it did not declare the WAIF class's :count field,
which the portable writer needs to emit that property. Corrected the fixture
using the existing task_owned_waif_roots pattern before production edits.
The corrected regression then failed only because admitted Eval could not
commit while encoding was blocked. The six-file phase split and state checks
are now implemented, and the focused regression set passes. Other in-progress
source changes remain outside this slice.

Original corrected RED command in verified Debian WSL, run in the campaign
worktree before this isolation:

```text
go test ./server -run '^TestCheckpointSerializationAllowsCommitAndRetainsFrozenAliases$' -count=1 -timeout=30s
```

Raw corrected RED output:

```text
--- FAIL: TestCheckpointSerializationAllowsCommitAndRetainsFrozenAliases (1.03s)
input_admission_test.go:212: admitted Eval could not commit during checkpoint encoding
FAIL github.com/MongooseMoo/barn/server 1.040s
```

## Decisions and why

Use the existing Checkpoint stage and Writer owners directly. Capture retains
the existing frozen writer; Serialize consumes only that captured writer.
Preserve explicit stage validation, cleanup and errors without a DTO, facade or
compatibility wrapper. Running capture stays inside the barrier; encoding and
publication follow outside. Terminal capture/encoding/publication remain inside.

Capture performs the existing task-root traversal, SnapshotWithRoots freezing,
task rewrite and Writer construction. Checkpoint retains that private writer.
Serialize has no live-state inputs and consumes/releases the writer before
encoding, preserving its identity sidecar data only on success. Close also drops
an unused captured writer. Capture and serialization each reject repeated or
out-of-order calls. File ownership, path reservation, native errors, cleanup-error
joins, publication journals and recovery behavior remain in their existing owners.

The direct WriteCheckpoint caller and server stage interface use Capture,
Serialize and Publish explicitly. Running dumpCheckpoint barriers cover only
task/connection collection and Capture. Terminal checkpoints encode and publish
inside the barrier because already-dispatched background work remains live.
The engine barrier comment now names capture/freezing rather than file encoding.

The probe has distinct beforeCapture and beforeSerialize phases. The existing
task/store consistency test still blocks before Capture after TaskSnapshots;
it was not weakened into an after-capture assertion. The new serialization test
uses a real queued fork with two aliases of a persistent WAIF. While encoding
is held after Capture, admitted Eval commits a scalar and WAIF update. Reload
asserts the captured scalar/WAIF value, original identity and both queued-variable
aliases share the one frozen property WAIF. Existing terminal background hold,
publication progress/failure and failure-hook-after-resume assertions remain.

Stage regressions now cover captured aborts, invalid capture, failed encoding,
Close releasing unconsumed state, Serialize before Capture and repeated/out-of-
order operations, alongside native sync/sidecar failures and recovery preservation.

GREEN command (only selectors from the two edited test files):

```text
go test ./db/format ./server -run '^(TestCheckpointStageAbortReleasesFilesAndPath|TestCheckpointStageRejectsOutOfOrderAndRepeatedCalls|TestCheckpointStageClosePreservesFailedPublicationRecovery|TestCheckpointPreventsExecutionBetweenTaskAndStoreCapture|TestCheckpointSerializationAllowsCommitAndRetainsFrozenAliases|TestCheckpointPublicationAllowsRuntimeCommit|TestCheckpointPublicationFailureRunsFailureHookAfterResume|TestTerminalCheckpointPublicationHoldsBackgroundActivation|TestTerminalCheckpointPublicationFailureReleasesBarrierBeforeFailureHook)$' -count=1 -timeout=60s
```

Raw GREEN output:

```text
gofmt_exit=0
diff_check_exit=0
ok github.com/MongooseMoo/barn/db/format 0.084s
ok github.com/MongooseMoo/barn/server 0.365s
```

Scoped source diff: 198 additions and 33 deletions across the six assigned Go
files. No broad local tests, build, benchmark or Elle checker ran. Focused results
do not establish isolation coverage or measured workload speedup. Parent owns
the independent PR CI and pinned Elle histories/manifests/verdicts.

## Next step

Return the scoped diff for parent source review and independent PR delivery.
No commit, review posting, push or merge was performed during implementation
or isolation. The coordinator owns publication.
