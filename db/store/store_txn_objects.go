package store

import (
	"slices"

	"github.com/MongooseMoo/barn/types"
)

type objectScalarWrite struct {
	nameSet  bool
	name     string
	ownerSet bool
	owner    types.ObjID
	flagsSet bool
	flags    ObjectFlags
}

func (tx *StoreTxn) ObjectExists(objID types.ObjID) types.ErrorCode {
	if tx.direct {
		return tx.store.objectExists(objID)
	}
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
	if tx.direct {
		return tx.store.valid(objID)
	}
	return validLiveObject(tx.object(objID))
}

func (tx *StoreTxn) ObjectName(objID types.ObjID) (string, types.ErrorCode) {
	if tx.direct {
		return tx.store.objectName(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return "", types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.name, types.E_NONE
}

// ObjectNameValue is ObjectName boxed as a TYPE_STR Value. The box is built
// when the name is written and shared by every reader, so `.name` does not
// allocate per read.
func (tx *StoreTxn) ObjectNameValue(objID types.ObjID) (types.Value, types.ErrorCode) {
	if tx.direct {
		return tx.store.objectNameValue(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.None, types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.nameValue(), types.E_NONE
}

func (tx *StoreTxn) ObjectOwner(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	if tx.direct {
		return tx.store.objectOwner(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.owner, types.E_NONE
}

func (tx *StoreTxn) ObjectFlags(objID types.ObjID) (ObjectFlags, types.ErrorCode) {
	if tx.direct {
		return tx.store.objectFlags(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return 0, types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.flags, types.E_NONE
}

func (tx *StoreTxn) HasObjectFlag(objID types.ObjID, flag ObjectFlags) (bool, types.ErrorCode) {
	if tx.direct {
		return tx.store.hasObjectFlag(objID, flag)
	}
	flags, errCode := tx.ObjectFlags(objID)
	if errCode != types.E_NONE {
		return false, errCode
	}
	return flags.Has(flag), types.E_NONE
}

func (tx *StoreTxn) ObjectIsAnonymous(objID types.ObjID) (bool, types.ErrorCode) {
	if tx.direct {
		return tx.store.objectIsAnonymous(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	return obj.anonymous, types.E_NONE
}

func (tx *StoreTxn) SetObjectName(objID types.ObjID, name string) types.ErrorCode {
	if tx.direct {
		return tx.store.setObjectName(objID, name)
	}
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	obj.setName(name)
	write := tx.scalarWrites[objID]
	write.nameSet = true
	write.name = name
	lazySet(&tx.scalarWrites, objID, write)
	return types.E_NONE
}

func (tx *StoreTxn) SetObjectOwner(objID types.ObjID, owner types.ObjID) types.ErrorCode {
	if tx.direct {
		return tx.store.setObjectOwner(objID, owner)
	}
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	obj.owner = owner
	write := tx.scalarWrites[objID]
	write.ownerSet = true
	write.owner = owner
	lazySet(&tx.scalarWrites, objID, write)
	return types.E_NONE
}

func (tx *StoreTxn) SetObjectFlag(objID types.ObjID, flag ObjectFlags, enabled bool) types.ErrorCode {
	if tx.direct {
		return tx.store.setObjectFlag(objID, flag, enabled)
	}
	obj := tx.mutableObject(objID)
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
	lazySet(&tx.scalarWrites, objID, write)
	return types.E_NONE
}

// CreateObject stages creation of a new NUMBERED object as a child of `parents`, owned
// by `owner`, committing on the decentralized MVCC path. It atomically allocates the id
// (immediately usable by the rest of the verb), builds the object's inherited-property
// image THROUGH the txn (recording the ancestor read deps that are its conflict
// footprint), records a PRISTINE base image in createdObjects (commit rebuilds from a
// clone + this id's staged self-writes, so published memory never aliases txn state),
// caches it for read-your-writes, and stages a commutative children-add on each parent.
// Anonymous creation stays coarse (out-of-band, no slot) — callers must not route it
// here. Retry-safe: on retry the whole txn is dropped and a fresh id is allocated (the
// abandoned id is wasted, never a live slot).
func (tx *StoreTxn) CreateObject(parents []types.ObjID, owner types.ObjID, anonymous ...bool) (types.ObjID, types.ErrorCode) {
	isAnonymous := len(anonymous) > 0 && anonymous[0]
	if tx.direct || isAnonymous {
		return tx.store.createObject(parents, owner, isAnonymous)
	}
	newID := tx.store.allocateID()
	if owner == types.ObjNothing {
		owner = newID
	}
	tx.privateVerbShape = true

	obj := NewObject(newID, owner)
	obj.parents = append([]types.ObjID(nil), parents...)
	obj.properties = tx.copyInheritedProperties(parents)
	// Placeholder versions; the commit build re-stamps every version to the commit ts
	// (a brand-new object is entirely at its creation version).
	stampObjectAll(obj, tx.readTS)

	if tx.createdObjects == nil {
		tx.createdObjects = make(map[types.ObjID]*Object)
	}
	tx.createdObjects[newID] = cloneObjectForReadTxn(obj)
	tx.objects[newID] = obj
	if tx.owned == nil {
		tx.owned = make(map[types.ObjID]bool)
	}
	tx.owned[newID] = true

	// Add the new object to each parent's children (commutative setadd — two creates
	// under one parent commute; no read dep on the parent beyond copyInheritedProperties'
	// property-scan). Also update the cached parent for read-your-writes.
	for _, parentID := range parents {
		if p := tx.object(parentID); validLiveObject(p) {
			m := tx.mutableObject(parentID)
			if !slices.Contains(m.children, newID) {
				m.children = append(m.children, newID)
			}
			tx.stageChildrenDelta(parentID, contentsDelta{add: true, id: newID})
		}
	}

	if newID > tx.maxObjID {
		tx.maxObjID = newID
	}
	return newID, types.E_NONE
}

// MaxObject returns the highest non-anonymous id visible to this txn, including its own
// staged creates (read-your-writes for max_object()).
func (tx *StoreTxn) MaxObject() types.ObjID {
	if tx.direct {
		return tx.store.maxObject()
	}
	if tx == nil {
		return -1
	}
	return tx.maxObjID
}

// RecycleObject stages recycling of a SIMPLE numbered object (no children, no contents)
// on the decentralized path: it removes the object from its location's contents and its
// parents' children (commutative deltas) and stages the recycled tombstone. It records a
// relationship read on the object — the conflict guard: a concurrent move-into or
// create-under it must conflict this recycle (do NOT ForgetObject, Fable P1-1). Returns
// handled=false for a COMPLEX object (has children or contents), so the caller falls
// back to the coarse store.Recycle (which reparents children). Anonymous objects are not
// routed here. recycledID is appended at commit time (under RLock), not here.
func (tx *StoreTxn) RecycleObject(id types.ObjID) (handled bool, ec types.ErrorCode) {
	if tx.direct {
		return false, types.E_NONE
	}
	obj := tx.object(id)
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	tx.markObjectRelationshipRead(id, obj)
	if len(obj.children) > 0 || len(obj.contents) > 0 {
		return false, types.E_NONE // complex: caller uses coarse recycle
	}
	tx.privateVerbShape = true

	oldLoc := obj.location
	if oldLoc != types.ObjNothing {
		if loc := tx.object(oldLoc); validLiveObject(loc) {
			m := tx.mutableObject(oldLoc)
			m.contents = removeObjID(m.contents, id)
			tx.stageContentsDelta(oldLoc, contentsDelta{add: false, id: id})
		}
	}
	for _, parentID := range obj.parents {
		if p := tx.object(parentID); validLiveObject(p) {
			m := tx.mutableObject(parentID)
			m.children = removeObjID(m.children, id)
			tx.stageChildrenDelta(parentID, contentsDelta{add: false, id: id})
		}
	}

	// Stage the tombstone and reflect it in the cache (read-your-writes).
	if tx.recycleWrites == nil {
		tx.recycleWrites = make(map[types.ObjID]bool)
	}
	tx.recycleWrites[id] = true
	m := tx.mutableObject(id)
	m.contents = []types.ObjID{}
	m.location = types.ObjNothing
	m.properties = make(map[string]Property)
	m.verbs = make(map[string]*Verb)
	m.recycled = true
	m.flags = m.flags.Set(FlagRecycled | FlagInvalid)
	return true, types.E_NONE
}

// IsRecycled reports whether id resolves to a recycled tombstone in this txn's view —
// including a recycle this task staged decentrally but has not yet committed. Mirrors
// Store.IsRecycled, which sees only committed live state.
func (tx *StoreTxn) IsRecycled(id types.ObjID) bool {
	if tx.direct {
		return tx.store.isRecycled(id)
	}
	if tx.recycleWrites[id] {
		return true
	}
	obj := tx.object(id)
	return obj != nil && obj.recycled
}
