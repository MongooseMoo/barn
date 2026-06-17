# Writer Snapshot Boundary Fixed Point

Date: 2026-06-17

## Target

Phase 4 writer access to live mutable store state during snapshot serialization.

## Change

Added a package-private store snapshot boundary:
- `store.snapshot()`
- `storeSnapshot`
- snapshot object, property, and verb cloning helpers

Writer now takes one store-owned snapshot at the start of `WriteDatabase()` and serializes from that snapshot.

## Deletion

Deleted live writer reads of:
- `w.store.Players()`
- `w.store.MaxObject()`
- `w.store.GetUnsafe()`
- `w.store.GetAnonymousObjects()`
- `w.store.All()`
- `w.store.Get()` during WAIF serialization
- the now-dead `Store.PropertyNames()` accessor

Remaining writer store reach:
- `w.store.snapshot()` at the start of `WriteDatabase()`

## Search Gates

`rg -n "NewWriter\\(|WriteDatabase\\(|CheckpointManager|checkpoint\\(" db server cmd`

Result:
- No `CheckpointManager` hits.
- Checkpoint orchestration remains in `server.checkpoint`.
- Writer construction remains in server, commands, and tests.

`rg -n "argspecToString|argspecToInt|prepToString|prepToInt|collectPropertyNames" db`

Result:
- Zero hits.

## Runtime Gates

Passed:
- `go test ./db ./vm`
- `git diff --check`

Known unrelated baseline reproduced:
- `go test ./db ./server` fails only in `barn/server` `TestTLSListenerLoginAndEval`.

Commit:
- Pending
