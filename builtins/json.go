package builtins

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/MongooseMoo/barn/types"
)

// builtinGenerateJson converts MOO value to JSON string
// Signature: generate_json(value [, mode [, disable_binary_escapes]]) → STR
func builtinGenerateJson(ctx *Execution, args []types.Value) types.Result {
	value := args[0]
	embeddedTypes := false

	// Parse options if provided
	if len(args) > 1 {
		optsVal := args[1]
		ok := optsVal.Type() == types.TYPE_STR
		if !ok {
			return types.Err(types.E_TYPE)
		}
		opts := optsVal.Str()
		switch {
		case strings.EqualFold(opts, "common-subset"):
		case strings.EqualFold(opts, "embedded-types"):
			embeddedTypes = true
		default:
			return types.Err(types.E_INVARG)
		}
	}
	disableBinaryEscapes := len(args) == 3 && args[2].Truthy()

	// Convert MOO value to Go value suitable for JSON marshaling
	jsonValue, err := mooToJSON(value, embeddedTypes, false, disableBinaryEscapes)
	if err != types.E_NONE {
		return types.Err(err)
	}

	// Marshal to JSON
	data, jsonErr := json.Marshal(jsonValue)

	if jsonErr != nil {
		return types.Err(types.E_INVARG)
	}

	// Convert lowercase \uxxxx escapes to uppercase \uXXXX for MOO compatibility
	// Also convert \t to \u0009 to match MOO behavior
	result := normalizeJSONEscapes(string(data))
	return types.Ok(types.NewStr(result))
}

// mooToJSON converts a MOO value to a Go value suitable for JSON marshaling
// embeddedTypes: when true, add type suffixes (|obj, |err, |int, |float)
// isKey: when true, this value is being used as a map key
func mooToJSON(v types.Value, embeddedTypes bool, isKey bool, disableBinaryEscapes bool) (interface{}, types.ErrorCode) {
	switch v.Type() {
	case types.TYPE_INT:
		if embeddedTypes && isKey {
			return fmt.Sprintf("%d|int", v.Int()), types.E_NONE
		}
		return v.Int(), types.E_NONE

	case types.TYPE_FLOAT:
		f := v.Float()
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

	case types.TYPE_STR:
		// Convert MOO binary escapes (~XX) to actual bytes
		// JSON marshaler will then produce proper \n, \r, \t, \uXXXX escapes
		s := v.Str()
		result := decodeBinaryEscapes(s, disableBinaryEscapes)
		return result, types.E_NONE

	case types.TYPE_BOOL:
		return v.Bool(), types.E_NONE

	case types.TYPE_OBJ, types.TYPE_ANON:
		// Anonymous objects cannot be serialized to JSON
		if v.IsAnonymous() {
			return nil, types.E_INVARG
		}
		if embeddedTypes {
			return fmt.Sprintf("#%d|obj", v.ID()), types.E_NONE
		}
		return fmt.Sprintf("#%d", v.ID()), types.E_NONE

	case types.TYPE_ERR:
		if embeddedTypes {
			return v.String() + "|err", types.E_NONE
		}
		return v.String(), types.E_NONE

	case types.TYPE_LIST:
		arr := make([]interface{}, v.Len())
		for i := 1; i <= v.Len(); i++ {
			elem := v.Get(i)
			jsonElem, err := mooToJSON(elem, embeddedTypes, false, disableBinaryEscapes)
			if err != types.E_NONE {
				return nil, err
			}
			arr[i-1] = jsonElem
		}
		return arr, types.E_NONE

	case types.TYPE_MAP:
		// Tree order — Toast's generate_json iterates the rbtree (mapforeach)
		// with no re-sort, and Pairs() is that traversal.
		sortedPairs := v.Pairs()

		om := &orderedMap{entries: make([]orderedMapEntry, len(sortedPairs))}
		for i, pair := range sortedPairs {
			key := pair[0]
			value := pair[1]

			// Anonymous objects as map keys are invalid for JSON encoding.
			if isObjectRef(key) && key.IsAnonymous() {
				return nil, types.E_INVARG
			}

			// Convert key to string
			var keyStr string
			if embeddedTypes {
				// In embedded mode, keys get type annotations
				keyVal, err := mooToJSON(key, true, true, disableBinaryEscapes)
				if err != types.E_NONE {
					return nil, err
				}
				keyStr = fmt.Sprintf("%v", keyVal)
			} else {
				// Default mode - use raw value for strings, String() for others
				if key.Type() == types.TYPE_STR {
					keyStr = decodeBinaryEscapes(key.Str(), disableBinaryEscapes)
				} else {
					keyStr = key.String()
				}
			}

			// Convert value
			jsonValue, err := mooToJSON(value, embeddedTypes, false, disableBinaryEscapes)
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
func builtinParseJson(ctx *Execution, args []types.Value) types.Result {
	strVal := args[0]
	ok := strVal.Type() == types.TYPE_STR
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Parse optional mode argument
	embeddedTypes := false
	if len(args) == 2 {
		modeVal := args[1]
		if modeVal.Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		mode := modeVal.Str()
		embeddedTypes = strings.Contains(mode, "embedded")
	}

	jsonStr := strVal.Str()
	// Toast's yajl decodes escapes differently from Go: \b, \f, \n, \r and
	// every \uXXXX below U+0080 become MOO binary escape text ("~0A", "~41"),
	// \t becomes a tab, and higher \uXXXX escapes become UTF-8 bytes. Tag the
	// escapes that become binary escape text while the JSON escape boundaries
	// are still visible. The tag is selected after a preliminary decode so
	// legitimate JSON text can never collide with it.
	var preliminary interface{}
	if err := json.NewDecoder(strings.NewReader(jsonStr)).Decode(&preliminary); err != nil {
		return types.Err(types.E_INVARG)
	}
	controlTag := unusedJSONControlTag(preliminary)
	jsonStr = tagJSONBinaryEscapes(jsonStr, controlTag)

	// Use json.Decoder to parse just one JSON value, ignoring trailing chars
	// This matches ToastStunt behavior where parse_json("12abc") returns 12
	decoder := json.NewDecoder(strings.NewReader(jsonStr))
	decoder.UseNumber()
	data, err := decodeJSONValue(decoder)
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	value, ok := jsonToMOO(data, embeddedTypes)
	if !ok {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(value)
}

// errDuplicateJSONKey reports two members of one JSON object with the same
// name. Toast fails the whole parse rather than keeping either value.
var errDuplicateJSONKey = errors.New("duplicate JSON object key")

func decodeJSONValue(decoder *json.Decoder) (interface{}, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return token, nil
	}
	switch delim {
	case '[':
		values := make([]interface{}, 0)
		for decoder.More() {
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		_, err = decoder.Token()
		return values, err
	case '{':
		values := make(map[string]interface{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("JSON object key is not a string")
			}
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			if _, exists := values[key]; exists {
				return nil, errDuplicateJSONKey
			}
			values[key] = value
		}
		_, err = decoder.Token()
		return values, err
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}

// jsonToMOO converts a Go value from JSON unmarshaling to a MOO value.
// embeddedTypes: when true, parse type-annotated strings (|int, |float, |str, |obj, |err).
// It reports false when an object has two member names that become the same
// MOO key (they differ only in case, or decode to equal embedded values); Toast
// rejects such a document.
func jsonToMOO(v interface{}, embeddedTypes bool) (types.Value, bool) {
	switch val := v.(type) {
	case nil:
		// MOO has no null; ToastStunt maps JSON null to the error value E_NONE.
		return types.NewErr(types.E_NONE), true

	case bool:
		return types.NewBool(val), true

	case json.Number:
		text := val.String()
		if !strings.ContainsAny(text, ".eE") {
			if integer, err := val.Int64(); err == nil {
				return types.NewInt(integer), true
			}
		}
		floating, err := val.Float64()
		if err != nil {
			return types.NewInt(0), true
		}
		return types.NewFloat(floating), true

	case float64:
		if val == float64(int64(val)) && val >= float64(math.MinInt64) && val <= float64(math.MaxInt64) {
			return types.NewInt(int64(val)), true
		}
		return types.NewFloat(val), true

	case string:
		return jsonStringToMOO(val, embeddedTypes), true

	case []interface{}:
		// JSON array becomes MOO list
		elements := make([]types.Value, len(val))
		for i, item := range val {
			element, ok := jsonToMOO(item, embeddedTypes)
			if !ok {
				return types.None, false
			}
			elements[i] = element
		}
		return types.NewList(elements), true

	case map[string]interface{}:
		// JSON object becomes MOO map; keys take the same conversion as strings.
		pairs := make([][2]types.Value, 0, len(val))
		for k, item := range val {
			value, ok := jsonToMOO(item, embeddedTypes)
			if !ok {
				return types.None, false
			}
			pairs = append(pairs, [2]types.Value{jsonStringToMOO(k, embeddedTypes), value})
		}
		// NewMap replaces a repeated key, so a short map means two names collided.
		m := types.NewMap(pairs)
		if m.Len() != len(pairs) {
			return types.None, false
		}
		return m, true

	default:
		// Unknown type - return 0
		return types.NewInt(0), true
	}
}

// jsonStringToMOO converts a decoded JSON string or object member name.
func jsonStringToMOO(s string, embeddedTypes bool) types.Value {
	s = encodeParsedJSONString(s)
	if embeddedTypes {
		if parsed, ok := parseEmbeddedType(s); ok {
			return parsed
		}
	}
	return types.NewStr(s)
}

const jsonControlTagBase = "\uE000barn-json-control-"

var jsonShortControls = map[byte]string{'b': "~08", 'f': "~0C", 'n': "~0A", 'r': "~0D"}

// unusedJSONControlTag returns a prefix absent from every decoded string and
// object key. This makes the tagging pass safe even for documents containing
// private-use characters or text resembling an earlier tag.
func unusedJSONControlTag(v interface{}) string {
	for n := 0; ; n++ {
		tag := fmt.Sprintf("%s%d-", jsonControlTagBase, n)
		if !jsonValueContains(v, tag) {
			return tag
		}
	}
}

func jsonValueContains(v interface{}, needle string) bool {
	switch value := v.(type) {
	case string:
		return strings.Contains(value, needle)
	case []interface{}:
		for _, item := range value {
			if jsonValueContains(item, needle) {
				return true
			}
		}
	case map[string]interface{}:
		for key, item := range value {
			if strings.Contains(key, needle) || jsonValueContains(item, needle) {
				return true
			}
		}
	}
	return false
}

// tagJSONBinaryEscapes replaces each JSON string escape that Toast decodes to
// MOO binary escape text with the marker tag, the escape letter, and that
// three-byte text, which encodeParsedJSONString later turns back into the text.
func tagJSONBinaryEscapes(s, tag string) string {
	var result strings.Builder
	inString := false
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			inString = !inString
			result.WriteByte(s[i])
			continue
		}
		if inString && s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			if replacement, width, ok := jsonBinaryEscape(s[i+1:]); ok {
				result.WriteString(tag)
				result.WriteByte(next)
				result.WriteString(replacement)
				i += width
				continue
			}
			result.WriteByte(s[i])
			i++
			result.WriteByte(s[i])
			continue
		}
		result.WriteByte(s[i])
	}
	return result.String()
}

// jsonBinaryEscape returns the MOO binary escape text Toast's yajl decodes
// the escape after a backslash to, and the escape's width after the
// backslash: the short controls \b \f \n \r, and \uXXXX below U+0080.
func jsonBinaryEscape(escape string) (string, int, bool) {
	if replacement, ok := jsonShortControls[escape[0]]; ok {
		return replacement, 1, true
	}
	if escape[0] != 'u' || len(escape) < 5 {
		return "", 0, false
	}
	codepoint := 0
	for _, c := range []byte(escape[1:5]) {
		digit, ok := hexDigit(c)
		if !ok {
			return "", 0, false
		}
		codepoint = codepoint<<4 | digit
	}
	if codepoint >= 0x80 {
		return "", 0, false
	}
	return fmt.Sprintf("~%02X", codepoint), 5, true
}

// jsonTaggedReplacement validates the binary escape text that follows a
// marker's escape letter and returns it.
func jsonTaggedReplacement(control byte, text string) (string, bool) {
	if len(text) < 3 {
		return "", false
	}
	replacement := text[:3]
	if control == 'u' {
		_, hi := hexDigit(replacement[1])
		_, lo := hexDigit(replacement[2])
		return replacement, replacement[0] == '~' && hi && lo
	}
	want, ok := jsonShortControls[control]
	return want, ok && replacement == want
}

func encodeParsedJSONString(s string) string {
	var result strings.Builder
	for len(s) > 0 {
		start := strings.Index(s, jsonControlTagBase)
		if start < 0 {
			result.WriteString(encodeJSONDecodedBytes(s))
			break
		}
		result.WriteString(encodeJSONDecodedBytes(s[:start]))
		s = s[start:]
		suffix := s[len(jsonControlTagBase):]
		dash := strings.IndexByte(suffix, '-')
		if dash < 1 || dash+1 >= len(suffix) {
			result.WriteString(jsonControlTagBase)
			s = suffix
			continue
		}
		replacement, ok := jsonTaggedReplacement(suffix[dash+1], suffix[dash+2:])
		if !ok {
			result.WriteString(jsonControlTagBase)
			s = suffix
			continue
		}
		result.WriteString(replacement)
		s = s[len(jsonControlTagBase)+dash+2+len(replacement):]
	}
	return result.String()
}

// encodeJSONDecodedBytes writes decoded JSON text as a MOO string, spelling
// control and non-ASCII bytes as binary escapes. A tilde stays a tilde, as in
// Toast.
func encodeJSONDecodedBytes(s string) string {
	var result strings.Builder
	for _, b := range []byte(s) {
		switch {
		case b == '\t':
			result.WriteByte(b)
		case b < 32 || b > 126:
			fmt.Fprintf(&result, "~%02X", b)
		default:
			result.WriteByte(b)
		}
	}
	return result.String()
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
		// Toast: strtol() of the text after an optional '#'.
		return types.NewObj(types.ObjID(cStrtol(strings.TrimPrefix(s[:len(s)-4], "#")))), true
	} else if strings.HasSuffix(s, "|err") {
		errStr := s[:len(s)-4]
		if errStr == "" {
			return types.NewErr(types.E_NONE), true
		}
		if errCode, ok := types.ErrorFromString(errStr); ok {
			return types.NewErr(errCode), true
		}
	}
	return types.None, false
}

// normalizeJSONEscapes converts JSON escapes to match MOO behavior:
// - \uxxxx -> \uXXXX (uppercase hex)
// - \t -> \u0009 (tab as unicode escape)
func normalizeJSONEscapes(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '\\' {
			next := s[i+1]
			if next == 'u' && i+5 < len(s) {
				// Found \u, check for 4 hex digits and uppercase them
				hex := s[i+2 : i+6]
				result.WriteString("\\u")
				result.WriteString(strings.ToUpper(hex))
				i += 6
			} else if next == 't' {
				// Convert \t to \u0009
				result.WriteString("\\u0009")
				i += 2
			} else {
				// Other escape sequences pass through unchanged
				result.WriteByte(s[i])
				i++
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

// decodeBinaryEscapes converts MOO binary escapes (~XX) to actual bytes
// Only decodes control characters (0x00-0x1F) so JSON can escape them as \uXXXX
// Other escapes (~20-~7F, ~80-~FF) stay as literal text
// A backslash before a tilde is removed in either mode, leaving a literal tilde.
func decodeBinaryEscapes(s string, disabled bool) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '\\' && s[i+1] == '~' {
			result.WriteByte('~')
			i += 2
			continue
		}
		if !disabled && i+2 < len(s) && s[i] == '~' {
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

// cStrtol is C's strtol(s, &end, 10) on a 64-bit long: optional leading
// whitespace and sign, then the longest run of decimal digits, 0 when there
// are none, clamped to the int64 range.
func cStrtol(s string) int64 {
	s = strings.TrimLeft(s, " \t\n\v\f\r")
	negative := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		negative = s[0] == '-'
		s = s[1:]
	}
	var magnitude uint64
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		if magnitude > (math.MaxUint64-9)/10 {
			magnitude = math.MaxUint64
			break
		}
		magnitude = magnitude*10 + uint64(s[i]-'0')
	}
	if negative {
		if magnitude > uint64(math.MaxInt64)+1 {
			return math.MinInt64
		}
		return -int64(magnitude-1) - 1
	}
	if magnitude > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(magnitude)
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

func jsonDescriptors() []Descriptor {
	return []Descriptor{
		function("generate_json", builtinGenerateJson, 1, 3, -1, 2, -1),
		function("parse_json", builtinParseJson, 1, 2, 2, 2),
	}
}
