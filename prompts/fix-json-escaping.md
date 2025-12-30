# Task: Fix JSON Tab Escaping and Anonymous Object Serialization

## Context
4 tests failing for JSON operations: tab escaping and anonymous object serialization.

## Objective
Fix generate_json() to properly handle tabs and anonymous objects.

## Failing Tests

- generate_json_escape_tab - Tab character should be escaped as \t
- generate_json_anon_obj - Anonymous object serialization
- generate_json_anon_obj_common - Anonymous object (common mode)
- generate_json_anon_obj_embedded - Anonymous object (embedded mode)

## Investigation

1. Find JSON builtins:
```bash
grep -rn "generate_json\|builtinGenerateJSON" /c/Users/Q/code/barn/builtins/
```

2. Run failing test to see error:
```bash
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_json.py::test_generate_json_escape_tab --transport socket --moo-port 9300 -v -s
```

3. Test manually:
```bash
./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return generate_json(\"a\tb\");"
# Should produce "a\tb" (with escaped tab)
```

## Expected Behavior

- Tab character (\t, ASCII 9) should be escaped as `\t` in JSON output
- Anonymous objects should serialize to some representation (check ToastStunt)

## Verification

```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
# restart server

cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_json.py --transport socket --moo-port 9300 -v
```

## After Fix Verified

Commit:
```bash
git add builtins/*.go
git commit -m "Fix JSON tab escaping and anonymous object serialization

- Properly escape tab characters as \\t in generate_json output
- Handle anonymous objects in JSON serialization"
```

## Output
Write status to `./reports/fix-json-escaping.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
5. If all formats fail, STOP and report - do not use bash workarounds
