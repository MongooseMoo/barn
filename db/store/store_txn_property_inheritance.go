package store

import (
	"github.com/MongooseMoo/barn/types"
)

func (tx *StoreTxn) HasDuplicateDefinedPropertyAmong(ids []types.ObjID) (bool, types.ErrorCode) {
	seen := make(map[string]bool)
	for _, id := range ids {
		obj := tx.object(id)
		if !validLiveObject(obj) {
			return false, types.E_INVARG
		}
		tx.markPropertyScan(id, obj)
		for name, prop := range obj.properties.all() {
			if !prop.defined {
				continue
			}
			key := propertyNameKey(name)
			if seen[key] {
				return true, types.E_NONE
			}
			seen[key] = true
		}
	}
	return false, types.E_NONE
}

func (tx *StoreTxn) DefinedPropertyNamesInAncestry(objID types.ObjID) (map[string]bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	return tx.definedPropertyNamesInAncestry([]types.ObjID{objID}), types.E_NONE
}

func (tx *StoreTxn) definedPropertyNamesInAncestry(start []types.ObjID) map[string]bool {
	names := make(map[string]bool)
	visited := make(map[types.ObjID]bool)
	queue := append([]types.ObjID(nil), start...)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] || currentID == types.ObjNothing {
			continue
		}
		visited[currentID] = true

		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markPropertyScan(currentID, current)
		tx.markObjectRelationshipRead(currentID, current)
		for name, prop := range current.properties.all() {
			if prop.defined {
				names[propertyNameKey(name)] = true
			}
		}
		queue = append(queue, current.parents...)
	}

	return names
}

func (tx *StoreTxn) HasDefinedPropertyConflictWithAncestry(objID types.ObjID, parentIDs []types.ObjID) (bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	tx.markPropertyScan(objID, obj)
	for _, parentID := range parentIDs {
		if !validLiveObject(tx.object(parentID)) {
			return false, types.E_INVARG
		}
	}

	ancestorNames := tx.definedPropertyNamesInAncestry(parentIDs)
	for name, prop := range obj.properties.all() {
		if prop.defined && ancestorNames[propertyNameKey(name)] {
			return true, types.E_NONE
		}
	}
	return false, types.E_NONE
}

func (tx *StoreTxn) HasChparentDescendantPropertyConflict(objID types.ObjID, names map[string]bool) (bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}

	visited := make(map[types.ObjID]bool)
	var check func(*Object) bool
	check = func(current *Object) bool {
		if current == nil || visited[current.id] {
			return false
		}
		visited[current.id] = true
		tx.markObjectRelationshipRead(current.id, current)
		for childID := range current.chparentChildren {
			child := tx.object(childID)
			if !validLiveObject(child) {
				continue
			}
			tx.markPropertyScan(childID, child)
			for name, prop := range child.properties.all() {
				if prop.defined && names[propertyNameKey(name)] {
					return true
				}
			}
			if check(child) {
				return true
			}
		}
		return false
	}

	return check(obj), types.E_NONE
}

// ReseedInheritedProperties brings the txn's view of objID, whose parents were
// oldParents, and of every descendant of objID up to the live reseed that
// Store.ChangeParents has done. Each takes its live property table, and the
// slots this txn wrote or deleted are replayed on it where the reseed kept
// them; the rest are dropped. The commit publishes only those.
func (tx *StoreTxn) ReseedInheritedProperties(objID types.ObjID, oldParents []types.ObjID) types.ErrorCode {
	if tx.direct {
		return types.E_NONE
	}
	if tx.store == nil {
		return types.E_INVARG
	}
	tx.invalidateResolveCaches()
	object := func(id types.ObjID) *Object {
		if o := tx.object(id); validLiveObject(o) {
			return o
		}
		return nil
	}
	// The subtree the live reseed rewrote, walked without caching it in the txn.
	tx.store.mu.RLock()
	subtree, inSubtree := inheritanceSubtree(objID, func(id types.ObjID) *Object {
		if o := tx.store.liveObjectLocked(id); validLiveObject(o) {
			return o
		}
		return nil
	})
	tx.store.mu.RUnlock()
	staged := make(map[types.ObjID]*objectWrites)
	at := func(id types.ObjID) *objectWrites {
		if staged[id] == nil {
			staged[id] = &objectWrites{}
		}
		return staged[id]
	}
	for key := range tx.propertyReads {
		if inSubtree[key.objID] {
			delete(tx.propertyReads, key)
		}
	}
	for key := range tx.propertyWrites {
		if inSubtree[key.objID] {
			at(key.objID).props = append(at(key.objID).props, key)
		}
	}
	for key, actualName := range tx.propertyDeletes {
		if inSubtree[key.objID] {
			at(key.objID).propDeletes = append(at(key.objID).propDeletes, actualName)
		}
	}

	for _, id := range subtree {
		tx.store.mu.RLock()
		live := tx.store.liveObjectLocked(id)
		if !validLiveObject(live) {
			tx.store.mu.RUnlock()
			return types.E_INVIND
		}
		obj, cached := tx.objects[id]
		if cached && obj != nil && (tx.owned[id] || live.anonymous || !sameOutsideProperties(obj, live)) {
			// The txn's view of an object it has read keeps its other facets
			// and takes the live slots; its staged writes are replayed below.
			if !tx.owned[id] {
				obj = tx.privatizeCached(id, obj)
			}
			obj.properties = live.properties.clone()
			obj.propertyVersion = live.propertyVersion
			obj.propertyShapeVersion = live.propertyShapeVersion
		} else if live.anonymous {
			// Anonymous images are mutated in place, so a txn reads a clone
			// (objectLocked).
			obj = cloneObjectForReadTxn(live)
			tx.objects[id] = obj
		} else {
			// Published images are immutable, so the txn reads the live one
			// directly, as objectLocked does. A view it had read differs from
			// it only in its properties.
			obj = live
			tx.objects[id] = obj
		}
		tx.store.mu.RUnlock()
		tx.propertyScans[id] = obj.propertyVersion
		tx.propertyShapeScans[id] = obj.propertyShapeVersion

		w := staged[id]
		if w == nil {
			continue
		}
		tx.dropStagedSlotsNotKept(obj, w, objID, oldParents, object)
		// The replay stamps versions a read of a staged slot never records; the
		// view keeps the live ones it adopted (as ApplyStagedObjectWrites does).
		version, shapeVersion := obj.propertyVersion, obj.propertyShapeVersion
		tx.applyTailWrites(obj, w, tx.readTS, true)
		obj.propertyVersion, obj.propertyShapeVersion = version, shapeVersion
	}
	return types.E_NONE
}

// sameOutsideProperties reports whether a and b are the same image of an object
// apart from its properties.
func sameOutsideProperties(a, b *Object) bool {
	return a.scalarVersion == b.scalarVersion && a.relationshipVersion == b.relationshipVersion && a.verbVersion == b.verbVersion
}

// dropStagedSlotsNotKept removes from w, and from the txn, the staged writes and
// deletes of obj's slots that the reseed did not keep: obj keeps the slots it
// defines and those of ancestors in both its old and new ancestry.
func (tx *StoreTxn) dropStagedSlotsNotKept(obj *Object, w *objectWrites, moved types.ObjID, movedOldParents []types.ObjID, object func(types.ObjID) *Object) {
	kept := keptInheritedNames(obj.id, moved, movedOldParents, object)
	keeps := func(key string) bool {
		if prop, ok := obj.properties.lookup(key); ok && prop.defined {
			return true
		}
		return kept[key]
	}
	props := w.props[:0]
	for _, key := range w.props {
		if keeps(key.name) {
			props = append(props, key)
		} else {
			delete(tx.propertyWrites, key)
		}
	}
	w.props = props
	deletes := w.propDeletes[:0]
	for _, actualName := range w.propDeletes {
		key := propertyWriteKey{objID: obj.id, name: propertyNameKey(actualName)}
		if keeps(key.name) {
			deletes = append(deletes, actualName)
		} else {
			delete(tx.propertyDeletes, key)
		}
	}
	w.propDeletes = deletes
}

func (tx *StoreTxn) copyInheritedProperties(parents []types.ObjID, owner types.ObjID) propTable {
	result := newPropTable(0)
	visited := make(map[types.ObjID]bool)
	queue := append([]types.ObjID(nil), parents...)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markPropertyScan(currentID, current)
		for name, prop := range current.properties.all() {
			if _, _, exists := result.find(name); exists {
				continue
			}
			result.put(name, inheritedSlot(prop, owner))
		}
		queue = append(queue, current.parents...)
	}

	return result
}

func (tx *StoreTxn) propagateDefinedProperty(objID types.ObjID, name string, prop Property) {
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markObjectRelationshipRead(currentID, current)
		for _, childID := range current.children {
			// child is mutated in place below (reseed inherited slot), so it must be
			// a txn-private copy, not a shared alias.
			child := tx.mutableObject(childID)
			if !validLiveObject(child) {
				continue
			}
			if actualName, existing, ok := child.properties.find(name); ok {
				tx.markPropertyRead(childID, actualName, existing)
				if existing.defined {
					queue = append(queue, childID)
					continue
				}
				child.properties.remove(actualName)
			} else {
				tx.markPropertyScan(childID, child)
			}
			override := inheritedSlot(prop, child.owner)
			child.properties.put(propertyNameKey(name), override)
			key := propertyWriteKey{objID: childID, name: propertyNameKey(name)}
			delete(tx.propertyDeletes, key)
			lazySet(&tx.propertyWrites, key, propertyWrite{
				name:  name,
				value: override.value,
				prop:  override,
			})
			queue = append(queue, childID)
		}
		// Anonymous children get the slot too (Toast insert_prop_recursively,
		// db_properties.cc:162-166, :176-179). The staged write puts the anon id
		// in the write set, which routes the commit to the exclusive path.
		for _, anonID := range current.anonymousChildren {
			anon := tx.object(anonID)
			if !validLiveObject(anon) {
				continue
			}
			if actualName, existing, ok := anon.properties.find(name); ok {
				tx.markPropertyRead(anonID, actualName, existing)
				if existing.defined {
					continue
				}
				tx.mutableObject(anonID).properties.remove(actualName)
			} else {
				tx.markPropertyScan(anonID, anon)
			}
			override := inheritedSlot(prop, anon.owner)
			tx.mutableObject(anonID).properties.put(propertyNameKey(name), override)
			key := propertyWriteKey{objID: anonID, name: propertyNameKey(name)}
			delete(tx.propertyDeletes, key)
			lazySet(&tx.propertyWrites, key, propertyWrite{
				name:  name,
				value: override.value,
				prop:  override,
			})
		}
	}
}

func (tx *StoreTxn) HasDefinedPropertyInDescendants(objID types.ObjID, name string) bool {
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markObjectRelationshipRead(currentID, current)
		for _, childID := range current.children {
			child := tx.object(childID)
			if !validLiveObject(child) {
				continue
			}
			if actualName, prop, ok := child.properties.find(name); ok {
				tx.markPropertyRead(childID, actualName, prop)
				if prop.defined {
					return true
				}
			} else {
				tx.markPropertyScan(childID, child)
			}
			queue = append(queue, childID)
		}
	}
	return false
}

func (tx *StoreTxn) DeleteDefinedProperty(objID types.ObjID, name string) types.ErrorCode {
	if tx.direct {
		return tx.store.writeNow(func(w *StoreTxn) types.ErrorCode { return w.DeleteDefinedProperty(objID, name) })
	}
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	actualName, prop, ok := obj.properties.find(name)
	if !ok {
		tx.markPropertyScan(objID, obj)
		return types.E_PROPNF
	}
	tx.markPropertyRead(objID, actualName, prop)
	if !prop.defined {
		return types.E_PROPNF
	}

	obj.properties.remove(actualName)
	obj.propOrder = removeStringFold(obj.propOrder, actualName)
	if obj.propDefsCount > 0 {
		obj.propDefsCount--
	}

	key := propertyWriteKey{objID: objID, name: propertyNameKey(actualName)}
	_, stagedDefine := tx.propertyDefines[key]
	delete(tx.propertyDefines, key)
	delete(tx.propertyWrites, key)
	delete(tx.propertyDeletes, key)
	if !stagedDefine {
		lazySet(&tx.propertyDefinitionDeletes, key, actualName)
	}

	tx.removeInheritedProperty(objID, actualName)
	return types.E_NONE
}

func (tx *StoreTxn) removeInheritedProperty(objID types.ObjID, name string) {
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markObjectRelationshipRead(currentID, current)
		for _, childID := range current.children {
			// child.properties is mutated in place below when it holds an inherited
			// (non-defined) slot, so it must be a txn-private copy, not a shared alias.
			child := tx.mutableObject(childID)
			if !validLiveObject(child) {
				continue
			}
			if actualName, prop, ok := child.properties.find(name); ok {
				tx.markPropertyRead(childID, actualName, prop)
				if !prop.defined {
					child.properties.remove(actualName)
					key := propertyWriteKey{objID: childID, name: propertyNameKey(actualName)}
					delete(tx.propertyDefines, key)
					delete(tx.propertyWrites, key)
					// The decentralized commit removes the definition from the definer
					// only; each descendant's slot goes through its own staged delete.
					lazySet(&tx.propertyDeletes, key, actualName)
				}
			} else {
				tx.markPropertyScan(childID, child)
			}
			queue = append(queue, childID)
		}
		// Anonymous children lose the slot too (Toast remove_prop_recursively,
		// db_properties.cc:325-328, :340-341). The staged delete puts the anon id
		// in the write footprint, which routes the commit to the coarse path.
		for _, anonID := range current.anonymousChildren {
			anon := tx.object(anonID)
			if !validLiveObject(anon) {
				continue
			}
			actualName, prop, ok := anon.properties.find(name)
			if !ok {
				tx.markPropertyScan(anonID, anon)
				continue
			}
			tx.markPropertyRead(anonID, actualName, prop)
			if prop.defined {
				continue
			}
			tx.mutableObject(anonID).properties.remove(actualName)
			key := propertyWriteKey{objID: anonID, name: propertyNameKey(actualName)}
			delete(tx.propertyWrites, key)
			lazySet(&tx.propertyDeletes, key, actualName)
		}
	}
}
