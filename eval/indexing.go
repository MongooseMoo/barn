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

	// Resolve index - handle special markers ^ (first) and $ (last)
	var index types.Value
	if marker, ok := node.Index.(*parser.IndexMarkerExpr); ok {
		// Get collection length for $ resolution
		length := getCollectionLength(expr)
		if length < 0 {
			return types.Err(types.E_TYPE) // Not a collection
		}

		if marker.Marker == parser.TOKEN_CARET {
			index = types.NewInt(1) // ^ means first (index 1)
		} else if marker.Marker == parser.TOKEN_DOLLAR {
			index = types.NewInt(int64(length)) // $ means last
		} else {
			return types.Err(types.E_TYPE)
		}
	} else {
		// Normal index - evaluate it
		indexResult := e.Eval(node.Index, ctx)
		if !indexResult.IsNormal() {
			return indexResult
		}
		index = indexResult.Val
	}

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

	// Resolve start index - handle special markers ^ (first) and $ (last)
	var startIdx int64
	if marker, ok := node.Start.(*parser.IndexMarkerExpr); ok {
		if marker.Marker == parser.TOKEN_CARET {
			startIdx = 1 // ^ means first (index 1)
		} else if marker.Marker == parser.TOKEN_DOLLAR {
			startIdx = int64(length) // $ means last
		} else {
			return types.Err(types.E_TYPE)
		}
	} else {
		startResult := e.Eval(node.Start, ctx)
		if !startResult.IsNormal() {
			return startResult
		}
		startInt, ok := startResult.Val.(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		startIdx = startInt.Val
	}

	// Resolve end index - handle special markers ^ (first) and $ (last)
	var endIdx int64
	if marker, ok := node.End.(*parser.IndexMarkerExpr); ok {
		if marker.Marker == parser.TOKEN_CARET {
			endIdx = 1 // ^ means first (index 1)
		} else if marker.Marker == parser.TOKEN_DOLLAR {
			endIdx = int64(length) // $ means last
		} else {
			return types.Err(types.E_TYPE)
		}
	} else {
		endResult := e.Eval(node.End, ctx)
		if !endResult.IsNormal() {
			return endResult
		}
		endInt, ok := endResult.Val.(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		endIdx = endInt.Val
	}

	// Dispatch based on collection type
	switch coll := expr.(type) {
	case types.ListValue:
		return evalListRange(coll, startIdx, endIdx)
	case types.StrValue:
		return evalStrRange(coll, startIdx, endIdx)
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
// MOO uses copy-on-write semantics: creates new collection with modified element
func (e *Evaluator) evalAssignIndex(target *parser.IndexExpr, value types.Value, ctx *types.TaskContext) types.Result {
	// Get the collection (must be a variable reference)
	varName, ok := getBaseVariable(target)
	if !ok {
		return types.Err(types.E_TYPE) // Not assignable
	}

	// Get the current value of the variable
	collVal, exists := e.env.Get(varName)
	if !exists {
		return types.Err(types.E_VARNF)
	}

	// Resolve the index - handle special markers ^ and $
	var indexVal types.Value
	if marker, ok := target.Index.(*parser.IndexMarkerExpr); ok {
		length := getCollectionLength(collVal)
		if length < 0 {
			return types.Err(types.E_TYPE)
		}

		if marker.Marker == parser.TOKEN_CARET {
			indexVal = types.NewInt(1)
		} else if marker.Marker == parser.TOKEN_DOLLAR {
			indexVal = types.NewInt(int64(length))
		} else {
			return types.Err(types.E_TYPE)
		}
	} else {
		indexResult := e.Eval(target.Index, ctx)
		if !indexResult.IsNormal() {
			return indexResult
		}
		indexVal = indexResult.Val
	}

	// Perform the assignment based on collection type
	var newColl types.Value
	switch coll := collVal.(type) {
	case types.ListValue:
		idx, ok := indexVal.(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		i := int(idx.Val)
		if i < 1 || i > coll.Len() {
			return types.Err(types.E_RANGE)
		}
		newColl = coll.Set(i, value)

	case types.StrValue:
		idx, ok := indexVal.(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		i := int(idx.Val)
		s := coll.Value()
		if i < 1 || i > len(s) {
			return types.Err(types.E_RANGE)
		}
		// Value must be a single-character string
		newChar, ok := value.(types.StrValue)
		if !ok || len(newChar.Value()) != 1 {
			return types.Err(types.E_INVARG)
		}
		// Create new string with replaced character
		newStr := s[:i-1] + newChar.Value() + s[i:]
		newColl = types.NewStr(newStr)

	case types.MapValue:
		// Map assignment - key can be any valid map key
		newColl = coll.Set(indexVal, value)

	default:
		return types.Err(types.E_TYPE)
	}

	// Store the new collection back to the variable
	e.env.Set(varName, newColl)
	return types.Ok(value)
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
