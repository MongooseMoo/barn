package builtins

import (
	"barn/types"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// builtinTypeof returns the type code of a value
// typeof(value) -> int (TYPE_INT=0, TYPE_OBJ=1, TYPE_STR=2, etc.)
func builtinTypeof(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	return types.Ok(types.IntValue{Val: int64(args[0].Type())})
}

// builtinTostr converts a value to its string representation
// tostr(value) -> str
// For most types, returns the MOO literal representation
func builtinTostr(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	val := args[0]

	switch v := val.(type) {
	case types.StrValue:
		// Already a string, return as-is
		return types.Ok(v)

	case types.IntValue:
		// Convert integer to string
		return types.Ok(types.NewStr(fmt.Sprintf("%d", v.Val)))

	case types.FloatValue:
		// Convert float to string
		// MOO expects whole numbers to still show decimal (3.0 not 3)
		s := strconv.FormatFloat(v.Val, 'g', -1, 64)
		// Add .0 if no decimal point and not in scientific notation
		if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") {
			s += ".0"
		}
		return types.Ok(types.NewStr(s))

	case types.ObjValue:
		// Convert object to string: #123
		return types.Ok(types.NewStr(fmt.Sprintf("#%d", v.ID())))

	case types.ErrValue:
		// Convert error to string: E_TYPE
		return types.Ok(types.NewStr(v.String()))

	case types.BoolValue:
		// Convert bool to string
		if v.Val {
			return types.Ok(types.NewStr("true"))
		}
		return types.Ok(types.NewStr("false"))

	case types.ListValue:
		// MOO tostr() on list returns "{list}", not the literal representation
		return types.Ok(types.NewStr("{list}"))

	case types.MapValue:
		// MOO tostr() on map returns "[map]", not the literal representation
		return types.Ok(types.NewStr("[map]"))

	default:
		// Unknown type - shouldn't happen
		return types.Err(types.E_TYPE)
	}
}

// builtinToint converts a value to an integer
// toint(str) -> int (parse string as integer)
// toint(float) -> int (truncate to integer)
// toint(obj) -> int (object ID)
// toint(int) -> int (identity)
func builtinToint(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	val := args[0]

	switch v := val.(type) {
	case types.IntValue:
		// Already an int, return as-is
		return types.Ok(v)

	case types.FloatValue:
		// Truncate float to int
		return types.Ok(types.IntValue{Val: int64(v.Val)})

	case types.ObjValue:
		// Object ID as int
		return types.Ok(types.IntValue{Val: int64(v.ID())})

	case types.StrValue:
		// Parse string as integer
		str := strings.TrimSpace(v.Value())
		i, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.IntValue{Val: i})

	default:
		// Cannot convert this type to int
		return types.Err(types.E_TYPE)
	}
}

// builtinTofloat converts a value to a float
// tofloat(int) -> float (convert to float)
// tofloat(str) -> float (parse string as float)
// tofloat(float) -> float (identity)
func builtinTofloat(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	val := args[0]

	switch v := val.(type) {
	case types.FloatValue:
		// Already a float, return as-is
		return types.Ok(v)

	case types.IntValue:
		// Convert int to float
		return types.Ok(types.FloatValue{Val: float64(v.Val)})

	case types.StrValue:
		// Parse string as float
		str := strings.TrimSpace(v.Value())
		f, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.FloatValue{Val: f})

	default:
		// Cannot convert this type to float
		return types.Err(types.E_TYPE)
	}
}

// builtinToliteral converts a value to its MOO literal string representation
// toliteral(value) -> str
func builtinToliteral(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	return types.Ok(types.NewStr(args[0].String()))
}

// builtinToobj converts a value to an object reference
// toobj(int) -> obj (object with that ID)
// toobj(str) -> obj (parse "#123" format)
// toobj(obj) -> obj (identity)
func builtinToobj(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	val := args[0]

	switch v := val.(type) {
	case types.ObjValue:
		return types.Ok(v)

	case types.IntValue:
		return types.Ok(types.NewObj(types.ObjID(v.Val)))

	case types.StrValue:
		str := strings.TrimSpace(v.Value())
		// Parse "#123" format
		if len(str) > 0 && str[0] == '#' {
			str = str[1:]
		}
		i, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			// Invalid string - return #0 per MOO semantics
			return types.Ok(types.NewObj(0))
		}
		return types.Ok(types.NewObj(types.ObjID(i)))

	default:
		return types.Err(types.E_TYPE)
	}
}

// builtinEqual tests deep equality of two values
// equal(val1, val2) -> bool
// For maps, this is case-SENSITIVE (unlike == operator)
func builtinEqual(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if strictEqual(args[0], args[1]) {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// strictEqual performs case-sensitive deep equality comparison
// This is used by equal() builtin, not by == operator
func strictEqual(a, b types.Value) bool {
	// For maps, do case-sensitive comparison of keys and values
	aMap, aIsMap := a.(types.MapValue)
	bMap, bIsMap := b.(types.MapValue)
	if aIsMap && bIsMap {
		if aMap.Len() != bMap.Len() {
			return false
		}
		aPairs := aMap.Pairs()
		bPairs := bMap.Pairs()
		// Compare in sorted order
		sortPairs(aPairs)
		sortPairs(bPairs)
		for i, ap := range aPairs {
			bp := bPairs[i]
			if !strictEqual(ap[0], bp[0]) || !strictEqual(ap[1], bp[1]) {
				return false
			}
		}
		return true
	}

	// For lists, recursively check with strictEqual
	aList, aIsList := a.(types.ListValue)
	bList, bIsList := b.(types.ListValue)
	if aIsList && bIsList {
		if aList.Len() != bList.Len() {
			return false
		}
		for i := 1; i <= aList.Len(); i++ {
			if !strictEqual(aList.Get(i), bList.Get(i)) {
				return false
			}
		}
		return true
	}

	// For strings, case-SENSITIVE comparison
	aStr, aIsStr := a.(types.StrValue)
	bStr, bIsStr := b.(types.StrValue)
	if aIsStr && bIsStr {
		return aStr.Value() == bStr.Value()
	}

	// For other types, use standard Equal
	return a.Equal(b)
}

// sortPairs sorts key-value pairs by key for consistent comparison
func sortPairs(pairs [][2]types.Value) {
	sort.Slice(pairs, func(i, j int) bool {
		return comparePairKeys(pairs[i][0], pairs[j][0]) < 0
	})
}

// comparePairKeys compares two keys for sorting
// Order: INT (0) < OBJ (1) < FLOAT (2) < ERR (3) < STR (4)
// This matches MOO/ToastStunt map key ordering
func comparePairKeys(a, b types.Value) int {
	typeOrder := func(v types.Value) int {
		switch v.Type() {
		case types.TYPE_INT:
			return 0
		case types.TYPE_OBJ:
			return 1
		case types.TYPE_FLOAT:
			return 2
		case types.TYPE_ERR:
			return 3
		case types.TYPE_STR:
			return 4
		default:
			return 5
		}
	}

	aOrder := typeOrder(a)
	bOrder := typeOrder(b)
	if aOrder != bOrder {
		return aOrder - bOrder
	}

	// Same type, compare values
	switch av := a.(type) {
	case types.IntValue:
		bv := b.(types.IntValue)
		if av.Val < bv.Val {
			return -1
		} else if av.Val > bv.Val {
			return 1
		}
		return 0
	case types.StrValue:
		bv := b.(types.StrValue)
		return strings.Compare(av.Value(), bv.Value())
	}
	return 0
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// listToString converts a ListValue to its MOO string representation
func listToString(list types.ListValue) string {
	if list.Len() == 0 {
		return "{}"
	}

	parts := make([]string, list.Len())
	for i := 1; i <= list.Len(); i++ {
		elem := list.Get(i)
		parts[i-1] = elem.String()
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// mapToString converts a MapValue to its MOO string representation
func mapToString(m types.MapValue) string {
	pairs := m.Pairs()
	if len(pairs) == 0 {
		return "[]"
	}

	parts := make([]string, len(pairs))
	for i, pair := range pairs {
		parts[i] = pair[0].String() + " -> " + pair[1].String()
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
