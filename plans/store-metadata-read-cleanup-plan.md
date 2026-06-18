# Store Metadata Read Cleanup Plan - 2026-06-18

## Control Rules

- Execute this plan as the active control surface until every phase is complete, explicitly deferred by the user, or truly blocked.
- After every passing full-suite run, and after every passing substantial targeted test run, reread this plan before choosing the next step.
- Commit each kept reduction before moving to a different slice.
- Do not introduce backend interfaces, adapters, wrappers, shims, aliases, compatibility branches, fallback readers, or dual paths.
- Do not add `ObjectBuiltinProperty()` until scalar metadata reads prove whether that larger semantic method is actually needed.
- Keep `db.Object` as the in-memory record shape. `db.Store` is the runtime read owner.

## Target Architecture

`db.Store` owns object metadata reads used by runtime callers. Runtime code should ask the store for object facts instead of reading `Name`, `Owner`, `Flags`, or `Anonymous` directly from objects returned by `Store.Get`.

Initial store contract:

```go
ObjectName(id types.ObjID) (string, types.ErrorCode)
ObjectOwner(id types.ObjID) (types.ObjID, types.ErrorCode)
ObjectFlags(id types.ObjID) (db.ObjectFlags, types.ErrorCode)
HasObjectFlag(id types.ObjID, flag db.ObjectFlags) (bool, types.ErrorCode)
ObjectIsAnonymous(id types.ObjID) (bool, types.ErrorCode)
```

## Forbidden Surfaces

- Direct production reads of object `Name`, `Owner`, `Flags`, or `Anonymous` in the active slice.
- Caller-local metadata helpers that duplicate store-owned reads.
- New interface layers, helper wrappers, adapters, shims, or broad compatibility surfaces.
- Claiming backend pluggability while runtime callers still depend on mutable `*db.Object` internals.

Allowed remaining hits must be recorded explicitly, for example:

- `Property.Owner`
- `Verb.Owner`
- task owner fields
- waif owner fields
- reader/writer/snapshot/load boundaries
- debug-only or CLI surfaces deferred to a later slice

## Phase 0 - Plan Commit

Status: complete.

Work:
- Write this plan to `plans/store-metadata-read-cleanup-plan.md`.
- Commit the plan before implementation.

Gates:
- `git diff --check -- plans/store-metadata-read-cleanup-plan.md`

## Phase 1 - First Metadata Read Slice

Status: complete.

Slice boundary:
- `db/store.go`
- `vm/op_property.go`
- `builtins/objects_movement.go`
- `builtins/objects_hierarchy.go`
- `reports/cleanup-store-metadata-read-contract.md`

Work:
- Add scalar metadata read methods to `db.Store`.
- Move VM builtin object metadata reads:
  - `.name`
  - `.owner`
  - `.programmer`
  - `.wizard`
  - `.player`
  - `.r`
  - `.w`
  - `.f`
  - `.a`
- Move VM builtin property assignment anonymous checks to `store.ObjectIsAnonymous`.
- Move `occupants(..., player_flag)` to `store.HasObjectFlag`.
- Move `chparent` owner/fertile permission check to `store.ObjectOwner` and `store.HasObjectFlag`.
- Leave property-map reads in `objects_hierarchy.go` for the property-read slice.

Search gates:

```powershell
rg -n --pcre2 "(?<!waif)\.(Name|Owner|Flags|Anonymous)\b|\.Flags\.Has" vm/op_property.go builtins/objects_movement.go builtins/objects_hierarchy.go --glob "!**/*_test.go"
```

Expected:
- Zero object metadata hits for the Phase 1 work items.
- `builtins/objects_hierarchy.go` hits in `locate_by_name` and `owned_objects` are Phase 3 deferrals and must be recorded if still present.
- `Property.Owner` hits in VM permission checks are non-object metadata and must be recorded if still present.
- Remaining `store.Get` in the active slice must be metadata/property-map work recorded for a later phase.

Runtime gates:

```powershell
go test ./db ./builtins ./vm ./server
go test -timeout 120s ./vm -run "Test.*Property"
go test -timeout 120s ./builtins -run "Test.*(Move|Occupants|Parent|Child|Object)"
go build -o barn.exe ./cmd/barn/
uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
git diff --check -- db/store.go vm/op_property.go builtins/objects_movement.go builtins/objects_hierarchy.go reports/cleanup-store-metadata-read-contract.md
```

Commit policy:
- Commit the code plus fixed-point record.
- If the record says `Pending` for the code commit hash, update it and make a second record-only commit.

## Phase 2 - Permission-Heavy Runtime Metadata Reads

Status: complete.

Slice boundary:
- `builtins/objects.go`
- `builtins/objects_players.go`
- `builtins/properties.go`
- `builtins/verbs.go`
- `vm/registry.go`
- `vm/op_verb.go`
- `server/scheduler.go`
- `server/scheduler_login.go`
- `server/scheduler_call_verb.go`
- `reports/cleanup-store-metadata-read-contract.md`

Work:
- Replace wizard, programmer, player, fertile, read, write, and anonymous checks with `store.HasObjectFlag`.
- Replace owner checks with `store.ObjectOwner`.
- Replace anonymous checks with `store.ObjectIsAnonymous`.
- Keep `Property.Owner`, `Verb.Owner`, and task ownership out of scope unless the caller is actually reading object metadata.

Search gate:

```powershell
rg -n --pcre2 "\.(Owner|Flags|Anonymous)\b|\.Flags\.Has" builtins vm server --glob "!**/*_test.go"
```

Expected:
- Remaining hits are recorded as non-object surfaces, reader/writer/load boundaries, or later slices.

Runtime gates:

```powershell
go test ./db ./builtins ./vm ./server
go test -timeout 120s ./builtins -run "Test.*(Property|Verb|Player|Object|Perm|Wizard)"
go test -timeout 120s ./vm -run "Test.*(Verb|Property|Perm|Wizard)"
go build -o barn.exe ./cmd/barn/
uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
git diff --check
```

Commit policy:
- Commit the kept reduction before Phase 3.

## Phase 3 - Name Reads and World Scan Queries

Status: complete.

Slice boundary:
- `builtins/objects_hierarchy.go`
- `server/matcher.go`
- `builtins/objects_misc.go`
- `reports/cleanup-store-metadata-read-contract.md`

Candidate store methods:

```go
ObjectIDsByNameSubstring(needle string, caseSensitive bool) []types.ObjID
ObjectsOwnedBy(owner types.ObjID) []types.ObjID
AliasStrings(id types.ObjID) ([]string, types.ErrorCode)
```

Work:
- Replace `locate_by_name` and `owned_objects` direct object scans with store-owned query methods.
- Replace server matcher direct name reads with `store.ObjectName` or a store-owned fact query.
- Keep command matching semantics in `server`; only data reads belong in `db.Store`.
- Decide separately whether `object_bytes` should remain a debug-size estimator over object internals or move to a store-owned size estimate.

Search gate:

```powershell
rg -n --pcre2 "\.(Name|Owner|Flags|Anonymous)\b|\.Flags\.Has|store\.Get\(" builtins/objects_hierarchy.go server/matcher.go builtins/objects_misc.go --glob "!**/*_test.go"
```

Runtime gates:

```powershell
go test ./db ./builtins ./vm ./server
go test -timeout 120s ./builtins -run "Test.*(Locate|Owned|Object|Bytes)"
go test -timeout 120s ./server -run "Test.*(Match|Command)"
go build -o barn.exe ./cmd/barn/
uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
git diff --check
```

Commit policy:
- Commit the kept reduction before declaring the metadata-read plan complete.

## Completion Criteria

- Every phase is complete or explicitly deferred by the user.
- Production runtime code outside `db.Store` no longer reads object `Name`, `Owner`, `Flags`, or `Anonymous` directly, except for recorded non-object surfaces and snapshot/load/debug boundaries.
- Runtime gates pass.
- Full managed conformance passes after the final implemented phase.
- `reports/cleanup-store-metadata-read-contract.md` records every slice disposition, gate result, commit, and next action.
