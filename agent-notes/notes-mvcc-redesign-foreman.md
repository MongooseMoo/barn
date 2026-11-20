# MVCC Concurrency Redesign — driving log (foreman/coder/verifier)

Branch: `mvcc-concurrency-redesign` (off master @ cf3687d).
Plan: plans/mvcc-concurrency-redesign-2026-07-21.md — follow in order.

## Goal
Absolute 32-core goodput (committed cmds/sec) on a mongoose-shaped mix, with abort
rate + p99. NEVER the serial/pool ratio (June metric trap).

## Current phase: Phase 0 — measurement harness that cannot lie

### What I know (verified by reading code)
- Existing bench infra lives in scheduler/*_test.go. Key helpers (unexported, so
  harness must be a _test.go in package scheduler):
  - `s.buildBenchTask(t, id, code)` — compiles MOO string → *task.Task, wizard perms.
  - `s.runTask(tk)` — runs one task synchronously (task_runtime.go:34). Production
    per-connection goroutines call this concurrently (input_processor.go:669) — so
    driving P goroutines each calling runTask is the FAITHFUL parallelism model,
    NOT the batch scheduler (ProcessReadyTasks).
  - `newSchedulerWithWorkerCount(store, opts, workers)`.
  - Commit counters: store.CommitAttempts/Successes/Conflicts/Retries();
    `sampleCommitCounters(store)` + `.sub()` gives per-run deltas
    (concurrency_matrix_test.go:15-40). Fresh store per run avoids reset races.
  - Store build: `dbstore.NewStore()`, `NewObjectBuilder`, `store.CreateObject(parents,
    owner, anon)`, `store.DefineProperty`, `store.AddVerb`.
- move() builtin (builtins/objects_movement.go:11) calls markLiveStoreMutated(:70)
  → coarse EXCLUSIVE commit (stop-the-world). This is Phase 3's target.
- markLiveStoreMutated callers: objects.go, objects_hierarchy.go, objects_misc.go,
  objects_movement.go, verbs.go, store_reads.go.

### Design decision (so the instrument can measure EVERY phase)
Harness drives REAL MOO through a fixture with a real inheritance chain so:
- Phase 2 (alias reads): wide `look` iterating room.contents reading occupant props.
- Phase 3 (move off stop-the-world): move-shaped uses the REAL move() builtin.
- Phase 4 (scan deps): look/say resolve verbs+inherited props UP shared ancestry;
  a writer task periodically touches a shared-ancestor prop (pathological case).
Mix ~ look40/say30/move20/misc10; Zipfian room occupancy; sweep players×rooms.
P concurrent goroutines, each = one simulated player running a task stream.
Report: goodput(committed/sec), abort rate(retries+conflicts/attempts), p99 latency,
allocs/op, GODEBUG=gctrace summary. Warm-up then timed window, repeat>=5.

### Blocker / open questions
- Confirm runTask is goroutine-safe to call concurrently on one scheduler with
  distinct tasks (production says yes via connectionWorker; must verify no shared
  per-scheduler mutable state in buildBenchTask path).
- Need exact MOO syntax for calling move() and for list setadd/setremove on contents.
- Decide fixture parent ids (#3 is wizard). Root/#0/room-generic/player-generic.

### Verified API facts (ready to write harness)
- move(what,where): MOO builtin, calls markLiveStoreMutated → coarse EXCLUSIVE
  commit (stop-the-world). No `accept` verb needed (E_VERBNF handled; IsWizard
  bypasses truthiness). Phase 3 target.
- `.contents`/`.location` are builtin pseudo-props (properties.go:544) → relationship
  reads. look verb does `this.contents` iterate.
- random(n) → [1,n] (math.go:112), 1-based-index friendly. USES GLOBAL math/rand
  (mutex) — known instrument caveat, constant across phases, only move() calls it.
- create/recycle/move/notify/tostr/length/random all registered.
- NewTaskFull(id, owner, *bytecode.Program, tickLimit, secondsLimit) — Program is
  immutable/reusable → precompile each shape ONCE outside timed window.
- compileTestProgram(t, s.registry, code) → *bytecode.Program.
- ObjectBuilder: SetParents/SetOwner/SetFlags/SetProperty/AppendVerb/Build; or use
  store.CreateObject(parents,owner,anon)+DefineProperty+AddVerb+MoveObject+SetPropertyValue.
- runTask (task_runtime.go:34): recovers panics, Begins fresh BeginReadOnly txn per
  attempt, Release() on defer. Concurrent calls = production model (connectionWorker).

### FIXTURE DESIGN (ids captured from CreateObject, NOT hardcoded)
- wizard #3 (builder, owner 3, wiz|prog|user). sysID, rootID, roomGenID(parent root),
  playerGenID(parent root) via CreateObject.
- rootID props: desc(str,r) [inherited by all → scan-dep target], churn(int,rw)
  [Phase 4 ancestor-write target]. roomGenID: tick(int,rw). playerGenID: last_activity(int,rw).
- roomGenID verbs: look (reads this.contents, iterates o.name+o.desc), announce (contents+name).
- R rooms(parent roomGenID), P players(parent playerGenID, moved into Zipf room),
  inert objs(parent rootID, Zipf into rooms). sysID.destpool = Zipf-weighted room list.
- Shapes (fmt with ids): look `loc=#P.location;loc:look();` / say `loc:announce()` /
  move `dest=#sys.destpool[random(N)];move(#P,dest)` / stamp `#P.last_activity+=1` /
  build `o=create(#root);recycle(o)` / churn `#root.churn+=1` (Phase4 pathology).
- WHY faithful to every phase: look=wide+deep read (P2), move=real stop-the-world (P3),
  o.desc inherited fall-through + churn on rootID = scan-dep false-conflict (P4).

### Reporting: goodput(committed/measureDur), abort=retries/attempts (+conflicts),
  p99/p50/max latency (collect all, sort), allocs/op+bytes/op+NumGC+pauseTotal via
  MemStats delta (runtime.GC() before snapshot). Gate behind env BARN_MVCC_BENCH.

### BLOCKER (current): confirm runTask does NOT leak task into a persistent scheduler
  map over a long window (millions of tasks). Serial benches call runTask directly
  then removeTasksForOwner — need to see if direct runTask registers persistently.

### DONE: harness written = scheduler/mvcc_workload_bench_test.go
- go vet clean, go build ./... OK, smoke under -race PASS & race-clean (runTask IS
  concurrent-safe as production assumes). Gated behind BARN_MVCC_BENCH=1.
- Env knobs: BARN_MVCC_PLAYERS, _ROOMS, _WARMUP, _MEASURE, _REPEATS (comma lists).
- Two scenarios: realistic (look40/say30/move20/stamp7/build3) and churn-stress
  (adds churn10 = ancestor-prop write = Phase 4 pathology).
- direct runTask does NOT register in s.tasks (task_factory:32 QueueTask does; runTask
  34-340 does not) → no task-map leak over long window. Verified.

### BASELINE SIGNALS (non-race, prelim, rooms=16, 600ms) — instrument is FAITHFUL:
  realistic:  1p 22.3k/s 0%ab | 8p 28.6k/s 0% | 32p 29.7k/s 0%  → only 1.33x on 32
    threads @ 0% abort = STOP-THE-WORLD move() coarse lock (Phase 3 target). NOT conflicts.
  churn:      1p 23.1k/s 0% | 8p 35.3k/s 13%ab | 32p 31.8k/s 28%ab → abort storm rises
    with contention, goodput DROPS 8->32 = false-conflict on ancestor write (Phase 4).
  ALLOC TAX: ~190 allocs / ~34KB per op, 241 GCs/0.6s = deep-clone-on-read wall (Phase 2).
  => instrument shows all 3 diseases (P2 alloc wall, P3 no-scaling, P4 abort storm).

### NOW: full baseline sweep (players{1,4,16,32}×rooms{4,16,64}×2 scenarios×5 repeats,
  2s window ~5min) → write experiments/2026-07-21-mvcc-baseline-curve.md → commit.
## PHASE 0 COMMITTED: 2d4ef1a (harness + baseline curve + raw log). scheduler -short green.

## PHASE 1 IN PROGRESS: re-land floor shard + GOGC=400
- `git cherry-pick --no-commit c09bcae` applied with NO textual conflict but a
  SEMANTIC one: master-side method `ActiveReadTransactions()` (store_core.go:166-176)
  still references removed `s.floorMu` / `s.activeReadTS`. Build fails.
- Shard layout NOW: `readTSShards [16]readTSShard`; readTSShard{mu sync.Mutex;
  counts map[uint64]int; pad}. register/deregister at store_history_gc.go:30/44 via
  `readTSShards[readTS%16]`. historyFloor() scans all shards (:69-73).
- FIX: reimplement ActiveReadTransactions() to lock each shard.mu and sum counts.
- Floor shard COMMITTED: 604dd09. Fixed ActiveReadTransactions() to scan shards.
  db/store -race full pkg GREEN.
- GOGC: edited cmd/barn/main.go const 200->400 (:39) + comment (measured sweep
  100->4.56x..400->8.50x..off->11.41x, from 8d47c44) + flag help (:162) + precedence
  comment (:179). NEXT: grep other "200" GOGC refs; build; verify log emits percent=400.
- THEN Phase 1 close: re-run Phase 0 harness on branch HEAD, compare vs baseline
  (2d4ef1a numbers), record experiments/ delta, commit GOGC. Phase 1 exit = both on
  branch, conformance green.
## PHASE 1 near-done. Commits: 604dd09 (floor shard), 2609bd8 (GOGC=400).
- Verified GOGC=400 logs at boot (percent=400). db/store -race green. build clean.
- Harness re-measure (GOGC single-variable, -count=1 to defeat test cache): GOGC=400
  gives ~1.5-1.8x goodput, GC count 294->31 (robust signal). Recorded in
  experiments/2026-07-21-mvcc-phase1-reland.md.
- GOTCHA LOGGED: `go test` caches results; env-only change (GOGC) served STALE
  identical-to-digit output. ALWAYS -count=1 for env-varied harness runs.
- Floor-shard win within noise on read-heavy mix (proven on commit-dominated
  microbench; not claimed here). Harness variance ~±20% at 2s window — trust only
  GC-count + >1.4x deltas.

### OWED before Phase 1 fully closed:
1. Commit the phase1 experiments record.
2. Full go test ./... (in-repo gate).
3. Managed conformance gate (plan invariant). Behavior-preserving changes (floor
   refactor + runtime GC tuning) so risk low, but invariant is explicit — run the
   managed suite per plans/barn-toast-mongoose-convergence-workstreams.md OR confirm
   with Q that build+race+full-go-test suffices for these two behavior-neutral relands.

## Q APPROVED (2026-07-22): proceed on my judgment. Managed conformance gate DEFERRED
to end of Phase 2 (Phase 1 changes behavior-neutral; full go test green). Phase 1 DONE.

## PHASE 2 IN PROGRESS — aliasing safety audit (read-only, changes nothing yet)
Design: read-only txns alias the immutable published image ptr; writer txns copy on
first STAGED WRITE (true COW). Today tx.object (store_txn.go:157) caches a DEEP clone
(objectLocked:174 -> cloneObjectForReadTxn:177/184/198) and serves it for all reads.

### KEY FINDING SO FAR (must verify): where is tx.objects[id] mutated in place?
tx.objects[id] assignment sites (store_txn.go): 158(read cache hit), 162/210/220/255/
321/480(=nil miss/invalidate), 170(cache fill w/ clone), 223(AdoptLiveObject re-clone),
324/327(?), 566(obj:=tx.objects[id] then ?). Staging funcs: SetObjectName:732,
SetObjectOwner:746, SetObjectFlag:760, SetObjectLocationRaw:778, SetPropertyValue:1226,
SetPropertyInfo:1260, DefineProperty:1293, DeleteDefinedProperty:1426, SetVerbCode:1778/1787.
QUESTION being answered: do these stage into SEPARATE write-set maps (safe to alias base
image) OR mutate the cached *Object in place (would corrupt an aliased shared image)?
If separate write-set maps -> aliasing the base object struct is safe; only need to
confirm (a) no in-place mutation of the cached obj, (b) VM copies MOO list/map values on
mutation (never aliases a value out of the image and mutates it).

### CRUX INSIGHT (verified by reading code myself) — narrows the whole audit:
1. Store NEVER mutates a published image or its *Property/*Verb nodes after Store
   (contract store_core.go:12-27). VERIFIED.
2. Commit path buildImageWithPropertyValue/Scalar/Relationship (store_cow.go:34-103)
   all do `img:=*old` then build a NEW map / overwrite only the new struct — the BASE
   image is never mutated ("old image's stored value left untouched"). VERIFIED →
   writer-txn aliasing (2b) safe at commit.
3. ***VALUE BACKINGS ARE ALREADY SHARED TODAY***: cloneObjectForReadTxn (store_txn.go:
   624) copies the properties MAP but each Property is copied BY VALUE (:636-638), so
   Property.value's types.Value.ref points to the SAME *sliceList/*mapData as the
   published image. So the list/map/str append-in-place COW machinery (types/list.go:
   82-100 watermark fast path; str.go same) is shared txn<->image REGARDLESS of Phase 2.
   => Phase 2 aliasing does NOT change value-backing sharing. The VM list/map/str
   value-semantics question is PRE-EXISTING and NEUTRAL to Phase 2 (if safe today, safe
   after). This removes it from Phase 2's NEW risk surface.
4. Therefore Phase 2's ONLY NEW risk = in-place mutation of the aliased OBJECT STRUCT /
   its properties map / propOrder+parents+children+contents slices / verbs map+list /
   chparentChildren map / the Property & Verb NODES. If staging methods write to
   SEPARATE per-txn write-set maps (not the cached *Object), Phase 2 is safe.

### STILL TO VERIFY (crux of new risk): staging model + in-place *Object mutation sites.
Scout agent (a7628d40) enumerating in parallel -> notes/mvcc-phase2-scout-enum.md. I
must VERIFY its staging-model cites myself (subagent report = claim not evidence).
Watermark append-in-place concurrency (2 concurrent txns sharing a frontier header)
is a PRE-EXISTING question — note it, but not a Phase-2 blocker.
### AUDIT COMPLETE (notes/mvcc-phase2-aliasing-audit.md). Scout reconciled+verified.
VERDICT: Phase 2 SAFE with one COW boundary. New-risk surface = in-place mutation of
the cached obj by staging methods (Category A) + adopt paths (Category A', which clone
ONLY on cache-miss so they'd mutate the alias — VERIFIED hazard, corrected my 1st glance).
Value semantics SAFE (O1 resolved: fresh-loaded lists watermark==nil -> first append
copies). Store-side coarse mutators = Phase 3 (O3).

Category A/A' sites to route through mutableObject (all db/store/store_txn.go):
SetObjectName:738, SetObjectOwner:752, SetObjectFlag:767/769, SetObjectLocationRaw:784,
SetPropertyValue:1238/1255, SetPropertyInfo:1275, DefineProperty:1309/1315,
propagateDefinedProperty:1350/1354, ReseedInheritedProperties:1131/1139,
ApplyStagedProperties:575-600, DeleteDefinedProperty:1441-1444,
removeInheritedProperty:1483, stageVerbCode:1803-1804 (on DEFINER's verb node),
AdoptLiveVerbs:260-266, AdoptLiveRelationships:324-338 (clone-on-miss -> must clone
UNCONDITIONALLY). AdoptLiveObject:223 already unconditional-clone (safe).

### IMPLEMENTATION PLAN (slice 2a, behavior-preserving refactor FIRST, then flip):
1. Add tx.owned tracking + tx.mutableObject(objID): if cached obj not owned, clone it
   (cloneObjectForReadTxn), store back, mark owned; return. Idempotent. mutableVerb via
   copying the definer object.
2. Route every Category-A/A' site: `obj := tx.object` -> `obj := tx.mutableObject`.
   While tx.object STILL clones, mutableObject on an already-private clone just marks
   owned (no 2nd copy) => pure no-op refactor, behavior identical. Verify full suite.
3. Clone-and-canary guard (test build): assert tx.object's returned image is never
   mutated in place (only mutableObject copies are). Run full go test + conformance.
4. FLIP tx.object read path to ALIAS (return live/history/anon image, no clone).
   mutableObject now does the real copy-on-first-write. Measure harness (allocs/op must
   drop hard, esp 64-room cells; goodput improve-or-neutral; GC down). db/store -race.
5. Conformance gate (managed) at Phase 2 end (owed from Phase 1 too).

### 2a IMPLEMENTATION IN PROGRESS (store_txn.go):
- DONE: added `owned map[ObjID]bool` field + `mutableObject(objID)` COW helper (clones
  from cache if not owned, marks owned, idempotent). Placed before objectLocked.
- DONE routing -> mutableObject: SetObjectName:766, SetObjectOwner:780, SetObjectFlag:794,
  SetObjectLocationRaw:812.
- TODO routing (verified sites, mutated obj noted): SetPropertyValue:1260(obj),
  SetPropertyInfo:1294(obj), DefineProperty:1327(obj), propagateDefinedProperty CHILD:1373
  (current:1367 is read-only-leave), ClearPropertyOverride:1407(obj) [NEW-found, mutates
  1417], DeleteDefinedProperty:1460(obj), removeInheritedProperty CHILD:1509 (current:1503
  read-only-leave), ApplyStagedProperties:599 (uses tx.objects[id] directly -> mutableObject),
  ReseedInheritedProperties:1149(read next), SetVerbCodeByIndex:1820 + SetVerbCode:1811
  (verb DEFINER obj), AdoptLiveVerbs:268(obj), AdoptLiveRelationships:333 (clone-on-miss ->
  clone UNCONDITIONALLY).
- HasDefinedPropertyInDescendants:1425 = READ-ONLY, leave as tx.object.
- SAFETY NET: canary guard (step 3) will catch any MISSED site by failing tests before
  the flip ships. So route comprehensively then trust guard+suite+conformance.
- STRATEGY: step1 = route all + KEEP tx.object cloning (double-clone, behavior-identical)
  -> verify suite green. step2 = flip tx.object to alias + canary guard -> verify+measure.

### 2a ROUTING PROGRESS (store_txn.go, tx.object -> tx.mutableObject):
DONE: mutableObject refactored to use privatizeCached (lock-free clone+own helper, so
callers holding store.mu don't nest RLock). DONE routed: SetObjectName/Owner/Flag/
LocationRaw, SetPropertyValue, SetPropertyInfo, DefineProperty, ReseedInheritedProperties,
ClearPropertyOverride, DeleteDefinedProperty, propagateDefinedProperty CHILD (->mutableObject).
TODO: removeInheritedProperty CHILD:1509, ApplyStagedProperties:599 (inline privatizeCached,
lock-free, no load), AdoptLiveVerbs:272 (->mutableObject, before its RLock), 
AdoptLiveRelationships:357-361 (inline privatizeCached: if nil clone-from-live, elif !owned
privatize alias — MUST NOT call mutableObject, holds store.mu.RLock), SetVerbCodeByIndex:1821
(->mutableObject, verb from private clone), SetVerbCode:1811 (privatize DEFINER + re-resolve
verb via findVerb after privatizing, else stageVerbCode mutates aliased verb node).
LOCK RULE: mutableObject calls tx.object which RLocks; NEVER call it while holding store.mu
-> use privatizeCached inline there (AdoptLiveRelationships, ApplyStagedProperties).
### 2a ROUTING COMPLETE + GREEN (uncommitted). All Category-A/A' sites routed through
mutableObject/privatizeCached. go build OK, go test ./db/store green (behavior-identical
pre-flip: mutableObject double-clones while tx.object still clones — a no-op refactor).

### *** HARD BLOCKER FOUND — flip is UNSAFE. Plan kill-criterion tripped. ***
The COARSE commit path (store_txn.go:1609+ under store.mu.Lock EXCLUSIVE) mutates the
PUBLISHED live *Object IN PLACE: live.name=1644, live.owner=1647, live.flags=1650, +
relationship/property/verb writes below. rememberObjectLocked (store_core.go:231-234)
clones the OLD image into history first, then mutates live in place + stamps new version.
=> New readers & old-readTS readers are served correctly (live / history-clone). BUT an
existing ALIASING reader that cached `live` before the coarse mutation reads it LOCK-FREE
and would (a) race with the in-place write, (b) see its readTS snapshot violated.
TODAY safe ONLY because readers clone (they hold a clone, not live). Phase 2 aliasing
BREAKS this. NOT fixable by staging routing (it's the COMMIT mutating published images).
This VIOLATES the store's own immutability contract (store_core.go:12-27) which the
coarse path already breaks — safe today only via read-cloning.
Coarse builtins that hit this: create/recycle/chparent/move/add_verb/delete_verb/
setverbinfo/setverbargs/renumber (10 markLiveStoreMutated sites). Also non-txn Store.SetX
API mutates live in place (load/setup + coarse), same hazard if aliased.

### DECISION FORK for Q (reported): Phase 2 (alias reads) is COUPLED to coarse-path
in-place mutation. To flip safely, the coarse commit must PUBLISH NEW IMAGES instead of
mutating live in place (restores true immutability; separable from Phase 3's exclusive-
lock removal). Options: (A) fix coarse apply -> build+publish new images (F1), then flip
Phase 2 [my rec]; (B) narrow Phase 2 scope; (C) reorder coarse-conversion before aliasing.
Anon objects (no slot) need care under F1. Uncommitted routing kept as foundation (do NOT
commit alone — it double-clones = pessimization without the flip).
### Q APPROVED OPTION A (2026-07-22): make coarse path publish new immutable images
instead of mutating live in place, then flip Phase 2. Keep exclusive lock (Phase 3 later).

## OPTION A EXECUTION — make published images truly immutable
SURFACE = every `rememberObjectLocked(X)` + mutate-X-in-place site = ~50 sites, 6 files:
store_core.go(5), store_lifecycle.go(19), store_properties.go(9), store_relationships.go(6),
store_txn.go(5 coarse-commit), store_verbs.go(6). UNIFORM pattern (verified: MoveObject
store_relationships.go:41-55 == coarse commit == all: hold s.mu.Lock, rememberObjectLocked,
mutate in place).

TRANSFORM: `s.rememberObjectLocked(X)` + mutate X  ->  `X = s.republishForMutation(X)` +
mutate the returned fresh. republishForMutation(old): clone old->fresh; publish fresh
(slot.ptr.Store for numbered / s.anonObjects[id]=fresh for anon); retain OLD immutable in
history (like rememberOldImageLocked, NO clone) + pruneObjectHistory; return fresh. Safe
because caller holds s.mu.Lock EXCLUSIVE -> fresh not yet aliasable (readers need RLock);
old never mutated -> pre-existing lock-free aliasers safe. Mirrors rememberObjectLocked
(store_core.go:226-245) but stashes old (not a clone) + publishes fresh first.
PER-SITE RULE: capture return into the mutated local. Sites doing rememberObjectLocked(
s.load(id)) then re-load via s.load: slot-swap makes later s.load return fresh (works);
sites mutating the SAME passed local MUST capture. Coarse-commit loops: `live=liveObjectLocked;
if !remembered{ live=republishForMutation(live); remembered=true }; mutate live` — 2nd-loop
touches return fresh via liveObjectLocked (already swapped). 

SAFETY NET: deterministic immutability test (red->green): for each mutation kind assert the
SUPERSEDED image is byte-stable + slot pointer CHANGED. Catches any missed site.

PLAN: (1) add republishForMutation helper (store_core.go near rememberObjectLocked).
(2) PILOT: convert MoveObject(3 sites) + red->green immutability_test for move. verify+ -race.
(3) scale file-by-file, running immutability test+build+-race after each. (4) flip tx.object
to alias + reader-survives-concurrent-mutation test. (5) full suite+-race+conformance+harness.
### Uncommitted: step-1 txn routing (mutableObject) — foundation, commit WITH the flip.
### PROGRESS:
- republishForMutation helper ADDED (store_core.go, after rememberObjectLocked).
- immutability_test.go ADDED: TestMovePublishesNewImageNotInPlace (RED->GREEN proven).
- PILOT DONE: MoveObject (store_relationships.go) converted -> GREEN, full db/store + -race pass.
- Transform PROVEN: `s.rememberObjectLocked(X)` + mutate X -> `X = s.republishForMutation(X)` + mutate.

### REMAINING SITES (post-shift line numbers):
- store_core.go: 448/463/478/493 = SetObjectName/Owner/LocationRaw/Flag, ALL `s.remember(obj)`
  + mutate local obj. All 4 identical -> replace_all `\ts.rememberObjectLocked(obj)` ->
  `\tobj = s.republishForMutation(obj)` (only these 4 match that exact call form in file).
- store_verbs.go: 356/387/434/516 = `s.remember(obj)` local; 469/495 = `s.remember(s.load(objID))`
  inline -> READ to see if mutation re-loads via s.load (then discard-return ok) or captured local.
- store_relationships.go: 291(oldParent)/299(obj)/302(s.load(parentID)) = chparent -> READ.
- store_lifecycle.go: 19 sites (create/recycle/chparent/renumber, child-walks) -> READ, most delicate.
- store_properties.go: 9 sites (define/delete/set prop, child-walks) -> READ.
- store_txn.go coarse commit: 1640/1660/1704/1730/1748 = `tx.store.remember(live)` -> `live =
  tx.store.republishForMutation(live)` (live is local, reloaded per loop, remembered-guarded).

### GATE after each file: go test ./db/store -race + immutability tests. Expand immutability_test
to cover create/chparent/defineprop/setprop/addverb/setverbcode/scalar as I convert each file.
### THEN flip tx.object to alias + reader-survives-concurrent-mutation test; commit ALL
(step-1 routing + republish conversion + flip) together; measure harness; conformance.
### DECISION LOG: doing conversion MYSELF file-by-file (delicate safety edits, direct work
beats orchestration overhead). ~47 sites remain, all the SAME proven transform.

### GATE STATE (immutability_test.go — comprehensive, 1 test/kind). GREEN so far:
Move, SetObjectName. RED remaining (each maps to a file to convert):
CreateObject(store_lifecycle), ChangeParents(store_relationships - JUST converted 291/299/302),
DefineProperty/SetPropertyValue/DeleteDefinedProperty(store_properties),
AddVerb/DeleteVerb/SetVerbCode(store_verbs).

### VERB OP SUBTLETY (store_verbs.go): verb ptr comes from OLD obj. After republishing obj,
must RE-RESOLVE verb via findVerbOnObjectLocked (uses liveObjectLocked -> fresh) else mutate
old node + lose change. AddVerb (new verb) & SetVerbCodeByIndex (indexes fresh obj) need NO
re-resolve. DeleteVerb/SetVerbInfo/SetVerbArgs/SetVerbCode NEED republish + re-resolve.
DONE: AddVerb (356 -> republish). TODO store_verbs: SetVerbCodeByIndex(516 obj-republish),
DeleteVerb(387), SetVerbInfo(434), SetVerbArgs(469), SetVerbCode(495) -- last 4 = republish
s.load(objID)/obj THEN re-resolve verb.
### CHANGEPARENTS caveat: reseedInheritedPropertiesLocked(obj) propagates to DESCENDANTS
(mutates them) -> those republishes live in store_properties.go reseed. My CP test only
checks obj+parents; full -race + property conversion covers descendants.
### GREEN now: Move, SetObjectName, AddVerb, DeleteVerb, SetVerbCode, ChangeParents (store_core,
store_verbs, store_relationships ALL converted). RED remaining: CreateObject(lifecycle),
DefineProperty/SetPropertyValue/DeleteDefinedProperty(properties - converting NOW).

### store_properties.go conversion IN PROGRESS: converted SetPropertyInfo(420),
SetPropertyValue(445), definePropertyLocked(490). TODO: deleteDefinedPropertyLocked(532),
ClearPropertyOverride(560), propagate child(645,648), removeInherited child(683).
*** SPECIAL: ResetInheritedProperties(608-617) mutates obj.properties IN PLACE at 610 BEFORE
remember(615). Must RESTRUCTURE: read-only scan for `changed`, republish BEFORE the delete
loop, then mutate fresh. (Not in immutability_test suite -> full -race is its only gate.)
### store_lifecycle.go (19 sites, CreateObject/Recycle/Renumber + attachChild callers) still TODO
- most delicate; attachChildToParentsLocked mutates parent via s.load (no own remember) so
  EVERY caller must republish parents first. CreateObject test will verify.
### store_txn.go coarse commit (5 sites 1640/1660/1704/1730/1748): live=republishForMutation(live).
### store_properties.go DONE (all 9 + ResetInheritedProperties restructured). GREEN: all
immutability tests EXCEPT CreateObject. store_lifecycle.go IN PROGRESS:
- CreateObject:36 DONE (republish parents). invalidateAnon 151/156 DONE. Recycle:190 DONE.
- TODO Recycle: 216(child) 223(newParent) 233(content) 243(oldLoc) 256(parent) -> all
  `X = s.republishForMutation(X)`. Recreate:306 -> republish s.load(parent).
- Recreate:290 LEAVE AS-IS: remember-then-REPLACE (obj replaced by newObj via publishLocked
  303, never mutated in place) -> already alias-safe.
- Renumber (396+, sites 424/426/474/483/492/501/509/536): NOT YET READ. Read before converting.
### store_txn.go coarse commit (1640/1660/1704/1730/1748): live=tx.store.republishForMutation(live).
### After all GREEN: full db/store -race, then FLIP tx.object->alias + reader-survives test,
then go test ./... + harness measure + conformance. Commit everything together.
### store_lifecycle.go DONE: CreateObject:36, invalidateAnon 151/156, Recycle 190/216/223/233/
243/256, Recreate:306 converted. Renumber: obj.id-in-place -> build fresh `renumbered` clone;
6 `other` loop sites -> republish. LEFT AS-IS (remember-then-REPLACE, safe): Recreate:290,
Renumber recycledTarget:424, Renumber obj:426.
### ANON DECISION: republishForMutation returns `old` unchanged for anon (in-place). Anon are
NOT aliased on read (still deep-cloned) — rare, out-of-band, maybe pointer-referenced. So
coarse commit / mutators mutate anon in place safely. THE FLIP: objectLocked aliases NUMBERED
only, clones anon. This sidesteps anon-pointer-identity risk.
### NEXT: coarse commit store_txn.go 1640/1660/1704/1730/1748: change
`if !live.anonymous && !remembered { rememberObjectLocked(live); remembered=true }` ->
`if !remembered { live = republishForMutation(live); remembered=true }` (drop !anonymous;
republishForMutation returns old for anon). Then build + immutability gate + full db/store -race.
Then FLIP objectLocked (alias numbered, clone anon) + reader-survives test.

## *** REPUBLISH CONVERSION COMPLETE + FLIP APPLIED ***
- ALL ~47 in-place mutation sites converted across store_core/relationships/verbs/properties/
  lifecycle/txn-coarse-commit. Only 3 remember-then-REPLACE sites remain (safe): Recreate:290,
  Renumber recycledTarget:424, Renumber obj:426.
- verbWrites coarse-commit: verb re-fetched from fresh image after republish (was fetched before).
- ANON: republishForMutation returns old unchanged (in-place); coarse commit dropped !anonymous
  guard. Anon still CLONED on read (not aliased).
- GATES PASSED pre-flip: build ./..., immutability gate (all kinds GREEN), full db/store -race,
  FULL go test ./... EXIT=0.
- FLIP DONE (store_txn.go objectLocked): numbered live+history images ALIASED (return ptr,
  no clone); anon still cloneObjectForReadTxn. mutableObject/privatizeCached = copy-on-first-
  staged-write (unchanged, now the sole clone point).
### NEXT (verify the flip): go build; FULL go test ./... ; go test ./db/store ./scheduler -race
  (concurrency tests = real aliasing-vs-mutation stress); add reader-aliases-survives-concurrent-
  coarse-mutation test; then HARNESS MEASURE (expect allocs/op DROP hard esp 64-room, goodput up,
  GC down) recorded vs Phase 0/1 baseline; then managed conformance gate. Then COMMIT everything
  (step-1 txn routing + republish conversion + flip + immutability tests) as Phase 2.
## *** PHASE 2 VERIFIED + MEASURED — BIG WIN ***
- RACE GATE PASSED: go test ./db/store ./scheduler -race GREEN (scheduler 230s concurrency
  stress). Full go test ./... EXIT=0. Flip-safety test + all immutability tests GREEN.
- MEASURED @ GOGC=400 (-count=1): realistic 16/16: goodput 54,401->72,728 (1.34x vs P1),
  allocs 210->100 (2.1x), bytes 36,757->18,574 (2x), GC 30. 32p 48,998->70,552 (1.44x),
  stops regressing below 16p. Cumulative vs P0 baseline: 1.73x goodput, 4.7x fewer GCs.
  Recorded: experiments/2026-07-22-mvcc-phase2-alias-reads.md.
- Churn abort unchanged ~22-27% (Phase 4). move() still coarse (Phase 3).

## CONFORMANCE GATE (owed P1+P2). Linux barn cross-compiled CGO_ENABLED=0 ->
/tmp/barn_phase2_linux + copied to /c/Users/Q/code/barn/barn_phase2_linux (chmod +x in WSL,
runs OK). WSL alive. Conformance repo: /mnt/c/Users/Q/code/moo-conformance-tests.
CMD: wsl --cd /mnt/c/Users/Q/code/moo-conformance-tests --exec bash -lc \
  'uv run moo-conformance --server-command "/mnt/c/Users/Q/code/barn/barn_phase2_linux -db {db} -port {port}" -k "<slice>" -v'
### NEXT: run arithmetic slice (substrate check), then property/verb/move/create/recycle/
chparent families (Phase 2 blast radius), then broader run. If green -> COMMIT Phase 2
(step-1 routing + republish conversion + flip + immutability tests + experiments record).
### CLEANUP: remove barn_phase2_linux from repo dir before/at commit (gitignore or rm; do NOT
commit the 21MB binary). Also /tmp/barn_phase2_linux.

## CONFORMANCE INVESTIGATION (RULE ZERO differential — NOT assuming)
- Phase 2 FULL suite (linux barn_phase2, CGO=0): 11426 passed, 128 skipped, 4 FAILED. All 4 =
  *_dump_persistence family (dump-DB-then-restart): dump_persistence::inherited_override,
  map_dump_persistence::ordering..., string_dump_persistence::raw_bytes..., waif_dump_persistence::
  waif_backreference. FAILURE MODES VARY BY RUN (assertion [1,42,1]vs[0,7,1] in full; BrokenPipe
  isolated) = the KNOWN suite non-isolation/nondeterminism (ci-conformance notes).
- DIFFERENTIAL (isolated, same 4 -k, SAME on both barns):
  - Phase 1 baseline (992a528, linux CGO=0): 3 fail (map/string/waif BrokenPipe) + 1 PASS (inherited_override).
  - Phase 2 (linux CGO=0): IDENTICAL 3 fail + 1 pass. => Phase 2 == baseline, NO REGRESSION.
- CONTEXT: master's recorded GREEN gate (investigations/mvcc-conformance-timeout.md:171) =
  "11,412 passed, 0 failed" but on WINDOWS barn.exe. My linux CGO=0 build differs (dump-persistence
  fails on linux CGO=0 on BOTH baseline+phase2). => the 4 failures are a LINUX-BUILD/PLATFORM
  artifact PRE-EXISTING on baseline, not Phase 2.
- Windows conformance BLOCKED: `uv run` in conformance dir fails on .venv/lib64 linux symlink
  (Access denied) — documented issue (mvcc-conformance-timeout.md:40).
- PENDING: baseline linux FULL run (bg bc1mf3bex) to confirm baseline full-run failure set ==
  Phase 2 full-run set (4 dump-persistence). If match -> definitively no Phase 2 conformance
  regression -> COMMIT Phase 2 with honest conformance note (neutral vs baseline; 4 failures are
  pre-existing linux-CGO0 artifact absent from the Windows green gate).
- OPTIONAL bonus: get Windows green (fix .venv or build CGO=1 linux barn in WSL w/ gcc so
  dump-persistence works). Try if quick; not the gate.

### ARTIFACTS TO CLEAN before commit: barn_phase2_linux, barn_p1base_linux, barn_phase2.exe (repo
dir, DO NOT COMMIT). git worktree /c/Users/Q/code/_p2base (remove). Stage ONLY: 6 store *.go +
immutability_test.go + experiments/2026-07-22-mvcc-phase2-alias-reads.md. Commit msg ready at
scratchpad/phase2-commit-msg.txt.

## *** PHASE 2 COMMITTED: 1e4e6cf *** (alias immutable reads + republish COW)
CONFORMANCE DIFFERENTIAL DEFINITIVE: baseline(P1) full run == Phase 2 full run, byte-identical:
both 11,426 passed / 128 skipped / 4 failed, SAME 4 dump_persistence tests, same value
[1,42,1]vs[0,7,1]. => ZERO Phase 2 conformance regression. The 4 failures are a pre-existing
CGO-free-linux-build artifact (master's green gate 11,412/0 was Windows barn.exe). Commit msg
states this honestly. Artifacts cleaned (binaries, worktree _p2base removed). Build green, tree clean.

## SESSION SCORECARD (all committed on branch mvcc-concurrency-redesign):
- P0 2d4ef1a: harness+baseline.  P1 604dd09/2609bd8/992a528: floor-shard+GOGC=400 (~1.6x).
- P2 1e4e6cf: alias reads. MEASURED cumulative vs P0 baseline: 1.73x goodput, 4.7x fewer GCs,
  ~half memory/command. allocs/op 210->100, bytes 36757->18574.

## PHASE 3 NEXT: take move()/create()/recycle()/chparent off the stop-the-world path.
KEY: Phase 2's republishForMutation ALREADY made these publish-new-images (not in-place). The
REMAINING P3 work = remove the EXCLUSIVE store.mu.Lock / markLiveStoreMutated coarse routing,
so move() commits through the decentralized MVCC path (relationshipVersion + buildImageWith
Relationship already exist). Two disjoint-room moves must commit in parallel; same-room conflict
+ retry. Order: 3a move (222 verbs, dominant write), 3b create/recycle (versioned obj directory),
3c chparent/add_verb. Conformance risk: enterfunc/exitfunc mid-builtin ordering -> Rule Zero vs Toast.
Gate each 3x slice: full conformance + harness (realistic goodput must SCALE past 16p toward 32p;
32p rows stop regressing below 16p peak). START: investigate why move() marks live mutation
(builtins/objects_movement.go:70 markLiveStoreMutated) + what blocks routing it decentralized.

## PHASE 3a IN PROGRESS: move() through decentralized MVCC. Q said GO (2026-07-22).
### DESIGN (from reading commit internals; scout ae8422b9 confirming move semantics):
- .location ALREADY decentralized (SetObjectLocationRaw -> relationshipWrites -> 
  buildImageWithRelationship store_cow.go:103). .contents NOT staged (coarse store.MoveObject only).
- PLAN: extend objectRelationshipWrite (store_txn.go:81) with contentsSet bool + contents []ObjID.
  buildImageWithRelationship (store_cow.go:103): `if w.contentsSet { img.contents = w.contents }`.
  Contents rides EXISTING relationshipWrites plumbing: commitDecentralized footprint/slot-lock/
  validate/build already cover relationshipWrites objects (store_cow.go:326,506).
- CONFLICT: contents write bumps relationshipVersion (buildImageWithRelationship stamps ts);
  validateObjectRelationshipReadsLocked (store_txn.go:1779) checks it. So reader of R.contents
  (markObjectRelationshipRead) conflicts w/ concurrent move writing R.contents. Disjoint rooms parallel.
- NEW tx method tx.MoveObject(what,where,pos): reads what.loc(oldLoc)+oldLoc.contents+where.contents
  (mark rel reads), stages via mutableObject (read-your-writes) + relationshipWrites: what.location=
  where, oldLoc.contents=remove(what), where.contents=insert(what,pos). Mirrors SetObjectLocationRaw.
- builtinMove: replace store.MoveObject + markLiveStoreMutated + AdoptLiveRelationships with
  tx.MoveObject (staging). KEEP accept verb call + HasContentDescendant recursive check (make
  txn-based). enterfunc/exitfunc = TODO in code (Phase 9) - confirm not required by conformance.
### GATE 3a: full conformance (move/accept/contents/location/enterfunc families) + harness
(realistic goodput must SCALE past 16p toward 32p; disjoint moves parallel). TDD: red test =
two disjoint-room moves commit without coarse serialization + conflict test = two same-room moves.
### RISK: mid-builtin side-effect ordering (accept/enterfunc/exitfunc) vs Toast — Rule Zero.
Also: does move-to-nowhere (#-1) / move-from-nowhere work through staging? recursive-move check.
### PLUMBING DONE (additive, design-independent, no behavior change yet — nothing stages
contents yet): objectRelationshipWrite +contentsSet/+contents (store_txn.go:81);
buildImageWithRelationship applies contents (store_cow.go:103, decentralized path);
coarse commit relationshipWrites loop applies contents (store_txn.go:1679-1684). NOT yet built.
### AWAITING scout notes/mvcc-phase3-move-survey.md before cutting tx.MoveObject + builtinMove
(conformance risk = accept/enterfunc/exitfunc ordering, contents position semantics).
### readTxn(ctx) always present during task exec. builtinMove -> replace store.MoveObject +
markLiveStoreMutated + AdoptLiveRelationships with tx.MoveObject(what,where,pos). Keep accept
verb + HasContentDescendant (make tx-based). insertObjIDAtMOOPosition/removeObjID = contents ops
(store_relationships.go) to replicate in tx.MoveObject.
## SCOUT (notes/mvcc-phase3-move-survey.md) VALIDATED DESIGN + KEY DE-RISK:
- ONLY 1 runtime caller of store.MoveObject = builtinMove (objects_movement.go:67).
- enterfunc/exitfunc = UNIMPLEMENTED TODO, ZERO conformance coverage -> NOT needed. So only
  mid-builtin side effect is `accept` (BEFORE move) = low risk.
- contents stored explicitly; shared relationshipVersion is sufficient conflict substrate.

## PHASE 3a IMPLEMENTED (uncommitted):
- store_txn.go: tx.MoveObject(what,where,pos) [reads what.loc+oldLoc.contents+where.contents w/
  markObjectRelationshipRead; stages via mutableObject + relationshipWrites location+contents];
  tx.HasContentDescendant (tx-based recmove, records deps); tx.stageContentsWrite.
- objectRelationshipWrite +contentsSet/contents; buildImageWithRelationship + coarse loop apply contents.
- builtins/objects_movement.go: builtinMove now uses tx.MoveObject (decentralized) + tx.HasContentDescendant,
  DROPPED markLiveStoreMutated + AdoptLiveRelationships (store.MoveObject only as no-txn fallback).
- TESTS GREEN (db/store/move_mvcc_test.go): DecentralizedMaintainsContents, DisjointRoomsCommitIn
  Parallel (no false conflict), SameRoomConflicts. Asserts move NOT liveMutated (not coarse).
- go build OK; go test ./db/store ./builtins ./scheduler GREEN.

### NEXT (Phase 3a verification before commit):
1. HARNESS @ GOGC=400 -count=1: realistic goodput must now SCALE past 16p toward 32p (Phase 2
   was flat 16p~=32p b/c move was coarse). Compare to Phase 2 (72.7k@16p, 70.5k@32p).
2. go test ./db/store ./scheduler -race (move concurrency race-clean).
3. CONFORMANCE (RULE ZERO): move.yaml (positional contents order), move_call_shapes, move_invalid_args,
   accept-on-move+wizard override, E_RECMOVE. Differential vs Phase 2 baseline (1e4e6cf) — must be
   SAME failure set (only the 4 pre-existing dump_persistence). Build linux barn_p3, run full suite.
4. If all green -> commit Phase 3a. Then 3b create/recycle, 3c chparent/add_verb.
### WATCH: move-to-#-1 (ObjNothing) path; anon-object move (routes coarse via writeFootprintHasAnon?);
recmove tx-based correctness.

## PHASE 3a MEASURED — WIN + surfaced hotspot (experiments/2026-07-22-mvcc-phase3a-move-decentralized.md):
- realistic 16/16: 72,728->102,055 (1.40x). churn 16/16: 87k->151k (1.74x). Cumulative vs P0: 2.43x.
- ABORT ROSE 0%->44% (realistic 16/16), but it's PURE room.contents contention: gradient 16r 44.5%
  -> 64r 28.7% -> 256r 14.9% (Zipfian hotspot, NOT a bug). Old coarse path hid it via global block.
- => This is the Phase 5 hot-cell symptom: move's contents edits are setadd/setremove (COMMUTATIVE).
  Commutative-merge contents write would collapse the abort. NEXT after 3a lands.
- Per SHIP-THE-WIN-FIRST: commit 3a (verified win) THEN do commutative-contents.

## GATES RUNNING (both bg): race bd0030tn4 (db/store+scheduler); conformance biu1i2f4u (linux
barn_p3, differential vs P2 baseline = expect SAME 4 dump_persistence pre-existing failures).
### ON BOTH GREEN: commit Phase 3a (store_txn.go + store_cow.go + objects_movement.go +
move_mvcc_test.go + experiments record). Clean barn_p3_linux artifact. Then commutative-contents.
### DESIGN SEED (commutative contents, next): stage contents as DELTAS (add X / remove Y) not
whole-list; apply to LIVE contents at commit (not read snapshot); relax conflict detection so two
commutative contents-deltas on same room DON'T conflict (only a whole-list read/replace does).
Subtle: readers of room.contents still need a dep, but a commutative appender shouldn't conflict
another commutative appender. Design carefully next phase.

## *** PHASE 3a HAS REAL CONFORMANCE REGRESSIONS — DO NOT COMMIT ***
Race GREEN. Conformance: 9 failed vs P2 baseline 4. Isolated differential (RULE ZERO):
- locations_basic_chain, locations_with_base_object, locations_with_parent_check,
  renumber_multiple_inheritance: PASS on P2-baseline barn, FAIL on P3a barn = REAL P3a REGRESSIONS.
- audit_yin_yields (task_scheduling): flaky (passed isolated). dump_persistence x4: pre-existing.
ROOT CAUSE (from renumber test [1,1,1,0]): `move(d,a)` then `renumber(d)` -> renumber reads
d.location from LIVE store, but decentralized move only STAGED it in txn -> coarse builtin sees
stale live state. Under P2 coarse move mutated live immediately so coarse ops saw it.
SECOND FACET (suspected): objects CREATED within task have version > readTS -> tx.object() 
objectLocked version check (store_txn.go:219 objectVersion(live)<=readTS) may not see them unless
create() ADOPTS them into tx cache. My move_mvcc_test PASSED because it created objs in SEPARATE
committed txns (version <= readTS). Conformance creates INSIDE the task.
### FIX DIRECTION (candidate): move() should use COARSE store.MoveObject when tx.liveMutated is
already set (a coarse op like create/recycle ran before it) — then live is mutated, coarse ops +
txn-aware reads both see it. Handles create-before-move (the failing tests). BUT move-BEFORE-coarse
(move then renumber) still stale -> need coarse ops to FLUSH staged rel writes, OR broader rule.
Investigate exact mechanism via repro FIRST (scheduler/repro_move_test.go - build failed, fixing).
### Artifacts: barn_p3_linux, barn_p2base_linux, worktree _p2base (clean up later). barn_phase2.exe? gone.
### ROOT CAUSE CONFIRMED via repro (scheduler/repro_move_test.go): create(x3)+move+move+
locations OK alone, but adding recycle(x3) -> E_INVIND. Mechanism: staged decentralized move +
coarse builtin (recycle/renumber) reading LIVE store = stale live within one task.
### FIX APPLIED (builtins/objects_movement.go): `decentralized := tx != nil && !ctx.LiveStoreMutated`.
Pure-move tasks (harness hot path) -> decentralized. Tasks that already live-mutated (create/
recycle/renumber before move) -> COARSE move (store.MoveObject + markLiveStoreMutated + adopt).
recmove check matches (tx-based if decentralized, store-based if coarse).
### VERIFIED: repro now clean (locations={b,a}); 4 regressed conformance tests PASS isolated;
move_mvcc tests still green (decentralized preserved); regression test added
(TestMoveMixedWithCoarseBuiltinsStaysConsistent).
### GAP (watch): move-BEFORE-coarse in one task (move then recycle, NO prior coarse) would still
stage-then-read-stale. Full conformance run (bg b65m03kv3) will reveal if any such test exists.
If it fails -> need coarse builtins to FLUSH staged rel writes before reading live (bigger fix).
### *** PHASE 3a COMMITTED: 668cfbb *** move() decentralized + regression fix.
- Full conformance CLEAN: 11,426 passed, SAME 4 pre-existing dump_persistence, ZERO new
  (regression fixed; no move-before-coarse test exists). -race GREEN (db/store+scheduler+builtins).
  Harness win preserved (16/16 ~113k realistic, ~138k churn). Artifacts cleaned. build ./... OK.

## SESSION SCORECARD (branch mvcc-concurrency-redesign, all committed):
P0 2d4ef1a harness | P1 604dd09/2609bd8 floor+GOGC | P2 1e4e6cf alias-reads | P3a 668cfbb move.
CUMULATIVE vs P0 baseline (realistic 16/16 @ GOGC=400): 41,920 -> 102,055 = 2.43x goodput.

## NEXT — highest value = COMMUTATIVE CONTENTS (Phase 5 hot-cell), NOT 3b/3c.
Rationale: Phase 3a's residual ceiling is the room.contents abort storm (44% @16r). That's the
DOMINANT realistic-mix cost now. move's contents edits = setadd/setremove = COMMUTATIVE. Fixing
it recovers the goodput lost to retries (102k could go much higher). 3b/3c (create/recycle/chparent)
target build=3% of mix (minor). Phase 4 targets CHURN abort (ancestor writes), not realistic move abort.
### COMMUTATIVE CONTENTS DESIGN (design carefully, correctness-sensitive):
- Stage contents as DELTAS (add whatID at pos / remove whatID) not whole-list.
- Commit applies deltas to CURRENT LIVE contents (removeObjID/insert on live), NOT read-snapshot.
- Two moves into same room: both stage "add X"/"add Y" deltas -> both apply to live -> no conflict,
  serialize on slot.mu only (cheap). A delta-move records NO whole-list read dep on the room ->
  no version conflict between commutative appenders.
- SUBTLE: recmove (HasContentDescendant) reads contents -> records deps (but trivial for player-into-
  room: player has empty contents). look reads contents (read-only, skips commit -> no conflict).
- RISK: relaxing conflict detection for commutative ops is correctness-sensitive. TDD: two same-room
  moves BOTH commit (no conflict, both objects in room); ordering; conformance (move positional).
### ALTERNATIVE order if Q prefers plan-literal: 3b create/recycle, 3c chparent/add_verb, then 4, 5.

## COMMUTATIVE CONTENTS IMPLEMENTED (uncommitted) — Q said GO. MASSIVE WIN.
- objectRelationshipWrite: contentsSet/contents (whole-list) -> contentsDeltas []contentsDelta
  {add,id,position}. applyContentsDeltas(contents,deltas) applies in order (removeObjID/insert,
  both return fresh slices). buildImageWithRelationship + coarse loop apply deltas to CURRENT
  live contents (not snapshot). tx.MoveObject stages deltas + NO room read dep (only reads what.location
  -> dep on what). stageContentsDelta replaces stageContentsWrite.
- WHY correct/0-abort: reads skip commit; different-object moves share no read dep; same-room moves
  commute (no room dep) -> both commit, serialize on slot.mu only. Two moves of SAME object conflict
  (read dep on what). Read-then-write of a room's contents still conflicts (room relationshipVersion
  still bumps). Double-add impossible (moving same obj conflicts on what.location).
- TESTS GREEN: TxnMoveDecentralizedMaintainsContents, DisjointRoomsCommitInParallel,
  SameRoomCommutes (NEW: both commit, both objs in room - was Conflicts). MixedWithCoarse regression.
- MEASURED @ GOGC=400: realistic 16/16 abort 44%->0%, goodput 102k->127k. 32/16 0% abort. CUMULATIVE
  vs P0 baseline: 41,920 -> 126,977 = 3.03x, 0% realistic abort. Churn abort 41%->22% (residual =
  ancestor-write = Phase 4 target, NOT move).

## GATES RUNNING (bg): conformance bwkagcvhn (barn_commut_linux, expect same 4 dump_persistence);
race bnvxh5vke (db/store+scheduler+builtins). ON BOTH GREEN: commit commutative contents (store_txn.go
+ store_cow.go + move_mvcc_test.go). Clean barn_commut_linux. Then report milestone (3.03x cumulative).
### *** COMMUTATIVE CONTENTS COMMITTED: a2bcc15 ***
Conformance CLEAN (11,426 passed, same 4 pre-existing dump_persistence, 0 new). -race GREEN.
build OK, tree clean, barn_commut_linux removed.

## ===== SESSION SCORECARD (branch mvcc-concurrency-redesign, 6 commits) =====
P0 2d4ef1a harness | P1 604dd09/2609bd8 floor+GOGC | P2 1e4e6cf alias-reads |
P3a 668cfbb move-decentralized | a2bcc15 commutative-contents.
*** CUMULATIVE realistic 16p/16r @ GOGC=400: 41,920 -> 126,977 = 3.03x goodput, 0% abort. ***
Arc: GC wall (P2) -> global move lock (P3a) -> abort storm (commutative), each removed+measured,
conformance clean at every step. The plan's "massive real-workload win" (absolute goodput +
abort) is DELIVERED for the realistic mix: 3.03x, 0% abort.

## REMAINING (realistic mix is at 0% abort — these target edge cases / scaling):
- 32p realistic (104k) still < 16p (127k): slot-mutex serialization on hottest rooms + coarse
  build(create/recycle) path. Phase 3b (create/recycle decentralized) + versioned obj directory.
- Phase 4 (shapeVersion split): helps CHURN/ancestor-write workloads (churn-stress residual 22%).
  Realistic mix has churn=0 (stamp is disjoint per-player) so P4 doesn't move realistic. Matters
  for real cores with hot-ancestor writes ($local connect lists, per workload characterization).
- Phase 3c chparent/add_verb (rare).
## DECISION POINT for Q: core goal delivered (3.03x, 0% abort). Options: (a) continue 3b (32p
scaling / create-recycle), (b) Phase 4 (ancestor abort), (c) merge branch to master (6 verified
commits), (d) stop. Report milestone + let Q steer.
### NOTE: managed conformance is via CGO-free linux cross-compile (4 pre-existing dump_persistence
fail on it AND on baseline; absent from master's Windows barn.exe green gate). Consistent across
all phases — every phase differential-verified identical to baseline.

## MERGED TO MASTER: master @ a2bcc15 (fast-forward, 7 commits). origin NOT pushed (Q's call).
Branch mvcc-phase3b created for continued work.

## PHASE 3b SCOPING (create/recycle decentralized) — MEASURED + architectural fork found:
### DIAGNOSTIC (decisive): realistic 32p/16r with build=0 (no coarse create/recycle) = 190,919
vs build=3 = 104,909 = 1.82x. And 32p>16p (scaling restored). So 3% coarse create/recycle ops
CAP 32-core scaling. 3b is strongly justified.
### STRUCTURAL FACTS: only s.objects skeleton mutation = slotFor (store_core.go:128 s.objects[id]=slot).
recycle does NOT change skeleton (keeps slot, publishes recycled image) -> recycle CAN decentralize
like move. CREATE is the hard one (new slot insert needs exclusive vs readers' RLock).
### THE FORK: decentralized create needs a CONCURRENT object directory (slot insert must not block
readers). Options + tradeoffs (READ HOT PATH = s.load->s.objects[id] on EVERY read, 70% of mix):
  (a) sync.Map: concurrent insert+read, but per-read overhead (interface boxing) -> may slow reads.
  (b) sharded RWMutex map: per-read shard RLock (sharded=low contention), insert locks 1 shard.
  (c) segmented append-only array indexed by id (ids sequential): O(1) lockless indexed read + O(1)
      concurrent append, NO copy. IDEAL for append-only sequential-id directory. Biggest refactor.
  (d) short exclusive lock just for slot insert: simplest but still briefly blocks readers.
  24 s.objects sites (12 range, 5 lookup, 3 len, misc) — all need updating for a/b/c.
### ALSO: create allocates id mid-task (returns it) = irreversible side effect -> today sets
liveMutated (no retry). Decentralized create needs id-alloc atomic + slot published at commit; on
retry, old id wasted (no orphan slot if slot inserted only at commit). Retry-safety = design care.
### RECOMMENDATION: (c) segmented array — keeps reads lockless/fast (critical for 70%-read workload),
ideal for append-only sequential ids, biggest upside. But biggest refactor. Report fork to Q.
### RECYCLE-alone is easy (no directory change) but doesn't help build task (create still coarse ->
liveMutated). So must do create too. Recycle-decentralized helps recycle-only + Phase 3c chparent.

## Q APPROVED OPTION C (segmented lock-free directory). ON BRANCH mvcc-phase3b.
### SLICE 1 (directory swap, behavior-preserving) IN PROGRESS:
- db/store/object_dir.go WRITTEN: objectDir{segments atomic.Pointer[[]*dirSegment], growMu, count}.
  dirSegment{slots [4096]atomic.Pointer[objectSlot]}. Methods: slot(id) lockless read, getOrCreate
  (CAS), ensureSegment (COW grow under growMu), forEach(id,slot), len(). segShift=12.
- Store.objects map -> Store.dir objectDir. slotFor -> s.dir.getOrCreate. load -> s.dir.slot.
- CONVERTED range->forEach: store_core.go ObjectsByName/OwnedBy (585,609). store_lifecycle All(330).
- BUG TO FIX NOW: store_lifecycle Players() (346) — I wrote `s.dir.forEachSlot(` which DOESN'T EXIST
  (only forEach(id,slot) defined) + left the for-loop body closing mismatched -> syntax error line 354.
  FIX: use forEach(func(_ ObjID, slot *objectSlot) bool{...}), convert continue->return true, fix closing.
### CONVERSION PROGRESS (map->dir): DONE store_core (load/slotFor/2 ranges), store_lifecycle
(All/Players/Renumber-loop), store_metrics(133), store_reachability(51 rangeAnon). 
### REMAINING: store_reachability 67(PersistentAnonReach - continue x2, nested prop loop, queue append),
217; store_cow 391(lookup s.objects[id]->s.dir.slot),536(?); store_snapshot 45,46,66,76,141;
cow_disjoint_race_test 40(TEST - store.objects). Pattern: `for _,slot:=range s.objects{...}` ->
`s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {...; return true})`; continue->return true.
s.objects[id] lookup -> s.dir.slot(id). len(s.objects)->s.dir.len().
### CONVERSION nearly done: store_core, store_lifecycle, store_metrics, store_reachability,
store_cow(391 slot,536 publishLocked), store_snapshot(45,46 len; 66,77 forEach) ALL DONE.
### LAST 2: store_snapshot 143; cow_disjoint_race_test.go:40 (store.objects - TEST white-box,
may need s.dir or a test helper). Then go build ./..., go test ./db/store -race.
### SLICE 1 COMMITTED: a77af5b (segmented dir). Full suite green, -race green, harness read-neutral
(actually slightly faster: 16p 127k->140k, 32p 105k->126k — noise/faster reads). NOT the win yet.

## SLICE 2 (decentralize create) — DESIGN, next chunk (commit-path surgery, do CAREFULLY):
GOAL: create() commits decentralized (no liveMutated) using the lock-free dir. Target: 32p realistic
104k->~190k (diagnostic).
### CHANGES NEEDED:
1. highWaterID atomic (concurrent creates allocate distinct ids). Currently plain field under s.mu.Lock.
2. tx.CreateObject(parents, owner) (newID, EC): atomic-alloc newID; build *Object (NewObject + parents +
   inherited props via TXN reads copyInheritedProperties); tx.objects[newID]=obj (owned); record in
   tx.createdObjects[newID]; stage parent.children += newID (NEW childrenDeltas on relationshipWrites,
   parallel to contentsDeltas); read-your-writes mutableObject(parent).children append; stamp obj. Return newID.
3. objectRelationshipWrite: add childrenDeltas []contentsDelta. buildImageWithRelationship + coarse loop apply.
4. commitDecentralized: created ids in write footprint. Slot-lock loop (store_cow.go:389): for CREATED ids,
   getOrCreate slot (not require-exists). Build+publish new obj image. Update maxObjID/highWaterID (atomic).
   Add createdObjects footprint to writeIDs.
5. builtinCreate: decentralized when tx!=nil && !ctx.LiveStoreMutated (mirror move fix); else coarse store.CreateObject.
6. RETRY: create re-runs -> new id, old wasted (no orphan slot: slot only at commit). OK.
7. ANON create: keep COARSE (rare, out-of-band). Only numbered create decentralized.
### RECYCLE (slice 3): more complex (children reparent + contents clear + location.contents - + parents.children -).
Uses childrenDeltas + contentsDeltas + location writes. But build task only needs CREATE decentralized
(create sets liveMutated today -> recycle in same task also coarse; if create decentralized, recycle after
it... recycle sets liveMutated -> whole commit coarse! So MUST decentralize recycle too, else build task
still coarse via recycle). => build = create+recycle, BOTH must decentralize. Slice 2 = create, slice 3 = recycle.
### TDD: create in txn -> commit decentralized (not liveMutated); new obj exists post-commit with right parents;
parent.children has newID; two concurrent creates -> distinct ids, both commit. conformance (create/recycle
families). Measure 32p.
### STATE: master @ a2bcc15 (3.03x, merged). branch mvcc-phase3b @ a77af5b (slice1). Slice 2 next.

## FABLE ADVERSARIAL REVIEW (verify claims before acting; full notes in scratchpad/
create-recycle-review-notes.md). Design directionally right; found real bugs:
P0-1 created-obj base image DOUBLE-APPLIED: don't publish tx-owned cache; store PRISTINE base
  image in createdObjects, commit clones + applies write maps (same kind order as everyone).
P0-2 staged creates DROPPED on coarse fallback (writeFootprintHasAnon / later liveMutated) + on
  empty-commit guard (store_txn.go:1684 all-maps-empty early return + HasWrites :510). MUST teach
  Commit guards + HasWrites + COARSE apply branch about createdObjects (precedent: coarse already
  applies contentsDeltas :1793).
P0-3 id uniqueness: atomic alone NOT enough. Unified allocateID()=highWaterID.Add(1) for coarse+
  anon+decentralized. commit must verify slot.ptr==nil (occupancy from ResetMaxObject lowering
  highWater / Renumber-into-gap) -> else validationFail conflict. 
P0-4 spurious E_INVIND: validators liveObjectLocked(newID)->nil->E_INVIND. EXEMPT createdObjects ids
  in ALL validators + slot-nil write check (store_cow:393) + liveness loop (:427).
P0-5 recycle side effects (RecyclePlayer boot, verb-cache) fire at STAGE time -> defer to post-commit
  OR set IrreversibleSideEffect for player recycle (rare, ok).
P0-6 s.recycledID append RACE under RLock -> own mutex.
P0-7 maxObjID/highWaterID -> atomics w/ CAS-max; ALL readers convert (Valid/NextID/MaxObject/
  LowestFreeID/BeginReadOnly:182/insertObjectLocked/ResetMaxObject). RLock-read vs atomic-write = race.
P1-1 recycle: do NOT ForgetObject (builtins/objects.go:434) — the tx.Parents/Children/Contents/Location
  reads ARE the conflict guard vs concurrent move-into/create-under dying obj.
P1-2 childrenDeltas = SETADD-idempotent (diamond reparent dup). child.parents = whole-list write +
  read (non-commutative, must conflict).
P1-3 per-content/child staging MUST record reads (use SetObjectLocationRaw which marks read).
P1-4 widen retry gate: E_INVIND with validationFail should RETRY (task_runtime.go:245 requires E_INVARG).
  create-under-P vs recycle-P -> E_INVIND today not retried -> raw error no serial order produces.
P1-5 max_object() read-your-writes: route builtinMaxObject through tx.MaxObject() (tx.maxObjID exists).
P1-6 anon create CANNOT decentralize: gate = tx!=nil && !liveMutated && !anonymous.
P1-7 create-then-recycle same id in one txn: publish tombstone; test (the build-task shape).
Q4 HISTORY/GC = SOUND (rememberOldImageLocked + publish + floor pins pre-recycle image for old readers).

### DE-RISKED ORDERING (Fable): 1) atomics + unified allocateID (standalone, race-verifiable, 0 behav
change). 2) retryable validation-E_INVIND. 3) tx.CreateObject (pristine base + exemptions + all guards
+ coarse branch + slot-empty check + CAS-max + tx.MaxObject). 4) flip builtinCreate (non-anon gate).
5) tx.RecycleObject (recycleWrites kind + buildImageRecycled + no ForgetObject + read-recording +
recycledID mutex + deferred RecyclePlayer). 6) flip builtinRecycle.
### STEP 1 (atomics + unified allocateID) IN PROGRESS:
DONE: struct maxObjID/highWaterID -> atomic.Int64. Helpers added (store_core.go): maxObjectID(),
highWater(), allocateID()=highWaterID.Add(1), casMaxID(a,v) CAS-max. NewStore inits Store(-1).
insertObjectLocked -> casMaxID. CreateObject uses allocateID (anon redundant-bump removed).
NextID/MaxObject -> helpers.
TODO (compile errors remaining): store_lifecycle Valid(86 id>s.highWaterID->s.highWater()),
LowestFreeID(366 id<=s.maxObjID->s.maxObjectID(); 377 s.maxObjID+1->s.maxObjectID()+1);
store_metrics ResetMaxObject(147,148 -> .Store(int64(...))); store_snapshot(44 ->maxObjectID(),
187 ->maxObjectID()+1); store_txn BeginReadOnly(182,183 ->maxObjectID()/highWater()).
Then go build + go test ./db/store -race (behavior-preserving; all id allocation still coarse-serialized
so no new race yet). NOTE Fable P0-3: ResetMaxObject LOWERS highWater -> races decentralized alloc;
defer to step 3 (create still coarse now). commit step1.
### STEP 1 DONE + VERIFIED: all maxObjID/highWaterID -> atomic.Int64. helpers maxObjectID/highWater/
allocateID/casMaxID. All ~18 sites converted. build ./... OK. go test ./db/store + -race GREEN.
Behavior-preserving (create still coarse -> all alloc serialized under s.mu.Lock, no new race).
DEFERRED to step 3: Fable P0-3 ResetMaxObject lowers highWater (races decentralized alloc) — only
matters once create decentralizes; ResetMaxObject + decentralized alloc both under proper sync then.
### NEXT: run full go test ./..., commit step 1 (foundation), THEN steps 2-6 (Fable ordering) =
the actual create+recycle surgery (7 P0s). Steps 2-6 = intricate commit-path work, do CAREFULLY.
### Full plan in notes above + scratchpad/create-recycle-review-notes.md (Fable's detailed traces).
### PACING: slice1(dir a77af5b) + step1(atomics c61ec93) committed. Q said KEEP GOING carefully.
## STEP 2 DONE (uncommitted): retry gate widened task_runtime.go:245 -> (E_INVARG||E_INVIND) &&
ValidationFailed(). Verified: validationFail set ONLY by read validators (store_cow 407-419, store_txn
1740-1752), so E_INVIND+validationFail = conflict (retryable), not execution-time E_INVIND. build+tests green.
## STEP 3 (tx.CreateObject) IN PROGRESS — sub-pieces:
- PIECE 1 DONE: childrenDeltas []contentsDelta on objectRelationshipWrite (SETADD-idempotent via
  applyChildrenDeltas + slices.Contains). buildImageWithRelationship + coarse loop apply. build OK.
- PIECE 2 DONE: tx.createdObjects field added; HasWrites(:544) + Commit empty guard(:1718) include it.
  TODO still: stageChildrenDelta helper. Then release/ForgetObject/reset paths must clear createdObjects
  (retry discards it) — CHECK where write maps get niled at commit end + retry reset.
- PIECE 3 DONE: tx.CreateObject + stageChildrenDelta + tx.MaxObject added. build OK.
- PIECE 4 DONE (commitDecentralized): footprint += createdObjects; lock loop getOrCreate for created;
  empty-slot conflict check (P0-3); liveness loop exempt created(:450); build from clone(base)+stampAll
  +writemaps, casMaxID, no rememberOldImage; createdObjects niled at end.
  *** STILL TODO PIECE 4: EXEMPT created ids in the READ VALIDATORS (validateObjectScalarReadsLocked
  etc. in store_txn.go ~1898-1990) — task reads own new obj -> read on created id -> liveObjectLocked
  nil -> E_INVIND. Must skip createdObjects ids in all 5 validators (scalar/rel/prop/propscan/verb).
- PIECE 5 TODO: COARSE Commit branch apply createdObjects (publish) — Fable P0-2. Coarse branch is
  store_txn.go ~1736+ exclusive apply. Must publish createdObjects there too (+ childrenDeltas already
  applied via my earlier coarse-loop edit).
- PIECE 6 TODO: builtinCreate flip (tx!=nil && !liveMutated && !anonymous); builtinMaxObject via tx.MaxObject.
### PIECE 4 DONE: all 6 read validators exempt createdObjects ids (store_txn.go 2001-2071).
### PIECE 5 DONE: coarse Commit branch publishes createdObjects FIRST (after bumpClock, before write
loops) so self-writes+childrenDeltas find them live; casMaxID; createdObjects niled at coarse end too.
### PIECE 6 TODO: builtinCreate flip (tx!=nil && !liveMutated && !anonymous -> tx.CreateObject; else coarse
store.CreateObject+markLiveStoreMutated+adopt). builtinMaxObject -> tx.MaxObject() when tx present.
### NEXT: go build; TDD store-level test (tx.CreateObject commits decentralized, obj exists w/ parents,
parent.children has newID, two concurrent creates distinct ids both commit); then builtinCreate flip +
scheduler test (create;move;recycle in one task stays consistent — reuse the mixed-builtin regression);
build+ -race+ full suite + conformance differential. THEN step 5 recycle.
### KEY DESIGN: created obj = pristine base in createdObjects; commit builds clone(base)+stampAll(ts)+
self-write-maps; parents get childrenDelta setadd; NO read dep on parent for child (commutes);
copyInheritedProperties records ancestor propertyScan deps (conflict footprint). Retry drops txn (fresh id).

## *** HIT A REAL COUPLING: decentralized create BREAKS mixed tasks ***
Flipping builtinCreate -> decentralized broke create;...;recycle (mixed test E_INVARG): create stages,
recycle is STILL COARSE and reads STALE LIVE (created obj not published). Was masked before because
create-coarse made whole task coarse. UNMASKED now. Same for create;renumber (renumber coarse).
=> ACTIVATING decentralized create needs: (a) recycle decentralized (step 5) so create;recycle fully
decentralized; (b) a FLUSH of staged writes to live before ANY still-coarse builtin (renumber/chparent/
add_verb) reads live mid-task. Also affects move;renumber (was masked by create-coarse too).
### DECISION (ZERO SUM — don't rush commit-path flush+recycle at marathon tail): REVERTED the two
builtin FLIPS (builtinCreate, builtinMaxObject) -> tree consistent, create still coarse. KEPT the
verified machinery (tx.CreateObject, commit-path created-obj handling, atomics, retry-gate). The
machinery is DORMANT when no builtin stages a create (createdObjects always empty -> all the new
`if createdObjects[id]!=nil` checks false, loops empty) -> safe, tests pass, unit-tested by create_mvcc_test.
### NEXT UNIT (careful, fresh): the FLUSH (factor coarse-commit apply into applyStagedWritesLocked +
tx.flushStagedToLive called at start of remaining coarse builtins) + step 5 recycle decentralize +
flip create+recycle together. Then mixed test + conformance (locations_basic_chain, renumber_multiple_
inheritance) must pass. THEN measure (32p ->~190k).
### COMMITTABLE NOW: atomics(done), retry-gate(done), dir(committed), create machinery(dormant+unit-tested).

## *** CROSSING THE FINISH LINE (Q: "Let's cross the finish line!") ***
### THE FLUSH DONE (store side): factored coarse-commit apply -> applyStagedToLiveLocked() (Commit
validates then calls it; store_txn.go:1865). FlushStagedToLive() (EXPORTED) = Lock+apply+Unlock, then
DROP read maps (task becomes non-isolated, no stale-read conflict at coarse commit). builtins/store_reads.go
flushStagedBeforeCoarse(ctx) = if HasWrites: FlushStagedToLive + markLiveStoreMutated. build OK.
### WIRING flushStagedBeforeCoarse into 8 coarse builtins (start of each): DONE Chparent. TODO:
Chparents(objects_hierarchy:276), Recreate(:702), Renumber(objects_misc:11), AddVerb(verbs:347),
DeleteVerb(:501), SetVerbInfo(:553), SetVerbArgs(:624). NOT SetVerbCode (transactional/staged).
### THEN: step 5 tx.RecycleObject (Fable: recycleWrites/buildImageRecycled, NO ForgetObject, read-record
per relative, recycledID mutex, deferred RecyclePlayer). Flip builtinCreate + builtinRecycle
(tx&&!liveMutated&&!anonymous). Re-flip builtinMaxObject via tx.MaxObject.
### VERIFY: mixed test (create;move;recycle), create/recycle unit tests, full suite, -race, CONFORMANCE
differential (locations_basic_chain, renumber_multiple_inheritance, create;add_verb ubiquitous), measure 32p.
### WHY FLUSH CORRECT: created obj private (no conflict); other staged writes last-writer-wins = Toast
immediate semantics for the rare mixed-with-coarse task; conformance single-session (no concurrency).

### FLUSH DONE + WIRED + GREEN. Two bugs fixed during: (1) flush nil'd read maps -> coarse builtin's
subsequent markVerbScan panicked -> RE-INIT read maps to empty (not nil). (2) flush too broad (HasWrites)
changed timing of normal property/verb writes -> broke eval_fork -> NARROWED trigger to HasStagedTopology()
= createdObjects>0 || relationshipWrites>0 (topology only; property/verb writes still defer to commit).
All 8 coarse builtins wired. go test builtins+store+scheduler GREEN.

### NOW: STEP 5 tx.RecycleObject. SCOPE DECISION: full recycle (reparent children w/ parents-writes) is
big. Do SIMPLE-recycle decentralized (no children AND no contents -> just tombstone + parent.children
remove + location.contents remove); COMPLEX recycle (has children/contents) -> coarse (flush handles it).
Build task o=create(#1);recycle(o): o has no children/contents -> simple -> decentralized -> WIN.
locations_basic_chain recycles c,b,a in reverse (each simple after prior removed from contents) -> all decentralized.
PIECES: recycleWrites set on tx; buildImageRecycled(old,ts) tombstone (clear props/verbs/contents, loc
nothing, recycled+invalid flags, stampAll); tx.RecycleObject(id) simple-only returns ok/complex sentinel;
commitDecentralized+coarse-branch build loop apply buildImageRecycled for recycleWrites ids (LAST, after
write maps); recycledID needs mutex (Fable P0-6); NO ForgetObject (Fable P1-1, reads=conflict guard);
deferred RecyclePlayer/beginRecycle if player. Then flip builtinCreate + builtinRecycle (simple case) +
re-flip builtinMaxObject via tx.MaxObject. VERIFY mixed test + full suite + -race + CONFORMANCE + measure 32p.

### RECYCLE IMPL PROGRESS: recycleWrites field on tx; recycledMu + appendRecycledID on Store; LowestFreeID
read guarded by recycledMu; buildImageRecycled(old,ts) tombstone (matches store.Recycle: clear contents/
location/properties/verbs, recycled+invalid flags, stampAll). tx.RecycleObject(id)->(handled,ec): simple
only (no children/contents -> handled=true; else handled=false=coarse fallback), reads obj (conflict guard),
removes from location.contents + parents.children (deltas), stages recycleWrites[id] + tombstone cache.
recycledID appended at COMMIT (RLock), NOT task time (avoids racing coarse Lock appends).
HasWrites/HasStagedTopology/empty-guard += recycleWrites. DONE so far.
### TODO: commitDecentralized: footprint += recycleWrites; slot-lock (recycled id has slot already unless
created-and-recycled -> getOrCreate handles via createdObjects); build loop apply buildImageRecycled LAST
(after write maps, for recycleWrites ids); appendRecycledID per recycled id; nil recycleWrites at end.
coarse branch applyStagedToLiveLocked: publish tombstone for recycleWrites (republishForMutation+buildImage
Recycled) + appendRecycledID; nil at end. THEN flip builtinCreate/builtinRecycle/builtinMaxObject. VERIFY.
### CRITICAL for create;recycle-SAME-obj (build task): id in BOTH createdObjects + recycleWrites. Build:
clone(base)+stampAll+writemaps+buildImageRecycled -> tombstone. parent childrenDelta: create add + recycle
remove -> net none. Works. slot getOrCreate (created). appendRecycledID(id).
### THEN step 5 recycle, step 6 flip recycle. VERIFY each: build + go test + -race + conformance differential.
### Fable full traces: scratchpad/create-recycle-review-notes.md.
### THEN: go build; go test ./db/store -race (behavior-preserving check). THEN SLICE 2: decentralize
create (id atomic alloc, stage image+parent.children, commit publishes into lock-free dir, NO liveMutated;
retry-safe: id waste on retry ok). THEN recycle decentralize. Measure (32p realistic ->~190k target),
conformance, commit.

### KEY TRANSFORM RULE (stable): remember-then-mutate-X -> X=republishForMutation(X)+mutate.
remember-then-REPLACE (publishLocked new obj) -> LEAVE (already safe). mutate-BEFORE-remember
(ResetInheritedProperties) -> restructure: scan, republish, mutate fresh. verb ops -> republish
+ re-resolve verb from fresh. helpers mutating via s.load (attachChildToParentsLocked) -> caller
republishes target first.

## 2026-07-22 FINISH LINE — builtinRecycle FLIPPED (all 3 flips now done)
State: builtinCreate (decentralized when tx!=nil && !liveMutated && !anon), builtinMaxObject
(tx.MaxObject()), and builtinRecycle ALL flipped in builtins/objects.go.
builtinRecycle flip: after the :recycle hook + anon-cascade (cascade sets liveMutated so
anon-holding objects go coarse), decision:
  tx := readTxn(ctx); isPlayer := hasObjectFlagForRead(objID, FlagUser)
  if tx && !ctx.LiveStoreMutated && !isPlayer { handled,ec := tx.RecycleObject(objID); decentralized=handled }
  if decentralized { store.NoteVerbCacheClear() }  // NO ForgetObject (reads=guard), NO player boot, NO markLiveStoreMutated
  else { store.Recycle + markLiveStoreMutated + ForgetObject + AdoptLiveRelationships + RecyclePlayer + NoteVerbCacheClear }
Fable P0-5 honored: players stay coarse (isPlayer guard). P1-1 honored: decentralized path skips ForgetObject.
P1-6 honored: anon-cascade sets liveMutated -> coarse.
### NEXT (verify gate, in order): 1) go build ./... 2) go test ./db/store ./builtins ./scheduler incl
TestMoveMixedWithCoarseBuiltinsStaysConsistent 3) full go test ./... 4) go test -race on the 3 pkgs
5) conformance differential (linux CGO0 barn vs baseline; expect only 4 pre-existing dump_persistence)
6) measure 32p realistic goodput (target ~190k). THEN commit finish-line work.

## 2026-07-22 DIAGNOSING scheduler test FAILURE
FAIL: TestCommandEvalChparentPropertyResetUsesTransactionReseed (scheduler pkg).
Uncaught E_INVARG during eval. Test: a=create($nothing);b=create;c=create; add_property(a,..);
add_property(b,..); chparent(c,a); c.foo="bar"; chparent(c,b); property_info. Mixes DECENTRALIZED
create with COARSE chparent + transactional add_property.
- Build clean. db/store + builtins PASS. Only this scheduler test fails.
- No recycle in this test -> the CREATE flip (not recycle flip) is the likely cause.
- add_property uses tx.DefineProperty (transactional, read-your-writes) -> should see created obj.
- applyStagedToLiveLocked publishes createdObjects FIRST (line 1938) then writes -> flush order OK.
- objectExistsForRead -> validForRead. tx.object(id) (store_txn.go:233) checks tx.objects cache then
  tx.objectLocked. Need to confirm objectLocked/createdObjects visibility for a decentralized-created id
  in read-your-writes BEFORE flush. CHECKING objectLocked + createdObjects read path next.
HYPOTHESIS: a read validator or objectLocked does NOT surface createdObjects images to reads, so
add_property(a) or chparent(c,a) sees a as nonexistent -> E_INVARG. OR: flush at chparent applies
created but the staged propertyDefines were keyed/ordered wrong. NEXT: instrument minimal repro OR
read objectLocked createdObjects branch. Do NOT guess — reproduce.

## 2026-07-22 ROOT CAUSE FOUND (via bisect repro TestReproBisect)
create-only commits fine; create+add_property on the created obj PANICS: nil deref in
commitDecentralized store_cow.go:493. `live := s.load(key.objID)` returns NIL for a created id
(not published yet), then propertyByName(live.properties,...) derefs nil.
Two pre-build validation loops (propertyDefines collision @491-498, propertyDefinitionDeletes @501-507)
call s.load unconditionally — but a created id's base image is tx.createdObjects[id], not s.load.
The writeIDs liveness loop @470-477 already skips created; these two did NOT.
Build loop @554-607 IS correct (uses created base). FIX: in both validation loops, fall back to
tx.createdObjects[key.objID] when s.load==nil. Applying fix now. Then rerun TestReproBisect (expect all
5 cases green) + the failing scheduler test + full suite. zz_repro_test.go is a TEMP scratch file — DELETE before commit.

## 2026-07-22 FIX #1 LANDED (partial): propertyDefines/definitionDeletes validation loops in
commitDecentralized now fall back to tx.createdObjects[id] when s.load==nil. Case 2 (create+addprop)
NOW PASSES. But case 3b STILL FAILS: a=create; c=create; add_property(a,foo); chparent(c,a) -> E_INVARG
(uncaught in chparent builtin). 3a (chparent two created, NO addprop) PASSES -> the property DEFINE is
the trigger. Flush DOES clear staged maps (store_txn.go:2099-2107) so double-apply ruled out.
builtinChparent uses tx.* predicates (objects_hierarchy.go:186 HasDefinedPropertyConflictWithAncestry,
:205 DefinedPropertyNamesInAncestry, :213 HasChparentDescendantPropertyConflict) that mix tx.objects
cache + live. After flush, tx.objects cache still holds pre-flush a(with foo)/b/c while live has fresh
published images. HYPOTHESIS: one of those 3 predicates falsely reports conflict OR returns E_INVARG
for the created/flushed object. Writing store-level repro (db/store/zz_repro_test.go) to find WHICH
predicate. Build err: NewProperty perms arg type — fixing signature. zz_repro_test.go files (scheduler+
db/store) are TEMP — DELETE before commit.

## 2026-07-22 store repro PASSES but eval FAILS -> divergence is ENVIRONMENTAL
Corrected store repro: create a(nil parents); create c(nil); DefineProperty(a,foo); FlushStagedToLive;
then FULL chparent tail (all 3 predicates + ChangeParents + AdoptLiveRelationships + ReseedInherited
Properties). ALL PASS at store level (c parents -> [a], no error). So the mechanism in isolation is fine.
builtinCreate FILTERS $nothing -> tx.CreateObject gets EMPTY parents (not [-1]). Phantom-parent theory dead.
Eval still fails 3b E_INVARG in chparent. Divergence candidates: Test_conf.db start state vs immutFixture;
owner=player#8 wizard vs owner 3; scheduler/task ctx. NEXT: instrument builtinChparent (temporary
println at each E_INVARG/E_INVIND return + after flush) run scheduler TestReproBisect 3b, see which return
fires. Then remove instrumentation. TEMP FILES to delete before commit: scheduler/zz_repro_test.go,
db/store/zz_repro_test.go, any println.

## 2026-07-22 chparent SUCCEEDS; failure is at eval FINAL COMMIT (validation conflict)
Instrumented builtinChparent: 3b reaches tail, NO tail error marker -> chparent returns Ok. Task runs
chparent TWICE = RETRY. So commit of attempt1 fails E_INVARG (ValidationFailed) -> retry -> fail -> uncaught.
Mechanism suspected: flush clears read maps + sets liveMutated; chparent re-records reads of a/c AFTER
flush (createdObjects now nil, so those ids no longer read-exempt); chparent/Reseed coarsely republish
c/a bumping version; final commit validates the stale recorded read vs bumped live version -> E_INVARG.
WHY original (coarse create) passes: TBD — likely reads-after-liveMutated handled differently, OR
createdObjects exemption. NEXT: instrument the 4 validateObject*ReadsLocked to print objID + recorded
vs live version on mismatch; run 3b; identify the conflicting read. Then fix (likely: flush should NOT
leave the task validating self-writes — either keep createdObjects ids exempt post-flush, or drop
post-flush reads at commit for a liveMutated task). Files w/ temp instrumentation: objects_hierarchy.go
(CHP- prints), + will add validator prints. ALL temp -> remove before commit.

## 2026-07-22 ROOT CAUSE #2 CONFIRMED + partial fix + NEW regression
Validator instrumentation: 3b commit conflict = VAL-propscan id=9(a) rec=14 live=15. CONFIRMED: flush
applied add_property define (a->v15) + cleared read maps, but tx.objects CACHE still held pre-flush a@v14;
chparent post-flush conflict-checks (DefinedPropertyNamesInAncestry(a)) read stale cache -> recorded
propertyScan[a]=14 -> commit validates 14 vs live 15 -> E_INVARG self-conflict -> retry x2 -> uncaught.
FIX applied: FlushStagedToLive now also clears tx.objects + tx.owned (drop stale cache; writes already in
live). Version conflict GONE. BUT NEW REGRESSION: clearing whole cache broke 3a (was passing): now ALL
cases raise E_INVIND "{#-1,...}" (this=#-1). Clearing entire cache mid-task too broad. NEXT: find where
E_INVIND raised (eval body vs commit). Consider targeted invalidation: only drop cache entries for FLUSHED
ids (staged-write footprint), not the whole cache; OR don't clear tx.owned. Instrumenting validLiveObject
branches. temp: CHP- prints (objects_hierarchy.go), VAL- prints (store_txn.go), zz_repro_test.go x2.

## 2026-07-22 cache-clear regression localized: `chparent; return 42` fails E_INVIND
3a0 (two creates, no chparent) PASSES. 3a1 (chparent; return 42) FAILS E_INVIND. So chparent's
FlushStagedToLive cache-clear corrupts running task state — even returning a constant raises E_INVIND
(traceback this=#-1). No VAL-INVIND (not commit), no CHP tail err (chparent returns Ok). Clearing the
WHOLE tx.objects cache mid-task is UNSAFE: live VM frames alias cached *Object pointers; nuking the map
strands them. DECISION: surgical fix — refresh ONLY the flushed footprint ids in the cache (re-fetch
fresh live image) instead of clearing the whole cache. That fixes the stale-version reads without
disturbing VM-held aliases (this/verb objects). Testing owned-clear culprit + footprint-refresh next.

## 2026-07-22 TRUE ROOT CAUSE (both symptoms): flushed created objs invisible after cache clear
CHP-validForRead-obj-FALSE id=10: after clearing tx.objects, tx.Valid(created c=#10)=FALSE. WHY:
objectLocked gates on `objectVersion(live) <= tx.readTS`. Flush publishes created/written objs at
FLUSH ts > readTS, so snapshot re-fetch MISSES them -> created obj reads invalid mid-task. The cache
was the ONLY thing making created objects visible. So the "#-1 / return 42" regression = chparent's
early validForRead(c) failing, NOT a VM-alias problem. And the ORIGINAL stale-version self-conflict =
cache holding pre-flush (old version) images.
FIX (replaces the clear): after applyStagedToLiveLocked, REFRESH the cache — re-clone each cached id
from CURRENT live (via s.load, bypassing readTS). Created/written objs -> visible + current version;
non-flushed objs -> same version. Task is liveMutated/non-isolated so live-as-of-flush is correct.
Implementing now. Then all 5 repro cases should pass; then the real scheduler test; then full suite +
race + conformance + measure. Remove ALL temp: CHP-/VAL- prints, zz_repro_test.go x2.

## 2026-07-22 FIX LANDED + verified: all 3 pkgs GREEN
FlushStagedToLive now REFRESHES the object cache from current live (re-clone each cached id via s.load,
bypass readTS; drop invalid). Fixes BOTH the stale-version self-conflict (E_INVARG) AND the flushed-
created-object-invisible (E_INVIND). All TestReproBisect cases pass incl full case5 -> {1,{1,1}}.
CLEANUP DONE: removed all CHP-/VAL- prints + 2 scratch test files. CAUGHT+FIXED a self-inflicted bug:
cleanup strip-loop deleted the two `return types.Err(E_INVARG)` inside chparent's `if conflict{}` blocks
(line dropped whole `println(...); return` line); restored both. objects_hierarchy.go diff now clean
(only 3 flushStagedBeforeCoarse additions). store_txn.go validators intact.
VERIFIED: go test ./db/store ./builtins ./scheduler -count=1 -> ALL ok (incl the previously-FAILING
TestCommandEvalChparentPropertyResetUsesTransactionReseed + TestMoveMixedWithCoarseBuiltinsStaysConsistent).
### REMAINING GATE (in order): full `go test ./...`; -race on 3 pkgs; conformance differential (linux CGO0
barn vs baseline, expect only 4 pre-existing dump_persistence); measure 32p realistic goodput (~190k target).
THEN commit finish-line work.

## 2026-07-22 GATES PASSING
- full go test ./... -count=1: GREEN (no failures).
- go test ./db/store ./builtins ./scheduler -race -count=1: ALL ok, 0 DATA RACE (scheduler 245s).
- linux barn cross-compiled: barn_finish_linux (CGO0, 21MB). WSL alive, binary runs. DELETE before commit
  (barn_finish_linux in repo dir + copy).
- conformance substrate: arithmetic slice 10/10 pass (harness healthy end-to-end).
- conformance blast-radius (create/recycle/move/chparent/chparents/renumber/verb/propert/parent):
  RUNNING in background (task bizly94in). Awaiting result; expect only the 4 known dump_persistence
  artifacts (none in this -k set actually, so expect fully green for blast radius).
### AFTER conformance green: measure 32p realistic goodput (~190k target) THEN commit finish-line:
flush cache-refresh fix + created-obj property-define validation fix + builtinCreate/MaxObject/Recycle
flips. Commit msg should note the flush cache-refresh subtlety.

## 2026-07-22 CONFORMANCE REGRESSION (differential CONFIRMED) — anon fix insufficient
Blast-radius finish run: 56 failed (24 unique, 17 anonymous::). BASELINE (HEAD, stashed) same families:
only 1 fail [limits::setremove_fails_if_result_too_large] (pre-existing E_QUOTA). So my change REGRESSED
~23 tests (anonymous/stress_objects/gc/create/recycle). Fix #1 (skip anon in flush cache-refresh: leave
anon cache entries untouched since s.load can't resolve out-of-band anon) applied + Go tests still green,
BUT conformance re-run still fails anon/stress_objects/gc (37 failed). So skip-anon INSUFFICIENT — deeper
issue. Suite is known-nondeterministic (notes 430-431) so MUST run isolated single-test differential to
separate real regression from flake. NEXT: run representative failures (anonymous::defined_properties_work_x,
stress_objects::ancestors_include_self_multi_level, gc::single_cyclic_inherited_anonymous) ISOLATED on
BOTH barn_finish_linux and barn_baseline_linux; compare. Then localize which flip/flush breaks anon.
binaries in repo dir: barn_finish_linux, barn_baseline_linux (DELETE before commit).

## 2026-07-22 REGRESSION mechanisms (2 found, 1 fixed)
Isolated single-test differential (finish FAIL, baseline PASS) = REAL regression, not flake. Two causes:
(1) ANON child of STAGED numbered parent: `a=create(z,1)` (anon) takes COARSE create path which did NOT
    flushStagedBeforeCoarse -> store.CreateObject reads live z (only staged) -> anon inherits nothing ->
    E_INVIND. FIX: added flushStagedBeforeCoarse(ctx) at top of builtinCreate coarse branch. VERIFIED:
    anonymous::defined_properties_work_x + gc::single_cyclic_inherited_anonymous now PASS.
(2) READ builtins bypass tx: builtinAncestors/Descendants call store.Ancestors/store.Descendants DIRECTLY
    (objects_hierarchy.go:445,472) -> miss STAGED decentralized creates -> stress_objects::ancestors_*
    fails. At HEAD creates were coarse(live) so store.X saw them; the create flip broke this. tx has
    Parents/Children (read-your-writes) but NOT Ancestors/Descendants.
FIX PLAN for (2): add tx.Ancestors/tx.Descendants (walk via tx.Parents/tx.Children RYW); builtinAncestors/
Descendants use tx when present. CONCERN: this may be a BROADER pattern (other read builtins calling
store.X directly miss staged creates: isa?, etc.). NEXT: implement tx.Ancestors/Descendants + wire; then
run FULL blast radius to measure remaining scope; audit other store.-direct read builtins if more fail.

## 2026-07-22 fix (2) DONE (tx.Ancestors/Descendants RYW) -> anon/gc/ancestors GREEN. NEW cluster: RECYCLE
After create-flush + ancestry-RYW fixes: blast radius 24 failed, now dominated by objects::recycle_* +
recycle::*anonymous + stress_objects recycle. Two gaps in builtinRecycle flip (objects.go ~454-490),
SAME pattern as create:
  (a) coarse recycle fallback (store.Recycle @470) does NOT flushStagedBeforeCoarse -> staged children/
      objID invisible to live -> reparent/clear fails.
  (b) NO anon guard: tx.RecycleObject(anonID) stages decentralized recycle of an out-of-band anon (no
      numbered slot) -> commit mishandles. Need !isAnonymous on the decentralized condition.
FIX: add objectIsAnonymousForRead guard to decentralized cond; add flushStagedBeforeCoarse at top of
coarse recycle branch. helpers: objectIsAnonymousForRead (store_reads.go:67), flushStagedBeforeCoarse.
Applying now, then rebuild + rerun recycle family. If MORE recycle regressions cascade -> consider
reverting recycle flip, shipping create+move+flush (SHIP THE WIN). tx.Ancestors/Descendants added
store_txn.go; builtinAncestors/Descendants wired tx-path (objects_hierarchy.go). Go tests green.

## 2026-07-22 recycle-reparent fixed (24->12). REMAINING = broader read-builtin-bypasses-tx pattern
12 failures, categorized by mechanism (all: read builtin calls store.X directly, misses STAGED
decentralized create/recycle):
  - objects::parent_on_recycled_object_error, children_on_recycled_object_error, recycle_parent_returns_
    invarg, recycle_children_returns_invarg, stress_objects::{object_bytes,parent_invalid}_recycled_object
    -> parent()/children() on a STAGED-recycled obj: builtinParent maps E_INVIND->E_INVARG only if
    store.IsRecycled(id); staged recycle not in live -> IsRecycled false -> wrong code. NEED tx-aware IsRecycled.
  - miscellaneous::isa_with_create_no_opt -> isa() bypasses tx (like ancestors did). NEED tx-aware isa.
  - property::create_child_of_root, create_grandchild -> property inheritance from staged-created parent. INVESTIGATE.
  - object_hierarchy::recycled_objects_includes_recycled -> recycled_objects() scans store, misses staged recycle.
  - object_hierarchy::owned_objects_{wizard_owns,respects_owner} -> owned_objects() scans store, misses staged create.
STRATEGY: the 3.03x is ALREADY on master; this branch adds create+recycle decentralization (~1.82x more).
Read-builtin blast radius affects BOTH create & recycle, is a BOUNDED tx-aware pattern. PUSH THROUGH: batch-fix
isa/IsRecycled/recycled_objects/owned_objects + property, re-measure. If count->0 ship; if unrelated failures
keep appearing, revert recycle flip & ship create+move+flush. Current: 12 failed, 3265 passed.

## 2026-07-22 batch tx-aware read fixes (in progress)
Added tx.HasAncestor + tx.IsRecycled (store_txn.go). Wired: builtinIsa->tx.HasAncestor; parent/parents/
children recycled-mapping -> isRecycledForRead helper (store_reads.go, tx.IsRecycled). recycled_objects ->
flushStagedBeforeCoarse (rare scan). STILL TODO: owned_objects flush, next_recycled_object flush,
objects_misc.go:163 IsRecycled, investigate property::create_child_of_root + isa_with_create_no_opt.
DIAG: store unused in builtinParent/Parents/Children (15,47,84) after IsRecycled swap -> remove `store :=
ctx.Store` there. Then rebuild + rerun 12-failure set. Current uncommitted surface growing but bounded &
all same pattern (decentralized create/recycle -> read builtins must be tx-aware or flush). temp binaries
barn_finish_linux/barn_baseline_linux in repo (DELETE before commit).

## 2026-07-22 12->4 failures. LAST cluster: occupants/locations parent-filter uses store.HasAncestor
Batch tx-aware fixes landed: isa, parent/parents/children IsRecycled, object_bytes IsRecycled, recycled_
objects/owned_objects/next_recycled flush. Broad blast radius now 4 failed / 3537 passed. Remaining 4 =
object_hierarchy::occupants_* + locations_with_parent_check -> occupants (objects_movement.go:185) and
locations (objects_hierarchy.go:630) call store.HasAncestor directly. FIXED occupants:185 -> tx.HasAncestor.
TODO: locations:630 -> tx.HasAncestor. Then rebuild + rerun; expect blast radius GREEN. Then FULL conformance
differential (finish vs baseline) to confirm ONLY pre-existing failures remain (setremove E_QUOTA + 4 dump_
persistence). Then measure 32p goodput. Then DELETE temp binaries + commit.

## 2026-07-22 *** BLAST RADIUS GREEN *** (0 failed / 3587 passed)
locations:630 -> tx.HasAncestor fixed. Full blast radius (anonymous/stress_objects/gc/create/recycle/
chparent/renumber/objects/property/miscellaneous/object_hierarchy/occupants/location): 3587 passed, 0 FAILED.
COMPLETE list of tx-aware/flush read-builtin fixes for decentralized create+recycle visibility:
  tx-aware (RYW, no flush - common reads): ancestors, descendants, isa, occupants(parent filter),
    locations(parent filter), parent/parents/children (IsRecycled mapping), object_bytes (IsRecycled).
  flush (rare scans): owned_objects, recycled_objects, next_recycled_object. + coarse create & coarse
    recycle branches flushStagedBeforeCoarse. + anon/player guards on decentralized recycle.
New tx methods: tx.Ancestors, tx.Descendants, tx.HasAncestor, tx.IsRecycled + isRecycledForRead helper.
GATES NOW RUNNING (bg): (1) FULL conformance suite finish barn (b35h69mrq) — expect only pre-existing
setremove + 4 dump_persistence. (2) go test -race all 3 pkgs (bb2z3368e). AFTER both green: differential-
confirm the full-suite failures == baseline's; measure 32p goodput; delete temp binaries; COMMIT.

## 2026-07-22 FULL SUITE: 1 real regression left = waif_colon_properties (create flip)
Full conformance finish barn: 5 failed / 11425 passed. Differential (isolated, both barns):
  - 4 dump_persistence (map/string/waif_dump + inherited_override): inherited_override PASSES isolated on
    BOTH -> full-run failure = suite nondeterminism, NOT regression. map/string/waif_dump = known CGO0
    artifacts (fail baseline too). => NOT regressions.
  - waif::waif_colon_properties_are_instance_properties: FAILS finish isolated, PASSES baseline isolated
    = REAL REGRESSION. Returns E_INVIND.
ATTRIBUTION (disable experiments, isolated waif): recycle-flip-disabled+create-on => FAIL; BOTH flips
disabled => PASS. => the CREATE flip is the culprit. `x=create($waif);y=create(x);z=create(y);q=create(z)`
staged; then z:new()/w.x/wq.class.q waif ops read staged class via a path bypassing tx -> E_INVIND.
Flips RESTORED (both active). Race re-run GREEN (0 races, all 3 pkgs ok). Go tests green.
NEXT: find the waif read path(s) that call store.X directly (waif :new verb resolution, waif property read,
.class). Make tx-aware like the others. Full suite is broad coverage (11425 pass) so waif is likely the
LAST staged-create-invisibility gap. Fix -> rerun waif isolated + full suite -> measure 32p -> commit.
temp binaries barn_finish_linux/barn_baseline_linux (DELETE before commit).

## 2026-07-22 WAIF REGRESSION FIXED (last one)
Root: getWaifProp `class` case (vm/op_property.go:98) used vm.Store.ObjectExists(classID) DIRECTLY ->
staged class q missed -> wq.class returns #-1 -> wq.class.q = #-1.q -> E_INVIND. FIX: use tx-aware
objectExistsForRead(vm.Store, vm.storeTxn(), classID) (pattern already used 25 lines below). waif test now
PASSES isolated. Go tests ./vm ./builtins green. Full go test ./... GREEN.
DIFF: 12 files, +572/-59. No stray debug/&&false. Commit msg drafted at scratchpad/commit_msg.txt.
GATE RUNNING (bg byf60yqxs): full conformance suite after waif fix — expect only pre-existing 4 dump_
persistence (nondeterministic). AFTER green: measure 32p goodput (clean CPU, ~190k target); rm temp
binaries barn_finish_linux/barn_baseline_linux; git commit -F scratchpad/commit_msg.txt.

## 2026-07-22 CONFORMANCE CLEAN (differential confirmed) + PERF MEASUREMENT (delta modest/noisy)
CONFORMANCE: full suite finish 4 failed/11426 passed; ALL 4 = dump_persistence family. Baseline dump_
persistence -k: 5 failed (superset). => finish failures are a SUBSET of baseline's nondeterministic CGO0
dump_persistence flakiness. ZERO new conformance failures. DIFFERENTIAL CLEAN.
PERF (32p/16r realistic, 0% abort both):
  - baseline (coarse create/recycle, stashed): 96,086 (1 sample)
  - finish (decentralized): 108,369 / 96,501 / 105,594 (3 samples) -> median ~105k
  - Delta ~ +10% but NOISY (finish spans 96-108k; GCs swing 163-537 per 3s window; noise ~ margin).
  - NOTE: build=0 ceiling was 190k but that REMOVES build tasks entirely. Bench build task =
    `o=create(#root); recycle(o)` on the SHARED root -> all 32 goroutines' build commits serialize on
    root's slot mutex -> caps decentralized benefit. Worst-case contention shape.
Robust measurement running (bg b650ywqr1): finish 8s x9. THEN baseline 8s x9 interleaved to get honest
delta. DECISION PENDING on magnitude: correctness is solid + zero regression, so landing is safe either
way (reversible, in git); but if delta is within noise, report honestly to Q rather than overclaim.

## 2026-07-22 *** ROBUST PERF: +17% at 32p/16r realistic, 0% abort ***
Like-for-like median-of-9, 8s window, 32p/16r realistic:
  - baseline (coarse create/recycle):    109,724 /s  0% abort
  - finish  (decentralized create/recycle): 128,442 /s  0% abort   => +17.1%
Short 3s windows were noisy (GC); 8s x9 median is the honest comparison. Confirming finish sample
running (bg b67lqabdf). If it holds ~125-130k -> COMMIT.
Honest framing: modest but real +17% goodput, zero abort, ZERO conformance regression (differential
clean), race clean. Not the 1.8x the plan's build=0 ceiling implied (that removed build tasks; real
bench build task hammers the SHARED root slot). Worth landing: reversible, no regression, positive win,
and the tx-aware read plumbing + decentralized topology machinery enable further wins.
COMMIT PLAN: rm barn_finish_linux barn_baseline_linux (temp); git commit -F scratchpad/commit_msg.txt
(on branch mvcc-phase3b). Commit msg already drafted - update the perf line to +17% (110k->128k).

## 2026-07-22 *** COMMITTED 26da777 *** (branch mvcc-phase3b)
feat(store): decentralized create + recycle (Phase 3b finish line). 12 files, +572/-59.
Final gate: go test ./db/store ./builtins ./scheduler ./vm -count=1 = ALL ok.
Baseline robust samples {92k,110k}; finish {112k,128k} -> non-overlapping, ~+15-19%, 0% abort.
Conformance differential clean (only pre-existing dump_persistence CGO0 flakiness). Race clean.
NOT pushed (branch only, per no-push-without-okay). Temp linux binaries deleted.
DONE with the finish-line create+recycle decentralization.
