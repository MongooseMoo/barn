# Offline checkpoint mode

## Goal

`barn -db PATH -offline-checkpoint` loads a database and writes it as a full
ordinary checkpoint (PATH.new and PATH.new.waifids: objects, verbs, queued,
suspended and interrupted tasks, connection records, WAIF sidecar), then exits,
without running a task, opening a listener or calling a database verb.
`-omit-suspended` leaves suspended and interrupted tasks out. Purpose: show
whether current Barn saves a real Mongoose database, and produce a file to
compare with the original and to try loading into Toast.

## State

- `internal/dbtool/offline_checkpoint.go`: `WriteOfflineCheckpoint`.
- `internal/app/app.go`, `cmd/barn/main.go`: the two flags.
- `internal/dbtool/offline_checkpoint_test.go`: synthetic database with a WAIF
  property, a queued fork, a task suspended holding a WAIF and one connection
  record; checks they survive, are absent with the omit option (suspended
  only), and that a second pass is byte-identical, sidecar included.
- Not pushed; no PR (assignment forbids it).

## Decisions and why

- A new flag beside `-dump`, not a change to it. `-dump` writes exactly the
  named file without tasks, connections or sidecar
  (`internal/dbtool/dbtool.go` `dumpDatabase` calls `NewWriter(...).WriteDatabase()`
  with no `SetTaskSnapshots`/`SetActiveConnections`) and has its own reload and
  compare contract and tests.
- Reuses `dbformat.WriteCheckpoint`, the server's checkpoint path, and the
  server's task restore (`LoadQueuedTasks`, `LoadSuspendedTasks`,
  `TaskSnapshots`) on a runtime that is never started. Loading registers tasks
  on the scheduler heap only; nothing dispatches without the input processor.
- Does not use `Server.LoadDatabase`: it moves the file's pending
  finalizations into the runtime (`AdoptPendingFinalizations`), so they would
  not be written back, and arms deferred GC.
- Connection records are the loaded ones written back unchanged. A live
  checkpoint writes the live connection list, which offline is empty.
- Loads with `LoadDatabaseReadOnly`, so the input is never changed.

## Surprises

- None in the code. Tasks the server would drop at boot (fork source that no
  longer compiles; suspended program not matching its source) are dropped here
  too, with the same log records; the printed read/wrote counts show it.

## Next step

Run on a real database copy, compare pass one with pass two, then try the
output in Toast.
