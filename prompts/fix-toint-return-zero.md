# Task: Fix toint() to Return 0 for Unparseable Strings

## Context
The `toint()` builtin in Barn currently returns E_INVARG when given a string that can't be parsed as an integer. This is wrong - Toast returns 0.

## Evidence
```bash
# Toast behavior (correct):
./toast_oracle.exe 'toint("[::1]")'
# Returns: 0

# Barn behavior (wrong):
# Returns: E_INVARG
```

## File to Fix
`builtins/types.go` - the `builtinToint` function, around line 110-118

## The Fix
Change this:
```go
if err != nil {
    return types.Err(types.E_INVARG)
}
```

To this:
```go
if err != nil {
    return types.Ok(types.IntValue{Val: 0})
}
```

## Verification
1. Build barn: `go build -o barn_test.exe ./cmd/barn/`
2. Start server: `./barn_test.exe -db toastcore.db -port 9500 > server_9500.log 2>&1 &`
3. Test: `./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return toint(\"[::1]\");"`
4. Expected: `{1, 0}` (not an error)

## Output
Write completion report to `./reports/fix-toint-return-zero.md`
