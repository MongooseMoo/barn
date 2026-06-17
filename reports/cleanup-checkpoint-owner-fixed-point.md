# Checkpoint Owner Cleanup Fixed Point

Date: 2026-06-17

## Target

Phase 4 checkpoint ownership. Decide whether `db.CheckpointManager` or `server.checkpoint` owns checkpoint orchestration, then delete the loser.

## Decision

`server.checkpoint` is the live checkpoint owner.

Reasons:
- It runs checkpoint hooks before writing.
- It writes pending finalizations before serializing.
- It passes the scheduler task source to the database writer.
- It writes the sibling `.new` checkpoint file used by the server flow.

`db.CheckpointManager` had no production callers and duplicated a weaker checkpoint path, so it was deleted instead of kept as a second flow.

## Deletion

Deleted:
- `db/checkpoint.go`

## Search Gates

`rg -n "NewWriter\\(|WriteDatabase\\(|CheckpointManager|checkpoint\\(" db server cmd`

Result:
- No `CheckpointManager` references remain.
- Remaining checkpoint orchestration is in `server.checkpoint`.
- Remaining writer construction is in server, command-line tooling, and tests.

## Runtime Gates

Passed:
- `go test -timeout 120s ./server -run "TestDoLoginCommandDispatchesOnListenerWithArgstr|TestLoginPlayerRunsListenerCreatedAndConnectedHooks"`
- `git diff --check -- db/checkpoint.go`

Known unrelated baseline reproduced:
- `go test ./db ./server` fails only in `barn/server` `TestTLSListenerLoginAndEval`.
- Isolated `go test -timeout 120s ./server -run "TestTLSListenerLoginAndEval"` reproduces the same failure.

Commit:
- Pending
