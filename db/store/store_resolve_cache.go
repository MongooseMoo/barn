package store

import "github.com/MongooseMoo/barn/types"

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
//   - A StoreTxn is a fixed MVCC snapshot (readTS, store_txn.go BeginReadOnly)
//     and lives for a whole task slice (scheduler/task_runtime.go:115 begins one
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
// CORRECTNESS: staged writes bypass the memo. The memo is live only while
// len(tx.owned) == 0 — i.e. while the transaction has not privatized a single
// object. Every staging path (SetPropertyValue, DefineProperty, SetVerbCode,
// CreateObject, MoveObject, RecycleObject, ...) goes through
// mutableObject/privatizeCached first, which marks the object owned, and
// `owned` only ever grows within a transaction. So the first staged write
// disables the memo for the remainder of the transaction and its own writes are
// always read back by a real walk. (The single exception is
// FlushStagedToLive, which publishes the staged writes, re-clones every cached
// object from current live and resets tx.owned; it invalidates the memo
// explicitly, and the fresh clones it installs are unowned, so nothing can be
// mutated in place without a new privatizeCached.) The gate also guarantees no
// memoized entry can ever reference a
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

// resolveCacheActive reports whether the resolution memo may be read or
// written. See the staged-write argument in this file's header: a transaction
// that has privatized any object (i.e. staged any write) never uses the memo
// again.
func (tx *StoreTxn) resolveCacheActive() bool {
	return len(tx.owned) == 0
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
// walk visited is still the identical *Object pointer.
func (tx *StoreTxn) verbStepsCurrent(steps []verbWalkStep) bool {
	for i := range steps {
		if tx.objects[steps[i].id] != steps[i].obj {
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
			tx.markPropertyRead(st.id, st.actualName, st.prop)
		} else {
			tx.markPropertyScan(st.id, st.obj)
		}
	}
}

func (tx *StoreTxn) storeVerbResolve(key verbResolveKey, steps []verbWalkStep, verb *Verb, definer types.ObjID, err error) {
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

// resolveCacheLenForTest exposes the memo sizes to in-package tests.
func (tx *StoreTxn) resolveCacheLenForTest() (verbs, props int) {
	return len(tx.verbResolve), len(tx.propResolve)
}
