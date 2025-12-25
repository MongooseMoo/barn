package eval

import (
	"barn/db"
	"barn/parser"
	"barn/types"
)

// evalProperty evaluates property access: obj.property
// Returns E_INVIND if object is invalid
// Returns E_PROPNF if property not found
// Returns E_PERM if permission denied
func (e *Evaluator) evalProperty(node *parser.PropertyExpr, ctx *types.TaskContext) types.Result {
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

	// Look up property (will handle inheritance in Layer 8.3)
	prop, errCode := e.findProperty(obj, node.Property, ctx)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// Check read permission (Layer 8.5 will add full permission checks)
	// For now, allow all reads
	_ = ctx // Will use for permission checks later

	return types.Ok(prop.Value)
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

// evalAssignProperty handles property assignment: obj.property = value
// Returns E_INVIND if object is invalid
// Returns E_PROPNF if property not found
// Returns E_PERM if permission denied
func (e *Evaluator) evalAssignProperty(node *parser.PropertyExpr, value types.Value, ctx *types.TaskContext) types.Result {
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

	// Check if property exists
	prop, ok := obj.Properties[node.Property]
	if !ok {
		// Property not found (Layer 8.6 will add add_property)
		return types.Err(types.E_PROPNF)
	}

	// Check write permission (Layer 8.5 will add full permission checks)
	// For now, allow all writes
	_ = ctx // Will use for permission checks later

	// If property is clear, writing to it un-clears it (per spec)
	// This sets a local value instead of inheriting
	prop.Clear = false
	prop.Value = value

	// Assignment returns the assigned value
	return types.Ok(value)
}
