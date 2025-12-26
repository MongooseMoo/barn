package builtins

import (
	"barn/types"
	"encoding/json"
	"fmt"
	"math"
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

	// Parse options if provided
	if len(args) > 1 {
		optsVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		opts := optsVal.Value()
		pretty = strings.Contains(opts, "pretty")
	}

	// Convert MOO value to Go value suitable for JSON marshaling
	jsonValue, err := mooToJSON(value)
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

	return types.Ok(types.NewStr(string(data)))
}

// mooToJSON converts a MOO value to a Go value suitable for JSON marshaling
func mooToJSON(v types.Value) (interface{}, types.ErrorCode) {
	switch val := v.(type) {
	case types.IntValue:
		return val.Val, types.E_NONE

	case types.FloatValue:
		f := val.Val
		// Check for NaN and Infinity
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, types.E_FLOAT
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
		return fmt.Sprintf("#%d", val.ID()), types.E_NONE

	case types.ErrValue:
		return val.String(), types.E_NONE

	case types.ListValue:
		arr := make([]interface{}, val.Len())
		for i := 1; i <= val.Len(); i++ {
			elem := val.Get(i)
			jsonElem, err := mooToJSON(elem)
			if err != types.E_NONE {
				return nil, err
			}
			arr[i-1] = jsonElem
		}
		return arr, types.E_NONE

	case types.MapValue:
		obj := make(map[string]interface{})
		pairs := val.Pairs()
		for _, pair := range pairs {
			key := pair[0]
			value := pair[1]

			// Convert key to string - use raw value for strings, String() for others
			var keyStr string
			if strKey, ok := key.(types.StrValue); ok {
				keyStr = strKey.Value() // Raw string without quotes
			} else {
				keyStr = key.String() // For numbers, objects, etc.
			}

			// Convert value
			jsonValue, err := mooToJSON(value)
			if err != types.E_NONE {
				return nil, err
			}
			obj[keyStr] = jsonValue
		}
		return obj, types.E_NONE

	default:
		// Unsupported types (WAIF, ANON)
		return nil, types.E_TYPE
	}
}

// builtinParseJson parses JSON string to MOO value
// Signature: parse_json(string [, mode]) → VALUE
// Modes: "common-subset", "embedded", or default (no mode)
func builtinParseJson(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	strVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Parse optional mode argument (currently ignored - same behavior for all modes)
	if len(args) == 2 {
		_, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
	}

	jsonStr := strVal.Value()

	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(jsonToMOO(data))
}

// jsonToMOO converts a Go value from JSON unmarshaling to a MOO value
func jsonToMOO(v interface{}) types.Value {
	switch val := v.(type) {
	case nil:
		// JSON null becomes MOO integer 0
		return types.NewInt(0)

	case bool:
		return types.NewBool(val)

	case float64:
		// JSON numbers are always float64
		// Check if it's really an integer
		if val == float64(int64(val)) && val >= float64(math.MinInt64) && val <= float64(math.MaxInt64) {
			return types.NewInt(int64(val))
		}
		return types.NewFloat(val)

	case string:
		return types.NewStr(val)

	case []interface{}:
		// JSON array becomes MOO list
		elements := make([]types.Value, len(val))
		for i, item := range val {
			elements[i] = jsonToMOO(item)
		}
		return types.NewList(elements)

	case map[string]interface{}:
		// JSON object becomes MOO map
		pairs := make([][2]types.Value, 0, len(val))
		for k, v := range val {
			pairs = append(pairs, [2]types.Value{
				types.NewStr(k),
				jsonToMOO(v),
			})
		}
		return types.NewMap(pairs)

	default:
		// Unknown type - return 0
		return types.NewInt(0)
	}
}

// decodeBinaryEscapes converts MOO binary escapes (~XX) to actual bytes
// This allows JSON marshaler to produce proper escape sequences
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
				result.WriteByte(b)
				i += 3
				continue
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
