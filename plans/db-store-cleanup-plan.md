# Barn Database and Store Cleanup Plan - 2026-06-17

## Current Baseline

Completed slice:
- `db.Store.FindProperty` is now the canonical property resolver.
- Duplicate property-chain helpers were removed from VM, builtins, and server code.
- Commits:
  - `8f2ab64 Consolidate property resolution in store`
  - `5035412 Record property resolution cleanup commit`

Known unrelated baseline:
- `go test -timeout 180s ./db ./builtins ./vm ./server` still fails in `barn/server` at `TestTLSListenerLoginAndEval` with `"I couldn't understand that.\r\n"` instead of `{1, 3}`.

Architectural rule for the remaining work:
- Do not introduce backend interfaces, adapters, wrappers, or shims until live mutable object access is under store-owned methods.
- Delete duplicate local helpers as each store-owned capability lands.
- Keep one target surface/family per slice.

## Phase 1 - Close Property Mutation Through `db.Store`

Target owner:
- `db.Store` owns property mutation, not VM or builtins callers.

Forbidden surfaces:
- Direct production writes to `obj.Properties[...]`.
- Direct production deletes from `obj.Properties`.
- Direct production mutation of `prop.Value`, `prop.Clear`, `prop.Owner`, or `prop.Perms` when the object came from `Store.Get`.

Work:
- Add store-owned methods for property value updates, local override creation, property definition, property deletion, inherited propagation, and clear-slot removal.
- Move VM property assignment in `vm/op_property.go` to those methods.
- Move builtins property definition/deletion/clear/info mutation in `builtins/properties.go` to those methods.
- Preserve Toast-backed clear-slot semantics from the new `Store.FindProperty`.

Search gates:
- `rg -n "obj\\.Properties\\[|delete\\(obj\\.Properties|prop\\.Value =|prop\\.Clear =|prop\\.Owner =|prop\\.Perms =" builtins vm server db`
- Remaining hits must be either inside `db.Store` owner methods, test setup, or explicitly recorded as the next slice.

Runtime gates:
- `go test ./db ./builtins ./vm`
- `go test -timeout 120s ./builtins -run "Test.*Property"`
- `go test -timeout 120s ./vm -run "Test.*Property"`
- `git diff --check`

## Phase 2 - Close Verb Mutation Through `db.Store`

Target owner:
- `db.Store` owns verb map/list consistency.

Forbidden surfaces:
- Production callers writing `obj.Verbs[...]`.
- Production callers splicing `obj.VerbList`.
- Production callers clearing bytecode/source metadata directly when changing verb code.

Work:
- Add store-owned verb operations for add, delete, rename/info update, args update, and code update.
- Preserve definition-order alias semantics currently enforced by `VerbList`.
- Centralize the map/list repair logic now open-coded in `builtins/verbs.go`.

Search gates:
- `rg -n "obj\\.Verbs\\[|delete\\(obj\\.Verbs|obj\\.VerbList\\s*=|append\\(obj\\.VerbList" builtins server vm db`
- Remaining production hits outside store methods must be deleted or moved in the same slice.

Runtime gates:
- `go test ./db ./builtins ./vm ./server`
- If `server` remains blocked only by the known TLS eval baseline, run targeted verb/server gates and record the baseline.
- `go test -timeout 120s ./builtins -run "Test.*Verb"`
- `git diff --check`

## Phase 3 - Close Object Lifecycle and Relationship Mutation

Target owner:
- `db.Store` owns object allocation, recycle/recreate, parent/child links, location/contents links, anonymous-child tracking, and max/high-water invariants.

Forbidden surfaces:
- Production callers mutating `obj.Parents`, `obj.Children`, `obj.Location`, `obj.Contents`, `obj.AnonymousChildren`, `obj.Anonymous`, or object lifecycle flags directly.
- Loader code duplicating allocation/high-water bookkeeping.

Work:
- Move create/recreate relationship updates from builtins into store methods.
- Move movement and hierarchy mutation from `builtins/objects.go`, `builtins/objects_movement.go`, and `builtins/objects_hierarchy.go` into store-owned operations.
- Convert `Database.NewStoreFromDatabase()` to use a shared loaded-object insertion path so loader and runtime invariants cannot drift.
- Keep object references as `ObjID`; do not introduce object pointer graphs.

Search gates:
- `rg -n "obj\\.(Parents|Children|Location|Contents|AnonymousChildren|Anonymous|Recycled|Flags)\\s*=|append\\(.*\\.(Parents|Children|Contents|AnonymousChildren)" builtins server vm db`
- `rg -n "store\\.objects\\[|highWaterID|maxObjID" db builtins server vm`

Runtime gates:
- `go test ./db ./builtins ./vm`
- `go test -timeout 120s ./builtins -run "Test.*(Create|Recycle|Recreate|Move|Parent|Child|Location|Object)"`
- Managed conformance if relationship or lifecycle semantics change.
- `git diff --check`

## Phase 4 - Separate Snapshot Persistence From Live Store

Target owner:
- `db.Store` owns live in-memory world state.
- Reader/writer code owns LambdaMOO/ToastStunt text snapshot format.
- Server owns when checkpoints happen, not how object graph serialization works.

Forbidden surfaces:
- Server-local checkpoint serialization logic duplicating `db.CheckpointManager`.
- Writer reaching into live mutable data without a consistent snapshot boundary.
- Reader/writer property-order algorithms drifting apart.

Work:
- Decide whether `db.CheckpointManager` or `server.checkpoint` is the real checkpoint owner; delete the loser rather than preserving both flows.
- Introduce a store-owned snapshot/export operation only after mutation paths are inside the store.
- Share property-order and arg/prep conversion logic between reader and writer where the same concept exists.
- Keep v4/v5/v17 parsing support as format readers, not runtime storage backends.

Search gates:
- `rg -n "NewWriter\\(|WriteDatabase\\(|CheckpointManager|checkpoint\\(" db server cmd`
- `rg -n "argspecToString|argspecToInt|prepToString|prepToInt|collectPropertyNames" db`

Runtime gates:
- `go test ./db ./server`
- Existing round-trip tests in `db` and `vm`.
- Managed conformance checkpoint/restart slices when checkpoint ownership changes.
- `git diff --check`

## Phase 5 - Reassess Backend Pluggability

Entry criteria:
- No production caller mutates object internals returned by `Store.Get`.
- Property, verb, relationship, and lifecycle mutation are store-owned.
- Snapshot/export has a clear boundary.

Decision questions:
- Is the desired backend a live transactional object store, or just alternate snapshot persistence?
- Is SQLite/Bolt/etc. meant to replace runtime memory, or to store checkpoints/indexes?
- Does the VM require object identity/pointer stability, or can all access go through store methods?

Possible outcomes:
- Keep in-memory runtime plus cleaner snapshot/checkpoint persistence.
- Add an optional derived index/query layer for introspection.
- Add a real backend interface only if store-owned operations form a narrow complete contract.

Do not start this phase by adding an interface. Start by verifying the entry criteria with search gates.

## Suggested Execution Order

1. Phase 1 property mutation.
2. Phase 2 verb mutation.
3. Phase 3 object lifecycle and relationship mutation.
4. Phase 4 snapshot/checkpoint ownership.
5. Phase 5 backend decision.

Each phase should be a deletion-first fixed-point loop with its own record file under `reports/`, a tight commit, and explicit separation of known unrelated test failures.
