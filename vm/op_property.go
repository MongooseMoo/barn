package vm

import (
	"fmt"
	"strings"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

// Property operations

func (vm *VM) executeGetProp() error {
	propNameIdx := vm.FetchByte()
	if propNameIdx == 0xFF {
		return vm.executeGetPropDynamic()
	}
	return vm.executeGetPropStatic(int(propNameIdx))
}

func (vm *VM) executeGetPropWide() error {
	return vm.executeGetPropStatic(int(vm.ReadShort()))
}

func (vm *VM) executeGetPropStatic(index int) error {
	propName, err := vm.staticNameFromConstant(index, "property")
	if err != nil {
		return err
	}
	return vm.executeGetPropNamed(propName)
}

func (vm *VM) executeGetPropDynamic() error {
	propName, err := vm.popDynamicName("property")
	if err != nil {
		return err
	}
	return vm.executeGetPropNamed(propName)
}

func (vm *VM) executeGetPropNamed(propName string) error {
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

	txn := vm.Context.StoreTxn
	if errCode := objectExistsForRead(txn, objID); errCode != types.E_NONE {
		return fmt.Errorf("E_INVIND: invalid object #%d", objID)
	}

	// Built-in property names (.name/.owner/.location/...) can never be defined
	// properties — add_property rejects them with E_INVARG — so serve them from
	// the built-in path directly, skipping the (always-failing) inheritance walk.
	if builtins.IsBuiltinProperty(propName) {
		if val, ok := getBuiltinProperty(txn, objID, propName); ok {
			vm.Push(val)
			return nil
		}
		return fmt.Errorf("E_PROPNF: Property not found: #%d.%s", objID, propName)
	}

	// Look up defined property (with inheritance via breadth-first search).
	prop, errCode := findPropertyForRead(txn, objID, propName)
	if errCode == types.E_NONE {
		// Check read permission
		if err := vm.checkPropertyReadPerm(prop); err != nil {
			return err
		}
		vm.Push(prop.Value)
		return nil
	}

	// Property not found
	return fmt.Errorf("E_PROPNF: Property not found: #%d.%s", objID, propName)
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
		// Check if class object still exists — through the txn (read-your-writes) so a
		// class this task created decentrally (still staged, not yet in live) is seen. A
		// direct store lookup would miss it and wrongly report the class recycled (#-1).
		if vm.Store != nil {
			if errCode := objectExistsForRead(vm.Context.StoreTxn, classID); errCode != types.E_NONE {
				// Class has been recycled - return #-1
				vm.Push(types.NewObj(types.ObjNothing))
				return nil
			}
		}
		vm.Push(types.NewObj(classID))
		return nil
	case "wizard", "programmer":
		vm.Push(types.NewInt(0))
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
	txn := vm.Context.StoreTxn
	if errCode := objectExistsForRead(txn, classID); errCode != types.E_NONE {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	classPropName := propName
	if !strings.HasPrefix(classPropName, ":") {
		classPropName = ":" + classPropName
	}
	prop, errCode := findPropertyForRead(txn, classID, classPropName)
	if errCode != types.E_NONE {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	vm.Push(prop.Value)
	return nil
}

func (vm *VM) executeSetProp() error {
	propNameIdx := vm.FetchByte()
	if propNameIdx == 0xFF {
		return vm.executeSetPropDynamic()
	}
	return vm.executeSetPropStatic(int(propNameIdx))
}

func (vm *VM) executeSetPropWide() error {
	return vm.executeSetPropStatic(int(vm.ReadShort()))
}

func (vm *VM) executeSetPropStatic(index int) error {
	propName, err := vm.staticNameFromConstant(index, "property")
	if err != nil {
		return err
	}
	return vm.executeSetPropNamed(propName)
}

func (vm *VM) executeSetPropDynamic() error {
	propName, err := vm.popDynamicName("property")
	if err != nil {
		return err
	}
	return vm.executeSetPropNamed(propName)
}

func (vm *VM) executeSetPropNamed(propName string) error {
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

	txn := vm.Context.StoreTxn
	if errCode := objectExistsForRead(txn, objID); errCode != types.E_NONE {
		return fmt.Errorf("E_INVIND: invalid object #%d", objID)
	}

	// Check for built-in property assignment first
	if isBuiltin, errCode := setBuiltinProperty(vm.Builtins, txn, objID, propName, value, vm.Context); isBuiltin {
		if errCode != types.E_NONE {
			return fmt.Errorf("%s: cannot set built-in property %s", errCode, propName)
		}
		return nil
	}

	prop, ok, errCode := localPropertyForRead(txn, objID, propName)
	if errCode != types.E_NONE {
		return fmt.Errorf("%s: invalid object #%d", errCode, objID)
	}
	if ok {
		// Check write permission
		if err := vm.checkPropertyWritePerm(prop); err != nil {
			return err
		}
		errCode := types.E_NONE
		errCode = txn.SetPropertyValue(objID, propName, value)
		if errCode != types.E_NONE {
			return fmt.Errorf("%s: property not set: %s", errCode, propName)
		}
		return nil
	}

	// Property not on this object - check if inherited
	inheritedProp, errCode := findPropertyForRead(txn, objID, propName)
	if errCode != types.E_NONE {
		return fmt.Errorf("E_PROPNF: property not found: %s", propName)
	}

	// Check write permission on the inherited property
	if err := vm.checkPropertyWritePerm(inheritedProp); err != nil {
		return err
	}

	errCode = types.E_NONE
	errCode = txn.SetPropertyValue(objID, propName, value)
	if errCode != types.E_NONE {
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

func objectExistsForRead(txn *dbstore.StoreTxn, objID types.ObjID) types.ErrorCode {
	return txn.ObjectExists(objID)
}

func findPropertyForRead(txn *dbstore.StoreTxn, objID types.ObjID, name string) (dbstore.PropertyView, types.ErrorCode) {
	return txn.FindProperty(objID, name)
}

func localPropertyForRead(txn *dbstore.StoreTxn, objID types.ObjID, name string) (dbstore.PropertyView, bool, types.ErrorCode) {
	return txn.LocalProperty(objID, name)
}

// getBuiltinProperty returns server-maintained object properties.
func getBuiltinProperty(txn *dbstore.StoreTxn, objID types.ObjID, name string) (types.Value, bool) {
	switch strings.ToLower(name) {
	case "name":
		// Shared box built when the name was written — no strRep per read.
		nameVal, errCode := txn.ObjectNameValue(objID)
		if errCode != types.E_NONE {
			return types.None, false
		}
		return nameVal, true
	case "owner":
		var (
			ownerID types.ObjID
			errCode types.ErrorCode
		)
		ownerID, errCode = txn.ObjectOwner(objID)
		if errCode != types.E_NONE {
			return types.None, false
		}
		return types.NewObj(ownerID), true
	case "location":
		var (
			locationID types.ObjID
			errCode    types.ErrorCode
		)
		locationID, errCode = txn.Location(objID)
		if errCode != types.E_NONE {
			return types.None, false
		}
		return types.NewObj(locationID), true
	case "contents":
		var (
			contentsIDs []types.ObjID
			errCode     types.ErrorCode
		)
		contentsIDs, errCode = txn.Contents(objID)
		if errCode != types.E_NONE {
			return types.None, false
		}
		return types.NewList(objIDsToValues(contentsIDs)), true
	case "last_move":
		var (
			lastMove types.Value
			errCode  types.ErrorCode
		)
		lastMove, errCode = txn.LastMove(objID)
		if errCode != types.E_NONE {
			return types.None, false
		}
		return lastMove, true
	case "programmer":
		return boolPropertyValue(txn, objID, dbstore.FlagProgrammer)
	case "wizard":
		return boolPropertyValue(txn, objID, dbstore.FlagWizard)
	case "r":
		return boolPropertyValue(txn, objID, dbstore.FlagRead)
	case "w":
		return boolPropertyValue(txn, objID, dbstore.FlagWrite)
	case "f":
		return boolPropertyValue(txn, objID, dbstore.FlagFertile)
	case "a":
		var (
			hasFlag     bool
			isAnonymous bool
			flagErr     types.ErrorCode
			anonErr     types.ErrorCode
		)
		hasFlag, flagErr = txn.HasObjectFlag(objID, dbstore.FlagAnonymous)
		isAnonymous, anonErr = txn.ObjectIsAnonymous(objID)
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

func boolPropertyValue(txn *dbstore.StoreTxn, objID types.ObjID, flag dbstore.ObjectFlags) (types.Value, bool) {
	var (
		hasFlag bool
		errCode types.ErrorCode
	)
	hasFlag, errCode = txn.HasObjectFlag(objID, flag)
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

// setBuiltinProperty sets mutable server-maintained object properties.
func setBuiltinProperty(session *builtins.Session, txn *dbstore.StoreTxn, objID types.ObjID, name string, value types.Value, ctx *kernel.TaskContext) (bool, types.ErrorCode) {
	switch strings.ToLower(name) {
	case "name":
		if value.Type() != types.TYPE_STR {
			return true, types.E_TYPE
		}
		if errCode := checkBuiltinPropertyOwner(session, txn, objID, "name", ctx, true); errCode != types.E_NONE {
			return true, errCode
		}
		return true, txn.SetObjectName(objID, value.Str())
	case "owner":
		if !isObjLike(value) {
			return true, types.E_TYPE
		}
		if ctx != nil && !ctx.IsWizard {
			return true, types.E_PERM
		}
		return true, txn.SetObjectOwner(objID, value.ID())
	case "location", "contents", "last_move":
		return true, types.E_PERM
	case "programmer", "wizard":
		if ctx != nil && !ctx.IsWizard {
			return true, types.E_PERM
		}
		isAnonymous, errCode := objectIsAnonymousForRead(txn, objID)
		if errCode != types.E_NONE {
			return true, errCode
		}
		if isAnonymous {
			return true, types.E_INVARG
		}
		flag := dbstore.FlagProgrammer
		if strings.EqualFold(name, "wizard") {
			flag = dbstore.FlagWizard
		}
		return true, txn.SetObjectFlag(objID, flag, value.Truthy())
	case "r", "w", "f", "a":
		if errCode := checkBuiltinPropertyOwner(session, txn, objID, strings.ToLower(name), ctx, false); errCode != types.E_NONE {
			return true, errCode
		}
		flags := map[string]dbstore.ObjectFlags{"r": dbstore.FlagRead, "w": dbstore.FlagWrite, "f": dbstore.FlagFertile, "a": dbstore.FlagAnonymous}
		flag := flags[strings.ToLower(name)]
		return true, txn.SetObjectFlag(objID, flag, value.Truthy())
	default:
		return false, types.E_NONE
	}
}

func checkBuiltinPropertyOwner(session *builtins.Session, txn *dbstore.StoreTxn, objID types.ObjID, name string, ctx *kernel.TaskContext, rejectUser bool) types.ErrorCode {
	if ctx == nil || ctx.IsWizard {
		return types.E_NONE
	}
	if session.IsProtectedBuiltin(name) {
		return types.E_PERM
	}
	owner, errCode := txn.ObjectOwner(objID)
	if errCode != types.E_NONE {
		return errCode
	}
	if owner != ctx.Programmer {
		return types.E_PERM
	}
	if rejectUser {
		isUser, errCode := txn.HasObjectFlag(objID, dbstore.FlagUser)
		if errCode != types.E_NONE {
			return errCode
		}
		if isUser {
			return types.E_PERM
		}
	}
	return types.E_NONE
}

func objectIsAnonymousForRead(txn *dbstore.StoreTxn, objID types.ObjID) (bool, types.ErrorCode) {
	return txn.ObjectIsAnonymous(objID)
}
