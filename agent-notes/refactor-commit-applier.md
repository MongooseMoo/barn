# refactor/commit-applier (Wave 3b: one commit applier)

## Goal

Implement the Wave 3b design (one commit applier for db/store) with Q's decisions:
- Q1 (c): keep the store-side propagation walkers (definePropertyLocked /
  deleteDefinedPropertyLocked descendant walks) on the exclusive (Lock) routes: the
  coarse commit and FlushStagedToLive. The parallel route keeps applying staged
  descendant writes. No double apply. Probe 3 stays green.
- Q2: keep liveMutated in the routing condition.
- Q3: no Elle row; store tests pin D3.

Design: scratchpad design-3b-commit-applier.md (session scratchpad). Worktree
C:/Users/Q/code/barn-commit-applier off origin/master 491d7462.

## State

- Step 1 committed (52fc0249): db/store/commit_applier_test.go. Red on 491d7462:
  TestCoarseCommitHistoryKeepsCommittedImage (3 rows: create+name reader name="";
  scalar+define E_PROPNF; delete-then-redefine E_PROPNF) and
  TestApplyStagedObjectWritesDeleteThenRedefine (view a E_PROPNF). Green:
  TestCommitRoutesPublishSameState (9 scenarios x 2 routes),
  TestFlushDefineReachesChildAddedAfterStaging (Probe 3).
- Step 2: objectWrites + writesByObject (store_cow.go); preflight, writesTouchAnon,
  CommitAndRenewCarryingReads use it; writeFootprintObjects/HasAnon deleted.
- Step 2 committed ea643e42. Step 3 committed 9d8704ca: ownImage, applyHeadWrites,
  applyTailWrites, publishStagedLocked in store_cow.go; commitDecentralized on them;
  buildImageWith*, applyPropertyValueOwned, buildImageRecycled, verbWrite2 deleted.
- Step 4 1663c943 (coarse + flush on publishStagedLocked with walkers; 1a green), step 5
  cbe9e278 (one Commit body), step 6 a595e8ae (ApplyStagedObjectWrites on the applier;
  1c green). go test ./db/store, ./builtins, ./engine green after step 6.
- Baseline allocs (491d7462 + tests): CommitScratch objects1 34, objects8 142, objects32
  546; TxnPropertyWriteCommit 33 (4703-4715 B); TxnReadOnlyCommit 17.

## Decisions and why

- propertyDefinesByObject: defines missing from the view's propOrder are appended (map
  order). Needed because ApplyStagedObjectWrites runs after AdoptLiveObject replaced the
  view with live, which lacks a fresh define's name
  (store_txn_test.go TestTransactionRenumber... anon child reads xyz after renumber). At
  commit every staged define is in the view's propOrder (DefineProperty staging inserts
  it), so commit order is unchanged.
- writesTouchAnon checks every written id, including creates and recycles (the old
  writeFootprintHasAnon skipped those two). A staged recycle of an anon id previously went
  to commitDecentralized, which would clone s.load(id)=nil.
- Q1 (c) plan: per object, apply "head" writes (scalar, relationship, definition deletes,
  defines) to an owned image; on the exclusive route only, run the store walkers over
  live children, reading/writing through the commit's image set (no republish); then
  "tail" writes (property writes, property deletes, verb code, verb deletes, recycle);
  then publish each image once. Walker-before-property-writes is the coarse path's
  current order (store_txn_commit.go: define loop before propertyWrites loop), so a
  staged override of a child slot after a define still wins.
- Pause points ("commit/validated", "commit/timestamped") stay on the parallel route only,
  as today: engine/elle_workload_test.go holds the first commit at "commit/timestamped"
  while a reader begins a snapshot (needs RLock); an exclusive commit held there under
  Lock would block that reader.

## Surprises

- The design's objectWrites carried propertyWrite values. Gathered before the lock (the
  anon routing check needs the keys), they missed rebaseMapKeyWritesLocked
  (store_txn_mapkeys.go:293-331), which rewrites tx.propertyWrites values after
  preflight: TestMapKeyWritesToDifferentEntriesBothCommit failed ("a = 1, want 2").
  objectWrites.props now holds the keys; applyTailWrites reads tx.propertyWrites[key].

- ApplyStagedObjectWrites needs a nil guard on verb code (old C had one): the adopted live
  image may lack the verb. Commits never hit it (preflight E_VERBNF).

## Next step

Step 7: comments (store_cow.go header, stale references to commitDecentralized,
writeFootprintHasAnon, applyStagedToLiveLocked, "coarse path" apply).
