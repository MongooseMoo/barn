# Database Reader Fix Report

## Problem

The `connect wizard` command failed because `$player_db:find_exact` was not found. When inspecting #39 (player_db), it showed no parent, even though it should inherit from #37 (Generic Database) which has `find_exact`.

## Root Cause

The database reader was incorrectly parsing parent relationships in **version 17** databases.

In the v17 format, the `parents` field is read as a MOO **Value**, which can be either:
1. A single `ObjValue` (the common case - single parent)
2. A `ListValue` (for multiple inheritance)

Our Go code in `db/reader.go` (lines 691-702) only handled case #2 (list of parents). When an object had a single parent, the value was an `ObjValue`, not a `ListValue`, so the parent was silently skipped.

## Reference Implementation Analysis

### Python (lambdamoo-db-py)

The Python reference implementation at `~/src/lambdamoo-db-py/lambdamoo_db/reader.py` lines 266-268:

```python
parents = self.readValue(db)
if not isinstance(parents, list):
    parents = [parents]
```

This correctly handles both cases by converting a single parent to a list.

### C++ (ToastStunt)

The C++ implementation uses a similar approach - the parent field can be either a single object or a list.

## Fix Applied

Modified `db/reader.go` lines 691-709 to handle both cases:

```go
// Read parents
parentsVal, err := readValue(r, db.Version)
if err != nil {
    return nil, err
}
// Parents can be either a single object or a list of objects
if listVal, ok := parentsVal.(types.ListValue); ok {
    // Multiple parents (list)
    for i := 1; i <= listVal.Len(); i++ {
        if objVal, ok := listVal.Get(i).(types.ObjValue); ok {
            obj.Parents = append(obj.Parents, objVal.ID())
        }
    }
} else if objVal, ok := parentsVal.(types.ObjValue); ok {
    // Single parent (common case)
    if objVal.ID() != -1 {
        obj.Parents = append(obj.Parents, objVal.ID())
    }
}
```

## Verification

### Before Fix
```
$ ./barn.exe -db toastcore.db -eval 'parent(#39)'
=> #-1
```

### After Fix
```
$ ./barn.exe -db toastcore.db -eval 'parent(#39)'
=> #37
```

### Verb Lookup Test
```
$ ./barn.exe -db toastcore.db -verb-lookup "#39:find_exact"
=== Verb Lookup: #39:find_exact ===

Starting object: #39 (Player Database)

Result: FOUND on #37
  (inherited from parent)

Inheritance chain:
#39 (Player Database)
  #37 (Generic Database) *** VERB DEFINED HERE ***

Verb details:
  Name:    find_exact
  Names:   find_exact
  Owner:   #36
  Perms:   rxd
  ArgSpec: this none this
  Code:    14 lines
```

## Tests Added

Added comprehensive tests to `db/reader_test.go`:

1. **TestParentParsing** - Verifies #39 has parent #37, #37 has parent #1, #1 has no parent
2. **TestVerbCount** - Verifies #37 has verbs including `find_exact`
3. **TestVerbInheritance** - Verifies #39 can find inherited verb `find_exact` from #37

All tests pass:
```
$ go test ./db -v
=== RUN   TestLoadDatabase
--- PASS: TestLoadDatabase (0.01s)
=== RUN   TestParentParsing
--- PASS: TestParentParsing (0.01s)
=== RUN   TestVerbCount
--- PASS: TestVerbCount (0.01s)
=== RUN   TestVerbInheritance
--- PASS: TestVerbInheritance (0.01s)
[... other tests ...]
PASS
ok  	barn/db	0.290s
```

## CLI Tooling Added

Added three new CLI flags to `cmd/barn/main.go` for database inspection:

### 1. `-dump-obj-raw "#N"` - Raw Database Fields

Dumps raw object data for debugging:
```
$ ./barn.exe -db toastcore.db -dump-obj-raw "#39"
=== Raw Object Data #39 ===
ID:         39
Name:       "Player Database"
Owner:      #36
Location:   #-1
Flags:      0x10 (16)
Anonymous:  false

Parents:    [#37] (count=1)
Children:   [] (count=0)
Contents:   [] (count=0)

VerbList:   7 verbs
  [0] "load" (names=1, owner=#36, code=39 lines)
  ...
```

### 2. `-verb-lookup "#N:verbname"` - Verb Inheritance Lookup

Shows where a verb is found in the parent chain:
```
$ ./barn.exe -db toastcore.db -verb-lookup "#39:find_exact"
=== Verb Lookup: #39:find_exact ===

Starting object: #39 (Player Database)

Result: FOUND on #37
  (inherited from parent)

Inheritance chain:
#39 (Player Database)
  #37 (Generic Database) *** VERB DEFINED HERE ***
```

### 3. `-ancestry "#N"` - Full Parent Chain

Shows the complete ancestry tree:
```
$ ./barn.exe -db toastcore.db -ancestry "#39"
=== Ancestry for #39 (Player Database) ===

#39 - Player Database
       owner=#36, verbs=7, props=14
  #37 - Generic Database
         owner=#36, verbs=20, props=7
    #1 - Root Class
           owner=#2, verbs=37, props=4
           (root object - no parent)

Total depth: 2
```

## Impact

This fix affects **all v17 databases** and is critical for proper object inheritance. Without this fix:
- Single-parent objects (the vast majority) appear to have no parent
- Verb inheritance breaks completely
- Property inheritance breaks completely
- The database is essentially unusable

The fix is minimal, safe, and follows the reference implementations exactly.

## Database Format Notes

For future reference:

### v4 Format (older)
- Parent is a simple integer field
- Always single parent
- Has a blank line after object name

### v17 Format (current)
- Parent is a Value (can be OBJ or LIST)
- Supports multiple inheritance
- No blank line after object name
- Location, contents, parents, children are all Values

Our v4 parsing was already correct - this issue only affected v17 databases.
