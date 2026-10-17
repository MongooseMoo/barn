# perf/waif-parallel-commit

## Goal
Let commits that read or write a WAIF take the decentralized commit path instead of
the exclusive store lock. Q approved the sequence on 2026-10-12 ("yup yup and yup":
pause fix #474, then Elle WAIF row #475, then this). Background and measurements:
`agent-notes/mvcc-simplify.md` in the master checkout, "Follow-up 2026-10-12: why are
77% of writing commits coarse?". On the real Mongoose workload every coarse commit was
WAIF-related (1p: 122 of 201 writing commits; 94 WAIF writes, 28 WAIF read-only).

## State
Branch off master c840f0eb (after #474 and #475 merged).
- `db/store/store_txn_commit.go`: routing drops `len(tx.waifs) == 0`.
- `db/store/store_cow.go` `commitDecentralized`: publishes staged WAIF images at the
  reserved timestamp, after the object images and before the clock CAS.
- Tests (`store_txn_waif_test.go`): new `TestWaifCommitDoesNotWaitForStoreReaders`
  (red under the old rule: "WAIF commit waited for a reader holding store.mu", both
  subtests); the two `TestWaifCoarseCommitValueWrite*` tests became
  `TestWaifDependentValueWrite*` and run on both paths (coarse forced with
  MarkLiveMutated) so the coarse path keeps its coverage.
- `go test ./db/store ./engine ./builtins ./vm ./types` ok.

## Decisions and why
- Why it is safe: WAIF images are versioned per WAIF under the WAIF's own mutex;
  `validateReadsLocked` (incl. `validateWaifsLocked`) runs on both paths; the store's
  WAIF history index (map + heap) is mutated only in `publishWaifLocked` and
  `pruneWaifHistory[Locked]`; the former now runs under commitMu+RLock (one
  decentralized commit at a time) or store.mu.Lock (coarse, direct txns), the latter
  under store.mu.Lock, which excludes RLock holders.
- No `noteWaifRootsChanged` for a WAIF-only publication: waifRootsEpoch tracks the
  WAIFs object properties hold; a WAIF publish changes only WAIF-internal refs, and
  `PublishWaifImage` bumps `types.WaifGraphEpoch` itself when they change. (The
  scratch branch exp/waif-route set it; unnecessary.)

## Next step
Gates: lint set, `go test ./...`, race db/store+engine, Elle driver (incl.
waif-reads fault), conformance. Then coarse-route probe + harness A/B from a separate
detached worktree. Then PR. Remove scratch `barn-waif-route` once this is pushed.
