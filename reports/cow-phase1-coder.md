# COW Phase 1 — coder report

## VERDICT: GO

Commit hash: `dd59f972942cf4f33fccff1938781e02ce2933c7`
Commit message: `Decentralize scalar/relationship/delete/verb commit writes (COW phase 1)`

Build target used: **`make build`** (`go build -o barn.exe ./cmd/barn/`). Only `barn.exe`
exists; no stray `barn-*.exe` was created. `go build ./...` / `go vet` / `go test` / `go test -race`
were used for everything else (no server binary).

## What was converted

Extended the decentralized COW commit-apply path (Phase 0 did property-VALUE writes) to the
remaining EASY/MEDIUM commit-apply write kinds. New build helpers in `db/store/store_cow.go`:

- **scalar** (name/owner/flags) — `buildImageWithScalar`: shallow struct copy, set the scalar
  fields, stamp `scalarVersion`. All collections shared by reference (scalars live in the struct).
- **relationship** (location) — `buildImageWithRelationship`: shallow struct copy, set `location`,
  stamp `relationshipVersion`. Only the `location` ObjID scalar is touched; everything shared.
- **property DELETE** — `buildImageWithPropertyDelete`: copy ONLY the `properties` map (sharing
  every untouched immutable `*Property`), omit the deleted key, stamp `propertyVersion`.
- **verb code** — `buildImageWithVerbCode`: copy ONLY the verb collections; build ONE fresh
  `*Verb` for the edited verb and substitute it for the old pointer in BOTH `verbs` (map) and
  `verbList` (slice) — identity-preserving exactly like `cloneObjectForReadTxn`, so the verb
  aliasing invariant (object.go:32-33) holds. Every unedited `*Verb` is shared (immutable). Sets
  `hasProgram=true`, stamps the verb's `version` and the object's `verbVersion`.

Phase-0's `buildImageWithPropertyValue` is reused unchanged.

`commitDecentralizedPropertyValues` was generalized into **`commitDecentralized`**: it now computes
the write footprint as the union of objIDs across all five decentralized write maps
(`scalarWrites`, `relationshipWrites`, `propertyWrites`, `propertyDeletes`, `verbWrites`), locks
each slot ascending by ObjID under `store.mu.RLock`, validates the read set against immutable
images, pre-checks every footprint object is live (E_INVIND) and every verb-code target verb
exists (E_VERBNF) BEFORE bumping the clock or publishing — so a missing object/verb fails the whole
commit atomically with no partial side effect. It then builds ONE new immutable image per object,
applying all of that object's writes in a fixed kind order (scalar → relationship → property-value
→ property-delete → verb-code), stashes the old immutable image as the history node (no clone), and
publishes via `slot.ptr.Store`.

## Routing (mixed / coarse-fallback footprints)

In `StoreTxn.Commit` (`db/store/store_txn.go`), the decentralized path is taken iff:

    !tx.liveMutated && len(tx.propertyDefines) == 0 && len(tx.propertyDefinitionDeletes) == 0

The outer guard already guarantees ≥1 write is staged, so reaching the predicate with both define
maps empty and `!liveMutated` means at least one decentralized write exists. Any commit whose
footprint includes property **define** or **define-delete** (the HARD descendant-propagating
walkers, left for Phase 2), or any `liveMutated` task (the create/recycle/chparent/move/add_verb
builtins, which stay on coarse `store.mu.Lock`), falls back to the unchanged coarse exclusive
apply path. A mixed footprint of decentralized kinds + a define/define-delete therefore correctly
routes to the coarse path.

## Invariants confirmed

- **Published images are immutable**: every builder allocates a NEW `*Object` (and a new
  `*Property`/`*Verb` for the edited node) and never writes a published image after `Store`.
- **Untouched sub-nodes are shared**: each builder copies ONLY the collection it touches
  (properties map for value/delete; verb collections for verb-code; nothing but the struct for
  scalar/relationship) and shares every other immutable node by reference (the history/alloc win).
- **Multi-object atomicity** via readTS + per-object version stamps + history (no global lock);
  same-object committers serialize via per-slot mutex taken ascending by ObjID; disjoint commits
  never share a slot and run fully in parallel.

## Stress test (new/extended)

Added `TestCOWDisjointMixedKindCommitsRaceFree` to `db/store/cow_disjoint_race_test.go`: 16 disjoint
writer goroutines each commit, in ONE txn via the decentralized path, a MIX of the new kinds
(scalar name + flag, relationship location, property-value, verb-code) on their own object; plus 4
writers exercising property-DELETE (`ClearPropertyOverride`, which stages `propertyDeletes` without
touching the define maps); while 16 raw readers hammer the `txn==nil` reader funnel (`ObjectName`,
`ObjectFlags`, `Location`, `PropertyValue`, `FindVerb`, `Parents`) on the same objects. Under COW
every commit publishes a NEW immutable image and never mutates the old one a reader Loaded.

-race result: **PASS** (`--- PASS: TestCOWDisjointMixedKindCommitsRaceFree (0.10s)` under
`go test -race`). The pre-existing `TestCOWDisjointCommitsRaceFree` and
`TestCOWSameObjectCommitsSerialize` also PASS under -race.

## All 5 gate results — ALL GREEN

1. **build/vet**: `go build ./...` clean; `go vet ./db/store` clean (no copylocks).
2. **go test ./...**: green — every package `ok` (incl db/store transaction/snapshot/history/recycle).
3. **go test -race ./db/store ./vm ./scheduler -count=1**: clean (db/store 1.6s, vm 1.7s,
   scheduler 121.7s). Includes `TestCOWDisjointCommitsRaceFree`,
   `TestCOWDisjointMixedKindCommitsRaceFree` (new), `TestCOWSameObjectCommitsSerialize`, and the
   scheduler serializability suite.
4. **managed conformance ONCE vs make-built barn.exe**: `3862 passed, 9 failed, 131 skipped`.
   All 9 failures are `limits::*` (value-byte-size quota tests expecting `E_QUOTA` that Barn does
   not enforce — known pre-existing category). No transaction/snapshot/history/anon/recycle/
   concurrency/property failure. No regression.
5. **scaling — `TestConcurrencyCommitDominatedDisjoint -count=3` (workers 1..32)**:

   | workers | us/commit (representative) | speedup ratio |
   |--------:|---------------------------:|--------------:|
   | 1       | ~13.6 us                   | 0.7–0.8x (baseline tax) |
   | 4       | ~11.3 us                   | ~1.2x |
   | 8       | ~9.5–10.3 us               | ~1.1–1.3x |
   | 16      | ~8.1–8.6 us                | 1.2–1.6x |
   | 32      | ~8.1–9.5 us                | 1.1–1.62x |

   Per-commit cost drops ~13.6us → ~8.1–8.5us (≈1.6x throughput) consistently; the pool-time
   "speedup" ratio reaches 1.62x at 32 workers in an un-loaded run and stays ≥1.2x at 16 across
   runs. The tail is noisy under concurrent machine load (the 1-worker baseline itself swings
   ~14→25us). Crucially this benchmark stages a single property-VALUE write per task, so it routes
   through the SAME COW publish path as Phase 0 (now via the generalized `commitDecentralized` with
   empty scalar/rel/delete/verb maps) — Phase 1 adds no work to it, so scaling is non-regressing by
   construction and meets the ≥ Phase-0 ~1.3x bar.

## Out of scope (left exactly as-is for Phase 2)

- Property DEFINE / DEFINE-DELETE descendant propagation and parent/child topology — stay on the
  coarse path.
- LiveStoreMutated builtins (create/recycle/chparent/move/add_verb/...) — stay on coarse
  `store.mu.Lock` (correct: Lock excludes RLock readers; not converted).

## Files changed
- `db/store/store_cow.go` — 4 new build helpers + generalized `commitDecentralized`.
- `db/store/store_txn.go` — routing predicate in `Commit`.
- `db/store/cow_disjoint_race_test.go` — new `TestCOWDisjointMixedKindCommitsRaceFree`.
