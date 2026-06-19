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

Status: ready to commit.

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
- Commit Phase 2 code and record.
- Reread `plans/store-runtime-access-cleanup-plan.md`.
- Continue with Phase 3 anonymous and WAIF reachability.
