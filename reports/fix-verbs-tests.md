# Fix Verbs Tests Report

## Objective
Fix 10 conformance test failures for verbs builtins (add_verb and verb_args).

## Tests Fixed
All 10 tests now pass:
1. `verbs::add_verb_basic` - add_verb creates new verb that can be called
2. `verbs::add_verb_invalid_owner` - rejects invalid owner
3. `verbs::add_verb_invalid_perms` - rejects invalid perms string
4. `verbs::add_verb_invalid_args` - rejects invalid arg specs
5. `verbs::add_verb_with_write_permission` - allows with write permission
6. `verbs::add_verb_wizard_bypasses_write` - wizard bypasses write check
7. `verbs::add_verb_not_owner` - rejects when caller not owner
8. `verbs::add_verb_is_owner` - allows when caller is owner
9. `verbs::add_verb_wizard_sets_owner` - wizard can set different owner
10. `verbs::verb_args_basic` - returns expanded prep spec

## Root Causes

### 1. add_verb Return Value
**Problem**: add_verb was returning 0 instead of the 1-based verb index.

**ToastStunt Behavior**:
```c
// db_verbs.cc:218-225
if (o->verbdefs) {
    for (v = o->verbdefs, count = 2; v->next; v = v->next, ++count);
    v->next = newv;
} else {
    o->verbdefs = newv;
    count = 1;  // First verb gets index 1
}
return count;
```

**Fix**: Changed return value from `types.NewInt(0)` to `types.NewInt(int64(len(obj.VerbList)))` after appending to VerbList.

### 2. Missing Validation
**Problem**: add_verb wasn't validating:
- Owner is a valid object
- Perms string only contains rwxdRWXD
- Arg specs are "this", "none", or "any"
- Prep spec matches known prepositions

**ToastStunt Behavior**:
```c
// verbs.cc:77-116 - validate_verb_info checks owner valid and perms chars
// verbs.cc:156-172 - validate_verb_args checks arg/prep specs
```

**Fix**: Added validation for all parameters with E_INVARG errors.

### 3. Missing Permission Checks
**Problem**: add_verb had TODO comments but no actual permission checking.

**ToastStunt Behavior**:
```c
// verbs.cc:198-201
if (!db_object_allows(obj, progr, FLAG_WRITE)
    || (progr != owner && !is_wizard(progr))) {
    free_str(names);
    e = E_PERM;
}
```

**Fix**: Added checks for:
- Write permission on object (or wizard)
- Caller must be the owner specified in verbinfo (or wizard)

### 4. Preposition Expansion Missing
**Problem**: verb_args returned abbreviated prep (e.g., "on") instead of full spec (e.g., "on top of/on/onto/upon").

**ToastStunt Behavior**:
```c
// db_verbs.cc:183-191 - db_unparse_prep returns prep_list[prep]
// prep_list[4] = "on top of/on/onto/upon"
```

**Fix**:
- Added prepList array matching ToastStunt's prep_list
- Added unparsePrepSpec() to expand abbreviated preps
- Modified verb_args to call unparsePrepSpec before returning

## Implementation Details

### Preposition List
Added prepList matching ToastStunt exactly:
```go
var prepList = []string{
    "with/using",                          // 0
    "at/to",                               // 1
    "in front of",                         // 2
    "in/inside/into",                      // 3
    "on top of/on/onto/upon",              // 4
    "out of/from inside/from",             // 5
    "over",                                // 6
    "through",                             // 7
    "under/underneath/beneath",            // 8
    "behind",                              // 9
    "beside",                              // 10
    "for/about",                           // 11
    "is",                                  // 12
    "as",                                  // 13
    "off/off of",                          // 14
}
```

### Helper Functions
Added three helper functions:
- `matchArgSpec(s string) bool` - validates "this"/"none"/"any"
- `matchPrepSpec(s string) int` - validates prep and returns index (-1 if invalid)
- `unparsePrepSpec(prepStr string) string` - expands abbreviated prep to full string

### Permission Logic
```go
if !ctx.IsWizard {
    // Check write permission on object
    if !obj.Flags.Has(db.FlagWrite) && obj.Owner != ctx.Player {
        return types.Err(types.E_PERM)
    }
    // Check caller is the owner in verbinfo
    if ownerID != ctx.Player {
        return types.Err(types.E_PERM)
    }
}
```

## Testing

### Test Execution
```bash
cd ~/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9302 &

cd ~/code/cow_py
uv run pytest tests/conformance/ -k "verbs::add_verb_basic or verbs::add_verb_invalid_owner or verbs::add_verb_invalid_perms or verbs::add_verb_invalid_args or verbs::add_verb_with_write_permission or verbs::add_verb_wizard_bypasses_write or verbs::add_verb_not_owner or verbs::add_verb_is_owner or verbs::add_verb_wizard_sets_owner or verbs::verb_args_basic" --transport socket --moo-port 9302 -v
```

### Results
```
10 passed, 1469 deselected in 0.63s
```

All 10 tests now pass on first try.

## Files Modified
- `C:\Users\Q\code\barn\builtins\verbs.go`
  - Added prepList array (15 entries)
  - Added matchArgSpec helper
  - Added matchPrepSpec helper
  - Added unparsePrepSpec helper
  - Modified builtinAddVerb to validate and check permissions
  - Modified builtinVerbArgs to expand prep specs
  - Fixed return value to be 1-based index

## Commit
```
commit 3bb5e68
Fix verb builtins conformance failures
```

## Summary
Fixed all 10 failing conformance tests for verb builtins by:
1. Returning correct 1-based verb index from add_verb
2. Adding comprehensive validation for owner, perms, and arg specs
3. Implementing permission checks matching ToastStunt
4. Expanding preposition specs in verb_args output

The implementation now matches ToastStunt's behavior exactly for these operations.
