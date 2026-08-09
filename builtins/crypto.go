package builtins

import (
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
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"

	cryptbcrypt "github.com/go-crypt/x/bcrypt"
	//lint:ignore SA1019 RIPEMD-160 is required for string_hash compatibility with Toast.
	"golang.org/x/crypto/ripemd160"
)

// ============================================================================
// CRYPTO AND ENCODING BUILTINS
// ============================================================================

// builtinEncodeBase64 encodes a string to base64
// encode_base64(str [, url_safe]) -> str
// Input string may contain ~XX binary escapes which are decoded first
func builtinEncodeBase64(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str := args[0]
	ok := str.Type() == types.TYPE_STR
	if !ok {
		return types.Err(types.E_TYPE)
	}

	urlSafe := false
	if len(args) == 2 {
		urlSafe = args[1].Truthy()
	}

	// First decode any ~XX escapes in the input
	bytes, hasError := decodeBinaryString(str.Str())
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
func builtinDecodeBase64(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str := args[0]
	ok := str.Type() == types.TYPE_STR
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
		// URL-safe can be with or without padding, or partial padding
		input := strings.TrimRight(str.Str(), "=")
		decoded, err = base64.RawURLEncoding.DecodeString(input)
	} else {
		decoded, err = base64.StdEncoding.DecodeString(str.Str())
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
func builtinEncodeBinary(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) == 0 {
		return types.Ok(types.NewStr(""))
	}

	var result strings.Builder

	// Helper to encode a single value, returns error code or 0 if ok
	var encodeValue func(v types.Value) types.ErrorCode
	encodeValue = func(v types.Value) types.ErrorCode {
		switch v.Type() {
		case types.TYPE_STR:
			for _, b := range []byte(v.Str()) {
				encodeByte(&result, b)
			}
		case types.TYPE_INT:
			if v.Int() < 0 || v.Int() > 255 {
				return types.E_INVARG
			}
			encodeByte(&result, byte(v.Int()))
		case types.TYPE_LIST:
			// List can contain strings or integers
			for i := 1; i <= v.Len(); i++ {
				if err := encodeValue(v.Get(i)); err != 0 {
					return err
				}
			}
		default:
			return types.E_INVARG
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
func builtinDecodeBinary(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str := args[0]
	ok := str.Type() == types.TYPE_STR
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
		switch args[1].Type() {
		case types.TYPE_STR:
			if args[1].Str() == "as_str" {
				asStr = true
			}
		case types.TYPE_INT:
			if args[1].Int() != 0 {
				fullyNumeric = true
			}
		}
	}

	// Decode the binary string
	bytes, hasErr := decodeBinaryString(str.Str())
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
		if err := CheckListLimitForTask(ctx, result); err != types.E_NONE {
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
// - bcrypt ($2a$, $2x$, $2y$)
func builtinCrypt(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str := args[0]
	ok := str.Type() == types.TYPE_STR
	if !ok {
		return types.Err(types.E_TYPE)
	}

	password := str.Str()

	// Salt is optional - generate random if not provided
	salt := ""
	if len(args) == 2 {
		saltVal := args[1]
		ok := saltVal.Type() == types.TYPE_STR
		if !ok {
			return types.Err(types.E_TYPE)
		}
		salt = saltVal.Str()
	}

	// Check if player is wizard (not just verb owner)
	// This allows wizard players to use SHA256/SHA512 with custom rounds
	// even when called from non-wizard verbs
	playerIsWizard := ctx.IsWizard || isPlayerWizard(ctx, ctx.Player)

	// Determine algorithm from salt prefix
	bcryptCost, shaRounds, errCode := cryptWork(salt)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	maxBcryptCost, maxSHARounds := GetCryptWorkLimits()
	if pending := pendingServerOptions(ctx); pending != nil {
		maxBcryptCost = pending.MaxCryptBcryptCost
		maxSHARounds = pending.MaxCryptSHARounds
	}
	if bcryptCost > maxBcryptCost || shaRounds > maxSHARounds {
		return types.Err(types.E_QUOTA)
	}
	// Charge work before starting it. One tick represents 1,000 SHA rounds or
	// one bcrypt cost-4 work unit, so expensive hashes participate in the
	// task's existing tick budget instead of hiding behind a single opcode.
	workTicks := int64((shaRounds + 999) / 1000)
	if bcryptCost >= 4 {
		workTicks = int64(1) << (bcryptCost - 4)
	}
	if !ctx.ChargeBuiltinTicks(workTicks) {
		return types.Err(types.E_QUOTA)
	}

	result, errCode := cryptPasswordWithPerm(password, salt, playerIsWizard)
	if errCode != 0 {
		return types.Err(errCode)
	}
	return types.Ok(types.NewStr(result))
}

// cryptWork returns the caller-controlled work factor encoded in salt.
func cryptWork(salt string) (bcryptCost, shaRounds int, errCode types.ErrorCode) {
	if strings.HasPrefix(salt, "$2a$") || strings.HasPrefix(salt, "$2x$") || strings.HasPrefix(salt, "$2y$") {
		cost, err := parseBcryptPrefixCost(salt)
		if err != nil {
			return 0, 0, types.E_INVARG
		}
		return cost, 0, types.E_NONE
	}
	if strings.HasPrefix(salt, "$5$") {
		_, rounds, _ := parseCryptSalt(salt, "$5$", true, shaCryptSaltLenMax)
		return 0, rounds, types.E_NONE
	}
	if strings.HasPrefix(salt, "$6$") {
		_, rounds, _ := parseCryptSalt(salt, "$6$", true, shaCryptSaltLenMax)
		return 0, rounds, types.E_NONE
	}
	return 0, 0, types.E_NONE
}

// cryptPasswordWithPerm implements crypt with algorithm detection and permission checking
func cryptPasswordWithPerm(password, salt string, isWizard bool) (string, types.ErrorCode) {
	// Parse algorithm and parameters from salt
	if strings.HasPrefix(salt, "$2a$") || strings.HasPrefix(salt, "$2x$") || strings.HasPrefix(salt, "$2y$") {
		// bcrypt - first validate cost range, then check permissions
		cost, err := parseBcryptPrefixCost(salt)
		if err != nil {
			return "", types.E_INVARG
		}
		// Non-wizards can only use Toast's default cost after validation.
		if !isWizard && cost != 5 {
			return "", types.E_PERM
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
	// A "$tag$..." salt whose tag is not a recognized algorithm is rejected by
	// glibc crypt, which returns the failure marker "*0" (matching Toast).
	if strings.HasPrefix(salt, "$") {
		return "*0", 0
	}
	// Non-"$" salts with characters outside the canonical DES alphabet fall
	// through to traditional crypt, which keeps the original two leading
	// characters in the output (matching Toast).
	result, err := cryptDESUnknownPrefix(password, salt)
	if err != nil {
		return "", types.E_INVARG
	}
	return result, 0
}

// cryptPassword implements crypt with algorithm detection from salt (legacy, no perm check)
// ----------------------------------------------------------------------------
// Standard Unix crypt(3) password hashing: md5crypt ($1$), sha256crypt ($5$),
// sha512crypt ($6$).
//
// These implement the EXACT algorithms that glibc crypt(3) uses, which is what
// ToastStunt delegates to for these schemes (toaststunt/src/crypto.cc:373,
// bf_crypt -> system crypt(string, salt)).  sha256crypt/sha512crypt follow
// Ulrich Drepper's SHA-crypt specification (http://www.akkadia.org/drepper/
// SHA-crypt.txt); md5crypt follows Poul-Henning Kamp's FreeBSD MD5-crypt.
//
// The rounds= parameter is HONORED, with clamping ONLY as the spec defines
// (1000..999999999, default 5000) -- never a silent 1000 cap.  The algorithm
// and the GNU base64 output ordering are ported from the well-known pure-Go
// implementation github.com/GehirnInc/crypt (BSD-licensed), which is verified
// against the published Drepper/glibc known-answer vectors.
// ----------------------------------------------------------------------------

const (
	shaCryptRoundsMin     = 1000
	shaCryptRoundsMax     = 999999999
	shaCryptRoundsDefault = 5000
	shaCryptSaltLenMax    = 16
	md5CryptSaltLenMax    = 8
)

// cryptB64Alphabet is the GNU/crypt base64 alphabet used by md5crypt and
// SHA-crypt (note: starts with "./", differs from standard base64).
const cryptB64Alphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// cryptBase64_24Bit encodes bytes using the crypt-specific base64 variant that
// processes up to 3 bytes at a time in little-endian 6-bit groups, emitting no
// padding.  Matches glibc's b64_from_24bit ordering.
func cryptBase64_24Bit(src []byte) []byte {
	if len(src) == 0 {
		return []byte{}
	}
	dst := make([]byte, (len(src)*8+5)/6)
	di, si := 0, 0
	n := len(src) / 3 * 3
	for si < n {
		val := uint(src[si+2])<<16 | uint(src[si+1])<<8 | uint(src[si])
		dst[di+0] = cryptB64Alphabet[val&0x3f]
		dst[di+1] = cryptB64Alphabet[val>>6&0x3f]
		dst[di+2] = cryptB64Alphabet[val>>12&0x3f]
		dst[di+3] = cryptB64Alphabet[val>>18]
		di += 4
		si += 3
	}
	rem := len(src) - si
	if rem == 0 {
		return dst
	}
	val := uint(src[si+0])
	if rem == 2 {
		val |= uint(src[si+1]) << 8
	}
	dst[di+0] = cryptB64Alphabet[val&0x3f]
	dst[di+1] = cryptB64Alphabet[val>>6&0x3f]
	if rem == 2 {
		dst[di+2] = cryptB64Alphabet[val>>12]
	}
	return dst
}

// cryptRepeatBytes returns a slice of the given length filled by repeating
// input (the "seq" construction from the SHA-crypt spec, steps 16/20).
func cryptRepeatBytes(input []byte, length int) []byte {
	if len(input) == 0 {
		return make([]byte, length)
	}
	out := make([]byte, length)
	for i := 0; i < length; i += len(input) {
		copy(out[i:], input)
	}
	return out
}

// parseCryptSalt parses a "$<id>$[rounds=N$]salt" prefix.  It returns the salt
// (truncated to saltLenMax, terminated at the next '$'), the effective rounds
// (clamped to [shaCryptRoundsMin, shaCryptRoundsMax]), and whether a rounds=
// parameter was explicitly present.  hasRounds controls whether the rounds=
// parameter is recognized (md5crypt has no rounds).
func parseCryptSalt(salt, magic string, hasRounds bool, saltLenMax int) (saltValue string, rounds int, roundsGiven bool) {
	rounds = shaCryptRoundsDefault
	rest := strings.TrimPrefix(salt, magic)
	if hasRounds && strings.HasPrefix(rest, "rounds=") {
		afterEq := rest[len("rounds="):]
		if dollar := strings.IndexByte(afterEq, '$'); dollar >= 0 {
			if n, err := strconv.Atoi(afterEq[:dollar]); err == nil {
				rounds = n
				if rounds < shaCryptRoundsMin {
					rounds = shaCryptRoundsMin
				}
				if rounds > shaCryptRoundsMax {
					rounds = shaCryptRoundsMax
				}
				roundsGiven = true
				rest = afterEq[dollar+1:]
			}
		}
	}
	// Salt ends at the next '$' (the hash separator) if present.
	if dollar := strings.IndexByte(rest, '$'); dollar >= 0 {
		rest = rest[:dollar]
	}
	if len(rest) > saltLenMax {
		rest = rest[:saltLenMax]
	}
	return rest, rounds, roundsGiven
}

// cryptMD5 implements Poul-Henning Kamp's MD5-crypt ($1$), the algorithm used
// by glibc crypt(3) for the "$1$" prefix.
func cryptMD5(password, salt string) (string, error) {
	// A random salt is only synthesized when no salt argument was supplied at
	// all; "$1$$" is a deliberately EMPTY salt (matches glibc crypt(3)).
	if salt == "" {
		salt = "$1$" + generateRandomSalt(md5CryptSaltLenMax)
	}
	saltValue, _, _ := parseCryptSalt(salt, "$1$", false, md5CryptSaltLenMax)
	key := []byte(password)
	saltB := []byte(saltValue)
	keyLen := len(key)

	// Compute sumB = MD5(key || salt || key)
	h := md5.New()
	h.Write(key)
	h.Write(saltB)
	h.Write(key)
	sumB := h.Sum(nil)

	// Compute sumA = MD5(key || "$1$" || salt || repeat(sumB, keyLen) || ...)
	h.Reset()
	h.Write(key)
	h.Write([]byte("$1$"))
	h.Write(saltB)
	h.Write(cryptRepeatBytes(sumB, keyLen))
	for i := keyLen; i > 0; i >>= 1 {
		if i%2 == 0 {
			h.Write(key[0:1])
		} else {
			h.Write([]byte{0})
		}
	}
	sumA := h.Sum(nil)

	// 1000 rounds of strengthening.
	for i := 0; i < 1000; i++ {
		h.Reset()
		if i%2 != 0 {
			h.Write(key)
		} else {
			h.Write(sumA)
		}
		if i%3 != 0 {
			h.Write(saltB)
		}
		if i%7 != 0 {
			h.Write(key)
		}
		if i&1 != 0 {
			h.Write(sumA)
		} else {
			h.Write(key)
		}
		copy(sumA, h.Sum(nil))
	}

	out := cryptBase64_24Bit([]byte{
		sumA[12], sumA[6], sumA[0],
		sumA[13], sumA[7], sumA[1],
		sumA[14], sumA[8], sumA[2],
		sumA[15], sumA[9], sumA[3],
		sumA[5], sumA[10], sumA[4],
		sumA[11],
	})
	return "$1$" + saltValue + "$" + string(out), nil
}

// cryptSHA256 implements Ulrich Drepper's sha256crypt ($5$), the algorithm used
// by glibc crypt(3) for the "$5$" prefix.  rounds= is honored (clamped only to
// the spec range 1000..999999999, default 5000).
func cryptSHA256(password, salt string) (string, error) {
	if salt == "" {
		salt = "$5$" + generateRandomSalt(shaCryptSaltLenMax)
	}
	saltValue, rounds, roundsGiven := parseCryptSalt(salt, "$5$", true, shaCryptSaltLenMax)
	sumA := shaCryptDigest(sha256.New, []byte(password), []byte(saltValue), rounds)
	ordered := []byte{
		sumA[20], sumA[10], sumA[0],
		sumA[11], sumA[1], sumA[21],
		sumA[2], sumA[22], sumA[12],
		sumA[23], sumA[13], sumA[3],
		sumA[14], sumA[4], sumA[24],
		sumA[5], sumA[25], sumA[15],
		sumA[26], sumA[16], sumA[6],
		sumA[17], sumA[7], sumA[27],
		sumA[8], sumA[28], sumA[18],
		sumA[29], sumA[19], sumA[9],
		sumA[30], sumA[31],
	}
	return shaCryptOutput("$5$", saltValue, rounds, roundsGiven, ordered), nil
}

// cryptSHA512 implements Ulrich Drepper's sha512crypt ($6$), the algorithm used
// by glibc crypt(3) for the "$6$" prefix.  rounds= is honored (clamped only to
// the spec range 1000..999999999, default 5000).
func cryptSHA512(password, salt string) (string, error) {
	if salt == "" {
		salt = "$6$" + generateRandomSalt(shaCryptSaltLenMax)
	}
	saltValue, rounds, roundsGiven := parseCryptSalt(salt, "$6$", true, shaCryptSaltLenMax)
	sumA := shaCryptDigest(sha512.New, []byte(password), []byte(saltValue), rounds)
	ordered := []byte{
		sumA[42], sumA[21], sumA[0],
		sumA[1], sumA[43], sumA[22],
		sumA[23], sumA[2], sumA[44],
		sumA[45], sumA[24], sumA[3],
		sumA[4], sumA[46], sumA[25],
		sumA[26], sumA[5], sumA[47],
		sumA[48], sumA[27], sumA[6],
		sumA[7], sumA[49], sumA[28],
		sumA[29], sumA[8], sumA[50],
		sumA[51], sumA[30], sumA[9],
		sumA[10], sumA[52], sumA[31],
		sumA[32], sumA[11], sumA[53],
		sumA[54], sumA[33], sumA[12],
		sumA[13], sumA[55], sumA[34],
		sumA[35], sumA[14], sumA[56],
		sumA[57], sumA[36], sumA[15],
		sumA[16], sumA[58], sumA[37],
		sumA[38], sumA[17], sumA[59],
		sumA[60], sumA[39], sumA[18],
		sumA[19], sumA[61], sumA[40],
		sumA[41], sumA[20], sumA[62],
		sumA[63],
	}
	return shaCryptOutput("$6$", saltValue, rounds, roundsGiven, ordered), nil
}

// shaCryptOutput assembles the final "$id$[rounds=N$]salt$hash" string.  The
// "rounds=" segment is emitted only when the caller explicitly requested rounds
// (matching glibc, which omits it for the default).
func shaCryptOutput(magic, saltValue string, rounds int, roundsGiven bool, ordered []byte) string {
	var b strings.Builder
	b.WriteString(magic)
	if roundsGiven {
		b.WriteString("rounds=")
		b.WriteString(strconv.Itoa(rounds))
		b.WriteByte('$')
	}
	b.WriteString(saltValue)
	b.WriteByte('$')
	b.Write(cryptBase64_24Bit(ordered))
	return b.String()
}

// shaCryptDigest runs the Drepper SHA-crypt key derivation for the given hash
// constructor (sha256.New or sha512.New) and returns the raw digest bytes
// (before the algorithm-specific output permutation).
func shaCryptDigest(newHash func() hash.Hash, key, saltB []byte, rounds int) []byte {
	keyLen := len(key)
	saltLen := len(saltB)
	h := newHash()

	// sumB = H(key || salt || key)  (steps 4-8)
	h.Write(key)
	h.Write(saltB)
	h.Write(key)
	sumB := h.Sum(nil)

	// sumA  (steps 1-3, 9-12)
	h.Reset()
	h.Write(key)
	h.Write(saltB)
	h.Write(cryptRepeatBytes(sumB, keyLen))
	for i := keyLen; i > 0; i >>= 1 {
		if i%2 == 0 {
			h.Write(key)
		} else {
			h.Write(sumB)
		}
	}
	sumA := h.Sum(nil)

	// seqP  (steps 13-16): repeat(H(key * keyLen), keyLen)
	h.Reset()
	for i := 0; i < keyLen; i++ {
		h.Write(key)
	}
	seqP := cryptRepeatBytes(h.Sum(nil), keyLen)

	// seqS  (steps 17-20): repeat(H(salt * (16 + sumA[0])), saltLen)
	h.Reset()
	for i := 0; i < 16+int(sumA[0]); i++ {
		h.Write(saltB)
	}
	seqS := cryptRepeatBytes(h.Sum(nil), saltLen)

	// Step 21: the strengthening loop, honoring the requested round count.
	for i := 0; i < rounds; i++ {
		h.Reset()
		if i&1 != 0 {
			h.Write(seqP)
		} else {
			h.Write(sumA)
		}
		if i%3 != 0 {
			h.Write(seqS)
		}
		if i%7 != 0 {
			h.Write(seqP)
		}
		if i&1 != 0 {
			h.Write(sumA)
		} else {
			h.Write(seqP)
		}
		copy(sumA, h.Sum(nil))
	}
	return sumA
}

func parseBcryptPrefixCost(salt string) (int, error) {
	separator := strings.IndexByte(salt[4:], '$')
	if separator < 0 {
		return 0, nil
	}
	separator += 4
	if len(salt) < 7 || separator == 4 {
		return 0, fmt.Errorf("invalid bcrypt cost")
	}
	costToken := salt[4:separator]
	for _, digit := range costToken {
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf("invalid bcrypt cost")
		}
	}
	cost, err := strconv.Atoi(costToken)
	if err != nil {
		return 0, fmt.Errorf("invalid bcrypt cost")
	}
	if cost < 4 || cost > 31 {
		return 0, fmt.Errorf("invalid bcrypt cost: must be 4-31")
	}
	return cost, nil
}

func parseBcryptCost(salt string) (int, error) {
	if len(salt) < 7 || salt[4] < '0' || salt[4] > '9' ||
		salt[5] < '0' || salt[5] > '9' || salt[6] != '$' {
		return 0, fmt.Errorf("invalid bcrypt cost: expected exactly two digits")
	}
	cost := int(salt[4]-'0')*10 + int(salt[5]-'0')
	if cost < 4 || cost > 31 {
		return 0, fmt.Errorf("invalid bcrypt cost: must be 4-31")
	}
	return cost, nil
}

// cryptBcrypt implements bcrypt ($2a$, $2x$, $2y$)
func cryptBcrypt(password, salt string) (string, error) {
	// bcrypt format: $2a$NN$<salt>
	// Salt can be either 16 raw bytes or 22 base64-encoded chars
	if len(salt) < 7 {
		return "", fmt.Errorf("invalid bcrypt salt: too short")
	}
	prefix := salt[:4]

	cost, err := parseBcryptCost(salt)
	if err != nil {
		return "", err
	}

	// Salt portion - can be 16 raw bytes or 22 base64 chars
	saltPortion := salt[7:]
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

	rawSalt, err := cryptbcrypt.Base64Decode([]byte(saltEncoded))
	if err != nil || len(rawSalt) != 16 {
		return "", fmt.Errorf("invalid bcrypt salt")
	}

	key := []byte(password)
	if prefix == "$2x$" {
		key = bcrypt2xSchedule(key)
	}
	hash, err := cryptbcrypt.GenerateFromPasswordSalt(key, rawSalt, cost)
	if err != nil {
		return "", err
	}

	hashStr := string(hash)
	// GenerateFromPasswordSalt emits $2a$. The 2y corrected variant differs
	// only in its marker; 2x uses the historical schedule synthesized above.
	if (prefix == "$2x$" || prefix == "$2y$") && strings.HasPrefix(hashStr, "$2a$") {
		hashStr = prefix + hashStr[4:]
	}
	return hashStr, nil
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

// cryptDESUnknownPrefix handles crypt fallback for unknown "$"-prefixed salts.
// Compatibility behavior keeps the original two salt characters in output while
// normalizing them for hashing.
func cryptDESUnknownPrefix(password, salt string) (string, error) {
	if len(salt) < 2 {
		return cryptDES(password, salt)
	}
	original := salt[:2]
	normalized := string([]byte{
		normalizeDESSaltChar(original[0]),
		normalizeDESSaltChar(original[1]),
	})
	hash, err := cryptDESPlatform(password, normalized)
	if err != nil {
		return "", err
	}
	if len(hash) >= 2 {
		hash = original + hash[2:]
	}
	return hash, nil
}

func normalizeDESSaltChar(c byte) byte {
	const alphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	if strings.ContainsRune(alphabet, rune(c)) {
		return c
	}
	// Legacy mapping used by old crypt implementations for out-of-alphabet bytes.
	idx := (int(c) - int('.')) & 0x3f
	return alphabet[idx]
}

// extractSalt extracts the salt value from a crypt-style salt string
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
func builtinStringHash(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	str := args[0]
	ok := str.Type() == types.TYPE_STR
	if !ok {
		return types.Err(types.E_TYPE)
	}

	algo := "sha256"
	if len(args) >= 2 {
		algoVal := args[1]
		ok := algoVal.Type() == types.TYPE_STR
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Str()
	}

	binaryOutput := false
	if len(args) >= 3 {
		binaryOutput = args[2].Truthy()
	}

	hasher, ok := getHasher(algo)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	hasher.Write([]byte(str.Str()))
	hashBytes := hasher.Sum(nil)

	if binaryOutput {
		// Return all bytes as ~XX encoded string (each byte = 3 chars)
		return types.Ok(types.NewStr(encodeAllBinaryStr(hashBytes)))
	}
	return types.Ok(types.NewStr(strings.ToUpper(hex.EncodeToString(hashBytes))))
}

// builtinBinaryHash hashes a binary string with specified algorithm
// binary_hash(str [, algo [, binary]]) -> str
func builtinBinaryHash(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	str := args[0]
	ok := str.Type() == types.TYPE_STR
	if !ok {
		return types.Err(types.E_TYPE)
	}

	algo := "sha256"
	if len(args) >= 2 {
		algoVal := args[1]
		ok := algoVal.Type() == types.TYPE_STR
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Str()
	}

	binaryOutput := false
	if len(args) >= 3 {
		binaryOutput = args[2].Truthy()
	}

	// Decode binary string
	bytes, hasErr := decodeBinaryString(str.Str())
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
		// Return all bytes as ~XX encoded string (each byte = 3 chars)
		return types.Ok(types.NewStr(encodeAllBinaryStr(hashBytes)))
	}
	return types.Ok(types.NewStr(strings.ToUpper(hex.EncodeToString(hashBytes))))
}

// builtinValueHash hashes any MOO value with specified algorithm
// value_hash(val [, algo [, binary]]) -> str
func builtinValueHash(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	algo := "sha256"
	if len(args) >= 2 {
		algoVal := args[1]
		ok := algoVal.Type() == types.TYPE_STR
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Str()
	}

	binaryOutput := false
	if len(args) >= 3 {
		binaryOutput = args[2].Truthy()
	}

	hasher, ok := getHasher(algo)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	// Hash the public literal representation. Anonymous object identity is not
	// observable, so every ANON value hashes as *anonymous*.
	literal := args[0].String()
	if args[0].Type() == types.TYPE_ANON {
		literal = "*anonymous*"
	}
	hasher.Write([]byte(literal))
	hashBytes := hasher.Sum(nil)

	if binaryOutput {
		// Return all bytes as ~XX encoded string (each byte = 3 chars)
		return types.Ok(types.NewStr(encodeAllBinaryStr(hashBytes)))
	}
	return types.Ok(types.NewStr(strings.ToUpper(hex.EncodeToString(hashBytes))))
}

// ============================================================================
// HMAC BUILTINS
// ============================================================================

// builtinStringHmac computes HMAC for a string
// string_hmac(str, key [, algo [, binary]]) -> str
func builtinStringHmac(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	str := args[0]
	ok := str.Type() == types.TYPE_STR
	if !ok {
		return types.Err(types.E_TYPE)
	}

	keyVal := args[1]
	if keyVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	// Decode key as binary string
	key, hasErr := decodeBinaryString(keyVal.Str())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	algo := "sha256"
	if len(args) >= 3 {
		algoVal := args[2]
		ok := algoVal.Type() == types.TYPE_STR
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Str()
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
	mac.Write([]byte(str.Str()))
	hashBytes := mac.Sum(nil)

	if binaryOutput {
		// Return all bytes as ~XX encoded string (each byte = 3 chars)
		return types.Ok(types.NewStr(encodeAllBinaryStr(hashBytes)))
	}
	return types.Ok(types.NewStr(strings.ToUpper(hex.EncodeToString(hashBytes))))
}

// builtinBinaryHmac computes HMAC for a binary string
// binary_hmac(str, key [, algo [, binary]]) -> str
func builtinBinaryHmac(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	str := args[0]
	ok := str.Type() == types.TYPE_STR
	if !ok {
		return types.Err(types.E_TYPE)
	}

	keyVal := args[1]
	if keyVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	// Decode both as binary strings
	data, hasErr := decodeBinaryString(str.Str())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	key, hasErr := decodeBinaryString(keyVal.Str())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	algo := "sha256"
	if len(args) >= 3 {
		algoVal := args[2]
		ok := algoVal.Type() == types.TYPE_STR
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Str()
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
		// Return all bytes as ~XX encoded string (each byte = 3 chars)
		return types.Ok(types.NewStr(encodeAllBinaryStr(hashBytes)))
	}
	return types.Ok(types.NewStr(strings.ToUpper(hex.EncodeToString(hashBytes))))
}

// builtinValueHmac computes HMAC for any MOO value
// value_hmac(val, key [, algo [, binary]]) -> str
func builtinValueHmac(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	keyVal := args[1]
	if keyVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	key, hasErr := decodeBinaryString(keyVal.Str())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	algo := "sha256"
	if len(args) >= 3 {
		algoVal := args[2]
		ok := algoVal.Type() == types.TYPE_STR
		if !ok {
			return types.Err(types.E_TYPE)
		}
		algo = algoVal.Str()
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
		// Return all bytes as ~XX encoded string (each byte = 3 chars)
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
func builtinSalt(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	prefix := args[0]
	if prefix.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	randomVal := args[1]
	if randomVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	// Decode the random data as binary string
	randomBytes, hasErr := decodeBinaryString(randomVal.Str())
	if hasErr {
		return types.Err(types.E_INVARG)
	}

	prefixStr := prefix.Str()
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

	case strings.HasPrefix(prefixStr, "$2a$") || strings.HasPrefix(prefixStr, "$2y$"):
		// bcrypt - needs 16 bytes
		if len(randomBytes) < 16 {
			return types.Err(types.E_INVARG)
		}
		// Get cost factor
		costStr := "05"
		if len(prefixStr) > 4 {
			parts := strings.SplitN(prefixStr, "$", 4)
			if len(parts) >= 3 {
				costToken := parts[2]
				if len(costToken) < 2 {
					return types.Err(types.E_INVARG)
				}
				for _, c := range costToken {
					if c < '0' || c > '9' {
						return types.Err(types.E_INVARG)
					}
				}
				cost, err := strconv.Atoi(costToken)
				if err != nil {
					return types.Err(types.E_INVARG)
				}
				if cost < 4 || cost > 31 {
					return types.Err(types.E_INVARG)
				}
				costStr = fmt.Sprintf("%02d", cost)
			}
		}
		// Encode using bcrypt's radix64 encoding
		salt := bcryptBase64Encode(randomBytes[:16])
		result = prefixStr[:4] + costStr + "$" + salt

	default:
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewStr(result))
}

// builtinRandomBytes generates random bytes
// random_bytes(count) -> str (binary encoded)
func builtinRandomBytes(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	countVal := args[0]
	ok := countVal.Type() == types.TYPE_INT
	if !ok {
		return types.Err(types.E_TYPE)
	}

	count := int(countVal.Int())
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
