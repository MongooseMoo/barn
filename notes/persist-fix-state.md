# Persistence-cluster fix — working state

## Worktree
- C:/Users/Q/code/barn-fix-persist, branch fix/persistence-fidelity, off b9d3f61. CLEAN.

## Files read & understood
- db/store/store_snapshot.go — Snapshot(), snapshotObjectValue, snapshotPropertyNamesSelfFirst[Recursive].
  - line 64-66: routes `!recycled && anonymous` into AnonymousObjects (FIX 1 target).
  - Snapshot has AnonymousObjects []*SnapshotObject, Objects map keyed by id, AllObjects, PropertyNames.
  - PropertyNames computed for validLiveObject only (line 67). NEED to check validLiveObject.
- db/format/reader_object.go — readObjectCommon, readObject, readAnonymousObjects.
  - readAnonymousObjects (288-310): for each anon obj read, SetAnonymous(true) + database.Objects[id]=obj (FIX 1 read-side target).
  - recycled ids -> RecycledObjs only, returns nil (no object created).
- db/format/writer_object.go — writeObjects, writeAnonymousObjects(57-76), writeObject, writeProperties(195-243).
  - writeProperties writes len(propNames) as total + iterates same list. Short list shrinks count (FIX 3).
- db/format/reader_helpers.go — resolvePropertyNames, rawPropertyNames, propertyNamesSelfFirst[Recursive].
  - LOAD-side prop name walk. visited keyed by ObjID (snapshot side keyed by string name — ASYMMETRY noted).

## Still to read
- db/format/reader.go / reader_v17.go — where readAnonymousObjects is called, Database model fields.
- db/store/builder.go — ObjectBuilder API (SetAnonymous, etc).
- db/format/startup_repair.go:50-66 — parent drop logic.
- store.go validLiveObject / how store ingests builders.

## Plan (3 staged fixes)
FIX 1 (B3 crash): out-of-band anon tracking.
  - reader: don't key anon objs into Objects map by numeric id; track separately.
  - snapshot: only route genuine out-of-band anon into AnonymousObjects.
  - writer: emit anon only when reference-reachable, else just `0`.
  - GATE A: WSL Toast loads barn v17 of canonical + Test.db, no crash.
FIX 2 (B5 parent drops): verify FIX 1 killed 24 drops. GATE B: zero drops.
FIX 3 (B4 propval loss): faithful inherited prop-name reconstruction; #6 6 propvals. GATE C.

## STOP-ON-AMBIGUITY watch
- FIX 1 structural anon tracking + FIX 3 prop-name rework are the risky data-correctness pieces.

## Tooling
- db_roundtrip.exe (cmd/db_roundtrip) — load->write->reload + comparator (load store vs reload store).
- WSL Toast: /root/src/toaststunt/build-release/moo  (input-db output-db -p PORT). Access via `wsl bash -lc`.
- canonical: WSL ~/src/toastcore/toastcore.db -> _b3_canonical.db (2104432 bytes, COPIED).
- worktree toastcore.db is OLD committed copy (2083333) — use _b3_canonical.db for canonical tests.

## BASELINE (before any fix)
- Test.db roundtrip: 24 parent drops, 2844 "props N vs 0/partial". #6 props 6 vs 0. #53 9 vs 6.
  Loaded maxObj=#15577 players=411 objects=13451; reload counts match (lenient reader).
- Canonical roundtrip: SUCCESS (from reports).

## STRUCTURAL UNDERSTANDING (key for FIX 1)
- Database.Objects is map[ObjID]*ObjectBuilder, keyed by numeric id.
- parseV17: regular objs -> Objects[id]; then readAnonymousObjects -> ALSO Objects[id] + SetAnonymous(true).
  => anon obj with numeric id #510 COLLIDES/coexists with recycled regular #510 (which is nil/RecycledObjs only).
- NewStoreFromDatabase: s.Add(b.Build()) for every builder in Objects.
- repairStartupIssues runs AFTER full parse. validObjectID(#509)=false (recycled, no builder) -> parent dropped.
- Snapshot: routes !recycled && anonymous -> AnonymousObjects. writer emits them as anon section -> CRASH.

## RULE ZERO — Toast anon model (db_file.cc, db_io.cc, db_objects.cc) DECISIVE
- Anon obj has flags bit 256 (FlagAnonymous); recycled/invalid anon adds 512. Test.db #510 flags=768.
- Toast _TYPE_ANON value (db_io.cc db_read_anonymous): stores an oid. On read, if objects[oid] absent,
  allocates an anon object AT that oid slot (num_objects=oid; dbpriv_new_anonymous_object()).
  => anon oids ARE real array slots, created LAZILY from _TYPE_ANON references in tasks/values/finalizations.
- Anon SECTION (ng_read_object(1)): does dbpriv_find_object(oid) — EXPECTS the slot to already exist
  (pre-created from a _TYPE_ANON ref). It does NOT create. If never referenced => stale/garbage ptr => SIGSEGV.
- Toast writer only emits anon objs in the anon section that are reachable (have live _TYPE_ANON refs).
- CRASH ROOT: Barn writes anon objs (#510,#4895...) into anon section but writes NO _TYPE_ANON refs to
  pre-create them. Toast find_object(#510) on a never-created slot -> SIGSEGV. Confirmed by reports.

## DESIGN DECISION FOR FIX 1 (data-correctness)
- These anon objects in Test.db are NOT reachable from any live _TYPE_ANON reference (none of the
  3502/2642 are reference-reachable per investigate report). They are artifacts of a prior bad Barn dump.
- Toast's faithful output for this world = anon section is just `0` (canonical proves clean DBs do this).
- Are they referenced from the REGULAR world (as parent/prop OBJ value of a regular object)? Their oids
  are below maxObj and collide with RECYCLED regular slots. A regular OBJ value #510 would resolve to the
  recycled slot, not the anon object, in Toast. So they are effectively unreachable garbage.
- SAFE FAITHFUL FIX: track anon objects OUT-OF-BAND (not in Database.Objects keyed by numeric id), so
  they (a) don't collide with regular/recycled ids, (b) don't get ingested as phantom regular objects,
  (c) don't get re-emitted into the anon section unless reference-reachable. For Test.db -> anon section `0`.
- This means the 2642 anon objects are NOT ingested into the store as regular objects. That is CORRECT:
  they were never legitimate regular objects; Toast would never have them as numbered objects either.
  Their disappearance from store.All() is the bug being FIXED, not new data loss (Toast crashes on them).

## FIX 1 IMPLEMENTED (pending build/GATE A)
- reader.go: added Database.AnonymousObjs []*store.ObjectBuilder.
- reader_object.go readAnonymousObjects: anon builders -> AnonymousObjs (out-of-band), NOT Objects[id].
  => anon objs no longer ingested as regular objs, no numeric-id collision, not re-emitted.
- store_snapshot.go Snapshot(): added persistentAnonymousReachabilityLocked(); AnonymousObjects now
  filtered to reachable anon ids only. Empty for file-loaded DBs w/ no live anon refs -> anon section `0`.
- writer_object.go writeAnonymousObjects: comment clarified; logic unchanged (correct given filtered set).
- store_reachability.go already had collectAnonymousObjectRefs + expandAnonymousReachabilityLocked (reused).
NEXT: go build, db_roundtrip Test.db (measure), GATE A (WSL Toast loads canonical+Test.db barn output).

## FIX 1 RESULTS (MEASURED)
- build OK. Test.db roundtrip: parent drops 24->0; props losses 2844->2564; existence mismatches 0.
  objects 13451->9949 (phantom anon objs removed). Canonical roundtrip still SUCCESS.
- GATE A GREEN:
  * canonical barn output: Toast loads 127 objs, 1949 verbs, VALIDATE pass, LISTEN, clean dump, exit 0.
  * Test.db barn output: Toast "Reading 15578 objects ... Done reading 15578 objects" (NO bogus 3502
    second header), VALIDATE Phase 1/2/3 pass, LISTEN port 9492, clean dump, exit 0. NO SIGSEGV.
- FIX 2: parent drops already 0 after FIX 1 (the 24 were anon-id-collision artifacts). GATE B GREEN.
- Remaining: FIX 3 (B4) — 2564 props N vs 0/partial incl #6 6 vs 0.

## FIX 3 / B4 DIAGNOSIS (probe output, post-FIX-1)
- #1 "Root Class": propCount=0, snapshot names=[]. ROOT HAS ZERO PROPS in store after load.
- #6 "Server Options": propCount=6 (values kept as _inherited_0.._inherited_5), parents=[#1],
  snapshot PropertyNames=[] => writer writes 0 props. NAMES NEVER RESOLVED ON LOAD.
- #53: propCount=9, parents=[#0], snapshot names len=6 (PARTIAL). 3 unresolved (_inherited_6/7/8).
- KEY: #6 inherits 6 propvals but its parent #1 reports 0 props. So either #1's propdefs were lost
  on load, OR the inherited names come from a DIFFERENT chain than reader_helpers walks.
- NEED: inspect #1 raw propdef section in Test.db (after verbcount=1 + 1 verb meta). Determine #1's
  propdefcount/totalprops. If #1 raw has 6 propdefs but store shows 0 -> LOAD bug (resolvePropertyNames
  drops them). If #1 raw has 0 -> the 6 names live elsewhere (a child? #0?) and #6's parent in canonical
  is different. RULE ZERO: Test.db crashes Toast so can't read Toast's #6 directly; authoritative target
  per report = #6 round-trips 6 propvals (123/2147483647/1/2000/2000/3210; owner #9 perms 1).
- store_reachability still has the original PersistentAnonymousReachability (unused dup now). OK leave.

## FIX 3 RULE ZERO RESOLUTION (Toast ng_read_object db_file.cc ~386-396)
- Toast reads nprops = dbio_read_num(), then EXACTLY nprops propvals positionally (o->propval[i]).
  NO validation vs parent propdef count at read time. Writes back nprops + values. => propvals are
  POSITIONAL; names resolved later. #6's 6 propvals are legitimate, must round-trip by COUNT.
- #6 chain is #6->#1, #1 has 0 propdefs. The 6 names [nothing,system,object,anonymous,server_options,
  waif] are defined on #0 (sibling, parents=[1]). #6 inheriting from #1 yields 0 names positionally —
  this DB stores 6 "extra" positional propvals that Toast keeps. So names can't come from #6's true
  ancestry; resolution to _inherited_N is the honest state.
- FIX 3 = writer/snapshot MUST emit count == object's stored propval count, preserving value/owner/perms,
  never shrinking. Names: use resolved name where available, else the stored placeholder. NOT a corruption:
  values/owner/perms are faithful; Toast round-trips by count. This matches the report's authoritative
  target exactly (#6 -> 6 propvals 123/2147483647/1/2000/2000/3210, owner #9, perms 1).

## DESIGN: make snapshot PropertyNames = object's FULL propOrder (stored), not a re-walked short list
- store keeps obj.propOrder (the per-object ordered name list, len == propCount). Snapshot should hand the
  writer THAT (already in correct stored order incl placeholders), so count never shrinks. The parent-chain
  re-walk (snapshotPropertyNamesSelfFirst) is what produces the SHORT list. Replace with obj.propOrder.
- propDefsCount stays = obj.propDefsCount (local defs), which writer uses for the propdef-name prefix.
- Verify: does store preserve propOrder through ingest? Object has propOrder field; builder.Build keeps it;
  snapshotObjectValue currently does NOT copy propOrder into SnapshotObject. NEED to add it.

## FIX 3 FINAL DESIGN (handles loaded AND runtime)
- propagatePropertyToDescendantsLocked (store_properties.go:588) adds inherited prop to descendant
  .properties map but does NOT append to descendant.propOrder => propOrder can be SHORT for runtime.
  That is WHY original used parent-chain walk. But walk goes SHORT when chain broken/unresolved (#6).
- ROBUST FIX: snapshot name list = obj.propOrder (authoritative load order) PLUS any property key in
  obj.properties not already in propOrder (runtime-inherited), appended. Guarantees count==len(properties)
  ALWAYS, preserves per-name value/owner/perms. Order: propOrder first (correct), runtime extras appended.
- Implement in store_snapshot.go Snapshot(): replace snapshotPropertyNamesSelfFirst with a function that
  reads obj.propOrder + appends missing map keys (sorted for determinism). propDefsCount unchanged.
- Writer (writer_object.go) already iterates PropertyNames + writes len as total + first propDefsCount as
  propdefs. With full list, count never shrinks. Add guard: propDefsCount = min(propDefsCount, len(names)).
- GATE C target: zero "props N vs 0", #6 -> 6 propvals intact.

## !!! BLOCKER: FIX 3 REGRESSED GATE A on Test.db !!!
- FIX-3 db_roundtrip Test.db: SUCCESS, 0 parent drops, 0 prop losses, #6 6 propvals correct.
- BUT WSL Toast on FIX-3 Test.db output: 224 "VALIDATE: #N.parents/children/location/contents is not
  an object/list of objects" -> "READ_DB_FILE: Errors in object hierarchies" -> "DB_LOAD: Cannot load!".
  FIX-1 output had 0 such errors and loaded+listened. So FIX 3 changed object serialization in a way
  that corrupts ~224 objects' parents/children/location/contents fields (NOT propvals).
- Affected: #14757, #14763, #15229, #15234 ... (high ids). HYPOTHESIS: my snapshotPropertyNames change
  causes an object to emit WRONG propval COUNT for some object EARLIER, desyncing the byte stream so a
  later object's parents/children parse as garbage. OR: an object whose properties map has MORE entries
  than before now writes extra propvals, shifting alignment. Toast reads positionally; a wrong count on
  obj K mis-frames obj K+1..., surfacing as "not a list of objects" downstream.
- KEY: Barn's lenient reader masks this (db_roundtrip SUCCESS) but Toast strict-validates. RULE ZERO:
  Toast is the authority. Must find which object emits a count Toast disagrees with.
- NEXT: find first object where FIX-3 output diverges from FIX-1 output in a way that breaks framing.
  Compare per-object propval counts FIX-1 vs FIX-3; the regression is where count grew incorrectly.

## DESYNC INVESTIGATION (FIX-3 GATE A regression)
- FIX-3 object section = FIX-1 + ADDED propvals only (diff shows only `>` additions). Barn self-consistent
  (count==emitted per obj; db_roundtrip SUCCESS; probe: 0 anomalies, names==props for all).
- Toast read loop "Done reading 15578 objects" completes, but VALIDATE rejects 224 objs (#5179 first,
  high ids). Toast read_propval = var + owner(objid) + num(perms). CLEAR var = 1 line (type only).
  Barn CLEAR = type+owner+perms = 3 lines == Toast. INT = type+val = 2 + owner+perms. Consistent.
- So a restored propval VALUE must serialize to a different LINE COUNT in Toast vs Barn for SOME type.
  Suspect types: the restored inherited slots whose VALUE is non-INT (list/map/float/str/err/waif/anon).
  If Barn writes e.g. an OBJ value as 2 lines but the value is actually something Toast reads as 1, or a
  multi-line value mismatch, framing desyncs AFTER that object -> later object headers land on garbage ->
  VALIDATE sees garbage parents/children. (#5179 etc are recycled in Barn output; Toast reading them as
  full objs confirms desync.)
- NEXT: find FIRST restored propval whose value type serializes differently. Check writeValue vs Toast
  dbio_write_var for each type. Esp: does Barn write owner/perms for a propval whose value is CLEAR via
  writeProperty (yes). Check the "missing name" branch (TypeClear+(-1)+0). Check writeValue map/list/float.

## !!! DESYNC ROOT CAUSE FOUND (NOT framing) !!!
- Toast VALIDATE Phase 1 (db_file.cc 614+): iterates oid 0..last_used; for each EXISTING object checks
  parents/children/location/contents type. Error "#5179.parents is not an object or list" => slot 5179
  IS occupied by an object with garbage fields. Barn writes #5179 as RECYCLED (dbpriv_new_recycled_object
  bumps count, no object) so find_object(5179) should be NULL.
- It is NON-NULL because Toast read a _TYPE_ANON value with oid=5179 in some propval -> db_read_anonymous
  -> num_objects=5179; dbpriv_new_anonymous_object() -> allocates UNINITIALIZED anon obj at slot 5179.
  VALIDATE then sees garbage parents. => FIX 3 RESTORED propvals that contain _TYPE_ANON refs (NewAnon)
  pointing at the very anon objects FIX 1 removed. So those anon refs ARE present in the world (in
  property values), CONTRADICTING the assumption that none are reference-reachable!
- This means: (a) some anon objects ARE referenced from regular props (via NewAnon values), so FIX 1's
  "anon section = 0" drops referenced anon objects -> dangling _TYPE_ANON refs -> Toast allocates empty
  slots -> validate fail. (b) FIX-1 masked this only because it ALSO dropped the propvals holding those
  refs (#6 etc lost all props incl any anon refs). Restoring propvals re-exposed the dangling refs.
- error ids: 5179,5185,5480,5484,5899,5905... PAIRS ~6 apart, groups ~300 apart. 56 ids. Structured.
  These are anon oids referenced by restored propvals.
- CORRECT FIX (RULE ZERO): the anon objects referenced by live _TYPE_ANON values MUST be emitted in the
  anon section so Toast fills them in (not left as bare allocations). My reachability filter SHOULD have
  caught them — but the anon objects were removed from the STORE entirely (FIX 1), so reachability finds
  the ref ids but no store object to emit. => Need to RE-INGEST referenced anon objects, OR emit the
  loaded out-of-band AnonymousObjs that are referenced. THE WRITER PATH (db_roundtrip) uses store.Snapshot
  which has NO access to db.AnonymousObjs. ARCHITECTURE GAP.

## STOP-ON-AMBIGUITY POINT REACHED?
- This is a genuine fork on data correctness. Options under consideration before deciding:
  A. Re-ingest referenced anon objects into the store with their loaded ids (risk: id collision w/ recycled
     regular slots; Toast tolerates via db_read_anonymous allocating at that slot — but our store keys by id).
  B. Keep anon out-of-band AND thread db.AnonymousObjs through to the writer; emit referenced ones in anon
     section; rewrite their _TYPE_ANON refs. Larger structural change.
  C. Verify whether these refs are REAL (in canonical/legit data) or artifacts of the prior bad Barn dump.
     If artifact-only, the faithful output may legitimately drop both the ref AND the anon object.
- MUST verify (C) FIRST before any structural choice. Check: do the restored propvals on #6 etc actually
  contain NewAnon values, or is the _TYPE_ANON in some OTHER restored object? Identify which restored
  propval holds NewAnon(5179).

## DECISION ANALYSIS (the fork)
- VERIFIED: canonical (clean Toast DB) has 0 anon refs, 0 anon objs. ALL Test.db anon machinery is a
  corrupt prior-Barn-dump artifact. 56 anon refs in props; referenced anon objs are empty(name="",0 props)
  or MISSING entirely. Toast's loader allocates a slot for EVERY _TYPE_ANON value (db_read_anonymous) and
  expects the anon section to fill it; an unfilled slot => VALIDATE garbage => DB_LOAD fail.
- Two correct-ish paths, both structural/risky (the STOP-ON-AMBIGUITY scenario named in the prompt):
  PATH-EMIT: thread db.AnonymousObjs to the writer; for every reference-reachable anon oid emit an anon
    object (loaded data if present, else minimal valid empty obj). Matches Toast exactly. BUT db_roundtrip
    writer uses store.Snapshot which has no db.AnonymousObjs; needs the anon objects RE-INGESTED into the
    store (reintroducing id-collision w/ recycled regular slots) OR a new writer channel. Larger change,
    data-correctness risk (the exact thing I was told to STOP on).
  PATH-STRIP: a property holding a corrupt _TYPE_ANON ref is itself corrupt; faithful output would not
    contain it. But silently mutating property values is its own corruption risk and not clearly "Toast".
- GATE STATUS at decision point:
  * GATE A canonical: GREEN (Toast loads barn canonical output, 127 objs, 1949 verbs).
  * GATE A Test.db: was GREEN after FIX-1 (props dropped); FIX-3 restored props -> exposes the corrupt
    anon refs -> RED (224 VALIDATE errors, DB_LOAD fail). The crash (SIGSEGV) is GONE either way.
  * GATE B (parent drops): GREEN (0).
  * GATE C (db_roundtrip prop loss): GREEN (0; #6 6 propvals intact).
- This is a true data-correctness fork. STOPPING per directive. FIX 1+2 are solid and committed. FIX 3 is
  correct in isolation (faithfully restores propvals) but surfaces a latent pre-existing corruption
  (dangling _TYPE_ANON refs) that needs a structural anon-emit design decision from Q.

## FIX 1 design decision (out-of-band anon tracking)
- Add Database.AnonymousObjs (separate list/map NOT in Objects), so anon ids don't collide w/ regular space
  and don't get re-emitted into anon section on write unless genuinely reference-reachable.
- For these reference DBs none are reference-reachable anon => correct anon section == `0`.
- Need: builder has SetAnonymous + anonymous flag flows to SnapshotObject.Anonymous. Store must still
  ingest anon objects for runtime? Check: does NewStoreFromDatabase need anon objs? They were keyed into
  Objects so currently they ARE ingested. Removing from Objects drops them from store. DECISION below.
