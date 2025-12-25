package builtins

import (
	"barn/types"
	"fmt"
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
		// Use Go's default formatting which handles scientific notation
		return types.Ok(types.NewStr(fmt.Sprintf("%g", v.Val)))

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
		// Convert list to string representation: {1, 2, 3}
		return types.Ok(types.NewStr(listToString(v)))

	case types.MapValue:
		// Convert map to string representation: ["a" -> 1, "b" -> 2]
		return types.Ok(types.NewStr(mapToString(v)))

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
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.NewObj(types.ObjID(i)))

	default:
		return types.Err(types.E_TYPE)
	}
}

// builtinEqual tests deep equality of two values
// equal(val1, val2) -> bool
func builtinEqual(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Equal(args[1]) {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
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
