package store

import (
	"strings"

	"github.com/MongooseMoo/barn/types"
)

type propertyReadKey struct {
	objID types.ObjID
	name  string
}

type propertyWriteKey struct {
	objID types.ObjID
	name  string
}

type propertyWrite struct {
	// name is the original-case property name. The Property value no longer
	// carries its own name (it is stored keyed by name in the object's map),
	// and the propertyWriteKey carries only the lowercased match key, so the
	// original-case name is threaded here for storage/propOrder insertion.
	name  string
	value types.Value
	prop  Property
	// orig is the slot as this txn first saw it before staging any write to
	// it (hasOrig=false when the slot did not exist on this object, i.e. the
	// write creates an override of an inherited property). A later write
	// that restores an Identical value to a non-clear orig is dropped again:
	// e.g. `x = setadd(x, "The"); x = setremove(x, "the")` nets to no change
	// and must not publish a version bump (see SetPropertyValue).
	orig    Property
	hasOrig bool
}

// propertyDefine is a staged property DEFINITION carrying the original-case name
// alongside the property value. Like propertyWrite, it exists because Property no
// longer embeds its own name and the write key is lowercased.
type propertyDefine struct {
	name string
	prop Property
}

// stagePropertyValue stages value for objID.name. before is the slot as it was
// on the txn's object view immediately before this write (hasBefore=false when
// the write creates a new override slot). The first staging of a key remembers
// before as the write's orig; if the staged value is Identical to a non-clear
// orig the net effect of the txn on that slot is nil and the write is dropped.
func (tx *StoreTxn) stagePropertyValue(objID types.ObjID, name string, prop Property, value types.Value, before Property, hasBefore bool) {
	prop.value = value
	prop.clear = false
	key := propertyWriteKey{objID: objID, name: propertyNameKey(name)}
	delete(tx.propertyDeletes, key)
	if _, stagedDefine := tx.propertyDefines[key]; stagedDefine {
		lazySet(&tx.propertyDefines, key, propertyDefine{name: name, prop: prop})
		return
	}
	orig, hasOrig := before, hasBefore
	if existing, staged := tx.propertyWrites[key]; staged {
		orig, hasOrig = existing.orig, existing.hasOrig
	}
	if hasOrig && !orig.clear && orig.value.Identical(value) {
		// Net no-op for this slot: the read mark recorded by the caller keeps
		// the dependency on orig; nothing to publish.
		delete(tx.propertyWrites, key)
		if tx.store != nil {
			tx.store.propertyWriteElisions.Add(1)
		}
		return
	}
	lazySet(&tx.propertyWrites, key, propertyWrite{
		name:    name,
		value:   value,
		prop:    prop,
		orig:    orig,
		hasOrig: hasOrig,
	})
}

func (tx *StoreTxn) FindProperty(objID types.ObjID, name string) (PropertyView, types.ErrorCode) {
	if tx.direct {
		return tx.store.findProperty(objID, name)
	}
	prop, actualName, errCode := tx.findProperty(objID, name)
	if errCode != types.E_NONE {
		return PropertyView{}, errCode
	}
	return prop.View(actualName), types.E_NONE
}

func (tx *StoreTxn) findProperty(objID types.ObjID, name string) (Property, string, types.ErrorCode) {
	cacheable := tx.resolveCacheActive()
	key := propResolveKey{objID: objID, name: name}
	if cacheable {
		if entry, ok := tx.propResolve[key]; ok && tx.propStepsCurrent(entry.steps) {
			tx.replayPropSteps(entry.steps)
			return entry.prop, entry.name, entry.ec
		}
	}

	prop, actualName, ec, steps := tx.walkProperty(objID, name)
	if cacheable {
		tx.storePropResolve(key, steps, prop, actualName, ec)
	}
	return prop, actualName, ec
}

// walkProperty is the ancestry BFS behind findProperty. It returns the
// resolution plus the ordered record of every object it visited (which the memo
// stores so a later hit can reproduce the identical read set). The scratch it
// walks on is reused across calls; a (currently impossible) reentrant call
// falls back to a private scratch rather than corrupting the outer walk.
func (tx *StoreTxn) walkProperty(objID types.ObjID, name string) (Property, string, types.ErrorCode, []propWalkStep) {
	sc := &tx.propWalk
	if sc.inUse {
		sc = &propScratch{}
	}
	sc.inUse = true
	sc.visited.reset()
	sc.steps = sc.steps[:0]
	queue := append(sc.queue[:0], objID)

	var targetProp Property
	var targetName string
	haveTarget := false

	resultProp := Property{}
	resultName := ""
	resultErr := types.E_PROPNF
	// Object.properties is keyed by the canonical lowercase name, so lower the
	// lookup once here rather than once per ancestor (an E_PROPNF walk on a
	// deep chain otherwise re-lowers the same name at every level).
	key := propertyNameKey(name)

	for head := 0; head < len(queue); head++ {
		currentID := queue[head]
		if !sc.visited.add(currentID) {
			continue
		}

		current := tx.object(currentID)
		if !validLiveObject(current) {
			sc.steps = append(sc.steps, propWalkStep{id: currentID, obj: current})
			continue
		}

		if prop, ok := current.properties[key]; ok {
			actualName := key
			sc.steps = append(sc.steps, propWalkStep{
				id: currentID, obj: current, valid: true,
				found: true, actualName: actualName, prop: prop,
			})
			tx.markPropertyReadKey(currentID, actualName, prop)
			firstFound := !haveTarget
			if !haveTarget {
				targetProp = prop
				targetName = actualName
				haveTarget = true
			}
			if !prop.clear {
				if !firstFound {
					result := targetProp
					result.value = prop.value
					result.clear = false
					resultProp, resultName, resultErr = result, targetName, types.E_NONE
				} else {
					resultProp, resultName, resultErr = prop, actualName, types.E_NONE
				}
				break
			}
		} else {
			sc.steps = append(sc.steps, propWalkStep{id: currentID, obj: current, valid: true})
			tx.markPropertyShapeScan(currentID, current)
		}
		queue = append(queue, current.parents...)
	}

	sc.queue = queue[:0]
	sc.inUse = false
	return resultProp, resultName, resultErr, sc.steps
}

func (tx *StoreTxn) PropertyValue(objID types.ObjID, name string) (types.Value, types.ErrorCode) {
	if tx.direct {
		return tx.store.propertyValue(objID, name)
	}
	prop, errCode := tx.FindProperty(objID, name)
	if errCode != types.E_NONE {
		return types.None, errCode
	}
	return prop.Value, types.E_NONE
}

func (tx *StoreTxn) PropertyValues(objID types.ObjID) ([]types.Value, types.ErrorCode) {
	if tx.direct {
		return tx.store.propertyValues(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markPropertyScan(objID, obj)

	values := make([]types.Value, 0, len(obj.properties))
	for pname, prop := range obj.properties {
		tx.markPropertyRead(objID, pname, prop)
		values = append(values, prop.value)
	}
	return values, types.E_NONE
}

func (tx *StoreTxn) LocalProperty(objID types.ObjID, name string) (PropertyView, bool, types.ErrorCode) {
	if tx.direct {
		return tx.store.localProperty(objID, name)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return PropertyView{}, false, types.E_INVIND
	}
	actualName, prop, ok := propertyByName(obj.properties, name)
	if !ok {
		tx.markPropertyScan(objID, obj)
		return PropertyView{}, false, types.E_NONE
	}
	tx.markPropertyRead(objID, actualName, prop)
	return prop.View(actualName), true, types.E_NONE
}

func (tx *StoreTxn) DefinedPropertyNames(objID types.ObjID) ([]string, types.ErrorCode) {
	if tx.direct {
		return tx.store.definedPropertyNames(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markPropertyScan(objID, obj)

	names := make([]string, 0, len(obj.properties))
	for _, name := range obj.propOrder {
		if prop, ok := obj.properties[propertyNameKey(name)]; ok && prop.defined {
			names = append(names, name)
		}
	}
	return names, types.E_NONE
}

func (tx *StoreTxn) TruthyPropertiesWithPrefixInAncestry(objID types.ObjID, prefix string) (map[string]bool, types.ErrorCode) {
	if tx.direct {
		return tx.store.truthyPropertiesWithPrefixInAncestry(objID, prefix)
	}
	if !validLiveObject(tx.object(objID)) {
		return nil, types.E_INVIND
	}

	result := make(map[string]bool)
	seenObjects := make(map[types.ObjID]bool)
	decidedNames := make(map[string]bool)
	lowerPrefix := strings.ToLower(prefix)
	queue := []types.ObjID{objID}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seenObjects[currentID] {
			continue
		}
		seenObjects[currentID] = true

		current := tx.object(currentID)
		if !validLiveObject(current) {
			continue
		}
		tx.markPropertyScan(currentID, current)
		for propName, prop := range current.properties {
			if !strings.HasPrefix(strings.ToLower(propName), lowerPrefix) {
				continue
			}
			tx.markPropertyRead(currentID, propName, prop)
			name := propName[len(prefix):]
			if name == "" || decidedNames[name] || prop.clear {
				continue
			}
			decidedNames[name] = true
			if !prop.value.IsNone() && prop.value.Truthy() {
				result[name] = true
			}
		}
		queue = append(queue, current.parents...)
	}

	return result, types.E_NONE
}

func (tx *StoreTxn) PropertyClearState(objID types.ObjID, name string) (bool, types.ErrorCode) {
	if tx.direct {
		return tx.store.propertyClearState(objID, name)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	actualName, prop, exists := propertyByName(obj.properties, name)
	if !exists {
		tx.markPropertyShapeScan(objID, obj)
		return true, types.E_NONE
	}
	tx.markPropertyRead(objID, actualName, prop)
	if prop.defined {
		return false, types.E_NONE
	}
	return prop.clear, types.E_NONE
}

func (tx *StoreTxn) SetPropertyValue(objID types.ObjID, name string, value types.Value) types.ErrorCode {
	if tx.direct {
		return tx.store.setPropertyValue(objID, name, value)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if actualName, prop, ok := propertyByName(obj.properties, name); ok && !prop.clear && prop.value.Identical(value) {
		// Same-value elision. The slot already holds this exact value and is
		// not clear, so the write changes nothing MOO code can observe (value,
		// clear state, owner and perms are all unchanged). Staging it anyway
		// would clone the object, bump the slot version at commit, and turn
		// every concurrent reader of the slot into a validation conflict —
		// Mongoose's #6:title rewrites `.aliases` on every call, so under
		// optimistic MVCC that turned `look`/`@who` into a retry storm. The
		// read mark stays: the decision to elide depends on the current value,
		// so a concurrent change to the slot must still invalidate this txn.
		tx.markPropertyRead(objID, actualName, prop)
		if tx.store != nil {
			tx.store.propertyWriteElisions.Add(1)
		}
		return types.E_NONE
	}
	obj = tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}

	if actualName, prop, ok := propertyByName(obj.properties, name); ok {
		tx.markPropertyRead(objID, actualName, prop)
		before := prop
		prop.clear = false
		prop.value = value
		// Properties are stored by value: write the mutated copy back so reads
		// within this txn (e.g. PropertyValues) see the staged change.
		obj.properties[actualName] = prop
		tx.stagePropertyValue(objID, actualName, prop, value, before, true)
		return types.E_NONE
	}

	inherited, inheritedName, err := tx.findProperty(objID, name)
	if err != types.E_NONE {
		return err
	}
	override := Property{
		value:   value,
		owner:   inherited.owner,
		perms:   inherited.perms,
		clear:   false,
		defined: false,
		version: inherited.version,
	}
	obj.properties[inheritedName] = override
	tx.stagePropertyValue(objID, inheritedName, override, value, Property{}, false)
	return types.E_NONE
}

func (tx *StoreTxn) SetPropertyInfo(objID types.ObjID, name string, owner *types.ObjID, perms *PropertyPerms) types.ErrorCode {
	if tx.direct {
		return tx.store.setPropertyInfo(objID, name, owner, perms)
	}
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if actualName, prop, ok := propertyByName(obj.properties, name); ok {
		tx.markPropertyRead(objID, actualName, prop)
		if owner != nil {
			prop.owner = *owner
		}
		if perms != nil {
			prop.perms = *perms
		}
		// Properties are stored by value: write the mutated copy back so reads
		// within this txn see the staged owner/perms change.
		obj.properties[actualName] = prop
		key := propertyWriteKey{objID: objID, name: propertyNameKey(actualName)}
		delete(tx.propertyDeletes, key)
		if _, stagedDefine := tx.propertyDefines[key]; stagedDefine {
			lazySet(&tx.propertyDefines, key, propertyDefine{name: actualName, prop: prop})
			return types.E_NONE
		}
		lazySet(&tx.propertyWrites, key, propertyWrite{
			name:  actualName,
			value: prop.value,
			prop:  prop,
		})
		return types.E_NONE
	}
	tx.markPropertyScan(objID, obj)
	return types.E_PROPNF
}

func (tx *StoreTxn) DefineProperty(objID types.ObjID, name string, prop Property) types.ErrorCode {
	if tx.direct {
		return tx.store.defineProperty(objID, name, prop)
	}
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if existingName, existing, ok := propertyByName(obj.properties, name); ok {
		tx.markPropertyRead(objID, existingName, existing)
		return types.E_INVARG
	}
	tx.markPropertyScan(objID, obj)

	prop.defined = true
	prop.clear = false

	key := propertyWriteKey{objID: objID, name: propertyNameKey(name)}
	delete(tx.propertyDeletes, key)
	lazySet(&tx.propertyDefines, key, propertyDefine{name: name, prop: prop})
	obj.properties[propertyNameKey(name)] = prop

	pos := obj.propDefsCount
	if pos > len(obj.propOrder) {
		pos = len(obj.propOrder)
	}
	obj.propOrder = append(obj.propOrder, "")
	copy(obj.propOrder[pos+1:], obj.propOrder[pos:])
	obj.propOrder[pos] = name
	obj.propDefsCount++

	tx.propagateDefinedProperty(objID, name, prop)
	return types.E_NONE
}

func (tx *StoreTxn) ClearPropertyOverride(objID types.ObjID, name string) types.ErrorCode {
	if tx.direct {
		return tx.store.clearPropertyOverride(objID, name)
	}
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	actualName, prop, ok := propertyByName(obj.properties, name)
	if !ok {
		tx.markPropertyScan(objID, obj)
		return types.E_NONE
	}
	tx.markPropertyRead(objID, actualName, prop)
	delete(obj.properties, actualName)
	key := propertyWriteKey{objID: objID, name: propertyNameKey(actualName)}
	delete(tx.propertyWrites, key)
	delete(tx.propertyDefines, key)
	lazySet(&tx.propertyDeletes, key, actualName)
	return types.E_NONE
}
