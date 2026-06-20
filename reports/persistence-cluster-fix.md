# Persistence-cluster fix (B3 crash + B5 parent-drop + B4 propval-loss) — CODER report

Worktree: `C:/Users/Q/code/barn-fix-persist`, branch `fix/persistence-fidelity`, off master `b9d3f61`.
NOT merged (verifier next). RULE ZERO followed: every correctness call tested against WSL Toast
(`/root/src/toaststunt/build-release/moo`) and the canonical DB (`~/src/toastcore/toastcore.db`).

> UPDATE (PATH-EMIT, real DB): the full proper anonymous-object fix is now implemented and verified
> against the REAL database `mongoose.db.new` (fetched fresh; barn-dir `Test.db` was a stale bad dump and
> is no longer treated as truth). All gates green including Toast LOAD **and** VALIDATE. See
> "PATH-EMIT — full anonymous-object serialization" at the bottom; it supersedes the earlier
> STOP-ON-AMBIGUITY fork.

## Outcome summary
- FIX 1 (B3 crash): DONE. GATE A canonical + mongoose GREEN (no SIGSEGV).
- FIX 2 (B5 parent drops): DONE by FIX 1. GATE B GREEN (0 drops on mongoose).
- FIX 3 (B4 propval loss): DONE. GATE C GREEN (db_roundtrip mongoose: 0 prop losses).
- PATH-EMIT (anonymous objects): DONE. Reachable anonymous objects are emitted with above-max
  serialization ids and never occupy a regular numeric id; Toast LOADS and VALIDATES Barn's v17 output of
  mongoose.db.new (738 anon objects) with zero VALIDATE errors.

## Commits
- `1561b29` Fix B3 crash: track anonymous objects out-of-band (FIX 1, also resolves FIX 2).
- `fb50408` Restore inherited propvals on dump; widen anon-reachability scope (FIX 3).
HEAD = `fb50408`.

---

## FIX 1 — B3 crash (per-stage changes)
RULE ZERO findings (Toast source `db_file.cc`, `db_io.cc`, `db_objects.cc`):
- Anonymous objects are NOT regular numbered objects. Toast creates them lazily from `_TYPE_ANON`
  references (`db_read_anonymous`: `num_objects = oid; dbpriv_new_anonymous_object()`), and the
  anon-objects section merely FILLS IN those pre-created slots (`ng_read_object(1)` →
  `dbpriv_find_object(oid)`). An anon record whose oid was never referenced makes Toast deref a
  never-created slot → SIGSEGV. Canonical's anon section is the single `0` terminator.

Changes:
- `db/format/reader.go`: added `Database.AnonymousObjs []*store.ObjectBuilder`.
- `db/format/reader_object.go:readAnonymousObjects`: anon records now collected into `AnonymousObjs`
  (out-of-band), NOT keyed into `Objects[id]`. They are no longer ingested as phantom regular objects,
  so they cannot collide with recycled regular ids nor be re-emitted into the anon section.
- `db/store/store_snapshot.go:Snapshot`: anonymous objects are placed in `AnonymousObjects` only when
  reference-reachable (added `persistentAnonymousReachabilityLocked`, reusing the existing
  `collectAnonymousObjectRefs`/`expandAnonymousReachabilityLocked`). For a world with no live anon
  refs the section becomes just `0`, matching canonical Toast.
- `db/format/writer_object.go:writeAnonymousObjects`: documented the invariant (logic already correct
  given a filtered set).

GATE A (WSL Toast loads Barn's v17 output):
- canonical: `Reading 127 objects … Done reading 127 objects`, VALIDATE Phase 1/2/3 pass,
  `Reading 1949 MOO verb programs`, LISTEN, clean dump, exit 0.
- Test.db (was SIGSEGV): `Reading 15578 objects … Done reading 15578 objects` (NO bogus second
  "Reading 3502 objects" header), VALIDATE Phase 1/2/3 pass, LISTEN, clean dump, exit 0. CRASH GONE.

## MEASURE after FIX 1 (`db_roundtrip -db Test.db`)
- parent drops 24 → **0**; prop losses 2844 → **2564**; object-existence mismatches **0**.
- objects 13451 → 9949 (the 3502 phantom anon objects removed from the world model — correct; Toast
  would never carry them as numbered objects, and it crashes on them).

## FIX 2 — B5 parent drops
- Verified FIX 1 eliminated all 24 drops (they were anon-id-collision artifacts: anon objects keyed at
  recycled regular ids whose parent pointed at a recycled-regular id). `startup_repair.go` unchanged.
- GATE B GREEN: `db_roundtrip Test.db` reports **0** `#N.parent … removed`.

## FIX 3 — B4 propval loss (per-stage changes)
RULE ZERO findings:
- Toast `ng_read_object` reads `nprops` then EXACTLY `nprops` propvals positionally (no read-time
  validation vs ancestry). #6's 6 propvals are legitimate and must round-trip by COUNT.
- #6 "Server Options" chain is #6 → #1, and #1 has 0 propdefs; the 6 names are defined on #0 (a
  sibling). So the names are NOT recoverable from #6's true ancestry — `_inherited_N` placeholders are
  the honest state. Faithful behavior = preserve the 6 values/owner/perms by count.

Root cause: the dump re-walked the parent chain for the property-name list and returned a SHORT list
when the chain was broken/unresolved; the writer emits one triple per name, so the count (and thus the
propvals) shrank. 2564 objects affected; #6 lost all 6.

Change:
- `db/store/store_snapshot.go`: replaced the parent-chain re-walk with `snapshotPropertyNames`, which
  derives the list from the object's stored `propOrder` (authoritative load order, maintained by
  `DefineProperty`/`DeleteDefinedProperty`) and appends any property in the map not represented there
  (e.g. runtime-inherited slots added by `propagatePropertyToDescendantsLocked`, which does not extend
  `propOrder`). This guarantees `len(names) == len(properties)` in every case; the writer can no longer
  shrink the propval count below the object's real property count.

GATE C GREEN: `db_roundtrip Test.db` reports **0** "props N vs 0"/partial losses; #6 round-trips its 6
propvals exactly — `123, 2147483647, 1, 2000, 2000, 3210`, each owner `#9`, perms `1`.

---

## STOP-ON-AMBIGUITY FORK (data correctness) — Test.db GATE A under FIX 3

Restoring propvals (FIX 3) re-exposed a latent corruption in barn-dir `Test.db` (itself a prior bad
Barn dump): **56 property values hold `_TYPE_ANON` references** (matching exactly the 56 objects Toast
flags). The referenced anon objects are either empty (`name=""`, 0 props) or absent from the file.
Toast allocates a slot for every `_TYPE_ANON` reference and expects the anon section to fill it; an
unfilled slot fails VALIDATE Phase 1 (`#N.parents is not an object or list of objects`) →
`DB_LOAD: Cannot load database!` (224 errors). FIX 1 masked this only because it ALSO dropped the
propvals that held those refs.

Verified the canonical (clean Toast DB) has **0** anon refs and **0** anon objects, so all of this is a
Test.db artifact, not legitimate world data.

This is exactly the structural anon-tracking fork the task flagged as STOP-worthy. Three defensible
designs, each with different data-correctness semantics — I did NOT guess:
- **PATH-EMIT** (most Toast-faithful): re-ingest reference-reachable anon objects into the world model
  (store) at their loaded ids so `Snapshot()` emits them in the anon section. Risk: anon ids collide
  with recycled-regular slots; Toast tolerates this (anon obj occupies the slot) but Barn's store keys
  by id and would need to model "anon object at an otherwise-recycled id". Structural change to the
  loader/store.
- **PATH-THREAD**: keep anon out-of-band but thread `db.AnonymousObjs` to the writer (db_roundtrip's
  writer path runs off `store.Snapshot()` and has no access to it) and emit/rewrite referenced anon
  objects there. Structural change to the writer wiring.
- **PATH-STRIP**: treat a property holding a dangling `_TYPE_ANON` ref as corruption and drop it.
  Silent property mutation; not clearly "what Toast does".

Recommendation: PATH-EMIT (matches Toast's model), but it is a loader/store structural change with
collision risk on the loaded world model — the precise thing the task said to stop and surface. The
ship-critical crash (B3) and the parent drops (B5) are fully fixed independent of this; the propval
restore (FIX 3) is correct in isolation. Awaiting Q's design decision before doing the anon-emit work.

---

## Final gates (quoted)
- `go build ./...` → exit 0.
- `go vet ./...` → exactly the 2 known: `cmd/moo_client/main.go:53` (IPv6 fmt) and `vm/stack.go:49`
  (ReadByte signature).
- `go test ./...` → only known fixture failures (missing files): `TestLoadMongooseSnapshot`
  (`mongoose7_snapshot.db` absent) and `conformance` `TestConformance/TestLoadAllTests/TestYAMLParsing`
  (`../cow_py/tests/conformance` absent). `db/store` and `types` pass.
- `go list -deps ./db/store | grep parser` → EMPTY (parser-free).
- Conformance (managed Barn, synchronous foreground): **3871 passed, 0 failed, 131 skipped** in 143.66s.
  Read/runtime path not regressed.
- GATE A canonical: GREEN. GATE A Test.db: GREEN for the crash (no SIGSEGV); RED for VALIDATE under
  FIX 3 due to the corrupt-anon-ref fork above.
- GATE B: GREEN (0 parent drops). GATE C: GREEN (0 propval loss; #6 6 propvals intact).
- B6 (NOT regressed): canonical source is **1950** verbs (Toast-native); Barn's canonical round-trip
  yields **1949** (1949 vs 1950 — the better end of the report's 1948–1949 range, not worse).

## Not touched
B2c, W1, and the perf branches. No conformance YAML edits. The `moo-conformance-tests` checkout
unchanged.

---

## PATH-EMIT — full anonymous-object serialization (supersedes the fork above)

Q's call: do the proper fix on this branch, and re-anchor on the REAL database. Q's key correction:
anonymous objects must NEVER occupy a regular numeric id — that collision WAS the bug, not a constraint.
In ToastStunt anon objects exist only as `_TYPE_ANON` values at runtime and are assigned **above-max**
serialization ids at dump time (`db_file.cc:write_db_file` `while (last_oid > max_oid)` +
`db_objects.cc:db_write_anonymous` which allocates `num_objects` above the current max). RULE ZERO
confirmed against Toast source and behavior.

### Real database
- `scp mongoose@mongoose.world:~/mongoose/mongoose.db.new ./mongoose.db.new` → **succeeded** (exit 0),
  102,076,427 bytes, Format Version 17. (gitignored; not committed.)
- It contains real anonymous data: **27957 regular objects, 738 anonymous objects**, 185 players,
  maxObj #31225, and **739 `_TYPE_ANON` references** in regular property values (733 resolve to a present
  anon object; 6 reference an anon object not present out-of-band).

### Implementation (file:line)
- `db/store/store_core.go`: `Store.anonObjects map[ObjID]*Object` (out-of-band, keyed by identity id,
  never in `objects`, never affecting `maxObjID`/`highWaterID`); init in `NewStore`; `AddAnonymous`.
- `db/format/reader.go:NewStoreFromDatabase`: ingest `database.AnonymousObjs` via `AddAnonymous`.
- `db/store/store_snapshot.go`: `planAnonymousSerializationLocked` computes reachability over
  `anonObjects` from live property `_TYPE_ANON` refs (transitively), assigns serialization ids starting
  at `maxObj+1` in identity-id order, and records a rewrite map (present → serial id, absent → NOTHING).
  `anonSerializationPlan.rewriteValue`/`rewriteSnapshotObject` deep-rewrite `_TYPE_ANON` references in
  every snapshot property value. `Snapshot` then emits the reachable anon objects as `AnonymousObjects`
  with their serialization ids and rewritten property values.
- `db/format/writer_object.go:writeAnonymousObjects` (unchanged logic): emits the count + objects + `0`.

### The 6 absent references
6 of the 739 `_TYPE_ANON` references point at anon objects not present out-of-band in mongoose.db.new.
They are rewritten to `#-1` (NOTHING) — exactly Toast's own `db_write_anonymous` `is_valid==false` path:
Toast reads `_TYPE_ANON(-1)` as a null anon value, allocates no slot, and VALIDATE passes. Nothing is
fabricated and no spurious object is created. (This is the Toast-reference behavior for a dead anon value,
not a guess; flagged here for visibility.)

### Gates (all green, anchored on mongoose.db.new)
- `db_roundtrip mongoose.db.new`: **0 parent drops, 0 "props N vs 0", 0 existence mismatches, SUCCESS**
  (maxObj #31225, 185 players, 27957 objects).
- **GATE A mongoose (LOAD + VALIDATE)**: WSL Toast reads 31226 regular objects then a **738-object anon
  batch**, **VALIDATE Phase 1/2/3 pass with ZERO errors**, reads 11347 verb programs, LISTEN, clean dump
  (Toast's redump writes 31226 + 738 = 31964 objects — it round-trips the anon objects identically).
- GATE A canonical (LOAD + VALIDATE): 127 objects, VALIDATE pass, 1949 verbs, LISTEN, clean.
- **Anon invariant** (verified on mongoose AND canonical): 0 anon objects at a regular numeric id, 0 anon
  objects in the regular `objects` map, 0 duplicate serialization ids.
- Conformance (managed Barn, synchronous): **3871 passed, 0 failed, 131 skipped** (143s).
- `go build ./...` exit 0; `go vet ./...` = the 2 known (`cmd/moo_client/main.go:53`, `vm/stack.go:49`);
  `go test ./db/store ./types` pass; `db/format` only the known missing-fixture fail
  (`TestLoadMongooseSnapshot`); `go list -deps ./db/store | grep parser` EMPTY (parser-free).
- B6 NOT regressed: canonical round-trip yields **1949** verbs (vs 1950 Toast-native).

### Status
PATH-EMIT complete and verified on the real database. The earlier "56 dangling refs in Test.db /
recoverable? / strip-vs-placeholder" fork is MOOT (Test.db was a stale bad dump; the real DB's anon
objects are present and round-trip faithfully). NOT merged (verifier next). B2c/W1/perf untouched.
