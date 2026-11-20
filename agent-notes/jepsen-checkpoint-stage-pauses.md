# Jepsen checkpoint stage pauses

## Goal
Implement plan section 2 (`plans/jepsen-expansion.md`, "precise narrow boundaries") Go side:
named checkpoint pause points at each interruption-table boundary, plus an
`elle`-tagged test-only startup callback (`BARN_CHECKPOINT_STOP=point:N`) that
writes a flushed boundary record and SIGSTOPs its own process. Normal builds
compile it out.

## State
- Read: plan section 2, `internal/isolationprobe/{enabled,disabled}.go`,
  `server/server.go` checkpointWith/dumpCheckpoint, `server/signals_unix.go`,
  `db/format/checkpoint_stage.go`, `db/format/checkpoint_publication.go`,
  `db/format/checkpoint_recovery_test.go` (interruption helper),
  `cmd/barn/main.go`, `.github/workflows/elle.yml` (runs on every PR).
- Implemented. Points (names in `internal/isolationprobe/checkpoint.go`):
  `checkpoint/capture` (Checkpoint.Capture after the writer is built),
  `checkpoint/serialize` (Serialize after WriteDatabase),
  `checkpoint/staged` (Publish after temp sync + sidecar hash, before journal
  preparation), `checkpoint/journal-published`, `checkpoint/database-renamed`,
  `checkpoint/pair-renamed` (publishHashedCheckpointPair),
  `checkpoint/recovery-database-restored` (recoverCheckpointPair between the
  restore renames, only when the saved pair has a sidecar).
- Startup: `cmd/barn/main.go` calls `isolationprobe.InstallCheckpointStop(os.Stderr)`
  after logging setup; no-op in normal builds.
- Tests pass on Windows and in WSL Linux: db/format
  `TestCheckpointPausePointsAtPublicationBoundaries`,
  `TestCheckpointPausePointBetweenRecoveryRestoreRenames` (elle); isolationprobe
  `TestCheckpointStopRejectsInvalidSelections`,
  `TestCheckpointStopStopsOnlyAtSelectedOccurrence`,
  `TestInstallCheckpointStopLeavesFaultsInactive` (elle),
  `TestCheckpointStopRecordsThenStopsOwnProcess` (elle, linux: real SIGSTOP,
  /proc state T, SIGCONT resumes), `TestNormalBuildHasNoCheckpointStop` (!elle).
- `go build`/`go vet` pass with and without `-tags elle`; linux elle build with
  CGO_ENABLED=0 compiles.
- Docs: paragraph in `docs/elle-isolation.md`.
- CI caveat: ci.yml runs `go test ./...` untagged and elle.yml runs only
  `TestElleRecorder`, so the elle-tagged tests above do not run in CI. Workflow
  changes are out of scope; recorded here for the harness PR.

## Decisions and why
- Pause points live in `db/format` (Capture, Serialize, Publish,
  publishHashedCheckpointPair, recoverCheckpointPair), not `server/server.go`:
  every boundary in the interruption table is a call inside those functions,
  and the format tests can reach them in-process without a server.
- `isolationprobe.Pause` gains variadic artifact paths; `SetPause` callbacks
  receive them. Store callers are unchanged; the two engine test callbacks take
  the extra argument. Point names are constants in one untagged isolationprobe
  file so the startup callback can reject an unknown name.
- Env name `BARN_ELLE_CHECKPOINT_STOP=point:N`, not `BARN_CHECKPOINT_STOP`:
  the latter is already used by the db/format interruption test helper with a
  different meaning (`checkpoint_recovery_test.go:196`).
- Boundary record: one JSON line written with a single unbuffered write to
  stderr (the Python harness merges stderr into its log file,
  `scripts/mongoose_barn_process.py:215`), then `SIGSTOP` to own pid. Non-Unix
  elle builds compile; selecting a stop there is a startup error.

## Surprises
- `BARN_CHECKPOINT_STOP` already exists as a test-helper env in db/format.

## Next step
Commit, push, open PR, queue auto-merge, watch checks. Later (harness PR):
Python adapter reads the `checkpoint stop` stderr line, verifies the owned pid is
stopped, then SIGCONT/SIGKILL; add the elle-tagged tests to a workflow.
