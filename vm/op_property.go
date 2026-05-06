package vm

import (
	"barn/db"
	"barn/types"
	"fmt"
	"strings"
)

// Property operations

func (vm *VM) executeGetProp() error {
	propNameIdx := vm.ReadByte()

	// Determine property name: static (from constant pool) or dynamic (from stack)
	var propName string
	if propNameIdx == 0xFF {
		// Dynamic property: name is on top of stack
		nameVal := vm.Pop()
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("E_TYPE: dynamic property name must be a string")
		}
		propName = strVal.Value()
	} else {
		// Static property: name from constant pool
		nameVal := vm.CurrentFrame().Program.Constants[propNameIdx]
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("internal error: property name constant is not a string")
		}
		propName = strVal.Value()
	}

	// Pop the object
	objVal := vm.Pop()

	// Check if it's a waif (must check before ObjValue since waifs are a different type)
	if waifVal, ok := objVal.(types.WaifValue); ok {
		return vm.vmGetWaifProp(waifVal, propName)
	}

	// Check if it's an object reference
	objRef, ok := objVal.(types.ObjValue)
	if !ok {
		return fmt.Errorf("E_TYPE: property access requires an object")
	}

	objID := objRef.ID()

	// Need a store to look up properties
	if vm.Store == nil {
		return fmt.Errorf("E_INVIND: no object store available")
	}

	obj := vm.Store.Get(objID)
	if obj == nil {
		return fmt.Errorf("E_INVIND: invalid object #%d", objID)
	}

	// Look up defined property first (with inheritance via breadth-first search)
	prop, errCode := vmFindProperty(vm.Store, obj, propName)
	if errCode == types.E_NONE {
		// Check read permission
		if err := vm.checkPropertyReadPerm(prop); err != nil {
			return err
		}
		vm.Push(prop.Value)
		return nil
	}

	// Check for built-in properties (flag properties like .name, .owner, .wizard, etc.)
	if val, ok := vmGetBuiltinProperty(obj, propName); ok {
		vm.Push(val)
		return nil
	}

	// Property not found
	return fmt.Errorf("E_PROPNF: property not found: %s", propName)
}

// vmGetWaifProp handles property read on a waif value.
// Mirrors the tree-walker's waifProperty() logic.
func (vm *VM) vmGetWaifProp(waif types.WaifValue, propName string) error {
	// Special waif properties
	switch propName {
	case "owner":
		vm.Push(types.NewObj(waif.Owner()))
		return nil
	case "class":
		classID := waif.Class()
		// Check if class object has been recycled
		if vm.Store != nil {
			classObj := vm.Store.Get(classID)
			if classObj == nil {
				// Class has been recycled - return #-1
				vm.Push(types.NewObj(types.ObjNothing))
				return nil
			}
		}
		vm.Push(types.NewObj(classID))
		return nil
	}

	// Check waif's own properties first
	if val, ok := waif.GetProperty(propName); ok {
		vm.Push(val)
		return nil
	}

	// Fall back to waif instance properties defined on the class with a colon prefix.
	if vm.Store == nil {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	classID := waif.Class()
	classObj := vm.Store.Get(classID)
	if classObj == nil {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	classPropName := propName
	if !strings.HasPrefix(classPropName, ":") {
		classPropName = ":" + classPropName
	}
	prop, errCode := vmFindProperty(vm.Store, classObj, classPropName)
	if errCode != types.E_NONE {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	vm.Push(prop.Value)
	return nil
}

func (vm *VM) executeSetProp() error {
	propNameIdx := vm.ReadByte()

	// Determine property name: static (from constant pool) or dynamic (from stack)
	var propName string
	if propNameIdx == 0xFF {
		// Dynamic property: name is on top of stack, then obj, then value_copy
		nameVal := vm.Pop()
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("E_TYPE: dynamic property name must be a string")
		}
		propName = strVal.Value()
	} else {
		// Static property: name from constant pool
		nameVal := vm.CurrentFrame().Program.Constants[propNameIdx]
		strVal, ok := nameVal.(types.StrValue)
		if !ok {
			return fmt.Errorf("internal error: property name constant is not a string")
		}
		propName = strVal.Value()
	}

	// Pop the object
	objVal := vm.Pop()

	// Pop the value to assign
	value := vm.Pop()

	// Check if it's a waif (must check before ObjValue since waifs are a different type)
	if waifVal, ok := objVal.(types.WaifValue); ok {
		return vm.vmSetWaifProp(waifVal, propName, value)
	}

	// Check if it's an object reference
	objRef, ok := objVal.(types.ObjValue)
	if !ok {
		return fmt.Errorf("E_TYPE: property assignment requires an object")
	}

	objID := objRef.ID()

	// Need a store to set properties
	if vm.Store == nil {
		return fmt.Errorf("E_INVIND: no object store available")
	}

	obj := vm.Store.Get(objID)
	if obj == nil {
		return fmt.Errorf("E_INVIND: invalid object #%d", objID)
	}

	// Check for built-in property assignment first
	if isBuiltin, errCode := vmSetBuiltinProperty(obj, propName, value, vm.Context); isBuiltin {
		if errCode != types.E_NONE {
			return fmt.Errorf("%s: cannot set built-in property %s", errCode, propName)
		}
		return nil
	}

	// Check if property exists directly on this object
	prop, ok := obj.Properties[propName]
	if ok {
		// Check write permission
		if err := vm.checkPropertyWritePerm(prop); err != nil {
			return err
		}
		// Property exists locally - update it
		prop.Clear = false
		prop.Value = value
		return nil
	}

	// Property not on this object - check if inherited
	inheritedProp, errCode := vmFindProperty(vm.Store, obj, propName)
	if errCode != types.E_NONE {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	// Check write permission on the inherited property
	if err := vm.checkPropertyWritePerm(inheritedProp); err != nil {
		return err
	}

	// Property is inherited - create a local copy with the new value
	newProp := &db.Property{
		Name:    propName,
		Value:   value,
		Owner:   inheritedProp.Owner,
		Perms:   inheritedProp.Perms,
		Clear:   false,
		Defined: false,
	}
	obj.Properties[propName] = newProp

	return nil
}

// vmSetWaifProp handles property assignment on a waif value.
// Mirrors the tree-walker's assignWaifProperty() logic.
func (vm *VM) vmSetWaifProp(waif types.WaifValue, propName string, value types.Value) error {
	// These properties cannot be set on waifs
	switch propName {
	case "owner", "class", "wizard", "programmer":
		return fmt.Errorf("E_PERM: cannot set .%s on a waif", propName)
	}

	// Check for self-reference (circular reference)
	if containsWaif(value, waif) {
		return fmt.Errorf("E_RECMOVE: value contains the waif itself")
	}

	// Set property on waif (creates a new waif with the property set)
	// Note: Waifs use copy-on-write semantics. The VM does not currently
	// propagate the new waif back to the source variable. This matches
	// the tree-walker's limitation for non-simple-identifier cases.
	_ = waif.SetProperty(propName, value)

	return nil
}

// checkPropertyReadPerm checks if the current programmer has read permission on a property.
// Wizards and property owners always have access.
func (vm *VM) checkPropertyReadPerm(prop *db.Property) error {
	if vm.Context == nil {
		return nil // No context = no permission check
	}
	if vm.Context.IsWizard {
		return nil
	}
	if vm.Context.Programmer == prop.Owner {
		return nil
	}
	if !prop.Perms.Has(db.PropRead) {
		return fmt.Errorf("E_PERM: property not readable")
	}
	return nil
}

// checkPropertyWritePerm checks if the current programmer has write permission on a property.
// Wizards and property owners always have access.
func (vm *VM) checkPropertyWritePerm(prop *db.Property) error {
	if vm.Context == nil {
		return nil // No context = no permission check
	}
	if vm.Context.IsWizard {
		return nil
	}
	if vm.Context.Programmer == prop.Owner {
		return nil
	}
	if !prop.Perms.Has(db.PropWrite) {
		return fmt.Errorf("E_PERM: property not writable")
	}
	return nil
}

// vmFindProperty finds a property on an object with inheritance (breadth-first search).
// Permission info (owner/perms) comes from the TARGET object's property entry,
// while the value comes from the first non-clear ancestor. This matches Toast's
// db_find_property where h.ptr always points to the original object's propval.
func vmFindProperty(store *db.Store, obj *db.Object, name string) (*db.Property, types.ErrorCode) {
	var targetProp *db.Property // target object's property entry (for owner/perms)

	queue := []types.ObjID{obj.ID}
	visited := make(map[types.ObjID]bool)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		current := store.Get(currentID)
		if current == nil {
			continue
		}

		prop, ok := current.Properties[name]
		if ok {
			// Save the first property entry found (the target object's) for permissions.
			if targetProp == nil {
				targetProp = prop
			}
			if !prop.Clear {
				if targetProp != prop {
					// Value from ancestor, but use target's owner/perms
					result := *targetProp
					result.Value = prop.Value
					result.Clear = false
					return &result, types.E_NONE
				}
				return prop, types.E_NONE
			}
		}

		queue = append(queue, current.Parents...)
	}

	return nil, types.E_PROPNF
}

// vmGetBuiltinProperty returns built-in object properties (name, owner, location, etc.).
// This mirrors the tree-walker's getBuiltinProperty logic.
func vmGetBuiltinProperty(obj *db.Object, name string) (types.Value, bool) {
	switch name {
	case "name":
		return types.NewStr(obj.Name), true
	case "owner":
		return types.NewObj(obj.Owner), true
	case "location":
		return types.NewObj(obj.Location), true
	case "contents":
		vals := make([]types.Value, len(obj.Contents))
		for i, id := range obj.Contents {
			vals[i] = types.NewObj(id)
		}
		return types.NewList(vals), true
	case "parents":
		vals := make([]types.Value, len(obj.Parents))
		for i, id := range obj.Parents {
			vals[i] = types.NewObj(id)
		}
		return types.NewList(vals), true
	case "parent":
		if len(obj.Parents) > 0 {
			return types.NewObj(obj.Parents[0]), true
		}
		return types.NewObj(types.ObjNothing), true
	case "children":
		vals := make([]types.Value, len(obj.Children))
		for i, id := range obj.Children {
			vals[i] = types.NewObj(id)
		}
		return types.NewList(vals), true
	case "programmer":
		if obj.Flags.Has(db.FlagProgrammer) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "wizard":
		if obj.Flags.Has(db.FlagWizard) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "player":
		if obj.Flags.Has(db.FlagUser) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "r":
		if obj.Flags.Has(db.FlagRead) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "w":
		if obj.Flags.Has(db.FlagWrite) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "f":
		if obj.Flags.Has(db.FlagFertile) {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	case "a":
		if obj.Flags.Has(db.FlagAnonymous) || obj.Anonymous {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	default:
		return nil, false
	}
}

// vmSetBuiltinProperty sets a built-in object property.
// This mirrors the tree-walker's setBuiltinProperty logic.
func vmSetBuiltinProperty(obj *db.Object, name string, value types.Value, ctx *types.TaskContext) (bool, types.ErrorCode) {
	switch name {
	case "name":
		if str, ok := value.(types.StrValue); ok {
			obj.Name = str.Value()
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "owner":
		if objVal, ok := value.(types.ObjValue); ok {
			if obj.Anonymous && ctx != nil && !ctx.IsWizard {
				return true, types.E_PERM
			}
			obj.Owner = objVal.ID()
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "location":
		if objVal, ok := value.(types.ObjValue); ok {
			obj.Location = objVal.ID()
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "programmer":
		if intVal, ok := value.(types.IntValue); ok {
			if obj.Anonymous {
				if ctx != nil && ctx.IsWizard {
					return true, types.E_INVARG
				}
				return true, types.E_PERM
			}
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagProgrammer)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagProgrammer)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "wizard":
		if intVal, ok := value.(types.IntValue); ok {
			if obj.Anonymous {
				if ctx != nil && ctx.IsWizard {
					return true, types.E_INVARG
				}
				return true, types.E_PERM
			}
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagWizard)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagWizard)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "player":
		if intVal, ok := value.(types.IntValue); ok {
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagUser)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagUser)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "r":
		if intVal, ok := value.(types.IntValue); ok {
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagRead)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagRead)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "w":
		if intVal, ok := value.(types.IntValue); ok {
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagWrite)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagWrite)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "f":
		if intVal, ok := value.(types.IntValue); ok {
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagFertile)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagFertile)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "a":
		if intVal, ok := value.(types.IntValue); ok {
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagAnonymous)
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagAnonymous)
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	default:
		return false, types.E_NONE
	}
}
