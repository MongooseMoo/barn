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
- `6a15a82 Move first metadata reads into store`

Next slice:
- Phase 2 permission-heavy runtime metadata reads in `builtins/objects.go`, `builtins/objects_players.go`, `builtins/properties.go`, `builtins/verbs.go`, `vm/registry.go`, `vm/op_verb.go`, and server scheduler/login/call-verb surfaces.

## Iteration 2 - Phase 2 permission-heavy metadata reads

Slice read:
- `plans/store-metadata-read-cleanup-plan.md`
- `builtins/objects.go`
- `builtins/objects_players.go`
- `builtins/properties.go`
- `builtins/verbs.go`
- `vm/registry.go`
- `vm/op_verb.go`
- `server/scheduler.go`
- `server/scheduler_login.go`
- `server/scheduler_call_verb.go`

Surfaces:
- `builtins/objects_players.go` player and wizard flag checks
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote wizard, player, and anonymous checks to use `HasObjectFlag`, `ObjectIsAnonymous`, and `Valid`.
  - Evidence: The scoped Phase 2 search reports no object metadata field reads in this file.
- `builtins/objects.go` `create()` owner and parent permission checks
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote explicit owner validation to `Valid`, and parent owner/fertile/anonymous permission checks to `ObjectOwner` and `HasObjectFlag`.
  - Evidence: The scoped Phase 2 search reports no object metadata field reads in the converted permission path.
- `builtins/properties.go` property permission checks
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote programmer wizard checks and `add_property` anonymous guard to store-owned metadata methods.
  - Evidence: Remaining hits are `Property.Owner`, not object metadata.
- `builtins/verbs.go` verb permission checks
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote object readable, write, owner, and anonymous checks in `respond_to` and `add_verb` to store-owned metadata methods.
  - Evidence: Remaining hits are `Verb.Owner`, not object metadata.
- `vm/registry.go` programmer permission check
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote programmer flag validation to `HasObjectFlag`.
  - Evidence: The file has no remaining object metadata field hits.
- `vm/op_verb.go` and server scheduler call/login helpers
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Rewrote verb-owner wizard checks, login player flag validation, server-call anonymous `this`, and scheduler `isWizard` to use store-owned metadata methods.
  - Evidence: Remaining hits are `Verb.Owner` and task owner fields.
- Out-of-scope hits from the broad Phase 2 gate
  - Disposition: keep for later slices
  - Owner after cleanup: pending
  - Action: Recorded `vm/anonymous_gc.go`, `builtins/tasks.go`, `builtins/signatures.go`, `builtins/objects_misc.go`, `builtins/objects_hierarchy.go`, and scheduler task factory/runtime surfaces as outside this Phase 2 boundary.
  - Evidence: The broad gate reports these as remaining object metadata, task owner, waif owner, or Phase 3 world-scan/debug surfaces.

Gate results:
- Pass with recorded deferrals: `rg -n --pcre2 "\.(Owner|Flags|Anonymous)\b|\.Flags\.Has" builtins/objects.go builtins/objects_players.go builtins/properties.go builtins/verbs.go vm/registry.go vm/op_verb.go server/scheduler.go server/scheduler_login.go server/scheduler_call_verb.go --glob "!**/*_test.go"`
  - Remaining Phase 2 boundary hits are `Property.Owner`, `Verb.Owner`, and task owner fields.
- Pass with recorded deferrals: `rg -n --pcre2 "\.(Owner|Flags|Anonymous)\b|\.Flags\.Has" builtins vm server --glob "!**/*_test.go"`
  - Remaining broad hits include Phase 3 `objects_hierarchy.go` and `objects_misc.go`, future anonymous-GC/task/signature surfaces, plus `Property.Owner`, `Verb.Owner`, waif owner, and task owner fields.
- Pass: `go test ./db ./builtins ./vm ./server`
- Pass: `go test -timeout 120s ./builtins -run "Test.*(Property|Verb|Player|Object|Perm|Wizard)"` (`[no tests to run]`)
- Pass: `go test -timeout 120s ./vm -run "Test.*(Verb|Property|Perm|Wizard)"`
- Pass: `go build -o barn.exe ./cmd/barn/`
- Pass: `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"` (`3871 passed, 131 skipped in 141.40s`)
- Pass: `git diff --check`

Commit:
- `5816e96 Move permission metadata reads into store`

Next slice:
- Phase 3 name reads and world-scan queries in `builtins/objects_hierarchy.go`, `server/matcher.go`, and `builtins/objects_misc.go`.

## Iteration 3 - Phase 3 name reads and world-scan queries

Slice read:
- `plans/store-metadata-read-cleanup-plan.md`
- `db/store.go`
- `builtins/objects_hierarchy.go`
- `server/matcher.go`
- `builtins/objects_misc.go`

Surfaces:
- `db.Store` world-scan and alias read methods
  - Disposition: move
  - Owner after cleanup: `db.Store`
  - Action: Added `ObjectIDsByNameSubstring`, `ObjectsOwnedBy`, `AliasStrings`, and `ObjectByteEstimate`.
  - Evidence: Callers no longer scan object `Name`, `Owner`, `Properties["aliases"]`, or byte-estimate internals directly.
- `builtins/objects_hierarchy.go` `locate_by_name`
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Replaced direct `store.All()` plus `obj.Name` scan with `ObjectIDsByNameSubstring`.
  - Evidence: Targeted field search has no direct object metadata hits in this function.
- `builtins/objects_hierarchy.go` `owned_objects`
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Replaced direct `store.All()` plus `obj.Owner` scan with `ObjectsOwnedBy`.
  - Evidence: Targeted field search has no direct object metadata hits in this function.
- `server/matcher.go` command object matching
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Replaced direct object content/name/alias reads with `Contents`, `ObjectName`, and `AliasStrings`; deleted the caller-local `getAliases` object helper.
  - Evidence: `server/matcher.go` has no remaining `store.Get` or direct object metadata field hits.
- `builtins/objects_misc.go` `new_waif` and `object_bytes`
  - Disposition: rewrite
  - Owner after cleanup: `db.Store`
  - Action: Replaced anonymous class validation with `ObjectIsAnonymous`; moved the approximate object byte estimator into `db.Store`.
  - Evidence: Deleted caller-local `calculateObjectBytes` and `calculateValueBytes`; `objects_misc.go` has no remaining direct object metadata field hits.
- `builtins/objects_hierarchy.go` property-conflict traversal
  - Disposition: keep for later property-structure slice
  - Owner after cleanup: pending store-owned property-structure query methods
  - Action: Recorded remaining `store.Get` calls as property-map and chparent-child traversal, not scalar object metadata reads.
  - Evidence: The Phase 3 search gate reports `store.Get` at lines for `chparent`, `chparents`, `collectAncestorProperties`, and `hasChparentDescendantConflict`.
- Broad remaining hits outside this plan
  - Disposition: keep recorded
  - Owner after cleanup: pending future slices or non-object surfaces
  - Action: Recorded remaining broad-audit hits in tasks, signatures, anonymous GC, scheduler task ownership, `Property.Owner`, `Verb.Owner`, waif owner, AST/file names, and file I/O names.
  - Evidence: These are outside the Phase 3 boundary or are non-object metadata surfaces matched by the broad regex.

Gate results:
- Pass with recorded property-structure deferrals: `rg -n --pcre2 "\.(Name|Owner|Flags|Anonymous)\b|\.Flags\.Has|store\.Get\(" builtins/objects_hierarchy.go server/matcher.go builtins/objects_misc.go --glob "!**/*_test.go"`
  - Remaining targeted hits are `store.Get` calls in property-conflict traversal only.
- Pass: `rg -n --pcre2 "(?<!waif)\.(Name|Owner|Flags|Anonymous)\b|\.Flags\.Has" builtins/objects_hierarchy.go server/matcher.go builtins/objects_misc.go --glob "!**/*_test.go"` (zero hits)
- Pass with recorded deferrals: `rg -n --pcre2 "(?<!waif)\.(Name|Owner|Flags|Anonymous)\b|\.Flags\.Has" builtins vm server --glob "!**/*_test.go"`
  - Remaining broad hits are non-object names, task owner fields, `Property.Owner`, `Verb.Owner`, waif owner, anonymous-GC direct object internals, task/signature direct object internals, and file/stat names.
- Pass: `go test ./db ./builtins ./vm ./server`
- Pass: `go test -timeout 120s ./builtins -run "Test.*(Locate|Owned|Object|Bytes)"` (`[no tests to run]`)
- Pass: `go test -timeout 120s ./server -run "Test.*(Match|Command)"`
- Pass: `go build -o barn.exe ./cmd/barn/`
- Pass: `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"` (`3871 passed, 131 skipped in 141.53s`)
- Pass: `git diff --check`

Commit:
- `fbca560 Move world-scan metadata reads into store`

Next slice:
- This metadata-read plan is complete.
