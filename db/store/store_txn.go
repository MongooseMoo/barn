package store

import (
	"fmt"
	"strings"

	"barn/types"
)

type StoreTxn struct {
	readTS         uint64
	store          *Store
	objects        map[types.ObjID]*Object
	scalarReads    map[types.ObjID]uint64
	scalarWrites   map[types.ObjID]objectScalarWrite
	propertyReads  map[propertyReadKey]uint64
	propertyScans  map[types.ObjID]uint64
	propertyWrites map[propertyWriteKey]propertyWrite
	verbReads      map[verbReadKey]uint64
	verbScans      map[types.ObjID]uint64
	maxObjID       types.ObjID
	highWaterID    types.ObjID
}

type propertyReadKey struct {
	objID types.ObjID
	name  string
}

type objectScalarWrite struct {
	nameSet  bool
	name     string
	ownerSet bool
	owner    types.ObjID
	flagsSet bool
	flags    ObjectFlags
}

type propertyWriteKey struct {
	objID types.ObjID
	name  string
}

type propertyWrite struct {
	value types.Value
	prop  Property
}

type verbReadKey struct {
	objID types.ObjID
	name  string
}

func (s *Store) BeginReadOnly(readTS uint64) *StoreTxn {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if readTS == 0 {
		readTS = s.clock
	}
	return &StoreTxn{
		readTS:         readTS,
		store:          s,
		objects:        make(map[types.ObjID]*Object),
		scalarReads:    make(map[types.ObjID]uint64),
		scalarWrites:   make(map[types.ObjID]objectScalarWrite),
		propertyReads:  make(map[propertyReadKey]uint64),
		propertyScans:  make(map[types.ObjID]uint64),
		propertyWrites: make(map[propertyWriteKey]propertyWrite),
		verbReads:      make(map[verbReadKey]uint64),
		verbScans:      make(map[types.ObjID]uint64),
		maxObjID:       s.maxObjID,
		highWaterID:    s.highWaterID,
	}
}

func (tx *StoreTxn) ReadTimestamp() uint64 {
	if tx == nil {
		return 0
	}
	return tx.readTS
}

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

func (tx *StoreTxn) objectLocked(objID types.ObjID) *Object {
	live := tx.store.objects[objID]
	if live != nil && objectVersion(live) <= tx.readTS {
		return cloneObjectForReadTxn(live)
	}

	history := tx.store.history[objID]
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].ts <= tx.readTS {
			return cloneObjectForReadTxn(history[i].obj)
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

func (tx *StoreTxn) markPropertyRead(objID types.ObjID, prop *Property) {
	if tx == nil || prop == nil {
		return
	}
	tx.propertyReads[propertyReadKey{objID: objID, name: propertyNameKey(prop.name)}] = prop.version
}

func (tx *StoreTxn) markPropertyScan(objID types.ObjID, obj *Object) {
	if tx == nil || obj == nil {
		return
	}
	tx.propertyScans[objID] = obj.propertyVersion
}

func (tx *StoreTxn) stagePropertyValue(objID types.ObjID, prop Property, value types.Value) {
	prop.value = value
	prop.clear = false
	tx.propertyWrites[propertyWriteKey{objID: objID, name: propertyNameKey(prop.name)}] = propertyWrite{
		value: value,
		prop:  prop,
	}
}

func (tx *StoreTxn) markVerbRead(objID types.ObjID, verb *Verb) {
	if tx == nil || verb == nil {
		return
	}
	tx.verbReads[verbReadKey{objID: objID, name: verb.name}] = verb.version
}

func (tx *StoreTxn) markVerbScan(objID types.ObjID, obj *Object) {
	if tx == nil || obj == nil {
		return
	}
	tx.verbScans[objID] = obj.verbVersion
}

func (tx *StoreTxn) HasWrites() bool {
	return tx != nil && (len(tx.scalarWrites) > 0 || len(tx.propertyWrites) > 0)
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

	clone.properties = make(map[string]*Property, len(obj.properties))
	for name, prop := range obj.properties {
		clone.properties[name] = cloneProperty(prop)
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
	clone.code = append([]string(nil), verb.code...)
	return &clone
}

func (tx *StoreTxn) ObjectExists(objID types.ObjID) types.ErrorCode {
	obj := tx.object(objID)
	if validLiveObject(obj) {
		return types.E_NONE
	}
	if obj != nil && obj.recycled {
		return types.E_INVARG
	}
	return types.E_INVIND
}

func (tx *StoreTxn) Valid(objID types.ObjID) bool {
	return validLiveObject(tx.object(objID))
}

func (tx *StoreTxn) ObjectName(objID types.ObjID) (string, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return "", types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.name, types.E_NONE
}

func (tx *StoreTxn) ObjectOwner(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.owner, types.E_NONE
}

func (tx *StoreTxn) ObjectFlags(objID types.ObjID) (ObjectFlags, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return 0, types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.flags, types.E_NONE
}

func (tx *StoreTxn) HasObjectFlag(objID types.ObjID, flag ObjectFlags) (bool, types.ErrorCode) {
	flags, errCode := tx.ObjectFlags(objID)
	if errCode != types.E_NONE {
		return false, errCode
	}
	return flags.Has(flag), types.E_NONE
}

func (tx *StoreTxn) ObjectIsAnonymous(objID types.ObjID) (bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.anonymous, types.E_NONE
}

func (tx *StoreTxn) SetObjectName(objID types.ObjID, name string) types.ErrorCode {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	obj.name = name
	write := tx.scalarWrites[objID]
	write.nameSet = true
	write.name = name
	tx.scalarWrites[objID] = write
	return types.E_NONE
}

func (tx *StoreTxn) SetObjectOwner(objID types.ObjID, owner types.ObjID) types.ErrorCode {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	obj.owner = owner
	write := tx.scalarWrites[objID]
	write.ownerSet = true
	write.owner = owner
	tx.scalarWrites[objID] = write
	return types.E_NONE
}

func (tx *StoreTxn) SetObjectFlag(objID types.ObjID, flag ObjectFlags, enabled bool) types.ErrorCode {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	if enabled {
		obj.flags = obj.flags.Set(flag)
	} else {
		obj.flags = obj.flags.Clear(flag)
	}
	write := tx.scalarWrites[objID]
	write.flagsSet = true
	write.flags = obj.flags
	tx.scalarWrites[objID] = write
	return types.E_NONE
}

func (tx *StoreTxn) Parent(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	if len(obj.parents) == 0 {
		return types.ObjNothing, types.E_NONE
	}
	return obj.parents[0], types.E_NONE
}

func (tx *StoreTxn) Parents(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	return append([]types.ObjID(nil), obj.parents...), types.E_NONE
}

func (tx *StoreTxn) Children(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	return append([]types.ObjID(nil), obj.children...), types.E_NONE
}

func (tx *StoreTxn) Contents(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	return append([]types.ObjID(nil), obj.contents...), types.E_NONE
}

func (tx *StoreTxn) Location(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	return obj.location, types.E_NONE
}

func (tx *StoreTxn) FindProperty(objID types.ObjID, name string) (PropertyView, types.ErrorCode) {
	prop, errCode := tx.findProperty(objID, name)
	if errCode != types.E_NONE {
		return PropertyView{}, errCode
	}
	return prop.View(), types.E_NONE
}

func (tx *StoreTxn) findProperty(objID types.ObjID, name string) (*Property, types.ErrorCode) {
	var targetProp *Property
	visited := make(map[types.ObjID]bool)
	queue := []types.ObjID{objID}

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

		if _, prop, ok := propertyByName(current.properties, name); ok {
			tx.markPropertyRead(currentID, prop)
			if targetProp == nil {
				targetProp = prop
			}
			if !prop.clear {
				if targetProp != prop {
					result := *targetProp
					result.value = prop.value
					result.clear = false
					return &result, types.E_NONE
				}
				return prop, types.E_NONE
			}
		} else {
			tx.markPropertyScan(currentID, current)
		}
		queue = append(queue, current.parents...)
	}

	return nil, types.E_PROPNF
}

func (tx *StoreTxn) PropertyValue(objID types.ObjID, name string) (types.Value, types.ErrorCode) {
	prop, errCode := tx.FindProperty(objID, name)
	if errCode != types.E_NONE {
		return nil, errCode
	}
	return prop.Value, types.E_NONE
}

func (tx *StoreTxn) LocalProperty(objID types.ObjID, name string) (PropertyView, bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return PropertyView{}, false, types.E_INVIND
	}
	_, prop, ok := propertyByName(obj.properties, name)
	if !ok {
		tx.markPropertyScan(objID, obj)
		return PropertyView{}, false, types.E_NONE
	}
	tx.markPropertyRead(objID, prop)
	return prop.View(), true, types.E_NONE
}

func (tx *StoreTxn) DefinedPropertyNames(objID types.ObjID) ([]string, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markPropertyScan(objID, obj)

	names := make([]string, 0, len(obj.properties))
	for _, name := range obj.propOrder {
		prop := obj.properties[name]
		if prop != nil && prop.defined {
			names = append(names, name)
		}
	}
	return names, types.E_NONE
}

func (tx *StoreTxn) PropertyClearState(objID types.ObjID, name string) (bool, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	_, prop, exists := propertyByName(obj.properties, name)
	if !exists {
		tx.markPropertyScan(objID, obj)
		return true, types.E_NONE
	}
	tx.markPropertyRead(objID, prop)
	if prop.defined {
		return false, types.E_NONE
	}
	return prop.clear, types.E_NONE
}

func (tx *StoreTxn) SetPropertyValue(objID types.ObjID, name string, value types.Value) types.ErrorCode {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}

	if _, prop, ok := propertyByName(obj.properties, name); ok {
		tx.markPropertyRead(objID, prop)
		prop.clear = false
		prop.value = value
		tx.stagePropertyValue(objID, *prop, value)
		return types.E_NONE
	}

	inherited, err := tx.findProperty(objID, name)
	if err != types.E_NONE {
		return err
	}
	override := Property{
		name:    inherited.name,
		value:   value,
		owner:   inherited.owner,
		perms:   inherited.perms,
		clear:   false,
		defined: false,
		version: inherited.version,
	}
	obj.properties[inherited.name] = &override
	tx.stagePropertyValue(objID, override, value)
	return types.E_NONE
}

func (tx *StoreTxn) Commit() types.ErrorCode {
	if tx == nil || (len(tx.scalarWrites) == 0 && len(tx.propertyWrites) == 0) {
		return types.E_NONE
	}
	if tx.store == nil {
		return types.E_INVARG
	}

	tx.store.mu.Lock()
	defer tx.store.mu.Unlock()

	if errCode := tx.validateObjectScalarReadsLocked(); errCode != types.E_NONE {
		return errCode
	}
	if errCode := tx.validatePropertyReadsLocked(); errCode != types.E_NONE {
		return errCode
	}

	ts := tx.store.bumpClockLocked()
	remembered := make(map[types.ObjID]bool)
	for objID, write := range tx.scalarWrites {
		live := tx.store.objects[objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if !remembered[objID] {
			tx.store.rememberObjectLocked(live)
			remembered[objID] = true
		}
		if write.nameSet {
			live.name = write.name
		}
		if write.ownerSet {
			live.owner = write.owner
		}
		if write.flagsSet {
			live.flags = write.flags
		}
		stampObjectScalar(live, ts)
	}
	for key, write := range tx.propertyWrites {
		live := tx.store.objects[key.objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if !remembered[key.objID] {
			tx.store.rememberObjectLocked(live)
			remembered[key.objID] = true
		}
		if _, prop, ok := propertyByName(live.properties, write.prop.name); ok {
			prop.clear = false
			prop.value = write.value
			stampProperty(prop, ts)
		} else {
			prop := write.prop
			prop.value = write.value
			prop.clear = false
			prop.version = ts
			live.properties[prop.name] = &prop
		}
		stampObjectProperties(live, ts)
	}
	tx.scalarWrites = make(map[types.ObjID]objectScalarWrite)
	tx.propertyWrites = make(map[propertyWriteKey]propertyWrite)
	return types.E_NONE
}

func (tx *StoreTxn) validateObjectScalarReadsLocked() types.ErrorCode {
	for objID, version := range tx.scalarReads {
		live := tx.store.objects[objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if live.scalarVersion != version {
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func (tx *StoreTxn) validatePropertyReadsLocked() types.ErrorCode {
	for key, version := range tx.propertyReads {
		live := tx.store.objects[key.objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		_, prop, ok := propertyByName(live.properties, key.name)
		if !ok || prop.version != version {
			return types.E_INVARG
		}
	}
	for objID, version := range tx.propertyScans {
		live := tx.store.objects[objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if live.propertyVersion != version {
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func (tx *StoreTxn) FindVerb(objID types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	verb, definer, err := tx.findVerb(objID, verbName)
	if err != nil {
		return VerbView{}, definer, err
	}
	return verb.View(), definer, nil
}

func (tx *StoreTxn) findVerb(objID types.ObjID, verbName string) (*Verb, types.ObjID, error) {
	visited := make(map[types.ObjID]bool)
	queue := []types.ObjID{objID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true

		obj := tx.object(current)
		if obj == nil || obj.recycled {
			continue
		}
		tx.markVerbScan(current, obj)
		for _, verb := range obj.verbList {
			for _, alias := range verb.names {
				if matchVerbName(alias, verbName) {
					tx.markVerbRead(current, verb)
					return verb, current, nil
				}
			}
		}
		if !strings.Contains(verbName, "*") {
			if verb, ok := obj.verbs[verbName]; ok {
				tx.markVerbRead(current, verb)
				return verb, current, nil
			}
			if verb, ok := obj.verbs[":"+verbName]; ok {
				tx.markVerbRead(current, verb)
				return verb, current, nil
			}
		}
		queue = append(queue, obj.parents...)
	}
	return nil, types.ObjNothing, fmt.Errorf("verb not found: %s", verbName)
}

func (tx *StoreTxn) FindVerbOnObject(objID types.ObjID, verbName string) (VerbView, error) {
	verb, err := tx.findVerbOnObject(objID, verbName)
	if err != nil {
		return VerbView{}, err
	}
	return verb.View(), nil
}

func (tx *StoreTxn) findVerbOnObject(objID types.ObjID, verbName string) (*Verb, error) {
	obj := tx.object(objID)
	if obj == nil || obj.recycled {
		return nil, fmt.Errorf("verb not found: %s", verbName)
	}
	tx.markVerbScan(objID, obj)
	for _, verb := range obj.verbList {
		for _, alias := range verb.names {
			if matchVerbName(alias, verbName) {
				tx.markVerbRead(objID, verb)
				return verb, nil
			}
		}
	}
	if !strings.Contains(verbName, "*") {
		if verb, ok := obj.verbs[verbName]; ok {
			tx.markVerbRead(objID, verb)
			return verb, nil
		}
		if verb, ok := obj.verbs[":"+verbName]; ok {
			tx.markVerbRead(objID, verb)
			return verb, nil
		}
	}
	return nil, fmt.Errorf("verb not found: %s", verbName)
}

func (tx *StoreTxn) VerbNames(objID types.ObjID) ([]string, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markVerbScan(objID, obj)

	names := make([]string, 0, len(obj.verbList))
	for _, verb := range obj.verbList {
		names = append(names, verb.name)
	}
	return names, types.E_NONE
}

func (tx *StoreTxn) VerbByIndex(objID types.ObjID, index int) (VerbView, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return VerbView{}, types.E_INVIND
	}
	if index < 0 || index >= len(obj.verbList) {
		return VerbView{}, types.E_RANGE
	}
	tx.markVerbScan(objID, obj)
	verb := obj.verbList[index]
	tx.markVerbRead(objID, verb)
	return verb.View(), types.E_NONE
}

func (tx *StoreTxn) FindParentVerb(verbLoc types.ObjID, verbName string) (VerbView, types.ObjID, error) {
	verbLocObj := tx.object(verbLoc)
	if !validLiveObject(verbLocObj) {
		return VerbView{}, types.ObjNothing, fmt.Errorf("defining object #%d not found", verbLoc)
	}

	visited := make(map[types.ObjID]bool)
	queue := append([]types.ObjID(nil), verbLocObj.parents...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true

		obj := tx.object(current)
		if !validLiveObject(obj) {
			continue
		}
		tx.markVerbScan(current, obj)
		if verb, ok := obj.verbs[verbName]; ok {
			tx.markVerbRead(current, verb)
			return verb.View(), current, nil
		}
		for _, verb := range obj.verbList {
			for _, alias := range verb.names {
				if alias == verbName {
					tx.markVerbRead(current, verb)
					return verb.View(), current, nil
				}
			}
		}
		queue = append(queue, obj.parents...)
	}
	return VerbView{}, types.ObjNothing, fmt.Errorf("verb not found: %s", verbName)
}
