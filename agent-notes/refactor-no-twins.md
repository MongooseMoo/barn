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

## Next step
Lead: review, decide on the direct-read cost, push and open PRs (drafts in the session
scratchpad: pr-3c-barn.md; pr-3c-mct.md says there is no mct PR).
