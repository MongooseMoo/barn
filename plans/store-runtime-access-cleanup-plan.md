# Store Runtime Access Cleanup Plan - 2026-06-18

## Control Rules

- Execute this plan deletion-first, one slice at a time.
- Reread this plan after every passing full managed conformance run and after every passing substantial targeted test run before choosing the next step.
- Commit each kept reduction before moving to a different slice.
- Do not introduce backend interfaces, adapters, wrappers, shims, aliases, compatibility branches, fallback readers, or dual paths.
- Keep `db.Object`, `db.Property`, and `db.Verb` as in-memory record shapes until callers no longer depend on mutable pointers from `Store.Get`.
- Keep reader/writer files as snapshot-format boundaries. Do not turn v4/v5/v17 readers into runtime storage backends.
- Treat `db.Store` as the live runtime owner. A pluggable backend decision is deferred until production callers no longer require object internals.

## Current Baseline

Recent completed work:
- Scalar metadata reads moved behind `db.Store`.
- Relationship reads and common world-scan reads moved behind `db.Store`.
- `server/matcher.go` no longer reads object names, contents, or aliases directly.
- `object_bytes` estimate moved into `db.Store`.

Current evidence from inventory:
- Direct `Store.Get` production callers remain in `builtins`, `server`, and `vm`.
- The remaining important direct field reads are property structure, verb dispatch/hook lookup, anonymous/WAIF reachability, and a few permission/context stragglers.
- `cmd/*` and `conformance/setup.go` still inspect or mutate internals for debug/fixture setup; those are lower priority than runtime surfaces.

Known dirty-worktree constraint:
- Ignore unrelated pre-existing dirt and untracked reports/prompts/test artifacts.
- Stage only files in the active slice and its fixed-point record.

## Target Architecture

`db.Store` owns runtime access to object internals. Production runtime callers should ask the store for facts, queries, mutations, and lifecycle operations instead of holding mutable `*db.Object` pointers and reading or mutating fields.

Allowed boundaries:
- `db.Store` internals.
- `db` reader/writer/startup repair/snapshot code.
- Explicit debug/CLI surfaces, only if recorded and kept out of runtime slices.
- Test and conformance fixture setup, only if recorded separately.

Forbidden production surfaces in active slices:
- `store.Get(...).Properties`, `obj.Properties`, or caller-side property schema traversal.
- `store.Get(...).Verbs`, `obj.Verbs`, `obj.VerbList`, or caller-side verb ancestry traversal where the store can own the query.
- `store.All()`, `store.GetUnsafe()`, or `store.GetAnonymousObjects()` outside store-owned reachability/lifecycle queries.
- Direct object `Flags`, `Owner`, `Name`, `Anonymous`, `Recycled`, `Parents`, `Children`, `Contents`, `Location`, or `ChparentChildren` reads outside the store.
- Direct production writes to object internals outside `db.Store`.

## Phase 0 - Plan Commit

Status: pending.

Work:
- Write this plan to `plans/store-runtime-access-cleanup-plan.md`.
- Commit the plan before implementation if execution is requested.

Gates:
```powershell
git diff --check -- plans/store-runtime-access-cleanup-plan.md
```

## Phase 1 - Property Structure Queries

Status: pending.

Target owner:
- `db.Store` owns property schema and property-value traversal queries.

Slice boundary:
- `db/store.go`
- `builtins/objects.go`
- `builtins/objects_hierarchy.go`
- `builtins/protected.go`
- `vm/op_misc.go`
- `reports/cleanup-store-runtime-access.md`

Work:
- Move duplicate parent property-definition conflict checks in `create()` into store-owned query methods.
- Move `chparent` and `chparents` direct property conflict traversal into store-owned query methods.
- Move protected builtin `protect_*` property traversal into store-owned query methods.
- Move primitive prototype lookup from `vm/op_misc.go` to a store-owned property-value query.
- Keep property policy in builtins/VM; only object/property data traversal belongs in `db.Store`.
- Delete caller-local traversal helpers once store methods replace them.

Candidate store methods:
```go
DefinedPropertyNamesInAncestry(id types.ObjID) (map[string]bool, types.ErrorCode)
HasDuplicateDefinedPropertyAmong(ids []types.ObjID) (bool, types.ErrorCode)
HasDefinedPropertyConflictWithAncestry(objID types.ObjID, parentIDs []types.ObjID) (bool, types.ErrorCode)
HasChparentDescendantPropertyConflict(objID types.ObjID, names map[string]bool) (bool, types.ErrorCode)
PropertyValue(id types.ObjID, name string) (types.Value, types.ErrorCode)
TruthyPropertiesWithPrefixInAncestry(id types.ObjID, prefix string) (map[string]bool, types.ErrorCode)
```

Search gates:
```powershell
rg -n --pcre2 "\.(Properties|PropOrder|PropDefsCount|ChparentChildren)\b|store\.Get\(" builtins/objects.go builtins/objects_hierarchy.go builtins/protected.go vm/op_misc.go --glob "!**/*_test.go"
rg -n --pcre2 "collectAncestorProperties|hasChparentDescendantConflict|collectProtectFlags" builtins vm server --glob "!**/*_test.go"
```

Expected:
- Zero production direct property-structure reads in the active slice.
- Remaining `store.Get` hits must be existence-only, moved, or recorded for later slices.

Runtime gates:
```powershell
go test ./db ./builtins ./vm ./server
go test -timeout 120s ./builtins -run "Test.*(Create|Parent|Property|Protected|Object)"
go test -timeout 120s ./vm -run "Test.*(Property|Primitive|Prototype)"
go build -o barn.exe ./cmd/barn/
uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
git diff --check
```

Commit policy:
- Commit the code plus fixed-point record.
- If the record needs the final commit hash, make a second record-only commit.

## Phase 2 - Verb Dispatch and Hook Queries

Status: pending.

Target owner:
- `db.Store` owns verb ancestry traversal and hook existence queries.
- `server` owns command parse semantics, command search order, and argument-spec matching policy unless a narrower store method is clearly better.

Slice boundary:
- `db/store.go`
- `server/verbs.go`
- `server/server.go`
- `server/scheduler_login.go`
- `builtins/verbs.go`
- `reports/cleanup-store-runtime-access.md`

Work:
- Replace `server/verbs.go` direct `obj.Verbs` and `obj.Parents` traversal with store-owned verb candidate or verb-name query methods.
- Replace lifecycle/checkpoint hook existence checks in `server/server.go` with store-owned verb existence checks.
- Replace `scheduler_login.go` direct `systemObj.Verbs["do_login_command"]` lookup with a store-owned local verb query.
- In `builtins/verbs.go`, remove object existence checks that only use `store.Get`; use a store validity/error-classification method.
- Do not introduce a generic backend or adapter. Extend `db.Store` directly.

Candidate store methods:
```go
ObjectExists(id types.ObjID) types.ErrorCode
HasLocalVerb(id types.ObjID, name string) bool
HasVerbNameInAncestry(id types.ObjID, name string) bool
VerbCandidatesInAncestry(id types.ObjID) ([]VerbCandidate, types.ErrorCode)
```

Candidate DTO:
```go
type VerbCandidate struct {
    Definer types.ObjID
    Verb    *Verb
}
```

Search gates:
```powershell
rg -n --pcre2 "\.(Verbs|VerbList)\b|store\.Get\(" server/verbs.go server/server.go server/scheduler_login.go builtins/verbs.go --glob "!**/*_test.go"
rg -n --pcre2 "hasVerbNameOnObject|findVerbOnObject" server --glob "!**/*_test.go"
```

Expected:
- Zero direct `obj.Verbs`, `obj.VerbList`, or caller-owned verb ancestry traversal in the active slice.
- `builtins/verbs.go` should not use `store.Get` only for existence classification.

Runtime gates:
```powershell
go test ./db ./builtins ./vm ./server
go test -timeout 120s ./builtins -run "Test.*Verb"
go test -timeout 120s ./server -run "Test.*(Verb|Command|Hook|Login)"
go build -o barn.exe ./cmd/barn/
uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
git diff --check
```

## Phase 3 - Anonymous and WAIF Reachability

Status: pending.

Target owner:
- `db.Store` owns persistent property-value scans and anonymous-object candidate enumeration.
- VM/server own live stack roots and lifecycle invocation policy.

Slice boundary:
- `db/store.go`
- `vm/anonymous_gc.go`
- `server/waif_lifecycle.go`
- `reports/cleanup-store-runtime-access.md`

Work:
- Move persistent anonymous reachability scans from `vm/anonymous_gc.go` into `db.Store`.
- Move anonymous candidate enumeration off `store.GetUnsafe`, `store.All`, and `store.GetAnonymousObjects`.
- Move persistent WAIF root scans from `server/waif_lifecycle.go` into `db.Store`.
- Keep VM stack/root inspection in `vm`.
- Keep lifecycle hook invocation in `server` or builtin recycle flow.

Candidate store methods:
```go
PersistentAnonymousReachability() map[types.ObjID]struct{}
ExpandAnonymousReachability(reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{})
AnonymousRecycleCandidates(reachable map[types.ObjID]struct{}, minID types.ObjID) []types.ObjID
PersistentWaifRoots() []types.WaifValue
```

Search gates:
```powershell
rg -n --pcre2 "store\.(All|GetUnsafe|GetAnonymousObjects)\(|\.(Properties|Flags|Anonymous|Recycled)\b" vm/anonymous_gc.go server/waif_lifecycle.go --glob "!**/*_test.go"
```

Expected:
- Zero direct object scans or unsafe object reads in the active slice.
- Remaining live VM value traversal is allowed and recorded.

Runtime gates:
```powershell
go test ./db ./vm ./server
go test -timeout 120s ./vm -run "Test.*(Anonymous|Waif|GC|Finalization)"
go test -timeout 120s ./server -run "Test.*(Waif|Anonymous|Recycle|Finalization)"
go build -o barn.exe ./cmd/barn/
uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
git diff --check
```

## Phase 4 - Small Runtime Permission and Context Stragglers

Status: pending.

Target owner:
- `db.Store` owns scalar object permission and relationship reads everywhere in runtime code.

Slice boundary:
- `server/scheduler.go`
- `builtins/tasks.go`
- `builtins/signatures.go`
- `builtins/objects_players.go`
- `reports/cleanup-store-runtime-access.md`

Work:
- Replace `server/scheduler.go` player location `store.Get` with `store.Location`.
- Replace `builtins/tasks.go` wizard flag checks with `store.HasObjectFlag`.
- Replace `builtins/signatures.go` player owner check with `store.ObjectOwner`.
- Decide whether `builtins/objects_players.go` `Players()` is acceptable as a store-owned query or whether it should become `PlayerObjectIDs()` for clarity.

Search gates:
```powershell
rg -n --pcre2 "store\.Get\(|\.(Location|Flags|Owner)\b" server/scheduler.go builtins/tasks.go builtins/signatures.go builtins/objects_players.go --glob "!**/*_test.go"
```

Runtime gates:
```powershell
go test ./db ./builtins ./server
go test -timeout 120s ./builtins -run "Test.*(Task|Read|Player|Perm|Signature)"
go test -timeout 120s ./server -run "Test.*Command"
go build -o barn.exe ./cmd/barn/
uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
git diff --check
```

## Phase 5 - Runtime Fixed-Point Audit

Status: pending.

Target owner:
- Production runtime code outside `db` no longer depends on mutable `*db.Object` internals.

Slice boundary:
- `builtins`
- `server`
- `vm`
- `conformance/setup.go` only if fixture setup is in scope
- `cmd` only for recorded debug surfaces

Work:
- Run broad searches after Phases 1-4.
- Classify every remaining hit as:
  - production runtime violation;
  - `db` owner boundary;
  - reader/writer/snapshot/load boundary;
  - debug/CLI surface;
  - test/conformance fixture setup;
  - non-object type with same field name.
- Move or delete production violations.
- Leave debug/test surfaces only if explicitly recorded.

Search gates:
```powershell
rg -n --pcre2 "\b(store|s\.store|vm\.Store)\.(Get|GetUnsafe|All|GetAnonymousObjects)\(" builtins server vm --glob "!**/*_test.go"
rg -n --pcre2 "\.(Properties|PropOrder|PropDefsCount|Verbs|VerbList|Parents|Children|Contents|Location|Flags|Anonymous|Owner|Name|Recycled|ChparentChildren)\b" builtins server vm --glob "!**/*_test.go"
```

Runtime gates:
```powershell
go test ./db ./builtins ./vm ./server
go build -o barn.exe ./cmd/barn/
uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
git diff --check
```

## Phase 6 - Snapshot/Checkpoint Ownership Reassessment

Status: pending.

Entry criteria:
- Phases 1-5 are complete or explicitly deferred.
- Runtime callers no longer require `Store.Get` object internals.

Target owner:
- `db.Store` owns live world state.
- `db.Writer` owns v17 snapshot serialization from a stable store snapshot.
- `db.Database` readers own v4/v5/v17 import and repair.
- `server` owns checkpoint timing and lifecycle hooks.

Work:
- Reassess `NewWriter`, task serialization, and checkpoint orchestration.
- Decide whether checkpoint manager code or server checkpoint code is duplicated; delete the loser if one exists.
- Keep suspended-task serialization limitations explicit.
- Do not add backend pluggability here unless the entry criteria prove a narrow complete store contract.

Search gates:
```powershell
rg -n --pcre2 "NewWriter\(|WriteDatabase\(|SetTaskSource|Checkpoint|checkpoint" db server cmd --glob "!**/*_test.go"
rg -n --pcre2 "store\.snapshot\(|storeSnapshot|cloneObjectForSnapshot" db server cmd --glob "!**/*_test.go"
```

Runtime gates:
```powershell
go test ./db ./server
go test -timeout 120s ./db -run "Test.*(RoundTrip|Load|Repair|Persistence|Task)"
go build -o barn.exe ./cmd/barn/
uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
git diff --check
```

## Backend Pluggability Rule

Do not begin backend-pluggability implementation from this plan.

Reassess only after:
- Active production runtime code no longer reads mutable object internals from `Store.Get`.
- Property, verb, relationship, lifecycle, anonymous/WAIF reachability, and checkpoint/snapshot ownership are store-owned or explicitly bounded.
- Search gates prove remaining object internals are only in `db`, debug/CLI, or tests.

Possible later outcomes:
- Keep in-memory runtime and improve snapshot/checkpoint persistence only.
- Add a derived query/index layer for introspection.
- Add a live backend interface only if the store contract is narrow, complete, and not bypassed by pointer access.

## Completion Criteria

- Every phase is complete or explicitly deferred by the user.
- Runtime production code outside `db` does not rely on mutable object internals returned by `Store.Get`.
- Every remaining direct field hit is recorded as `db` owner, reader/writer/snapshot/load boundary, debug/CLI, test/conformance setup, or non-object field.
- Runtime gates pass.
- Full managed conformance passes after the final implemented phase.
- `reports/cleanup-store-runtime-access.md` records every slice disposition, gate result, commit, and next action.
