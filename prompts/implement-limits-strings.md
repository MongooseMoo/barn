# Task: Complete String Limits Implementation

## Overview
The core limits infrastructure is done (value_bytes, CheckListLimit, CheckMapLimit, CheckStringLimit).
Now add string limit checks to remaining string operations.

## Failing Tests
- `substitute_limit` - substitute() needs string limit check
- `substitute_exceeds_limit` - substitute() needs string limit check
- `encode_base64_exceeds_limit` - encode_base64() needs string limit check
- `random_bytes_exceeds_limit` - random_bytes() needs to check requested size

## 1. Add Check to substitute() in builtins/strings.go

Find `builtinSubstitute` and add after building result string:
```go
// Check string limit
if err := CheckStringLimit(result); err != types.E_NONE {
    return types.Err(err)
}
```

## 2. Add Check to encode_base64() in builtins/crypto.go

Find `builtinEncodeBase64` and add after encoding:
```go
// Check string limit
if err := CheckStringLimit(encoded); err != types.E_NONE {
    return types.Err(err)
}
```

## 3. Add Check to random_bytes() in builtins/crypto.go

Find `builtinRandomBytes` and add BEFORE generating bytes:
```go
// Check if requested size exceeds limit
limit := GetMaxStringConcat()
if limit > 0 && count > limit {
    return types.Err(types.E_QUOTA)
}
```

## 4. Add Checks to Other String Builtins

Also add checks to:
- `tostr()` - after converting to string
- `toliteral()` - after converting to literal
- `strsub()` - after substitution
- `implode()` - after joining strings

## Test Command

```bash
cd ~/code/barn && go build -o barn_test.exe ./cmd/barn/
taskkill //F //IM barn_test.exe 2>&1 || true
cp ~/src/toaststunt/test/Test.db ./Test.db
./barn_test.exe -db Test.db -port 9390 > server_9390.log 2>&1 &
sleep 2
cd ~/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9390 -k "limits" -v
```

## Output
Write progress to `./reports/implement-limits-strings.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
