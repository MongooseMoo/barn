# Task: Implement Pure Go DES Crypt

## Context
Barn needs `crypt()` to work on ALL platforms (including Windows) for password verification when loading old MOO databases. The current implementation returns E_INVARG on Windows.

## Requirements

1. Must produce IDENTICAL output to Unix `crypt(3)` with 2-char salt
2. Must work on Windows (no CGO dependency)
3. Must pass: `crypt("foobar", "SA")` → `"SAEmC5UwrAl2A"`

## The Algorithm

Traditional Unix DES crypt:
1. Takes 8-char password (null-padded or truncated)
2. Uses 2-char salt to modify DES S-boxes
3. Encrypts a constant string (all zeros) 25 times using modified DES
4. Encodes result in base64-like format (./0-9A-Za-z)

## Resources

Look for existing pure Go implementations:
- `golang.org/x/crypto` may have something
- Search for "descrypt go" or "unix crypt go"
- Check ToastStunt source for algorithm details: `~/src/toaststunt/`

If no library exists, the algorithm is well-documented. Key reference:
- https://en.wikipedia.org/wiki/Crypt_(C)
- The salt modifies the DES E-table expansion

## Files to Modify

- `builtins/crypto_windows.go` - Replace stub with real implementation
- OR create `builtins/descrypt.go` with pure Go implementation used by all platforms

## Test Command
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9275 &
sleep 2
printf 'connect wizard\n; return crypt("foobar", "SA");\n' | nc -w 3 localhost 9275
```

Expected: `{1, "SAEmC5UwrAl2A"}`

## Output
Write findings to `./reports/fix-crypt-pure-go.md`

## CRITICAL
- Must work on Windows without CGO
- Must produce EXACT same output as ToastStunt/Unix crypt
- This is blocking for database compatibility
