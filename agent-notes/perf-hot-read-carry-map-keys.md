# perf/hot-read-carry-map-keys

## Goal
Hot-read renewal (db/store/store_hot_read.go renewAtCurrentClock) refused every
transaction holding keyed map state (`len(tx.mapKeys) > 0`), which was 712 of 715
refusals on the Mongoose `say` workload (#2585.handle). Carry keyed map state per
key instead, per the lead's approved design.

## State
- Built: clone of tx.mapKeys into the renewed txn (reads and written copied);
  refusal only when a state has hasPendingBase; keyed-written slots without a
  whole read re-staged the way SetPropertyMapKey stages (restagePropertyValue),
  everything else via SetPropertyValue.
- Tests in db/store/store_hot_read_test.go: unchanged keyed read renews with no
  whole read; stale keyed read refuses; carried keyed write survives with other
  keys committed before and after renewal; pending base refuses. Each was shown
  to fail under a mutation of the code it guards.
- Baseline reset (approved by the lead): a carried state with no written keys
  takes the renewed txn's view of the slot as baseline. Guarded by
  TestRenewalKeepsDependenciesOnEntriesReadAfterIt (fails without the reset).
- go test ./db/store, -race ./db/store, ./engine, vet and gofmt pass. Committed
  locally; not pushed, no PR.

## Decisions and why
- Version of a carried state stays the S0 version: rebaseMapKeyWritesLocked
  compares the live version against it to decide whether to rebase, and a
  degrade then records a conservative whole read.
- Written states keep the S0 baseline: their view stays the staged S0-based map,
  so entries other than their own still equal it. States with nothing written
  get the new clock's map as baseline, because that is what the renewed txn
  sees; otherwise an entry changed between S0 and S1 and read later records no
  dependency (lead approved).

## Surprises
- A carried state with no written keys keeps baseline = the S0 map while the
  renewed txn's view of the slot is the S1 map. ObserveMapKey
  (store_txn_mapkeys.go:139-143) treats an entry differing from baseline as the
  txn's own and records no dependency, so an entry changed between S0 and S1 and
  read after renewal is never validated. Fixed as recorded under Decisions.
- A read-only txn whose keyed read went stale still commits (no validation for
  read-only commits), so the refusal test asserts the refusal count only.

## Next step
Measure renewals/refusals on the real Mongoose workload (16p `say`) to confirm
the refusals from keyed state are gone; then push and open a PR when the lead
says to.
