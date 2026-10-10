# Checkpoint publication hashing (issue #394)

Wall-clock latency of a replacing `format.WriteCheckpoint`, which is the work
`server.checkpointWith` runs inside `Runtime.WithCheckpointBarrier`, at three
commits.

## Fixture

`mongoose.db.new` (106,067,660 bytes) is not present in this Linux container,
so these numbers do **not** satisfy the issue's "measure on `mongoose.db.new`"
criterion. They use a stand-in of the same size:

- Source: `toastcore.db`, 2,083,333 bytes, SHA-256
  `da1c3ea32e57857855be94999efba930eaf8d1f5f3cbd04c16165ce7f54fd483`.
- Padded with 1,040 pending-finalization strings of 100,000 bytes each, giving
  a checkpoint of 106,085,886 bytes (within 0.02% of the Mongoose fixture).
- No WAIFs, so the sidecar is a header line. Hash and copy costs scale with
  database bytes, not object shape, so the size match is the relevant one.

The harness is `checkpoint_latency_measure_test.go.txt`. Copy it into
`db/format/` as `*_test.go` and run
`CHECKPOINT_FIXTURE=$PWD/toastcore.db go test ./db/format -run TestMeasureCheckpointLatency -count=1 -v`.
Each run writes one first checkpoint, then times seven replacing checkpoints.
Two interleaved rounds were run per commit (14 samples each).

Host: 4 vCPU Linux container, go1.27.1, `t.TempDir()` on the container's
local filesystem (hard links succeed, so the copy fallback is not timed here).

## Results (ms per replacing checkpoint)

| Commit | Whole-file hashes | Samples | Median | Min |
|---|---|---|---|---|
| `ab78a08` (before #392) | 1 | 516 518 361 318 316 321 366 / 345 317 327 335 362 347 553 | 346 | 316 |
| `800174d` (after #392) | 5 | 789 807 712 662 703 699 664 / 653 702 654 682 677 667 658 | 680 | 653 |
| this branch | 2 | 436 388 417 393 389 386 401 / 383 405 408 425 408 385 389 | 397 | 383 |

The #392 regression is about 334 ms for four extra passes, about 83 ms per pass
over 106 MB. This branch removes three of them. The remaining 51 ms over the
pre-#392 baseline is the one pass over the previous generation, plus the
journal write and directory syncs that recovery needs.

`barn.checkpoint_last_ms` also includes `#0:checkpoint_started` and
`#0:checkpoint_finished` hooks and the task snapshot. Those are unchanged by
this work, so the deltas above carry over to the metric.

## Where the pause goes on this branch

Temporary instrumentation (not committed) split each replacing checkpoint:

| Phase | ms (7 runs) |
|---|---|
| Serialize (`WriteDatabase`) | 55–72 |
| `fsync` of the staged file | 96–120 |
| Sidecar hash + publication (journal, links, renames, directory syncs, previous-generation hash) | 220–251 |

Only serialization needs the barrier. About 80% of the pause is file work on
an already-serialized snapshot.

## Decisions

**Publication can move outside `WithCheckpointBarrier`; do it as a separate
change.** Once `Writer.WriteDatabase` returns, the barrier's reasons are gone.
The store snapshot and task snapshots are fully serialized, and the WAIF
identities the sidecar needs are captured in `writer.waifIdentities`.
Everything after that (staged `fsync`, sidecar, journal, links, renames)
touches only files, and `lockCheckpointPath` already serializes writers and
loaders on the output path. Moving that tail out would cut the pause from
about 400 ms to about 65 ms on this fixture, a bigger win than the hashing fix.
It is left out of this change because it splits `WriteCheckpoint` into
serialize and publish phases and moves the admission resume ahead of
`#0:checkpoint_finished`. The follow-up must keep `checkpoint_finished(false)`
for a publication failure after admission has resumed, and must keep shutdown
waiting for an in-flight publication.

**Read-only inspection loads refuse; they do not recover.** `-verb-code`,
`-list-verbs`, `-obj-info`, `-eval`, `-eval-file`, `-dump-obj-raw`,
`-verb-lookup`, `-ancestry` and the `-dump` source now use
`format.LoadDatabaseReadOnly`. It takes the path lock and fails with
`format.ErrCheckpointRecoveryPending` when `<db>.publication` exists, without
renaming or removing anything. An inspection flag should not rewrite the files
next to a database. Recovery stays with the process that will own the
database: server startup (`LoadDatabase`) and the next checkpoint write.
