# Snapshot Format Codec Cleanup Fixed Point

Date: 2026-06-17

## Target

Phase 4 reader/writer drift in snapshot text-format helpers.

## Deletion

Deleted from `db/writer_object.go`:
- Writer-local `argspecToInt`
- Writer-local `prepToInt`
- Writer-local `collectPropertyNames`
- Writer-local recursive parent-chain property traversal

Renamed and shared in `db/reader_helpers.go`:
- Arg spec code/string conversion
- Prep code/string conversion
- Self-first property-name traversal used by reader load resolution and store dump export

Added in `db.Store`:
- `PropertyNames`, a store-owned ordered property-name export for snapshot writing.

## Search Gates

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
