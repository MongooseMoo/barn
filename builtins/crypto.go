package builtins

import (
	"barn/types"
	"encoding/base64"
	"strings"
)

// ============================================================================
// CRYPTO AND ENCODING BUILTINS
// ============================================================================

// builtinEncodeBase64 encodes a string to base64
// encode_base64(str [, url_safe]) -> str
// Input string may contain ~XX binary escapes which are decoded first
func builtinEncodeBase64(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	urlSafe := false
	if len(args) == 2 {
		urlSafe = args[1].Truthy()
	}

	// First decode any ~XX escapes in the input
	bytes, hasError := decodeBinaryString(str.Value())
	if hasError {
		return types.Err(types.E_INVARG)
	}

	var encoded string
	if urlSafe {
		// URL-safe encoding without padding
		encoded = base64.RawURLEncoding.EncodeToString(bytes)
	} else {
		encoded = base64.StdEncoding.EncodeToString(bytes)
	}

	return types.Ok(types.NewStr(encoded))
}

// builtinDecodeBase64 decodes a base64 string
// decode_base64(str [, url_safe]) -> str
// Returns a binary string with ~XX escapes for non-printable bytes
func builtinDecodeBase64(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	urlSafe := false
	if len(args) == 2 {
		urlSafe = args[1].Truthy()
	}

	var decoded []byte
	var err error
	if urlSafe {
		// URL-safe can be with or without padding
		decoded, err = base64.RawURLEncoding.DecodeString(str.Value())
		if err != nil {
			// Try with padding
			decoded, err = base64.URLEncoding.DecodeString(str.Value())
		}
	} else {
		decoded, err = base64.StdEncoding.DecodeString(str.Value())
	}

	if err != nil {
		return types.Err(types.E_INVARG)
	}

	// Encode the result as a binary string with ~XX escapes
	var result strings.Builder
	for _, b := range decoded {
		if b == '~' {
			result.WriteString("~7E")
		} else if b < 32 || b > 126 {
			result.WriteString(encodeByteHex(b))
		} else {
			result.WriteByte(b)
		}
	}

	return types.Ok(types.NewStr(result.String()))
}

// builtinEncodeBinary converts values to ~XX binary encoding
// encode_binary(str) -> str
// encode_binary(list of strings/ints) -> str
// encode_binary(val1, val2, ...) -> str (varargs)
func builtinEncodeBinary(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}

	var result strings.Builder

	// Helper to encode a single value, returns error code or 0 if ok
	var encodeValue func(v types.Value) types.ErrorCode
	encodeValue = func(v types.Value) types.ErrorCode {
		switch val := v.(type) {
		case types.StrValue:
			for _, b := range []byte(val.Value()) {
				encodeByte(&result, b)
			}
		case types.IntValue:
			if val.Val < 0 || val.Val > 255 {
				return types.E_INVARG
			}
			encodeByte(&result, byte(val.Val))
		case types.ListValue:
			// List can contain strings or integers
			for i := 1; i <= val.Len(); i++ {
				if err := encodeValue(val.Get(i)); err != 0 {
					return err
				}
			}
		default:
			return types.E_TYPE
		}
		return 0
	}

	// Process all arguments
	for _, arg := range args {
		if err := encodeValue(arg); err != 0 {
			return types.Err(err)
		}
	}

	return types.Ok(types.NewStr(result.String()))
}

// encodeByte writes a byte to the builder, escaping non-printable chars
func encodeByte(result *strings.Builder, b byte) {
	if b == '~' {
		result.WriteString("~7E")
	} else if b < 32 || b > 126 {
		result.WriteString(encodeByteHex(b))
	} else {
		result.WriteByte(b)
	}
}

// encodeByteHex encodes a byte as ~XX
func encodeByteHex(b byte) string {
	const hexDigits = "0123456789ABCDEF"
	return string([]byte{'~', hexDigits[b>>4], hexDigits[b&0xF]})
}

// builtinDecodeBinary decodes a ~XX binary-encoded string
// decode_binary(str) -> list grouping printable chars as strings, non-printable as ints
// decode_binary(str, "as_str") -> str (raw bytes as string)
func builtinDecodeBinary(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	asStr := false
	if len(args) == 2 {
		flag, ok := args[1].(types.StrValue)
		if ok && flag.Value() == "as_str" {
			asStr = true
		}
	}

	// Decode the binary string
	bytes, hasErr := decodeBinaryString(str.Value())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	if asStr {
		return types.Ok(types.NewStr(string(bytes)))
	}

	// Group printable ASCII (32-126, excluding ~) as strings, non-printable as ints
	var elements []types.Value
	var currentStr strings.Builder

	flushString := func() {
		if currentStr.Len() > 0 {
			elements = append(elements, types.NewStr(currentStr.String()))
			currentStr.Reset()
		}
	}

	for _, b := range bytes {
		if b >= 32 && b <= 126 {
			// Printable ASCII - accumulate into string
			currentStr.WriteByte(b)
		} else {
			// Non-printable - flush any accumulated string, then add as int
			flushString()
			elements = append(elements, types.NewInt(int64(b)))
		}
	}
	flushString() // Flush any remaining string

	return types.Ok(types.NewList(elements))
}

// decodeBinaryString decodes a ~XX encoded string
func decodeBinaryString(s string) ([]byte, bool) {
	var result []byte
	i := 0
	for i < len(s) {
		if s[i] == '~' {
			if i+2 >= len(s) {
				return nil, true // Error: incomplete escape
			}
			hi := hexValue(s[i+1])
			lo := hexValue(s[i+2])
			if hi < 0 || lo < 0 {
				return nil, true // Error: invalid hex
			}
			result = append(result, byte(hi<<4|lo))
			i += 3
		} else {
			result = append(result, s[i])
			i++
		}
	}
	return result, false
}

// hexValue returns the value of a hex digit, or -1 if invalid
func hexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'F':
		return int(c - 'A' + 10)
	case c >= 'a' && c <= 'f':
		return int(c - 'a' + 10)
	default:
		return -1
	}
}

// builtinCrypt hashes a string (simple placeholder)
// crypt(str [, salt]) -> str
func builtinCrypt(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Salt is optional
	salt := ""
	if len(args) == 2 {
		saltVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		salt = saltVal.Value()
	}

	// For now, just return a simple hash-like string
	// A real implementation would use Unix crypt
	_ = salt
	result := "$1$" + simpleHash(str.Value())
	return types.Ok(types.NewStr(result))
}

// simpleHash creates a simple hash string (placeholder)
func simpleHash(s string) string {
	// This is a placeholder - real MOO uses Unix crypt
	var hash uint32 = 0
	for _, c := range s {
		hash = hash*31 + uint32(c)
	}
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789./"
	result := make([]byte, 8)
	for i := range result {
		result[i] = chars[hash%64]
		hash /= 64
	}
	return string(result)
}
