package db

import (
	"barn/db/store"
	"barn/types"
	"fmt"
	"sort"
)

func (db *Database) repairStartupIssues() {
	db.repairInvalidObjectReferences()
	db.repairCycles()
	db.repairTopDownInconsistencies()
	db.repairBottomUpInconsistencies()
}

func (db *Database) sortedObjectIDs() []types.ObjID {
	ids := make([]types.ObjID, 0, len(db.Objects))
	for id := range db.Objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (db *Database) validObjectID(id types.ObjID) bool {
	if id < 0 {
		return id == types.ObjNothing
	}
	obj := db.Objects[id]
	return obj != nil && !obj.Recycled && !obj.Flags.Has(store.FlagInvalid)
}

func containsObjID(ids []types.ObjID, target types.ObjID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func appendUniqueObjID(ids []types.ObjID, target types.ObjID) []types.ObjID {
	if containsObjID(ids, target) {
		return ids
	}
	return append(ids, target)
}

func (db *Database) repairInvalidObjectReferences() {
	for _, id := range db.sortedObjectIDs() {
		obj := db.Objects[id]
		if obj == nil {
			continue
		}

		validParents := obj.Parents[:0]
		for _, parentID := range obj.Parents {
			if db.validObjectID(parentID) {
				validParents = append(validParents, parentID)
				continue
			}
			db.recordStartupRepair(fmt.Sprintf("#%d.parent = #%d <invalid> ... removed", obj.ID, parentID))
		}
		obj.Parents = validParents

		validChildren := obj.Children[:0]
		for _, childID := range obj.Children {
			if db.validObjectID(childID) {
				validChildren = append(validChildren, childID)
				continue
			}
			db.recordStartupRepair(fmt.Sprintf("#%d.child = #%d <invalid> ... removed", obj.ID, childID))
		}
		obj.Children = validChildren

		if obj.Location != types.ObjNothing && !db.validObjectID(obj.Location) {
			db.recordStartupRepair(fmt.Sprintf("#%d.location = #%d <invalid> ... fixed", obj.ID, obj.Location))
			obj.Location = types.ObjNothing
		}

		validContents := obj.Contents[:0]
		for _, contentID := range obj.Contents {
			if db.validObjectID(contentID) {
				validContents = append(validContents, contentID)
				continue
			}
			db.recordStartupRepair(fmt.Sprintf("#%d.content = #%d <invalid> ... removed", obj.ID, contentID))
		}
		obj.Contents = validContents
	}
}

func (db *Database) repairCycles() {
	parentCycles := make(map[types.ObjID]bool, len(db.Objects))
	locationCycles := make(map[types.ObjID]bool, len(db.Objects))
	for _, id := range db.sortedObjectIDs() {
		obj := db.Objects[id]
		if obj == nil {
			continue
		}
		parentCycles[obj.ID] = hasParentCycle(db, obj.ID)
		locationCycles[obj.ID] = hasLocationCycle(db, obj.ID)
	}

	for _, id := range db.sortedObjectIDs() {
		obj := db.Objects[id]
		if obj == nil {
			continue
		}
		if parentCycles[obj.ID] {
			db.recordStartupRepair(fmt.Sprintf("Cycle in parent chain of #%d", obj.ID))
			obj.Parents = nil
		}
		if locationCycles[obj.ID] {
			db.recordStartupRepair(fmt.Sprintf("Cycle in location chain of #%d", obj.ID))
			obj.Location = types.ObjNothing
		}
	}
}

func hasParentCycle(db *Database, start types.ObjID) bool {
	visited := make(map[types.ObjID]bool)
	var visit func(types.ObjID) bool
	visit = func(id types.ObjID) bool {
		obj := db.Objects[id]
		if obj == nil {
			return false
		}
		visited[id] = true
		for _, parentID := range obj.Parents {
			if parentID == start {
				return true
			}
			if visited[parentID] {
				continue
			}
			if visit(parentID) {
				return true
			}
		}
		return false
	}
	return visit(start)
}

func hasLocationCycle(db *Database, start types.ObjID) bool {
	seen := make(map[types.ObjID]bool)
	current := start
	for current != types.ObjNothing {
		if seen[current] {
			return true
		}
		seen[current] = true
		obj := db.Objects[current]
		if obj == nil {
			return false
		}
		current = obj.Location
	}
	return false
}

func (db *Database) repairTopDownInconsistencies() {
	for _, id := range db.sortedObjectIDs() {
		obj := db.Objects[id]
		if obj == nil {
			continue
		}

		if obj.Location != types.ObjNothing {
			if location := db.Objects[obj.Location]; location != nil && !containsObjID(location.Contents, obj.ID) {
				db.recordStartupRepair(fmt.Sprintf("#%d not in it's location's (#%d) contents", obj.ID, obj.Location))
				location.Contents = appendUniqueObjID(location.Contents, obj.ID)
			}
		}

		for _, parentID := range obj.Parents {
			if parent := db.Objects[parentID]; parent != nil && !containsObjID(parent.Children, obj.ID) {
				db.recordStartupRepair(fmt.Sprintf("#%d not in it's parent's (#%d) children", obj.ID, parentID))
				parent.Children = appendUniqueObjID(parent.Children, obj.ID)
			}
		}
	}
}

func (db *Database) repairBottomUpInconsistencies() {
	for _, id := range db.sortedObjectIDs() {
		obj := db.Objects[id]
		if obj == nil {
			continue
		}

		for _, childID := range obj.Children {
			if child := db.Objects[childID]; child != nil && !containsObjID(child.Parents, obj.ID) {
				db.recordStartupRepair(fmt.Sprintf("#%d not in it's child's (#%d) parents", obj.ID, childID))
				child.Parents = appendUniqueObjID(child.Parents, obj.ID)
			}
		}

		for _, contentID := range obj.Contents {
			if content := db.Objects[contentID]; content != nil && content.Location != obj.ID {
				db.recordStartupRepair(fmt.Sprintf("#%d not in it's content's (#%d) location", obj.ID, contentID))
				content.Location = obj.ID
			}
		}
	}
}
