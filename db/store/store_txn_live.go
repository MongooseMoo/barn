package store

import (
	"strings"

	"github.com/MongooseMoo/barn/types"
)

// MarkLiveMutated records that the owning task has mutated the live Store directly,
// outside this transaction (create/recycle/chparent/move/add_verb/...). Callers adopt
// the specific live object facets changed by their own mutation; unrelated read-set
// versions must remain at the original snapshot so concurrent changes still conflict.
func (tx *StoreTxn) MarkLiveMutated() {
	if tx != nil && !tx.direct {
		// The live op has already moved verbShapeChangeTS, so the memo's clock
		// check can no longer tell this txn's own change from a concurrent one.
		// Callers reach PrepareLiveMutation first; this is the safety net.
		tx.materializeVerbMemoMarks()
		tx.liveMutated = true
		// The task mutated the store outside this txn; anything memoized from
		// the pre-mutation view must not be replayed.
		tx.invalidateResolveCaches()
	}
}

func (tx *StoreTxn) AdoptLiveObject(objID types.ObjID) types.ErrorCode {
	if tx == nil || tx.direct {
		return types.E_NONE
	}
	// Replaces the txn's binding for objID without marking it owned.
	tx.invalidateResolveCaches()
	if tx.store == nil {
		tx.objects[objID] = nil
		return types.E_INVIND
	}
	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	// liveObjectLocked falls back to s.anonObjects, so a freshly-created anonymous
	// object (create(parent, 1) -> this) is adopted, not just numbered objects.
	live := tx.store.liveObjectLocked(objID)
	if !validLiveObject(live) {
		tx.objects[objID] = nil
		return types.E_INVIND
	}
	tx.objects[objID] = cloneObjectForReadTxn(live)
	// Anonymous objects do not participate in max_object() (CreateObject /
	// insertObjectLocked bump only highWaterID for anon); mirror that here.
	if !live.anonymous && objID > tx.maxObjID {
		tx.maxObjID = objID
	}
	if objID > tx.highWaterID {
		tx.highWaterID = objID
	}
	return types.E_NONE
}

func (tx *StoreTxn) AdoptLiveVerbs(objID types.ObjID) types.ErrorCode {
	if tx == nil || tx.direct {
		return types.E_NONE
	}
	tx.invalidateResolveCaches()
	// obj.verbList/obj.verbs are rebuilt in place below, so obj must be a txn-private
	// copy. mutableObject is called BEFORE the store.mu.RLock, so its own RLock does
	// not nest.
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if tx.store == nil {
		return types.E_INVARG
	}

	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	// Resolve through liveObjectLocked for symmetry with the other tx resolvers so
	// an anonymous definer is found out-of-band (anon carry no local verbList, so
	// this is defensive — add_verb on an anon is rejected at the builtin).
	live := tx.store.liveObjectLocked(objID)
	if !validLiveObject(live) {
		tx.objects[objID] = nil
		return types.E_INVIND
	}

	verbClones := make(map[*Verb]*Verb, len(live.verbList))
	obj.verbList = make([]*Verb, 0, len(live.verbList))
	for _, verb := range live.verbList {
		verbClone := cloneVerbForReadTxn(verb)
		verbClones[verb] = verbClone
		obj.verbList = append(obj.verbList, verbClone)
	}
	obj.verbs = make(map[string]*Verb, len(live.verbs))
	for name, verb := range live.verbs {
		if verbClone, ok := verbClones[verb]; ok {
			obj.verbs[name] = verbClone
			continue
		}
		obj.verbs[name] = cloneVerbForReadTxn(verb)
	}
	for key, write := range tx.verbWrites {
		if key.objID != objID {
			continue
		}
		verb := obj.verbs[key.name]
		if verb == nil {
			continue
		}
		verb.setCodeCopy(write.code)
	}
	obj.verbIdx = live.verbIdx // same aliases in the same order as live
	obj.verbVersion = live.verbVersion
	tx.verbScans[objID] = live.verbVersion
	for key := range tx.verbReads {
		if key.objID != objID {
			continue
		}
		if verb := live.verbs[key.name]; verb != nil {
			tx.verbReads[key] = verb.version
			continue
		}
		delete(tx.verbReads, key)
	}
	return types.E_NONE
}

func (tx *StoreTxn) AdoptLiveRelationships(objIDs ...types.ObjID) types.ErrorCode {
	if tx == nil || tx.direct {
		return types.E_NONE
	}
	tx.invalidateResolveCaches()
	if tx.store == nil {
		return types.E_INVARG
	}

	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	for _, objID := range objIDs {
		if objID == types.ObjNothing {
			continue
		}
		// Resolve through liveObjectLocked so an anonymous relative (which lives
		// out-of-band in s.anonObjects, not the numbered slot map) is adopted, not
		// just numbered objects. This mirrors objectLocked's anonymous resolution
		// and the non-tx liveObjectLocked path.
		live := tx.store.liveObjectLocked(objID)
		if live == nil {
			tx.objects[objID] = nil
			return types.E_INVIND
		}
		// obj's relationship facets are overwritten in place below. It must be a
		// txn-private copy: clone from live when nothing is cached, else privatize the
		// cached alias. privatizeCached is lock-free, so it is safe under the RLock
		// held above (mutableObject must NOT be used here — it would nest the RLock).
		obj := tx.objects[objID]
		if obj == nil {
			obj = tx.privatizeCached(objID, live)
		} else if !tx.owned[objID] {
			obj = tx.privatizeCached(objID, obj)
		}
		obj.location = live.location
		obj.parents = append([]types.ObjID(nil), live.parents...)
		obj.children = append([]types.ObjID(nil), live.children...)
		obj.contents = append([]types.ObjID(nil), live.contents...)
		obj.anonymousChildren = append([]types.ObjID(nil), live.anonymousChildren...)
		obj.chparentChildren = make(map[types.ObjID]bool, len(live.chparentChildren))
		for id, tracked := range live.chparentChildren {
			obj.chparentChildren[id] = tracked
		}
		obj.relationshipVersion = live.relationshipVersion
		tx.relationshipReads[objID] = live.relationshipVersion
	}
	return types.E_NONE
}

func (tx *StoreTxn) ForgetObject(objID types.ObjID) {
	if tx == nil || tx.direct {
		return
	}
	// Rebinds tx.objects[objID] and drops read marks without marking it owned.
	tx.invalidateResolveCaches()
	tx.objects[objID] = nil
	delete(tx.scalarReads, objID)
	delete(tx.scalarWrites, objID)
	delete(tx.relationshipReads, objID)
	delete(tx.relationshipWrites, objID)
	delete(tx.propertyScans, objID)
	delete(tx.propertyShapeScans, objID)
	delete(tx.verbScans, objID)
	for key := range tx.propertyReads {
		if key.objID == objID {
			delete(tx.propertyReads, key)
		}
	}
	for key := range tx.propertyDefines {
		if key.objID == objID {
			delete(tx.propertyDefines, key)
		}
	}
	for key := range tx.propertyDefinitionDeletes {
		if key.objID == objID {
			delete(tx.propertyDefinitionDeletes, key)
		}
	}
	for key := range tx.propertyWrites {
		if key.objID == objID {
			delete(tx.propertyWrites, key)
		}
	}
	for key := range tx.propertyDeletes {
		if key.objID == objID {
			delete(tx.propertyDeletes, key)
		}
	}
	for key := range tx.verbReads {
		if key.objID == objID {
			delete(tx.verbReads, key)
		}
	}
	for key := range tx.verbWrites {
		if key.objID == objID {
			delete(tx.verbWrites, key)
		}
	}
	keptDeletes := tx.verbDeletes[:0]
	for _, deletion := range tx.verbDeletes {
		if deletion.objID != objID {
			keptDeletes = append(keptDeletes, deletion)
		}
	}
	tx.verbDeletes = keptDeletes
}

func (tx *StoreTxn) MoveStagedProperties(oldID, newID types.ObjID) {
	if tx == nil || tx.direct || oldID == newID {
		return
	}
	tx.invalidateResolveCaches()
	for key, prop := range tx.propertyDefines {
		if key.objID != oldID {
			continue
		}
		delete(tx.propertyDefines, key)
		key.objID = newID
		lazySet(&tx.propertyDefines, key, prop)
	}
	for key, actualName := range tx.propertyDefinitionDeletes {
		if key.objID != oldID {
			continue
		}
		delete(tx.propertyDefinitionDeletes, key)
		key.objID = newID
		lazySet(&tx.propertyDefinitionDeletes, key, actualName)
	}
	for key, write := range tx.propertyWrites {
		if key.objID != oldID {
			continue
		}
		delete(tx.propertyWrites, key)
		key.objID = newID
		lazySet(&tx.propertyWrites, key, write)
	}
	for key, actualName := range tx.propertyDeletes {
		if key.objID != oldID {
			continue
		}
		delete(tx.propertyDeletes, key)
		key.objID = newID
		lazySet(&tx.propertyDeletes, key, actualName)
	}
}

func (tx *StoreTxn) ApplyStagedProperties(objID types.ObjID) {
	if tx == nil || tx.direct {
		return
	}
	tx.invalidateResolveCaches()
	obj := tx.objects[objID]
	if !validLiveObject(obj) {
		return
	}
	// obj.properties/propOrder are mutated in place below; privatize first so an
	// aliased shared image is never written. Lock-free: no store.mu held here.
	if !tx.owned[objID] {
		obj = tx.privatizeCached(objID, obj)
	}
	for key, def := range tx.propertyDefines {
		if key.objID != objID {
			continue
		}
		if actualName, _, ok := propertyByName(obj.properties, def.name); ok {
			delete(obj.properties, actualName)
		}
		obj.properties[propertyNameKey(def.name)] = def.prop
		foundOrder := false
		for _, name := range obj.propOrder {
			if strings.EqualFold(name, def.name) {
				foundOrder = true
				break
			}
		}
		if !foundOrder {
			pos := obj.propDefsCount
			if pos > len(obj.propOrder) {
				pos = len(obj.propOrder)
			}
			obj.propOrder = append(obj.propOrder, "")
			copy(obj.propOrder[pos+1:], obj.propOrder[pos:])
			obj.propOrder[pos] = def.name
			obj.propDefsCount++
		}
	}
	for key, write := range tx.propertyWrites {
		if key.objID != objID {
			continue
		}
		obj.properties[propertyNameKey(write.name)] = write.prop
	}
	for key, actualName := range tx.propertyDefinitionDeletes {
		if key.objID != objID {
			continue
		}
		if liveActual, _, ok := propertyByName(obj.properties, actualName); ok {
			delete(obj.properties, liveActual)
		}
	}
	for key, actualName := range tx.propertyDeletes {
		if key.objID != objID {
			continue
		}
		if liveActual, _, ok := propertyByName(obj.properties, actualName); ok {
			delete(obj.properties, liveActual)
		}
	}
}

// FlushStagedToLive applies this txn's staged decentralized writes to the LIVE store
// immediately and clears them, so a subsequent COARSE builtin that reads/mutates the
// live store mid-task (renumber/chparent/add_verb) sees them instead of stale live
// state. It also drops the object read set: the task has now mutated the live store, so it is
// non-isolated (Toast-like) and its eventual coarse commit must not conflict on reads
// taken against the pre-flush snapshot. Transactions with WAIF dependencies first
// validate their complete read set; other coarse flushes retain their established
// immediate-mutation semantics. WAIF dependencies survive with own writes rebased.
// The complete operation
// footprint is still preflighted before any publication, including allocated-id
// occupancy. Exact verb deletion is rejected because it must cross a validating
// CommitAndRenew boundary. No-op if nothing is staged.
func (tx *StoreTxn) FlushStagedToLive() types.ErrorCode {
	if tx == nil || tx.direct {
		return types.E_NONE
	}
	if tx.terminalErr != types.E_NONE {
		return tx.terminalErr
	}
	if !tx.hasStagedWrites() {
		return types.E_NONE
	}
	if len(tx.verbDeletes) > 0 {
		// Exact verb deletion depends on scalar and ordered-list generations.
		// Its coarse boundary must use CommitAndRenew so the ordinary commit path
		// validates the complete read set before publishing anything.
		return tx.markTerminal(types.E_INVARG)
	}
	tx.validationFail = false
	tx.store.mu.Lock()
	if len(tx.waifs) != 0 {
		if errCode := tx.validateReadsLocked(); errCode != types.E_NONE {
			tx.validationFail = true
			tx.store.mu.Unlock()
			return errCode
		}
	}
	if errCode := tx.preflightStagedToLiveLocked(); errCode != types.E_NONE {
		tx.store.mu.Unlock()
		return tx.markTerminal(errCode)
	}
	// Only a successful preflight may discard the old memo/view bookkeeping.
	// A failure must leave the task's complete private view available to its
	// builtin error handler.
	tx.invalidateResolveCaches()
	recycledByFlush := make(map[types.ObjID]bool, len(tx.recycleWrites))
	for id := range tx.recycleWrites {
		recycledByFlush[id] = true
	}
	ec := tx.applyStagedToLiveLocked()
	tx.store.mu.Unlock()
	if ec != types.E_NONE {
		return tx.markTerminal(ec)
	}

	// Drop the pre-flush reads but keep the maps allocated: the coarse builtin that
	// triggered the flush still records reads afterward (markVerbScan, etc.), so niling
	// them would panic on the next read.
	tx.scalarReads = make(map[types.ObjID]uint64)
	tx.relationshipReads = make(map[types.ObjID]uint64)
	tx.propertyReads = make(map[propertyReadKey]uint64)
	tx.propertyScans = make(map[types.ObjID]uint64)
	tx.propertyShapeScans = make(map[types.ObjID]uint64)
	tx.verbReads = make(map[verbReadKey]uint64)
	tx.verbScans = make(map[types.ObjID]uint64)

	// Drop the object cache too: the flush advanced live (a staged property define bumped
	// the definer's version), but the cache still holds the pre-flush images. A post-flush
	// read served from the stale cache would record the OLD version, and the eventual coarse
	// commit would then validate that stale read against advanced live and self-conflict
	// (E_INVARG) forever. Re-fetching from live keeps post-flush reads coherent with the
	// state the task just installed. Staged writes are already applied, so no private
	// mutation is lost; tx.owned resets with it.
	// Refresh the object cache from CURRENT live (NOT the readTS snapshot). The flush
	// advanced live past this txn's readTS: a staged create/define is published at the
	// flush timestamp, which exceeds readTS. Two failures follow if the cache is left as
	// is or merely cleared:
	//   - Left as is, a post-flush read is served the pre-flush image and records the OLD
	//     version; the coarse commit then validates that stale read against advanced live
	//     and self-conflicts (E_INVARG) forever.
	//   - Cleared, a post-flush read re-resolves through objectLocked's readTS gate
	//     (objectVersion(live) <= readTS), which now MISSES the just-created object — so a
	//     freshly-created object reads back as invalid mid-task (E_INVIND).
	// Re-cloning each cached object from current live fixes both: created/written objects
	// stay visible at their new version, non-flushed objects re-clone at their unchanged
	// version. The task is already liveMutated (non-isolated), so seeing live as of the
	// flush is the correct Toast-like semantics. Staged writes are already applied, so no
	// private mutation is lost; owned resets with the fresh (unowned) copies.
	tx.store.mu.RLock()
	for id, cached := range tx.objects {
		// Anonymous objects live out-of-band in s.anonObjects with NO numbered slot, so
		// s.load can't resolve them (it walks the numbered directory only). They are also
		// never part of a decentralized flush — the whole decentralized path routes any
		// anon-touching commit onto the coarse exclusive path (writeFootprintHasAnon), so
		// the flush never republished them and their cached snapshot is not stale on its
		// account. Leave anon entries untouched; refreshing via s.load would wrongly drop
		// them and make a live anonymous object read as invalid mid-task.
		if cached != nil && cached.anonymous {
			continue
		}
		if live := tx.store.load(id); live != nil {
			// A recycled tombstone is cached as such on purpose: dropping the entry
			// would let the next read re-resolve through the readTS gate, which
			// still sees the object as it was before this flush recycled it, and a
			// coarse builtin that then treats the resurrected object as valid dies
			// with E_INVIND deep inside its own reads instead of returning E_INVARG.
			tx.objects[id] = cloneObjectForReadTxn(live)
		} else {
			delete(tx.objects, id)
		}
	}
	for id := range recycledByFlush {
		if _, cached := tx.objects[id]; cached {
			continue
		}
		if live := tx.store.load(id); live != nil {
			tx.objects[id] = cloneObjectForReadTxn(live)
		}
	}
	tx.store.mu.RUnlock()
	tx.owned = make(map[types.ObjID]bool)
	return types.E_NONE
}
