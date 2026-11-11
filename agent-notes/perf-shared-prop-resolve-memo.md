# Store-global property resolution memo

## Goal
On the 128-player Mongoose profile findPropertyFrom was 14.3% of CPU, mostly
walkProperty on per-transaction memo misses (first lookup of a key in each
transaction). Add a cross-transaction property memo, the property twin of the
store-level verb dispatch memo (Q approved, "make it so").

## State
Built on branch perf/shared-prop-resolve-memo; not pushed, no PR. Tests in
db/store/prop_resolve_memo_test.go. go test ./..., go test -race ./db/store
./engine, go vet, gofmt, staticcheck and golangci-lint (db/store, engine) pass.
A/B goodput comparison not done here (the requester does it).

## Design
- `sharedResolveMemo` (store_core.go) is the shared machinery: map, size cap
  (1<<17, swapped whole), shape clock. `Store.dispatch` (verbs) and
  `Store.props` (properties) are both one.
- Key (start object, canonical name); entry: readTS, the walk path with
  valid/found per step, index of the first found slot (metadata) and of the
  holder (value); holder < 0 is E_PROPNF.
- Usable when entry.readTS >= props.shapeChangeTS and tx.readTS >= it.
- Reader (no owned objects): reads and marks the first-found slot and the
  holder slot from its own view, sets usedPropMemo and lists the key in
  propMemoHits; validation (store_txn_validation.go), readPublishedAfter,
  hot-read renewal and PrepareLiveMutation/MarkLiveMutated honour it exactly
  as usedVerbMemo. The hit is filed in the per-txn memo as a record
  [start (unmarked), first-found, holder].
- Writer (owned objects): replays the walk's exact marks. Private copies on
  the path are accepted after checking valid/found/clear per step (their
  parents cannot differ: topology changes are live mutations).
- Populated after every walk whose path is unowned and whose start object is
  valid and numbered, unless the snapshot is already older than the clock.
- Clock moves: publishStagedLocked (defines, definition deletes, renames,
  slot deletes, a slot added or its clear flag flipped, staged recycle,
  numbered create) stores the commit's own ts before the clock CAS; coarse
  Add/Recycle/Recreate/Renumber/ChangeParents stamp both memos with one bump
  (noteResolutionShapeChanged).

## Decisions and why
- Both the first-found and the holder slot are re-read and marked: walkProperty
  returns the first found slot's owner/perms/version with the holder's value,
  and set_property_info on a clear slot does not flip clear, so marking only
  the holder would miss a metadata change.
- Property stamp stored before the clock CAS (not after, as
  noteVerbShapeChanged does): after-the-CAS leaves a window where a new
  snapshot sees new images with the old stamp.
- Global clock kept, not per-object versions: measured rejection for a moved
  clock was 1.7% of global lookups (plan threshold 50%).
- No isolationprobe fault for the new validation check: one needs an Elle
  negative-control workload, the docs/elle-isolation.md table and the
  scripts/check_elle.py list.

## Measurements (64 players, 30 s window, temporary counters, not committed)
- Before building: 1873 commits/s; property-shape-changing commits 1.2/s
  (clear flip or new slot 1.1/s, 3.1 slots/s; definitions 0.1/s; staged
  numbered create 0.1/s; recycles and coarse topology 0).
- After building: findPropertyFrom 3.44M/s, per-txn hits 57%; global lookups
  1.47M/s with 97.6% hits (reader 42%, writer 55% of lookups); rejected
  because the snapshot predated the clock 0.49%, entry predated it 1.22%,
  absent 0.71%.
- Conflict census: `property-memo-shape` lost=565 in the window, 19.6% of all
  lost validations (499 sole): reading-phase memo users later condemned by an
  unrelated clear flip anywhere in the store (about 2 per second).

## Surprises
- The existing verb memo has the window above: a snapshot that begins between
  a verb-shape commit's clock CAS and its noteVerbShapeChanged sees the new
  images with the old stamp, so a read-only task can dispatch through a
  pre-change entry. Writers are safe (commitMu is held through the stamp).
  Not changed here.
- The global clock costs validation: see the census figure above.

## Next step
Requester's A/B goodput comparison. If the property-memo-shape losses outweigh
the CPU saved, the plan's fallback (per-object property-shape versions on the
path) or validating a reader's dependency per path object at commit are the
candidates.
