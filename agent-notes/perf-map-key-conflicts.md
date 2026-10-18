# perf/map-key-conflicts

## Goal
Q (2026-10-17): "can't we be smarter and know what keys are being accessed in large
shared maps?" then "we should absolutely do it, but maybe we don't have to follow it
through the prototype code? we can rewrite it to be like a oneline try expression that
actually indexes the map and defer following prototypes".

Make two transactions that touch different keys of one map-valued property stop
conflicting. Background: `agent-notes/mvcc-simplify.md` (master checkout), map-key probe.

## Evidence that motivated it (map-key probe, 8 s windows, mongoose.db.new 197 MB)
- 16p: 1757 lost commits; `#68987.state` ("Away Lines", map keyed by player) stale in
  645 (612 read it without writing; 33 wrote the same key #83); sole stale key in 281.
- Away Lines verbs touch only `state[who]`: `this.state:get(who, 0)` and
  `this.state[who] = s`. `#2951:get` (Map Prototype) is `try return this[key]; except
  (E_RANGE) continue;` -> `` `this.state[who] ! E_RANGE => 0' `` is equivalent for a
  scalar key. Toast: missing map key -> E_RANGE (execute.cc:1729, maplookup
  case_matters=0).
- Not addressed: `#2585.handle` (id counter whose value is used; 52% sole at 16p),
  `#9655.pending` read-only losers (whole-map prefix scans in `invalidate`).

## Design
Only forms where the map never leaves the expression; everything else stays a whole
property read, exactly as today.
1. Keyed read `obj.prop[k]` (static name, no `$` in k): base op records no whole read
   when the slot is a map held directly (not clear) on obj; the index op records a key
   observation (key, value or absent). Errors and tick charges identical to
   GET_PROP + INDEX; evaluation order unchanged (obj, prop fetch, k, index).
2. Keyed write `obj.prop[k] = v` (single level, static name, no `$`): base fetch
   variant records a pending keyed base instead of a whole read when the slot is a
   map; writeback variant carries k and stages the new map plus "written key k"
   without marking a whole read. Non-map base -> generic path.
3. Validation of a key observation: live slot present, not clear, owner and perms as
   observed, value a map, entry at k Identical (or still absent).
4. Commit/flush: after validation, for each property with keyed writes and no whole
   read, rebuild the staged value as live map + my written keys (rebase). One helper,
   called on the decentralized, coarse and flush paths.
5. Any other read of that property in the same txn records a whole read at the
   snapshot version. Hot-read renewal refuses txns with key observations or keyed
   writes (first cut).
6. Elle: map row keyed per worker + faults `map-key-reads` (skip key validation) and
   `map-key-rebase` (publish the stale whole map).
7. Measure with the harness rewriting the Away Lines verbs at boot; live game edits
   are Q's call.

## State
Branch off master c840f0eb. Store layer written (uncommitted), builds, existing
`go test ./db/store ./types` pass:
- `db/store/store_txn_mapkeys.go`: mapKeyState; keyedSlot; MapPropertyForKey(obj,
  name, forWrite); ObserveMapKey; SetPropertyMapKey (pendingBase must be the same
  map payload as the view, else plain SetPropertyValue; per-key elision);
  degradeMapKeys; validateMapKeysLocked (fault `map-key-reads`);
  rebaseMapKeyWritesLocked (fault `map-key-rebase`).
- Hooks: StoreTxn.mapKeys; ConflictMapKey ("map-key"); validateReadsLocked calls
  validateMapKeysLocked; rebase after preflight on decentralized and coarse paths;
  degrade in MarkLiveMutated, FlushStagedToLive, CommitAndRenewCarryingReads;
  renewAtCurrentClock refuses when mapKeys non-empty; markPropertyReadKey records a
  whole read at st.version when the slot has keyed writes staged.
- types: MapKeysEqual (mapKeyMatches), SameMapValue (same payload pointer).
- TestRenewalAccountsForEveryTxnField: mapKeys = declines.
- `store_txn_mapkeys_test.go`: 9 tests (different entries both commit; same entry
  conflicts; absent-entry dependency; whole read after keyed write; perm change;
  per-key elision; non-map slot = whole read; stale writeback = whole write; live
  mutation degrades). Shown red: rebase disabled -> "a = 1, want 2"; key
  validation disabled -> 4 tests fail. `go test ./db/store ./types` ok, gofmt clean.
Next: compiler/VM opcodes.

## Surprises
- `FlushStagedToLive` applies staged writes without validating (unless WAIFs) and
  then drops the read set. Keyed writes get rebased there too; existing behaviour
  otherwise untouched.
- `SetPropertyValue` marks a read of the slot it writes (elision depends on value).

## Next step
Store layer: key observations, keyed writes, validation, rebase, renewal refusal,
tests. Then compiler/VM opcodes, then Elle, then measurement.
