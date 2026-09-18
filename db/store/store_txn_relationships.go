package store

import (
	"slices"
	"time"

	"github.com/MongooseMoo/barn/types"
)

type objectRelationshipWrite struct {
	locationSet bool
	location    types.ObjID
	lastMoveSet bool
	lastMove    types.Value
	// contentsDeltas stages COMMUTATIVE add/remove edits to the inverse relationship
	// edge (a room's contents), applied IN ORDER to the room's CURRENT live contents
	// at commit — not a whole-list overwrite computed from a stale snapshot. move()
	// stages a remove on the old room and an add on the new room this way and does NOT
	// record a read dep on either room, so two moves into the SAME room both commit
	// (setadd/setremove commute) and merely serialize on that room's slot mutex instead
	// of one aborting and re-running the whole verb. The room's relationshipVersion
	// still bumps, so a task that READ the room's contents and then writes still
	// conflicts correctly; only blind commutative appenders avoid conflicting.
	contentsDeltas []contentsDelta
	// childrenDeltas stages commutative SETADD/setremove edits to the parent-side
	// `children` edge, used by create (parent.children += newChild) and recycle
	// (grandparent.children += reparented child). Adds are idempotent (setadd), so two
	// creates under one parent, or a diamond reparent, never duplicate an entry, and no
	// read dep is recorded on the parent — so concurrent creates under the same parent
	// commute. (The child's own `parents` list, by contrast, is a whole-list write with
	// a read, so two recycles of different parents of one child correctly conflict.)
	childrenDeltas []contentsDelta
}

// contentsDelta is one commutative edit to a relationship list: add id (at a MOO
// position, for contents) or remove id (position-independent).
type contentsDelta struct {
	add      bool
	id       types.ObjID
	position int64 // 1-based MOO insert position for contents adds; ignored otherwise
}

// applyChildrenDeltas applies setadd (idempotent) / setremove children edits to a
// copy, returning a fresh slice (the input immutable image's slice is never mutated).
func applyChildrenDeltas(children []types.ObjID, deltas []contentsDelta) []types.ObjID {
	result := children
	for _, d := range deltas {
		if d.add {
			if !slices.Contains(result, d.id) {
				result = append(append([]types.ObjID(nil), result...), d.id)
			}
		} else {
			result = removeObjID(result, d.id)
		}
	}
	return result
}

// applyContentsDeltas applies deltas in order to a copy-on-each-op contents slice.
// removeObjID and insertObjIDAtMOOPosition both return fresh slices, so the input
// (an immutable published image's contents) is never mutated.
func applyContentsDeltas(contents []types.ObjID, deltas []contentsDelta) []types.ObjID {
	result := contents
	for _, d := range deltas {
		if d.add {
			if !slices.Contains(result, d.id) {
				result = insertObjIDAtMOOPosition(result, d.id, d.position)
			}
		} else {
			result = removeObjID(result, d.id)
		}
	}
	return result
}

func (tx *StoreTxn) SetObjectLocationRaw(objID types.ObjID, location types.ObjID) types.ErrorCode {
	if tx.direct {
		return tx.store.setObjectLocationRaw(objID, location)
	}
	obj := tx.mutableObject(objID)
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	obj.location = location
	write := tx.relationshipWrites[objID]
	write.locationSet = true
	write.location = location
	lazySet(&tx.relationshipWrites, objID, write)
	return types.E_NONE
}

// stageContentsDelta appends one commutative contents edit for objID (the inverse
// relationship edge). It does NOT mark a relationship read on objID: a blind
// add/remove commutes with any other, so two moves touching the same room must not
// conflict. The caller updates the cached image (read-your-writes) via mutableObject.
func (tx *StoreTxn) stageContentsDelta(objID types.ObjID, d contentsDelta) {
	write := tx.relationshipWrites[objID]
	write.contentsDeltas = append(write.contentsDeltas, d)
	lazySet(&tx.relationshipWrites, objID, write)
}

func (tx *StoreTxn) stageChildrenDelta(objID types.ObjID, d contentsDelta) {
	write := tx.relationshipWrites[objID]
	write.childrenDeltas = append(write.childrenDeltas, d)
	lazySet(&tx.relationshipWrites, objID, write)
}

// MoveObject stages moving `what` into `where` at `position` through the txn's
// decentralized write path: what.location (a scalar edge), plus COMMUTATIVE contents
// deltas (remove from the old room, add to the new room) on the two rooms. It records
// a relationship read only on `what` (so two moves of the SAME object conflict), NOT
// on the rooms — so two moves into the same room both commit and merely serialize on
// that room's slot mutex at publish time, instead of one aborting and re-running the
// whole verb. It mutates the txn's cached images for read-your-writes. Retry-safe.
//
// Mirrors store.MoveObject's imperative order (remove from old, set location, insert
// into new) so a same-location move re-orders identically.
func (tx *StoreTxn) MoveObject(whatID, whereID types.ObjID, position int64) types.ErrorCode {
	if tx.direct {
		return tx.store.moveObject(whatID, whereID, position)
	}
	what := tx.object(whatID)
	if !validLiveObject(what) {
		return types.E_INVIND
	}
	oldLocID := what.location
	tx.markObjectRelationshipRead(whatID, what)

	// Remove `what` from its old location's contents (commutative delta; no room read).
	if oldLocID != types.ObjNothing {
		if oldLoc := tx.object(oldLocID); validLiveObject(oldLoc) {
			m := tx.mutableObject(oldLocID)
			m.contents = removeObjID(m.contents, whatID)
			tx.stageContentsDelta(oldLocID, contentsDelta{add: false, id: whatID})
		}
	}

	// Set `what`'s location.
	m := tx.mutableObject(whatID)
	m.location = whereID
	lastMove := types.NewMap([][2]types.Value{
		{types.NewStr("time"), types.NewInt(time.Now().Unix())},
		{types.NewStr("source"), types.NewObj(oldLocID)},
	})
	m.lastMove = lastMove
	locWrite := tx.relationshipWrites[whatID]
	locWrite.locationSet = true
	locWrite.location = whereID
	locWrite.lastMoveSet = true
	locWrite.lastMove = lastMove
	lazySet(&tx.relationshipWrites, whatID, locWrite)

	// Insert `what` into the new location's contents at the MOO position (commutative
	// delta; no room read).
	if whereID != types.ObjNothing {
		if where := tx.object(whereID); validLiveObject(where) {
			mw := tx.mutableObject(whereID)
			mw.contents = insertObjIDAtMOOPosition(mw.contents, whatID, position)
			tx.stageContentsDelta(whereID, contentsDelta{add: true, id: whatID, position: position})
		}
	}
	return types.E_NONE
}

// HasContentDescendant reports whether targetID is objID or lies within objID's
// contents tree, reading through the txn snapshot and recording relationship reads
// on every object walked so a concurrent move that would change the answer conflicts
// this txn (preventing two concurrent moves from each creating a containment cycle).
func (tx *StoreTxn) HasContentDescendant(objID, targetID types.ObjID) bool {
	if tx.direct {
		return tx.store.hasContentDescendant(objID, targetID)
	}
	if objID == targetID {
		return true
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
		if currentID == targetID {
			return true
		}
		current := tx.object(currentID)
		if validLiveObject(current) {
			tx.markObjectRelationshipRead(currentID, current)
			queue = append(queue, current.contents...)
		}
	}
	return false
}

func (tx *StoreTxn) Parent(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	if tx.direct {
		return tx.store.parent(objID)
	}
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
	if tx.direct {
		return tx.store.parents(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return append([]types.ObjID(nil), obj.parents...), types.E_NONE
}

func (tx *StoreTxn) Children(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	if tx.direct {
		return tx.store.children(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return append([]types.ObjID(nil), obj.children...), types.E_NONE
}

// Ancestors returns objID's ancestors in breadth-first parent order, resolving each hop
// through the txn (read-your-writes) so a chain built by this task's own decentralized
// creates is visible before commit. Mirrors Store.Ancestors, which walks live only and
// therefore misses staged creates. An invalid start object is E_INVIND; an ancestor that
// becomes invalid mid-walk is still listed but not descended through (as in the store).
func (tx *StoreTxn) Ancestors(objID types.ObjID, includeSelf bool) ([]types.ObjID, types.ErrorCode) {
	if tx.direct {
		return tx.store.ancestors(objID, includeSelf)
	}
	parents, ec := tx.Parents(objID)
	if ec != types.E_NONE {
		return nil, ec
	}
	result := make([]types.ObjID, 0)
	seen := make(map[types.ObjID]bool)
	if includeSelf {
		result = append(result, objID)
		seen[objID] = true
	}
	queue := append([]types.ObjID(nil), parents...)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		result = append(result, currentID)
		if p, ec := tx.Parents(currentID); ec == types.E_NONE {
			queue = append(queue, p...)
		}
	}
	return result, types.E_NONE
}

// Descendants is the child-direction counterpart of Ancestors (read-your-writes).
func (tx *StoreTxn) Descendants(objID types.ObjID, includeSelf bool) ([]types.ObjID, types.ErrorCode) {
	if tx.direct {
		return tx.store.descendants(objID, includeSelf)
	}
	children, ec := tx.Children(objID)
	if ec != types.E_NONE {
		return nil, ec
	}
	result := make([]types.ObjID, 0)
	seen := make(map[types.ObjID]bool)
	if includeSelf {
		result = append(result, objID)
		seen[objID] = true
	}
	queue := append([]types.ObjID(nil), children...)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		result = append(result, currentID)
		if c, ec := tx.Children(currentID); ec == types.E_NONE {
			queue = append(queue, c...)
		}
	}
	return result, types.E_NONE
}

// HasAncestor reports whether ancestorID is objID itself or reachable by walking objID's
// parents through the txn (read-your-writes), so an inheritance chain this task staged with
// decentralized creates is honored before commit. Mirrors Store.HasAncestor.
func (tx *StoreTxn) HasAncestor(objID, ancestorID types.ObjID) bool {
	if tx.direct {
		return tx.store.hasAncestor(objID, ancestorID)
	}
	if !validLiveObject(tx.object(objID)) || !validLiveObject(tx.object(ancestorID)) {
		return false
	}
	if objID == ancestorID {
		return true
	}
	seen := make(map[types.ObjID]bool)
	parents, ec := tx.Parents(objID)
	if ec != types.E_NONE {
		return false
	}
	queue := append([]types.ObjID(nil), parents...)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		if currentID == ancestorID {
			return true
		}
		if p, ec := tx.Parents(currentID); ec == types.E_NONE {
			queue = append(queue, p...)
		}
	}
	return false
}

func (tx *StoreTxn) AnonymousChildren(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	if tx.direct {
		tx.store.mu.RLock()
		defer tx.store.mu.RUnlock()
		obj := tx.store.liveObjectLocked(objID)
		if !validLiveObject(obj) {
			return nil, types.E_INVIND
		}
		return append([]types.ObjID(nil), obj.anonymousChildren...), types.E_NONE
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return append([]types.ObjID(nil), obj.anonymousChildren...), types.E_NONE
}

func (tx *StoreTxn) Contents(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	if tx.direct {
		return tx.store.contents(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return append([]types.ObjID(nil), obj.contents...), types.E_NONE
}

func (tx *StoreTxn) Location(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	if tx.direct {
		return tx.store.location(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return obj.location, types.E_NONE
}

func (tx *StoreTxn) LastMove(objID types.ObjID) (types.Value, types.ErrorCode) {
	if tx.direct {
		return tx.store.lastMove(objID)
	}
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return types.None, types.E_INVIND
	}
	tx.markObjectRelationshipRead(objID, obj)
	return obj.lastMove, types.E_NONE
}
