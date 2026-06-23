package store

import (
	"fmt"
	"strings"

	"barn/types"
)

type StoreTxn struct {
	readTS                    uint64
	store                     *Store
	objects                   map[types.ObjID]*Object
	scalarReads               map[types.ObjID]uint64
	scalarWrites              map[types.ObjID]objectScalarWrite
	relationshipReads         map[types.ObjID]uint64
	relationshipWrites        map[types.ObjID]objectRelationshipWrite
	propertyReads             map[propertyReadKey]uint64
	propertyScans             map[types.ObjID]uint64
	propertyDefines           map[propertyWriteKey]Property
	propertyDefinitionDeletes map[propertyWriteKey]string
	propertyWrites            map[propertyWriteKey]propertyWrite
	propertyDeletes           map[propertyWriteKey]string
	verbReads                 map[verbReadKey]uint64
	verbScans                 map[types.ObjID]uint64
	verbWrites                map[verbWriteKey]verbWrite
	validationFail            bool
	maxObjID                  types.ObjID
	highWaterID               types.ObjID
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

type objectRelationshipWrite struct {
	locationSet bool
	location    types.ObjID
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

type verbWriteKey struct {
	objID types.ObjID
	name  string
}

type verbWrite struct {
	code []string
}

func (s *Store) BeginReadOnly(readTS uint64) *StoreTxn {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if readTS == 0 {
		readTS = s.clock
	}
	return &StoreTxn{
		readTS:                    readTS,
		store:                     s,
		objects:                   make(map[types.ObjID]*Object),
		scalarReads:               make(map[types.ObjID]uint64),
		scalarWrites:              make(map[types.ObjID]objectScalarWrite),
		relationshipReads:         make(map[types.ObjID]uint64),
		relationshipWrites:        make(map[types.ObjID]objectRelationshipWrite),
		propertyReads:             make(map[propertyReadKey]uint64),
		propertyScans:             make(map[types.ObjID]uint64),
		propertyDefines:           make(map[propertyWriteKey]Property),
		propertyDefinitionDeletes: make(map[propertyWriteKey]string),
		propertyWrites:            make(map[propertyWriteKey]propertyWrite),
		propertyDeletes:           make(map[propertyWriteKey]string),
		verbReads:                 make(map[verbReadKey]uint64),
		verbScans:                 make(map[types.ObjID]uint64),
		verbWrites:                make(map[verbWriteKey]verbWrite),
		maxObjID:                  s.maxObjID,
		highWaterID:               s.highWaterID,
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

func (tx *StoreTxn) AdoptLiveObject(objID types.ObjID) types.ErrorCode {
	if tx == nil {
		return types.E_NONE
	}
	if tx.store == nil {
		tx.objects[objID] = nil
		return types.E_INVIND
	}
	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	live := tx.store.objects[objID]
	if !validLiveObject(live) {
		tx.objects[objID] = nil
		return types.E_INVIND
	}
	tx.objects[objID] = cloneObjectForReadTxn(live)
	if objID > tx.maxObjID {
		tx.maxObjID = objID
	}
	if objID > tx.highWaterID {
		tx.highWaterID = objID
	}
	return types.E_NONE
}

func (tx *StoreTxn) AdoptLiveVerbs(objID types.ObjID) types.ErrorCode {
	if tx == nil {
		return types.E_NONE
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if tx.store == nil {
		return types.E_INVARG
	}

	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	live := tx.store.objects[objID]
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
		verb.code = append([]string(nil), write.code...)
		verb.hasProgram = true
	}
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
	if tx == nil {
		return types.E_NONE
	}
	if tx.store == nil {
		return types.E_INVARG
	}

	tx.store.mu.RLock()
	defer tx.store.mu.RUnlock()

	for _, objID := range objIDs {
		if objID == types.ObjNothing {
			continue
		}
		live := tx.store.objects[objID]
		if !validLiveObject(live) {
			tx.objects[objID] = nil
			return types.E_INVIND
		}
		obj := tx.objects[objID]
		if obj == nil {
			obj = cloneObjectForReadTxn(live)
			tx.objects[objID] = obj
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

func (tx *StoreTxn) markPropertyRead(objID types.ObjID, prop *Property) {
	if tx == nil || prop == nil {
		return
	}
	key := propertyWriteKey{objID: objID, name: propertyNameKey(prop.name)}
	if _, staged := tx.propertyDefines[key]; staged {
		return
	}
	if _, staged := tx.propertyWrites[key]; staged {
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
	key := propertyWriteKey{objID: objID, name: propertyNameKey(prop.name)}
	delete(tx.propertyDeletes, key)
	if _, stagedDefine := tx.propertyDefines[key]; stagedDefine {
		tx.propertyDefines[key] = prop
		return
	}
	tx.propertyWrites[key] = propertyWrite{
		value: value,
		prop:  prop,
	}
}

func (tx *StoreTxn) markVerbRead(objID types.ObjID, verb *Verb) {
	if tx == nil || verb == nil {
		return
	}
	if _, staged := tx.verbWrites[verbWriteKey{objID: objID, name: verb.name}]; staged {
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
	return tx != nil && (len(tx.scalarWrites) > 0 || len(tx.relationshipWrites) > 0 || len(tx.propertyDefines) > 0 || len(tx.propertyDefinitionDeletes) > 0 || len(tx.propertyWrites) > 0 || len(tx.propertyDeletes) > 0 || len(tx.verbWrites) > 0)
}

func (tx *StoreTxn) ForgetObject(objID types.ObjID) {
	if tx == nil {
		return
	}
	tx.objects[objID] = nil
	delete(tx.scalarReads, objID)
	delete(tx.scalarWrites, objID)
	delete(tx.relationshipReads, objID)
	delete(tx.relationshipWrites, objID)
	delete(tx.propertyScans, objID)
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
}

func (tx *StoreTxn) ValidationFailed() bool {
	return tx != nil && tx.validationFail
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

func (tx *StoreTxn) SetObjectLocationRaw(objID types.ObjID, location types.ObjID) types.ErrorCode {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	obj.location = location
	write := tx.relationshipWrites[objID]
	write.locationSet = true
	write.location = location
	tx.relationshipWrites[objID] = write
	return types.E_NONE
}

func (tx *StoreTxn) Parent(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
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
	tx.markObjectRelationshipRead(objID, obj)
	return append([]types.ObjID(nil), obj.parents...), types.E_NONE
}

func (tx *StoreTxn) Children(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return append([]types.ObjID(nil), obj.children...), types.E_NONE
}

func (tx *StoreTxn) Contents(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return append([]types.ObjID(nil), obj.contents...), types.E_NONE
}

func (tx *StoreTxn) Location(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
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

func (tx *StoreTxn) HasDuplicateDefinedPropertyAmong(ids []types.ObjID) (bool, types.ErrorCode) {
	seen := make(map[string]bool)
	for _, id := range ids {
		obj := tx.object(id)
		if !validLiveObject(obj) {
			return false, types.E_INVARG
		}
		tx.markPropertyScan(id, obj)
		for name, prop := range obj.properties {
			if prop == nil || !prop.defined {
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

func (tx *StoreTxn) SetPropertyInfo(objID types.ObjID, name string, owner *types.ObjID, perms *PropertyPerms) types.ErrorCode {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if _, prop, ok := propertyByName(obj.properties, name); ok {
		tx.markPropertyRead(objID, prop)
		if owner != nil {
			prop.owner = *owner
		}
		if perms != nil {
			prop.perms = *perms
		}
		key := propertyWriteKey{objID: objID, name: propertyNameKey(prop.name)}
		delete(tx.propertyDeletes, key)
		if _, stagedDefine := tx.propertyDefines[key]; stagedDefine {
			tx.propertyDefines[key] = *prop
			return types.E_NONE
		}
		tx.propertyWrites[key] = propertyWrite{
			value: prop.value,
			prop:  *prop,
		}
		return types.E_NONE
	}
	tx.markPropertyScan(objID, obj)
	return types.E_PROPNF
}

func (tx *StoreTxn) DefineProperty(objID types.ObjID, prop Property) types.ErrorCode {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if _, existing, ok := propertyByName(obj.properties, prop.name); ok {
		tx.markPropertyRead(objID, existing)
		return types.E_INVARG
	}
	tx.markPropertyScan(objID, obj)

	prop.defined = true
	prop.clear = false
	key := propertyWriteKey{objID: objID, name: propertyNameKey(prop.name)}
	delete(tx.propertyDeletes, key)
	tx.propertyDefines[key] = prop
	obj.properties[prop.name] = cloneProperty(&prop)

	pos := obj.propDefsCount
	if pos > len(obj.propOrder) {
		pos = len(obj.propOrder)
	}
	obj.propOrder = append(obj.propOrder, "")
	copy(obj.propOrder[pos+1:], obj.propOrder[pos:])
	obj.propOrder[pos] = prop.name
	obj.propDefsCount++

	tx.propagateDefinedProperty(objID, &prop)
	return types.E_NONE
}

func (tx *StoreTxn) propagateDefinedProperty(objID types.ObjID, prop *Property) {
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
			if actualName, existing, ok := propertyByName(child.properties, prop.name); ok {
				tx.markPropertyRead(childID, existing)
				if existing.defined {
					queue = append(queue, childID)
					continue
				}
				delete(child.properties, actualName)
			} else {
				tx.markPropertyScan(childID, child)
			}
			child.properties[prop.name] = &Property{
				name:    prop.name,
				value:   prop.value,
				owner:   prop.owner,
				perms:   prop.perms,
				clear:   true,
				defined: false,
			}
			tx.propertyWrites[propertyWriteKey{objID: childID, name: propertyNameKey(prop.name)}] = propertyWrite{
				value: prop.value,
				prop:  *child.properties[prop.name],
			}
			queue = append(queue, childID)
		}
	}
}

func (tx *StoreTxn) ClearPropertyOverride(objID types.ObjID, name string) types.ErrorCode {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	actualName, prop, ok := propertyByName(obj.properties, name)
	if !ok {
		tx.markPropertyScan(objID, obj)
		return types.E_NONE
	}
	tx.markPropertyRead(objID, prop)
	delete(obj.properties, actualName)
	key := propertyWriteKey{objID: objID, name: propertyNameKey(actualName)}
	delete(tx.propertyWrites, key)
	delete(tx.propertyDefines, key)
	tx.propertyDeletes[key] = actualName
	return types.E_NONE
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
			if _, prop, ok := propertyByName(child.properties, name); ok {
				tx.markPropertyRead(childID, prop)
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
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	actualName, prop, ok := propertyByName(obj.properties, name)
	if !ok {
		tx.markPropertyScan(objID, obj)
		return types.E_PROPNF
	}
	tx.markPropertyRead(objID, prop)
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
		tx.propertyDefinitionDeletes[key] = actualName
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
			child := tx.object(childID)
			if !validLiveObject(child) {
				continue
			}
			if actualName, prop, ok := propertyByName(child.properties, name); ok {
				tx.markPropertyRead(childID, prop)
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

func (tx *StoreTxn) Commit() types.ErrorCode {
	if tx == nil || (len(tx.scalarWrites) == 0 && len(tx.relationshipWrites) == 0 && len(tx.propertyDefines) == 0 && len(tx.propertyDefinitionDeletes) == 0 && len(tx.propertyWrites) == 0 && len(tx.propertyDeletes) == 0 && len(tx.verbWrites) == 0) {
		return types.E_NONE
	}
	if tx.store == nil {
		return types.E_INVARG
	}
	tx.validationFail = false

	tx.store.mu.Lock()
	defer tx.store.mu.Unlock()

	if errCode := tx.validateObjectScalarReadsLocked(); errCode != types.E_NONE {
		tx.validationFail = true
		return errCode
	}
	if errCode := tx.validateObjectRelationshipReadsLocked(); errCode != types.E_NONE {
		tx.validationFail = true
		return errCode
	}
	if errCode := tx.validatePropertyReadsLocked(); errCode != types.E_NONE {
		tx.validationFail = true
		return errCode
	}
	if errCode := tx.validateVerbReadsLocked(); errCode != types.E_NONE {
		tx.validationFail = true
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
	for objID, write := range tx.relationshipWrites {
		live := tx.store.objects[objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if !remembered[objID] {
			tx.store.rememberObjectLocked(live)
			remembered[objID] = true
		}
		if write.locationSet {
			live.location = write.location
		}
		stampObjectRelationship(live, ts)
	}
	for key, prop := range tx.propertyDefines {
		live := tx.store.objects[key.objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if errCode := tx.store.definePropertyLocked(key.objID, prop, ts); errCode != types.E_NONE {
			return errCode
		}
		remembered[key.objID] = true
	}
	for key, actualName := range tx.propertyDefinitionDeletes {
		live := tx.store.objects[key.objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if errCode := tx.store.deleteDefinedPropertyLocked(key.objID, actualName, ts); errCode != types.E_NONE {
			return errCode
		}
		remembered[key.objID] = true
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
			prop.value = write.prop.value
			prop.owner = write.prop.owner
			prop.perms = write.prop.perms
			prop.clear = write.prop.clear
			prop.defined = write.prop.defined
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
	for key, actualName := range tx.propertyDeletes {
		live := tx.store.objects[key.objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if !remembered[key.objID] {
			tx.store.rememberObjectLocked(live)
			remembered[key.objID] = true
		}
		if liveActual, _, ok := propertyByName(live.properties, actualName); ok {
			delete(live.properties, liveActual)
		}
		stampObjectProperties(live, ts)
	}
	for key, write := range tx.verbWrites {
		live := tx.store.objects[key.objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		verb := live.verbs[key.name]
		if verb == nil {
			return types.E_VERBNF
		}
		if !remembered[key.objID] {
			tx.store.rememberObjectLocked(live)
			remembered[key.objID] = true
		}
		verb.code = append([]string(nil), write.code...)
		verb.hasProgram = true
		stampVerb(verb, ts)
		stampObjectVerbs(live, ts)
	}
	tx.scalarWrites = make(map[types.ObjID]objectScalarWrite)
	tx.relationshipWrites = make(map[types.ObjID]objectRelationshipWrite)
	tx.propertyDefines = make(map[propertyWriteKey]Property)
	tx.propertyDefinitionDeletes = make(map[propertyWriteKey]string)
	tx.propertyWrites = make(map[propertyWriteKey]propertyWrite)
	tx.propertyDeletes = make(map[propertyWriteKey]string)
	tx.verbWrites = make(map[verbWriteKey]verbWrite)
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

func (tx *StoreTxn) validateObjectRelationshipReadsLocked() types.ErrorCode {
	for objID, version := range tx.relationshipReads {
		live := tx.store.objects[objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if live.relationshipVersion != version {
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

func (tx *StoreTxn) validateVerbReadsLocked() types.ErrorCode {
	for key, version := range tx.verbReads {
		live := tx.store.objects[key.objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		verb := live.verbs[key.name]
		if verb == nil || verb.version != version {
			return types.E_INVARG
		}
	}
	for objID, version := range tx.verbScans {
		live := tx.store.objects[objID]
		if !validLiveObject(live) {
			return types.E_INVIND
		}
		if live.verbVersion != version {
			return types.E_INVARG
		}
	}
	return types.E_NONE
}

func (tx *StoreTxn) SetVerbCode(objID types.ObjID, name string, lines []string) types.ErrorCode {
	verb, definer, err := tx.findVerb(objID, name)
	if err != nil || verb == nil {
		return types.E_VERBNF
	}
	tx.stageVerbCode(definer, verb, lines)
	return types.E_NONE
}

func (tx *StoreTxn) SetVerbCodeByIndex(objID types.ObjID, index int, lines []string) types.ErrorCode {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if index < 0 || index >= len(obj.verbList) {
		return types.E_RANGE
	}
	tx.markVerbScan(objID, obj)
	verb := obj.verbList[index]
	tx.markVerbRead(objID, verb)
	tx.stageVerbCode(objID, verb, lines)
	return types.E_NONE
}

func (tx *StoreTxn) stageVerbCode(objID types.ObjID, verb *Verb, lines []string) {
	verb.code = append([]string(nil), lines...)
	verb.hasProgram = true
	tx.verbWrites[verbWriteKey{objID: objID, name: verb.name}] = verbWrite{
		code: append([]string(nil), lines...),
	}
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
