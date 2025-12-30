# Task: Fix crypt() to Match ToastStunt Behavior

## Context
Barn is a Go MOO server. The `crypt()` builtin returns different hashes than ToastStunt. This is CRITICAL because old databases store password hashes using crypt() - if we can't match the algorithm, users can't log in.

## The Bug
```
Test 'crypt_hashes_string' expected value 'SAEmC5UwrAl2A', but got 'SAT0ivJegCg8T'
```

Both start with "SA" (the salt), but produce different hashes for the same input.

## What to Investigate

1. **Check ToastStunt's crypt implementation**
   - Location: `~/src/toaststunt/`
   - Look for crypt function in the source
   - Understand what algorithm it uses (DES-based Unix crypt? Something else?)

2. **Check barn's current crypt implementation**
   - Location: `builtins/crypto.go` or similar
   - What algorithm are we using?

3. **Check the test to understand expected behavior**
   - Location: `~/code/cow_py/tests/conformance/basic/string.yaml`
   - What input produces 'SAEmC5UwrAl2A'?

## MOO crypt() Semantics
Standard MOO crypt() is typically:
- `crypt(text)` - generate random salt, hash text
- `crypt(text, salt)` - use given salt (first 2 chars), hash text
- Returns 13-char string: 2-char salt + 11-char hash

The traditional algorithm is DES-based Unix crypt (not modern bcrypt/SHA).

## Test Command
```bash
# After fixing:
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9270 &
sleep 2
# Run whatever the test expects
printf 'connect wizard\n; return crypt("test", "SA");\n' | nc -w 3 localhost 9270
```

## Output
Write findings and fix to `./reports/fix-crypt-compat.md`

## CRITICAL: Do NOT modify tests
The tests match ToastStunt behavior. Only fix barn implementation.

## CRITICAL: File Modified Error Workaround
If Edit/Write fails:
1. Read the file again
2. Retry the Edit
3. Try path formats: `./builtins/crypto.go`, `C:/Users/Q/code/barn/builtins/crypto.go`
4. NEVER use cat, sed, echo
