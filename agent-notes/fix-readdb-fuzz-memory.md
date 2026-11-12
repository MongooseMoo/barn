# fix/readdb-fuzz-memory

## Goal

Find why the FuzzReadDatabase worker (`./db/format`) is the process the fuzz
guest's OOM killer picks (issue #717: anon-rss about 1.8 GB, killed at about 10m
after throughput fell to 0/s).

## State

Fixed and committed on this branch. Not pushed, no PR.

## Cause

The cause is not reader amplification. Every `Store` that registered a WAIF it
also holds was pinned forever:

- `db/store/store_lifecycle.go` `RegisterWaif` called
  `waif.AddWaifCleanup(func() { s.unregisterWaif(...) })`.
- `types/waif.go` `AddWaifCleanup` passes that closure as the `arg` of
  `runtime.AddCleanup(waifRep, ...)`.
- The closure reaches `s`, and `s` reaches the WAIF through object properties
  and pending finalizations. The Go docs (runtime/mcleanup.go) say that if ptr
  is reachable from cleanup or arg, it is never collected and the cleanup never
  runs.

`NewStoreFromDatabase` (`db/format/reader.go`) registers every loaded WAIF, and
the fuzz body builds three stores per accepted input, so each input with a
WAIF-holding object leaked whole stores (a 32 KB object-directory segment and
more each).

## Evidence

- Measured by a temporary probe that ran the fuzz body over about 60k numeric
  mutations of the seeds. Live heap after `runtime.GC()` grew from 2.3 MB to
  731 MB. A heap profile showed 519 MB in `store.(*objectDir).ensureSegment`
  and 86 MB in `store.NewStore`, all under `NewStoreFromDatabase`. With the fix
  applied, the same probe ended at 5.6 MB, all of it the probe's own result
  list.
- The largest single-input allocation was 4.4 MB. No count field amplifies.
  All reader `make` calls are capped at `maxTrustedCount`.
- In CI run 43109992532, other fuzz targets on the same guest stalled at the
  same wall time. They recovered when format.test was killed.

## Fix

The cleanup holds the store through `weak.Make(s)`. Regression test:
`db/format/reader_waif_store_release_test.go`.

## Ruled out

- Header-count allocations: capped.
- Sparse object ids: numbered objects sit at their record index.
- Global caches: the intern pool and the compiler cache are bounded LRUs.
- WAIF and parent DAG walks: memoized.
- Chain-of-propdefs normalization: quadratic, 45 KB of input allocates
  217 MB transiently. Not the leak.

## Next step

Push, open a PR, and watch CI. #716 (vm FuzzRestoreVMSnapshotDoesNotPanic,
same CI run 42790517087 as #717) is not shown to share this cause.
