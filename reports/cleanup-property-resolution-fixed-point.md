# Cleanup Refactor Fixed-Point Log - 2026-06-17

Target architecture:
- `db.Store` owns property resolution through object parent chains.
- Runtime packages call the store-owned resolver directly.

Forbidden surfaces:
- Local duplicate property-chain walker helpers in VM, builtins, or server code.
- New interfaces, adapters, wrappers, facades, compatibility shims, or renamed helper surfaces.

Search gates:
- `rg -n "func findProperty\\(|func findPropertyInChain|func findPropertyInherited" db builtins server vm`
- `rg -n "ResolveProperty|FindProperty" db builtins server vm`

Runtime gates:
- `go test -timeout 180s ./db ./builtins ./vm ./server`
- `go test -timeout 120s ./server -run "Test(ConnectionOptions|TrustedProxy|Login|DoLogin|UserConnected|ConnectMessage|ServerOption|Protected|Scheduler)"`
- `go test -timeout 120s ./builtins -run "Test.*(Property|Limit|Protected|ServerOption)"`
- `git diff --check`

## Iteration 1 - `property resolution`

Slice read:
- `db/store.go`
- `vm/op_property.go`
- `builtins/properties.go`
- `builtins/limits.go`
- `builtins/protected.go`
- `server/scheduler_login.go`

Surfaces:
- `vm.findProperty`
  - Disposition: consolidate
  - Owner after cleanup: `db.Store.FindProperty`
  - Action: Deleted local VM helper and moved its clear-slot inheritance semantics to `db.Store`.
  - Evidence: VM property access now calls `vm.Store.FindProperty(...)`.
- `builtins.findPropertyInChain`
  - Disposition: consolidate
  - Owner after cleanup: `db.Store.FindProperty`
  - Action: Deleted local builtins helper and moved callers to the store-owned resolver.
  - Evidence: property builtins now call `store.FindProperty(...)`.
- `builtins.findPropertyInherited`
  - Disposition: consolidate
  - Owner after cleanup: `db.Store.FindProperty`
  - Action: Deleted local limits helper and updated both limits and protected-builtin loading callers.
  - Evidence: `builtins/limits.go` and `builtins/protected.go` now call `store.FindProperty(...)`.
- `Scheduler.findPropertyInherited`
  - Disposition: consolidate
  - Owner after cleanup: `db.Store.FindProperty`
  - Action: Deleted scheduler-local helper and updated login server-option lookup.
  - Evidence: `server/scheduler_login.go` now calls `s.store.FindProperty(...)`.
- `db.Store.FindProperty`
  - Disposition: keep
  - Owner after cleanup: `db.Store`
  - Action: Added canonical breadth-first property resolver with VM-compatible clear-slot value inheritance.
  - Evidence: search gate shows only this method definition and direct production call sites.

Gate results:
- Pass: `rg -n "func findProperty\\(|func findPropertyInChain|func findPropertyInherited" db builtins server vm`
  - Exit code 1, zero matches.
- Pass: `rg -n "ResolveProperty|FindProperty" db builtins server vm`
  - Matches are the store-owned method plus direct call sites.
- Partial pass with known unrelated baseline: `go test -timeout 180s ./db ./builtins ./vm ./server`
  - `barn/db`, `barn/builtins`, and `barn/vm` passed.
  - `barn/server` failed in existing baseline `TestTLSListenerLoginAndEval`: eval response `"I couldn't understand that.\r\n"`, want `{1, 3}`.
- Pass: `go test -timeout 120s ./server -run "Test(ConnectionOptions|TrustedProxy|Login|DoLogin|UserConnected|ConnectMessage|ServerOption|Protected|Scheduler)"`
- Pass: `go test -timeout 120s ./builtins -run "Test.*(Property|Limit|Protected|ServerOption)"`
- Pass: `git diff --check`

Commit:
- `8f2ab64 Consolidate property resolution in store`

Next slice:
- Close property and object mutation paths through `db.Store` so runtime callers no longer mutate leaked `*db.Object` internals directly.
