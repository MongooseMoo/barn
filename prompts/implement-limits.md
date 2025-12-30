# Task: Implement MOO Value Size Limits

## Overview
Implement `value_bytes()` builtin and add limit checking to list/map/string operations.

## 1. Implement `value_bytes(value)` Builtin

**File:** `builtins/limits.go`

Add function to calculate byte size of any MOO value:

```go
func builtinValueBytes(ctx *types.TaskContext, args []types.Value) types.Result {
    if len(args) != 1 {
        return types.Err(types.E_ARGS)
    }
    size := ValueBytes(args[0])
    return types.Ok(types.NewInt(int64(size)))
}

func ValueBytes(v types.Value) int {
    base := 8 // sizeof pointer/interface
    switch val := v.(type) {
    case types.IntValue:
        return base + 8
    case types.FloatValue:
        return base + 8
    case types.StrValue:
        return base + len(val.Value()) + 1
    case types.ObjValue:
        return base + 8
    case types.ErrValue:
        return base + 4
    case types.ListValue:
        size := base + 8 // list header
        for i := 1; i <= val.Len(); i++ {
            size += ValueBytes(val.Get(i))
        }
        return size
    case types.MapValue:
        size := base + 8 // map header
        for _, pair := range val.Pairs() {
            size += ValueBytes(pair[0]) + ValueBytes(pair[1])
        }
        return size
    case types.WaifValue:
        // Waif size = base + class ref + property values
        size := base + 16
        // Add property values if accessible
        return size
    default:
        return base
    }
}
```

**Register in `builtins/registry.go`:**
```go
r.Register("value_bytes", builtinValueBytes)
```

## 2. Add Limit Checking Helper

**File:** `builtins/limits.go`

Add helpers to check limits and return E_QUOTA:

```go
func GetMaxListValueBytes() int {
    serverOptionsCache.RLock()
    defer serverOptionsCache.RUnlock()
    if serverOptionsCache.maxListValueBytes > 0 {
        return serverOptionsCache.maxListValueBytes
    }
    return 0 // 0 means unlimited
}

func GetMaxMapValueBytes() int {
    serverOptionsCache.RLock()
    defer serverOptionsCache.RUnlock()
    if serverOptionsCache.maxMapValueBytes > 0 {
        return serverOptionsCache.maxMapValueBytes
    }
    return 0
}

func CheckListLimit(list types.ListValue) types.ErrorCode {
    limit := GetMaxListValueBytes()
    if limit > 0 && ValueBytes(list) > limit {
        return types.E_QUOTA
    }
    return types.E_NONE
}

func CheckMapLimit(m types.MapValue) types.ErrorCode {
    limit := GetMaxMapValueBytes()
    if limit > 0 && ValueBytes(m) > limit {
        return types.E_QUOTA
    }
    return types.E_NONE
}

func CheckStringLimit(s string, ctx *types.TaskContext) types.ErrorCode {
    limit := GetMaxStringConcat()
    if limit > 0 && len(s) > limit {
        return types.E_QUOTA
    }
    return types.E_NONE
}
```

## 3. Add Checks to List Builtins

**File:** `builtins/lists.go`

For each of these functions, add limit check AFTER the operation:

### setadd
```go
// After creating result list:
if err := CheckListLimit(result); err != types.E_NONE {
    return types.Err(err)
}
```

### setremove
Same pattern.

### listinsert
Same pattern.

### listappend
Same pattern.

### listdelete
Same pattern.

### listset
Same pattern.

## 4. Add Checks to VM Operations

**File:** `vm/operators.go` or wherever these operations live

### List literal `{a, b, c}`
After constructing list, check limit.

### List append with `@`
After append operation, check limit.

### Index assignment `list[i] = value`
After assignment, check list limit.

### Range assignment `list[i..j] = value`
After assignment, check list limit.

## 5. Add Checks to Map Operations

### Map literal `["a" -> 1]`
After constructing map, check limit.

### Map index assignment `map[key] = value`
After assignment, check map limit.

### mapdelete
After operation, check limit.

## 6. Add Checks to String Operations

**File:** `builtins/strings.go`

### String concatenation
Check result length against max_string_concat.

### tostr, toliteral
Check result length.

### strsub, substitute
Check result length.

### encode_binary, encode_base64
Check result length.

### random_bytes
Check requested length before generating.

## Test Command
```bash
cd ~/code/barn && go build -o barn_test.exe ./cmd/barn/
taskkill //F //IM barn_test.exe 2>&1 || true
cp ~/src/toaststunt/test/Test.db ./Test.db
./barn_test.exe -db Test.db -port 9320 > server_9320.log 2>&1 &
sleep 2
cd ~/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9320 -k "limits" -v
```

## Output
Write progress to `./reports/implement-limits.md`

## Priority Order
1. `value_bytes()` builtin - tests won't run without it
2. List builtin checks (setadd, listinsert, etc.)
3. VM operation checks (literals, indexset, rangeset)
4. String operation checks

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
