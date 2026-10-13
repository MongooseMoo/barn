# Issue 219 checkpoint - 2026-08-14

## Request and delivery scope

- Resolve GitHub issue #219 with architectural fidelity and TDD.
- Commit the work, push a branch, and open a PR containing an explicit `Closes #219` phrase.
- Do not merge unless separately requested.

## Isolation and current state

- The primary checkout is on `master` at `ac3c7311e6170d4bee471d11bce133fe17598185` and contains extensive unrelated untracked files.
- Work is isolated in linked worktree `C:\Users\Q\code\barn-issue-219` on branch `agent/store-txn-non-optional`, based on the same current `origin/master` commit.
- No open PR currently uses that branch.
- Red TDD commit: `4cdadf2 Test non-optional store transaction architecture`.

## Findings

- Live source measurement confirms 114 exported `*Store` methods, 76 exported `*StoreTxn` methods, and exactly 56 shared names. The shared names exactly match issue #219.
- The issue requires a direct/pass-through transaction, always-populated runtime `TaskContext.StoreTxn`, deletion of nil selectors in VM/builtins, and deletion of the 56 exported `Store` mirrors.
- Explicitly forbidden: interfaces, adapters, wrappers, shims, aliases, compatibility branches, fallback readers, or dual runtime paths.
- `StoreTxn` methods for `ExpandAnonymousReachability` and `ObjectByteEstimate` are located outside `store_txn.go`, which explains why an initial single-file scan appeared to miss them; the all-file measurement found the full 56.
- Runtime tasks already create snapshot transactions in `engine.runTask`; direct VM/eval/test contexts are the important nil path that needs the store-owned direct transaction.

## Red contract evidence

Command:

```powershell
go test ./db/store ./vm -run 'Test(DirectTxn|StoreDoesNotMirror|EnsureContextDependencies)' -count=1
```

Expected and observed failure after correcting one test typo:

```text
db/store/store_direct_txn_test.go: s.DirectTxn undefined
vm/vm_store_txn_test.go: store.DirectTxn undefined
FAIL github.com/MongooseMoo/barn/db/store
FAIL github.com/MongooseMoo/barn/vm
```

## Next action

Implement one allocation-free direct `StoreTxn` owned by `Store`, route its 56 runtime methods directly to live state, populate it in VM contexts lacking a snapshot, then remove nil-selection branches and rename/delete the 56 exported `Store` mirrors. Compile failures will drive migration of remaining callers to the transaction spelling. Run focused tests, full Go verification, static searches, benchmarks, and the documented managed conformance command before publishing.

## Implementation checkpoint

- Added a store-owned singleton `directTxn` and `Store.DirectTxn()`; the focused write-through and VM context tests now pass.
- Direct transaction lifecycle is allocation-free and commit/release are no-ops.
- Added pass-through branches for the 56 shared methods and direct handling for transaction lifecycle operations reached by non-task execution.
- Renamed all 56 exported `Store` mirror declarations to private live-store implementations. `go build ./db/store` passes after updating its three internal property calls.
- `builtins.Registry.NewExecution` now fills a missing transaction from `ctx.Store.DirectTxn()`.
- `builtins/store_reads.go` now uses only `StoreTxn`; its read fallback branches are deleted.
- Current expected build state: `go build ./...` fails where production and test callers still use the removed exported `Store` methods. Those compile errors are the active migration list, not an unknown runtime failure.

## Immediate next action

Finish the production caller migration package by package, preferring the existing task transaction and using `Store.DirectTxn()` only at non-task host/CLI boundaries. Then build all packages, migrate tests, and run the red architecture suite.

## Production migration checkpoint

- `builtins/store_reads.go` is single-spelled on `StoreTxn` and VM property/index/misc/verb helpers no longer accept `(store, txn)` pairs.
- Deleted `vm.storeTxn()` and routed VM operations through the guaranteed context transaction.
- Engine in-task verb access now uses `TaskContext.StoreTxn`; task-factory and standalone hook lookups use the store-owned direct transaction.
- Server, command parsing, CLI inspection tools, anonymous-GC live walks, and internal app host boundaries now explicitly use `Store.DirectTxn()`.
- Added `StoreTxn.CreateAnonymousObject` because anonymous allocation remains live/out-of-band but must still be exposed through the single runtime transaction type. Numbered direct creation remains `StoreTxn.CreateObject`.
- `go build ./builtins` and `go build ./vm` pass. The most recent all-package build reached `internal/app`; its remaining direct-store calls have now been migrated but the all-package build has not yet been rerun.

## Immediate next action

Run `go build ./...`, repair any remaining production compile sites, then compile tests to migrate test fixtures from the removed `Store` method surface. After compilation, remove remaining nil selectors in builtins/engine and exercise direct-mode paths beyond the initial smoke test.

## Test migration and architecture checkpoint

- `go build ./...` passes.
- `go test ./... -run '^$'` passes after migrating 576 test call sites across 63 files from removed `Store` methods to `Store.DirectTxn()`; transaction receivers were explicitly excluded from the mechanical rewrite.
- The original red contracts pass.
- Live method-set measurement is now `Store=59 StoreTxn=76 Shared=0`.
- Added direct-mode no-ops for transaction adoption/staging/lifecycle methods and a live implementation for `StoreTxn.AnonymousChildren`, preventing the shared direct transaction from accumulating mutable snapshot state.
- Converted VM property/index/misc/verb helpers and builtins store-read helpers to a single non-nil transaction parameter.
- Started deleting residual optional-transaction branches from builtins mutation paths. `objects.go`, `objects_misc.go`, and `objects_players.go` build after conversion.
- Current expected build failure is only four now-unused `store := ctx.Store` declarations in `builtins/properties.go` after its transaction fallback branches were deleted.

## Immediate next action

Remove those unused declarations, finish optional-selector deletion in hierarchy/movement/verbs/protected and engine lifecycle gates, then run focused direct-mode behavior tests and the full suite. Pay special attention to direct execution side effects such as player boot behavior, because the transaction is no longer identified by nil.

## Optional-selector cleanup checkpoint

- Removed optional transaction selection from property, object, hierarchy, movement, player-flag, and verb mutation paths.
- Direct-mode adoption, staging, commit, and release operations are safe no-ops; snapshot behavior remains unchanged.
- Moved atomic direct `delete_verb` authority/identity validation behind `StoreTxn.DeleteResolvedVerbAuthorized`; the live-store helper is now private, preserving a single exported runtime spelling.
- Identified a semantic distinction that nil previously encoded for host side effects: retryable snapshot tasks defer notification/boot/switch effects, while direct host execution applies them immediately.
- Added `StoreTxn.IsDirect()` as an explicit mode query. Network and player-flag code now branches on direct-vs-snapshot mode, not presence-vs-absence of a transaction. This is not a second database path; all database access remains on `StoreTxn`.
- Current source still needs formatting/build after the latest `IsDirect` patch. Remaining nil selectors are expected mainly in engine lifecycle guards and the `Registry.NewExecution` invariant-establishment boundary.

## Immediate next action

Format/build, add direct-mode side-effect tests if existing coverage does not already prove behavior, remove engine `StoreTxn != nil` guards where task contexts are guaranteed populated, and run all focused/full verification.

## Blockers

- None currently.

## Conformance, benchmark, and staging checkpoint

- The documented managed conformance command completed on the feature worktree with `101 failed, 12249 passed, 412 skipped`.
- The identical command at clean `origin/master` produced the same counts and the exact same 101 failed-test names; both `failed-tests.txt` files have SHA-256 `3CAF1CB27126AA48961916E3D7A3C89834013F86E0F13582885AFBC1E54BAF40`.
- Existing transaction benchmarks were run five times on clean base and feature. Allocation counts are unchanged. Median hot-path timings were within noise or improved; the only slower median was uncached verb ancestry at approximately 0.7 percent.
- Production diff was reviewed in store, builtins, VM, engine, and host-boundary chunks. Snapshot tasks use `StoreTxn`; host/startup/CLI paths use the store-owned direct transaction; no compatibility interface, adapter, wrapper, fallback reader, or dual exported runtime surface was added.
- `uv.lock` was changed by the fresh conformance environment and restored before staging.
- All 109 changed Go files report formatted, `git diff --check` is clean, and exactly 109 tracked issue files are staged. Generated `reports/runs/` remains untracked and is not staged.

## Immediate next action

Inspect the staged diff/commit boundary, commit the green implementation, rerun fresh post-commit verification, audit branch commits and issue-closing PR text, then push and open the PR subject to the user's publish-text guardrail.

## Blockers

- The repository-wide conformance gate is red on `origin/master` with the exact same 101 failures; issue 219 introduces zero additional failed rows.

## Published PR checkpoint

- Red contract commit: `4cdadf2ec1ef60095d66dbf17f53cb051ca0e404` (`Test non-optional store transaction architecture`).
- Green implementation commit: `2ce3f2c5fdfdd53f08ab3972839aaa972dc64686` (`Make store transactions non-optional`).
- Branch `agent/store-txn-non-optional` was pushed without rewriting either commit.
- Draft PR `#226`, `Make StoreTxn non-optional`, is open at `https://github.com/MongooseMoo/barn/pull/226` with `Closes #219.` and GitHub reports issue 219 in `closingIssuesReferences`.
- Exact PR head is `2ce3f2c5fdfdd53f08ab3972839aaa972dc64686`.
- GitHub checks at that head: `Build & unit tests` passed in 1m41s; `MOO conformance suite` passed in 4m43s.

## Immediate next action

Perform one final read-only audit of PR metadata, commit/file scope, and local status, then hand off the draft PR. Do not merge; the user did not request merge.

## Focused behavior checkpoint

- The focused direct-mode builtins regressions now pass after making the shared network test fixture satisfy the production invariant with `Store.DirectTxn()`.
- Lifecycle audit found a pre-commit semantic defect: live `Store.ChangeParents` already calls `reseedInheritedPropertiesLocked`, while the direct transaction's `ReseedInheritedProperties` branch currently calls `Store.ResetInheritedProperties` afterward. That would remove inherited slots and does not match the old nil-transaction live path.
- Direct-mode `ReseedInheritedProperties` must therefore be a no-op; snapshot-mode reseeding remains unchanged.

## Immediate next action

Add a regression test proving direct parent changes retain inherited properties, change the direct reseed hook to return `E_NONE`, then rerun focused builtins/store tests and continue the full verification and diff audit.

## Verification checkpoint

- Added a direct parent-change regression that first failed because `ReseedInheritedProperties` removed the live inherited slot, then passed after making direct reseeding a no-op.
- Source method-set measurement: `Store=58 StoreTxn=78 Shared=0`.
- The only remaining `StoreTxn == nil` checks are invariant establishment in `vm/vm.go` and `builtins/registry.go`; the issue-named property/index/misc/store-read selectors are gone.
- Focused store/builtins/VM/engine/server tests pass.
- `go test ./... -count=1` passes.
- `go test -race ./db/store ./builtins ./vm ./engine -count=1` passes; the engine package completed in 252.814 seconds.
- `go vet ./...` emitted no diagnostics.
- `staticcheck.exe` is available at `C:\Users\Q\go\bin\staticcheck.exe` but has not yet been run.

## Immediate next action

Run `staticcheck ./...`, identify and run the documented managed conformance command, compare the existing store benchmarks against a clean base worktree, then audit the final diff and commit the green implementation.

## Blockers

- None currently.
