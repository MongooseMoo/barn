# Cleanup Refactor Fixed-Point Log - 2026-06-18

Target architecture:
- `db.Store` owns runtime object metadata reads.
- Runtime callers consume store methods for object name, owner, flags, and anonymous state.
- `db.Object` remains the in-memory record shape, not the caller-facing metadata read contract.

Forbidden surfaces:
- Direct production reads of object `Name`, `Owner`, `Flags`, or `Anonymous` in the active slice.
- Caller-local metadata helpers that duplicate store-owned reads.
- Backend interfaces, adapters, wrappers, shims, aliases, compatibility branches, fallback readers, or dual paths.

Search gates:
- `rg -n --pcre2 "(?<!waif)\.(Name|Owner|Flags|Anonymous)\b|\.Flags\.Has" vm/op_property.go builtins/objects_movement.go builtins/objects_hierarchy.go --glob "!**/*_test.go"`

Runtime gates:
- `go test ./db ./builtins ./vm ./server`
- `go test -timeout 120s ./vm -run "Test.*Property"`
- `go test -timeout 120s ./builtins -run "Test.*(Move|Occupants|Parent|Child|Object)"`
- `go build -o barn.exe ./cmd/barn/`
- `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"`
- `git diff --check -- db/store.go vm/op_property.go builtins/objects_movement.go builtins/objects_hierarchy.go reports/cleanup-store-metadata-read-contract.md`

## Iteration 1 - Phase 1 VM, movement, and chparent metadata reads

Slice read:
- `plans/store-metadata-read-cleanup-plan.md`
- `db/store.go`
- `vm/op_property.go`
- `builtins/objects_movement.go`
- `builtins/objects_hierarchy.go`

Surfaces:
- `db.Store` scalar metadata read methods
  - Disposition: move
  - Owner after cleanup: `db.Store`
  - Action: Added `ObjectName`, `ObjectOwner`, `ObjectFlags`, `HasObjectFlag`, and `ObjectIsAnonymous`.
  - Evidence: Phase 1 callers no longer need direct object metadata fields for converted paths.
- `vm/op_property.go` builtin metadata property reads
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote `.name`, `.owner`, `.programmer`, `.wizard`, `.player`, `.r`, `.w`, `.f`, and `.a` reads to use store metadata methods.
  - Evidence: Object metadata field hits are gone from VM builtin property reads; `Property.Owner` hits remain only in property permission checks.
- `vm/op_property.go` builtin metadata property assignment checks
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote anonymous-object guards on `.owner`, `.programmer`, and `.wizard` assignment to use `store.ObjectIsAnonymous`.
  - Evidence: Assignment code no longer reads `obj.Anonymous` directly.
- `builtins/objects_movement.go` occupants player filtering
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote player-flag filtering to use `store.HasObjectFlag`.
  - Evidence: The file has no remaining object metadata field hits.
- `builtins/objects_hierarchy.go` `chparent` permission check
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote new-parent owner/fertile checks to use `store.ObjectOwner` and `store.HasObjectFlag`.
  - Evidence: `chparent` no longer reads new-parent object metadata fields directly.
- `builtins/objects_hierarchy.go` `locate_by_name` and `owned_objects`
  - Disposition: keep for Phase 3
  - Owner after cleanup: pending world-scan query methods
  - Action: Recorded as deliberate deferrals because Phase 1 does not own name/owner world scans.
  - Evidence: Search gate still reports `obj.Name` in `locate_by_name` and `obj.Owner` in `owned_objects`.

Gate results:
- Pass with recorded deferrals: `rg -n --pcre2 "(?<!waif)\.(Name|Owner|Flags|Anonymous)\b|\.Flags\.Has" vm/op_property.go builtins/objects_movement.go builtins/objects_hierarchy.go --glob "!**/*_test.go"`
  - Remaining `builtins/objects_hierarchy.go:602` is `locate_by_name`, deferred to Phase 3.
  - Remaining `builtins/objects_hierarchy.go:694` is `owned_objects`, deferred to Phase 3.
  - Remaining `vm/op_property.go:259` and `vm/op_property.go:277` are `Property.Owner` permission checks, not object metadata.
- Pass: `go test ./db ./builtins ./vm ./server`
- Pass: `go test -timeout 120s ./vm -run "Test.*Property"` (`[no tests to run]`)
- Pass: `go test -timeout 120s ./builtins -run "Test.*(Move|Occupants|Parent|Child|Object)"` (`[no tests to run]`)
- Pass: `go build -o barn.exe ./cmd/barn/`
- Pass: `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"` (`3871 passed, 131 skipped in 142.10s`)
- Pass: `git diff --check -- db/store.go vm/op_property.go builtins/objects_movement.go builtins/objects_hierarchy.go`

Commit:
- Pending.

Next slice:
- Phase 2 permission-heavy runtime metadata reads in `builtins/objects.go`, `builtins/objects_players.go`, `builtins/properties.go`, `builtins/verbs.go`, `vm/registry.go`, `vm/op_verb.go`, and server scheduler/login/call-verb surfaces.
