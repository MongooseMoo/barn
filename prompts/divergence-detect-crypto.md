# Task: Detect Divergences in Crypto Builtins

## Context

We need to verify Barn's cryptographic builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all crypto builtins.

## Files to Read

- `spec/builtins/crypto.md` - expected behavior specification
- `builtins/crypto.go` - Barn implementation (if exists)

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Hashing
- `string_hash()` - hash strings (MD5, SHA1, SHA256, etc.)
- `binary_hash()` - hash binary data

### Encoding
- `encode_base64()` - base64 encoding
- `decode_base64()` - base64 decoding
- `encode_hex()` - hex encoding (if exists)
- `decode_hex()` - hex decoding (if exists)

### Random
- `random_bytes()` - cryptographically secure random (if exists)

### Encryption
- `encrypt()` / `decrypt()` - symmetric encryption (if exists)

## Edge Cases to Test

- Empty strings
- Binary data (null bytes)
- Invalid base64
- Unknown hash algorithms
- Unicode strings

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'string_hash("md5", "hello")'
./toast_oracle.exe 'encode_base64("hello")'
./toast_oracle.exe 'decode_base64("aGVsbG8=")'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return string_hash(\"md5\", \"hello\");"

# Check conformance tests
grep -r "string_hash\|encode_base64\|decode_base64" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-crypto.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- These may be ToastStunt-only extensions
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
