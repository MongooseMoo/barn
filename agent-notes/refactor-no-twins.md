# refactor/no-twins (Wave 3c: one implementation per store operation)

## Goal
Q approved design option (b) of the Wave 3c design (session scratchpad
design-3c-no-twins.md): the shared direct txn serves reads from the live store (no cache,
no read marks, no scratch) and writes through one `writeNow` helper (store.mu.Lock, stage on
a private txn, publishStagedLocked). Delete the Store twins and the `if tx.direct` forwards.
D2 (fix/store-b18, #512) and B17 (fix/store-b17, #511) were owned elsewhere and merged
first; this branch is rebased over them and over #499 (#518). 3d (Store field split) is a
later PR.

## State (done; nothing pushed, no PR opened)
- Branch refactor/no-twins, worktree C:/Users/Q/code/barn-no-twins, on origin/master
  c537d6c9. Commits: red tests; reads; writes; experiment record; #499 test adaptation.
- No moo-conformance-tests commit (see Decisions). Worktree C:/Users/Q/code/mct-no-twins,
  branch test/no-twins, at origin/main ec2788a with nothing on it.
- Production Go +254 -1510; tests +361 -17. `if tx.direct {` in db/store non-test files:
  66 -> 26.
- Gates run on d14ad3aa: go build, go vet, gofmt (clean); go test ./... (all ok except the
  three internal/app git-metadata tests, which fail on 4449b485 too); go test -race
  ./db/store ./engine ok; staticcheck and golangci-lint ./db/... clean; Elle Go generation
  (seed 412, 40 rounds) ok healthy and under all ten faults; managed conformance from WSL
  Debian on 51 builtins files (verbs, properties, move, create, recycle, recreate, renumber,
  chparent, objects, object_hierarchy, max_object): 6592 passed.
- Measurements: experiments/2026-10-26-no-twins.md.

## Decisions and why
- No conformance rows: none of D1, D3-D11 is reachable from MOO (design 1.3); D2 and B17
  were fixed and pinned by B18/B17. D1's Toast basis checked in WSL source:
  objects.cc:1258-1276, db_objects.cc:424-437.
- MaxObject and ReadTimestamp keep a direct branch: the direct txn has no snapshot max or
  readTS; the branch reads the store counter, not a twin.
- D3 (move of an anonymous `what`): the staging body remains, so the direct txn now moves
  it as a snapshot does. Not reachable from MOO.
- The writeNow writer is `locked`; `live()` (direct or locked) turns off the read marks and
  the resolution memos, so the writer needs no read maps and cannot publish a dispatch-memo
  entry stamped with readTS = MaxUint64 (which would never go stale).
- ResolveVerbOnObject resolves the index in the same object lookup as the match
  (findVerbOnObject returns object and index): on the direct txn two lookups could see two
  images and the pointer comparison would miss.
- D9: DeleteResolvedVerbAuthorized is one body (identity, then ObjectAllows on the txn view,
  then DeleteResolvedVerb); a snapshot also records the scalar read the check makes.
- propertyDefinesByObject groups each staged define once (see Surprises).
- Did not optimize the direct-read scratch (design says private scratch per walk); the cost
  is recorded for Q instead.

## Surprises
- TestCOWDisjointMixedKindCommitsRaceFree hung (not a deadlock): it clears a DEFINED slot
  with ClearPropertyOverride (production clear_property refuses, builtins/properties.go:
  423-425), leaving a stale propOrder entry; propertyDefinesByObject then appended one
  define per duplicate name, doubling propOrder per redefine. Latent on the snapshot path
  before; exposed when direct defines started staging.
- Direct reads cost more: FindProperty 5 -> 12 allocs, 40 -> 1632 B, ~560 -> ~2800 ns;
  FindCallableVerb 5 -> 12 allocs. Source: the per-walk private scratch and step record.
  Direct writes about double their allocations (private writer + double clone).
- origin/master moved twice during the work (b17/b18, then #499); #499's new test used the
  deleted Store.findProperty.

## Not verified
- Elle checker verdicts (Java) not run locally; only the Go history generation.
- Real-workload perf gate not run: machine not idle (28.7/19.6/18.7% at 23:26 MDT).
- Whether a loaded database can carry FlagInvalid without FlagRecycled (D4 reachability).
- Full conformance suite not run (CI runs it).

## Cost fixes (Q "make it so" for scratchpad design-3c-direct-cost.md revision 2)
Order R1, C1, W-A, W-A2, W-D', one commit each; after each, BenchmarkDirect* and the commit
benchmarks on branch, master c537d6c9 (worktree barn-no-twins-base) and pre-3b 491d7462
(worktree barn-pre3b), -count=3. Rebased onto origin/master 53f8227c first (no db/store
change since c537d6c9). Baselines (every run so far): pre-3b Direct 5/5/1/24/25/85, commits
34/142/546/33/17/0; master Direct the same, commits 36/148/567/35/17/0.
- R1 8f110081: Direct FindCallableVerb/FindProperty 12 -> 0; others unchanged.
- C1 ded6fe04: writeSet (value slice, index map past 24). Commits 35/140/533/34; Direct writes
  58/58/174. Gathering for a one-object commit 4 -> 2 (profile: make entries + props append).

- W-A d03866ad: cloneObjectSharingVerbs / ownVerbAt; Direct writes 26/26/74 (master 25/24/85),
  commits unchanged 35/140/533/34.
- W-A2: privatizeCached is a shallow copy; owned is map[ObjID]objectFacets; mutableFacets
  names the facets a site changes in place (properties, propOrder, contents, children,
  verbs); ApplyStagedObjectWrites owns all. Tests TestStagedWritesLeavePublishedImagesUnchanged
  and TestPrivateCopiesDoNotShareListCapacity; each shown to fail with its facet removed
  (properties in SetPropertyValue, contents in MoveObject, children in CreateObject).

- W-A2 0026da53: Direct writes 20/23/51, commits 33/124/469/32.
- W-D' 60e87eaf: store-owned writer reset whole each write; clearStagedWrites clear()s;
  writerKeepLimit 256. Direct writes 13/16/44. Added BenchmarkDirectDefinePropertyWide:
  25,282 vs master 14,085 (3c before fixes 31,296). OPEN DEFECT; cause in the experiments
  file (staging copy of each descendant's property table); only known route is W-B,
  which revision 2 rejected.
- Rebased onto 720df158 (#535 merged). #535's txn reseed replays staged writes on a private
  copy: 845a8015 sets its properties facet and fixes #535's oracle test (s.ancestors ->
  DirectTxn().Ancestors). The W-A2 and W-D' commits do not compile on their own after this
  rebase: #535's reseed reads `owned` as a bool. 845a8015 makes the branch build again.
  Interactive rebase is not available here, so the fix-up could not be squashed back.
- Gates. On 845a8015 and on ab065440 (base 6c413b45): build/vet/gofmt clean; go test
  ./db/... ./builtins ./engine ./server ok; -race ./db/store ./engine ok; Elle generation
  healthy and under all ten faults ok; narrowed managed conformance (52 builtins files) 6648
  passed. Final rebase onto e83980da (#541, db/format only): build/vet/gofmt, go test
  ./db/... ./builtins ./engine ./server ok, conformance 6648 passed; race and Elle not re-run
  on this base (db/store and engine unchanged by #541). The fix-up for #535 is 517e79a8.

## Wide-define fix and history rebuild (Q "make it so", lead brief 2026-10-26)
Spec: session scratchpad design-3c-wide-define.md (B+J+N), then the CommitScratch/objects32
B/op regression.
- History rebuilt by design section 8 (no editor), original kept as branch
  refactor/no-twins-orig (e2c531d5). Rebuilt head e75dc48f: `git diff --stat e2c531d5`
  empty; all 12 commits pass go build ./..., go vet ./..., and db/store test compile.
  e30a1202 and 517e79a8 are folded into e2648443 and 1ca23b29.
- origin/master is 11a17d8c (#544: engine elle test, docs only); db/store unchanged since
  c537d6c9, so the master baseline stands.

- f1602317 B+J+N (design section 6 plus the replaceImageLocked helper, the publishStagedLocked
  doc, the writeNow comment, and the writerKeepLimit comment, whose wide-define example B made
  untrue). Mutation check rerun here: walkLive=false with the early returns fails
  TestChownFlagSlotsOwnedByEachObjectOwner, TestDeleteDefinedPropertyRemovesAnonymousChildOverride,
  TestDefinePropertyGivesAnonymousDescendantsOwnSlot. Bench: wide define 12,110 allocs /
  2,359,065 B (master 14,085 / 2,378,333); every other row at or below master except
  CommitScratch/objects32 B/op 114,321.
- ce1db733: writesByObject reserves min(stagedEntries, max(24, len(tx.objects))).
  objects32 98,705 B / 436 allocs (master 102,288 / 567; pre-3b 99,313 / 546). Chose sizing
  over slimming the entry: one line, identical for commits of 24 or fewer entries, and
  len(tx.objects) keeps many writes on one object at 24. tx.objects is a hint, not a bound
  (a move's room delta is staged without a read), so an undercount only regrows.

## Next step
Rebase onto 11a17d8c, final three-tree bench, gates, PR draft scratchpad/pr-3c-barn.md.
