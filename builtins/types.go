package builtins

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/types"
)

// builtinTypeof returns the type code of a value
// typeof(value) -> int (TYPE_INT=0, TYPE_OBJ=1, TYPE_STR=2, etc.)
func builtinTypeof(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	return types.Ok(types.NewInt(int64(args[0].Type())))
}

// builtinTostr converts values to strings and concatenates them
// tostr(value, ...) -> str
// Accepts any number of arguments (0 or more), converts each to string, concatenates
func builtinTostr(ctx *Execution, args []types.Value) types.Result {
	// tostr() with no args returns empty string
	if len(args) == 0 {
		return types.Ok(types.NewStr(""))
	}

	var result strings.Builder
	for _, val := range args {
		result.WriteString(valueToStr(val))
	}

	// Check string length limit (update from load_server_options cache first)
	ctx.Session.UpdateContextLimits(ctx.TaskContext)
	resultStr := result.String()
	if err := ctx.CheckStringLimit(len(resultStr)); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(types.NewStr(resultStr))
}

// valueToStr converts a single value to its string representation
func valueToStr(val types.Value) string {
	switch val.Type() {
	case types.TYPE_STR:
		return val.Str()

	case types.TYPE_INT:
		return strconv.FormatInt(val.Int(), 10)

	case types.TYPE_FLOAT:
		// Delegate to the canonical float formatting (15 significant digits,
		// ToastStunt-compatible) so tostr() matches value output and toliteral().
		return val.String()

	case types.TYPE_OBJ:
		return fmt.Sprintf("#%d", val.ID())

	case types.TYPE_ANON:
		return "*anonymous*"

	case types.TYPE_WAIF:
		return "[[waif]]"

	case types.TYPE_ERR:
		return val.Code().Message()

	case types.TYPE_BOOL:
		if val.Bool() {
			return "true"
		}
		return "false"

	case types.TYPE_LIST:
		return "{list}"

	case types.TYPE_MAP:
		return "[map]"

	default:
		return ""
	}
}

// builtinToint converts a value to an integer
// toint(str) -> int (parse string as integer)
// toint(float) -> int (truncate to integer)
// toint(obj) -> int (object ID)
// toint(int) -> int (identity)
func builtinToint(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	val := args[0]

	switch val.Type() {
	case types.TYPE_INT:
		// Already an int, return as-is
		return types.Ok(val)

	case types.TYPE_FLOAT:
		// Truncate float to int
		return types.Ok(types.NewInt(int64(val.Float())))

	case types.TYPE_OBJ:
		// Object ID as int
		return types.Ok(types.NewInt(int64(val.ID())))

	case types.TYPE_ERR:
		// Error code ordinal as int
		return types.Ok(types.NewInt(int64(val.Code())))

	case types.TYPE_BOOL:
		if val.Bool() {
			return types.Ok(types.NewInt(1))
		}
		return types.Ok(types.NewInt(0))

	case types.TYPE_STR:
		// Parse string as integer first. If that fails, parse as float and truncate.
		// Per MOO semantics: returns 0 for unparseable strings (not E_INVARG).
		str := strings.TrimSpace(val.Str())
		i, err := strconv.ParseInt(str, 10, 64)
		if err == nil {
			return types.Ok(types.NewInt(i))
		}
		// On a pure-numeric overflow, ParseInt returns the saturated
		// MaxInt64/MinInt64 together with ErrRange. MOO's strtoll-based toint()
		// clamps the same way, so honor that value rather than falling through
		// to the float path (which would wrap to the wrong extreme).
		if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
			return types.Ok(types.NewInt(i))
		}
		if f, ferr := strconv.ParseFloat(str, 64); ferr == nil {
			return types.Ok(types.NewInt(int64(f)))
		}
		return types.Ok(types.NewInt(0))

	default:
		// Cannot convert this type to int
		return types.Err(types.E_TYPE)
	}
}

// builtinTofloat converts a value to a float
// tofloat(int) -> float (convert to float)
// tofloat(str) -> float (parse string as float)
// tofloat(float) -> float (identity)
func builtinTofloat(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	val := args[0]

	switch val.Type() {
	case types.TYPE_FLOAT:
		// Already a float, return as-is
		return types.Ok(val)

	case types.TYPE_INT:
		// Convert int to float
		return types.Ok(types.NewFloat(float64(val.Int())))

	case types.TYPE_OBJ:
		// Object ID as float
		return types.Ok(types.NewFloat(float64(val.ID())))

	case types.TYPE_ERR:
		// Error code ordinal as float
		return types.Ok(types.NewFloat(float64(val.Code())))

	case types.TYPE_STR:
		// Parse string as float. Go's ParseFloat accepts "inf"/"nan" tokens,
		// but MOO's strtod-based tofloat() does not: a non-finite result is
		// E_INVARG (as is an out-of-range magnitude, which ParseFloat already
		// reports via err).
		str := strings.TrimSpace(val.Str())
		if strings.HasPrefix(str, "0x") || strings.HasPrefix(str, "0X") ||
			strings.HasPrefix(str, "-0x") || strings.HasPrefix(str, "-0X") ||
			strings.HasPrefix(str, "+0x") || strings.HasPrefix(str, "+0X") {
			if integer, err := strconv.ParseInt(str, 0, 64); err == nil {
				return types.Ok(types.NewFloat(float64(integer)))
			}
			return types.Err(types.E_INVARG)
		}
		f, err := strconv.ParseFloat(str, 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.NewFloat(f))

	default:
		// Cannot convert this type to float
		return types.Err(types.E_TYPE)
	}
}

// builtinToliteral converts a value to its MOO literal string representation
// toliteral(value) -> str
func builtinToliteral(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	resultStr := publicLiteral(args[0])

	// Check string length limit (update from load_server_options cache first)
	ctx.Session.UpdateContextLimits(ctx.TaskContext)
	if err := ctx.CheckStringLimit(len(resultStr)); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(types.NewStr(resultStr))
}

func publicLiteral(value types.Value) string {
	switch value.Type() {
	case types.TYPE_ANON:
		return "*anonymous*"
	case types.TYPE_WAIF:
		return fmt.Sprintf("[[class = #%d, owner = #%d]]", value.Class(), value.Owner())
	case types.TYPE_LIST:
		elements := value.Elements()
		parts := make([]string, len(elements))
		for i := range elements {
			parts[i] = publicLiteral(elements[i])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case types.TYPE_MAP:
		// Tree order — Toast's toliteral walks the rbtree with no re-sort.
		pairs := value.Pairs()
		parts := make([]string, len(pairs))
		for i := range pairs {
			parts[i] = publicLiteral(pairs[i][0]) + " -> " + publicLiteral(pairs[i][1])
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return value.String()
	}
}

// builtinToobj converts a value to an object reference
// toobj(int) -> obj (object with that ID)
// toobj(str) -> obj (parse "#123" format)
// toobj(obj) -> obj (identity)
func builtinToobj(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	val := args[0]

	switch val.Type() {
	case types.TYPE_OBJ:
		return types.Ok(val)

	case types.TYPE_INT:
		return types.Ok(types.NewObj(types.ObjID(val.Int())))

	case types.TYPE_FLOAT:
		return types.Ok(types.NewObj(types.ObjID(int64(val.Float()))))

	case types.TYPE_ERR:
		return types.Ok(types.NewObj(types.ObjID(val.Code())))

	case types.TYPE_BOOL:
		if val.Bool() {
			return types.Ok(types.NewObj(1))
		}
		return types.Ok(types.NewObj(0))

	case types.TYPE_STR:
		str := strings.TrimSpace(val.Str())
		// Parse "#123" format
		if len(str) > 0 && str[0] == '#' {
			str = str[1:]
		}
		i, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
				return types.Ok(types.NewObj(types.ObjID(i)))
			}
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
func builtinEqual(ctx *Execution, args []types.Value) types.Result {
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
	if a.Type() == types.TYPE_MAP && b.Type() == types.TYPE_MAP {
		if a.Len() != b.Len() {
			return false
		}
		aPairs := a.Pairs()
		bPairs := b.Pairs()
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
	if a.Type() == types.TYPE_LIST && b.Type() == types.TYPE_LIST {
		if a.Len() != b.Len() {
			return false
		}
		for i := 1; i <= a.Len(); i++ {
			if !strictEqual(a.Get(i), b.Get(i)) {
				return false
			}
		}
		return true
	}

	// For strings, case-SENSITIVE comparison
	if a.Type() == types.TYPE_STR && b.Type() == types.TYPE_STR {
		return a.Str() == b.Str()
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
	switch a.Type() {
	case types.TYPE_INT:
		if a.Int() < b.Int() {
			return -1
		} else if a.Int() > b.Int() {
			return 1
		}
		return 0
	case types.TYPE_STR:
		return strings.Compare(a.Str(), b.Str())
	case types.TYPE_ERR:
		if a.Code() < b.Code() {
			return -1
		} else if a.Code() > b.Code() {
			return 1
		}
		return 0
	}
	return 0
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// listToString converts a list value to its MOO string representation
// mapToString converts a map value to its MOO string representation
