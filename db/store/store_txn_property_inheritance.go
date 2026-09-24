package store

import (
	"github.com/MongooseMoo/barn/types"
)

func (tx *StoreTxn) HasDuplicateDefinedPropertyAmong(ids []types.ObjID) (bool, types.ErrorCode) {
	if tx.direct {
		return tx.store.hasDuplicateDefinedPropertyAmong(ids)
	}
	seen := make(map[string]bool)
	for _, id := range ids {
		obj := tx.object(id)
		if !validLiveObject(obj) {
			return false, types.E_INVARG
		}
		tx.markPropertyScan(id, obj)
		for name, prop := range obj.properties {
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
	if tx.direct {
		return tx.store.definedPropertyNamesInAncestry(objID)
	}
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
		for name, prop := range current.properties {
			if prop.defined {
				names[propertyNameKey(name)] = true
			}
		}
		queue = append(queue, current.parents...)
	}

	return names
}

func (tx *StoreTxn) HasDefinedPropertyConflictWithAncestry(objID types.ObjID, parentIDs []types.ObjID) (bool, types.ErrorCode) {
	if tx.direct {
		return tx.store.hasDefinedPropertyConflictWithAncestry(objID, parentIDs)
	}
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
	for name, prop := range obj.properties {
		if prop.defined && ancestorNames[propertyNameKey(name)] {
			return true, types.E_NONE
		}
	}
	return false, types.E_NONE
}

func (tx *StoreTxn) HasChparentDescendantPropertyConflict(objID types.ObjID, names map[string]bool) (bool, types.ErrorCode) {
	if tx.direct {
		return tx.store.hasChparentDescendantPropertyConflict(objID, names)
	}
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
			for name, prop := range child.properties {
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

func (tx *StoreTxn) ReseedInheritedProperties(objID types.ObjID) types.ErrorCode {
	if tx.direct {
		return types.E_NONE
	}
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if tx.store == nil {
		return types.E_INVARG
	}

	newProps := tx.copyInheritedProperties(obj.parents)
	for name, prop := range obj.properties {
		if prop.defined {
			newProps[name] = prop
		}
	}
	obj.properties = newProps

	tx.store.mu.RLock()
	live := tx.store.liveObjectLocked(objID)
	if !validLiveObject(live) {
		tx.store.mu.RUnlock()
		return types.E_INVIND
	}
	obj.propertyVersion = live.propertyVersion
	obj.propertyShapeVersion = live.propertyShapeVersion
	liveVersion := live.propertyVersion
	liveShapeVersion := live.propertyShapeVersion
	tx.store.mu.RUnlock()

	tx.propertyScans[objID] = liveVersion
	tx.propertyShapeScans[objID] = liveShapeVersion
	for key := range tx.propertyReads {
		if key.objID == objID {
			delete(tx.propertyReads, key)
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
	for name, prop := range obj.properties {
		if prop.defined {
			continue
		}
		key := propertyWriteKey{objID: objID, name: propertyNameKey(name)}
		lazySet(&tx.propertyWrites, key, propertyWrite{
			name:  name,
			value: prop.value,
			prop:  prop,
		})
	}
	return types.E_NONE
}

func (tx *StoreTxn) copyInheritedProperties(parents []types.ObjID) map[string]Property {
	result := make(map[string]Property)
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
		for name, prop := range current.properties {
			if _, _, exists := propertyByName(result, name); exists {
				continue
			}
			result[name] = Property{
				value:   prop.value,
				owner:   prop.owner,
				perms:   prop.perms,
				clear:   true,
				version: prop.version,
			}
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
			if actualName, existing, ok := propertyByName(child.properties, name); ok {
				tx.markPropertyRead(childID, actualName, existing)
				if existing.defined {
					queue = append(queue, childID)
					continue
				}
				delete(child.properties, actualName)
			} else {
				tx.markPropertyScan(childID, child)
			}
			override := Property{
				value:   prop.value,
				owner:   prop.owner,
				perms:   prop.perms,
				clear:   true,
				defined: false,
			}
			child.properties[propertyNameKey(name)] = override
			key := propertyWriteKey{objID: childID, name: propertyNameKey(name)}
			delete(tx.propertyDeletes, key)
			lazySet(&tx.propertyWrites, key, propertyWrite{
				name:  name,
				value: prop.value,
				prop:  override,
			})
			queue = append(queue, childID)
		}
	}
}

func (tx *StoreTxn) HasDefinedPropertyInDescendants(objID types.ObjID, name string) bool {
	if tx.direct {
		return tx.store.hasDefinedPropertyInDescendants(objID, name)
	}
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
			if actualName, prop, ok := propertyByName(child.properties, name); ok {
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
		return tx.store.deleteDefinedProperty(objID, name)
	}
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	actualName, prop, ok := propertyByName(obj.properties, name)
	if !ok {
		tx.markPropertyScan(objID, obj)
		return types.E_PROPNF
	}
	tx.markPropertyRead(objID, actualName, prop)
	if !prop.defined {
		return types.E_PROPNF
	}

	delete(obj.properties, actualName)
	obj.propOrder = removeString(obj.propOrder, actualName)
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
			if actualName, prop, ok := propertyByName(child.properties, name); ok {
				tx.markPropertyRead(childID, actualName, prop)
				if !prop.defined {
					delete(child.properties, actualName)
					key := propertyWriteKey{objID: childID, name: propertyNameKey(actualName)}
					delete(tx.propertyDefines, key)
					delete(tx.propertyWrites, key)
					delete(tx.propertyDeletes, key)
				}
			} else {
				tx.markPropertyScan(childID, child)
			}
			queue = append(queue, childID)
		}
	}
}
