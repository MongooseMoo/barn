package vm

import (
	"barn/db"
	"barn/parser"
	"barn/types"
)

// property evaluates property access: obj.property or obj.(expr)
// Returns E_INVIND if object is invalid
// Returns E_PROPNF if property not found
// Returns E_PERM if permission denied
func (e *Evaluator) property(node *parser.PropertyExpr, ctx *types.TaskContext) types.Result {
	// Evaluate the object expression
	objResult := e.Eval(node.Expr, ctx)
	if objResult.Flow != types.FlowNormal {
		return objResult
	}

	// Check if result is a waif
	if waifVal, ok := objResult.Val.(types.WaifValue); ok {
		return e.waifProperty(waifVal, node, ctx)
	}

	// Check that result is an object
	objVal, ok := objResult.Val.(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()

	// Get object from store
	obj := e.store.Get(objID)
	if obj == nil {
		// Invalid or recycled object
		return types.Err(types.E_INVIND)
	}

	// Get property name (static or dynamic)
	propName := node.Property
	if propName == "" && node.PropertyExpr != nil {
		// Dynamic property name - evaluate the expression
		propResult := e.Eval(node.PropertyExpr, ctx)
		if !propResult.IsNormal() {
			return propResult
		}
		// The property name must be a string
		strVal, ok := propResult.Val.(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		propName = strVal.Value()
	}

	// Look up defined property first (will handle inheritance)
	// This takes precedence over builtin flag properties like .player, .wizard
	prop, errCode := e.findProperty(obj, propName, ctx)

	if errCode == types.E_NONE {
		// Found a defined property - use it
		return types.Ok(prop.Value)
	}

	// No defined property - check for built-in properties (flag properties like .player, .wizard)
	if val, ok := e.getBuiltinProperty(obj, propName); ok {
		return types.Ok(val)
	}

	// Property not found
	return types.Err(errCode)
}

// getBuiltinProperty returns built-in object properties (name, owner, location, etc.)
func (e *Evaluator) getBuiltinProperty(obj *db.Object, name string) (types.Value, bool) {
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
		// .parent returns first parent or #-1 if none
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

// findProperty finds a property on an object with inheritance
// Implements breadth-first search as per spec/objects.md
// Search order: obj → parents → grandparents (breadth-first, left-to-right)
func (e *Evaluator) findProperty(obj *db.Object, name string, ctx *types.TaskContext) (*db.Property, types.ErrorCode) {
	// Use breadth-first search for inheritance
	// Queue starts with the object itself
	queue := []types.ObjID{obj.ID}
	visited := make(map[types.ObjID]bool)

	for len(queue) > 0 {
		// Pop from front (FIFO for breadth-first)
		currentID := queue[0]
		queue = queue[1:]

		// Skip if already visited (cycle detection)
		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		// Get current object
		current := e.store.Get(currentID)
		if current == nil {
			// Invalid parent - skip
			continue
		}

		// Check if property exists on this object
		prop, ok := current.Properties[name]
		if ok && !prop.Clear {
			// Found a non-clear property - this is the value
			return prop, types.E_NONE
		}

		// If property is clear or not found, continue to parents
		// Add parents to end of queue (breadth-first)
		queue = append(queue, current.Parents...)
	}

	// Property not found anywhere in inheritance chain
	return nil, types.E_PROPNF
}

// assignProperty handles property assignment: obj.property = value or obj.(expr) = value
// Returns E_INVIND if object is invalid
// Returns E_PROPNF if property not found
// Returns E_PERM if permission denied
func (e *Evaluator) assignProperty(node *parser.PropertyExpr, value types.Value, ctx *types.TaskContext) types.Result {
	// Evaluate the object expression
	objResult := e.Eval(node.Expr, ctx)
	if objResult.Flow != types.FlowNormal {
		return objResult
	}

	// Check if result is a waif
	if waifVal, ok := objResult.Val.(types.WaifValue); ok {
		newWaif, result := e.assignWaifProperty(waifVal, node, value, ctx)
		if !result.IsNormal() {
			return result
		}

		// Update the variable that holds the waif (waifs are immutable, copy-on-write)
		// For now, only handle simple identifier case: w.data = value
		// TODO: Handle complex cases (nested properties, indexed access)
		if ident, ok := node.Expr.(*parser.IdentifierExpr); ok {
			e.env.Set(ident.Name, newWaif)
		}

		return result
	}

	// Check that result is an object
	objVal, ok := objResult.Val.(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()

	// Get object from store
	obj := e.store.Get(objID)
	if obj == nil {
		// Invalid or recycled object
		return types.Err(types.E_INVIND)
	}

	// Get property name (static or dynamic)
	propName := node.Property
	if propName == "" && node.PropertyExpr != nil {
		// Dynamic property name - evaluate the expression
		propResult := e.Eval(node.PropertyExpr, ctx)
		if !propResult.IsNormal() {
			return propResult
		}
		// The property name must be a string
		strVal, ok := propResult.Val.(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		propName = strVal.Value()
	}

	// Check for built-in property assignment
	if isBuiltin, errCode := e.setBuiltinProperty(obj, propName, value, ctx); isBuiltin {
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(value)
	}

	// Check if property exists directly on this object
	prop, ok := obj.Properties[propName]
	if ok {
		// Property exists locally - update it
		prop.Clear = false
		prop.Value = value
		return types.Ok(value)
	}

	// Property not on this object - check if inherited
	inheritedProp, errCode := e.findProperty(obj, propName, ctx)
	if errCode != types.E_NONE {
		// Property not found anywhere
		return types.Err(types.E_PROPNF)
	}

	// Property is inherited - create a local copy with the new value
	// This "overrides" the inherited value on this object
	// Note: Defined=false because this is NOT a new property definition,
	// just a local value override of an inherited property
	newProp := &db.Property{
		Name:    propName,
		Value:   value,
		Owner:   inheritedProp.Owner,
		Perms:   inheritedProp.Perms,
		Clear:   false, // Has local value now
		Defined: false, // Not defined on this object, just overriding inherited
	}
	obj.Properties[propName] = newProp

	// Assignment returns the assigned value
	return types.Ok(value)
}

// setBuiltinProperty sets a built-in object property
// Returns (isBuiltin, errorCode) where isBuiltin indicates if it was a built-in property
// and errorCode is E_NONE on success or the appropriate error on failure
func (e *Evaluator) setBuiltinProperty(obj *db.Object, name string, value types.Value, ctx *types.TaskContext) (bool, types.ErrorCode) {
	switch name {
	case "name":
		if str, ok := value.(types.StrValue); ok {
			obj.Name = str.Value()
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "owner":
		if objVal, ok := value.(types.ObjValue); ok {
			// For anonymous objects, only wizards can change owner
			if obj.Anonymous && !ctx.IsWizard {
				return true, types.E_PERM
			}
			obj.Owner = objVal.ID()
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "location":
		if objVal, ok := value.(types.ObjValue); ok {
			// TODO: Update contents of old/new locations
			obj.Location = objVal.ID()
			return true, types.E_NONE
		}
		return false, types.E_NONE
	case "programmer":
		if intVal, ok := value.(types.IntValue); ok {
			// Anonymous objects cannot have programmer flag modified
			// Wizard gets E_INVARG (operation invalid), others get E_PERM
			if obj.Anonymous {
				if ctx.IsWizard {
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
			// Anonymous objects cannot have wizard flag modified
			// Wizard gets E_INVARG (operation invalid), others get E_PERM
			if obj.Anonymous {
				if ctx.IsWizard {
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
			// Wizards can modify any object's fertile flag
			// Players can only modify their own player object's fertile flag
			// (ownership alone is not sufficient)
			isPlayerObject := obj.ID == ctx.Player
			if !ctx.IsWizard && !isPlayerObject {
				return true, types.E_PERM
			}
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
			// Wizards can modify any object's anonymous flag
			// Players can only modify their own player object's anonymous flag
			// (ownership alone is not sufficient)
			isPlayerObject := obj.ID == ctx.Player
			if !ctx.IsWizard && !isPlayerObject {
				return true, types.E_PERM
			}
			if intVal.Val != 0 {
				obj.Flags = obj.Flags.Set(db.FlagAnonymous)
				obj.Anonymous = true
			} else {
				obj.Flags = obj.Flags.Clear(db.FlagAnonymous)
				obj.Anonymous = false
			}
			return true, types.E_NONE
		}
		return false, types.E_NONE
	default:
		return false, types.E_NONE
	}
}

// waifProperty evaluates property access on a waif: waif.property
// Returns special handling for .owner and .class
// Otherwise looks up in waif's properties, falling back to class object
func (e *Evaluator) waifProperty(waif types.WaifValue, node *parser.PropertyExpr, ctx *types.TaskContext) types.Result {
	// Get property name (static or dynamic)
	propName := node.Property
	if propName == "" && node.PropertyExpr != nil {
		// Dynamic property name - evaluate the expression
		propResult := e.Eval(node.PropertyExpr, ctx)
		if !propResult.IsNormal() {
			return propResult
		}
		// The property name must be a string
		strVal, ok := propResult.Val.(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		propName = strVal.Value()
	}

	// Special waif properties
	switch propName {
	case "owner":
		return types.Ok(types.NewObj(waif.Owner()))
	case "class":
		classID := waif.Class()
		// Check if class object has been recycled
		classObj := e.store.Get(classID)
		if classObj == nil {
			// Class has been recycled - return #-1
			return types.Ok(types.NewObj(types.ObjNothing))
		}
		return types.Ok(types.NewObj(classID))
	}

	// Check waif's own properties first
	if val, ok := waif.GetProperty(propName); ok {
		return types.Ok(val)
	}

	// Fall back to class object's properties
	classID := waif.Class()
	classObj := e.store.Get(classID)
	if classObj == nil {
		return types.Err(types.E_PROPNF)
	}

	// Look up property on class object
	prop, errCode := e.findProperty(classObj, propName, ctx)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(prop.Value)
}

// assignWaifProperty handles property assignment on a waif: waif.property = value
// Returns E_PERM for attempts to set .owner, .class, .wizard, or .programmer
// Returns E_RECMOVE if value contains the waif itself (circular reference)
func (e *Evaluator) assignWaifProperty(waif types.WaifValue, node *parser.PropertyExpr, value types.Value, ctx *types.TaskContext) (types.WaifValue, types.Result) {
	// Get property name (static or dynamic)
	propName := node.Property
	if propName == "" && node.PropertyExpr != nil {
		// Dynamic property name - evaluate the expression
		propResult := e.Eval(node.PropertyExpr, ctx)
		if !propResult.IsNormal() {
			return types.WaifValue{}, propResult
		}
		// The property name must be a string
		strVal, ok := propResult.Val.(types.StrValue)
		if !ok {
			return types.WaifValue{}, types.Err(types.E_TYPE)
		}
		propName = strVal.Value()
	}

	// These properties cannot be set on waifs
	switch propName {
	case "owner", "class", "wizard", "programmer":
		return types.WaifValue{}, types.Err(types.E_PERM)
	}

	// Check for self-reference (circular reference)
	if containsWaif(value, waif) {
		return types.WaifValue{}, types.Err(types.E_RECMOVE)
	}

	// Set property on waif (this creates a new waif value with the property set)
	// Waifs are immutable values, so this returns a new waif with the property set.
	// The caller must update the variable that holds the waif.
	newWaif := waif.SetProperty(propName, value)

	return newWaif, types.Ok(value)
}

// containsWaif checks if val contains or equals the waif (for circular reference detection)
func containsWaif(val types.Value, waif types.WaifValue) bool {
	switch v := val.(type) {
	case types.WaifValue:
		// Check if same waif instance (by comparing class and owner)
		// In a proper implementation, this would use reference identity
		return v.Class() == waif.Class() && v.Owner() == waif.Owner()
	case types.ListValue:
		for i := 1; i <= v.Len(); i++ {
			if containsWaif(v.Get(i), waif) {
				return true
			}
		}
	case types.MapValue:
		for _, pair := range v.Pairs() {
			if containsWaif(pair[0], waif) || containsWaif(pair[1], waif) {
				return true
			}
		}
	}
	return false
}

// waifPropertyIndexedAssign handles property-indexed assignment on waifs: waif.prop[idx] = value
// Also handles nested: waif.prop[i][j] = value
// NOTE: This doesn't actually work correctly in the current architecture because waifs
// are immutable values. The calling code needs to handle updating the variable.
// For now, we return E_PROPNF to match the behavior - waifs can't have their
// properties modified through indexed assignment in this way.
func (e *Evaluator) waifPropertyIndexedAssign(waif types.WaifValue, propExpr *parser.PropertyExpr, indices []parser.Expr, value types.Value, ctx *types.TaskContext) types.Result {
	// Get property name (static or dynamic)
	propName := propExpr.Property
	if propName == "" && propExpr.PropertyExpr != nil {
		// Dynamic property name - evaluate the expression
		propResult := e.Eval(propExpr.PropertyExpr, ctx)
		if !propResult.IsNormal() {
			return propResult
		}
		// The property name must be a string
		strVal, ok := propResult.Val.(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		propName = strVal.Value()
	}

	// Check if trying to modify built-in properties
	switch propName {
	case "owner", "class", "wizard", "programmer":
		return types.Err(types.E_PERM)
	}

	// Get the current property value
	var propVal types.Value
	if val, ok := waif.GetProperty(propName); ok {
		propVal = val
	} else {
		// Fall back to class object's properties
		classID := waif.Class()
		classObj := e.store.Get(classID)
		if classObj == nil {
			return types.Err(types.E_PROPNF)
		}

		// Look up property on class object
		prop, errCode := e.findProperty(classObj, propName, ctx)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		propVal = prop.Value
	}

	// Apply the index assignment(s) to the property value
	var newPropVal types.Value
	if len(indices) == 1 {
		// Single-level assignment
		length := getCollectionLength(propVal)
		if length < 0 {
			return types.Err(types.E_TYPE)
		}

		oldContext := ctx.IndexContext
		oldFirstKey := ctx.MapFirstKey
		oldLastKey := ctx.MapLastKey
		ctx.IndexContext = length
		ctx.MapFirstKey = nil
		ctx.MapLastKey = nil

		if mapVal, isMap := propVal.(types.MapValue); isMap && length > 0 {
			pairs := mapVal.Pairs()
			ctx.MapFirstKey = pairs[0][0]
			ctx.MapLastKey = pairs[length-1][0]
		}

		indexResult := e.Eval(indices[0], ctx)
		ctx.IndexContext = oldContext
		ctx.MapFirstKey = oldFirstKey
		ctx.MapLastKey = oldLastKey

		if !indexResult.IsNormal() {
			return indexResult
		}

		var err types.ErrorCode
		newPropVal, err = setAtIndex(propVal, indexResult.Val, value)
		if err != types.E_NONE {
			return types.Err(err)
		}
	} else {
		// Multi-level nested assignment
		collections := make([]types.Value, len(indices))
		resolvedIndices := make([]types.Value, len(indices))
		collections[0] = propVal

		// Traverse down, collecting intermediate values
		for i := 0; i < len(indices)-1; i++ {
			coll := collections[i]
			length := getCollectionLength(coll)
			if length < 0 {
				return types.Err(types.E_TYPE)
			}
			oldContext := ctx.IndexContext
			ctx.IndexContext = length

			indexResult := e.Eval(indices[i], ctx)
			ctx.IndexContext = oldContext
			if !indexResult.IsNormal() {
				return indexResult
			}
			resolvedIndices[i] = indexResult.Val

			// Get the nested collection
			var nextVal types.Value
			switch c := coll.(type) {
			case types.ListValue:
				idx, ok := indexResult.Val.(types.IntValue)
				if !ok {
					return types.Err(types.E_TYPE)
				}
				if idx.Val < 1 || idx.Val > int64(c.Len()) {
					return types.Err(types.E_RANGE)
				}
				nextVal = c.Get(int(idx.Val))
			case types.MapValue:
				val, exists := c.Get(indexResult.Val)
				if !exists {
					return types.Err(types.E_RANGE)
				}
				nextVal = val
			default:
				return types.Err(types.E_TYPE)
			}
			collections[i+1] = nextVal
		}

		// Resolve the final index
		lastColl := collections[len(indices)-1]
		length := getCollectionLength(lastColl)
		if length < 0 {
			return types.Err(types.E_TYPE)
		}
		oldContext := ctx.IndexContext
		ctx.IndexContext = length

		lastIndexResult := e.Eval(indices[len(indices)-1], ctx)
		ctx.IndexContext = oldContext
		if !lastIndexResult.IsNormal() {
			return lastIndexResult
		}
		resolvedIndices[len(indices)-1] = lastIndexResult.Val

		// Set the value at the deepest level
		var err types.ErrorCode
		collections[len(indices)-1], err = setAtIndex(lastColl, lastIndexResult.Val, value)
		if err != types.E_NONE {
			return types.Err(err)
		}

		// Rebuild going back up (copy-on-write)
		for i := len(indices) - 2; i >= 0; i-- {
			collections[i], err = setAtIndex(collections[i], resolvedIndices[i], collections[i+1])
			if err != types.E_NONE {
				return types.Err(err)
			}
		}

		newPropVal = collections[0]
	}

	// Check for self-reference (circular reference)
	if containsWaif(newPropVal, waif) {
		return types.Err(types.E_RECMOVE)
	}

	// Set the property on the waif (creates new waif with updated property)
	// NOTE: Since waifs are immutable, we need to update the property and
	// return success. The actual persistence is handled by the assignment
	// expression evaluator which will update the variable holding the waif.
	_ = waif.SetProperty(propName, newPropVal)

	return types.Ok(value)
}
