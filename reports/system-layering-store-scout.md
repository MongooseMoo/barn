# Store ownership and snapshot lifetime survey

## State

P6 scout completed a read-only survey. Findings are persisted by the coordinator;
no implementation or test execution is claimed here.

## Repair: standalone verb snapshot lifetime

callVerbWithArgstr always begins a snapshot in engine/call_verb.go:338, but only
the sweep path defers Release, and that defer captures the original receiver.
Ordinary login producers call it from server/input_login.go:66 and
server/input_processor.go:530. The runtime GC hook can renew and replace the
context transaction. Commit itself does not release snapshot registration.
Use one unconditional late-bound deferred closure that releases ctx.StoreTxn
at return; remove the sweep-only captured defer. runTask already uses this
late-bound pattern. Finalizers remain a backstop, never the prompt lifetime owner.

Regressions should disable Go GC and assert the existing ActiveReadTransactions
counter returns to zero immediately after read/write/error hooks and actual
GC-renewed/sweep hooks. Preserve suspended sweep continuation and deferred roots.
The pinned Elle workflow and its raw witnesses/path manifests are required.

## Keep existing isolation owners

Snapshot registration occurs before BeginSnapshot returns and explicit Release
atomically deregisters once. Runtime replacement sites release their old view.
Keyed-map dependencies retain captured value/presence, permission/owner checks,
and entry-only rebasing with quota checks. Hot renewal clones and validates
granular keyed dependencies; irreversible-boundary renewal deliberately degrades
them to whole-object dependencies, so the routes are not duplicate semantics.
Ordinary publication holds the shared gate and commit/store locks; an exclusive
slice grant exemption requires the real grant. Validation and preflight precede
all image publication. Object/task-row publication shares its leaf task lock;
observers run after lock release. The clock advances after images, and history
pruning follows publication with registration/floor scans synchronized.

## Separate possible beauty cleanup

objectReadSet's seven maps are cloned manually in store_txn_commit.go:61-88 and
with maps.Clone in store_hot_read.go:290-299. A clone method on that existing
owner could consolidate this while preserving route-specific dependency carry.
Keep this separate from the lifetime defect; add no facade or broader conflicts.
