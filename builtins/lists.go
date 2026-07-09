package builtins

import (
	"fmt"
	"sort"

	"barn/kernel"
	"barn/types"
)

// ============================================================================
// LAYER 7.2: LIST BUILTINS
// ============================================================================

// builtinListappend inserts value after the specified position
// listappend(list, value [, index]) -> list
// Index range: 0 to length(list), default: length(list) (appends)
func builtinListappend(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	value := args[1]

	// Default: append to end
	index := list.Len()
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		index = int(args[2].Int())
		if index < 0 || index > list.Len() {
			return types.Err(types.E_RANGE)
		}
	}

	// Insert after index
	result := list.InsertAt(index+1, value)

	// Check size limit
	if err := CheckListLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinListinsert inserts value before the specified position
// listinsert(list, value [, index]) -> list
// Index range: 1 to length(list)+1, default: 1 (prepend)
// Out of bounds indices are clamped
func builtinListinsert(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	value := args[1]

	// Default: insert at beginning
	index := 1
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		index = int(args[2].Int())
		// Clamp to valid range
		if index <= 0 {
			index = 1
		} else if index > list.Len()+1 {
			index = list.Len() + 1
		}
	}

	// Insert at index (1-based)
	result := list.InsertAt(index, value)

	// Check size limit
	if err := CheckListLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinListdelete removes element at index
// listdelete(list, index) -> list
func builtinListdelete(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	if args[1].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}

	index := int(args[1].Int())
	if index < 1 || index > list.Len() {
		return types.Err(types.E_RANGE)
	}

	result := list.DeleteAt(index)

	// Check size limit (even for deletions, to be thorough)
	if err := CheckListLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinListset replaces element at index
// listset(list, value, index) -> list
func builtinListset(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	value := args[1]

	if args[2].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}

	index := int(args[2].Int())
	if index < 1 || index > list.Len() {
		return types.Err(types.E_RANGE)
	}

	result := list.Set(index, value)

	// Check size limit
	if err := CheckListLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinSetadd adds value if not already present
// setadd(list, value) -> list
func builtinSetadd(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	value := args[1]

	// Check if value already exists
	for i := 1; i <= list.Len(); i++ {
		if list.Get(i).Equal(value) {
			return types.Ok(list) // Already present, return unchanged
		}
	}

	// Not present, append
	result := list.Append(value)

	// Check size limit
	if err := CheckListLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
}

// builtinSetremove removes first occurrence of value
// setremove(list, value) -> list
func builtinSetremove(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	value := args[1]

	// Find first occurrence
	for i := 1; i <= list.Len(); i++ {
		if list.Get(i).Equal(value) {
			result := list.DeleteAt(i)

			// Check size limit
			if err := CheckListLimit(result); err != types.E_NONE {
				return types.Err(err)
			}

			return types.Ok(result)
		}
	}

	// Not found, return unchanged
	return types.Ok(list)
}

// builtinIsMember tests if value is in list
// is_member(value, list) -> int (1-based index or 0)
// promoteMemberEqual implements PROMOTE_NUMBERS membership equality for
// is_member: when promotion is on and the pair is mixed int/float, compare as
// doubles (mongoose utils.cc coercion). handled is false when the fallback
// (strictEqual) should decide instead.
func promoteMemberEqual(ctx *kernel.TaskContext, a, b types.Value) (equal bool, handled bool) {
	if ctx == nil || !ctx.RuntimeOptions.PromoteNumbers {
		return false, false
	}
	aIsInt := a.Type() == types.TYPE_INT
	bIsInt := b.Type() == types.TYPE_INT
	aIsFloat := a.Type() == types.TYPE_FLOAT
	bIsFloat := b.Type() == types.TYPE_FLOAT
	if !((aIsInt && bIsFloat) || (aIsFloat && bIsInt)) {
		return false, false
	}
	toF := func(v types.Value) float64 {
		if v.Type() == types.TYPE_INT {
			return float64(v.Int())
		}
		return v.Float()
	}
	return toF(a) == toF(b), true
}

func builtinIsMember(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	caseMatters := true
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		caseMatters = args[2].Int() != 0
	}

	value := args[0]

	switch args[1].Type() {
	case types.TYPE_LIST:
		collection := args[1]
		// Find value in list (case-sensitive for strings)
		for i := 1; i <= collection.Len(); i++ {
			item := collection.Get(i)
			if eq, handled := promoteMemberEqual(ctx, value, item); handled {
				if eq {
					return types.Ok(types.NewInt(int64(i)))
				}
				continue
			}
			if memberEqual(item, value, caseMatters) {
				return types.Ok(types.NewInt(int64(i)))
			}
		}
		return types.Ok(types.NewInt(0))

	case types.TYPE_MAP:
		collection := args[1]
		// For maps, is_member searches for a VALUE and returns the position
		// of its key in the sorted key list (1-based), or 0 if not found
		// This is case-SENSITIVE for string values (uses strictEqual)
		pairs := collection.Pairs()
		sortMapPairs(pairs)
		for i, pair := range pairs {
			if eq, handled := promoteMemberEqual(ctx, value, pair[1]); handled {
				if eq {
					return types.Ok(types.NewInt(int64(i + 1)))
				}
				continue
			}
			if memberEqual(pair[1], value, caseMatters) {
				return types.Ok(types.NewInt(int64(i + 1)))
			}
		}
		return types.Ok(types.NewInt(0))

	default:
		return types.Err(types.E_INVARG)
	}
}

func memberEqual(a, b types.Value, caseMatters bool) bool {
	if caseMatters {
		return strictEqual(a, b)
	}
	return a.Equal(b)
}

// builtinSort sorts a list, matching ToastStunt's bf_sort / sort_callback.
//
// Signature (toaststunt src/list.cc:1779, sort_callback 947-1020):
//
//		sort(list [, keys] [, natural] [, reverse]) -> list
//		register_function("sort", 1, 4, ..., TYPE_LIST, TYPE_LIST, TYPE_INT, TYPE_INT)
//
//	  - keys (LIST):    parallel list to sort BY; an empty list means "sort by the
//	    list itself". When non-empty it must have the same length as list, else
//	    E_INVARG, and the returned elements come from list (not keys).
//	  - natural (INT):  when true, strings compare with natural order (strnatcasecmp).
//	  - reverse (INT):  when true, the sorted order is reversed.
//
// Errors: bad arity -> E_ARGS; wrong arg types -> E_TYPE (Toast enforces these via
// register_function's type tokens); the sort-key list must be homogeneous and made
// of scalar sortable values (INT/FLOAT/OBJ/ERR/STR) or E_TYPE; an empty list/empty
// keys yields {}. String comparison is case-insensitive (strcasecmp), matching Toast.
func builtinSort(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	// arg 2: keys. Empty keys list == identity (sort by the list itself).
	var keys types.Value
	useKeys := false
	if len(args) >= 2 {
		if args[1].Type() != types.TYPE_LIST {
			return types.Err(types.E_TYPE)
		}
		keys = args[1]
		useKeys = keys.Len() > 0
	}

	// arg 3: natural flag (INT). arg 4: reverse flag (INT). is_true == nonzero.
	natural := false
	if len(args) >= 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		natural = args[2].Int() != 0
	}
	reverse := false
	if len(args) >= 4 {
		if args[3].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		reverse = args[3].Int() != 0
	}

	// The list we actually compare on: the keys list if provided, else list.
	sortList := list
	if useKeys {
		sortList = keys
	}

	n := sortList.Len()
	if n == 0 {
		return types.Ok(types.NewList([]types.Value{}))
	}
	if useKeys && list.Len() != keys.Len() {
		return types.Err(types.E_INVARG)
	}

	// All sort-key elements must share the first element's type and be a scalar
	// sortable value. LIST/MAP/ANON/WAIF (and any type mismatch) -> E_TYPE.
	keyType := sortList.Get(1).Type()
	for i := 1; i <= n; i++ {
		t := sortList.Get(i).Type()
		if t != keyType || t == types.TYPE_LIST || t == types.TYPE_MAP ||
			t == types.TYPE_ANON || t == types.TYPE_WAIF {
			return types.Err(types.E_TYPE)
		}
	}

	// Sort indices (1-based) so a keys-driven sort can map back into list.
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i + 1
	}
	sort.SliceStable(idx, func(i, j int) bool {
		return sortLess(sortList.Get(idx[i]), sortList.Get(idx[j]), natural)
	})
	if reverse {
		for i, j := 0, len(idx)-1; i < j; i, j = i+1, j-1 {
			idx[i], idx[j] = idx[j], idx[i]
		}
	}

	result := make([]types.Value, n)
	for p, it := range idx {
		result[p] = list.Get(it)
	}
	return types.Ok(types.NewList(result))
}

// sortLess implements Toast's VarCompare (list.cc:980-1006): numeric ordering for
// INT/FLOAT/OBJ, error-code ordering for ERR, and case-insensitive (optionally
// natural) ordering for STR. Both operands are guaranteed the same scalar type.
func sortLess(a, b types.Value, natural bool) bool {
	switch a.Type() {
	case types.TYPE_INT:
		return a.Int() < b.Int()
	case types.TYPE_FLOAT:
		return a.Float() < b.Float()
	case types.TYPE_OBJ, types.TYPE_ANON:
		return a.ID() < b.ID()
	case types.TYPE_ERR:
		return a.Code() < b.Code()
	case types.TYPE_STR:
		bs := b.Str()
		if natural {
			return strnatcasecmp(a.Str(), bs) < 0
		}
		return strcasecmp(a.Str(), bs) < 0
	default:
		// Toast's VarCompare logs and returns 0 for unknown types; treat as equal.
		return false
	}
}

// builtinReverse reverses a list or string
// reverse(list) -> list
// reverse(str) -> str
func builtinReverse(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	switch args[0].Type() {
	case types.TYPE_LIST:
		v := args[0]
		// Copy and reverse list elements.
		elements := make([]types.Value, v.Len())
		for i := 1; i <= v.Len(); i++ {
			elements[v.Len()-i] = v.Get(i)
		}
		return types.Ok(types.NewList(elements))
	case types.TYPE_STR:
		runes := []rune(args[0].Str())
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return types.Ok(types.NewStr(string(runes)))
	default:
		return types.Err(types.E_INVARG)
	}
}

// builtinUnique removes duplicate elements
// unique(list) -> list
func builtinUnique(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	// Dedup using MOO value equality (.Equal), the same equality setadd uses
	// (Toast list.cc setadd -> ismember(.., case_matters=0), case-insensitive for
	// strings). unique is a set-dedup operation, so it must agree with setadd: an
	// element setadd treats as a duplicate must also be collapsed here. A prior
	// elem.String() map key was case-SENSITIVE and disagreed with setadd.
	var unique []types.Value
	for i := 1; i <= list.Len(); i++ {
		elem := list.Get(i)
		dup := false
		for _, seen := range unique {
			if seen.Equal(elem) {
				dup = true
				break
			}
		}
		if !dup {
			unique = append(unique, elem)
		}
	}

	return types.Ok(types.NewList(unique))
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// strcasecmp is a byte-wise, ASCII case-insensitive string compare matching the
// libc strcasecmp() that Toast's VarCompare uses for default string sorting.
func strcasecmp(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		ca, cb := asciiLower(a[i]), asciiLower(b[i])
		if ca != cb {
			if ca < cb {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

// strnatcasecmp is a faithful port of strnatcasecmp() from Toast's
// dependencies/strnatcmp.c (strnatcmp0 with fold_case=1): natural-order, ASCII
// case-insensitive comparison used when sort()'s natural flag is set on strings.
func strnatcasecmp(a, b string) int {
	ai, bi := 0, 0
	for {
		ca, cb := byteAt(a, ai), byteAt(b, bi)

		// skip over leading spaces
		for isSpaceByte(ca) {
			ai++
			ca = byteAt(a, ai)
		}
		for isSpaceByte(cb) {
			bi++
			cb = byteAt(b, bi)
		}

		// process a run of digits
		if isDigitByte(ca) && isDigitByte(cb) {
			fractional := ca == '0' || cb == '0'
			var result int
			if fractional {
				result = natCompareLeft(a, ai, b, bi)
			} else {
				result = natCompareRight(a, ai, b, bi)
			}
			if result != 0 {
				return result
			}
		}

		if ca == 0 && cb == 0 {
			return 0
		}

		ca, cb = asciiUpper(ca), asciiUpper(cb)
		if ca < cb {
			return -1
		}
		if ca > cb {
			return 1
		}
		ai++
		bi++
	}
}

// natCompareRight mirrors compare_right() in strnatcmp.c: the longest run of
// digits wins, ties broken by the greatest value (remembered in bias).
func natCompareRight(a string, ai int, b string, bi int) int {
	bias := 0
	for {
		ca, cb := byteAt(a, ai), byteAt(b, bi)
		switch {
		case !isDigitByte(ca) && !isDigitByte(cb):
			return bias
		case !isDigitByte(ca):
			return -1
		case !isDigitByte(cb):
			return 1
		case ca < cb:
			if bias == 0 {
				bias = -1
			}
		case ca > cb:
			if bias == 0 {
				bias = 1
			}
		}
		ai++
		bi++
	}
}

// natCompareLeft mirrors compare_left() in strnatcmp.c: the first differing
// digit wins (used for fractional / zero-leading runs).
func natCompareLeft(a string, ai int, b string, bi int) int {
	for {
		ca, cb := byteAt(a, ai), byteAt(b, bi)
		switch {
		case !isDigitByte(ca) && !isDigitByte(cb):
			return 0
		case !isDigitByte(ca):
			return -1
		case !isDigitByte(cb):
			return 1
		case ca < cb:
			return -1
		case ca > cb:
			return 1
		}
		ai++
		bi++
	}
}

// byteAt returns s[i], or 0 when out of range, emulating C NUL termination.
func byteAt(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return 0
}

func asciiLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func asciiUpper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

func isSpaceByte(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	default:
		return false
	}
}

// builtinSlice: slice(list [, index] [, default_value]) → LIST
// Extracts elements from each item in a list of lists, strings, or maps.
func builtinSlice(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	// First arg must be a list
	if args[0].Type() != types.TYPE_LIST {
		fmt.Printf("[SLICE DEBUG] First arg not a list: %T = %v\n", args[0], args[0])
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	// Default index is 1
	var index types.Value = types.NewInt(1)
	if len(args) >= 2 {
		index = args[1]
	}

	// Optional default value (only for map key lookups)
	var defaultValue types.Value
	hasDefault := len(args) >= 3
	if hasDefault {
		defaultValue = args[2]
	}

	result := make([]types.Value, 0, list.Len())

	switch index.Type() {
	case types.TYPE_INT:
		// Single integer index
		i := int(index.Int())
		if i < 1 {
			return types.Err(types.E_RANGE)
		}

		for j := 1; j <= list.Len(); j++ {
			elem := list.Get(j)
			switch elem.Type() {
			case types.TYPE_LIST:
				if i > elem.Len() {
					return types.Err(types.E_RANGE)
				}
				result = append(result, elem.Get(i))
			case types.TYPE_STR:
				runes := []rune(elem.Str())
				if i > len(runes) {
					return types.Err(types.E_RANGE)
				}
				result = append(result, types.NewStr(string(runes[i-1])))
			default:
				fmt.Printf("[SLICE DEBUG] E_INVARG: element not list/str: %T = %v\n", elem, elem)
				return types.Err(types.E_INVARG)
			}
		}

	case types.TYPE_LIST:
		// List of indices
		if index.Len() == 0 {
			return types.Err(types.E_RANGE)
		}

		// Validate all indices are positive integers
		indices := make([]int, index.Len())
		for k := 1; k <= index.Len(); k++ {
			idxVal := index.Get(k)
			if idxVal.Type() != types.TYPE_INT {
				return types.Err(types.E_INVARG)
			}
			if idxVal.Int() < 1 {
				return types.Err(types.E_RANGE)
			}
			indices[k-1] = int(idxVal.Int())
		}

		for j := 1; j <= list.Len(); j++ {
			elem := list.Get(j)
			subResult := make([]types.Value, 0, len(indices))

			switch elem.Type() {
			case types.TYPE_LIST:
				for _, i := range indices {
					if i > elem.Len() {
						return types.Err(types.E_RANGE)
					}
					subResult = append(subResult, elem.Get(i))
				}
			case types.TYPE_STR:
				runes := []rune(elem.Str())
				for _, i := range indices {
					if i > len(runes) {
						return types.Err(types.E_RANGE)
					}
					subResult = append(subResult, types.NewStr(string(runes[i-1])))
				}
			default:
				return types.Err(types.E_INVARG)
			}

			result = append(result, types.NewList(subResult))
		}

	case types.TYPE_STR:
		// String key for map lookups
		key := index.Str()

		for j := 1; j <= list.Len(); j++ {
			elem := list.Get(j)
			if elem.Type() != types.TYPE_MAP {
				return types.Err(types.E_INVARG)
			}

			val, found := elem.MapGet(types.NewStr(key))
			if found {
				result = append(result, val)
			} else if hasDefault {
				result = append(result, defaultValue)
			}
			// If not found and no default, skip (don't append anything)
		}

	default:
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewList(result))
}
