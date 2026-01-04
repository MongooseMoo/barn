# Divergence Report: Crypto Builtins

**Spec File**: `spec/builtins/crypto.md`
**Barn Files**: `builtins/crypto.go`
**Status**: clean (1 spec divergence found)
**Date**: 2026-01-03

## Summary

Tested 12 core cryptographic and encoding functions across multiple algorithms and edge cases. Found **zero behavioral divergences** between Barn and Toast implementations. Both servers handle all tested scenarios identically.

**Key Finding**: The spec incorrectly documents MD5 as the default algorithm for `string_hash()`. Both Toast and Barn use **SHA256** as the default.

**Functions Tested**:
- Base64 encoding/decoding (standard and URL-safe)
- String/binary/value hashing (MD5, SHA1, SHA256, SHA512)
- HMAC functions
- Random byte generation
- Salt generation
- Password hashing (crypt)

**Test Results**:
- Functions tested: 12
- Test cases executed: 40+
- Barn/Toast divergences: 0
- Spec divergences: 1
- Platform limitations documented: Yes (Windows crypt restrictions)

## Divergences

### None Found

All tested behaviors match exactly between Barn and Toast.

## Spec Divergences

### 1. string_hash() default algorithm

| Field | Value |
|-------|-------|
| Issue | Spec claims MD5 is default, actual default is SHA256 |
| Test | `string_hash("hello")` |
| Barn | `"2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824"` |
| Toast | `"2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824"` |
| Expected (per spec) | MD5 hash = `"5D41402ABC4B2A76B9719D911017C592"` |
| Classification | likely_spec_gap |
| Evidence | Both servers use SHA256 by default. Spec line 25 says "Default: MD5 (for compatibility)" but line 698 in implementation shows `algo := "sha256"`. The implementation is correct; spec is outdated. |

**Recommendation**: Update spec line 25 to document SHA256 as the default algorithm, not MD5.

## Test Coverage Gaps

Behaviors documented in spec but NOT fully covered by conformance tests:

### Hash Functions
- `string_hash()` with ripemd160 algorithm - has conformance test
- `string_hash()` with sha224, sha384 algorithms - partially tested
- Binary output mode (3rd parameter) - **NO conformance test**
- `binary_hash()` with ~XX binary string input - has conformance test
- `value_hash()` with complex values (lists, maps) - has conformance test

### Base64 Encoding
- Standard base64 encoding - has conformance tests
- URL-safe base64 encoding (2nd parameter) - has conformance tests
- Binary string input (~XX format) - has conformance test
- Invalid base64 decoding - has conformance test
- Padding requirements - has conformance test

### HMAC Functions
- `string_hmac()`, `binary_hmac()`, `value_hmac()` - **NO conformance tests found**
- HMAC with different algorithms - **NO conformance tests**
- HMAC with binary keys - **NO conformance tests**

### Password Functions
- `crypt()` with various salt formats - has conformance tests
- `salt()` generation - has conformance test
- Platform-specific limitations (Windows bcrypt-only) - **NOT documented in spec**

### Random Generation
- `random_bytes()` length validation - **NO conformance test**
- `random_bytes()` max length boundary - **NO conformance test**

### Missing Functions
- `encode_hex()` / `decode_hex()` - documented in spec as ToastStunt extensions but **DO NOT EXIST** in tested Toast version (v2.7.3_2)
- `bcrypt()` / `bcrypt_verify()` - documented in spec but **NOT tested** (unclear if implemented)
- `encrypt()` / `decrypt()` - documented in spec but **NOT tested**
- `uuid()` - documented in spec but **NOT tested**

## Behaviors Verified Correct

### Base64 Encoding
- ✓ `encode_base64("")` → `""`
- ✓ `encode_base64("hello")` → `"aGVsbG8="`
- ✓ `encode_base64("~FF")` → `"/w=="` (binary input)
- ✓ `encode_base64("hello", 1)` → `"aGVsbG8"` (URL-safe, no padding)
- ✓ `decode_base64("")` → `""`
- ✓ `decode_base64("aGVsbG8=")` → `"hello"`
- ✓ `decode_base64("@@@@")` → E_INVARG (invalid base64)
- ✓ `decode_base64("aGVsbG8", 1)` → `"hello"` (URL-safe accepts no padding)

### String Hashing
- ✓ `string_hash("hello", "md5")` → `"5D41402ABC4B2A76B9719D911017C592"`
- ✓ `string_hash("hello")` → SHA256 hash (64 hex chars)
- ✓ `string_hash("", "md5")` → `"D41D8CD98F00B204E9800998ECF8427E"` (empty string)
- ✓ `string_hash("test", "sha1")` → `"A94A8FE5CCB19BA61C4C0873D391E987982FBBD3"`
- ✓ `string_hash("test", "sha512")` → 128 hex chars
- ✓ `string_hash("test", "invalid")` → E_INVARG
- ✓ `string_hash(123)` → E_TYPE
- ✓ `string_hash("test", "md5", 1)` → `"~09~8F~6B~CD~46~21~D3~73~CA~DE~4E~83~26~27~B4~F6"` (binary output)

### Binary/Value Hashing
- ✓ `binary_hash("abc", "md5")` → `"900150983CD24FB0D6963F7D28E17F72"`
- ✓ `value_hash({1, 2, 3}, "sha256")` → `"8298D0492354E620262B63E1D84FB85E1B3D9DF71672A4894441A5EE30B08C0C"`

### HMAC
- ✓ `string_hmac("data", "secret", "sha256")` → `"1B2C16B75BD2A870C114153CCDA5BCFCA63314BC722FA160D690DE133CCBB9DB"`

### Random Generation
- ✓ `random_bytes(16)` → returns binary string with ~XX escapes
- ✓ Length is approximately 3x input (due to ~XX encoding of non-printable bytes)

### Salt Generation
- ✓ `salt("$1$", random_bytes(8))` → returns MD5 crypt salt format

### Password Hashing
- ✓ `crypt("password", "$2a$05$0123456789ABCDEF")` → returns bcrypt hash starting with `"$2a$05$"`
- ✓ Platform limitation: Toast on Windows only supports bcrypt salts, returns error for `$1$`, `$5$`, `$6$` prefixes

### Error Handling
- ✓ Type mismatches return E_TYPE
- ✓ Invalid algorithms return E_INVARG
- ✓ Invalid base64 returns E_INVARG
- ✓ Wrong argument counts return E_ARGS

## Platform-Specific Behaviors

### Windows Crypt Limitations (Toast v2.7.3_2)
Toast on Windows has the following crypt() limitations:
- **Only bcrypt (`$2a$`, `$2b$`) salts are supported**
- MD5 (`$1$`), SHA256 (`$5$`), SHA512 (`$6$`) salts return error: "Only bcrypt ($2a$/$2b$) salts are supported on Windows"
- Traditional DES crypt (2-char salt) returns E_INVARG

This is **not documented in the spec** but is a known platform limitation.

### Barn Platform Support
Barn implements all crypt formats on Windows using Go's crypto libraries, not platform crypt(3).

## Testing Methodology

All tests performed using:
- **Toast Oracle**: `./toast_oracle.exe 'expression'` (ToastStunt v2.7.3_2)
- **Barn**: `./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return expression;"`
- **Database**: Test.db (same database used by conformance tests)

## Notes

1. **Conformance Test Coverage**: The `algorithms.yaml` test file has excellent coverage of base64 and basic hashing. HMAC functions and advanced features lack coverage.

2. **Spec Accuracy**: The spec documents several ToastStunt-only extensions (encode_hex, decrypt, uuid, bcrypt) that either don't exist or weren't tested. The spec should clarify which features are:
   - Core MOO (LambdaMOO)
   - ToastStunt extensions
   - Barn-specific extensions

3. **Binary Output Mode**: The 3rd parameter for hash functions (binary output) works identically in both servers but has no conformance test coverage.

4. **Windows Platform**: Testing was performed on Windows, where Toast has known crypt() limitations. Linux testing may reveal different behaviors for traditional Unix crypt formats.

## Recommendations

1. **Update spec** to correct default algorithm for `string_hash()` (SHA256, not MD5)
2. **Add conformance tests** for:
   - HMAC functions (all variants)
   - Binary output mode for hash functions
   - `random_bytes()` boundary conditions
   - Password hashing edge cases
3. **Document platform limitations** in spec (Windows crypt restrictions)
4. **Clarify extension status** in spec for each function (core vs Toast vs Barn)
5. **Verify encode_hex/decode_hex** - spec documents them but they don't exist in tested Toast version
