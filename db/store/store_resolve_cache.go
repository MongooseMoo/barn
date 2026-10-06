package store

import (
	"fmt"
	"sync"

	"github.com/MongooseMoo/barn/types"
)

// store_resolve_cache.go — allocation-free ancestry walks (Part A) and a
// per-transaction memo of verb/property resolution (Part B).
//
// PART A — reusable walk scratch.
//
// findVerb and findProperty re-walked the ancestry chain from scratch on every
// verb call and every property access, each walk allocating a fresh BFS queue
// slice and a fresh `visited` map. On the 16-player mongoose profile that was
// 11.5M queue allocations and 5.1M visited-map allocations (findProperty alone,
// 6.3% of alloc_objects) plus findVerb at 4.4%. The walk state is now reusable
// scratch hanging off the StoreTxn, so a steady-state walk allocates nothing.
//
// PART B — per-transaction resolution memo.
//
// The memo is keyed by (start object, QUERIED name) and stores the resolution
// result plus the exact list of objects the walk visited. It is deliberately
// PER-TRANSACTION rather than store-global, stamped by the txn's own snapshot:
//
//   - A StoreTxn is a fixed MVCC snapshot (readTS, store_txn.go BeginSnapshot)
//     and lives for a whole task slice (engine/task_runtime.go begins one
//     per attempt and only replaces it after a commit), which on the mongoose
//     workload is hundreds to thousands of verb calls and property reads. So the
//     memo has a real working set to amortize over.
//   - A store-global memo stamped with the global commit clock would be WRONG
//     for any transaction whose readTS lags the clock (a long-running or
//     retried task reads an older snapshot through s.history —
//     store_txn.go objectLocked). It would only be usable when
//     tx.readTS == clock, which is exactly the case a global epoch cannot
//     cheaply prove per lookup. Scoping the memo to the snapshot that produced
//     it removes that class of bug entirely.
//   - A per-txn memo needs no lock. StoreTxn is single-goroutine by
//     construction: tx.object (store_txn.go:247-262) writes the unsynchronized
//     tx.objects map on the pure-READ path, so two goroutines sharing one txn
//     would already be a data race today.
//
// CORRECTNESS: read-tracking is preserved exactly. The walk records every
// object it visited and whether it marked a scan or a read on it; a memo hit
// replays those same mark calls (replayVerbSteps / replayPropSteps below)
// before returning, so tx.verbScans/verbReads/propertyScans/propertyReads end
// up identical to an uncached run and committed-write conflict detection is
// unaffected.
//
// CORRECTNESS: staged writes bypass the property and store-global verb memos.
// The transaction-local verb memo additionally accepts walks whose entire path
// remains unowned: unrelated writes cannot mutate these immutable images. Paths
// through any owned object are never cached, even after their first mutation.
// The property and store-global memos are live only while
// len(tx.owned) == 0 — i.e. while the transaction has not privatized a single
// object. Every staging path (SetPropertyValue, DefineProperty, SetVerbCode,
// CreateObject, MoveObject, RecycleObject, ...) goes through
// mutableObject/privatizeCached first, which marks the object owned, and
// `owned` only ever grows within a transaction. So the first staged write
// disables those memos for the remainder of the transaction and its own writes are
// always read back by a real walk. (The single exception is a successful
// FlushStagedToLive, which publishes the staged writes, re-clones every cached
// object from current live and resets tx.owned; it invalidates the memo
// explicitly, and the fresh clones it installs are unowned, so nothing can be
// mutated in place without a new privatizeCached. A failed flush preserves
// owned and therefore keeps the memo disabled.) The gate also guarantees no
// property or store-global memoized entry can ever reference a
// txn-private object: with owned empty, every cached *Object is a shared
// IMMUTABLE published image, whose properties/verbs/parents cannot change
// under us.
//
// Belt and braces: even inside that window, a hit re-verifies that
// tx.objects[id] still holds the very same *Object pointer for every step of
// the recorded walk, which catches the paths that REPLACE a txn cache entry
// without owning it (AdoptLiveObject, ForgetObject).

// resolveCacheCap bounds each memo. MOO code can synthesize unlimited distinct
// verb and property names, so the map is dropped wholesale once it exceeds the
// cap rather than grown — the house pattern from builtins/regexcache.go.
const resolveCacheCap = 512

// objIDSetLinearMax is the size at which the walk's visited set stops being a
// linearly-scanned slice and promotes to a map. Real ancestry chains are
// shallow (< 8 on mongoose), where a linear scan of an already-hot slice beats
// hashing; the promotion keeps a pathological wide graph from going quadratic.
const objIDSetLinearMax = 24

// objIDSet is a reusable "visited" set for one ancestry walk.
type objIDSet struct {
	list []types.ObjID
	set  map[types.ObjID]struct{}
}

func (s *objIDSet) reset() {
	s.list = s.list[:0]
	if s.set != nil {
		clear(s.set)
	}
}

// add records id and reports whether it was NEWLY added (not already visited).
func (s *objIDSet) add(id types.ObjID) bool {
	if s.set != nil {
		if _, ok := s.set[id]; ok {
			return false
		}
		s.set[id] = struct{}{}
		return true
	}
	for _, v := range s.list {
		if v == id {
			return false
		}
	}
	s.list = append(s.list, id)
	if len(s.list) > objIDSetLinearMax {
		s.set = make(map[types.ObjID]struct{}, 2*len(s.list))
		for _, v := range s.list {
			s.set[v] = struct{}{}
		}
	}
	return true
}

// verbWalkStep records one object VISITED by a verb-resolution walk: the
// pointer tx.object returned for it (so a replay can prove the txn's view of
// that object is unchanged) and whether the walk marked a verb scan on it.
type verbWalkStep struct {
	id      types.ObjID
	obj     *Object
	scanned bool
}

// propWalkStep is the property-walk counterpart. `valid` mirrors
// validLiveObject(obj) (an invalid object is skipped, marking nothing);
// `found` distinguishes the markPropertyRead case from the markPropertyScan
// case, and carries the exact arguments the walk passed.
type propWalkStep struct {
	id         types.ObjID
	obj        *Object
	valid      bool
	found      bool
	actualName string
	prop       Property
}

// verbScratch is the reusable state of one verb-ancestry walk.
type verbScratch struct {
	queue   []types.ObjID
	visited objIDSet
	steps   []verbWalkStep
	inUse   bool
}

// propScratch is the reusable state of one property-ancestry walk.
type propScratch struct {
	queue   []types.ObjID
	visited objIDSet
	steps   []propWalkStep
	inUse   bool
}

// plainScratch is queue+visited only, for walkers that memoize nothing.
type plainScratch struct {
	queue   []types.ObjID
	visited objIDSet
	inUse   bool
}

// verbResolveKey keys the verb memo by the QUERIED name, verbatim. Storing the
// resolved target under the exact string the caller asked for is what preserves
// alias and `*` wildcard semantics (matchVerbNameLowered): the memo never has
// to reproduce the matching rules, only the answer they produced. Keeping the
// raw (not lowercased) name also means a case variant simply occupies its own
// entry instead of relying on the map's key canonicalization holding.
type verbResolveKey struct {
	objID          types.ObjID
	name           string
	requireExecute bool
}

type verbResolveEntry struct {
	steps   []verbWalkStep
	verb    *Verb // nil records a negative resolution (verb not found)
	definer types.ObjID
	// err is the exact error value the miss produced, memoized alongside it.
	// A "verb not found" error is built with fmt.Errorf on every failed lookup,
	// and MOO code probes for absent verbs constantly (:huh dispatch,
	// respond_to). Since the message is a pure function of the queried name —
	// which is part of the key — reusing the value is indistinguishable from
	// rebuilding it, minus three allocations per probe.
	err error
	// memo, when set, makes this a record of a store-level dispatch memo hit
	// rather than of a walk: steps is empty, memoSeen is what recordVerbMemoHit
	// has recorded for memo in this txn, and verb is the resolved verb once its
	// read mark is recorded from an unowned definer (see lookupVerbDispatchMemo).
	memo     *verbDispatchMemoEntry
	memoSeen uint8
}

type propResolveKey struct {
	objID types.ObjID
	name  string
}

type propResolveEntry struct {
	steps []propWalkStep
	prop  Property
	name  string
	ec    types.ErrorCode // E_PROPNF records a negative resolution
}

// resolveCacheActive gates property memoization. Local verb entries instead
// validate that every object on their path is unowned.
func (tx *StoreTxn) resolveCacheActive() bool {
	return len(tx.owned) == 0
}

// verbMemoActive gates the store-global verb dispatch memo. A memo entry names
// its definer and verb-list index, so it stays exact while every private copy
// keeps the snapshot's verb lists and parents: property-value, location and
// verb-code writes privatize objects without changing dispatch. Verb deletes,
// creates and recycles set privateVerbShape; topology and verb-definition
// builtins mutate the live store, which disables the memo in
// materializeVerbMemoMarks. Commit still fails a memo user whose snapshot
// predates a verb-shape change (validateVerbReadsLocked).
func (tx *StoreTxn) verbMemoActive() bool {
	return !tx.privateVerbShape
}

// invalidateResolveCaches drops both memos. Called from the paths that REPLACE
// a txn object-cache binding or the txn read set without owning the object
// (AdoptLive*, ForgetObject, MarkLiveMutated, Commit/Flush).
func (tx *StoreTxn) invalidateResolveCaches() {
	if tx == nil {
		return
	}
	tx.verbResolve = nil
	tx.propResolve = nil
}

// verbStepsCurrent reports whether the txn's view of every object the recorded
// walk visited is still the identical immutable, unowned *Object pointer.
func (tx *StoreTxn) verbStepsCurrent(steps []verbWalkStep) bool {
	for i := range steps {
		if tx.owned[steps[i].id] || tx.objects[steps[i].id] != steps[i].obj {
			return false
		}
	}
	return true
}

// replayVerbSteps re-registers the read set the recorded walk produced. It is
// only called after verbStepsCurrent has passed for the WHOLE list, so the
// marks are never applied partially.
func (tx *StoreTxn) replayVerbSteps(steps []verbWalkStep) {
	for i := range steps {
		if steps[i].scanned {
			tx.markVerbScan(steps[i].id, steps[i].obj)
		}
	}
}

func (tx *StoreTxn) propStepsCurrent(steps []propWalkStep) bool {
	for i := range steps {
		if tx.objects[steps[i].id] != steps[i].obj {
			return false
		}
	}
	return true
}

func (tx *StoreTxn) replayPropSteps(steps []propWalkStep) {
	for i := range steps {
		st := &steps[i]
		if !st.valid {
			continue
		}
		if st.found {
			tx.markPropertyReadKey(st.id, st.actualName, st.prop)
		} else {
			tx.markPropertyShapeScan(st.id, st.obj)
		}
	}
}

// verbDispatchMemoEntry is one store-level memoized resolution. readTS is the
// snapshot of the txn that computed it; the entry is usable by a txn whose
// snapshot, like this one, postdates the store's last verb-shape change.
type verbDispatchMemoEntry struct {
	readTS  uint64
	found   bool
	definer types.ObjID
	// The shape clock protects definition order; first aliases are not unique.
	index int
	// path lists the objects the producing walk scanned, in order. While the
	// shape clock holds, a walk on any later snapshot scans the same objects,
	// so a writing txn can record exact scan marks without re-matching names.
	path []types.ObjID
}

// lookupVerbDispatchMemo consults the store-level dispatch memo. A hit
// replaces the per-ancestor verb-scan marks with a single txn-level
// dependency on verbShapeChangeTS (validated at commit) plus the usual read
// mark on the resolved verb, which is re-fetched from this txn's view of the
// definer so a concurrent code edit is seen exactly as it would be by a walk.
// err is the "verb not found" error of a negative resolution.
//
// local is the txn's own record of an earlier hit on this key, if any. A verb
// called in a loop used to pay the shared map's lookup and both marks on every
// call; the record carries the memo entry and which of the marks this txn
// already holds, so a repeat hit only re-checks the clock and the definer. The
// record lives in tx.verbResolve, which is dropped wherever a binding in
// tx.objects is replaced or a read mark is removed, so what it says is recorded
// still is.
func (tx *StoreTxn) lookupVerbDispatchMemo(key verbResolveKey, local verbResolveEntry) (verb *Verb, definer types.ObjID, err error, hit bool) {
	s := tx.store
	if s == nil || tx.verbMemoDisabled || tx.liveMutated {
		// A live-mutated txn commits through the coarse path, which validates
		// scan marks, not the clock; it must never hold mark-less resolutions.
		return nil, types.ObjNothing, nil, false
	}
	last := s.verbShapeChangeTS.Load()
	if tx.readTS < last {
		return nil, types.ObjNothing, nil, false
	}
	entry := local.memo
	if entry == nil {
		raw, ok := s.verbMemo().Load(key)
		if !ok {
			return nil, types.ObjNothing, nil, false
		}
		entry = raw.(*verbDispatchMemoEntry)
		local = verbResolveEntry{} // a stale walk record says nothing about entry
	}
	if entry.readTS < last {
		return nil, types.ObjNothing, nil, false
	}
	// A txn that has staged writes will be validated at commit; give it the
	// walk's exact scan marks rather than a dependency on the global shape
	// clock, which every coarse commit advances.
	precise := len(tx.owned) > 0
	need := verbMemoNoted
	if precise {
		need = verbMemoReplayed
	}
	if !entry.found {
		if local.memoSeen < need {
			if !tx.recordVerbMemoHit(key, entry, precise) {
				return nil, types.ObjNothing, nil, false
			}
			if local.err == nil {
				local.err = fmt.Errorf("verb not found: %s", key.name)
			}
			local.memo, local.memoSeen, local.definer = entry, need, types.ObjNothing
			tx.storeVerbMemoUse(key, local)
		}
		return nil, types.ObjNothing, local.err, true
	}
	obj := tx.object(entry.definer)
	if !validLiveObject(obj) {
		return nil, types.ObjNothing, nil, false
	}
	if entry.index < 0 || entry.index >= len(obj.verbList) {
		return nil, types.ObjNothing, nil, false
	}
	verb = obj.verbList[entry.index]
	if verb == nil || (key.requireExecute && !verb.perms.Has(VerbExecute)) {
		return nil, types.ObjNothing, nil, false
	}
	if local.memoSeen < need && !tx.recordVerbMemoHit(key, entry, precise) {
		return nil, types.ObjNothing, nil, false
	}
	// An unowned definer is an immutable image, so the same *Verb yields the
	// same read mark; a private copy can be edited in place and is re-marked.
	marked := verb
	if precise && tx.owned[entry.definer] {
		marked = nil
	}
	if marked == nil || local.verb != marked {
		tx.markVerbRead(entry.definer, verb)
	}
	if local.memoSeen < need || local.verb != marked {
		local.memo, local.verb, local.definer = entry, marked, entry.definer
		local.memoSeen = max(local.memoSeen, need)
		tx.storeVerbMemoUse(key, local)
	}
	return verb, entry.definer, nil, true
}

// storeVerbMemoUse files the txn's record of a dispatch memo hit.
func (tx *StoreTxn) storeVerbMemoUse(key verbResolveKey, use verbResolveEntry) {
	if tx.verbResolve == nil {
		tx.verbResolve = make(map[verbResolveKey]verbResolveEntry)
	} else if len(tx.verbResolve) >= resolveCacheCap {
		if _, present := tx.verbResolve[key]; !present {
			tx.verbResolve = make(map[verbResolveKey]verbResolveEntry, resolveCacheCap)
		}
	}
	tx.verbResolve[key] = use
}

const (
	verbMemoNoted    uint8 = 1 // listed in verbMemoHits
	verbMemoReplayed uint8 = 2 // its walk path's scan marks are recorded
)

// recordVerbMemoHit gives the txn what it must hold for a memo resolution:
// the walk's scan marks when precise, otherwise a place in verbMemoHits. Both
// last for the txn, so each is recorded the first time an entry is used. A verb
// called in a loop used to replay its whole ancestor path, or grow the hit
// list, on every call. It reports false when the path cannot be replayed.
func (tx *StoreTxn) recordVerbMemoHit(key verbResolveKey, entry *verbDispatchMemoEntry, precise bool) bool {
	seen := tx.verbMemoSeen[entry]
	if precise {
		if seen == verbMemoReplayed {
			return true
		}
		if !tx.replayMemoPath(entry.path) {
			return false
		}
		tx.setVerbMemoSeen(entry, verbMemoReplayed)
		return true
	}
	if seen == 0 {
		tx.noteVerbMemoHit(key)
		tx.setVerbMemoSeen(entry, verbMemoNoted)
	}
	return true
}

func (tx *StoreTxn) setVerbMemoSeen(entry *verbDispatchMemoEntry, state uint8) {
	if tx.verbMemoSeen == nil {
		tx.verbMemoSeen = make(map[*verbDispatchMemoEntry]uint8)
	}
	tx.verbMemoSeen[entry] = state
}

// replayMemoPath records a verb-scan mark for every object on a memoized walk
// path. It marks nothing and reports false if any object is no longer valid in
// this txn's view, so the caller can fall back to a real walk.
func (tx *StoreTxn) replayMemoPath(path []types.ObjID) bool {
	for _, id := range path {
		if obj := tx.object(id); obj == nil || obj.recycled {
			return false
		}
	}
	for _, id := range path {
		tx.markVerbScan(id, tx.object(id))
	}
	return true
}

func (tx *StoreTxn) noteVerbMemoHit(key verbResolveKey) {
	tx.usedVerbMemo = true
	tx.verbMemoHits = append(tx.verbMemoHits, key)
}

// PrepareLiveMutation must run before a task's first direct mutation of the
// live store (a staged-topology flush or a coarse builtin). Those mutations
// move verbShapeChangeTS themselves, after which the memo's single clock check
// cannot distinguish the task's own change from a concurrent one and a
// live-mutated task cannot retry. So every resolution taken from the memo is
// re-walked here, on the snapshot the memo hit was equivalent to, into the
// ordinary per-ancestor scan marks that the coarse commit validates under the
// store lock; the memo is then off for the rest of the txn.
func (tx *StoreTxn) PrepareLiveMutation() {
	if tx == nil || tx.direct {
		return
	}
	tx.materializeVerbMemoMarks()
}

func (tx *StoreTxn) materializeVerbMemoMarks() {
	tx.verbMemoDisabled = true
	if !tx.usedVerbMemo {
		return
	}
	hits := tx.verbMemoHits
	tx.verbMemoHits = nil
	tx.verbMemoSeen = nil
	tx.usedVerbMemo = false
	for _, key := range hits {
		tx.walkVerb(key.objID, key.name, key.requireExecute)
	}
}

// storeVerbDispatchMemo publishes a walk's result for other transactions.
// Anonymous objects are never memoized: their ids are recycled constantly and
// invalidating the memo on each would make it useless. A txn that has mutated
// the live store has no clean snapshot to tag the entry with.
func (tx *StoreTxn) storeVerbDispatchMemo(key verbResolveKey, verb *Verb, definer types.ObjID, steps []verbWalkStep) {
	s := tx.store
	if s == nil || tx.liveMutated || tx.verbMemoDisabled {
		return
	}
	if obj := tx.object(key.objID); obj == nil || obj.anonymous {
		return
	}
	entry := &verbDispatchMemoEntry{readTS: tx.readTS, found: verb != nil, definer: definer}
	for _, step := range steps {
		if step.scanned {
			entry.path = append(entry.path, step.id)
		}
	}
	if verb != nil {
		entry.index = -1
		for i, candidate := range tx.object(definer).verbList {
			if candidate == verb {
				entry.index = i
				break
			}
		}
		if entry.index < 0 {
			return
		}
	}
	memo := s.verbMemo()
	if _, loaded := memo.LoadOrStore(key, entry); !loaded {
		if s.verbDispatchMemoSize.Add(1) > verbDispatchMemoCap {
			s.verbDispatchMemo.Store(&sync.Map{})
			s.verbDispatchMemoSize.Store(0)
		}
	} else {
		memo.Store(key, entry)
	}
}

func (tx *StoreTxn) storeVerbResolve(key verbResolveKey, steps []verbWalkStep, verb *Verb, definer types.ObjID, err error) {
	for _, step := range steps {
		if tx.owned[step.id] {
			return
		}
	}
	if tx.verbResolve == nil {
		tx.verbResolve = make(map[verbResolveKey]verbResolveEntry)
	} else if len(tx.verbResolve) >= resolveCacheCap {
		tx.verbResolve = make(map[verbResolveKey]verbResolveEntry, resolveCacheCap)
	}
	tx.verbResolve[key] = verbResolveEntry{
		steps:   append([]verbWalkStep(nil), steps...),
		verb:    verb,
		definer: definer,
		err:     err,
	}
}

func (tx *StoreTxn) storePropResolve(key propResolveKey, steps []propWalkStep, prop Property, name string, ec types.ErrorCode) {
	if tx.propResolve == nil {
		tx.propResolve = make(map[propResolveKey]propResolveEntry)
	} else if len(tx.propResolve) >= resolveCacheCap {
		tx.propResolve = make(map[propResolveKey]propResolveEntry, resolveCacheCap)
	}
	tx.propResolve[key] = propResolveEntry{
		steps: append([]propWalkStep(nil), steps...),
		prop:  prop,
		name:  name,
		ec:    ec,
	}
}

// resolveCacheLenForTest exposes the memo sizes to in-package tests. Verbs
// counts recorded walks; a record of a dispatch memo hit holds no walk.
func (tx *StoreTxn) resolveCacheLenForTest() (verbs, props int) {
	for _, entry := range tx.verbResolve {
		if entry.memo == nil {
			verbs++
		}
	}
	return verbs, len(tx.propResolve)
}
