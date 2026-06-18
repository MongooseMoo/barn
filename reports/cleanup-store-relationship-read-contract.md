# Cleanup Refactor Fixed-Point Log - 2026-06-18

Target architecture:
- `db.Store` owns runtime relationship reads and relationship traversals.
- Runtime callers consume store methods that return scalar object IDs, copied object-ID slices, or boolean relationship facts.
- `db.Object` remains the in-memory record shape, not the default MOO object read interface.

Forbidden surfaces:
- Direct production reads of `obj.Parents`, `obj.Children`, `obj.Contents`, or `obj.Location` in the first-slice callers.
- Caller-local relationship walkers that duplicate parent, child, content, or location traversal.
- Backend interfaces, adapters, wrappers, shims, or dual read paths.

Slice boundary:
- `db/store.go`
- `builtins/objects_hierarchy.go`
- `builtins/objects_movement.go`
- `vm/op_property.go`

Search gates:
- `rg -n --pcre2 "(?<!store)\.(Parents|Children|Contents|Location)\b" builtins/objects_hierarchy.go builtins/objects_movement.go vm/op_property.go --glob "!**/*_test.go"`
- `rg -n "store\.Get\(" builtins/objects_hierarchy.go builtins/objects_movement.go vm/op_property.go`

Runtime gates:
- `go test ./db ./builtins ./vm`
- `go test -timeout 120s ./builtins -run "Test.*(Parent|Child|Move|Location|Object|Occupants|Isa|Ancestor|Descendant)"`
- `go test -timeout 120s ./vm -run "Test.*Property"`
- `go test ./server`
- `git diff --check -- db/store.go builtins/objects_hierarchy.go builtins/objects_movement.go vm/op_property.go`

## Iteration 1 - relationship reads in hierarchy, movement, and VM builtin properties

Slice read:
- `plans/db-store-cleanup-plan.md`
- `docs/reports/store-read-contract-inventory.md`
- `db/store.go`
- `builtins/objects_hierarchy.go`
- `builtins/objects_movement.go`
- `vm/op_property.go`

Surfaces:
- `db.Store` relationship read methods
  - Disposition: move
  - Owner after cleanup: `db.Store`
  - Action: Added `Parent`, `Parents`, `Children`, `Contents`, `Location`, `Ancestors`, `Descendants`, `HasAncestor`, `HasDescendant`, and `HasContentDescendant`.
  - Evidence: Slice callers no longer need direct relationship fields for the converted paths; slice-returning methods copy relationship slices.
- `builtins/objects_hierarchy.go` parent and traversal builtins
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote `parent`, `parents`, `children`, `ancestors`, `descendants`, `isa`, and `locations` to use store relationship reads.
  - Evidence: Deleted `isChildOf`, `isDescendant`, and `objectHasAncestor` caller-local relationship walkers.
- `builtins/objects_hierarchy.go` property conflict helpers
  - Disposition: keep for later slice
  - Owner after cleanup: pending property-read contract
  - Action: Left property-map reads in place but moved their parent-chain traversal to `store.Parents`.
  - Evidence: Remaining `store.Get` hits in this file read properties, owner/flags, or validate initialize behavior, not relationship fields.
- `builtins/objects_movement.go` move and occupants relationship checks
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Replaced recursive move-content traversal with `store.HasContentDescendant` and occupants ancestry filtering with `store.HasAncestor`.
  - Evidence: Remaining `store.Get` hit reads object flags for the later metadata/flag slice.
- `vm/op_property.go` builtin relationship properties
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote `.location`, `.contents`, `.parents`, `.parent`, and `.children` reads to use store methods.
  - Evidence: `.name`, `.owner`, flag, and anonymous reads remain for the later metadata/permission slice.

Gate results:
- Pass: `rg -n --pcre2 "(?<!store)\.(Parents|Children|Contents|Location)\b" builtins/objects_hierarchy.go builtins/objects_movement.go vm/op_property.go --glob "!**/*_test.go"` returned no hits.
- Recorded deferral: `rg -n "store\.Get\(" builtins/objects_hierarchy.go builtins/objects_movement.go vm/op_property.go` returns only metadata, flags, property maps, and initialization checks in the slice files.
- Pass: `go test ./db ./builtins ./vm`
- Pass: `go test -timeout 120s ./builtins -run "Test.*(Parent|Child|Move|Location|Object|Occupants|Isa|Ancestor|Descendant)"` (`[no tests to run]`)
- Pass: `go test -timeout 120s ./vm -run "Test.*Property"` (`[no tests to run]`)
- Pass: `go test ./server`
- Pass: `git diff --check -- db/store.go builtins/objects_hierarchy.go builtins/objects_movement.go vm/op_property.go`

Commit:
- Pending.

Next slice:
- Built-in object property read and metadata/permission reads: `ObjectName`, `ObjectOwner`, `HasObjectFlag`, `ObjectIsAnonymous`, and related caller rewrites.
