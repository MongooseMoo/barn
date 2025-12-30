# Task: Implement String Length Limits

## Context

Barn (Go MOO server) is failing 38 conformance tests in the `limits` category. These tests check that string-producing builtins respect the MAX_STRING_CONCAT limit.

## Test Location

Tests are in: `~/code/cow_py/tests/conformance/builtins/limits.yaml`

## What Needs to Be Done

1. Read the limits.yaml test file to understand what's being tested
2. Find where ToastStunt implements these limits: `~/src/toaststunt/`
3. Implement the limits in barn's builtins

## Key Builtins That Need Limits

Based on test names:
- `tostr` - string conversion
- `toliteral` - literal conversion
- `strsub` - string substitution
- `encode_binary` - binary encoding
- `substitute` - pattern substitution
- `encode_base64` - base64 encoding
- `random_bytes` - random byte generation

## Reference

ToastStunt likely has a `MAX_STRING_CONCAT` or similar constant. Search for it.

cow_py may also have limit implementations in `~/code/cow_py/`

## Test Command

```bash
cd ~/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9302 -k "limits" -v
```

Server should already be running on port 9302.

## Output

Write findings and implementation to `./reports/fix-limits.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
