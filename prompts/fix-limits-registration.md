# Task: Register Missing Builtins for Limits

## Context
Investigation found that 39 limits tests fail because three builtins are implemented but not registered:
1. `load_server_options()` - RegisterSystemBuiltins() is commented out
2. `value_bytes()` - implemented in builtins/limits.go but not registered
3. `substitute()` - implemented in builtins/strings.go but not registered

## Objective
Register these three builtins so limits tests can run.

## Files to Modify

### 1. vm/eval.go
Uncomment the RegisterSystemBuiltins calls in all four evaluator constructors (lines ~26, 46, 65, 84).

Look for lines like:
```go
// TODO: Implement when needed
// registry.RegisterSystemBuiltins(store)
```

Change to:
```go
registry.RegisterSystemBuiltins(store)
```

### 2. builtins/registry.go
Register `value_bytes` and `substitute`:

Find where system builtins are registered (near `server_version`, `memory_usage`, etc.) and add:
```go
r.Register("value_bytes", builtinValueBytes)
```

Find where string builtins are registered (near `strsub`, `index`, etc.) and add:
```go
r.Register("substitute", builtinSubstitute)
```

## Verification

After changes, test manually:
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
# Restart server if needed
./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return load_server_options();"
# Should return a map, not E_VERBNF

./moo_client.exe -port 9300 -cmd "; return value_bytes({1, 2, 3});"
# Should return an integer, not E_VERBNF

./moo_client.exe -port 9300 -cmd "; return substitute(\"%1\", {{1, 2, 3, \"hello\"}});"
# Should return "hello", not E_VERBNF
```

Then run some limits tests:
```bash
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_limits.py::test_string_concat_limit --transport socket --moo-port 9300 -v
```

## After Fix Verified
Stage and commit the changes:
```bash
git add vm/eval.go builtins/registry.go
git commit -m "Register load_server_options, value_bytes, substitute builtins

These builtins were implemented but not registered in the builtin
registry, causing all 39 limits tests to fail with E_VERBNF."
```

## Output
Write status to `./reports/fix-limits-registration.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
5. If all formats fail, STOP and report - do not use bash workarounds
