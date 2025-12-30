# Task: Fix Recycled Object Validation

## Context
~36 tests are failing because operations on recycled objects don't return proper errors. Operations like add_verb, verb_info, add_property, property_info should return E_INVARG when the target object has been recycled.

## Objective
Add recycled object checks to verb and property builtins so they return E_INVARG.

## Failing Tests Pattern

Verbs:
- add_verb_recycled_object
- delete_verb_recycled_object
- verb_info_recycled_object
- verb_args_recycled_object
- verb_code_recycled_object
- set_verb_info_recycled_object
- set_verb_args_recycled_object
- set_verb_code_recycled_object
- verbs_recycled_object

Properties:
- add_property_recycled_object
- delete_property_recycled_object
- is_clear_property_recycled_object
- clear_property_recycled_object
- property_info_recycled_object
- set_property_info_recycled_object
- properties_recycled_object

## Investigation

1. Find where verb/property builtins are implemented:
```bash
grep -rn "builtinAddVerb\|add_verb" /c/Users/Q/code/barn/builtins/
grep -rn "builtinAddProperty\|add_property" /c/Users/Q/code/barn/builtins/
```

2. Check how other builtins handle recycled objects:
```bash
grep -rn "IsRecycled\|recycled" /c/Users/Q/code/barn/
```

3. Test manually what happens with a recycled object:
```bash
./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; x = create(\$nothing); recycle(x); return valid(x);"
```

## Implementation

Each verb/property builtin that takes an object argument should check:
```go
if obj.IsRecycled() {
    return E_INVARG, nil  // or however errors are returned
}
```

This check should happen early, before any other validation.

## Files to Modify

Likely in:
- `builtins/verbs.go` - verb-related builtins
- `builtins/properties.go` - property-related builtins

## Verification

```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
# restart server

cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_verbs.py -k "recycled" --transport socket --moo-port 9300 -v
uv run pytest tests/conformance/test_properties.py -k "recycled" --transport socket --moo-port 9300 -v
```

## After Fix Verified

Commit:
```bash
git add builtins/verbs.go builtins/properties.go
git commit -m "Add recycled object validation to verb/property builtins

Return E_INVARG when operations are attempted on recycled objects.
This affects add_verb, delete_verb, verb_info, verb_args, verb_code,
set_verb_info, set_verb_args, set_verb_code, verbs, add_property,
delete_property, is_clear_property, clear_property, property_info,
set_property_info, and properties builtins."
```

## Output
Write status to `./reports/fix-recycled-object-validation.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
5. If all formats fail, STOP and report - do not use bash workarounds
