# refactor/commit-applier (Wave 3b: one commit applier)

## Goal

Implement the Wave 3b design (one commit applier for db/store) with Q's decisions:
- Q1 (c): keep the store-side propagation walkers on the exclusive (Lock) routes: the
  coarse commit and FlushStagedToLive. The parallel route keeps applying staged
  descendant writes. No double apply. Probe 3 stays green.
- Q2: keep liveMutated in the routing condition.
- Q3: no Elle row; store tests pin D3.

Design: session scratchpad design-3b-commit-applier.md. Worktree
C:/Users/Q/code/barn-commit-applier, branch refactor/commit-applier, rebased on
origin/master c305e0bd (#506 B14 merged underneath). Not pushed; no PR opened.
PR body draft: session scratchpad pr-3b-barn.md.

## State (done)

Commits on origin/master c305e0bd:
1. a56bfa4c tests: db/store/commit_applier_test.go
2. 4402ecd0 objectWrites + writesByObject; preflight, writesTouchAnon,
   CommitAndRenewCarryingReads use it
3. da77aefc ownImage / applyHeadWrites / applyTailWrites / publishStagedLocked; builders gone
4. 3a11a9a3 coarse commit + flush on publishStagedLocked, walkers through commitImages;
   applyStagedToLiveLocked gone
5. 5f9933d8 one Commit body; commitDecentralized gone
6. 8f48aed4 ApplyStagedObjectWrites on the applier
7. a6e97dce comments
plus this note and experiments/2026-10-26-commit-applier.md.

Red/green: TestApplyStagedObjectWritesDeleteThenRedefine red at commit 1 ("view a =
None, E_PROPNF; want 2"), green from commit 6. TestCoarseCommitHistoryKeepsCommittedImage
was red on 491d7462 (3 rows) and is green on c305e0bd before this branch (#506 changed
history append to replace a same-ts entry). TestCommitRoutesPublishSameState (9 scenarios
x 2 routes) and TestFlushDefineReachesChildAddedAfterStaging (Probe 3) green throughout.

Final checks on the rebased tree (raw output in the final report): go build ./...,
go vet ./..., gofmt -l . empty, go test ./db/... ./builtins ./engine ./server ok,
go test -race ./db/store ok, BARN_ELLE_FAULT=commit-exclusion go test -tags elle
./internal/isolationprobe ok, TestElleIsolationHistories (-tags elle, seed 412,
rounds 40) PASS healthy and under commit-exclusion and map-key-rebase (Go side only:
histories generated, Elle checker not run locally).

Lines vs origin/master: production Go +488 -1026 (net -538); tests +384 -5.

Allocations went UP (+2 per single-object commit; objects8 142->148, objects32
546->567). Cause profiled: writesByObject's heap map + one objectWrites per object.
Recorded in experiments/2026-10-26-commit-applier.md. Real-workload gate not run
(CPU 19-27%).

## How each route propagates a define / definition delete

- Shared-lock route (Commit, !liveMutated and no anonymous write): the definer's image
  gets the define or definition delete (applyHeadWrites). Descendants change only
  through the writes staging recorded: propagateDefinedProperty staged a clear
  inheritedSlot propertyWrite per inheriting numbered child, removeInheritedProperty
  staged a propertyDelete per inheriting child (numbered and anonymous; an anonymous one
  sends the commit to the exclusive route). No store walk. Staging recorded a
  relationship read on every walked object, so a child added since fails validation
  (design Probe 4).
- Exclusive routes (Commit with liveMutated or an anonymous write; FlushStagedToLive):
  publishStagedLocked(walkLive=true). Order: every written object is owned once and gets
  its head writes (scalar, relationship, definition deletes, defines); then for each
  staged definition delete removeInheritedPropertyLocked, and for each staged define
  propagatePropertyToDescendantsLocked, both walking through commitImages: they read the
  commit's own images (else live) and an object they change that the commit does not
  write yet is owned and added to the writes map; then every object gets its tail writes
  (property writes, property deletes, verb code, verb deletes, recycle) and is published
  once. A staged descendant clear slot is written by the walker and again by its staged
  write, into the same image, with the same slot; nothing is cloned or published twice.
  The walker runs before the property writes, as the old coarse path's define loop ran
  before its property-write loop, so a staged override of a child slot still lands last.
- Direct twins (Store.defineProperty / deleteDefinedProperty): unchanged behaviour; the
  walkers get liveImages (republishForMutation per changed child). definePropertyLocked
  and deleteDefinedPropertyLocked lost their ts parameter (only these callers remain).

## Decisions and why

- objectWrites.props holds property-write keys; applyTailWrites reads
  tx.propertyWrites[key] at apply time. The design had values; gathered before the lock,
  they missed rebaseMapKeyWritesLocked (store_txn_mapkeys.go:293-331), which rewrites
  values after preflight: TestMapKeyWritesToDifferentEntriesBothCommit failed ("a = 1,
  want 2") until this change.
- propertyDefinesByObject appends defines missing from the view's propOrder. Needed for
  ApplyStagedObjectWrites, which runs after AdoptLiveObject replaced the view with live
  (TestTransactionRenumber... in store_txn_test.go: fresh define on a renumbered parent).
  At commit every staged define is in the view's order, so commit order is unchanged.
- writesTouchAnon checks every written id incl. creates and recycles (old check skipped
  those two; a staged recycle of an anon id went to a path that cloned s.load(id)=nil).
- Pause points ("commit/validated", "commit/timestamped") stay on the shared route only,
  as before: engine/elle_workload_test.go holds a commit at "commit/timestamped" while a
  reader begins a snapshot (store.mu.RLock); an exclusive commit held there would block it.
- ApplyStagedObjectWrites: verb-code nil guard kept (old C had it); stamps use tx.readTS
  and the five object-level versions are restored. Staged slots/verbs record no read
  version (markPropertyReadKey early return for staged define/write keys;
  markVerbRead for staged verbWrites).
- Allocation increase not fixed: a slab for objectWrites would be machinery the design
  did not include; left for the lead/Q with the profile.

## Behaviour changes (all on the exclusive routes unless noted)

- One image per object per commit (D3) — by construction; no test counts publishes.
- New property slot keeps the staged clear flag (D1) — not reachable before (walker made
  the slot first); no dedicated test.
- WAIF-roots epoch bumped only when needed and after publish (D6); verb-shape note after
  publish; a create-then-recycle no longer notes verb shape. No dedicated tests.
- Renumber replay: delete-then-redefine kept, definition delete updates propOrder (D5) —
  TestApplyStagedObjectWritesDeleteThenRedefine.

## Surprises

- B14 (#506) merged mid-task; rebased twice. Commit 1's message was rewritten (branch
  rebuilt by cherry-pick) because 1a no longer fails on the new base.
- Worktrees share refs: origin/master moved under me when another agent fetched.

## Next step

Lead: review, push, open PR from scratchpad pr-3b-barn.md. Decide on the allocation
increase. CI must run conformance, the Elle matrix (Java verdicts), and lint.
