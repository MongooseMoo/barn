package vm

import (
	"barn/db"
	"barn/parser"
	"barn/types"
)

// index evaluates indexing: expr[index]
// Supports: lists, strings, and maps
func (e *Evaluator) index(node *parser.IndexExpr, ctx *types.TaskContext) types.Result {
	// Evaluate the expression being indexed
	exprResult := e.Eval(node.Expr, ctx)
	if !exprResult.IsNormal() {
		return exprResult
	}

	expr := exprResult.Val

	// Get collection length for $ and ^ resolution in sub-expressions
	length := getCollectionLength(expr)
	if length < 0 {
		return types.Err(types.E_TYPE) // Not a collection
	}

	// Set IndexContext so ^ and $ can be resolved in sub-expressions
	oldContext := ctx.IndexContext
	oldFirstKey := ctx.MapFirstKey
	oldLastKey := ctx.MapLastKey
	ctx.IndexContext = length
	ctx.MapFirstKey = nil
	ctx.MapLastKey = nil

	// For maps, also store first/last keys for ^ and $ resolution
	if mapVal, isMap := expr.(types.MapValue); isMap && length > 0 {
		pairs := mapVal.Pairs()
		ctx.MapFirstKey = pairs[0][0]
		ctx.MapLastKey = pairs[length-1][0]
	}

	defer func() {
		ctx.IndexContext = oldContext
		ctx.MapFirstKey = oldFirstKey
		ctx.MapLastKey = oldLastKey
	}()

	// Evaluate the index expression
	indexResult := e.Eval(node.Index, ctx)
	if !indexResult.IsNormal() {
		return indexResult
	}
	index := indexResult.Val

	// Dispatch based on collection type
	switch coll := expr.(type) {
	case types.ListValue:
		return listIndex(coll, index)
	case types.StrValue:
		return strIndex(coll, index)
	case types.MapValue:
		return mapIndex(coll, index)
	default:
		return types.Err(types.E_TYPE)
	}
}

// getCollectionLength returns the length of a collection, or -1 if not a collection
func getCollectionLength(val types.Value) int {
	switch coll := val.(type) {
	case types.ListValue:
		return coll.Len()
	case types.StrValue:
		return len(coll.Value())
	case types.MapValue:
		return coll.Len()
	default:
		return -1
	}
}

// rangeExpr evaluates range indexing: expr[start..end]
// Supports: lists and strings
func (e *Evaluator) rangeExpr(node *parser.RangeExpr, ctx *types.TaskContext) types.Result {
	// Evaluate the expression being indexed
	exprResult := e.Eval(node.Expr, ctx)
	if !exprResult.IsNormal() {
		return exprResult
	}

	expr := exprResult.Val

	// Get collection length for index marker resolution
	length := getCollectionLength(expr)
	if length < 0 {
		return types.Err(types.E_TYPE) // Not a collection
	}

	// Set IndexContext so ^ and $ can be resolved in sub-expressions
	oldContext := ctx.IndexContext
	ctx.IndexContext = length
	defer func() { ctx.IndexContext = oldContext }()

	// Evaluate start expression
	startResult := e.Eval(node.Start, ctx)
	if !startResult.IsNormal() {
		return startResult
	}
	startInt, ok := startResult.Val.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	startIdx := startInt.Val

	// Evaluate end expression
	endResult := e.Eval(node.End, ctx)
	if !endResult.IsNormal() {
		return endResult
	}
	endInt, ok := endResult.Val.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	endIdx := endInt.Val

	// Dispatch based on collection type
	switch coll := expr.(type) {
	case types.ListValue:
		return listRange(coll, startIdx, endIdx)
	case types.StrValue:
		return strRange(coll, startIdx, endIdx)
	case types.MapValue:
		return mapRange(coll, startIdx, endIdx)
	default:
		return types.Err(types.E_TYPE)
	}
}

// listIndex evaluates list indexing
func listIndex(list types.ListValue, index types.Value) types.Result {
	// Index must be an integer
	indexInt, ok := index.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Convert to 1-based index
	idx := indexInt.Val

	// Check bounds (1-based indexing)
	length := list.Len()
	if idx < 1 || idx > int64(length) {
		return types.Err(types.E_RANGE)
	}

	// Get element (list.Get expects 1-based index)
	val := list.Get(int(idx))
	return types.Ok(val)
}

// listRange evaluates list range indexing
func listRange(list types.ListValue, start, end int64) types.Result {
	length := int64(list.Len())

	// If start > end, return empty list (before bounds checking per MOO semantics)
	if start > end {
		return types.Ok(types.NewList([]types.Value{}))
	}

	// Check bounds
	if start < 1 || start > length {
		return types.Err(types.E_RANGE)
	}
	if end < 1 || end > length {
		return types.Err(types.E_RANGE)
	}

	// Special case: when start == end, return the single element (not a list)
	if start == end {
		return types.Ok(list.Get(int(start)))
	}

	// Extract slice (1-based to 0-based conversion)
	result := []types.Value{}
	for i := start; i <= end; i++ {
		val := list.Get(int(i))
		result = append(result, val)
	}

	return types.Ok(types.NewList(result))
}

// strIndex evaluates string indexing (returns single character)
func strIndex(str types.StrValue, index types.Value) types.Result {
	// Index must be an integer
	indexInt, ok := index.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Convert to 1-based index
	idx := indexInt.Val

	// Get underlying string
	s := str.Value()
	length := int64(len(s))

	// Check bounds (1-based indexing)
	if idx < 1 || idx > length {
		return types.Err(types.E_RANGE)
	}

	// Get character (0-based in Go)
	char := s[idx-1 : idx]
	return types.Ok(types.NewStr(char))
}

// strRange evaluates string range indexing (returns substring)
func strRange(str types.StrValue, start, end int64) types.Result {
	// Get underlying string
	s := str.Value()
	length := int64(len(s))

	// If start > end, return empty string (before bounds checking per MOO semantics)
	if start > end {
		return types.Ok(types.NewStr(""))
	}

	// Check bounds
	if start < 1 || start > length {
		return types.Err(types.E_RANGE)
	}
	if end < 1 || end > length {
		return types.Err(types.E_RANGE)
	}

	// Extract substring (1-based to 0-based conversion, Go slice is [start:end+1])
	substr := s[start-1 : end]
	return types.Ok(types.NewStr(substr))
}

// mapRange evaluates map range indexing (returns submap)
// Maps are indexed by position, not key, for range operations
func mapRange(m types.MapValue, start, end int64) types.Result {
	length := int64(m.Len())

	// If start > end, return empty map (before bounds checking per MOO semantics)
	if start > end {
		return types.Ok(types.NewEmptyMap())
	}

	// Check bounds
	if start < 1 || start > length {
		return types.Err(types.E_RANGE)
	}
	if end < 1 || end > length {
		return types.Err(types.E_RANGE)
	}

	// Extract pairs in range (1-based indexing)
	pairs := m.Pairs()
	result := make([][2]types.Value, 0, int(end-start+1))
	for i := start; i <= end; i++ {
		result = append(result, pairs[i-1])
	}

	return types.Ok(types.NewMap(result))
}

// mapIndex evaluates map indexing
func mapIndex(m types.MapValue, key types.Value) types.Result {
	// Map keys must be scalar types (not list or map)
	// But we need to check specifically for list and map types that could be used as keys
	switch key.(type) {
	case types.ListValue, types.MapValue:
		return types.Err(types.E_TYPE)
	}

	// Look up key in map
	val, ok := m.Get(key)
	if !ok {
		return types.Err(types.E_RANGE)
	}

	return types.Ok(val)
}

// assignIndex handles index assignment: coll[idx] = value
// Also handles nested assignment: coll[i][j][k] = value (copy-on-write)
// Also handles property-indexed assignment: obj.prop[idx] = value
func (e *Evaluator) assignIndex(target *parser.IndexExpr, value types.Value, ctx *types.TaskContext) types.Result {
	// Build path of indices from the target expression
	var path []parser.Expr // Index expressions, innermost first
	var current parser.Expr = target

	// Walk up the chain to find the base variable or property
	for {
		switch expr := current.(type) {
		case *parser.IndexExpr:
			path = append(path, expr.Index)
			current = expr.Expr
		case *parser.IdentifierExpr:
			// Found the base variable - reverse path (now outermost first)
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return e.nestedAssign(expr.Name, path, value, ctx)
		case *parser.PropertyExpr:
			// Property-indexed assignment: obj.prop[idx] = value
			// Reverse path for processing
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return e.propertyIndexedAssign(expr, path, value, ctx)
		default:
			return types.Err(types.E_TYPE) // Not assignable
		}
	}
}

// nestedAssign handles nested index assignment with copy-on-write semantics
func (e *Evaluator) nestedAssign(varName string, indices []parser.Expr, value types.Value, ctx *types.TaskContext) types.Result {
	// Get the root collection
	rootVal, exists := e.env.Get(varName)
	if !exists {
		return types.Err(types.E_VARNF)
	}

	// For single-level assignment, use simple path
	if len(indices) == 1 {
		return e.simpleIndexAssign(varName, rootVal, indices[0], value, ctx)
	}

	// For nested assignment, we need to:
	// 1. Traverse down to get all intermediate collections
	// 2. Modify the deepest level
	// 3. Rebuild going back up (copy-on-write)

	// Collect all intermediate values and their indices
	collections := make([]types.Value, len(indices))
	resolvedIndices := make([]types.Value, len(indices))
	collections[0] = rootVal

	// Traverse down, collecting intermediate values
	for i := 0; i < len(indices)-1; i++ {
		coll := collections[i]

		// Set IndexContext for index resolution
		length := getCollectionLength(coll)
		if length < 0 {
			return types.Err(types.E_TYPE)
		}
		oldContext := ctx.IndexContext
		ctx.IndexContext = length

		// Evaluate index
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
			val, ok := c.Get(indexResult.Val)
			if !ok {
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

	// Assign value at the deepest level
	newVal, err := setAtIndex(lastColl, resolvedIndices[len(indices)-1], value)
	if err != types.E_NONE {
		return types.Err(err)
	}

	// Rebuild going back up (copy-on-write)
	for i := len(indices) - 2; i >= 0; i-- {
		newVal, err = setAtIndex(collections[i], resolvedIndices[i], newVal)
		if err != types.E_NONE {
			return types.Err(err)
		}
	}

	// Store the new root collection
	e.env.Set(varName, newVal)
	return types.Ok(value)
}

// simpleIndexAssign handles single-level index assignment
func (e *Evaluator) simpleIndexAssign(varName string, collVal types.Value, indexExpr parser.Expr, value types.Value, ctx *types.TaskContext) types.Result {
	// Get collection length for ^ and $ resolution
	length := getCollectionLength(collVal)
	if length < 0 {
		return types.Err(types.E_TYPE)
	}

	// Set IndexContext for index resolution
	oldContext := ctx.IndexContext
	oldFirstKey := ctx.MapFirstKey
	oldLastKey := ctx.MapLastKey
	ctx.IndexContext = length
	ctx.MapFirstKey = nil
	ctx.MapLastKey = nil

	// For maps, also store first/last keys for ^ and $ resolution
	if mapVal, isMap := collVal.(types.MapValue); isMap && length > 0 {
		pairs := mapVal.Pairs()
		ctx.MapFirstKey = pairs[0][0]
		ctx.MapLastKey = pairs[length-1][0]
	}

	defer func() {
		ctx.IndexContext = oldContext
		ctx.MapFirstKey = oldFirstKey
		ctx.MapLastKey = oldLastKey
	}()

	// Evaluate the index expression (for maps, ^ and $ will resolve to actual keys)
	indexResult := e.Eval(indexExpr, ctx)
	if !indexResult.IsNormal() {
		return indexResult
	}

	// Perform the assignment
	newColl, err := setAtIndex(collVal, indexResult.Val, value)
	if err != types.E_NONE {
		return types.Err(err)
	}

	e.env.Set(varName, newColl)
	return types.Ok(value)
}

// setAtIndex sets a value at an index in a collection, returning new collection (copy-on-write)
func setAtIndex(coll types.Value, index types.Value, value types.Value) (types.Value, types.ErrorCode) {
	switch c := coll.(type) {
	case types.ListValue:
		idx, ok := index.(types.IntValue)
		if !ok {
			return nil, types.E_TYPE
		}
		i := int(idx.Val)
		if i < 1 || i > c.Len() {
			return nil, types.E_RANGE
		}
		return c.Set(i, value), types.E_NONE

	case types.StrValue:
		idx, ok := index.(types.IntValue)
		if !ok {
			return nil, types.E_TYPE
		}
		i := int(idx.Val)
		s := c.Value()
		if i < 1 || i > len(s) {
			return nil, types.E_RANGE
		}
		// Value must be a single-character string
		newChar, ok := value.(types.StrValue)
		if !ok || len(newChar.Value()) != 1 {
			return nil, types.E_INVARG
		}
		// Create new string with replaced character
		newStr := s[:i-1] + newChar.Value() + s[i:]
		return types.NewStr(newStr), types.E_NONE

	case types.MapValue:
		// Map assignment - key can be any valid map key (not list or map)
		if !types.IsValidMapKey(index) {
			return nil, types.E_TYPE
		}
		return c.Set(index, value), types.E_NONE

	default:
		return nil, types.E_TYPE
	}
}

// assignRange handles range assignment: coll[start..end] = value
// Also handles nested cases like: list[i][start..end] = value
func (e *Evaluator) assignRange(target *parser.RangeExpr, value types.Value, ctx *types.TaskContext) types.Result {
	// Check if this is a nested range assignment (e.g., l[3][2..$] = "u")
	if indexExpr, ok := target.Expr.(*parser.IndexExpr); ok {
		return e.nestedRangeAssign(indexExpr, target.Start, target.End, value, ctx)
	}

	// Get the collection (must be a variable reference)
	varName, ok := getBaseVariableFromRange(target)
	if !ok {
		return types.Err(types.E_TYPE) // Not assignable
	}

	// Get the current value of the variable
	collVal, exists := e.env.Get(varName)
	if !exists {
		return types.Err(types.E_VARNF)
	}

	// Get collection length for index marker resolution
	length := getCollectionLength(collVal)
	if length < 0 {
		return types.Err(types.E_TYPE)
	}

	// For maps, we may need to handle string keys for ranges
	// This is converted to position-based indices
	isMapWithKeyRange := false
	mapVal, isMap := collVal.(types.MapValue)

	// Resolve start index
	var startIdx int64
	if marker, ok := target.Start.(*parser.IndexMarkerExpr); ok {
		if marker.Marker == parser.TOKEN_CARET {
			startIdx = 1
		} else if marker.Marker == parser.TOKEN_DOLLAR {
			startIdx = int64(length)
		} else {
			return types.Err(types.E_TYPE)
		}
	} else {
		startResult := e.Eval(target.Start, ctx)
		if !startResult.IsNormal() {
			return startResult
		}
		if startInt, ok := startResult.Val.(types.IntValue); ok {
			startIdx = startInt.Val
		} else if isMap {
			// For maps, non-integer start means key-based range
			isMapWithKeyRange = true
			startIdx = mapVal.KeyPosition(startResult.Val)
			if startIdx == 0 {
				return types.Err(types.E_RANGE) // Key not found
			}
		} else {
			return types.Err(types.E_TYPE)
		}
	}

	// Resolve end index
	var endIdx int64
	if marker, ok := target.End.(*parser.IndexMarkerExpr); ok {
		if marker.Marker == parser.TOKEN_CARET {
			endIdx = 1
		} else if marker.Marker == parser.TOKEN_DOLLAR {
			endIdx = int64(length)
		} else {
			return types.Err(types.E_TYPE)
		}
	} else {
		endResult := e.Eval(target.End, ctx)
		if !endResult.IsNormal() {
			return endResult
		}
		if endInt, ok := endResult.Val.(types.IntValue); ok {
			endIdx = endInt.Val
		} else if isMap || isMapWithKeyRange {
			// For maps, non-integer end means key-based range
			endIdx = mapVal.KeyPosition(endResult.Val)
			if endIdx == 0 {
				return types.Err(types.E_RANGE) // Key not found
			}
		} else {
			return types.Err(types.E_TYPE)
		}
	}

	// Perform the assignment based on collection type
	var newColl types.Value
	switch coll := collVal.(type) {
	case types.ListValue:
		// Value must be a list
		newVals, ok := value.(types.ListValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}

		// For inverted ranges (startIdx > endIdx), MOO has special semantics:
		// t[7..1] = x means: t[1..6] + x + t[2..7]
		// i.e., keep elements before startIdx, insert new values, keep elements after endIdx
		isInverted := startIdx > endIdx+1

		// Bounds check for normal ranges
		if !isInverted {
			if startIdx < 1 || startIdx > int64(length)+1 {
				return types.Err(types.E_RANGE)
			}
			if endIdx < 0 || endIdx > int64(length) {
				return types.Err(types.E_RANGE)
			}
		} else {
			// Bounds check for inverted ranges
			if startIdx < 1 || startIdx > int64(length)+1 {
				return types.Err(types.E_RANGE)
			}
			if endIdx < 0 || endIdx > int64(length) {
				return types.Err(types.E_RANGE)
			}
		}

		// Build new list: [1..start-1] + newVals + [end+1..$]
		// For inverted ranges, this naturally duplicates elements
		result := make([]types.Value, 0)
		for i := 1; i < int(startIdx); i++ {
			result = append(result, coll.Get(i))
		}
		for i := 1; i <= newVals.Len(); i++ {
			result = append(result, newVals.Get(i))
		}
		for i := int(endIdx) + 1; i <= length; i++ {
			result = append(result, coll.Get(i))
		}
		newColl = types.NewList(result)

	case types.StrValue:
		// Value must be a string
		newStr, ok := value.(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}

		s := coll.Value()
		strLen := int64(len(s))

		// For inverted ranges (startIdx > endIdx), MOO has special semantics:
		// s[7..1] = x means: s[1..6] + x + s[2..7]
		isInverted := startIdx > endIdx+1

		// Bounds check for normal ranges
		// MOO allows startIdx up to strLen+1 for appending
		// And endIdx can be beyond strLen for appending (we just use strLen as the effective end)
		if !isInverted {
			if startIdx < 1 || startIdx > strLen+1 {
				return types.Err(types.E_RANGE)
			}
			// endIdx can be beyond strLen for append operations
			if endIdx < 0 {
				return types.Err(types.E_RANGE)
			}
		} else {
			// Bounds check for inverted ranges
			if startIdx < 1 || startIdx > strLen+1 {
				return types.Err(types.E_RANGE)
			}
			if endIdx < 0 {
				return types.Err(types.E_RANGE)
			}
		}

		// Clamp endIdx to actual string length for slicing
		effectiveEnd := endIdx
		if effectiveEnd > strLen {
			effectiveEnd = strLen
		}

		// Build new string: s[1..start-1] + newStr + s[end+1..$]
		// For inverted ranges, this naturally duplicates characters
		result := s[:startIdx-1] + newStr.Value() + s[effectiveEnd:]
		newColl = types.NewStr(result)

	case types.MapValue:
		// Value must be a map
		newMap, ok := value.(types.MapValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}

		// For inverted ranges (startIdx > endIdx), MOO has special semantics:
		// m[7..1] = x means: m[1..6] + x + m[2..7]
		isInverted := startIdx > endIdx+1

		// Bounds check for normal ranges
		if !isInverted {
			if startIdx < 1 || startIdx > int64(length)+1 {
				return types.Err(types.E_RANGE)
			}
			if endIdx < 0 || endIdx > int64(length) {
				return types.Err(types.E_RANGE)
			}
		} else {
			// Bounds check for inverted ranges
			if startIdx < 1 || startIdx > int64(length)+1 {
				return types.Err(types.E_RANGE)
			}
			if endIdx < 0 || endIdx > int64(length) {
				return types.Err(types.E_RANGE)
			}
		}

		// Build new map: pairs[1..start-1] + newMap + pairs[end+1..$]
		// For inverted ranges, this naturally duplicates pairs
		pairs := coll.Pairs()
		result := make([][2]types.Value, 0)
		for i := 0; i < int(startIdx)-1; i++ {
			result = append(result, pairs[i])
		}
		for _, pair := range newMap.Pairs() {
			result = append(result, pair)
		}
		for i := int(endIdx); i < length; i++ {
			result = append(result, pairs[i])
		}
		newColl = types.NewMap(result)

	default:
		return types.Err(types.E_TYPE)
	}

	// Store the new collection back to the variable
	e.env.Set(varName, newColl)
	return types.Ok(value)
}

// propertyIndexedAssign handles property-indexed assignment: obj.prop[idx] = value
// Also handles nested: obj.prop[i][j] = value
func (e *Evaluator) propertyIndexedAssign(propExpr *parser.PropertyExpr, indices []parser.Expr, value types.Value, ctx *types.TaskContext) types.Result {
	// Evaluate the object expression
	objResult := e.Eval(propExpr.Expr, ctx)
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
		return types.Err(types.E_INVIND)
	}

	// Read the current property value
	propVal, errCode := e.getPropertyValue(obj, propExpr.Property, ctx)
	if errCode != types.E_NONE {
		return types.Err(errCode)
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

	// Write the new value back to the property
	return e.assignProperty(propExpr, newPropVal, ctx)
}

// getPropertyValue retrieves a property value from an object
// Returns the value and E_NONE on success, or nil and an error code on failure
func (e *Evaluator) getPropertyValue(obj *db.Object, name string, ctx *types.TaskContext) (types.Value, types.ErrorCode) {
	// Check for built-in properties first
	if val, ok := e.getBuiltinProperty(obj, name); ok {
		return val, types.E_NONE
	}

	// Look up property with inheritance
	prop, errCode := e.findProperty(obj, name, ctx)
	if errCode != types.E_NONE {
		return nil, errCode
	}

	return prop.Value, types.E_NONE
}

// getBaseVariable extracts the variable name from an IndexExpr chain
// Returns the variable name and true if successful, or empty string and false otherwise
func getBaseVariable(expr *parser.IndexExpr) (string, bool) {
	switch base := expr.Expr.(type) {
	case *parser.IdentifierExpr:
		return base.Name, true
	case *parser.IndexExpr:
		// Nested indexing - not supported for assignment yet
		return "", false
	default:
		return "", false
	}
}

// getBaseVariableFromRange extracts the variable name from a RangeExpr
func getBaseVariableFromRange(expr *parser.RangeExpr) (string, bool) {
	switch base := expr.Expr.(type) {
	case *parser.IdentifierExpr:
		return base.Name, true
	default:
		return "", false
	}
}

// nestedRangeAssign handles nested range assignment like: l[3][2..$] = "u"
// This replaces part of a nested collection element
func (e *Evaluator) nestedRangeAssign(indexExpr *parser.IndexExpr, start, end parser.Expr, value types.Value, ctx *types.TaskContext) types.Result {
	// Build path of indices from the IndexExpr chain
	var indices []parser.Expr
	var baseVarName string
	current := indexExpr

	for {
		indices = append([]parser.Expr{current.Index}, indices...) // Prepend to reverse order
		switch base := current.Expr.(type) {
		case *parser.IndexExpr:
			current = base
		case *parser.IdentifierExpr:
			baseVarName = base.Name
			goto foundBase
		default:
			return types.Err(types.E_TYPE)
		}
	}
foundBase:

	// Get the root collection
	rootVal, exists := e.env.Get(baseVarName)
	if !exists {
		return types.Err(types.E_VARNF)
	}

	// Traverse down to get the innermost element that will have the range applied
	collections := make([]types.Value, len(indices)+1)
	resolvedIndices := make([]types.Value, len(indices))
	collections[0] = rootVal

	for i := 0; i < len(indices); i++ {
		coll := collections[i]

		// Set IndexContext for index resolution
		length := getCollectionLength(coll)
		if length < 0 {
			return types.Err(types.E_TYPE)
		}
		oldContext := ctx.IndexContext
		ctx.IndexContext = length

		// Evaluate index
		indexResult := e.Eval(indices[i], ctx)
		ctx.IndexContext = oldContext
		if !indexResult.IsNormal() {
			return indexResult
		}
		resolvedIndices[i] = indexResult.Val

		// Get the nested element
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
			val, ok := c.Get(indexResult.Val)
			if !ok {
				return types.Err(types.E_RANGE)
			}
			nextVal = val
		default:
			return types.Err(types.E_TYPE)
		}
		collections[i+1] = nextVal
	}

	// Now apply the range assignment to the innermost element
	innerVal := collections[len(indices)]

	// Get length for range marker resolution
	length := getCollectionLength(innerVal)
	if length < 0 {
		// It might be a string which isn't a "collection" in the strict sense
		if _, ok := innerVal.(types.StrValue); !ok {
			return types.Err(types.E_TYPE)
		}
		length = len(innerVal.(types.StrValue).Value())
	}

	// Resolve start index
	oldContext := ctx.IndexContext
	ctx.IndexContext = length

	var startIdx int64
	if marker, ok := start.(*parser.IndexMarkerExpr); ok {
		if marker.Marker == parser.TOKEN_CARET {
			startIdx = 1
		} else if marker.Marker == parser.TOKEN_DOLLAR {
			startIdx = int64(length)
		}
	} else {
		startResult := e.Eval(start, ctx)
		if !startResult.IsNormal() {
			ctx.IndexContext = oldContext
			return startResult
		}
		startInt, ok := startResult.Val.(types.IntValue)
		if !ok {
			ctx.IndexContext = oldContext
			return types.Err(types.E_TYPE)
		}
		startIdx = startInt.Val
	}

	// Resolve end index
	var endIdx int64
	if marker, ok := end.(*parser.IndexMarkerExpr); ok {
		if marker.Marker == parser.TOKEN_CARET {
			endIdx = 1
		} else if marker.Marker == parser.TOKEN_DOLLAR {
			endIdx = int64(length)
		}
	} else {
		endResult := e.Eval(end, ctx)
		if !endResult.IsNormal() {
			ctx.IndexContext = oldContext
			return endResult
		}
		endInt, ok := endResult.Val.(types.IntValue)
		if !ok {
			ctx.IndexContext = oldContext
			return types.Err(types.E_TYPE)
		}
		endIdx = endInt.Val
	}
	ctx.IndexContext = oldContext

	// Apply range assignment based on type
	var newInnerVal types.Value
	switch inner := innerVal.(type) {
	case types.StrValue:
		// Value must be a string
		newStr, ok := value.(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		s := inner.Value()
		strLen := int64(len(s))

		// Bounds check
		if startIdx < 1 || startIdx > strLen+1 {
			return types.Err(types.E_RANGE)
		}
		if endIdx < 0 {
			return types.Err(types.E_RANGE)
		}

		// Clamp endIdx
		effectiveEnd := endIdx
		if effectiveEnd > strLen {
			effectiveEnd = strLen
		}

		result := s[:startIdx-1] + newStr.Value() + s[effectiveEnd:]
		newInnerVal = types.NewStr(result)

	case types.ListValue:
		// Value must be a list
		newList, ok := value.(types.ListValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		listLen := int64(inner.Len())

		// Bounds check
		if startIdx < 1 || startIdx > listLen+1 {
			return types.Err(types.E_RANGE)
		}
		if endIdx < 0 || endIdx > listLen {
			return types.Err(types.E_RANGE)
		}

		result := make([]types.Value, 0)
		for i := 1; i < int(startIdx); i++ {
			result = append(result, inner.Get(i))
		}
		for i := 1; i <= newList.Len(); i++ {
			result = append(result, newList.Get(i))
		}
		for i := int(endIdx) + 1; i <= int(listLen); i++ {
			result = append(result, inner.Get(i))
		}
		newInnerVal = types.NewList(result)

	default:
		return types.Err(types.E_TYPE)
	}

	// Rebuild going back up (copy-on-write)
	for i := len(indices) - 1; i >= 0; i-- {
		var err types.ErrorCode
		newInnerVal, err = setAtIndex(collections[i], resolvedIndices[i], newInnerVal)
		if err != types.E_NONE {
			return types.Err(err)
		}
	}

	// Store back to variable
	e.env.Set(baseVarName, newInnerVal)
	return types.Ok(value)
}
