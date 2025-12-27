package builtins

import (
	"barn/types"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// builtinGenerateJson converts MOO value to JSON string
// Signature: generate_json(value [, options]) → STR
func builtinGenerateJson(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	value := args[0]
	pretty := false
	embeddedTypes := false

	// Parse options if provided
	if len(args) > 1 {
		optsVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		opts := optsVal.Value()
		// Validate mode string - must be one of the valid modes or empty
		if opts != "" && opts != "common-subset" && opts != "embedded-types" &&
			!strings.HasPrefix(opts, "pretty") && !strings.Contains(opts, "embedded") {
			return types.Err(types.E_INVARG)
		}
		pretty = strings.Contains(opts, "pretty")
		embeddedTypes = strings.Contains(opts, "embedded")
	}

	// Convert MOO value to Go value suitable for JSON marshaling
	jsonValue, err := mooToJSON(value, embeddedTypes, false)
	if err != types.E_NONE {
		return types.Err(err)
	}

	// Marshal to JSON
	var data []byte
	var jsonErr error
	if pretty {
		data, jsonErr = json.MarshalIndent(jsonValue, "", "  ")
	} else {
		data, jsonErr = json.Marshal(jsonValue)
	}

	if jsonErr != nil {
		return types.Err(types.E_INVARG)
	}

	// Convert lowercase \uxxxx escapes to uppercase \uXXXX for MOO compatibility
	result := uppercaseUnicodeEscapes(string(data))
	return types.Ok(types.NewStr(result))
}

// mooToJSON converts a MOO value to a Go value suitable for JSON marshaling
// embeddedTypes: when true, add type suffixes (|obj, |err, |int, |float)
// isKey: when true, this value is being used as a map key
func mooToJSON(v types.Value, embeddedTypes bool, isKey bool) (interface{}, types.ErrorCode) {
	switch val := v.(type) {
	case types.IntValue:
		if embeddedTypes && isKey {
			return fmt.Sprintf("%d|int", val.Val), types.E_NONE
		}
		return val.Val, types.E_NONE

	case types.FloatValue:
		f := val.Val
		// Check for NaN and Infinity
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, types.E_FLOAT
		}
		if embeddedTypes && isKey {
			// Format float with decimal point for key
			s := fmt.Sprintf("%g", f)
			if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") {
				s += ".0"
			}
			return s + "|float", types.E_NONE
		}
		// Format float with decimal point (MOO semantics)
		s := fmt.Sprintf("%g", f)
		// Ensure we have a decimal point for whole numbers
		if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") {
			s += ".0"
		}
		// Use json.Number to avoid re-formatting
		return json.Number(s), types.E_NONE

	case types.StrValue:
		// Convert MOO binary escapes (~XX) to actual bytes
		// JSON marshaler will then produce proper \n, \r, \t, \uXXXX escapes
		s := val.Value()
		result := decodeBinaryEscapes(s)
		return result, types.E_NONE

	case types.BoolValue:
		return val.Val, types.E_NONE

	case types.ObjValue:
		if embeddedTypes {
			return fmt.Sprintf("#%d|obj", val.ID()), types.E_NONE
		}
		return fmt.Sprintf("#%d", val.ID()), types.E_NONE

	case types.ErrValue:
		if embeddedTypes {
			return val.String() + "|err", types.E_NONE
		}
		return val.String(), types.E_NONE

	case types.ListValue:
		arr := make([]interface{}, val.Len())
		for i := 1; i <= val.Len(); i++ {
			elem := val.Get(i)
			jsonElem, err := mooToJSON(elem, embeddedTypes, false)
			if err != types.E_NONE {
				return nil, err
			}
			arr[i-1] = jsonElem
		}
		return arr, types.E_NONE

	case types.MapValue:
		// Use orderedMap to preserve MOO key ordering
		pairs := val.Pairs()
		// Sort pairs by MOO key order (int < float < obj < err < str)
		sortedPairs := make([][2]types.Value, len(pairs))
		copy(sortedPairs, pairs)
		sortMapPairsForJSON(sortedPairs)

		om := &orderedMap{entries: make([]orderedMapEntry, len(sortedPairs))}
		for i, pair := range sortedPairs {
			key := pair[0]
			value := pair[1]

			// Convert key to string
			var keyStr string
			if embeddedTypes {
				// In embedded mode, keys get type annotations
				keyVal, err := mooToJSON(key, true, true)
				if err != types.E_NONE {
					return nil, err
				}
				keyStr = fmt.Sprintf("%v", keyVal)
			} else {
				// Default mode - use raw value for strings, String() for others
				if strKey, ok := key.(types.StrValue); ok {
					keyStr = strKey.Value()
				} else {
					keyStr = key.String()
				}
			}

			// Convert value
			jsonValue, err := mooToJSON(value, embeddedTypes, false)
			if err != types.E_NONE {
				return nil, err
			}
			om.entries[i] = orderedMapEntry{key: keyStr, value: jsonValue}
		}
		return om, types.E_NONE

	default:
		// Unsupported types (WAIF, ANON)
		return nil, types.E_TYPE
	}
}

// builtinParseJson parses JSON string to MOO value
// Signature: parse_json(string [, mode]) → VALUE
// Modes: "common-subset", "embedded-types", or default (no mode)
func builtinParseJson(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	strVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Parse optional mode argument
	embeddedTypes := false
	if len(args) == 2 {
		modeVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		mode := modeVal.Value()
		embeddedTypes = strings.Contains(mode, "embedded")
	}

	jsonStr := strVal.Value()

	// Use json.Decoder to parse just one JSON value, ignoring trailing chars
	// This matches ToastStunt behavior where parse_json("12abc") returns 12
	var data interface{}
	decoder := json.NewDecoder(strings.NewReader(jsonStr))
	if err := decoder.Decode(&data); err != nil {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(jsonToMOO(data, embeddedTypes))
}

// jsonToMOO converts a Go value from JSON unmarshaling to a MOO value
// embeddedTypes: when true, parse type-annotated strings (|int, |float, |str, |obj, |err)
func jsonToMOO(v interface{}, embeddedTypes bool) types.Value {
	switch val := v.(type) {
	case nil:
		// JSON null becomes MOO integer 0
		return types.NewInt(0)

	case bool:
		return types.NewBool(val)

	case float64:
		// JSON numbers are always float64
		// Check if it's really an integer and fits in 32-bit range
		// MOO treats numbers larger than 32-bit signed int as floats
		if val == float64(int64(val)) && val >= float64(math.MinInt32) && val <= float64(math.MaxInt32) {
			return types.NewInt(int64(val))
		}
		return types.NewFloat(val)

	case string:
		if embeddedTypes {
			// Check for type annotations
			if parsed, ok := parseEmbeddedType(val); ok {
				return parsed
			}
		}
		// Convert non-printable and non-ASCII bytes to ~XX format
		return types.NewStr(encodeBinaryEscapes(val))

	case []interface{}:
		// JSON array becomes MOO list
		elements := make([]types.Value, len(val))
		for i, item := range val {
			elements[i] = jsonToMOO(item, embeddedTypes)
		}
		return types.NewList(elements)

	case map[string]interface{}:
		// JSON object becomes MOO map
		pairs := make([][2]types.Value, 0, len(val))
		for k, v := range val {
			// In embedded mode, keys may have type annotations too
			var keyVal types.Value
			if embeddedTypes {
				if parsed, ok := parseEmbeddedType(k); ok {
					keyVal = parsed
				} else {
					keyVal = types.NewStr(k)
				}
			} else {
				keyVal = types.NewStr(k)
			}
			pairs = append(pairs, [2]types.Value{
				keyVal,
				jsonToMOO(v, embeddedTypes),
			})
		}
		return types.NewMap(pairs)

	default:
		// Unknown type - return 0
		return types.NewInt(0)
	}
}

// parseEmbeddedType parses a type-annotated string like "123|int" or "#5|obj"
// Empty prefix is valid and returns the default value for that type
func parseEmbeddedType(s string) (types.Value, bool) {
	if strings.HasSuffix(s, "|int") {
		numStr := s[:len(s)-4]
		if numStr == "" {
			return types.NewInt(0), true
		}
		var n int64
		if _, err := fmt.Sscanf(numStr, "%d", &n); err == nil {
			return types.NewInt(n), true
		}
	} else if strings.HasSuffix(s, "|float") {
		numStr := s[:len(s)-6]
		if numStr == "" {
			return types.NewFloat(0.0), true
		}
		var f float64
		if _, err := fmt.Sscanf(numStr, "%f", &f); err == nil {
			return types.NewFloat(f), true
		}
	} else if strings.HasSuffix(s, "|str") {
		return types.NewStr(s[:len(s)-4]), true
	} else if strings.HasSuffix(s, "|obj") {
		objStr := s[:len(s)-4]
		if objStr == "" {
			return types.NewObj(0), true
		}
		if len(objStr) > 0 && objStr[0] == '#' {
			var id int64
			if _, err := fmt.Sscanf(objStr[1:], "%d", &id); err == nil {
				return types.NewObj(types.ObjID(id)), true
			}
		}
	} else if strings.HasSuffix(s, "|err") {
		errStr := s[:len(s)-4]
		if errStr == "" {
			return types.NewErr(types.E_NONE), true
		}
		if errCode, ok := types.ErrorFromString(errStr); ok {
			return types.NewErr(errCode), true
		}
	}
	return nil, false
}

// uppercaseUnicodeEscapes converts \uxxxx escapes to \uXXXX (uppercase)
func uppercaseUnicodeEscapes(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i+5 < len(s) && s[i] == '\\' && s[i+1] == 'u' {
			// Found \u, check for 4 hex digits and uppercase them
			hex := s[i+2 : i+6]
			result.WriteString("\\u")
			result.WriteString(strings.ToUpper(hex))
			i += 6
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

// encodeBinaryEscapes converts non-printable and non-ASCII bytes to ~XX format
// This is the inverse of decodeBinaryEscapes
func encodeBinaryEscapes(s string) string {
	var result strings.Builder
	for _, b := range []byte(s) {
		if b == '~' {
			result.WriteString("~7E")
		} else if b < 32 || b > 126 {
			// Non-printable or non-ASCII: encode as ~XX
			const hexDigits = "0123456789ABCDEF"
			result.WriteByte('~')
			result.WriteByte(hexDigits[b>>4])
			result.WriteByte(hexDigits[b&0xF])
		} else {
			result.WriteByte(b)
		}
	}
	return result.String()
}

// decodeBinaryEscapes converts MOO binary escapes (~XX) to actual bytes
// Only decodes control characters (0x00-0x1F) so JSON can escape them as \uXXXX
// Other escapes (~20-~7F, ~80-~FF) stay as literal text
func decodeBinaryEscapes(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i+2 < len(s) && s[i] == '~' {
			// Check for hex escape ~XX
			hex1, ok1 := hexDigit(s[i+1])
			hex2, ok2 := hexDigit(s[i+2])
			if ok1 && ok2 {
				b := byte(hex1<<4 | hex2)
				// Only decode control characters (0x00-0x1F)
				if b < 0x20 {
					result.WriteByte(b)
					i += 3
					continue
				}
				// Leave other escapes as literal ~XX
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

// hexDigit returns the value of a hex digit and whether it's valid
func hexDigit(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	default:
		return 0, false
	}
}

// orderedMap preserves key order when marshaled to JSON
type orderedMapEntry struct {
	key   string
	value interface{}
}

type orderedMap struct {
	entries []orderedMapEntry
}

// MarshalJSON implements json.Marshaler for orderedMap
func (om *orderedMap) MarshalJSON() ([]byte, error) {
	var buf strings.Builder
	buf.WriteByte('{')
	for i, entry := range om.entries {
		if i > 0 {
			buf.WriteByte(',')
		}
		// Marshal key
		keyJSON, err := json.Marshal(entry.key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		// Marshal value
		valJSON, err := json.Marshal(entry.value)
		if err != nil {
			return nil, err
		}
		buf.Write(valJSON)
	}
	buf.WriteByte('}')
	return []byte(buf.String()), nil
}

// sortMapPairsForJSON sorts map pairs by MOO key order for JSON output
// Order: INT < FLOAT < OBJ < ERR < STR
func sortMapPairsForJSON(pairs [][2]types.Value) {
	sort.Slice(pairs, func(i, j int) bool {
		return compareJSONKeys(pairs[i][0], pairs[j][0]) < 0
	})
}

// compareJSONKeys compares two MOO values for JSON key ordering
// Order: INT (0) < OBJ (1) < FLOAT (2) < ERR (3) < STR (4)
// This matches MOO/ToastStunt map key ordering
func compareJSONKeys(a, b types.Value) int {
	typeOrder := func(v types.Value) int {
		switch v.(type) {
		case types.IntValue:
			return 0
		case types.ObjValue:
			return 1
		case types.FloatValue:
			return 2
		case types.ErrValue:
			return 3
		case types.StrValue:
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
	case types.FloatValue:
		bv := b.(types.FloatValue)
		if av.Val < bv.Val {
			return -1
		} else if av.Val > bv.Val {
			return 1
		}
		return 0
	case types.ObjValue:
		bv := b.(types.ObjValue)
		if av.ID() < bv.ID() {
			return -1
		} else if av.ID() > bv.ID() {
			return 1
		}
		return 0
	case types.ErrValue:
		bv := b.(types.ErrValue)
		if av.Code() < bv.Code() {
			return -1
		} else if av.Code() > bv.Code() {
			return 1
		}
		return 0
	case types.StrValue:
		bv := b.(types.StrValue)
		// Case-insensitive comparison for strings
		return strings.Compare(strings.ToLower(av.Value()), strings.ToLower(bv.Value()))
	}
	return 0
}
