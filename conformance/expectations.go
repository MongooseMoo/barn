package conformance

import (
	"barn/types"
	"fmt"
)

// checkExpectation checks if the result matches the expected outcome
func (r *Runner) checkExpectation(test TestCase, result types.Result) (bool, error) {
	expect := test.Expect

	// Check for expected error
	if expect.Error != "" {
		// Convert error name to ErrorCode
		expectedErr, ok := errorNameToCode(expect.Error)
		if !ok {
			return false, fmt.Errorf("unknown error code: %s", expect.Error)
		}

		if result.Flow != types.FlowException {
			return false, fmt.Errorf("expected error %s, got value: %v", expect.Error, result.Val)
		}

		if result.Error != expectedErr {
			return false, fmt.Errorf("expected error %s, got %s", expect.Error, errorCodeToName(result.Error))
		}

		return true, nil
	}

	// Check for normal result
	if result.Flow == types.FlowException {
		return false, fmt.Errorf("unexpected error: %s", errorCodeToName(result.Error))
	}

	// Check expected value
	if expect.Value != nil {
		expectedVal, err := convertYAMLValue(expect.Value)
		if err != nil {
			return false, fmt.Errorf("failed to convert expected value: %w", err)
		}

		// Handle nil result value
		if result.Val == nil {
			return false, fmt.Errorf("expected %v, got nil", expectedVal)
		}

		if !valuesEquivalent(result.Val, expectedVal) {
			return false, fmt.Errorf("expected %v, got %v", expectedVal, result.Val)
		}

		return true, nil
	}

	// Check expected type
	if expect.Type != "" {
		expectedType, ok := typeNameToCode(expect.Type)
		if !ok {
			return false, fmt.Errorf("unknown type: %s", expect.Type)
		}

		if result.Val.Type() != expectedType {
			return false, fmt.Errorf("expected type %s, got %s", expect.Type, typeCodeToName(result.Val.Type()))
		}

		return true, nil
	}

	// No expectation specified
	return false, fmt.Errorf("no expectation specified")
}

// valuesEquivalent checks if two values are equivalent for test purposes
// This handles YAML ambiguity where:
// - -1 could mean either integer or object #-1
// - "#0" could mean either string or object reference
func valuesEquivalent(actual, expected types.Value) bool {
	// Direct equality
	if actual.Equal(expected) {
		return true
	}

	// Handle integer <-> object comparison for YAML ambiguity
	// If expected is int and actual is object with that ID, consider equal
	if expectedInt, ok := expected.(types.IntValue); ok {
		if actualObj, ok := actual.(types.ObjValue); ok {
			return int64(actualObj.ID()) == expectedInt.Val
		}
	}
	// If expected is object and actual is int with that ID, consider equal
	if expectedObj, ok := expected.(types.ObjValue); ok {
		if actualInt, ok := actual.(types.IntValue); ok {
			return actualInt.Val == int64(expectedObj.ID())
		}
	}

	// Handle integer <-> error comparison for YAML ambiguity
	// If expected is int and actual is error with that code, consider equal
	if expectedInt, ok := expected.(types.IntValue); ok {
		if actualErr, ok := actual.(types.ErrValue); ok {
			return int64(actualErr.Code()) == expectedInt.Val
		}
	}
	// If expected is error and actual is int with that code, consider equal
	if expectedErr, ok := expected.(types.ErrValue); ok {
		if actualInt, ok := actual.(types.IntValue); ok {
			return actualInt.Val == int64(expectedErr.Code())
		}
	}

	// Handle integer <-> object comparison for YAML ambiguity
	// If expected is int and actual is obj with that ID, consider equal
	if expectedInt, ok := expected.(types.IntValue); ok {
		if actualObj, ok := actual.(types.ObjValue); ok {
			return expectedInt.Val == int64(actualObj.ID())
		}
	}
	// If expected is obj and actual is int with that ID, consider equal
	if expectedObj, ok := expected.(types.ObjValue); ok {
		if actualInt, ok := actual.(types.IntValue); ok {
			return int64(expectedObj.ID()) == actualInt.Val
		}
	}

	// Handle string <-> error comparison for YAML ambiguity (e.g., "E_ARGS")
	// If expected is string "E_*" and actual is error, consider equal
	if expectedStr, ok := expected.(types.StrValue); ok {
		if actualErr, ok := actual.(types.ErrValue); ok {
			if errCode, found := errorNameToCode(expectedStr.Value()); found {
				return errCode == actualErr.Code()
			}
		}
	}
	// If expected is error and actual is string "E_*", consider equal
	if expectedErr, ok := expected.(types.ErrValue); ok {
		if actualStr, ok := actual.(types.StrValue); ok {
			if errCode, found := errorNameToCode(actualStr.Value()); found {
				return errCode == expectedErr.Code()
			}
		}
	}

	// Handle string <-> object comparison for YAML ambiguity
	// If expected is object and actual is string "#N", consider equal
	if expectedObj, ok := expected.(types.ObjValue); ok {
		if actualStr, ok := actual.(types.StrValue); ok {
			expectedStr := fmt.Sprintf("#%d", expectedObj.ID())
			return actualStr.Value() == expectedStr
		}
	}
	// If expected is string "#N" and actual is object, consider equal
	if expectedStr, ok := expected.(types.StrValue); ok {
		if actualObj, ok := actual.(types.ObjValue); ok {
			s := expectedStr.Value()
			if len(s) > 0 && s[0] == '#' {
				var id int64
				if _, err := fmt.Sscanf(s, "#%d", &id); err == nil {
					return int64(actualObj.ID()) == id
				}
			}
		}
	}

	// Recursively compare lists
	if expectedList, ok := expected.(types.ListValue); ok {
		if actualList, ok := actual.(types.ListValue); ok {
			if expectedList.Len() != actualList.Len() {
				return false
			}
			for i := 1; i <= expectedList.Len(); i++ {
				if !valuesEquivalent(actualList.Get(i), expectedList.Get(i)) {
					return false
				}
			}
			return true
		}
	}

	// Recursively compare maps (order-independent)
	if expectedMap, ok := expected.(types.MapValue); ok {
		if actualMap, ok := actual.(types.MapValue); ok {
			if expectedMap.Len() != actualMap.Len() {
				return false
			}
			// Check each expected key exists in actual with equivalent value
			for _, pair := range expectedMap.Pairs() {
				key := pair[0]
				expectedVal := pair[1]
				// Find this key in actual (using equivalence)
				found := false
				for _, actualPair := range actualMap.Pairs() {
					if valuesEquivalent(actualPair[0], key) {
						if valuesEquivalent(actualPair[1], expectedVal) {
							found = true
							break
						}
					}
				}
				if !found {
					return false
				}
			}
			return true
		}
	}

	return false
}

// convertYAMLValue converts a YAML value to a MOO Value
func convertYAMLValue(v interface{}) (types.Value, error) {
	switch val := v.(type) {
	case int:
		return types.NewInt(int64(val)), nil
	case int64:
		return types.NewInt(val), nil
	case float64:
		return types.NewFloat(val), nil
	case string:
		// Check if string represents an object reference like "#2" or "#-1"
		// Must be ONLY the object reference, no extra characters
		if len(val) > 0 && val[0] == '#' {
			var id int64
			var extra string
			n, err := fmt.Sscanf(val, "#%d%s", &id, &extra)
			// Must parse exactly the ID with no extra characters
			if err == nil && n == 1 {
				return types.NewObj(types.ObjID(id)), nil
			}
		}
		return types.NewStr(val), nil
	case bool:
		return types.NewBool(val), nil
	case []interface{}:
		elements := make([]types.Value, len(val))
		for i, elem := range val {
			v, err := convertYAMLValue(elem)
			if err != nil {
				return nil, err
			}
			elements[i] = v
		}
		return types.NewList(elements), nil
	case map[string]interface{}:
		// Convert string-keyed map to MOO map
		pairs := make([][2]types.Value, 0, len(val))
		for k, v := range val {
			keyVal := types.NewStr(k)
			valVal, err := convertYAMLValue(v)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, [2]types.Value{keyVal, valVal})
		}
		return types.NewMap(pairs), nil
	case map[interface{}]interface{}:
		// Handle YAML's default map type (interface{} keys)
		pairs := make([][2]types.Value, 0, len(val))
		for k, v := range val {
			keyVal, err := convertYAMLValue(k)
			if err != nil {
				return nil, err
			}
			valVal, err := convertYAMLValue(v)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, [2]types.Value{keyVal, valVal})
		}
		return types.NewMap(pairs), nil
	default:
		return nil, fmt.Errorf("unsupported YAML type: %T", v)
	}
}
