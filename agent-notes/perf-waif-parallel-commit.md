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
- Committed da560311. Gates on it: gofmt clean; vet, staticcheck (plain and
  `-tags elle`), golangci-lint 0 issues; `go test ./...` exit 0;
  `go test -race ./db/store ./engine` ok (engine 243 s); Elle driver (WSL, pinned
  jar, JDK 21): 3 seeds true/true, 6 faults false incl. fault-waif-reads, exit 0.
  The waif-reads rejection now happens with WAIF commits on the decentralized path.
- `make conformance` (WSL): `12726 passed, 443 skipped in 949.23s`, exit 0.
- Route probe on the branch (8 s windows): 1p 699 writing commits, 0 coarse
  (305 waif-write, 158 waif-read-only, 236 plain, all decentralized); 16p 10840,
  0 coarse. Master before: ~60% coarse, all WAIF-related.
- A/B (CPU 3-13% before each run; first attempt skipped for lack of
  BARN_MONGOOSE_BENCH=1, discarded): 1p master 24,24 / branch 24,24 /s.
  16p master 402,259 / branch 393,299 /s; with BARN_DEBUG_RETRY 16p 337 vs 315,
  1p 22 vs 20; with LOG_DIR 16p 335 vs 303. Run-to-run spread 259-402: not
  enough to call. 6 more alternating 16p pairs (order flipped each pair):
  master 311,335,258,300,311,386 / branch 242,355,328,376,338,290; branch higher
  in 4 of 6. All 10 plain 16p pairs: mean master 323.4/s, branch 323.9/s.
  Conclusion: no goodput change on this workload; the exclusive lock was not the
  bottleneck (abort ~24%, mostly #2585.handle and #68987.state; the physics sim).
- Uncaught errors (BARN_DEBUG_RETRY): same kinds on both, all game-db:
  #9655 apply_containment_failure E_VERBNF, #2585:pick_value "cannot resolve
  173266", #424:request_options "Expected a url.", home E_NACC. Bench worktrees (detached): barn-bench-master2 (c840f0eb),
  barn-bench-waif (da560311), barn-bench-waif-probe (da560311 + route probe,
  BARN_COARSE_PROBE=1, uncommitted).
- Harness command: BARN_MONGOOSE_PLAYERS=$P BARN_MONGOOSE_WARMUP=2s
  BARN_MONGOOSE_MEASURE=8s BARN_MONGOOSE_GOGC=400 go test ./engine -run
  '^TestMongooseRealWorkload$' -count=1 -v -timeout=5m, plus BARN_MONGOOSE_DB and
  BARN_MONGOOSE_SOUND_DB absolute paths.

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
PR open against master; Q decides whether a change with no measured goodput gain
is worth merging. After merge: remove this worktree and branch.
