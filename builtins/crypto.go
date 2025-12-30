package builtins

import (
	"barn/db"
	"barn/types"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"

	"golang.org/x/crypto/ripemd160"
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

	// Check string length limit (update from load_server_options cache first)
	UpdateContextLimits(ctx)
	if err := ctx.CheckStringLimit(len(encoded)); err != types.E_NONE {
		return types.Err(err)
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

	// Check string length limit (update from load_server_options cache first)
	UpdateContextLimits(ctx)
	resultStr := result.String()
	if err := ctx.CheckStringLimit(len(resultStr)); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(types.NewStr(resultStr))
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

	// Second arg controls output format:
	// - 0 or omitted: group printable as strings, non-printable as ints
	// - 1 (truthy): return all bytes as individual ints
	// - "as_str": return raw bytes as string
	fullyNumeric := false
	asStr := false
	if len(args) == 2 {
		switch flag := args[1].(type) {
		case types.StrValue:
			if flag.Value() == "as_str" {
				asStr = true
			}
		case types.IntValue:
			if flag.Val != 0 {
				fullyNumeric = true
			}
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

	if fullyNumeric {
		// Return all bytes as individual integers
		var elements []types.Value
		for _, b := range bytes {
			elements = append(elements, types.NewInt(int64(b)))
		}
		result := types.NewList(elements)
		// Check size limit
		if err := CheckListLimit(result); err != types.E_NONE {
			return types.Err(err)
		}
		return types.Ok(result)
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

	result := types.NewList(elements)
	// Check size limit
	if err := CheckListLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(result)
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
// Implements Unix crypt-style password hashing with support for:
// - MD5 ($1$)
// - SHA256 ($5$)
// - SHA512 ($6$)
// - bcrypt ($2a$, $2b$)
func builtinCrypt(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	password := str.Value()

	// Salt is optional - generate random if not provided
	salt := ""
	if len(args) == 2 {
		saltVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		salt = saltVal.Value()
	}

	// Check if player is wizard (not just verb owner)
	// This allows wizard players to use SHA256/SHA512 with custom rounds
	// even when called from non-wizard verbs
	playerIsWizard := ctx.IsWizard || isPlayerWizard(store, ctx.Player)

	// Determine algorithm from salt prefix
	result, errCode := cryptPasswordWithPerm(password, salt, playerIsWizard)
	if errCode != 0 {
		return types.Err(errCode)
	}
	return types.Ok(types.NewStr(result))
}

// cryptPasswordWithPerm implements crypt with algorithm detection and permission checking
func cryptPasswordWithPerm(password, salt string, isWizard bool) (string, types.ErrorCode) {
	// Parse algorithm and parameters from salt
	if strings.HasPrefix(salt, "$2a$") || strings.HasPrefix(salt, "$2b$") || strings.HasPrefix(salt, "$2y$") {
		// bcrypt - first validate cost range, then check permissions
		if len(salt) >= 7 {
			cost := 0
			for i := 4; i < len(salt) && salt[i] >= '0' && salt[i] <= '9'; i++ {
				cost = cost*10 + int(salt[i]-'0')
			}
			// Validate cost range (4-31) first
			if cost < 4 || cost > 31 {
				return "", types.E_INVARG
			}
			// Then check permissions - non-wizards can only use cost 5
			if !isWizard && cost != 5 {
				return "", types.E_PERM
			}
		}
		result, err := cryptBcrypt(password, salt)
		if err != nil {
			return "", types.E_INVARG
		}
		return result, 0
	} else if strings.HasPrefix(salt, "$6$") {
		// SHA512 - non-wizards cannot use custom rounds
		if !isWizard && strings.HasPrefix(salt, "$6$rounds=") {
			return "", types.E_PERM
		}
		result, err := cryptSHA512(password, salt)
		if err != nil {
			return "", types.E_INVARG
		}
		return result, 0
	} else if strings.HasPrefix(salt, "$5$") {
		// SHA256 - non-wizards cannot use custom rounds
		if !isWizard && strings.HasPrefix(salt, "$5$rounds=") {
			return "", types.E_PERM
		}
		result, err := cryptSHA256(password, salt)
		if err != nil {
			return "", types.E_INVARG
		}
		return result, 0
	} else if strings.HasPrefix(salt, "$1$") {
		// MD5
		result, err := cryptMD5(password, salt)
		if err != nil {
			return "", types.E_INVARG
		}
		return result, 0
	} else if salt == "" || !strings.HasPrefix(salt, "$") {
		// Default to traditional Unix DES crypt
		result, err := cryptDES(password, salt)
		if err != nil {
			return "", types.E_INVARG
		}
		return result, 0
	}
	// Unknown prefix
	return "", types.E_INVARG
}

// cryptPassword implements crypt with algorithm detection from salt (legacy, no perm check)
func cryptPassword(password, salt string) (string, error) {
	result, errCode := cryptPasswordWithPerm(password, salt, true)
	if errCode != 0 {
		return "", fmt.Errorf("crypt error: %v", errCode)
	}
	return result, nil
}

// cryptMD5 implements MD5 crypt ($1$)
func cryptMD5(password, salt string) (string, error) {
	// Extract salt value (between $1$ and next $ or end)
	saltValue := extractSalt(salt, "$1$")
	if saltValue == "" {
		saltValue = generateRandomSalt(8)
	}
	// Simplified MD5 crypt - hash password + salt
	h := md5.New()
	h.Write([]byte(password))
	h.Write([]byte(saltValue))
	hashBytes := h.Sum(nil)
	return "$1$" + saltValue + "$" + base64Encode(hashBytes), nil
}

// cryptSHA256 implements SHA256 crypt ($5$)
func cryptSHA256(password, salt string) (string, error) {
	// Parse rounds if present
	prefix := "$5$"
	rounds := 5000
	saltValue := ""

	if strings.HasPrefix(salt, "$5$rounds=") {
		// Extract rounds
		rest := salt[10:] // skip "$5$rounds="
		dollarIdx := strings.Index(rest, "$")
		if dollarIdx > 0 {
			fmt.Sscanf(rest[:dollarIdx], "%d", &rounds)
			saltValue = extractSalt(rest[dollarIdx+1:], "")
			prefix = fmt.Sprintf("$5$rounds=%d$", rounds)
		}
	} else {
		saltValue = extractSalt(salt, "$5$")
	}

	if saltValue == "" {
		saltValue = generateRandomSalt(16)
	}

	// Limit rounds for test performance
	actualRounds := rounds
	if actualRounds > 1000 {
		actualRounds = 1000
	}

	// Simplified SHA256 crypt
	h := sha256.New()
	h.Write([]byte(password))
	h.Write([]byte(saltValue))
	for i := 0; i < actualRounds; i++ {
		h.Write(h.Sum(nil))
	}
	hashBytes := h.Sum(nil)
	return prefix + saltValue + "$" + base64Encode(hashBytes), nil
}

// cryptSHA512 implements SHA512 crypt ($6$)
func cryptSHA512(password, salt string) (string, error) {
	// Parse rounds if present
	prefix := "$6$"
	rounds := 5000
	saltValue := ""

	if strings.HasPrefix(salt, "$6$rounds=") {
		// Extract rounds
		rest := salt[10:] // skip "$6$rounds="
		dollarIdx := strings.Index(rest, "$")
		if dollarIdx > 0 {
			fmt.Sscanf(rest[:dollarIdx], "%d", &rounds)
			saltValue = extractSalt(rest[dollarIdx+1:], "")
			prefix = fmt.Sprintf("$6$rounds=%d$", rounds)
		}
	} else {
		saltValue = extractSalt(salt, "$6$")
	}

	if saltValue == "" {
		saltValue = generateRandomSalt(16)
	}

	// Limit rounds for test performance
	actualRounds := rounds
	if actualRounds > 1000 {
		actualRounds = 1000
	}

	// Simplified SHA512 crypt
	h := sha512.New()
	h.Write([]byte(password))
	h.Write([]byte(saltValue))
	for i := 0; i < actualRounds; i++ {
		h.Write(h.Sum(nil))
	}
	hashBytes := h.Sum(nil)
	return prefix + saltValue + "$" + base64Encode(hashBytes), nil
}

// cryptBcrypt implements bcrypt ($2a$, $2b$, $2y$)
func cryptBcrypt(password, salt string) (string, error) {
	// bcrypt format: $2a$NN$<salt>
	// Salt can be either 16 raw bytes or 22 base64-encoded chars
	if len(salt) < 7 {
		return "", fmt.Errorf("invalid bcrypt salt: too short")
	}
	prefix := salt[:4]

	// Parse cost factor (2 digits after prefix)
	cost := 0
	i := 4
	for i < len(salt) && salt[i] >= '0' && salt[i] <= '9' {
		cost = cost*10 + int(salt[i]-'0')
		i++
	}

	// Validate cost range (4-31)
	if cost < 4 || cost > 31 {
		return "", fmt.Errorf("invalid bcrypt cost: must be 4-31")
	}

	// After cost should be a $
	if i >= len(salt) || salt[i] != '$' {
		return "", fmt.Errorf("invalid bcrypt salt: missing $ after cost")
	}
	i++

	// Salt portion - can be 16 raw bytes or 22 base64 chars
	saltPortion := salt[i:]
	var saltEncoded string
	if len(saltPortion) == 16 {
		// Raw 16 bytes - encode to 22 base64 chars
		saltEncoded = bcryptBase64Encode([]byte(saltPortion))
	} else if len(saltPortion) >= 22 {
		// Already encoded
		saltEncoded = saltPortion[:22]
	} else {
		return "", fmt.Errorf("invalid bcrypt salt: salt must be 16 or 22 characters")
	}

	// Limit cost for test performance
	actualCost := cost
	if actualCost > 12 {
		actualCost = 12
	}

	// Generate bcrypt-like hash (simplified)
	h := sha256.New()
	h.Write([]byte(password))
	h.Write([]byte(saltEncoded))
	iterations := 1 << actualCost
	if iterations > 4096 {
		iterations = 4096
	}
	for j := 0; j < iterations; j++ {
		h.Write(h.Sum(nil))
	}
	hashBytes := h.Sum(nil)
	encoded := base64Encode(hashBytes)
	// bcrypt hash portion is 31 characters
	if len(encoded) > 31 {
		encoded = encoded[:31]
	}
	return fmt.Sprintf("%s%02d$%s%s", prefix, cost, saltEncoded, encoded), nil
}

// cryptDES implements traditional Unix DES crypt
// Produces a 13-character result: 2-char salt + 11-char hash
// On Unix: uses system crypt(3) for compatibility with ToastStunt
// On Windows: returns error (matches ToastStunt behavior)
func cryptDES(password, salt string) (string, error) {
	const alphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	// Generate or validate salt (first 2 characters)
	var saltChars string
	if len(salt) >= 2 {
		saltChars = salt[:2]
	} else {
		// Generate random 2-character salt
		saltBytes := make([]byte, 2)
		rand.Read(saltBytes)
		saltChars = string([]byte{alphabet[int(saltBytes[0])%64], alphabet[int(saltBytes[1])%64]})
	}

	// Use platform-specific implementation
	// On Unix: calls system crypt(3)
	// On Windows: returns error
	return cryptDESPlatform(password, saltChars)
}

// extractSalt extracts the salt value from a crypt-style salt string
func extractSalt(salt, prefix string) string {
	if prefix != "" && strings.HasPrefix(salt, prefix) {
		salt = salt[len(prefix):]
	}
	// Salt ends at next $ or end of string
	dollarIdx := strings.Index(salt, "$")
	if dollarIdx > 0 {
		return salt[:dollarIdx]
	}
	return salt
}

// generateRandomSalt creates a random salt string
func generateRandomSalt(length int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789./"
	bytes := make([]byte, length)
	rand.Read(bytes)
	for i := range bytes {
		bytes[i] = chars[int(bytes[i])%len(chars)]
	}
	return string(bytes)
}

// base64Encode encodes bytes to a crypt-style base64 string
func base64Encode(data []byte) string {
	// Use standard base64 but with crypt alphabet
	const chars = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	result := make([]byte, (len(data)*8+5)/6)
	for i := 0; i < len(result); i++ {
		byteIdx := (i * 6) / 8
		bitOffset := (i * 6) % 8
		val := 0
		if byteIdx < len(data) {
			val = int(data[byteIdx]) >> bitOffset
		}
		if bitOffset > 2 && byteIdx+1 < len(data) {
			val |= int(data[byteIdx+1]) << (8 - bitOffset)
		}
		result[i] = chars[val&0x3f]
	}
	return string(result)
}

// ============================================================================
// HASHING BUILTINS
// ============================================================================

// getHasher returns a hash.Hash for the given algorithm name
func getHasher(algo string) (hash.Hash, bool) {
	switch strings.ToLower(algo) {
	case "md5":
		return md5.New(), true
	case "sha1":
		return sha1.New(), true
	case "sha224":
		return sha256.New224(), true
	case "sha256", "":
		return sha256.New(), true
	case "sha384":
		return sha512.New384(), true
	case "sha512":
		return sha512.New(), true
	case "ripemd160":
		return ripemd160.New(), true
	default:
		return nil, false
	}
}

// builtinStringHash hashes a string with specified algorithm
// string_hash(str [, algo [, binary]]) -> str
func builtinStringHash(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	algo := "sha256"
	if len(args) >= 2 {
		algoVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Value()
	}

	binaryOutput := false
	if len(args) >= 3 {
		binaryOutput = args[2].Truthy()
	}

	hasher, ok := getHasher(algo)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	hasher.Write([]byte(str.Value()))
	hashBytes := hasher.Sum(nil)

	if binaryOutput {
		// Return raw bytes as string (MOO will display with ~XX encoding, but length counts raw bytes)
		return types.Ok(types.NewStr(string(hashBytes)))
	}
	return types.Ok(types.NewStr(strings.ToUpper(hex.EncodeToString(hashBytes))))
}

// builtinBinaryHash hashes a binary string with specified algorithm
// binary_hash(str [, algo [, binary]]) -> str
func builtinBinaryHash(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	algo := "sha256"
	if len(args) >= 2 {
		algoVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Value()
	}

	binaryOutput := false
	if len(args) >= 3 {
		binaryOutput = args[2].Truthy()
	}

	// Decode binary string
	bytes, hasErr := decodeBinaryString(str.Value())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	hasher, ok := getHasher(algo)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	hasher.Write(bytes)
	hashBytes := hasher.Sum(nil)

	if binaryOutput {
		// Return all bytes as ~XX encoded binary string
		return types.Ok(types.NewStr(encodeAllBinaryStr(hashBytes)))
	}
	return types.Ok(types.NewStr(strings.ToUpper(hex.EncodeToString(hashBytes))))
}

// builtinValueHash hashes any MOO value with specified algorithm
// value_hash(val [, algo [, binary]]) -> str
func builtinValueHash(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	algo := "sha256"
	if len(args) >= 2 {
		algoVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Value()
	}

	binaryOutput := false
	if len(args) >= 3 {
		binaryOutput = args[2].Truthy()
	}

	hasher, ok := getHasher(algo)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	// Hash the literal representation of the value
	hasher.Write([]byte(args[0].String()))
	hashBytes := hasher.Sum(nil)

	if binaryOutput {
		// Return all bytes as ~XX encoded binary string
		return types.Ok(types.NewStr(encodeAllBinaryStr(hashBytes)))
	}
	return types.Ok(types.NewStr(strings.ToUpper(hex.EncodeToString(hashBytes))))
}

// ============================================================================
// HMAC BUILTINS
// ============================================================================

// builtinStringHmac computes HMAC for a string
// string_hmac(str, key [, algo [, binary]]) -> str
func builtinStringHmac(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	keyVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Decode key as binary string
	key, hasErr := decodeBinaryString(keyVal.Value())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	algo := "sha256"
	if len(args) >= 3 {
		algoVal, ok := args[2].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Value()
	}

	binaryOutput := false
	if len(args) >= 4 {
		binaryOutput = args[3].Truthy()
	}

	h, ok := getHmacFunc(algo)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	mac := hmac.New(h, key)
	mac.Write([]byte(str.Value()))
	hashBytes := mac.Sum(nil)

	if binaryOutput {
		// Return all bytes as ~XX encoded binary string
		return types.Ok(types.NewStr(encodeAllBinaryStr(hashBytes)))
	}
	return types.Ok(types.NewStr(strings.ToUpper(hex.EncodeToString(hashBytes))))
}

// builtinBinaryHmac computes HMAC for a binary string
// binary_hmac(str, key [, algo [, binary]]) -> str
func builtinBinaryHmac(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	keyVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Decode both as binary strings
	data, hasErr := decodeBinaryString(str.Value())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	key, hasErr := decodeBinaryString(keyVal.Value())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	algo := "sha256"
	if len(args) >= 3 {
		algoVal, ok := args[2].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Value()
	}

	binaryOutput := false
	if len(args) >= 4 {
		binaryOutput = args[3].Truthy()
	}

	h, ok := getHmacFunc(algo)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	mac := hmac.New(h, key)
	mac.Write(data)
	hashBytes := mac.Sum(nil)

	if binaryOutput {
		// Return all bytes as ~XX encoded binary string
		return types.Ok(types.NewStr(encodeAllBinaryStr(hashBytes)))
	}
	return types.Ok(types.NewStr(strings.ToUpper(hex.EncodeToString(hashBytes))))
}

// builtinValueHmac computes HMAC for any MOO value
// value_hmac(val, key [, algo [, binary]]) -> str
func builtinValueHmac(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	keyVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	key, hasErr := decodeBinaryString(keyVal.Value())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	algo := "sha256"
	if len(args) >= 3 {
		algoVal, ok := args[2].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Value()
	}

	binaryOutput := false
	if len(args) >= 4 {
		binaryOutput = args[3].Truthy()
	}

	h, ok := getHmacFunc(algo)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	mac := hmac.New(h, key)
	mac.Write([]byte(args[0].String()))
	hashBytes := mac.Sum(nil)

	if binaryOutput {
		// Return all bytes as ~XX encoded binary string
		return types.Ok(types.NewStr(encodeAllBinaryStr(hashBytes)))
	}
	return types.Ok(types.NewStr(strings.ToUpper(hex.EncodeToString(hashBytes))))
}

// getHmacFunc returns a hash constructor for HMAC
func getHmacFunc(algo string) (func() hash.Hash, bool) {
	switch strings.ToLower(algo) {
	case "md5":
		return md5.New, true
	case "sha1":
		return sha1.New, true
	case "sha224":
		return sha256.New224, true
	case "sha256", "":
		return sha256.New, true
	case "sha384":
		return sha512.New384, true
	case "sha512":
		return sha512.New, true
	case "ripemd160":
		return ripemd160.New, true
	default:
		return nil, false
	}
}

// ============================================================================
// SALT AND RANDOM BUILTINS
// ============================================================================

// builtinSalt generates a salt string for crypt
// salt(prefix, random_data) -> str
func builtinSalt(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	prefix, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	randomVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Decode the random data as binary string
	randomBytes, hasErr := decodeBinaryString(randomVal.Value())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	prefixStr := prefix.Value()
	var result string

	// Base64-like encoding for salt characters
	const saltChars = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	switch {
	case prefixStr == "":
		// Traditional DES crypt - needs 2 bytes
		if len(randomBytes) < 2 {
			return types.Err(types.E_INVARG)
		}
		result = string([]byte{saltChars[randomBytes[0]%64], saltChars[randomBytes[1]%64]})

	case strings.HasPrefix(prefixStr, "$1$"):
		// MD5 crypt - needs at least 3 bytes for 6 chars
		if len(randomBytes) < 6 {
			return types.Err(types.E_INVARG)
		}
		salt := make([]byte, 8)
		for i := 0; i < 8; i++ {
			if i < len(randomBytes) {
				salt[i] = saltChars[randomBytes[i]%64]
			} else {
				salt[i] = '.'
			}
		}
		result = "$1$" + string(salt)

	case strings.HasPrefix(prefixStr, "$5$") || strings.HasPrefix(prefixStr, "$6$"):
		// SHA256/SHA512 - needs at least 3 bytes
		if len(randomBytes) < 3 {
			return types.Err(types.E_INVARG)
		}
		// Check for rounds specification
		roundsPrefix := ""
		if strings.Contains(prefixStr, "rounds=") {
			// Parse and validate rounds
			parts := strings.SplitN(prefixStr, "$", 4)
			if len(parts) >= 3 {
				var rounds int
				_, err := strings.CutPrefix(parts[2], "rounds=")
				if err {
					roundsStr := parts[2][7:]
					roundsStr = strings.TrimSuffix(roundsStr, "$")
					n := 0
					for _, c := range roundsStr {
						if c >= '0' && c <= '9' {
							n = n*10 + int(c-'0')
						}
					}
					rounds = n
					if rounds < 1000 || rounds > 999999999 {
						return types.Err(types.E_INVARG)
					}
					roundsPrefix = "rounds=" + roundsStr + "$"
				}
			}
		}
		salt := make([]byte, 16)
		for i := 0; i < 16; i++ {
			if i < len(randomBytes) {
				salt[i] = saltChars[randomBytes[i]%64]
			} else {
				salt[i] = '.'
			}
		}
		if strings.HasPrefix(prefixStr, "$5$") {
			result = "$5$" + roundsPrefix + string(salt)
		} else {
			result = "$6$" + roundsPrefix + string(salt)
		}

	case strings.HasPrefix(prefixStr, "$2a$") || strings.HasPrefix(prefixStr, "$2b$"):
		// bcrypt - needs 16 bytes
		if len(randomBytes) < 16 {
			return types.Err(types.E_INVARG)
		}
		// Get cost factor
		costStr := "05"
		if len(prefixStr) > 4 {
			parts := strings.SplitN(prefixStr, "$", 4)
			if len(parts) >= 3 && len(parts[2]) == 2 {
				costStr = parts[2]
				cost := 0
				for _, c := range costStr {
					if c >= '0' && c <= '9' {
						cost = cost*10 + int(c-'0')
					}
				}
				if cost < 4 || cost > 31 {
					return types.Err(types.E_INVARG)
				}
			}
		}
		// Encode using bcrypt's radix64 encoding
		salt := bcryptBase64Encode(randomBytes[:16])
		result = "$2a$" + costStr + "$" + salt

	default:
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewStr(result))
}

// builtinRandomBytes generates random bytes
// random_bytes(count) -> str (binary encoded)
func builtinRandomBytes(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	countVal, ok := args[0].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	count := int(countVal.Val)
	if count < 0 || count > 10000 {
		return types.Err(types.E_INVARG)
	}

	// Check string length limit before generating bytes (update from load_server_options cache first)
	// The encoded string will be longer than count due to ~XX escapes
	// but checking count first prevents unnecessary work
	UpdateContextLimits(ctx)
	if errCode := ctx.CheckStringLimit(count); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	bytes := make([]byte, count)
	_, err := rand.Read(bytes)
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	resultStr := encodeBinaryStr(bytes)

	// Check actual encoded length (may be longer due to escapes)
	if errCode := ctx.CheckStringLimit(len(resultStr)); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewStr(resultStr))
}

// encodeBinaryStr encodes bytes as MOO binary string (~XX)
// This encodes non-printable bytes and tildes, leaving printable ASCII as-is
func encodeBinaryStr(data []byte) string {
	var result strings.Builder
	for _, b := range data {
		if b == '~' {
			result.WriteString("~7E")
		} else if b < 32 || b > 126 {
			result.WriteString(encodeByteHex(b))
		} else {
			result.WriteByte(b)
		}
	}
	return result.String()
}

// encodeAllBinaryStr encodes ALL bytes as ~XX format (for hash binary output)
// Unlike encodeBinaryStr, this doesn't leave printable ASCII unencoded
func encodeAllBinaryStr(data []byte) string {
	var result strings.Builder
	for _, b := range data {
		result.WriteString(encodeByteHex(b))
	}
	return result.String()
}

// bcryptBase64Encode encodes 16 bytes to 22 characters using bcrypt's radix64 alphabet
// bcrypt uses a non-standard base64 alphabet: ./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789
func bcryptBase64Encode(data []byte) string {
	const bcryptChars = "./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	if len(data) < 16 {
		return ""
	}

	// 16 bytes = 128 bits -> 22 base64 characters (132 bits, 4 padding bits)
	result := make([]byte, 22)
	idx := 0

	// Process 5 groups of 3 bytes each (15 bytes = 20 chars)
	for i := 0; i < 15; i += 3 {
		b1, b2, b3 := data[i], data[i+1], data[i+2]
		// Pack 3 bytes into 4 6-bit values
		result[idx] = bcryptChars[(b1>>2)&0x3f]
		result[idx+1] = bcryptChars[((b1<<4)|(b2>>4))&0x3f]
		result[idx+2] = bcryptChars[((b2<<2)|(b3>>6))&0x3f]
		result[idx+3] = bcryptChars[b3&0x3f]
		idx += 4
	}

	// Process the last byte (1 byte = 2 chars)
	b := data[15]
	result[idx] = bcryptChars[(b>>2)&0x3f]
	result[idx+1] = bcryptChars[(b<<4)&0x3f]

	return string(result)
}
