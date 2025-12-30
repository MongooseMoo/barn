# Task: Implement Waif Support

## Overview
Implement `new_waif()` builtin and waif property/builtin restrictions.

## 1. Update WaifValue in types/waif.go

Add `owner` field:

```go
type WaifValue struct {
    class      ObjID
    owner      ObjID             // NEW: owner of the waif
    properties map[string]Value
}

func NewWaif(class ObjID, owner ObjID) WaifValue {
    return WaifValue{
        class:      class,
        owner:      owner,
        properties: make(map[string]Value),
    }
}

func (w WaifValue) Owner() ObjID {
    return w.owner
}
```

## 2. Implement new_waif() Builtin

**File:** `builtins/objects.go`

```go
func builtinNewWaif(ctx *types.TaskContext, args []types.Value) types.Result {
    if len(args) != 0 {
        return types.Err(types.E_ARGS)
    }

    // Get caller (the object whose verb called new_waif)
    caller := ctx.GetCaller()
    if caller == nil {
        return types.Err(types.E_INVIND)
    }

    // Caller must be a valid object, not anonymous
    callerObj, ok := caller.(types.ObjValue)
    if !ok {
        return types.Err(types.E_INVIND)
    }

    classID := callerObj.Value()

    // Check if class is anonymous (negative ID)
    if classID < 0 {
        return types.Err(types.E_INVARG)
    }

    // Check if class object is valid
    if ctx.Store != nil && ctx.Store.Get(classID) == nil {
        return types.Err(types.E_INVIND)
    }

    // Owner is the programmer (task permissions)
    owner := ctx.Programmer

    // Create the waif
    waif := types.NewWaif(classID, owner)
    return types.Ok(waif)
}
```

**Register in `builtins/registry.go`:**
```go
r.Register("new_waif", builtinNewWaif)
```

## 3. Add Waif Property Restrictions in vm/properties.go

In the property SET code, add waif restrictions:

```go
// For waif property set:
if waifVal, ok := target.(types.WaifValue); ok {
    propName := strings.ToLower(name)
    // These properties cannot be set on waifs at all
    if propName == "owner" || propName == "class" ||
       propName == "wizard" || propName == "programmer" {
        return types.Err(types.E_PERM)
    }
    // Check for self-reference
    if containsWaif(value, waifVal) {
        return types.Err(types.E_RECMOVE)
    }
}
```

For property GET on waifs:
- `.owner` - returns waif.Owner()
- `.class` - returns waif.Class()
- Other properties: look up in waif.properties, fall back to class object

## 4. Add Waif Checks in Builtins

### valid() in builtins/objects.go
```go
// Waifs are never valid
if _, ok := args[0].(types.WaifValue); ok {
    return types.Ok(types.NewInt(0))
}
```

### parents() in builtins/objects.go
```go
// Waifs have no parents
if _, ok := args[0].(types.WaifValue); ok {
    return types.Err(types.E_INVARG)
}
```

### children() in builtins/objects.go
```go
// Waifs have no children
if _, ok := args[0].(types.WaifValue); ok {
    return types.Err(types.E_INVARG)
}
```

### is_player() in builtins/network.go or objects.go
```go
// Waifs can't be players
if _, ok := args[0].(types.WaifValue); ok {
    return types.Err(types.E_TYPE)
}
```

### set_player_flag() in builtins/network.go or objects.go
```go
// Waifs can't have player flag
if _, ok := args[0].(types.WaifValue); ok {
    return types.Err(types.E_TYPE)
}
```

## 5. Self-Reference Check Helper

Add helper to detect circular waif references:

```go
// containsWaif checks if val contains or equals the waif
func containsWaif(val types.Value, waif types.WaifValue) bool {
    switch v := val.(type) {
    case types.WaifValue:
        // Check if same waif instance
        return &v == &waif || v.Equal(waif)
    case types.ListValue:
        for i := 1; i <= v.Len(); i++ {
            if containsWaif(v.Get(i), waif) {
                return true
            }
        }
    case types.MapValue:
        for _, pair := range v.Pairs() {
            if containsWaif(pair[0], waif) || containsWaif(pair[1], waif) {
                return true
            }
        }
    }
    return false
}
```

## 6. Waif Verb Calls

When calling a verb on a waif, look up the verb on the class object:

```go
// In verb call handling:
if waifVal, ok := target.(types.WaifValue); ok {
    // Look up verb on class object
    classID := waifVal.Class()
    // Call verb on class with waif as `this`
}
```

## Test Command

```bash
cd ~/code/barn && go build -o barn_test.exe ./cmd/barn/
taskkill //F //IM barn_test.exe 2>&1 || true
cp ~/src/toaststunt/test/Test.db ./Test.db
./barn_test.exe -db Test.db -port 9330 > server_9330.log 2>&1 &
sleep 2
cd ~/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9330 -k "waif" -v
```

## Expected Test Results

Tests from waif.yaml that should pass:
- `waifs_are_invalid` - valid() returns 0
- `waifs_have_no_parents` - parents() returns E_INVARG
- `waifs_have_no_children` - children() returns E_INVARG
- `waifs_cannot_check_player_flag` - is_player() returns E_TYPE
- `waifs_cannot_set_player_flag` - set_player_flag() returns E_TYPE
- `waif_owner_is_creator` - .owner == player
- `programmer_cannot_change_waif_owner` - E_PERM
- `wizard_cannot_change_waif_owner` - E_PERM
- All `.wizard` and `.programmer` set tests - E_PERM
- `waifs_cant_reference_each_other` - E_RECMOVE
- `anon_cant_be_waif_parent` - E_INVARG

## Output
Write progress to `./reports/implement-waifs.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write

## Priority Order
1. Update WaifValue with owner field
2. Implement new_waif() builtin
3. Add waif checks in valid(), parents(), children(), is_player(), set_player_flag()
4. Add waif property restrictions (.owner, .class, .wizard, .programmer)
5. Add self-reference check (E_RECMOVE)
