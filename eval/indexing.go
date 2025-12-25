package eval

import (
	"barn/parser"
	"barn/types"
)

// evalIndex evaluates indexing: expr[index]
// Supports: lists, strings, and maps
func (e *Evaluator) evalIndex(node *parser.IndexExpr, ctx *types.TaskContext) types.Result {
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
		return evalListIndex(coll, index)
	case types.StrValue:
		return evalStrIndex(coll, index)
	case types.MapValue:
		return evalMapIndex(coll, index)
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

// evalRange evaluates range indexing: expr[start..end]
// Supports: lists and strings
func (e *Evaluator) evalRange(node *parser.RangeExpr, ctx *types.TaskContext) types.Result {
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
		return evalListRange(coll, startIdx, endIdx)
	case types.StrValue:
		return evalStrRange(coll, startIdx, endIdx)
	case types.MapValue:
		return evalMapRange(coll, startIdx, endIdx)
	default:
		return types.Err(types.E_TYPE)
	}
}

// evalListIndex evaluates list indexing
func evalListIndex(list types.ListValue, index types.Value) types.Result {
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

// evalListRange evaluates list range indexing
func evalListRange(list types.ListValue, start, end int64) types.Result {
	length := int64(list.Len())

	// Check bounds
	if start < 1 || start > length {
		return types.Err(types.E_RANGE)
	}
	if end < 1 || end > length {
		return types.Err(types.E_RANGE)
	}

	// If start > end, return empty list
	if start > end {
		return types.Ok(types.NewList([]types.Value{}))
	}

	// Extract slice (1-based to 0-based conversion)
	result := []types.Value{}
	for i := start; i <= end; i++ {
		val := list.Get(int(i))
		result = append(result, val)
	}

	return types.Ok(types.NewList(result))
}

// evalStrIndex evaluates string indexing (returns single character)
func evalStrIndex(str types.StrValue, index types.Value) types.Result {
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

// evalStrRange evaluates string range indexing (returns substring)
func evalStrRange(str types.StrValue, start, end int64) types.Result {
	// Get underlying string
	s := str.Value()
	length := int64(len(s))

	// Check bounds
	if start < 1 || start > length {
		return types.Err(types.E_RANGE)
	}
	if end < 1 || end > length {
		return types.Err(types.E_RANGE)
	}

	// If start > end, return empty string
	if start > end {
		return types.Ok(types.NewStr(""))
	}

	// Extract substring (1-based to 0-based conversion, Go slice is [start:end+1])
	substr := s[start-1 : end]
	return types.Ok(types.NewStr(substr))
}

// evalMapRange evaluates map range indexing (returns submap)
// Maps are indexed by position, not key, for range operations
func evalMapRange(m types.MapValue, start, end int64) types.Result {
	length := int64(m.Len())

	// Check bounds
	if start < 1 || start > length {
		return types.Err(types.E_RANGE)
	}
	if end < 1 || end > length {
		return types.Err(types.E_RANGE)
	}

	// If start > end, return empty map
	if start > end {
		return types.Ok(types.NewEmptyMap())
	}

	// Extract pairs in range (1-based indexing)
	pairs := m.Pairs()
	result := make([][2]types.Value, 0, int(end-start+1))
	for i := start; i <= end; i++ {
		result = append(result, pairs[i-1])
	}

	return types.Ok(types.NewMap(result))
}

// evalMapIndex evaluates map indexing
func evalMapIndex(m types.MapValue, key types.Value) types.Result {
	// Look up key in map
	val, ok := m.Get(key)
	if !ok {
		return types.Err(types.E_RANGE)
	}

	return types.Ok(val)
}

// evalAssignIndex handles index assignment: coll[idx] = value
// Also handles nested assignment: coll[i][j][k] = value (copy-on-write)
func (e *Evaluator) evalAssignIndex(target *parser.IndexExpr, value types.Value, ctx *types.TaskContext) types.Result {
	// Build path of indices from the target expression
	var path []parser.Expr // Index expressions, innermost first
	var current parser.Expr = target

	// Walk up the chain to find the base variable
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
			return e.evalNestedAssign(expr.Name, path, value, ctx)
		default:
			return types.Err(types.E_TYPE) // Not assignable
		}
	}
}

// evalNestedAssign handles nested index assignment with copy-on-write semantics
func (e *Evaluator) evalNestedAssign(varName string, indices []parser.Expr, value types.Value, ctx *types.TaskContext) types.Result {
	// Get the root collection
	rootVal, exists := e.env.Get(varName)
	if !exists {
		return types.Err(types.E_VARNF)
	}

	// For single-level assignment, use simple path
	if len(indices) == 1 {
		return e.evalSimpleIndexAssign(varName, rootVal, indices[0], value, ctx)
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

// evalSimpleIndexAssign handles single-level index assignment
func (e *Evaluator) evalSimpleIndexAssign(varName string, collVal types.Value, indexExpr parser.Expr, value types.Value, ctx *types.TaskContext) types.Result {
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
		// Map assignment - key can be any valid map key
		return c.Set(index, value), types.E_NONE

	default:
		return nil, types.E_TYPE
	}
}

// evalAssignRange handles range assignment: coll[start..end] = value
func (e *Evaluator) evalAssignRange(target *parser.RangeExpr, value types.Value, ctx *types.TaskContext) types.Result {
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
		startInt, ok := startResult.Val.(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		startIdx = startInt.Val
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
		endInt, ok := endResult.Val.(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		endIdx = endInt.Val
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

		// Bounds check
		if startIdx < 1 || startIdx > int64(length)+1 {
			return types.Err(types.E_RANGE)
		}
		if endIdx < 0 || endIdx > int64(length) {
			return types.Err(types.E_RANGE)
		}

		// Build new list: [1..start-1] + newVals + [end+1..$]
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

		// Bounds check
		if startIdx < 1 || startIdx > int64(len(s))+1 {
			return types.Err(types.E_RANGE)
		}
		if endIdx < 0 || endIdx > int64(len(s)) {
			return types.Err(types.E_RANGE)
		}

		// Build new string: s[1..start-1] + newStr + s[end+1..$]
		result := s[:startIdx-1] + newStr.Value() + s[endIdx:]
		newColl = types.NewStr(result)

	case types.MapValue:
		// Value must be a map
		newMap, ok := value.(types.MapValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}

		// Bounds check
		if startIdx < 1 || startIdx > int64(length)+1 {
			return types.Err(types.E_RANGE)
		}
		if endIdx < 0 || endIdx > int64(length) {
			return types.Err(types.E_RANGE)
		}

		// Build new map: pairs[1..start-1] + newMap + pairs[end+1..$]
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
