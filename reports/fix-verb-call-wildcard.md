# Fix Verb Call Wildcard Matching - Task Report

## Summary

Successfully fixed Barn's MOO verb wildcard matching to handle the correct abbreviation semantics. Verb names with wildcards like `get_conj*ugation` now correctly match calls using any abbreviation from the minimum (`get_conj`) up to the full name (`get_conjugation`).

## Problem

MOO verb calls from code (e.g., `this:get_conj()`) were failing with E_VERBNF when the verb was defined with a wildcard pattern like `get_conj*ugation`. The wildcard matching implementation was incorrect.

### Evidence

```bash
./dump_verb.exe 41 get_conj
Verb 'get_conj' not found on #41
Available verbs:
  get_conj*ugation   <-- THIS should match get_conj
```

Calling `this:get_conj(...)` in MOO code would fail, even though the verb `get_conj*ugation` existed.

## Root Cause

The `matchVerbName()` function in `db/store.go` implemented the WRONG wildcard semantics.

**Incorrect implementation** (lines 376-394):
```go
// Split pattern at wildcard: "co*nnect" -> prefix="co", suffix="nnect"
prefix := pattern[:starPos]
suffix := pattern[starPos+1:]

// Check if search string has the required prefix and suffix
if !strings.HasPrefix(search, prefix) {
    return false
}
if !strings.HasSuffix(search, suffix) {
    return false
}

// Ensure prefix and suffix don't overlap
return len(search) >= len(prefix)+len(suffix)
```

This required the search to match BOTH the prefix AND suffix, which meant:
- Pattern `get_conj*ugation` would match `get_conjugation` (has both prefix and suffix)
- Pattern `get_conj*ugation` would NOT match `get_conj` (has prefix but no suffix)

But MOO wildcard semantics are different!

## MOO Wildcard Semantics

In MOO, the `*` marks the **minimum abbreviation point**, not an expansion point.

Pattern `get_conj*ugation`:
- Must type at least `get_conj` (everything before the `*`)
- Can type any prefix of the full name `get_conjugation` (remove the `*`)

**Valid calls:**
- `get_conj` ✓
- `get_conju` ✓
- `get_conjug` ✓
- `get_conjuga` ✓
- `get_conjugat` ✓
- `get_conjugati` ✓
- `get_conjugatio` ✓
- `get_conjugation` ✓

**Invalid calls:**
- `get_con` ✗ (too short - before the `*`)
- `get_conjugate` ✗ (not a prefix of `get_conjugation`)

## Solution

Fixed `matchVerbName()` to implement correct MOO semantics:

```go
func matchVerbName(verbPattern, searchName string) bool {
	// Case-insensitive matching
	pattern := strings.ToLower(verbPattern)
	search := strings.ToLower(searchName)

	// Find the wildcard position
	starPos := strings.Index(pattern, "*")
	if starPos == -1 {
		// No wildcard, exact match required
		return pattern == search
	}

	// MOO wildcard semantics:
	// Pattern "get_conj*ugation" matches any search that:
	// 1. Starts with the prefix "get_conj" (required minimum)
	// 2. Is a prefix of the full name "get_conjugation" (remove the *)

	prefix := pattern[:starPos]              // "get_conj" - required minimum
	full := pattern[:starPos] + pattern[starPos+1:] // "get_conjugation" - full name

	// Search must start with the required prefix
	if !strings.HasPrefix(search, prefix) {
		return false
	}

	// Search must be a prefix of the full name
	return strings.HasPrefix(full, search)
}
```

**Key changes:**
1. Construct the full verb name by removing the `*`
2. Check that search starts with the minimum prefix (before `*`)
3. Check that search is a valid prefix of the full name

## Testing

### Unit Tests

Created comprehensive unit test for wildcard matching:

```bash
$ go run test_wildcard_match.go
Testing verb wildcard matching for get_conj*ugation:

[PASS] Minimum abbreviation: 'get_conj' should match -> matched (found on #41: get_conj*ugation)
[PASS] Partial expansion 1: 'get_conju' should match -> matched (found on #41: get_conj*ugation)
[PASS] Partial expansion 2: 'get_conjug' should match -> matched (found on #41: get_conj*ugation)
[PASS] Partial expansion 3: 'get_conjuga' should match -> matched (found on #41: get_conj*ugation)
[PASS] Full expansion: 'get_conjugation' should match -> matched (found on #41: get_conj*ugation)
[PASS] Too short: 'get_con' should NOT match -> did NOT match
[PASS] Wrong name: 'get_conjugate' should NOT match -> did NOT match
[PASS] Case insensitive: 'GET_CONJ' should match -> matched (found on #41: get_conj*ugation)
[PASS] Case insensitive full: 'Get_Conjugation' should match -> matched (found on #41: get_conj*ugation)

Results: 9 passed, 0 failed

SUCCESS - All tests passed!
```

### Integration Tests

### Test 1: Minimum abbreviation (get_conj)
```bash
$ ./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return #41:get_conj(\"is/are\");"
{1, "is"}
```
✓ Works with minimum abbreviation

### Test 2: Partial expansion (get_conju)
```bash
$ ./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return #41:get_conju(\"is/are\");"
{1, "is"}
```
✓ Works with partial expansion

### Test 3: Full expansion (get_conjugation)
```bash
$ ./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return #41:get_conjugation(\"is/are\");"
{1, "is"}
```
✓ Works with full name

### Test 4: Too short (get_con)
```bash
$ ./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return #41:get_con(\"is/are\");"
{2, {E_VERBNF, "", 0}}
```
✓ Correctly fails for abbreviation shorter than minimum

### Test 5: Command from MOO code (look me)
```bash
$ ./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "look me"
Wizard
You see a wizard who chooses not to reveal its true appearance.
It is awake and looks alert.
```
✓ The `look me` command works, which internally calls `#41:get_conj()` from `#2:look_self`

### Test 6: Command parsing (l for l*ook)
```bash
$ ./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "l"
The First Room
This is all there is right now.
```
✓ Command parsing still works (uses same wildcard matching)

### Verification Against Toast

Toast (reference implementation) behavior confirmed:
```bash
$ ./moo_client.exe -port 7777 -cmd "connect wizard" -cmd "; return #41:get_conj(\"is/are\");"
=> "is"

$ ./moo_client.exe -port 7777 -cmd "connect wizard" -cmd "; return #41:get_conjugation(\"is/are\");"
=> "is"
```

Both minimum and full names work in Toast, confirming our fix is correct.

## Files Modified

- `C:\Users\Q\code\barn\db\store.go`
  - Fixed `matchVerbName()` function (lines 364-394)
  - Updated documentation comments (lines 358-363)

## Reference Implementation

The fix was based on cow_py's implementation in `src/cow_py/server_builtins.py`:

```python
def _verb_name_matches(self, search_name: str, verb_pattern: str) -> bool:
    search = search_name.lower()
    for alias in verb_pattern.split():
        if alias.startswith('@'):
            alias = alias[1:]
        alias_lower = alias.lower()

        if '*' in alias_lower:
            star_pos = alias_lower.index('*')
            prefix = alias_lower[:star_pos]  # Required minimum
            full = alias_lower.replace('*', '')  # Full name without *

            # Match if search starts with prefix and is a prefix of full
            if search.startswith(prefix) and full.startswith(search):
                return True
```

## Impact

This fix resolves verb call failures throughout the toastcore database. Any MOO code that calls verbs using abbreviated names (which is common practice) will now work correctly.

**Examples of affected code patterns:**
- `this:get_conj()` in `#2:look_self`
- Any verb calls using standard MOO abbreviation patterns
- User commands that rely on abbreviated verb names

## Status

✓ Task completed successfully
✓ All test cases passing
✓ Matches Toast/LambdaMOO behavior
✓ Command parsing unaffected
✓ Verb calls from MOO code now work correctly
