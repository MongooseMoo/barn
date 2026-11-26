package builtins

import (
	"bytes"
	"encoding/json/jsontext"
	"fmt"
	"math"
	"strconv"
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

	// HTML characters and U+2028/U+2029 are escaped, invalid UTF-8 becomes
	// U+FFFD, and a map may repeat a member name ([1 -> x, "1" -> y]).
	var buf bytes.Buffer
	enc := jsontext.NewEncoder(&buf,
		jsontext.EscapeForHTML(true),
		jsontext.EscapeForJS(true),
		jsontext.AllowInvalidUTF8(true),
		jsontext.AllowDuplicateNames(true))
	g := jsonGenerator{enc: enc, embeddedTypes: embeddedTypes, disableBinaryEscapes: disableBinaryEscapes}
	if code := g.write(value); code != types.E_NONE {
		return types.Err(code)
	}

	// The encoder ends each top-level value with a newline.
	data := bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})
	// Convert lowercase \uxxxx escapes to uppercase \uXXXX for MOO compatibility
	// Also convert \t to a \u escape to match MOO behavior
	return types.Ok(types.NewStr(normalizeJSONEscapes(string(data))))
}

// jsonGenerator writes MOO values as JSON tokens.
// embeddedTypes: when true, add type suffixes (|obj, |err, and on map keys |int, |float)
type jsonGenerator struct {
	enc                  *jsontext.Encoder
	embeddedTypes        bool
	disableBinaryEscapes bool
}

func (g *jsonGenerator) token(t jsontext.Token) types.ErrorCode {
	if err := g.enc.WriteToken(t); err != nil {
		return types.E_INVARG
	}
	return types.E_NONE
}

func (g *jsonGenerator) write(v types.Value) types.ErrorCode {
	switch v.Type() {
	case types.TYPE_INT:
		return g.token(jsontext.Int(v.Int()))

	case types.TYPE_FLOAT:
		s, code := formatJSONFloat(v.Float())
		if code != types.E_NONE {
			return code
		}
		if err := g.enc.WriteValue(jsontext.Value(s)); err != nil {
			return types.E_INVARG
		}
		return types.E_NONE

	case types.TYPE_STR:
		return g.token(jsontext.String(g.stringText(v.Str())))

	case types.TYPE_BOOL:
		return g.token(jsontext.Bool(v.Bool()))

	case types.TYPE_OBJ, types.TYPE_ANON, types.TYPE_ERR:
		s, code := g.scalarText(v)
		if code != types.E_NONE {
			return code
		}
		return g.token(jsontext.String(s))

	case types.TYPE_LIST:
		if code := g.token(jsontext.BeginArray); code != types.E_NONE {
			return code
		}
		for i := 1; i <= v.Len(); i++ {
			if code := g.write(v.Get(i)); code != types.E_NONE {
				return code
			}
		}
		return g.token(jsontext.EndArray)

	case types.TYPE_MAP:
		if code := g.token(jsontext.BeginObject); code != types.E_NONE {
			return code
		}
		// Tree order — Toast's generate_json iterates the rbtree (mapforeach)
		// with no re-sort, and Pairs() is that traversal.
		for _, pair := range v.Pairs() {
			key, code := g.memberName(pair[0])
			if code != types.E_NONE {
				return code
			}
			if code := g.token(jsontext.String(key)); code != types.E_NONE {
				return code
			}
			if code := g.write(pair[1]); code != types.E_NONE {
				return code
			}
		}
		return g.token(jsontext.EndObject)

	default:
		// Unsupported types (WAIF)
		return types.E_TYPE
	}
}

// scalarText is the JSON string text of an object reference or error value.
func (g *jsonGenerator) scalarText(v types.Value) (string, types.ErrorCode) {
	if v.Type() == types.TYPE_ERR {
		if g.embeddedTypes {
			return v.String() + "|err", types.E_NONE
		}
		return v.String(), types.E_NONE
	}
	// Anonymous objects cannot be serialized to JSON
	if v.IsAnonymous() {
		return "", types.E_INVARG
	}
	if g.embeddedTypes {
		return fmt.Sprintf("#%d|obj", v.ID()), types.E_NONE
	}
	return fmt.Sprintf("#%d", v.ID()), types.E_NONE
}

// memberName is the JSON object member name for a MOO map key.
func (g *jsonGenerator) memberName(key types.Value) (string, types.ErrorCode) {
	// Anonymous objects as map keys are invalid for JSON encoding.
	if isObjectRef(key) && key.IsAnonymous() {
		return "", types.E_INVARG
	}
	if key.Type() == types.TYPE_STR {
		return g.stringText(key.Str()), types.E_NONE
	}
	if !g.embeddedTypes {
		return key.String(), types.E_NONE
	}
	// In embedded mode, keys get type annotations
	switch key.Type() {
	case types.TYPE_INT:
		return fmt.Sprintf("%d|int", key.Int()), types.E_NONE
	case types.TYPE_FLOAT:
		s, code := formatJSONFloat(key.Float())
		return s + "|float", code
	case types.TYPE_BOOL:
		return strconv.FormatBool(key.Bool()) + "|bool", types.E_NONE
	case types.TYPE_OBJ, types.TYPE_ANON, types.TYPE_ERR:
		return g.scalarText(key)
	default:
		return "", types.E_TYPE
	}
}

func (g *jsonGenerator) stringText(s string) string {
	s = decodeBinaryEscapes(s, g.disableBinaryEscapes)
	if g.embeddedTypes {
		if _, tag := splitEmbeddedType(s); tag != "" {
			s += "|str"
		}
	}
	return s
}

// formatJSONFloat formats a float with a decimal point for whole numbers
// (MOO semantics). NaN and the infinities have no JSON form.
func formatJSONFloat(f float64) (string, types.ErrorCode) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", types.E_FLOAT
	}
	s := fmt.Sprintf("%g", f)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s, types.E_NONE
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
		// Toast accepts exactly the two mode names, in any case, and raises
		// E_INVARG for any other string (as generate_json does).
		mode := modeVal.Str()
		switch {
		case strings.EqualFold(mode, "common-subset"):
		case strings.EqualFold(mode, "embedded-types"):
			embeddedTypes = true
		default:
			return types.Err(types.E_INVARG)
		}
	}

	// Read just one JSON value and ignore what follows it, as Toast does:
	// parse_json("12abc") returns 12. Invalid UTF-8 becomes U+FFFD. The
	// decoder rejects an object that repeats a member name.
	decoder := jsontext.NewDecoder(strings.NewReader(strVal.Str()), jsontext.AllowInvalidUTF8(true))
	value, ok := decodeJSONToMOO(decoder, embeddedTypes)
	if !ok {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(value)
}

// decodeJSONToMOO reads the next JSON value as a MOO value.
// embeddedTypes: when true, parse type-annotated strings (|int, |float, |str, |obj, |err).
// It reports false on malformed JSON, on a number too large for a float, and
// when an object has two member names that become the same MOO key (an exact
// repeat, names that differ only in case, or equal embedded values); Toast
// rejects such a document.
func decodeJSONToMOO(decoder *jsontext.Decoder, embeddedTypes bool) (types.Value, bool) {
	token, err := decoder.ReadToken()
	if err != nil {
		return types.None, false
	}
	switch token.Kind() {
	case 'n':
		// MOO has no null; ToastStunt maps JSON null to the error value E_NONE.
		return types.NewErr(types.E_NONE), true

	case 't', 'f':
		return types.NewBool(token.Bool()), true

	case '0':
		return jsonNumberToMOO(token.String())

	case '"':
		return jsonStringToMOO(token.String(), embeddedTypes), true

	case '[':
		// JSON array becomes MOO list
		elements := make([]types.Value, 0)
		for decoder.PeekKind() != ']' {
			element, ok := decodeJSONToMOO(decoder, embeddedTypes)
			if !ok {
				return types.None, false
			}
			elements = append(elements, element)
		}
		if _, err := decoder.ReadToken(); err != nil {
			return types.None, false
		}
		return types.NewList(elements), true

	case '{':
		// JSON object becomes MOO map; names take the same conversion as strings.
		pairs := make([][2]types.Value, 0)
		for decoder.PeekKind() != '}' {
			name, err := decoder.ReadToken()
			if err != nil {
				return types.None, false
			}
			// Convert the name first: the next decoder call voids the token.
			key := jsonStringToMOO(name.String(), embeddedTypes)
			value, ok := decodeJSONToMOO(decoder, embeddedTypes)
			if !ok {
				return types.None, false
			}
			pairs = append(pairs, [2]types.Value{key, value})
		}
		if _, err := decoder.ReadToken(); err != nil {
			return types.None, false
		}
		// NewMap replaces a repeated key, so a short map means two names collided.
		m := types.NewMap(pairs)
		if m.Len() != len(pairs) {
			return types.None, false
		}
		return m, true

	default:
		return types.None, false
	}
}

// jsonNumberToMOO converts JSON number text: an integer when it has no
// fraction or exponent and fits, otherwise a float.
func jsonNumberToMOO(text string) (types.Value, bool) {
	if !strings.ContainsAny(text, ".eE") {
		if integer, err := strconv.ParseInt(text, 10, 64); err == nil {
			return types.NewInt(integer), true
		}
	}
	floating, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return types.None, false
	}
	return types.NewFloat(floating), true
}

// jsonStringToMOO converts a decoded JSON string or object member name.
func jsonStringToMOO(s string, embeddedTypes bool) types.Value {
	s = encodeJSONDecodedBytes(s)
	if embeddedTypes {
		if parsed, ok := parseEmbeddedType(s); ok {
			return parsed
		}
	}
	return types.NewStr(s)
}

// encodeJSONDecodedBytes writes decoded JSON text as a MOO string: a tab
// stays a tab, a tilde and every other control or non-ASCII byte become a
// ~XX binary escape.
func encodeJSONDecodedBytes(s string) string {
	var result strings.Builder
	for _, b := range []byte(s) {
		switch {
		case b == '\t':
			result.WriteByte(b)
		case b == '~':
			result.WriteString("~7E")
		case b < 32 || b > 126:
			fmt.Fprintf(&result, "~%02X", b)
		default:
			result.WriteByte(b)
		}
	}
	return result.String()
}

// splitEmbeddedType recognizes the final annotation, even on an empty prefix.
func splitEmbeddedType(s string) (string, string) {
	if i := strings.LastIndexByte(s, '|'); i >= 0 {
		switch tag := s[i+1:]; tag {
		case "int", "float", "str", "obj", "err", "bool":
			return s[:i], tag
		}
	}
	return s, ""
}

// parseEmbeddedType parses a type-annotated string like "123|int" or "#5|obj".
func parseEmbeddedType(s string) (types.Value, bool) {
	text, tag := splitEmbeddedType(s)
	switch tag {
	case "int":
		return types.NewInt(cStrtol(text)), true
	case "bool":
		return types.NewBool(text == "true"), true
	case "float":
		if text == "" {
			return types.NewFloat(0.0), true
		}
		var f float64
		if _, err := fmt.Sscanf(text, "%f", &f); err == nil {
			return types.NewFloat(f), true
		}
	case "str":
		return types.NewStr(text), true
	case "obj":
		// Toast: strtol() of the text after an optional '#'.
		return types.NewObj(types.ObjID(cStrtol(strings.TrimPrefix(text, "#")))), true
	case "err":
		if text == "" {
			return types.NewErr(types.E_NONE), true
		}
		if errCode, ok := types.ErrorFromString(text); ok {
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

func jsonDescriptors() []Descriptor {
	return []Descriptor{
		function("generate_json", builtinGenerateJson, 1, 3, -1, 2, -1),
		function("parse_json", builtinParseJson, 1, 2, 2, 2),
	}
}
