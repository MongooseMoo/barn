# test/task-retention-roundtrip

## Goal

Preserve accepted queued and suspended task records through the
FuzzReadDatabase round trip so the round-trip oracle checks them. Plan source:
`agent-notes/test-persistence-semantic-roundtrip.md`, section "Next focused
PR: retain parsed tasks in the database fuzz round trip".

## State

- Branch is origin/master (with PR #655 merged) plus one local commit. Not
  pushed; root owns publication, CI and merge. PR body draft: `.tmp/pr-body.md`.
- Changes:
  - `checkpoint_stage.go`: the root-collection / `SnapshotWithRoots` / rewrite
    body of `Checkpoint.Capture` is now `snapshotWithTaskRoots`; Capture and the
    fuzz helper both call it.
  - `reader_vm.go`, `reader_task.go`: reader-built task snapshots set
    `ReadingPlayer: ObjNothing` (see Surprises).
  - `reader_task.go`: `QueuedTask.persistenceSnapshot` re-expresses a parsed
    queued record as the writer's `task.Snapshot` (no compilation).
  - `roundtrip.go`: `persistedState`, `compareRoundTripPersistence` (one
    identity context over store and tasks), `queuedTask`, `suspendedTask`.
  - `reader_fuzz_test.go`: `checkpointTwice` / `captureDatabase` /
    `writeAndParse`; the fuzz body compares tasks too; the task fixture is a
    new seed. `dumpAndReload` (store-only, used by roundtrip_test.go) now
    reuses `writeAndParse`.
  - `task_roundtrip_test.go`: positive fixture, empty-body rejection, and 30
    per-field mutation cases.
- Focused run: `go test ./db/format -run '^(TestTaskRoundTrip(RetainsQueuedAndSuspendedTasks|ReportsUnwritableEmptyQueuedBody)|TestCompareRoundTripPersistenceDetectsTaskMutation|FuzzReadDatabase|TestCompareRoundTripStores(WriterReader|InvalidWaifClass|TrackedDatabases|DetectsMutation))$' -count=1`
  -> `ok github.com/MongooseMoo/barn/db/format 2.183s`. `gofmt -l db/format`
  and `go vet ./db/format` printed nothing; `staticcheck ./db/format` printed
  nothing. Broad tests not run locally (CI).
- Control: with the `reader_task.go` `ReadingPlayer` line removed,
  `TestTaskRoundTripRetainsQueuedAndSuspendedTasks` failed with
  `interrupted task after first reload: id 7, state queued, wake E_INTRPT`;
  restored afterwards.

## Decisions and why

- One shared root walker: Capture's body extracted, not copied.
- Each parsed `*Database` is captured exactly once; `TransformPersistenceValues`
  mutates suspended task slices in place, so the captured value is both written
  and compared and nothing is rewritten twice.
- Queued adapter uses FirstLine 1 and sorted variable names because the reader
  discards the header first line and declaration order. That is a limit of the
  retained representation, not native equivalence; FirstLine is not compared.
- `ReadingPlayer` is fixed in the reader (the owner that builds the snapshot),
  not patched in the test helper.
- Empty queued bodies (reader accepts, writer rejects) are an explicit error
  from `checkpointTwice`, so the fuzzer reports them; nothing is skipped.
- Fuzz still compares the first and second reload (idempotence). Preservation
  from the original input is asserted directly in the positive test (task
  counts, task-only anonymous objects, WAIF sharing across property and both
  tasks, negative start time, case-distinct variable names).
- Comparator families: queued = identity/time, descriptor, source, variables;
  suspended = category, identity/time, wake/task-local, task descriptor,
  activation count (incl. MaxStackDepth), per-frame source/IP/local names,
  locals, stack, call descriptor (incl. per-frame programmer). Uncompared VM
  frame metadata is listed in the `suspendedTask` doc comment; not complete VM
  coverage. Active connections are passed through but not compared.

## Surprises

- Reader-built task snapshots never set `ReadingPlayer` (zero value `#0`).
  `task.Snapshot.IsInterruptedForPersistence` (`task/snapshot.go:125`) treats
  any value other than `ObjNothing` as reading, so a parsed suspended snapshot
  given to `Writer.SetTaskSnapshots` went to the interrupted section.
  Production restore (`engine/task_load.go:49`) does not read the field, so
  only direct reader->writer paths were affected. The pre-existing fuzz seed
  (`reader_fuzz_test.go:55`) also omits `ReadingPlayer`, so its "suspended"
  seed is actually written as an interrupted task (left unchanged).
- `readInterruptedTask` discards the status string and returns a ready
  (`TaskQueued`) task with wake `E_INTRPT` and start `time.Unix(0,0)`; the first
  rewrite moves it to the suspended section with ready=1. Expected transform.
- A queued anonymous alias merge is detected at the suspended task's locals,
  not the queued variable: both anonymous objects have the same shape, so the
  bijection only conflicts when #38 is claimed a second time.

## Next step

- Root reviews, publishes, runs CI.
- Unverified: whether nightly fuzzing will now quickly find accepted inputs the
  writer rejects (empty queued body; VM frame with empty source, which
  `writeVMFrame` rejects). Those surface as fuzz failures by design. Whether
  Toast round-trips an empty fork body is not established; engine drops such
  queued tasks at load without a log (`engine/task_load.go:19`).
