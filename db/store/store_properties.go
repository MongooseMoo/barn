package store

import (
	"github.com/MongooseMoo/barn/types"
	"maps"
	"strings"
)

func (s *Store) copyInheritedPropertiesLocked(parents []types.ObjID, owner types.ObjID) propTable {
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

		current := s.liveObjectLocked(currentID)
		if !validLiveObject(current) {
			continue
		}
		for name, prop := range current.properties.all() {
			// Keys are canonical on every object, so a direct hit suffices.
			if _, exists := result.lookup(name); exists {
				continue
			}
			result.put(name, inheritedSlot(prop, owner))
		}
		queue = append(queue, current.parents...)
	}

	return result
}

// inheritedSlot is the slot an object owned by owner gets for a property it
// inherits and has not written: clear, with no value of its own, as a loaded
// object's is. Reads take the value from the ancestor. With the c flag the
// slot belongs to the object's owner (Toast db_properties.cc:974).
func inheritedSlot(from Property, owner types.ObjID) Property {
	if !from.perms.Has(PropChown) {
		owner = from.owner
	}
	return Property{value: types.None, owner: owner, perms: from.perms, clear: true}
}

func propertyNameKey(name string) string {
	return strings.ToLower(name)
}

// PropertyNameKey exposes the canonical property-map key form for external
// consumers that index snapshot property maps (the dump writer resolves
// display-cased names from the ordered name list against canonical keys).
func PropertyNameKey(name string) string {
	return propertyNameKey(name)
}

// reseedInheritedPropertiesLocked rebuilds the inherited slots of obj, whose
// parents were oldParents, and of every descendant of obj.
//
// obj's slots are each rewritten at ts: a read of a clear slot depends on the
// ancestry it resolved through, so its version must change with it. A
// descendant loses the slots of the ancestors obj lost and gains slots of those
// obj gained, except ancestors it also reaches by a path that does not pass
// through obj, and keeps the rest as they are: a read that resolves a clear slot
// of it through obj records obj's rewritten slot. A descendant with no slot to
// lose or gain is left alone.
func (s *Store) reseedInheritedPropertiesLocked(obj *Object, oldParents []types.ObjID, ts uint64) {
	object := func(id types.ObjID) *Object {
		if o := s.liveObjectLocked(id); validLiveObject(o) {
			return o
		}
		return nil
	}
	subtree, inSubtree := inheritanceSubtree(obj.id, object)
	lost, gained := changedAncestors(oldParents, obj.parents, object)
	s.rebuildInheritedSlotsLocked(obj, obj.id, oldParents, object, ts)
	others := otherAncestors(obj.id, inSubtree, object)
	for _, id := range subtree[1:] {
		current, other := object(id), others(id)
		if !ancestorChangeTouches(current, lost, gained, other) {
			continue
		}
		current = s.republishForMutation(current)
		applyAncestorChange(current, obj, lost, gained, other, ts)
		stampObjectProperties(current, ts)
	}
}

// ancestorChangeTouches reports whether applyAncestorChange would change any
// slot of obj.
func ancestorChangeTouches(obj *Object, lost, gained map[types.ObjID][]string, other map[types.ObjID]bool) bool {
	for ancestorID, names := range lost {
		if other[ancestorID] {
			continue
		}
		for _, name := range names {
			if prop, ok := obj.properties.lookup(name); ok && !prop.defined {
				return true
			}
		}
	}
	for ancestorID, names := range gained {
		if !other[ancestorID] && len(names) > 0 {
			return true
		}
	}
	return false
}

// rebuildInheritedSlotsLocked rebuilds current's inherited slots from its
// parents, keeping those of ancestors common to its old and new ancestry, and
// rewrites each at ts.
func (s *Store) rebuildInheritedSlotsLocked(current *Object, moved types.ObjID, movedOldParents []types.ObjID, object func(types.ObjID) *Object, ts uint64) {
	newProps := s.copyInheritedPropertiesLocked(current.parents, current.owner)
	keepCommonAncestorSlots(current, newProps, keptInheritedNames(current.id, moved, movedOldParents, object))
	for name, prop := range newProps.all() {
		if !prop.defined {
			prop.version = ts
			newProps.put(name, prop)
		}
	}
	current.properties = newProps
}

// applyAncestorChange removes obj's slots of the properties defined by the
// ancestors in lost, and gives it a new clear slot, at ts and from moved's
// slot, of each property defined by those in gained, skipping the ancestors in
// other. Slots obj defines stay.
func applyAncestorChange(obj, moved *Object, lost, gained map[types.ObjID][]string, other map[types.ObjID]bool, ts uint64) {
	for ancestorID, names := range lost {
		if other[ancestorID] {
			continue
		}
		for _, name := range names {
			if prop, ok := obj.properties.lookup(name); ok && !prop.defined {
				obj.properties.remove(name)
			}
		}
	}
	for ancestorID, names := range gained {
		if other[ancestorID] {
			continue
		}
		for _, name := range names {
			from, ok := moved.properties.lookup(name)
			if !ok {
				continue
			}
			if prop, ok := obj.properties.lookup(name); ok && prop.defined {
				continue
			}
			slot := inheritedSlot(from, obj.owner)
			slot.version = ts
			obj.properties.put(name, slot)
		}
	}
}

// otherAncestors returns, for an object in moved's subtree (inSubtree), the
// ancestors it reaches through a parent outside the subtree. Results are
// memoized and shared between objects; callers must not modify them.
func otherAncestors(moved types.ObjID, inSubtree map[types.ObjID]bool, object func(types.ObjID) *Object) func(types.ObjID) map[types.ObjID]bool {
	memo := map[types.ObjID]map[types.ObjID]bool{moved: nil}
	outside := make(map[types.ObjID]map[types.ObjID]bool)
	var others func(types.ObjID) map[types.ObjID]bool
	others = func(id types.ObjID) map[types.ObjID]bool {
		if set, ok := memo[id]; ok {
			return set
		}
		var result map[types.ObjID]bool
		shared := true
		current := object(id)
		if current != nil {
			for _, parentID := range current.parents {
				var set map[types.ObjID]bool
				if inSubtree[parentID] {
					set = others(parentID)
				} else {
					if outside[parentID] == nil {
						outside[parentID] = ancestorSet([]types.ObjID{parentID}, object)
					}
					set = outside[parentID]
				}
				switch {
				case len(set) == 0:
				case result == nil:
					result = set
				default:
					if shared {
						result = maps.Clone(result)
						shared = false
					}
					maps.Copy(result, set)
				}
			}
		}
		memo[id] = result
		return result
	}
	return others
}

// inheritanceSubtree returns id and its descendants, children and anonymous
// children, each once and after the parent it was first reached from, and the
// same objects as a set.
func inheritanceSubtree(id types.ObjID, object func(types.ObjID) *Object) ([]types.ObjID, map[types.ObjID]bool) {
	subtree := []types.ObjID{id}
	in := map[types.ObjID]bool{id: true}
	for i := 0; i < len(subtree); i++ {
		current := object(subtree[i])
		if current == nil {
			continue
		}
		for _, childID := range append(append([]types.ObjID(nil), current.children...), current.anonymousChildren...) {
			if !in[childID] && object(childID) != nil {
				in[childID] = true
				subtree = append(subtree, childID)
			}
		}
	}
	return subtree, in
}

// changedAncestors returns the ancestors that an object moving from oldParents
// to newParents loses, and those it gains, each with the names of the
// properties it defines.
func changedAncestors(oldParents, newParents []types.ObjID, object func(types.ObjID) *Object) (lost, gained map[types.ObjID][]string) {
	oldAncestry, newAncestry := ancestorSet(oldParents, object), ancestorSet(newParents, object)
	only := func(ancestry, other map[types.ObjID]bool) map[types.ObjID][]string {
		defs := make(map[types.ObjID][]string)
		for ancestorID := range ancestry {
			ancestor := object(ancestorID)
			if other[ancestorID] || ancestor == nil {
				continue
			}
			for name, prop := range ancestor.properties.all() {
				if prop.defined {
					defs[ancestorID] = append(defs[ancestorID], name)
				}
			}
		}
		return defs
	}
	return only(oldAncestry, newAncestry), only(newAncestry, oldAncestry)
}

// ancestorSet returns parents and all their ancestors.
func ancestorSet(parents []types.ObjID, object func(types.ObjID) *Object) map[types.ObjID]bool {
	seen := make(map[types.ObjID]bool)
	queue := append([]types.ObjID(nil), parents...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		if current := object(id); current != nil {
			queue = append(queue, current.parents...)
		}
	}
	return seen
}

// keptInheritedNames returns the names of the properties defined by an
// ancestor of id in both its old and new ancestry, where the old ancestry has
// moved's parents as movedOldParents. id keeps its slots for these across the
// change of parents (Toast db_properties.cc:950-962). Another ancestor
// defining the same name does not count.
func keptInheritedNames(id, moved types.ObjID, movedOldParents []types.ObjID, object func(types.ObjID) *Object) map[string]bool {
	ancestry := func(old bool) map[types.ObjID]bool {
		seen := make(map[types.ObjID]bool)
		queue := []types.ObjID{id}
		for len(queue) > 0 {
			currentID := queue[0]
			queue = queue[1:]
			parents := movedOldParents
			if !old || currentID != moved {
				current := object(currentID)
				if current == nil {
					continue
				}
				parents = current.parents
			}
			for _, parentID := range parents {
				if !seen[parentID] {
					seen[parentID] = true
					queue = append(queue, parentID)
				}
			}
		}
		return seen
	}
	oldAncestry := ancestry(true)
	kept := make(map[string]bool)
	for ancestorID := range ancestry(false) {
		ancestor := object(ancestorID)
		if !oldAncestry[ancestorID] || ancestor == nil {
			continue
		}
		for name, prop := range ancestor.properties.all() {
			if prop.defined {
				kept[name] = true
			}
		}
	}
	return kept
}

// keepCommonAncestorSlots puts into newProps, built from obj's new parents,
// obj's existing slot for each name in kept, and every slot obj defines.
func keepCommonAncestorSlots(obj *Object, newProps propTable, kept map[string]bool) {
	for name, prop := range obj.properties.all() {
		if prop.defined {
			newProps.put(name, prop)
			continue
		}
		if _, inherited := newProps.lookup(name); inherited && kept[name] {
			newProps.put(name, prop)
		}
	}
}

func (s *Store) findPropertyLocked(objID types.ObjID, name string) (string, Property, types.ErrorCode) {
	// Fast path: every object's map already carries its inherited properties
	// (copyInheritedPropertiesLocked seeds them as clear entries), so a
	// non-clear hit on the object itself is the final answer — identical to the
	// BFS's first iteration, without the per-call visited map and queue. Only a
	// clear entry (value lives on an ancestor) or a miss needs the walk.
	if self := s.liveObjectLocked(objID); validLiveObject(self) {
		if actualName, prop, ok := self.properties.find(name); ok && !prop.clear {
			return actualName, prop, types.E_NONE
		}
	}

	var targetProp Property
	var targetName string
	targetFound := false
	visited := make(map[types.ObjID]bool)
	queue := []types.ObjID{objID}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		current := s.liveObjectLocked(currentID)
		if !validLiveObject(current) {
			continue
		}

		if actualName, prop, ok := current.properties.find(name); ok {
			if !targetFound {
				targetProp = prop
				targetName = actualName
				targetFound = true
			}
			if !prop.clear {
				if targetProp.clear {
					targetProp.value = prop.value
					targetProp.clear = false
					return targetName, targetProp, types.E_NONE
				}
				return actualName, prop, types.E_NONE
			}
		}

		queue = append(queue, current.parents...)
	}

	return "", Property{}, types.E_PROPNF
}

func validLiveObject(obj *Object) bool {
	return obj != nil && !obj.recycled && !obj.flags.Has(FlagInvalid)
}

// DefinedProperty returns a read-only view of a property defined directly on the
// object.
func (s *Store) DefinedProperty(objID types.ObjID, name string) (PropertyView, bool, types.ErrorCode) {
	prop, ok, err := s.DirectTxn().LocalProperty(objID, name)
	if err != types.E_NONE || !ok || !prop.Defined {
		return PropertyView{}, false, err
	}
	return prop, true, types.E_NONE
}

// setPropertyInfo updates owner and/or permissions on a local property slot.
func (s *Store) setPropertyInfo(objID types.ObjID, name string, owner *types.ObjID, perms *PropertyPerms) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	actualName, prop, ok := obj.properties.find(name)
	if !ok {
		return types.E_PROPNF
	}
	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	s.noteWaifRootsChanged()
	if owner != nil {
		prop.owner = *owner
	}
	if perms != nil {
		prop.perms = *perms
	}
	prop.version = ts
	obj.properties.put(actualName, prop)
	stampObjectProperties(obj, ts)
	return types.E_NONE
}

// setPropertyValue updates an existing local property slot or creates a local
// override for an inherited property.
func (s *Store) setPropertyValue(objID types.ObjID, name string, value types.Value) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	s.noteWaifRootsChanged()
	if actualName, prop, ok := obj.properties.find(name); ok {
		prop.clear = false
		prop.value = value
		prop.version = ts
		obj.properties.put(actualName, prop)
		stampObjectProperties(obj, ts)
		return types.E_NONE
	}

	inheritedName, inherited, err := s.findPropertyLocked(objID, name)
	if err != types.E_NONE {
		return err
	}
	obj.properties.put(inheritedName, Property{
		value:   value,
		owner:   inherited.owner,
		perms:   inherited.perms,
		clear:   false,
		defined: false,
		version: ts,
	})
	stampObjectProperties(obj, ts)
	return types.E_NONE
}

// defineProperty adds a new property definition to an object and propagates
// inherited clear slots to existing descendants.
func (s *Store) defineProperty(objID types.ObjID, name string, prop Property) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.definePropertyLocked(objID, name, prop)
}

func (s *Store) definePropertyLocked(objID types.ObjID, name string, prop Property) types.ErrorCode {
	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	if _, _, exists := obj.properties.find(name); exists {
		return types.E_INVARG
	}
	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	s.noteWaifRootsChanged()
	prop.defined = true
	prop.clear = false
	prop.version = ts
	if prop.perms.Has(PropChown) {
		prop.owner = obj.owner
	}

	obj.properties.put(propertyNameKey(name), prop)

	pos := obj.propDefsCount
	if pos > len(obj.propOrder) {
		pos = len(obj.propOrder)
	}
	obj.propOrder = append(obj.propOrder, "")
	copy(obj.propOrder[pos+1:], obj.propOrder[pos:])
	obj.propOrder[pos] = name
	obj.propDefsCount++
	stampObjectProperties(obj, ts)

	s.propagatePropertyToDescendantsLocked(liveImages{s}, objID, name, prop, ts)
	return types.E_NONE
}

// deleteDefinedProperty removes a property defined directly on an object and
// removes inherited copies from descendants.
func (s *Store) deleteDefinedProperty(objID types.ObjID, name string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.deleteDefinedPropertyLocked(objID, name)
}

func (s *Store) deleteDefinedPropertyLocked(objID types.ObjID, name string) types.ErrorCode {
	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	actualName, prop, ok := obj.properties.find(name)
	if !ok || !prop.defined {
		return types.E_PROPNF
	}
	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	s.noteWaifRootsChanged()

	obj.properties.remove(actualName)
	// propOrder keeps display case while actualName is the canonical map key,
	// so the order entry is removed case-insensitively.
	obj.propOrder = removeStringFold(obj.propOrder, actualName)
	if obj.propDefsCount > 0 {
		obj.propDefsCount--
	}
	stampObjectProperties(obj, ts)
	s.removeInheritedPropertyLocked(liveImages{s}, objID, actualName, ts)
	return types.E_NONE
}

// clearPropertyOverride removes a local inherited-property slot so reads fall
// through to the parent chain.
func (s *Store) clearPropertyOverride(objID types.ObjID, name string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	actualName, _, ok := obj.properties.find(name)
	if ok {
		obj = s.republishForMutation(obj)
		ts := s.bumpClockLocked()
		s.noteWaifRootsChanged()
		obj.properties.remove(actualName)
		stampObjectProperties(obj, ts)
	}
	return types.E_NONE
}

// objectImages is what a store-side propagation walk reads objects through and
// takes a changeable image of an object from.
type objectImages interface {
	view(id types.ObjID) *Object
	own(obj *Object) *Object
}

// liveImages serves a direct store write: each object the walk changes is
// republished on its own.
type liveImages struct{ s *Store }

func (l liveImages) view(id types.ObjID) *Object { return l.s.liveObjectLocked(id) }
func (l liveImages) own(obj *Object) *Object     { return l.s.republishForMutation(obj) }

func (s *Store) propagatePropertyToDescendantsLocked(images objectImages, objID types.ObjID, name string, prop Property, ts uint64) {
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		current := images.view(currentID)
		if !validLiveObject(current) {
			continue
		}
		for _, childID := range current.children {
			child := images.view(childID)
			if !validLiveObject(child) {
				continue
			}
			if actualName, existing, ok := child.properties.find(name); ok {
				if existing.defined {
					queue = append(queue, childID)
					continue
				}
				child = images.own(child)
				child.properties.remove(actualName)
			} else {
				child = images.own(child)
			}
			slot := inheritedSlot(prop, child.owner)
			slot.version = ts
			child.properties.put(propertyNameKey(name), slot)
			stampObjectProperties(child, ts)
			queue = append(queue, childID)
		}
		// Anonymous children get the slot too (Toast insert_prop_recursively,
		// db_properties.cc:162-166, :176-179).
		for _, anonID := range current.anonymousChildren {
			anon := images.view(anonID)
			if !validLiveObject(anon) {
				continue
			}
			if actualName, existing, ok := anon.properties.find(name); ok {
				if existing.defined {
					continue
				}
				anon = images.own(anon)
				anon.properties.remove(actualName)
			} else {
				anon = images.own(anon)
			}
			slot := inheritedSlot(prop, anon.owner)
			slot.version = ts
			anon.properties.put(propertyNameKey(name), slot)
			stampObjectProperties(anon, ts)
		}
	}
}

func (s *Store) removeInheritedPropertyLocked(images objectImages, objID types.ObjID, name string, ts uint64) {
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		current := images.view(currentID)
		if !validLiveObject(current) {
			continue
		}
		for _, childID := range current.children {
			child := images.view(childID)
			if !validLiveObject(child) {
				continue
			}
			if actualName, prop, ok := child.properties.find(name); ok && !prop.defined {
				child = images.own(child)
				child.properties.remove(actualName)
				stampObjectProperties(child, ts)
			}
			queue = append(queue, childID)
		}
		// Anonymous children lose the slot too (Toast remove_prop_recursively,
		// db_properties.cc:325-328, :340-341).
		for _, anonID := range current.anonymousChildren {
			anon := images.view(anonID)
			if !validLiveObject(anon) {
				continue
			}
			if actualName, prop, ok := anon.properties.find(name); ok && !prop.defined {
				anon = images.own(anon)
				anon.properties.remove(actualName)
				stampObjectProperties(anon, ts)
			}
		}
	}
}

// removeStringFold removes entries matching value case-insensitively. Used for
// propOrder maintenance, where entries keep display case but callers hold the
// canonical (lowercase) property key.
func removeStringFold(items []string, value string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !strings.EqualFold(item, value) {
			result = append(result, item)
		}
	}
	return result
}
