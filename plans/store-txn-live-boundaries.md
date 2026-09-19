# StoreTxn live-state boundary contracts

Issue #313, final child of #309. Audited base:
`9923cb41fd2ef1cf3c49086aca8cd47f8fbc9420` (merged #312).

## Decision

Retain the distinct live-state boundaries. None of the remaining transitions below
has an equivalent contract that justifies deleting it or replacing it with another.
This change records the caller and ownership contracts; it changes no Go behavior
and introduces no forwarding facade. The shared validation and write-reset
sequences were already consolidated in #312.

The inventory describes the current implementation, not a new claim about MOO
behavior. Any future change to uncertain observable behavior must first establish
the expected result through the managed WSL Toast oracle, then add a failing
behavioral regression. No independent correctness bug is asserted by this audit.

## Production caller inventory

Paths are relative to the repository. Function names, rather than line numbers,
identify the call sites so the record survives mechanical relocation. This includes
all non-test calls to the eight operations and the carrying-reads variant at the
audited base.

| Operation | Every production caller | Why the caller needs this boundary |
| --- | --- | --- |
| `CommitAndRenew` | `db/store/store_txn_commit.go`: `CommitAndRenewCarryingReads`; `engine/runtime.go`: the `renewTransaction` closure in the `RunGC` host callback (called before and after the sweep); `builtins/store_reads.go`: `flushStagedBeforeCoarse` when exact verb deletions are staged | Publish through validation and replace the snapshot before continuing against live state. GC and exact deletion do not own the irreversible-boundary read-carrying policy. |
| `CommitAndRenewCarryingReads` | `engine/task_runtime.go`: `runTask`'s `BeforeIrreversibleEffect` closure | Validate even without writes, preserve prior dependencies, and continue under the exclusive escalation gate. |
| `FlushStagedToLive` | `builtins/store_reads.go`: `flushStagedBeforeCoarse`, topology branch without exact verb deletions | Intentional unvalidated immediate publication before a coarse operation. |
| `AdoptLiveObject` | `builtins/objects.go`: `builtinCreate`; `builtins/objects_hierarchy.go`: `builtinRecreate`; `builtins/objects_misc.go`: `builtinRenumber` | Bind a newly live identity or replacement object into this transaction. |
| `AdoptLiveVerbs` | `builtins/verbs.go`: `builtinAddVerb`, `builtinSetVerbInfo`, `builtinSetVerbArgs` | Refresh the changed local verb structure while retaining staged code writes. Exact `delete_verb` uses its validated staged-delete path instead. |
| `AdoptLiveRelationships` | `builtins/objects.go`: `builtinCreate`, `FinishRecycleLifecycle`, `applyRecycleMove`; `builtins/objects_hierarchy.go`: `builtinChparent`, `builtinChparents`, `builtinRecreate`; `builtins/objects_misc.go`: `builtinRenumber`; `builtins/objects_movement.go`: `ApplyMoveLifecycle` | Reconcile only the relationship facets changed by the caller's live operation, including affected relatives. |
| `ForgetObject` | `builtins/objects.go`: coarse branch of `FinishRecycleLifecycle`; `builtins/objects_misc.go`: `builtinRenumber` | Invalidate the old identity and remove its targeted staged/read bookkeeping. The decentralized recycle path deliberately retains its conflict guards. |
| `MoveStagedProperties` | `builtins/objects_misc.go`: `builtinRenumber` | Re-key staged property operations before forgetting the old identity. |
| `ApplyStagedProperties` | `builtins/objects_misc.go`: `builtinRenumber` | Overlay those operations onto the adopted new private view, without publishing them. |

Reproduce the direct-call census with:

```sh
rg -n 'CommitAndRenew|FlushStagedToLive|AdoptLive(Object|Verbs|Relationships)|ForgetObject|MoveStagedProperties|ApplyStagedProperties' . --glob '*.go' --glob '!**/*_test.go'
```

## Publication, failure and snapshot ownership

These contracts concern store-backed snapshot transactions. Direct transactions
remain the existing pass-through API; renewal returns the same direct transaction
without publication and the reconciliation operations are no-ops for direct mode.
None of these StoreTxn helpers owns the task's pending effects or fork lifecycle.

| Boundary | Publication and failure | Timestamp, registration and returned transaction |
| --- | --- | --- |
| `CommitAndRenew` | With writes, uses ordinary validated `Commit`: ordered read validation, operation preflight, then clock allocation/publication. A conflict leaves the original private view and staged writes; a terminal operation failure also keeps diagnostic state but prevents re-publication. Without writes it does not validate the old reads. | Success releases the old registration and returns `BeginSnapshot(0)`, registered at the current clock with fresh caches/read sets. Carries the caller's gate exemption. Failure returns the original unreleased transaction and `publishedWrites=false`. Caller must install `next` only on success. |
| `CommitAndRenewCarryingReads` | First validates even a transaction with no writes, materializes memoized ancestry dependencies, then delegates publication/renewal to `CommitAndRenew`. A validation loss is retryable and leaves the original transaction. | Same registration/return ownership as renewal, but transfers all seven read maps. Read versions for this transaction's own published footprint are rebased; deleted properties/verbs and recycled objects lose the corresponding marks. Unrelated dependencies remain. New object caches and resolution memo are not carried. |
| `FlushStagedToLive` | No read-set validation. Preflights the complete operation footprint before apply/clock allocation. Rejects staged exact verb deletions. Failure keeps private state; non-validation failures become terminal. Allocated-ID occupancy remains the existing retryable preflight conflict. Successful apply clears staged writes. | Keeps the same transaction, `readTS`, registration and gate exemption. Recreates the read maps, refreshes cached numbered objects from current live state (including recycled tombstones), retains cached anonymous entries, and resets `owned`. It does not return a replacement or call `Release`. No staged writes means no refresh. |

`markTerminal` changes the publication eligibility (`HasWrites` becomes false), not
the private object maps available to an error handler. A later `Commit` returns the
stored terminal error. Validation conflicts use `ValidationFailed`; error code
alone is insufficient to decide whether a task can retry. `Commit` invalidates
resolution caches before validating, whereas flush delays invalidation until
preflight succeeds: retaining the private view does not imply every memo field is
unchanged on every failure path.

Ordinary commits take the gate shared, then store locks, then numbered slot locks
where applicable. The irreversible boundary owns the exclusive gate and exempts
its current transaction; successful renewal preserves that exemption. StoreTxn
does not acquire or release the runtime's exclusive gate. `Release` deregisters
the read timestamp exactly once, with a finalizer backstop; released snapshots
must not be read again.

## Local reconciliation contracts

All six operations below keep the same transaction, timestamp, registration and
gate exemption; none publishes staged writes, validates the complete read set,
flushes/discards pending effects, or rolls back an already completed live mutation.
Each invalidates resolution caches. These are local reconciliation operations,
not failure-atomic publication transactions.

| Operation | Read/cache/ownership effect | Failure and anonymous-object handling |
| --- | --- | --- |
| `AdoptLiveObject` | Replaces the object binding with a clone of current live state; does not add an ownership mark or rebase the read maps. Advances local high-water bookkeeping; `maxObjID` excludes anonymous objects. | Resolves through `liveObjectLocked`, including the anonymous map. Missing/invalid live object leaves a nil binding and returns `E_INVIND`. |
| `AdoptLiveVerbs` | Privatizes the cached object, rebuilds the local verb list/map while preserving alias identity, reapplies staged code writes whose targets still exist, refreshes the verb scan and recorded verb versions, and drops reads for removed verbs. Other object facets stay private and unchanged by this adoption. | May invalidate/privatize before an error; not an atomic replacement guarantee. Uses anonymous-aware resolution, although builtin `add_verb` rejects anonymous targets. |
| `AdoptLiveRelationships` | Privatizes each cached object (or clones live if absent), replaces relationship facets, and updates only relationship read versions. Does not discard other staged operations or unrelated read dependencies. | Skips `ObjNothing`; resolves anonymous relatives through `liveObjectLocked`. A missing later object can return `E_INVIND` after earlier IDs were reconciled. Do not assume all-or-nothing adoption of the list. |
| `ForgetObject` | Sets the old binding nil; removes that ID's scalar/relationship reads and writes, property scans/reads/staged operations, verb scans/reads/code writes, and ordered verb deletions. Does not clear `createdObjects`, `recycleWrites`, ownership/high-water state or other IDs. | No publication/error return. The caller has already established the live lifecycle transition. Anonymous identities use the same bookkeeping; this is not anonymous GC. |
| `MoveStagedProperties` | Moves only property define/definition-delete/value-write/delete keys from old ID to new ID. Does not move read marks, object bindings, verb/scalar writes, or publish anything. | No-op for equal IDs; no object lookup or special anonymous resolution. This is renumber-specific ordering, not a general transaction remap. |
| `ApplyStagedProperties` | Privatizes an existing valid cached object if necessary; overlays defines, values and deletes on its properties/order. Keeps staged operations for later publication. | Returns without work if the cached object is invalid. No live lookup or publication; operates on the already adopted binding. |

The renumber sequence is deliberately explicit:
`Store.Renumber -> MarkLiveMutated -> MoveStagedProperties -> ForgetObject(old) ->
AdoptLiveObject(new) -> ApplyStagedProperties(new) -> AdoptLiveRelationships`.
Moving properties after forgetting would lose them; omitting the overlay would
lose read-your-writes at the new identity. Coarse recycle instead forgets the
recycled identity and adopts affected relatives. Decentralized recycle must not
forget the read guards used to detect competing topology changes.

## Runtime ownership and #305 constraints

`builtins/store_reads.go` owns `beforeCoarse`: cross the irreversible boundary,
prepare memo dependencies, then flush staged topology if any. Exact verb deletion
selects validated renewal; other topology selects immediate flush. Failure is
returned before `markLiveStoreMutated`. A successful publication marks the task
non-retryable and the transaction live-mutated. The helper itself does not flush
pending effects. The preceding runtime boundary may already have published them.

`engine/task_runtime.go` owns retry, final commit, suspension and error handling:

- A new attempt releases the prior transaction and installs `BeginSnapshot(0)`;
  a gated attempt takes its snapshot under the gate and marks it exempt.
- Before an irreversible effect, the runtime acquires the exclusive gate and
  calls carrying-reads renewal. A retryable validation loss aborts before the
  effect. Successful publication installs `next`, makes created forks durable,
  and flushes pending effects when writes were published.
- Retry discards only that attempt's pending effects/forks, restores task state,
  releases/replaces the old snapshot, and obeys the existing gate policy. A task
  with live mutation or an irreversible effect cannot be blindly re-executed.
- The ordinary completion boundary flushes pending effects after a successful
  commit outcome even when `HasWrites` is false. A task exception does not itself
  mean rollback: preceding valid staged writes still commit. Commit failure
  discards pending effects; terminal transactions cannot publish again.
- Inline yields and scheduler suspension have distinct VM/fork ownership. Their
  commit, snapshot replacement, effect flush and gate-release points remain
  explicit. The final defer releases the transaction held by the current context.

The `RunGC` host callback in `engine/runtime.go` renews before and after the live
sweep so callbacks see current state. It installs `next` and preserves the
live-mutated marker when needed, but does not itself flush pending effects. This
differs from the irreversible boundary's publication/flush contract.

`builtins/pending_effects.go` owns the task-local options snapshot and ordered
session publication. Reload affects the current task immediately; the session
changes only when pending effects flush. Last reload wins even after writes have
been elided. Flush clears the task-local view when consuming the queue; discard
clears it without session publication. StoreTxn renewal must not take ownership
of that queue or infer its emptiness from `HasWrites`.

## Consolidation candidates and disposition

| Candidate | Decision and concrete reason |
| --- | --- |
| Duplicate ordered validators and ten-field write resets | Already removed by #312: three validation sequences now share one helper; two reset sequences share one helper. Net 18 source lines removed, including helper comments. |
| Plain renewal vs carrying-reads renewal | Retain: no-write validation and transferred read dependencies differ. Delegation already shares the actual publication/replacement step. |
| Renewal vs flush | Retain: validation, exact-deletion admission, read-set disposal, timestamp registration and cache refresh differ. A common facade would obscure those choices. |
| Whole-object vs verb vs relationship adoption | Retain: replacing an object is not equivalent to reconciling one facet while keeping other private writes and conflict guards. Their error/partial-progress contracts also differ. |
| Forget vs successful-write cleanup | Retain: targeted identity invalidation removes reads and selected writes; successful-publication cleanup clears all staged writes while preserving reads/cache state. |
| Move/apply staged properties | Retain both ordered renumber operations: one changes staged keys, the other changes the private read view. Their only caller makes that sequence visible without an extra facade. |
| Runtime renew/flush/discard sequences | Retain: GC, irreversible effects, retry, inline resume and suspension own different effects, forks, gate and VM lifetimes. A repeated `Release`/`BeginSnapshot` pair alone does not establish equivalence. |

No further source reduction is implemented in #313. This is the explicit
retain-and-document outcome allowed by the issue, not evidence that every
possible future simplification is impossible. Traversal/COW/cache redesign stays
with #267/#274.

## Regression evidence and reproduction

Run the existing behavioral suite, not tests of file placement or helper spelling:

```sh
go test ./...
go vet ./...
go test -race ./db/store ./builtins ./engine
git diff --check
```

| Constraint | Existing regression anchors |
| --- | --- |
| Preflight before clock/publication; terminal vs retryable failure; private error view | `db/store/store_txn_coarse_atomic_test.go`, `store_txn_terminal_test.go`, `store_txn_flush_view_test.go`; `engine/terminal_commit_test.go` |
| Snapshot replacement, registration and gate exemption | `TestStoreTxnCommitAndRenewConflictLeavesTransactionIntact`, `TestStoreTxnCommitAndRenewPublishesAndPreservesGateExemption`, `TestStoreTxnCommitAndRenewTerminalFailureKeepsOriginalUnreleased`; `db/store/escalation_gate_test.go` |
| Read/memo dependencies across renewal | `db/store/store_txn_boundary_renew_test.go`, including no-write stale reads and reads of untouched/republished objects |
| Anonymous adoption, renumber and create/read-your-writes | `TestTransactionAdoptAndCommitAnonymousObject`, `TestTransactionAdoptLiveRelationshipsRefreshesAnonymousChildAfterRenumber`, `TestTransactionRenumberLeavesOldObjectIDInvalid`; existing transaction and immutable-image tests |
| Exact successive deletion and surviving code | `builtins/verbs_txn_test.go`, `builtins/object_metadata_permissions_test.go`, `db/store/store_verbs_test.go` |
| Same-task options/protected builtins and session timing | `engine/server_options_same_task_test.go`, `engine/protected_redirect_test.go`; `builtins/system_test.go` |
| Add/load/delete/load and last reload after `HasWrites=false` | `TestLoadServerOptionsSeesSameTaskPropertyDelete`, `TestLoadServerOptionsAfterElidedWrites` |
| Effect ordering, discard, terminal release and fork durability | `engine/worker_pool_test.go`, `engine/nonretryable_conflict_test.go`; `builtins/pending_effects.go` and its option/notification tests |

Full managed conformance is the exact `Run conformance suite` command in
[`.github/workflows/ci.yml`](../.github/workflows/ci.yml). It uses
`uv run --project moo-conformance-tests --frozen moo-conformance`, selects
`admission or conformance`, starts Barn through `--server-command`, owns disposable
database copies, uses the `barn-linux-testdb-outbound-on` profile and the
`profiles/toast/stock-wsl-testdb.json` oracle manifest, and enables
`--fail-on-unexpected-skip --strict-markers`. Run it through the terminal summary;
archive the JUnit and admission JSON (including its exact revision/context).

The canonical bundled `Test.db` source is 2035 bytes, SHA-256
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`;
managed startup tests select their own bundled fixtures. The post-#312 integration
run [35410934149](https://github.com/MongooseMoo/barn/actions/runs/35410934149)
recorded `tests=12944 failures=0 errors=0 skipped=420` on the audited base.
The #313 PR records its own exact head, local results, Claude review and full CI
results; this base result is not a substitute for those checks.

No new uncertain MOO behavior is introduced here, so no new focused oracle claim
is made. If a future behavioral repair needs one, use the documented managed WSL
oracle `/root/src/toaststunt/build-release/moo`, include `capability_admission`,
use identical source fixtures for oracle/Barn, and record both revisions and raw
results. Source consolidation and this inventory alone do not prove correctness.
