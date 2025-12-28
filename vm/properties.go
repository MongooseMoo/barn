package vm

import (
	"barn/db"
	"barn/parser"
	"barn/types"
	"fmt"
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
		propName := node.Property
		if propName == "" {
			propName = "(dynamic)"
		}
		fmt.Printf("[PROPERTY GET] E_INVIND: verb=%s trying to get .%s on invalid object #%d\n", ctx.Verb, propName, objID)
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

	// Check for built-in properties first
	if val, ok := e.getBuiltinProperty(obj, propName); ok {
		return types.Ok(val)
	}

	// Look up property (will handle inheritance in Layer 8.3)
	prop, errCode := e.findProperty(obj, propName, ctx)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// Check read permission (Layer 8.5 will add full permission checks)
	// For now, allow all reads
	_ = ctx // Will use for permission checks later

	return types.Ok(prop.Value)
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
