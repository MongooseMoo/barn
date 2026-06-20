package format

import (
	"barn/db/store"
	"barn/types"
	"fmt"
	"sort"
)

func (database *Database) repairStartupIssues() {
	database.repairInvalidObjectReferences()
	database.repairCycles()
	database.repairTopDownInconsistencies()
	database.repairBottomUpInconsistencies()
}

func (database *Database) sortedObjectIDs() []types.ObjID {
	ids := make([]types.ObjID, 0, len(database.Objects))
	for id := range database.Objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (database *Database) validObjectID(id types.ObjID) bool {
	if id < 0 {
		return id == types.ObjNothing
	}
	obj := database.Objects[id]
	return obj != nil && !obj.Recycled() && !obj.Flags().Has(store.FlagInvalid)
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

func (database *Database) repairInvalidObjectReferences() {
	for _, id := range database.sortedObjectIDs() {
		obj := database.Objects[id]
		if obj == nil {
			continue
		}

		parents := obj.Parents()
		validParents := parents[:0]
		for _, parentID := range parents {
			if database.validObjectID(parentID) {
				validParents = append(validParents, parentID)
				continue
			}
			database.recordStartupRepair(fmt.Sprintf("#%d.parent = #%d <invalid> ... removed", obj.ID(), parentID))
		}
		obj.SetParents(validParents)

		children := obj.Children()
		validChildren := children[:0]
		for _, childID := range children {
			if database.validObjectID(childID) {
				validChildren = append(validChildren, childID)
				continue
			}
			database.recordStartupRepair(fmt.Sprintf("#%d.child = #%d <invalid> ... removed", obj.ID(), childID))
		}
		obj.SetChildren(validChildren)

		if obj.Location() != types.ObjNothing && !database.validObjectID(obj.Location()) {
			database.recordStartupRepair(fmt.Sprintf("#%d.location = #%d <invalid> ... fixed", obj.ID(), obj.Location()))
			obj.SetLocation(types.ObjNothing)
		}

		contents := obj.Contents()
		validContents := contents[:0]
		for _, contentID := range contents {
			if database.validObjectID(contentID) {
				validContents = append(validContents, contentID)
				continue
			}
			database.recordStartupRepair(fmt.Sprintf("#%d.content = #%d <invalid> ... removed", obj.ID(), contentID))
		}
		obj.SetContents(validContents)
	}
}

func (database *Database) repairCycles() {
	parentCycles := make(map[types.ObjID]bool, len(database.Objects))
	locationCycles := make(map[types.ObjID]bool, len(database.Objects))
	for _, id := range database.sortedObjectIDs() {
		obj := database.Objects[id]
		if obj == nil {
			continue
		}
		parentCycles[obj.ID()] = hasParentCycle(database, obj.ID())
		locationCycles[obj.ID()] = hasLocationCycle(database, obj.ID())
	}

	for _, id := range database.sortedObjectIDs() {
		obj := database.Objects[id]
		if obj == nil {
			continue
		}
		if parentCycles[obj.ID()] {
			database.recordStartupRepair(fmt.Sprintf("Cycle in parent chain of #%d", obj.ID()))
			obj.SetParents(nil)
		}
		if locationCycles[obj.ID()] {
			database.recordStartupRepair(fmt.Sprintf("Cycle in location chain of #%d", obj.ID()))
			obj.SetLocation(types.ObjNothing)
		}
	}
}

func hasParentCycle(database *Database, start types.ObjID) bool {
	visited := make(map[types.ObjID]bool)
	var visit func(types.ObjID) bool
	visit = func(id types.ObjID) bool {
		obj := database.Objects[id]
		if obj == nil {
			return false
		}
		visited[id] = true
		for _, parentID := range obj.Parents() {
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

func hasLocationCycle(database *Database, start types.ObjID) bool {
	seen := make(map[types.ObjID]bool)
	current := start
	for current != types.ObjNothing {
		if seen[current] {
			return true
		}
		seen[current] = true
		obj := database.Objects[current]
		if obj == nil {
			return false
		}
		current = obj.Location()
	}
	return false
}

func (database *Database) repairTopDownInconsistencies() {
	for _, id := range database.sortedObjectIDs() {
		obj := database.Objects[id]
		if obj == nil {
			continue
		}

		if obj.Location() != types.ObjNothing {
			if location := database.Objects[obj.Location()]; location != nil && !containsObjID(location.Contents(), obj.ID()) {
				database.recordStartupRepair(fmt.Sprintf("#%d not in it's location's (#%d) contents", obj.ID(), obj.Location()))
				location.SetContents(appendUniqueObjID(location.Contents(), obj.ID()))
			}
		}

		for _, parentID := range obj.Parents() {
			if parent := database.Objects[parentID]; parent != nil && !containsObjID(parent.Children(), obj.ID()) {
				database.recordStartupRepair(fmt.Sprintf("#%d not in it's parent's (#%d) children", obj.ID(), parentID))
				parent.SetChildren(appendUniqueObjID(parent.Children(), obj.ID()))
			}
		}
	}
}

func (database *Database) repairBottomUpInconsistencies() {
	for _, id := range database.sortedObjectIDs() {
		obj := database.Objects[id]
		if obj == nil {
			continue
		}

		for _, childID := range obj.Children() {
			if child := database.Objects[childID]; child != nil && !containsObjID(child.Parents(), obj.ID()) {
				database.recordStartupRepair(fmt.Sprintf("#%d not in it's child's (#%d) parents", obj.ID(), childID))
				child.SetParents(appendUniqueObjID(child.Parents(), obj.ID()))
			}
		}

		for _, contentID := range obj.Contents() {
			if content := database.Objects[contentID]; content != nil && content.Location() != obj.ID() {
				database.recordStartupRepair(fmt.Sprintf("#%d not in it's content's (#%d) location", obj.ID(), contentID))
				content.SetLocation(obj.ID())
			}
		}
	}
}
