package vm

import (
	"fmt"
	"strings"

	"barn/builtins"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

// Property operations

func (vm *VM) executeGetProp() error {
	propNameIdx := vm.ReadByte()

	// Determine property name: static (from constant pool) or dynamic (from stack)
	var propName string
	if propNameIdx == 0xFF {
		// Dynamic property: name is on top of stack
		nameVal := vm.Pop()
		if nameVal.Type() != types.TYPE_STR {
			return fmt.Errorf("E_TYPE: dynamic property name must be a string")
		}
		propName = nameVal.Str()
	} else {
		// Static property: name from constant pool
		nameVal := vm.CurrentFrame().Program.Constants[propNameIdx]
		if nameVal.Type() != types.TYPE_STR {
			return fmt.Errorf("internal error: property name constant is not a string")
		}
		propName = nameVal.Str()
	}

	// Pop the object
	objVal := vm.Pop()

	// Check if it's a waif (must check before ObjValue since waifs are a different type)
	if objVal.Type() == types.TYPE_WAIF {
		return vm.getWaifProp(objVal, propName)
	}

	// Check if it's an object reference
	if !isObjLike(objVal) {
		return fmt.Errorf("E_TYPE: property access requires an object")
	}

	objID := objVal.ID()

	// Need a store to look up properties
	if vm.Store == nil {
		return fmt.Errorf("E_INVIND: no object store available")
	}

	if errCode := vm.Store.ObjectExists(objID); errCode != types.E_NONE {
		return fmt.Errorf("E_INVIND: invalid object #%d", objID)
	}

	// Built-in property names (.name/.owner/.location/...) can never be defined
	// properties — add_property rejects them with E_INVARG — so serve them from
	// the built-in path directly, skipping the (always-failing) inheritance walk.
	if builtins.IsBuiltinProperty(propName) {
		if val, ok := getBuiltinProperty(vm.Store, objID, propName); ok {
			vm.Push(val)
			return nil
		}
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	// Look up defined property (with inheritance via breadth-first search).
	prop, errCode := vm.Store.FindProperty(objID, propName)
	if errCode == types.E_NONE {
		// Check read permission
		if err := vm.checkPropertyReadPerm(prop); err != nil {
			return err
		}
		vm.Push(prop.Value)
		return nil
	}

	// Property not found
	return fmt.Errorf("E_PROPNF: property not found: %s", propName)
}

// getWaifProp handles property read on a waif value.
// Reads a waif property, falling back to the waif's class object.
func (vm *VM) getWaifProp(waif types.Value, propName string) error {
	// Special waif properties
	switch propName {
	case "owner":
		vm.Push(types.NewObj(waif.Owner()))
		return nil
	case "class":
		classID := waif.Class()
		// Check if class object has been recycled
		if vm.Store != nil {
			if errCode := vm.Store.ObjectExists(classID); errCode != types.E_NONE {
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
	if errCode := vm.Store.ObjectExists(classID); errCode != types.E_NONE {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	classPropName := propName
	if !strings.HasPrefix(classPropName, ":") {
		classPropName = ":" + classPropName
	}
	prop, errCode := vm.Store.FindProperty(classID, classPropName)
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
		if nameVal.Type() != types.TYPE_STR {
			return fmt.Errorf("E_TYPE: dynamic property name must be a string")
		}
		propName = nameVal.Str()
	} else {
		// Static property: name from constant pool
		nameVal := vm.CurrentFrame().Program.Constants[propNameIdx]
		if nameVal.Type() != types.TYPE_STR {
			return fmt.Errorf("internal error: property name constant is not a string")
		}
		propName = nameVal.Str()
	}

	// Pop the object
	objVal := vm.Pop()

	// Pop the value to assign
	value := vm.Pop()

	// Check if it's a waif (must check before ObjValue since waifs are a different type)
	if objVal.Type() == types.TYPE_WAIF {
		return vm.setWaifProp(objVal, propName, value)
	}

	// Check if it's an object reference
	if !isObjLike(objVal) {
		return fmt.Errorf("E_TYPE: property assignment requires an object")
	}

	objID := objVal.ID()

	// Need a store to set properties
	if vm.Store == nil {
		return fmt.Errorf("E_INVIND: no object store available")
	}

	if errCode := vm.Store.ObjectExists(objID); errCode != types.E_NONE {
		return fmt.Errorf("E_INVIND: invalid object #%d", objID)
	}

	// Check for built-in property assignment first
	if isBuiltin, errCode := setBuiltinProperty(vm.Store, objID, propName, value, vm.Context); isBuiltin {
		if errCode != types.E_NONE {
			return fmt.Errorf("%s: cannot set built-in property %s", errCode, propName)
		}
		return nil
	}

	prop, ok, errCode := vm.Store.LocalProperty(objID, propName)
	if errCode != types.E_NONE {
		return fmt.Errorf("%s: invalid object #%d", errCode, objID)
	}
	if ok {
		// Check write permission
		if err := vm.checkPropertyWritePerm(prop); err != nil {
			return err
		}
		if errCode := vm.Store.SetPropertyValue(objID, propName, value); errCode != types.E_NONE {
			return fmt.Errorf("%s: property not set: %s", errCode, propName)
		}
		return nil
	}

	// Property not on this object - check if inherited
	inheritedProp, errCode := vm.Store.FindProperty(objID, propName)
	if errCode != types.E_NONE {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	// Check write permission on the inherited property
	if err := vm.checkPropertyWritePerm(inheritedProp); err != nil {
		return err
	}

	if errCode := vm.Store.SetPropertyValue(objID, propName, value); errCode != types.E_NONE {
		return fmt.Errorf("%s: property not set: %s", errCode, propName)
	}
	return nil
}

// setWaifProp handles property assignment on a waif value.
// Assigns a waif property and returns the copied waif value.
func (vm *VM) setWaifProp(waif types.Value, propName string, value types.Value) error {
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
func (vm *VM) checkPropertyReadPerm(prop dbstore.PropertyView) error {
	if vm.Context == nil {
		return nil // No context = no permission check
	}
	if vm.Context.IsWizard {
		return nil
	}
	if vm.Context.Programmer == prop.Owner {
		return nil
	}
	if !prop.Perms.Has(dbstore.PropRead) {
		return fmt.Errorf("E_PERM: property not readable")
	}
	return nil
}

// checkPropertyWritePerm checks if the current programmer has write permission on a property.
// Wizards and property owners always have access.
func (vm *VM) checkPropertyWritePerm(prop dbstore.PropertyView) error {
	if vm.Context == nil {
		return nil // No context = no permission check
	}
	if vm.Context.IsWizard {
		return nil
	}
	if vm.Context.Programmer == prop.Owner {
		return nil
	}
	if !prop.Perms.Has(dbstore.PropWrite) {
		return fmt.Errorf("E_PERM: property not writable")
	}
	return nil
}

// getBuiltinProperty returns built-in object properties (name, owner, location, etc.).
func getBuiltinProperty(store *dbstore.Store, objID types.ObjID, name string) (types.Value, bool) {
	switch strings.ToLower(name) {
	case "name":
		name, errCode := store.ObjectName(objID)
		if errCode != types.E_NONE {
			return types.None, false
		}
		return types.NewStr(name), true
	case "owner":
		ownerID, errCode := store.ObjectOwner(objID)
		if errCode != types.E_NONE {
			return types.None, false
		}
		return types.NewObj(ownerID), true
	case "location":
		locationID, errCode := store.Location(objID)
		if errCode != types.E_NONE {
			return types.None, false
		}
		return types.NewObj(locationID), true
	case "contents":
		contentsIDs, errCode := store.Contents(objID)
		if errCode != types.E_NONE {
			return types.None, false
		}
		return types.NewList(objIDsToValues(contentsIDs)), true
	case "last_move":
		// Barn does not yet track per-object move history; last_move is reported
		// as an empty map (ToastStunt seeds a fresh object's last_move to []).
		return types.NewMap(nil), true
	case "programmer":
		return boolPropertyValue(store, objID, dbstore.FlagProgrammer)
	case "wizard":
		return boolPropertyValue(store, objID, dbstore.FlagWizard)
	case "r":
		return boolPropertyValue(store, objID, dbstore.FlagRead)
	case "w":
		return boolPropertyValue(store, objID, dbstore.FlagWrite)
	case "f":
		return boolPropertyValue(store, objID, dbstore.FlagFertile)
	case "a":
		hasFlag, flagErr := store.HasObjectFlag(objID, dbstore.FlagAnonymous)
		isAnonymous, anonErr := store.ObjectIsAnonymous(objID)
		if flagErr != types.E_NONE || anonErr != types.E_NONE {
			return types.None, false
		}
		if hasFlag || isAnonymous {
			return types.NewInt(1), true
		}
		return types.NewInt(0), true
	default:
		return types.None, false
	}
}

func boolPropertyValue(store *dbstore.Store, objID types.ObjID, flag dbstore.ObjectFlags) (types.Value, bool) {
	hasFlag, errCode := store.HasObjectFlag(objID, flag)
	if errCode != types.E_NONE {
		return types.None, false
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
func setBuiltinProperty(store *dbstore.Store, objID types.ObjID, name string, value types.Value, ctx *kernel.TaskContext) (bool, types.ErrorCode) {
	switch strings.ToLower(name) {
	case "name":
		if value.Type() == types.TYPE_STR {
			return true, store.SetObjectName(objID, value.Str())
		}
		return false, types.E_NONE
	case "owner":
		if isObjLike(value) {
			isAnonymous, errCode := store.ObjectIsAnonymous(objID)
			if errCode != types.E_NONE {
				return true, errCode
			}
			if isAnonymous && ctx != nil && !ctx.IsWizard {
				return true, types.E_PERM
			}
			return true, store.SetObjectOwner(objID, value.ID())
		}
		return false, types.E_NONE
	case "location":
		if isObjLike(value) {
			return true, store.SetObjectLocationRaw(objID, value.ID())
		}
		return false, types.E_NONE
	case "programmer":
		if value.Type() == types.TYPE_INT {
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
			return true, store.SetObjectFlag(objID, dbstore.FlagProgrammer, value.Int() != 0)
		}
		return false, types.E_NONE
	case "wizard":
		if value.Type() == types.TYPE_INT {
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
			return true, store.SetObjectFlag(objID, dbstore.FlagWizard, value.Int() != 0)
		}
		return false, types.E_NONE
	case "last_move":
		// last_move is server-maintained; it exists but is read-only -> E_PERM.
		return true, types.E_PERM
	case "r":
		if value.Type() == types.TYPE_INT {
			return true, store.SetObjectFlag(objID, dbstore.FlagRead, value.Int() != 0)
		}
		return false, types.E_NONE
	case "w":
		if value.Type() == types.TYPE_INT {
			return true, store.SetObjectFlag(objID, dbstore.FlagWrite, value.Int() != 0)
		}
		return false, types.E_NONE
	case "f":
		if value.Type() == types.TYPE_INT {
			return true, store.SetObjectFlag(objID, dbstore.FlagFertile, value.Int() != 0)
		}
		return false, types.E_NONE
	case "a":
		if value.Type() == types.TYPE_INT {
			return true, store.SetObjectFlag(objID, dbstore.FlagAnonymous, value.Int() != 0)
		}
		return false, types.E_NONE
	default:
		return false, types.E_NONE
	}
}
