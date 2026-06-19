# Store Runtime Access Cleanup Report

## Phase 0 - Plan Commit

- Commit: `087190a Plan store runtime access cleanup`
- Gate: `git diff --check -- plans/store-runtime-access-cleanup-plan.md` passed.

## Phase 1 - Property Structure Queries

Status: committed.

Commit:
- `fc3576a Move property structure reads into store`

Changes:
- Added store-owned property traversal queries in `db.Store`:
  - `DefinedPropertyNamesInAncestry`
  - `HasDuplicateDefinedPropertyAmong`
  - `HasDefinedPropertyConflictWithAncestry`
  - `HasChparentDescendantPropertyConflict`
  - `PropertyValue`
  - `PropertyValues`
  - `TruthyPropertiesWithPrefixInAncestry`
- Replaced `create()` duplicate parent property checks with `db.Store`.
- Replaced `recycle()` property-value traversal with `db.Store`.
- Replaced `chparent()` and `chparents()` property conflict traversal with `db.Store`.
- Deleted caller-local `collectAncestorProperties` and `hasChparentDescendantConflict`.
- Replaced protected builtin `protect_*` traversal with `db.Store`.
- Deleted caller-local `collectProtectFlags`.
- Replaced primitive prototype lookup in `vm/op_misc.go` with `db.Store.PropertyValue`.

Search gates:
- `rg -n --pcre2 "\.(Properties|PropOrder|PropDefsCount|ChparentChildren)\b|store\.Get\(" builtins/objects.go builtins/objects_hierarchy.go builtins/protected.go vm/op_misc.go --glob "!**/*_test.go"`: no matches.
- `rg -n --pcre2 "collectAncestorProperties|hasChparentDescendantConflict|collectProtectFlags" builtins vm server --glob "!**/*_test.go"`: no matches.

Runtime gates:
- `go test ./db ./builtins ./vm ./server`: passed.
- `go test -timeout 120s ./builtins -run "Test.*(Create|Parent|Property|Protected|Object)"`: passed, no tests matched.
- `go test -timeout 120s ./vm -run "Test.*(Property|Primitive|Prototype)"`: passed, no tests matched.
- `go build -o barn.exe ./cmd/barn/`: passed.
- `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"`: passed, `3871 passed, 131 skipped in 143.71s`.
- `git diff --check`: passed.

Disposition:
- Kept. Phase 1 removes production property-structure reads from the active slice without shims, fallback paths, or backend interfaces.

Next action:
- Reread `plans/store-runtime-access-cleanup-plan.md`.
- Continue with Phase 2 verb dispatch and hook queries.

## Phase 2 - Verb Dispatch and Hook Queries

Status: committed.

Commit:
- `e69dd32 Move verb dispatch reads into store`

Changes:
- Added store-owned verb/runtime query methods in `db.Store`:
  - `ObjectExists`
  - `HasLocalVerb`
  - `HasVerbNameInAncestry`
  - `VerbCandidatesInAncestry`
- Replaced `server/verbs.go` caller-side verb ancestry traversal with store-owned verb candidate and name queries.
- Deleted the old `hasVerbNameOnObject` helper.
- Renamed the remaining server dispatch helper to describe command policy, not object traversal.
- Replaced lifecycle/checkpoint hook direct verb-map checks with `HasLocalVerb`.
- Replaced login handler object and `do_login_command` direct verb-map checks with `ObjectExists` and `HasLocalVerb`.
- Replaced `builtins/verbs.go` existence-only `store.Get` checks with `ObjectExists`.

Search gates:
- `rg -n --pcre2 "\.(Verbs|VerbList)\b|store\.Get\(" server/verbs.go server/server.go server/scheduler_login.go builtins/verbs.go --glob "!**/*_test.go"`: no matches.
- `rg -n --pcre2 "hasVerbNameOnObject|findVerbOnObject" server --glob "!**/*_test.go"`: no matches.

Runtime gates:
- `go test ./db ./builtins ./vm ./server`: passed.
- `go test -timeout 120s ./builtins -run "Test.*Verb"`: passed, no tests matched.
- `go test -timeout 120s ./server -run "Test.*(Verb|Command|Hook|Login)"`: passed.
- `go build -o barn.exe ./cmd/barn/`: passed.
- `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"`: passed, `3871 passed, 131 skipped in 143.73s`.
- `git diff --check`: passed.

Disposition:
- Kept. Phase 2 removes production verb-structure reads from the active slice without shims, fallback paths, or backend interfaces.

Next action:
- Reread `plans/store-runtime-access-cleanup-plan.md`.
- Continue with Phase 3 anonymous and WAIF reachability.

## Phase 3 - Anonymous and WAIF Reachability

Status: committed.

Commit:
- `edef91b Move anonymous reachability reads into store`

Changes:
- Added store-owned persistent anonymous reachability queries:
  - `PersistentAnonymousReachability`
  - `ExpandAnonymousReachability`
  - `UnreachableAnonymousValues`
  - `AnonymousRecycleCandidates`
- Added store-owned persistent WAIF root query:
  - `PersistentWaifRoots`
- Replaced `vm/anonymous_gc.go` persistent object/property scans and anonymous candidate enumeration with store-owned queries.
- Replaced `server/waif_lifecycle.go` persistent WAIF object/property scan with `PersistentWaifRoots`.
- Kept live VM stack/root traversal in `vm`.
- Kept lifecycle invocation policy in `server` and builtin recycle paths.

Search gate:
- `rg -n --pcre2 "store\.(All|GetUnsafe|GetAnonymousObjects)\(|\.(Properties|Flags|Anonymous|Recycled)\b" vm/anonymous_gc.go server/waif_lifecycle.go --glob "!**/*_test.go"`: no matches.

Runtime gates:
- `go test ./db ./vm ./server`: passed.
- `go test -timeout 120s ./vm -run "Test.*(Anonymous|Waif|GC|Finalization)"`: passed.
- `go test -timeout 120s ./server -run "Test.*(Waif|Anonymous|Recycle|Finalization)"`: passed, no tests matched.
- `go build -o barn.exe ./cmd/barn/`: passed.
- `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"`: passed, `3871 passed, 131 skipped in 142.74s`.
- `git diff --check`: passed.

Disposition:
- Kept. Phase 3 removes production persistent reachability object scans from the active slice without shims, fallback paths, or backend interfaces.

Next action:
- Reread `plans/store-runtime-access-cleanup-plan.md`.
- Continue with Phase 4 small runtime permission and context stragglers.

## Phase 4 - Small Runtime Permission and Context Stragglers

Status: committed.

Commit:
- `d99eede Move small runtime reads into store`

Changes:
- Replaced `server/scheduler.go` player `store.Get(...).Location` with `store.Location`.
- Replaced `builtins/tasks.go` wizard flag checks with `store.HasObjectFlag`.
- Replaced `builtins/tasks.go` eval-wrapper player location read with `store.Location`.
- Replaced `builtins/signatures.go` read permission owner check with `store.ObjectOwner`.
- Left `builtins/objects_players.go` unchanged because `store.Players()` is already a store-owned player query.

Search gate:
- `rg -n --pcre2 "store\.Get\(|\.(Location|Flags|Owner)\b" server/scheduler.go builtins/tasks.go builtins/signatures.go builtins/objects_players.go --glob "!**/*_test.go"`: remaining matches are `task.Task.Owner` fields and `store.Location(...)` method calls, not direct object internals.

Runtime gates:
- `go test ./db ./builtins ./server`: passed.
- `go test -timeout 120s ./builtins -run "Test.*(Task|Read|Player|Perm|Signature)"`: passed.
- `go test -timeout 120s ./server -run "Test.*Command"`: passed.
- `go build -o barn.exe ./cmd/barn/`: passed.
- `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"`: passed, `3871 passed, 131 skipped in 142.22s`.
- `git diff --check`: passed.

Disposition:
- Kept. Phase 4 removes the remaining planned small runtime object reads without shims, fallback paths, or backend interfaces.

Next action:
- Reread `plans/store-runtime-access-cleanup-plan.md`.
- Continue with Phase 5 runtime fixed-point audit.

## Phase 5 - Runtime Fixed-Point Audit

Status: ready to commit.

Changes:
- Replaced the remaining production `store.Get` existence checks in `builtins/properties.go` with `store.ObjectExists`.
- Replaced the remaining production `vm.Store.Get` object reads in `vm/op_property.go` with store-owned existence and property queries.
- Kept property and builtin-property policy in `builtins`/`vm`; only object existence and traversal moved behind `db.Store`.

Search gates:
- `rg -n --pcre2 "\b(store|s\.store|vm\.Store)\.(Get|GetUnsafe|All|GetAnonymousObjects)\(" builtins server vm --glob "!**/*_test.go"`: no matches.
- `rg -n --pcre2 "\.(PropOrder|PropDefsCount|ChparentChildren|VerbList|Properties|Verbs)\b" builtins server vm --glob "!**/*_test.go"`: no matches.
- `rg -n --pcre2 "\b(obj|object|playerObj|parent|child|current|systemObj|classObj|base|target|ownerObj)\.(Parents|Children|Contents|Location|Flags|Anonymous|Owner|Name|Recycled)\b" builtins server vm --glob "!**/*_test.go"`: remaining matches are compiler AST target `Name` fields, not object internals.
- Full field gate `rg -n --pcre2 "\.(Properties|PropOrder|PropDefsCount|Verbs|VerbList|Parents|Children|Contents|Location|Flags|Anonymous|Owner|Name|Recycled|ChparentChildren)\b" builtins server vm --glob "!**/*_test.go"`: remaining matches classify as non-object fields/accessors (`task.Task.Owner`, `db.Verb.Owner`, `db.Verb.Name`, `db.Property.Owner`, parser/compiler AST `Name`, WAIF value accessors, file stat names) or store-owned query method calls (`store.Location`, `store.Contents`, `store.Parents`, `store.Children`).

Runtime gates:
- `go test ./db ./builtins ./vm ./server`: passed.
- `go build -o barn.exe ./cmd/barn/`: passed.
- `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"`: passed, `3871 passed, 131 skipped in 142.51s`.
- `git diff --check`: passed.

Disposition:
- Kept. Phase 5 reaches the runtime fixed point for `builtins`, `server`, and `vm`: no production direct `Store.Get`, `Store.GetUnsafe`, `Store.All`, `Store.GetAnonymousObjects`, property-structure fields, or verb-structure fields remain outside `db`.

Next action:
- Commit Phase 5 code plus this record.
- Reread `plans/store-runtime-access-cleanup-plan.md`.
- Continue with Phase 6 snapshot/checkpoint ownership reassessment.
