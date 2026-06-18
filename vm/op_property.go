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
		return vm.getWaifProp(waifVal, propName)
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
	prop, errCode := vm.Store.FindProperty(obj.ID, propName)
	if errCode == types.E_NONE {
		// Check read permission
		if err := vm.checkPropertyReadPerm(prop); err != nil {
			return err
		}
		vm.Push(prop.Value)
		return nil
	}

	// Check for built-in properties (flag properties like .name, .owner, .wizard, etc.)
	if val, ok := getBuiltinProperty(vm.Store, obj.ID, propName); ok {
		vm.Push(val)
		return nil
	}

	// Property not found
	return fmt.Errorf("E_PROPNF: property not found: %s", propName)
}

// getWaifProp handles property read on a waif value.
// Reads a waif property, falling back to the waif's class object.
func (vm *VM) getWaifProp(waif types.WaifValue, propName string) error {
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
	prop, errCode := vm.Store.FindProperty(classObj.ID, classPropName)
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
		return vm.setWaifProp(waifVal, propName, value)
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
	if isBuiltin, errCode := setBuiltinProperty(vm.Store, obj.ID, propName, value, vm.Context); isBuiltin {
		if errCode != types.E_NONE {
			return fmt.Errorf("%s: cannot set built-in property %s", errCode, propName)
		}
		return nil
	}

	prop, ok, errCode := vm.Store.LocalProperty(obj.ID, propName)
	if errCode != types.E_NONE {
		return fmt.Errorf("%s: invalid object #%d", errCode, obj.ID)
	}
	if ok {
		// Check write permission
		if err := vm.checkPropertyWritePerm(prop); err != nil {
			return err
		}
		if errCode := vm.Store.SetPropertyValue(obj.ID, propName, value); errCode != types.E_NONE {
			return fmt.Errorf("%s: property not set: %s", errCode, propName)
		}
		return nil
	}

	// Property not on this object - check if inherited
	inheritedProp, errCode := vm.Store.FindProperty(obj.ID, propName)
	if errCode != types.E_NONE {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	// Check write permission on the inherited property
	if err := vm.checkPropertyWritePerm(inheritedProp); err != nil {
		return err
	}

	if errCode := vm.Store.SetPropertyValue(obj.ID, propName, value); errCode != types.E_NONE {
		return fmt.Errorf("%s: property not set: %s", errCode, propName)
	}
	return nil
}

// setWaifProp handles property assignment on a waif value.
// Assigns a waif property and returns the copied waif value.
func (vm *VM) setWaifProp(waif types.WaifValue, propName string, value types.Value) error {
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
	// non-simple-identifier cases.
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

// getBuiltinProperty returns built-in object properties (name, owner, location, etc.).
func getBuiltinProperty(store *db.Store, objID types.ObjID, name string) (types.Value, bool) {
	switch name {
	case "name":
		name, errCode := store.ObjectName(objID)
		if errCode != types.E_NONE {
			return nil, false
		}
		return types.NewStr(name), true
	case "owner":
		ownerID, errCode := store.ObjectOwner(objID)
		if errCode != types.E_NONE {
			return nil, false
		}
		return types.NewObj(ownerID), true
	case "location":
		locationID, errCode := store.Location(objID)
		if errCode != types.E_NONE {
			return nil, false
		}
		return types.NewObj(locationID), true
	case "contents":
		contentsIDs, errCode := store.Contents(objID)
		if errCode != types.E_NONE {
			return nil, false
		}
		return types.NewList(objIDsToValues(contentsIDs)), true
	case "parents":
		parentIDs, errCode := store.Parents(objID)
		if errCode != types.E_NONE {
			return nil, false
		}
		return types.NewList(objIDsToValues(parentIDs)), true
	case "parent":
		parentID, errCode := store.Parent(objID)
		if errCode != types.E_NONE {
			return nil, false
		}
		return types.NewObj(parentID), true
	case "children":
		childIDs, errCode := store.Children(objID)
		if errCode != types.E_NONE {
			return nil, false
		}
		return types.NewList(objIDsToValues(childIDs)), true
	case "programmer":
		return boolPropertyValue(store, objID, db.FlagProgrammer)
	case "wizard":
		return boolPropertyValue(store, objID, db.FlagWizard)
	case "player":
		return boolPropertyValue(store, objID, db.FlagUser)
	case "r":
		return boolPropertyValue(store, objID, db.FlagRead)
	case "w":
		return boolPropertyValue(store, objID, db.FlagWrite)
	case "f":
		return boolPropertyValue(store, objID, db.FlagFertile)
	case "a":
		hasFlag, flagErr := store.HasObjectFlag(objID, db.FlagAnonymous)
		isAnonymous, anonErr := store.ObjectIsAnonymous(objID)
		if flagErr != types.E_NONE || anonErr != types.E_NONE {
			return nil, false
		}
		if hasFlag || isAnonymous {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	default:
		return nil, false
	}
}

func boolPropertyValue(store *db.Store, objID types.ObjID, flag db.ObjectFlags) (types.Value, bool) {
	hasFlag, errCode := store.HasObjectFlag(objID, flag)
	if errCode != types.E_NONE {
		return nil, false
	}
	if hasFlag {
		return types.NewInt(1), true
	}
	return types.NewInt(0), true
}

func objIDsToValues(ids []types.ObjID) []types.Value {
	values := make([]types.Value, len(ids))
	for i, id := range ids {
		values[i] = types.NewObj(id)
	}
	return values
}

// setBuiltinProperty sets a built-in object property.
func setBuiltinProperty(store *db.Store, objID types.ObjID, name string, value types.Value, ctx *types.TaskContext) (bool, types.ErrorCode) {
	switch name {
	case "name":
		if str, ok := value.(types.StrValue); ok {
			return true, store.SetObjectName(objID, str.Value())
		}
		return false, types.E_NONE
	case "owner":
		if objVal, ok := value.(types.ObjValue); ok {
			isAnonymous, errCode := store.ObjectIsAnonymous(objID)
			if errCode != types.E_NONE {
				return true, errCode
			}
			if isAnonymous && ctx != nil && !ctx.IsWizard {
				return true, types.E_PERM
			}
			return true, store.SetObjectOwner(objID, objVal.ID())
		}
		return false, types.E_NONE
	case "location":
		if objVal, ok := value.(types.ObjValue); ok {
			return true, store.SetObjectLocationRaw(objID, objVal.ID())
		}
		return false, types.E_NONE
	case "programmer":
		if intVal, ok := value.(types.IntValue); ok {
			isAnonymous, errCode := store.ObjectIsAnonymous(objID)
			if errCode != types.E_NONE {
				return true, errCode
			}
			if isAnonymous {
				if ctx != nil && ctx.IsWizard {
					return true, types.E_INVARG
				}
				return true, types.E_PERM
			}
			return true, store.SetObjectFlag(objID, db.FlagProgrammer, intVal.Val != 0)
		}
		return false, types.E_NONE
	case "wizard":
		if intVal, ok := value.(types.IntValue); ok {
			isAnonymous, errCode := store.ObjectIsAnonymous(objID)
			if errCode != types.E_NONE {
				return true, errCode
			}
			if isAnonymous {
				if ctx != nil && ctx.IsWizard {
					return true, types.E_INVARG
				}
				return true, types.E_PERM
			}
			return true, store.SetObjectFlag(objID, db.FlagWizard, intVal.Val != 0)
		}
		return false, types.E_NONE
	case "player":
		if intVal, ok := value.(types.IntValue); ok {
			return true, store.SetObjectFlag(objID, db.FlagUser, intVal.Val != 0)
		}
		return false, types.E_NONE
	case "r":
		if intVal, ok := value.(types.IntValue); ok {
			return true, store.SetObjectFlag(objID, db.FlagRead, intVal.Val != 0)
		}
		return false, types.E_NONE
	case "w":
		if intVal, ok := value.(types.IntValue); ok {
			return true, store.SetObjectFlag(objID, db.FlagWrite, intVal.Val != 0)
		}
		return false, types.E_NONE
	case "f":
		if intVal, ok := value.(types.IntValue); ok {
			return true, store.SetObjectFlag(objID, db.FlagFertile, intVal.Val != 0)
		}
		return false, types.E_NONE
	case "a":
		if intVal, ok := value.(types.IntValue); ok {
			return true, store.SetObjectFlag(objID, db.FlagAnonymous, intVal.Val != 0)
		}
		return false, types.E_NONE
	default:
		return false, types.E_NONE
	}
}
