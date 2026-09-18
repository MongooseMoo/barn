package store

import (
	"github.com/MongooseMoo/barn/types"
)

func (tx *StoreTxn) object(objID types.ObjID) *Object {
	if obj, ok := tx.objects[objID]; ok {
		return obj
	}
	if tx.store == nil {
		tx.objects[objID] = nil
		return nil
	}

	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	obj := tx.objectLocked(objID)
	tx.objects[objID] = obj
	return obj
}

// mutableObject returns a txn-PRIVATE, in-place-mutable copy of the cached object,
// materializing one (copy-on-write) if the cache currently holds a shared alias.
// Every staging method that mutates the cached *Object in place (name/owner/flags/
// location, the properties map/propOrder, the verbs map/list, chparentChildren, and
// the adopt-refresh paths) must obtain the object through this — never through
// object() — so the aliased published image is never written. Idempotent: once an
// object is owned, repeat writes reuse the same private copy.
//
// While object() still deep-clones on read (pre-flip), this is a harmless second
// copy; once object() aliases the immutable image, this becomes the sole copy point.
func (tx *StoreTxn) mutableObject(objID types.ObjID) *Object {
	obj := tx.object(objID)
	if obj == nil {
		return nil
	}
	if tx.owned[objID] {
		return obj
	}
	return tx.privatizeCached(objID, obj)
}

// privatizeCached installs a txn-private clone of base as the cache entry for
// objID and marks it owned. It takes NO store lock, so callers already holding
// store.mu (e.g. AdoptLiveRelationships) can use it directly without nesting the
// RLock. base is the object whose non-refreshed facets are preserved into the
// private copy (the current cached alias, or the live image when nothing is
// cached yet).
func (tx *StoreTxn) privatizeCached(objID types.ObjID, base *Object) *Object {
	// The txn's binding for objID changes and becomes in-place mutable; every
	// memoized resolution that walked it is now unsafe to replay. (This also
	// permanently disables the memo, since `owned` never shrinks — see
	// resolveCacheActive.)
	tx.invalidateResolveCaches()
	clone := cloneObjectForReadTxn(base)
	tx.objects[objID] = clone
	if tx.owned == nil {
		tx.owned = make(map[types.ObjID]bool)
	}
	tx.owned[objID] = true
	return clone
}

func (tx *StoreTxn) objectLocked(objID types.ObjID) *Object {
	// Phase 2 read aliasing: numbered published images (and their history entries)
	// are IMMUTABLE after publish — every runtime mutation goes through
	// republishForMutation, which supersedes the slot with a fresh image and never
	// writes the old one. So a read transaction can ALIAS the image pointer directly
	// instead of deep-cloning the whole object (properties + verbs + code) on every
	// first touch. The txn-local mutable copy is created only on the first STAGED
	// WRITE to the object (mutableObject/privatizeCached, true copy-on-write).
	live := tx.store.load(objID)
	if live != nil && objectVersion(live) <= tx.readTS {
		return live
	}
	// Anonymous objects are the exception: they live out-of-band with no COW slot
	// and are mutated IN PLACE (republishForMutation returns them unchanged), so a
	// reader must still deep-clone them. They are rare, so this costs little.
	if anon := tx.store.anonObjects[objID]; anon != nil && objectVersion(anon) <= tx.readTS {
		return cloneObjectForReadTxn(anon)
	}

	// The history slice header is read under historyMu: a decentralized COW
	// committer (holding only store.mu.RLock, which does not exclude this reader's
	// RLock) appends to s.history under historyMu. Capturing the slice header here
	// is enough — append never mutates the existing entries the walk reads, and the
	// committer reassigns the map value to a (possibly new) header, so the captured
	// header is a stable snapshot. The clone below runs outside the lock.
	tx.store.historyMu.Lock()
	history := tx.store.history[objID]
	tx.store.historyMu.Unlock()
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].ts <= tx.readTS {
			// History entries are superseded published images (numbered objects only —
			// anon carry no history) and are immutable, so alias them too.
			return history[i].obj
		}
	}

	return nil
}

func (tx *StoreTxn) markObjectScalarRead(objID types.ObjID, obj *Object) {
	if tx == nil || obj == nil {
		return
	}
	if _, exists := tx.scalarReads[objID]; exists {
		return
	}
	tx.scalarReads[objID] = obj.scalarVersion
}

func (tx *StoreTxn) markObjectRelationshipRead(objID types.ObjID, obj *Object) {
	if tx == nil || obj == nil {
		return
	}
	if _, exists := tx.relationshipReads[objID]; exists {
		return
	}
	tx.relationshipReads[objID] = obj.relationshipVersion
}

func (tx *StoreTxn) markPropertyRead(objID types.ObjID, name string, prop Property) {
	if tx == nil {
		return
	}
	tx.markPropertyReadKey(objID, propertyNameKey(name), prop)
}

// markPropertyReadKey is markPropertyRead for a name already in canonical
// (lowercase) key form — the map key returned by a properties lookup — so the
// hot read path lowers a name once per resolution instead of once per mark.
func (tx *StoreTxn) markPropertyReadKey(objID types.ObjID, key string, prop Property) {
	if tx == nil {
		return
	}
	wkey := propertyWriteKey{objID: objID, name: key}
	if _, staged := tx.propertyDefines[wkey]; staged {
		return
	}
	if _, staged := tx.propertyWrites[wkey]; staged {
		return
	}
	tx.propertyReads[propertyReadKey{objID: objID, name: key}] = prop.version
}

func (tx *StoreTxn) markPropertyScan(objID types.ObjID, obj *Object) {
	if tx == nil || obj == nil {
		return
	}
	tx.propertyScans[objID] = obj.propertyVersion
}

// markPropertyShapeScan records that this txn's result depends on which slots
// obj HAS (an ancestry walk fell through it), not on any slot's value. It is
// validated against propertyShapeVersion, which value writes do not move.
func (tx *StoreTxn) markPropertyShapeScan(objID types.ObjID, obj *Object) {
	if tx == nil || obj == nil {
		return
	}
	tx.propertyShapeScans[objID] = obj.propertyShapeVersion
}

func (tx *StoreTxn) markVerbRead(objID types.ObjID, verb *Verb) {
	if tx == nil || verb == nil {
		return
	}
	if _, staged := tx.verbWrites[verbWriteKey{objID: objID, name: verb.mapKey()}]; staged {
		return
	}
	tx.verbReads[verbReadKey{objID: objID, name: verb.mapKey()}] = verb.version
}

func (tx *StoreTxn) markVerbScan(objID types.ObjID, obj *Object) {
	if tx == nil || obj == nil {
		return
	}
	if _, exists := tx.verbScans[objID]; exists {
		return
	}
	tx.verbScans[objID] = obj.verbVersion
}

func cloneObjectForReadTxn(obj *Object) *Object {
	if obj == nil {
		return nil
	}
	clone := *obj
	clone.parents = append([]types.ObjID(nil), obj.parents...)
	clone.children = append([]types.ObjID(nil), obj.children...)
	clone.contents = append([]types.ObjID(nil), obj.contents...)
	clone.propOrder = append([]string(nil), obj.propOrder...)
	clone.anonymousChildren = append([]types.ObjID(nil), obj.anonymousChildren...)

	clone.properties = make(map[string]Property, len(obj.properties))
	for name, prop := range obj.properties {
		clone.properties[name] = prop
	}

	verbClones := make(map[*Verb]*Verb, len(obj.verbList))
	clone.verbList = make([]*Verb, 0, len(obj.verbList))
	for _, verb := range obj.verbList {
		verbClone := cloneVerbForReadTxn(verb)
		verbClones[verb] = verbClone
		clone.verbList = append(clone.verbList, verbClone)
	}
	clone.verbs = make(map[string]*Verb, len(obj.verbs))
	for name, verb := range obj.verbs {
		if verbClone, ok := verbClones[verb]; ok {
			clone.verbs[name] = verbClone
			continue
		}
		clone.verbs[name] = cloneVerbForReadTxn(verb)
	}

	clone.chparentChildren = make(map[types.ObjID]bool, len(obj.chparentChildren))
	for id, tracked := range obj.chparentChildren {
		clone.chparentChildren[id] = tracked
	}
	return &clone
}

func cloneVerbForReadTxn(verb *Verb) *Verb {
	if verb == nil {
		return nil
	}
	clone := *verb
	clone.names = append([]string(nil), verb.names...)
	// lowerNames is immutable once built (renames build a fresh slice), so the
	// clone can share the backing array instead of copying.
	clone.lowerNames = verb.lowerNames
	// codeKey rides along in the struct copy: the clone's code is byte-identical
	// to the original's, so the key still describes it. A later write to the
	// clone goes through setCodeOwned/setCodeCopy, which refresh both together.
	clone.code = append([]string(nil), verb.code...)
	return &clone
}
