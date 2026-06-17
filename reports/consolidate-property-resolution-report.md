# Consolidate Property Resolution Report

## Files Changed

- `db/store.go`
- `vm/op_property.go`
- `builtins/properties.go`
- `builtins/limits.go`
- `builtins/protected.go`
- `server/scheduler_login.go`
- `reports/consolidate-property-resolution-report.md`

`builtins/protected.go` was changed because it was a production caller of the deleted `builtins` helper.

## Duplicate Helpers Deleted

- `vm/op_property.go`: deleted local `findProperty`.
- `builtins/properties.go`: deleted local `findPropertyInChain`.
- `builtins/limits.go`: deleted local `findPropertyInherited`.
- `server/scheduler_login.go`: deleted local `Scheduler.findPropertyInherited`.

`db.Store.FindProperty` is now the canonical breadth-first property resolver. It preserves the VM resolver semantics: nearest property metadata is retained while clear slots inherit the first non-clear ancestor value.

## Search Gate Results

- Pass: `rg -n "func findProperty\\(|func findPropertyInChain|func findPropertyInherited" db builtins server vm`
  - Exit code 1, zero matches.
- Pass: `rg -n "ResolveProperty|FindProperty" db builtins server vm`
  - Matches are the store-owned method in `db/store.go` and direct call sites in `vm`, `builtins`, and `server`.

## Runtime Gate Results

- Partial pass: `go test -timeout 180s ./db ./builtins ./vm ./server`
  - `barn/db`: pass.
  - `barn/builtins`: pass.
  - `barn/vm`: pass.
  - `barn/server`: fail in `TestTLSListenerLoginAndEval`.
- Pass: `go test -timeout 120s ./server -run "Test(ConnectionOptions|TrustedProxy|Login|DoLogin|UserConnected|ConnectMessage|ServerOption|Protected|Scheduler)"`
- Pass: `go test -timeout 120s ./builtins -run "Test.*(Property|Limit|Protected|ServerOption)"`
- Pass: `git diff --check`

## Unrelated Pre-Existing Failure

`go test -timeout 180s ./db ./builtins ./vm ./server` still fails in `barn/server`:

```text
--- FAIL: TestTLSListenerLoginAndEval (0.06s)
    tls_listener_test.go:166: eval response "I couldn't understand that.\r\n", want {1, 3}
FAIL
FAIL    barn/server
```

This matches the previously known baseline server failure bucket and is unrelated to the property-resolution consolidation.
