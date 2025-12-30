# Task: Fix anonymous replace_parent corruption bug

## Context
Barn is a Go implementation of a MOO server. There's a failing conformance test related to anonymous object creation and parent replacement.

## Failing Test
Test: `replace_parent_does_not_corrupt_anonymous`
File: `cow_py/tests/conformance/language/anonymous.yaml`

```yaml
- name: replace_parent_does_not_corrupt_anonymous
  permission: wizard
  description: "Replacing parent object doesn't corrupt anonymous child state"
  statement: |
    p = create($nothing);
    a = create(p, 1);
    return typeof(a) == OBJ;
  expect:
    value: 1
```

## Error
Expected: `1` (true)
Got: `0` (false)

## Analysis Needed
1. Check how `create(parent, 1)` works - the second argument `1` indicates anonymous object creation
2. Check what `typeof(a)` returns for anonymous objects
3. Verify anonymous objects have type OBJ

## Key Files
- `barn/builtins/objects.go` - contains create() builtin
- `barn/builtins/types.go` - contains typeof() builtin
- `barn/types/obj.go` - object type definitions
- `barn/db/object.go` - object storage

## Test Commands
```bash
# Build and start server
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9500 > server_9500.log 2>&1 &

# Test manually
./moo_client.exe -port 9500 -timeout 3 -cmd "connect wizard" -cmd "; p = create(\$nothing); a = create(p, 1); return typeof(a);"

# Run conformance test
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9500 -v -k "replace_parent_does_not_corrupt_anonymous"
```

## Output
Write findings and fix to `./reports/fix-anon-replace-parent.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
