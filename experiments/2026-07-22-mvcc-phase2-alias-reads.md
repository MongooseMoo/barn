# Experiment: MVCC Redesign — Phase 2 Alias Immutable Reads (2026-07-22)

Plan: `plans/mvcc-concurrency-redesign-2026-07-21.md` Phase 2 (the biggest lever).
Branch: `mvcc-concurrency-redesign`.

## What changed

Read transactions now ALIAS the immutable published `*Object` image pointer
instead of deep-cloning the whole object (all properties + all verbs + code) on
every first touch (`objectLocked`, store_txn.go). A txn-local mutable copy is
created only on the first STAGED WRITE to an object (`mutableObject` /
`privatizeCached`, true copy-on-write).

To make that safe, every runtime mutation of a NUMBERED published image was
converted from mutate-in-place to **publish-a-fresh-image** (`republishForMutation`,
store_core.go): under the exclusive `store.mu.Lock` it clones the object, publishes
the clone into the slot (where no reader can alias it yet), and retains the old
image immutably in history. ~47 sites across store_core / store_relationships /
store_verbs / store_properties / store_lifecycle / the coarse commit in store_txn.
Anonymous objects are the exception — rare and possibly pointer-referenced, they are
kept clone-on-read and mutated in place.

## Safety verification

- `immutability_test.go`: one red→green test per mutation kind
  (move/create/chparent/define/setprop/delete/addverb/deleteverb/setverbcode) asserting
  the superseded image is byte-stable and the slot pointer changed. All green.
- `TestReadAliasSurvivesConcurrentMutation`: a reader that aliased an image observes
  its snapshot unchanged after another path mutates the object. Green.
- `go test ./db/store ./scheduler -race` GREEN (scheduler = 230s of concurrent
  txn+mutation stress under the race detector).
- Full `go test ./...` EXIT=0.

## Results (median of 5, 2s window, `-count=1`, GOGC=400)

### realistic mix (look40/say30/move20/stamp7/build3), rooms=16

| players | goodput/s | allocs/op | bytes/op | GCs |
|--------:|----------:|----------:|---------:|----:|
| 1  | 54,603 |  94.9 | 18,172 | 25 |
| 16 | 72,728 | 100.0 | 18,574 | 30 |
| 32 | 70,552 | 105.5 | 18,933 | 22 |

### rooms=64 (wider reads — more occupants deep-cloned pre-Phase-2)

Phase 2 realistic 64-room allocs/op ≈ 100 vs Phase 1 ≈ 300 (the wide-read cells
benefit most, as predicted — a look no longer clones every occupant).

## Comparison

| cell (realistic, 16p/16r) | goodput/s | allocs/op | bytes/op | GCs |
|---------------------------|----------:|----------:|---------:|----:|
| Phase 0 baseline (GOGC=100) | 41,920 | 210 | 36,747 | 140 |
| Phase 1 (GOGC=400)          | 54,401 | 210 | 36,757 |  31 |
| **Phase 2 (GOGC=400)**      | **72,728** | **100** | **18,574** | **30** |

- **Phase 2 vs Phase 1 (isolates aliasing, same GOGC):** 1.34x goodput; allocs/op
  210→100 (2.1x fewer); bytes/op 36,757→18,574 (1.98x less). At 32 players 1.44x
  (48,998→70,552), and 32p goodput stops regressing below the 16p peak.
- **Cumulative Phases 1+2 vs Phase 0 master baseline:** 1.73x goodput, 4.7x fewer GCs,
  ~half the memory per command.

The deep-clone-on-read was ~half of ALL per-command allocation (the remaining ~18KB/op
is harness task-construction overhead + move()'s coarse-path clones + value work). This
confirms the plan's thesis that read-time deep cloning is the dominant allocation tax.

## Still on the table (unchanged by Phase 2, as expected)

- Churn-stress abort stays ~22-27% at high player counts — that is the shared-ancestor
  false-conflict engine, Phase 4's target.
- move() is still on the coarse stop-the-world path — Phase 3's target; it caps realistic
  scaling past ~16 players even though the alloc relief lifted the absolute numbers.
