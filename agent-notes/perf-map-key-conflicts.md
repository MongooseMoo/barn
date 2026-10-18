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
Committed 325b65d2 (store layer). In progress (uncommitted):
- bytecode: OP_GET_PROP_KEYED, OP_INDEX_KEYED, OP_GET_PROP_KEYBASE,
  OP_SET_PROP_KEYED appended after OP_FOR_LIST_LOAD_VALUE ([index:short], tickOne
  each, names, operand count 2, verifier constant check).
- vm/op_property.go: executeGetPropAccess(name, propWhole|propKeyRead|propKeyBase)
  replaces the body of executeGetPropNamed; keyed modes call MapPropertyForKey and
  skip hot renewal. executeGetPropKeyed leaves [obj, value]; executeIndexKeyed
  reports the entry then runs executeIndex; executeSetPropKeyed uses
  KeyedWritebackView + write-perm check + SetPropertyMapKey, else OP_SET_PROP.
- Size limit: max_list_value_bytes/max_map_value_bytes DEFAULT to 64537861, so a
  limit always applies. The writeback passes Session.ListValueLimitForTask; the
  store keeps the smallest; rebase returns a lost validation (ConflictMapKey) if
  the rebased map reaches it, so the retry raises E_QUOTA as serial execution would.
- compiler: compileIndex -> compileKeyedPropertyIndex for `obj.prop[k]` (static
  name, no ^/$ in k); compileIndexAssign keyed when single index, static
  property, no ^/$: base via OP_GET_PROP_KEYBASE, writeback GET_VAR base, GET_VAR
  key, GET_VAR obj, OP_SET_PROP_KEYED (GET_VAR is tick-free, so tick totals match).
- Bug found and fixed: a slot this txn DEFINED (add_property in the same task) was
  tracked keyed and validated against live where it is missing -> 64 retries then
  E_INVARG. keyedSlot now skips propertyDefines and createdObjects.
- `moo -e` behaviour check (map hits, case-insensitive key, missing -> E_RANGE,
  list key -> E_TYPE, list index/E_RANGE, map set/replace, list set, `$`,
  E_PROPNF): identical output to the master-equivalent build.
- vm/map_key_test.go: 3 tests via compiler+VM (different entries both commit;
  same entry and whole-value write conflict; keyed read ignores another entry).
  Shown red with keyed emission off: "second commit: E_INVARG [{property 0 m ...}]".
- `go vet ./...`, `go test ./...` exit 0.
- Process slip: built the baseline moo with `git stash`/`pop` (shell rewrite of
  tracked files); tree verified intact; memory note updated: use a detached
  worktree for baselines.
Next: commit; Elle map row + faults; conformance; harness rewrite of Away Lines
and measurement.

## Surprises
- `FlushStagedToLive` applies staged writes without validating (unless WAIFs) and
  then drops the read set. Keyed writes get rebased there too; existing behaviour
  otherwise untouched.
- `SetPropertyValue` marks a read of the slot it writes (elision depends on value).

## Next step
Store layer: key observations, keyed writes, validation, rebase, renewal refusal,
tests. Then compiler/VM opcodes, then Elle, then measurement.
