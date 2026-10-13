# fix/commit-publish-atomic

## Goal
Close the torn snapshot on master and take the parallel-commit locking out of
`db/store` at the same stroke. Q approved on 2026-10-12, on condition that the Elle
fault matrix (PR #472, branch `test/elle-fault-matrix`) lands first.

The defect: `commitDecentralized` bumped the clock, then published its images. A
transaction that began in between read at the new timestamp, so objects it touched
before the publish resolved to old images and objects it touched after to new ones.
Read-only transactions never validate, so nothing caught it.

## State
#472 merged on 2026-10-12 as 1f1c0a17. This branch is rebased onto that master (one
commit) and its PR targets master. Worktree: `C:/Users/Q/code/barn-commit-atomic`.
After the rebase: `go test ./db/store ./engine` ok; full `check_elle.py` again
3 seeds true/true, 5 faults false, exit 0.

Done:
- `db/store/commit_atomicity_test.go`: `TestSnapshotNeverSeesPartOfACommit` (hook-free
  stress, 32 objects per commit, 4 readers) and
  `TestCommitsThatReadEachOthersWritesCannotBothCommit` (write-skew property).
- `Store.commitMu`; `commitDecentralized` takes it instead of locking the read∪write
  slots, reserves `ts = clock+1`, publishes, then `clock.Store(ts)`.
- Deleted `objectSlot.mu`, the lock-list construction and `unlockSlots`.
- `TestCOWCommitHoldsReadSetSlotsThroughPublish` became
  `TestCOWCommitWaitsForCommitInFlight` (holds commitMu instead of a slot mutex).
- Elle: `Pause("commit/timestamped")`; `elle_torn(step)` builtin; a writer/reader pair
  dispatched with `DispatchReady` so the reader's post-`suspend(0)` slice begins its
  snapshot while the writer is held.
- `commit-exclusion` fault now means "skip commitMu".

## Evidence (2026-10-12)
Before the fix, same branch:
- `TestSnapshotNeverSeesPartOfACommit` FAIL 3 of 3: "a snapshot saw #0.n=2 beside
  another object's n=1, after 2 commits and 33 snapshots".
- Elle healthy seed 412: list-append `false`, G-single-item: "T1 did not observe T2's
  append of 195 to "property"" / "T1 observed T2's append of 196 to key
  "inherited-property"".
After the fix:
- `go test ./db/store`: ok. Atomicity tests x20: ok.
- Elle healthy seed 412: list-append true, rw-register true.
- Elle `commit-exclusion`: false, G2-item and G-single-item witnesses.

Full gates on the finished tree (2026-10-12, late):
- gofmt clean; `go vet`, `staticcheck` with and without `-tags elle`; `golangci-lint`
  0 issues; `go test ./...` exit 0.
- `go test -race ./db/store ./engine`: ok (8.1 s, 222.3 s).
- `scripts/check_elle.py` full driver in WSL: 3 seeds true/true, 5 faults false, exit 0,
  1m35s.
- Managed conformance (`make conformance` in WSL, with --fail-on-unexpected-skip):
  `12726 passed, 443 skipped in 880.99s`, make exit 0.
- Mutation check of the write-skew property test without touching source:
  `BARN_ELLE_FAULT=commit-exclusion go test -tags elle ./db/store -run
  TestCommitsThatReadEachOthersWritesCannotBothCommit -count=3` FAILS 3 of 3
  ("round 1: 2 of the two commits succeeded"); passes without the fault.
- The clock store is a CompareAndSwap(ts-1, ts) that panics if the clock moved during
  the publication; none of the runs above tripped it.
- PR #472 CI: Build & unit tests pass, MOO conformance pass, Elle pass; mergeable,
  not merged (Q merges).

## Decisions and why
- Why the clock can be read-then-stored without CAS: every other bump
  (`bumpClockLocked`, `noteVerbShapeChanged` from coarse paths, direct WAIF writes)
  runs under `store.mu.Lock`; the committer holds `store.mu.RLock` and `commitMu`.
  Checked all 30 call sites on 2026-10-12.
- Why one mutex costs nothing: measured on the real Mongoose workload at 16 players,
  commits on this path locked a mean of 50 slots for 1.16 written, 99% of them locked
  #0, and 17 of 17 that overlapped another commit shared a slot with it. Run was
  degraded (no sound catalog) so only the shape counts. See
  `agent-notes/mvcc-simplify.md` in the master checkout.
- Reader test pair uses `DispatchReady`, not `elleDrain`: `ProcessReadyTasks` joins the
  whole batch, so a suspended reader would not resume until the held writer finished.
  Irreversible-effect renewal was not usable either: it takes the commit gate
  exclusively and the held writer has it shared.

## Surprises
- The torn snapshot needs no pause hook to reproduce: one or two 32-object commits
  with four readers is enough.

## Known and NOT addressed here
- `maxObjID` is raised (casMaxID) before the clock store, so a snapshot begun during a
  publish can see `max_object()` name an object it cannot yet resolve. Same looseness
  existed before; fixing it means versioning the watermark.
- `objectSlot` is now a one-field struct; the directory could hold
  `atomic.Pointer[Object]` directly. Left alone to keep this diff about the protocol.

## Next step
Watch the PR's CI. After merge, remove this worktree and branch.
