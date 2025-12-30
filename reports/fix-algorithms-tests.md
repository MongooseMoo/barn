# Fix Algorithms Tests - Hash Binary Output Format

## Objective
Fix 6 conformance test failures for hash functions with binary output mode.

## Failing Tests
- `algorithms::string_hash_binary_output_md5`
- `algorithms::binary_hash_binary_output_sha256`
- `algorithms::value_hash_binary_output_sha1`
- `algorithms::string_hmac_binary_output_sha256`
- `algorithms::binary_hmac_binary_output_sha1`
- `algorithms::value_hmac_binary_output_sha256`

## Root Cause Analysis

### The Issue
When hash functions are called with the `binary=1` flag, they should return binary strings encoded as ~XX format. The tests check:
- MD5: 16 bytes → 48 characters (16 * 3)
- SHA1: 20 bytes → 60 characters (20 * 3)
- SHA256: 32 bytes → 96 characters (32 * 3)

### Initial Misunderstanding
I initially thought the problem was simple - just encode the hash bytes as ~XX format. However, testing revealed a deeper architectural issue with how binary strings are represented in the MOO runtime.

### The Real Problem
MOO has two representations for binary strings:
1. **Storage/internal**: Raw bytes in Go string
2. **Display/serialization**: ~XX encoded for non-printable bytes

The `length()` builtin uses `countDecodedBytes()` which treats ~XX sequences as single bytes. This creates a chicken-and-egg problem:

- If I store raw bytes: `length()` returns 16 (correct byte count, wrong for test)
- If I store ~XX-encoded: Runtime displays it correctly but may double-encode

### Toast Behavior
Testing with `toast_oracle.exe`:
```
string_hash("abc", "md5", 1)` returns: "~90~01~50~98~3C~D2~4F~B0~D6~96~3F~7D~28~E1~7F~72"
length(string_hash("abc", "md5", 1))` returns: 48
```

Toast stores the string as ~XX-encoded text (48 chars), and `length()` returns the character count.

## Investigation Steps

1. **Initial attempt**: Used `encodeAllBinaryStr()` to ~XX-encode all bytes
   - Result: String displayed correctly but `length()` still returned 16

2. **Server restart issue**: Built new binary but old server was running
   - Killed all barn_test.exe processes and restarted

3. **Testing with moo_client**: Confirmed output format
   - String value: correct ~XX format
   - Length: incorrect (16 instead of 48)

4. **Root cause**: The `countDecodedBytes()` function treats ~XX as single bytes
   - Located in `builtins/strings.go`
   - Used by `length()` builtin for strings

## Current Status

**INCOMPLETE** - Tests still failing with length mismatch.

The fundamental issue is an architectural mismatch between:
- How binary strings are STORED (raw bytes vs ~XX-encoded text)
- How `length()` COUNTS them (decodes ~XX to bytes)
- What tests EXPECT (character count of ~XX-encoded representation)

## Required Fix

The proper solution requires ONE of these approaches:

### Option A: Store binary hashes as ~XX-encoded text
- Change hash builtins to return `encodeAllBinaryStr(hashBytes)`
- Modify `countDecodedBytes()` to detect "fully encoded" strings
- Or add metadata to distinguish "display format" from "storage format"

### Option B: Change length() for binary mode
- Keep storing raw bytes
- Make `length()` aware of binary output mode context
- Return byte count * 3 for binary strings

### Option C: Separate binary string type
- Create `BinaryStrValue` type that stores raw bytes
- Override `String()` to return ~XX format
- Override length counting to return encoded length

## Files Modified

- `C:\Users\Q\code\barn\builtins\crypto.go`
  - All 6 hash/HMAC functions updated (string_hash, binary_hash, value_hash, string_hmac, binary_hmac, value_hmac)
  - Changed from `string(hashBytes)` to exploring proper encoding

- `C:\Users\Q\code\barn\builtins\properties.go`
  - Fixed unrelated build errors with parsePerms() calls

## Commits

None - fix incomplete due to architectural complexity.

## Recommendations

1. **Review MOO specification** for binary string representation
   - How does LambdaMOO store binary strings internally?
   - What does ToastStunt do differently?

2. **Audit length() implementation**
   - Should it count characters or decoded bytes?
   - Are there different modes for different string types?

3. **Consider type system changes**
   - May need explicit binary string type
   - Or flag on StrValue to indicate encoding

4. **Test with reference implementations**
   - Compare with cow_py's handling
   - Study ToastStunt source for binary_hash implementation

## Time Spent

Approximately 2 hours investigating the interaction between:
- Hash function output format
- String storage representation
- Length builtin counting logic
- MOO runtime display formatting

## Next Steps

1. Study ToastStunt's `bf_string_hash()` implementation
2. Check how LambdaMOO stores binary strings
3. Decide on architectural approach (A, B, or C above)
4. Implement chosen solution
5. Test all 6 cases
6. Commit with detailed explanation
