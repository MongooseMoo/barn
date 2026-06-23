package store

import (
	"barn/types"
	"strings"
)

func (s *Store) copyInheritedPropertiesLocked(parents []types.ObjID) map[string]*Property {
	result := make(map[string]*Property)
	visited := make(map[types.ObjID]bool)
	queue := append([]types.ObjID(nil), parents...)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		current := s.objects[currentID]
		if !validLiveObject(current) {
			continue
		}
		for name, prop := range current.properties {
			if _, _, exists := propertyByName(result, name); exists {
				continue
			}
			result[name] = &Property{
				name:  prop.name,
				value: prop.value,
				owner: prop.owner,
				perms: prop.perms,
				clear: true,
			}
		}
		queue = append(queue, current.parents...)
	}

	return result
}

func propertyNameKey(name string) string {
	return strings.ToLower(name)
}

func propertyByName(properties map[string]*Property, name string) (string, *Property, bool) {
	if prop, ok := properties[name]; ok {
		return name, prop, true
	}
	for propName, prop := range properties {
		if strings.EqualFold(propName, name) {
			return propName, prop, true
		}
	}
	return "", nil, false
}

func (s *Store) reseedInheritedPropertiesLocked(obj *Object) {
	newProps := s.copyInheritedPropertiesLocked(obj.parents)
	for name, prop := range obj.properties {
		if prop.defined {
			newProps[name] = prop
		}
	}
	obj.properties = newProps
}

// FindProperty resolves a property (with inheritance) and returns a flat,
// read-only PropertyView value. The store never hands out a live *Property to
// external callers.
func (s *Store) FindProperty(objID types.ObjID, name string) (PropertyView, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prop, errCode := s.findPropertyLocked(objID, name)
	if errCode != types.E_NONE {
		return PropertyView{}, errCode
	}
	return prop.View(), types.E_NONE
}

func (s *Store) findPropertyLocked(objID types.ObjID, name string) (*Property, types.ErrorCode) {
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

		current := s.objects[currentID]
		if !validLiveObject(current) {
			continue
		}

		if _, prop, ok := propertyByName(current.properties, name); ok {
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
		}

		queue = append(queue, current.parents...)
	}

	return nil, types.E_PROPNF
}

func validLiveObject(obj *Object) bool {
	return obj != nil && !obj.recycled && !obj.flags.Has(FlagInvalid)
}

func cloneProperty(prop *Property) *Property {
	if prop == nil {
		return nil
	}
	clone := *prop
	return &clone
}

// DefinedPropertyNames returns properties defined directly on an object in
// definition order.

func (s *Store) DefinedPropertyNames(objID types.ObjID) ([]string, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}

	names := make([]string, 0, len(obj.properties))
	for _, name := range obj.propOrder {
		prop := obj.properties[name]
		if prop != nil && prop.defined {
			names = append(names, name)
		}
	}
	return names, types.E_NONE
}

// DefinedPropertyNamesInAncestry returns every property name defined on objID
// or its ancestors.

func (s *Store) DefinedPropertyNamesInAncestry(objID types.ObjID) (map[string]bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !validLiveObject(s.objects[objID]) {
		return nil, types.E_INVIND
	}
	return s.definedPropertyNamesInAncestryLocked([]types.ObjID{objID}), types.E_NONE
}

func (s *Store) definedPropertyNamesInAncestryLocked(start []types.ObjID) map[string]bool {
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

		current := s.objects[currentID]
		if !validLiveObject(current) {
			continue
		}
		for name, prop := range current.properties {
			if prop != nil && prop.defined {
				names[propertyNameKey(name)] = true
			}
		}
		queue = append(queue, current.parents...)
	}

	return names
}

func (s *Store) HasDuplicateDefinedPropertyAmong(ids []types.ObjID) (bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	for _, id := range ids {
		obj := s.objects[id]
		if !validLiveObject(obj) {
			return false, types.E_INVARG
		}
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

func (s *Store) HasDefinedPropertyConflictWithAncestry(objID types.ObjID, parentIDs []types.ObjID) (bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	for _, parentID := range parentIDs {
		if !validLiveObject(s.objects[parentID]) {
			return false, types.E_INVARG
		}
	}

	ancestorNames := s.definedPropertyNamesInAncestryLocked(parentIDs)
	for name, prop := range obj.properties {
		if prop != nil && prop.defined && ancestorNames[propertyNameKey(name)] {
			return true, types.E_NONE
		}
	}
	return false, types.E_NONE
}

func (s *Store) HasChparentDescendantPropertyConflict(objID types.ObjID, names map[string]bool) (bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
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
		for childID := range current.chparentChildren {
			child := s.objects[childID]
			if !validLiveObject(child) {
				continue
			}
			for name, prop := range child.properties {
				if prop != nil && prop.defined && names[propertyNameKey(name)] {
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

func (s *Store) PropertyValue(objID types.ObjID, name string) (types.Value, types.ErrorCode) {
	prop, errCode := s.FindProperty(objID, name)
	if errCode != types.E_NONE {
		return nil, errCode
	}
	return prop.Value, types.E_NONE
}

func (s *Store) PropertyValues(objID types.ObjID) ([]types.Value, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}

	values := make([]types.Value, 0, len(obj.properties))
	for _, prop := range obj.properties {
		if prop != nil {
			values = append(values, prop.value)
		}
	}
	return values, types.E_NONE
}

func (s *Store) TruthyPropertiesWithPrefixInAncestry(objID types.ObjID, prefix string) (map[string]bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !validLiveObject(s.objects[objID]) {
		return nil, types.E_INVIND
	}

	result := make(map[string]bool)
	seenObjects := make(map[types.ObjID]bool)
	decidedNames := make(map[string]bool)
	queue := []types.ObjID{objID}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seenObjects[currentID] {
			continue
		}
		seenObjects[currentID] = true

		current := s.objects[currentID]
		if !validLiveObject(current) {
			continue
		}
		for propName, prop := range current.properties {
			if prop == nil || !strings.HasPrefix(strings.ToLower(propName), strings.ToLower(prefix)) {
				continue
			}
			name := propName[len(prefix):]
			if name == "" || decidedNames[name] {
				continue
			}
			if prop.clear {
				continue
			}
			decidedNames[name] = true
			if prop.value != nil && prop.value.Truthy() {
				result[name] = true
			}
		}
		queue = append(queue, current.parents...)
	}

	return result, types.E_NONE
}

// LocalProperty returns a flat read-only view of a property slot defined or
// inherited directly on objID (not resolved up the parent chain).
func (s *Store) LocalProperty(objID types.ObjID, name string) (PropertyView, bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return PropertyView{}, false, types.E_INVIND
	}
	_, prop, ok := propertyByName(obj.properties, name)
	if !ok {
		return PropertyView{}, false, types.E_NONE
	}
	return prop.View(), true, types.E_NONE
}

// DefinedProperty returns a read-only view of a property defined directly on the
// object.

func (s *Store) DefinedProperty(objID types.ObjID, name string) (PropertyView, bool, types.ErrorCode) {
	prop, ok, err := s.LocalProperty(objID, name)
	if err != types.E_NONE || !ok || !prop.Defined {
		return PropertyView{}, false, err
	}
	return prop, true, types.E_NONE
}

func (s *Store) HasLocalProperty(objID types.ObjID, name string) (bool, types.ErrorCode) {
	_, ok, err := s.LocalProperty(objID, name)
	return ok, err
}

func (s *Store) IsPropertyDefinedOnObject(objID types.ObjID, name string) (bool, types.ErrorCode) {
	_, ok, err := s.DefinedProperty(objID, name)
	return ok, err
}

// PropertyClearState reports whether an existing inherited property is clear on
// the target object. A missing local slot means the property is inherited.

func (s *Store) PropertyClearState(objID types.ObjID, name string) (bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	_, prop, exists := propertyByName(obj.properties, name)
	if !exists {
		return true, types.E_NONE
	}
	if prop.defined {
		return false, types.E_NONE
	}
	return prop.clear, types.E_NONE
}

// SetPropertyInfo updates owner and/or permissions on a local property slot.

func (s *Store) SetPropertyInfo(objID types.ObjID, name string, owner *types.ObjID, perms *PropertyPerms) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	_, prop, ok := propertyByName(obj.properties, name)
	if !ok {
		return types.E_PROPNF
	}
	if owner != nil {
		prop.owner = *owner
	}
	if perms != nil {
		prop.perms = *perms
	}
	return types.E_NONE
}

// SetPropertyValue updates an existing local property slot or creates a local
// override for an inherited property.

func (s *Store) SetPropertyValue(objID types.ObjID, name string, value types.Value) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if _, prop, ok := propertyByName(obj.properties, name); ok {
		prop.clear = false
		prop.value = value
		return types.E_NONE
	}

	inherited, err := s.findPropertyLocked(objID, name)
	if err != types.E_NONE {
		return err
	}
	obj.properties[inherited.name] = &Property{
		name:    inherited.name,
		value:   value,
		owner:   inherited.owner,
		perms:   inherited.perms,
		clear:   false,
		defined: false,
	}
	return types.E_NONE
}

// DefineProperty adds a new property definition to an object and propagates
// inherited clear slots to existing descendants.

func (s *Store) DefineProperty(objID types.ObjID, prop Property) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if _, _, exists := propertyByName(obj.properties, prop.name); exists {
		return types.E_INVARG
	}
	prop.defined = true
	prop.clear = false
	obj.properties[prop.name] = cloneProperty(&prop)

	pos := obj.propDefsCount
	if pos > len(obj.propOrder) {
		pos = len(obj.propOrder)
	}
	obj.propOrder = append(obj.propOrder, "")
	copy(obj.propOrder[pos+1:], obj.propOrder[pos:])
	obj.propOrder[pos] = prop.name
	obj.propDefsCount++

	s.propagatePropertyToDescendantsLocked(objID, &prop)
	return types.E_NONE
}

// DeleteDefinedProperty removes a property defined directly on an object and
// removes inherited copies from descendants.

func (s *Store) DeleteDefinedProperty(objID types.ObjID, name string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	actualName, prop, ok := propertyByName(obj.properties, name)
	if !ok || !prop.defined {
		return types.E_PROPNF
	}

	delete(obj.properties, actualName)
	obj.propOrder = removeString(obj.propOrder, actualName)
	if obj.propDefsCount > 0 {
		obj.propDefsCount--
	}
	s.removeInheritedPropertyLocked(objID, actualName)
	return types.E_NONE
}

// ClearPropertyOverride removes a local inherited-property slot so reads fall
// through to the parent chain.

func (s *Store) ClearPropertyOverride(objID types.ObjID, name string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	actualName, _, ok := propertyByName(obj.properties, name)
	if ok {
		delete(obj.properties, actualName)
	}
	return types.E_NONE
}

func (s *Store) HasDefinedPropertyInDescendants(objID types.ObjID, name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		current := s.objects[currentID]
		if !validLiveObject(current) {
			continue
		}
		for _, childID := range current.children {
			child := s.objects[childID]
			if !validLiveObject(child) {
				continue
			}
			if _, prop, ok := propertyByName(child.properties, name); ok && prop.defined {
				return true
			}
			queue = append(queue, childID)
		}
	}
	return false
}

func (s *Store) ResetInheritedProperties(objID types.ObjID) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	for name, prop := range obj.properties {
		if !prop.defined {
			delete(obj.properties, name)
		}
	}
	return types.E_NONE
}

func (s *Store) propagatePropertyToDescendantsLocked(objID types.ObjID, prop *Property) {
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		current := s.objects[currentID]
		if !validLiveObject(current) {
			continue
		}
		for _, childID := range current.children {
			child := s.objects[childID]
			if !validLiveObject(child) {
				continue
			}
			if actualName, existing, ok := propertyByName(child.properties, prop.name); ok {
				if existing.defined {
					queue = append(queue, childID)
					continue
				}
				delete(child.properties, actualName)
			}
			child.properties[prop.name] = &Property{
				name:  prop.name,
				value: prop.value,
				owner: prop.owner,
				perms: prop.perms,
				clear: true,
			}
			queue = append(queue, childID)
		}
	}
}

func (s *Store) removeInheritedPropertyLocked(objID types.ObjID, name string) {
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		current := s.objects[currentID]
		if !validLiveObject(current) {
			continue
		}
		for _, childID := range current.children {
			child := s.objects[childID]
			if !validLiveObject(child) {
				continue
			}
			if actualName, prop, ok := propertyByName(child.properties, name); ok && !prop.defined {
				delete(child.properties, actualName)
			}
			queue = append(queue, childID)
		}
	}
}

func removeString(items []string, value string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item != value {
			result = append(result, item)
		}
	}
	return result
}

// matchVerbName checks if a search name matches a MOO verb name pattern
// Supports MOO wildcard matching where * marks the minimum abbreviation point
// Example: "co*nnect" matches "co", "con", "conn", "conne", "connec", "connect"
//   - Must type at least "co" (prefix before *)
//   - Can type any prefix of the full name "connect"
//
// Example: "get_conj*ugation" matches "get_conj", "get_conju", ..., "get_conjugation"
