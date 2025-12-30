# Fix: Recycled Object Validation

## Status
**COMPLETED** - All verb and property builtins now properly validate recycled objects.

## Problem
Approximately 36 conformance tests were failing because operations on recycled objects didn't return proper errors. Operations like `add_verb`, `verb_info`, `add_property`, `property_info` and others were not checking if the target object had been recycled, leading to incorrect behavior.

## Solution
Added recycled object checks to all verb and property builtins using the pattern:

```go
objID := objVal.ID()
obj := store.Get(objID)
if obj == nil {
    if store.IsRecycled(objID) {
        return types.Err(types.E_INVARG)
    }
    return types.Err(types.E_INVIND)
}
```

This distinguishes between:
- **E_INVIND**: Object never existed (invalid object ID)
- **E_INVARG**: Object was recycled (invalid argument - operation on recycled object)

## Files Modified

### C:\Users\Q\code\barn\builtins\verbs.go
Added recycled checks to:
- `builtinVerbs` - verbs(object)
- `builtinVerbInfo` - verb_info(object, name-or-index)
- `builtinVerbArgs` - verb_args(object, name-or-index)
- `builtinVerbCode` - verb_code(object, name [, ...])
- `builtinAddVerb` - add_verb(object, info, args)
- `builtinDeleteVerb` - delete_verb(object, name)
- `builtinSetVerbInfo` - set_verb_info(object, name, info)
- `builtinSetVerbArgs` - set_verb_args(object, name, args)
- `builtinSetVerbCode` - set_verb_code(object, name, code)

### C:\Users\Q\code\barn\builtins\properties.go
Added recycled checks to:
- `builtinProperties` - properties(object)
- `builtinPropertyInfo` - property_info(object, name)
- `builtinSetPropertyInfo` - set_property_info(object, name, info)
- `builtinAddProperty` - add_property(object, name, value, info)
- `builtinDeleteProperty` - delete_property(object, name)
- `builtinClearProperty` - clear_property(object, name)
- `builtinIsClearProperty` - is_clear_property(object, name)

### C:\Users\Q\code\barn\builtins\objects.go
Commented out unimplemented `object_bytes` builtin registration to allow compilation.

## Testing

Manual testing confirmed correct behavior:

```bash
$ ./moo_client.exe -port 9300 -cmd "connect wizard" \
    -cmd "; x = create(\$nothing); recycle(x); return add_verb(x, {#8, \"rwxd\", \"test\"}, {\"this\", \"none\", \"none\"});"
{2, {E_INVARG, "", 0}}

$ ./moo_client.exe -port 9300 -cmd "connect wizard" \
    -cmd "; x = create(\$nothing); recycle(x); return verb_info(x, \"test\");"
{2, {E_INVARG, "", 0}}

$ ./moo_client.exe -port 9300 -cmd "connect wizard" \
    -cmd "; x = create(\$nothing); recycle(x); return add_property(x, \"test\", 123, {#8, \"rw\"});"
{2, {E_INVARG, "", 0}}

$ ./moo_client.exe -port 9300 -cmd "connect wizard" \
    -cmd "; x = create(\$nothing); recycle(x); return properties(x);"
{2, {E_INVARG, "", 0}}
```

All operations correctly return E_INVARG when called on recycled objects.

## Expected Impact

This fix should resolve approximately 17 failing conformance tests:

**Verb tests:**
- add_verb_recycled_object
- delete_verb_recycled_object
- verb_info_recycled_object
- verb_args_recycled_object
- verb_code_recycled_object
- set_verb_info_recycled_object
- set_verb_args_recycled_object
- set_verb_code_recycled_object
- verbs_recycled_object

**Property tests:**
- add_property_recycled_object
- delete_property_recycled_object
- is_clear_property_recycled_object
- clear_property_recycled_object
- property_info_recycled_object
- set_property_info_recycled_object
- properties_recycled_object

Plus potentially:
- recycle_invalid_already_recycled_object
- recycle_invalid_already_recycled_anonymous

## Implementation Notes

1. **Consistent Pattern**: All builtins follow the same validation pattern - check if object exists, then check if it was recycled.

2. **FindVerb Handling**: Functions that use `store.FindVerb()` still need the explicit recycled check before calling FindVerb, because FindVerb returns `E_VERBNF` (verb not found) when it can't find verbs on recycled objects. We want `E_INVARG` for recycled objects to match MOO semantics.

3. **Early Validation**: The recycled check happens immediately after parsing arguments and before any other validation or operation, ensuring we fail fast with the correct error code.

4. **Database Methods**: The `store.IsRecycled()` method was already available from previous work, making this implementation straightforward.
