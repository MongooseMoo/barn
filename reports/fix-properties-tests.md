# Fix Properties Tests - Partial Completion Report

## Objective
Fix 12 conformance test failures in the properties category (`add_property`, `is_clear_property`, `clear_property`).

## Status: Partial Success (6/11 tests passing)

### Tests Fixed (6 passing)
1. **add_property_invalid_perms** - Now validates permission strings contain only 'r', 'w', 'c' characters
2. **add_property_builtin_name** - Rejects built-in property names (name, owner, location, etc.)
3. **add_property_defined_on_descendant** - Checks if property exists in any descendant object
4. **is_clear_property_builtin** - Returns 0 for built-in properties instead of E_PROPNF
5. **clear_property_builtin** - Returns E_PERM for built-in properties
6. **clear_property_on_definer** - Returns E_INVARG when called on the property definer

### Tests Still Failing (5 tests)
1. **add_property_invalid_owner** - Validation for invalid owner ($nothing) not working correctly
2. **add_property_not_owner** - Permission check allows non-owner to set different owner
3. **is_clear_property_works** - Permission check causing E_PERM on valid property
4. **is_clear_property_with_read_permission** - Permission check logic incorrect
5. **is_clear_property_wizard_bypasses_read** - Wizard bypass returning wrong value

## Changes Made

### File: `C:\Users\Q\code\barn\builtins\properties.go`

#### 1. Added Helper Functions

**`isBuiltinProperty(name string) bool`**
- Checks if property name matches built-in properties
- Built-ins: name, owner, location, contents, parents, parent, children, programmer, wizard, player, r, w, f, a

**`parsePerms(s string) (db.PropertyPerms, types.ErrorCode)`**
- Validates permission strings contain only r/R, w/W, c/C characters
- Returns E_INVARG for invalid characters
- Case-insensitive validation

**`hasPropertyInDescendants(objID types.ObjID, name string, store *db.Store) bool`**
- Performs breadth-first search through object's descendants
- Checks if any descendant has a defined property with given name
- Required for add_property validation

#### 2. Enhanced `builtinAddProperty`

Added validation checks:
- Built-in property name check (returns E_INVARG)
- Ancestor chain check (returns E_INVARG if property exists in parent chain)
- Descendant chain check (returns E_INVARG if property exists in any child)
- Owner validation (checks if owner >= 0 and exists)
- Permission validation (r/w/c characters only)
- Write permission check (unless wizard)
- Owner permission check (programmer must be owner unless wizard)

#### 3. Enhanced `is_clear_property`

Added logic:
- Built-in property check (returns 0, not E_PROPNF)
- Permission checks for read access
- Wizard bypass for permission checks
- Validates permissions on inherited properties

#### 4. Enhanced `clear_property`

Added logic:
- Built-in property check (returns E_PERM)
- Definer check (returns E_INVARG if called on property definer)
- Write permission check (unless wizard)

## Issues Remaining

### 1. add_property_invalid_owner
**Problem:** Test expects E_INVARG when owner is `$nothing` (#-1), but validation passes.

**Current Code:**
```go
if owner < 0 || store.Get(owner) == nil {
    return types.Err(types.E_INVARG)
}
```

**Issue:** Need to understand how ToastStunt's `valid()` function works for special objects like #-1.

### 2. add_property_not_owner
**Problem:** Non-owner can set different owner despite permission check.

**Current Code:**
```go
if !isWizard && ctx.Programmer != owner {
    return types.Err(types.E_PERM)
}
```

**Issue:** Logic may be checking wrong condition or order of checks is incorrect.

### 3. is_clear_property Permission Issues
**Problem:** Permission checks are too restrictive or checking wrong permissions.

**Current Code:** Checks `prop.Perms.Has(db.PropRead)` for both local and inherited properties.

**Issue:** May need to check permissions differently or handle ownership vs permission flags.

## Reference Implementation (ToastStunt)

Key insights from `C:\Users\Q\src\toaststunt\src\property.cc`:

### add_property (bf_add_prop, line 207)
```c
if ((e = validate_prop_info(info, &owner, &flags, &new_name)) != E_NONE)
    ; /* already failed */
else if (new_name || !obj.is_object())
    e = E_TYPE;
else if (!is_valid(obj))
    e = E_INVARG;
else if (!db_object_allows(obj, progr, FLAG_WRITE)
         || (progr != owner && !is_wizard(progr)))
    e = E_PERM;
else if (!db_add_propdef(obj, pname, value, owner, flags))
    e = E_INVARG;
```

### validate_prop_info (line 109)
```c
*owner = v.v.list[1].v.obj;
if (!valid(*owner))
    return E_INVARG;

for (*flags = 0, s = v.v.list[2].v.str; *s; s++) {
    switch (*s) {
        case 'r': case 'R':
            *flags |= PF_READ;
            break;
        case 'w': case 'W':
            *flags |= PF_WRITE;
            break;
        case 'c': case 'C':
            *flags |= PF_CHOWN;
            break;
        default:
            return E_INVARG;
    }
}
```

### is_clear_property (bf_is_clear_prop, line 299)
```c
if (!db_is_property_built_in(h) && !db_property_allows(h, progr, PF_READ))
    e = E_PERM;
else {
    r.type = TYPE_INT;
    r.v.num = (!db_is_property_built_in(h) && db_property_value(h).type == TYPE_CLEAR);
    e = E_NONE;
}
```

### clear_property (bf_clear_prop, line 263)
```c
if (db_is_property_built_in(h) || !db_property_allows(h, progr, PF_WRITE))
    e = E_PERM;
else if (db_is_property_defined_on(h, obj))
    e = E_INVARG;
```

## Next Steps

1. **Fix owner validation** - Understand how `valid()` works for special objects
2. **Fix permission checks** - Review `db_object_allows()` and `db_property_allows()` logic
3. **Test ownership vs permissions** - Clarify when owner permissions override property permissions
4. **Add comprehensive tests** - Test edge cases for each validation path

## Test Results

```
============================= test session starts =============================
collected 11 items

PASSED tests/conformance/test_conformance.py::test_yaml_case[properties::add_property_invalid_perms]
PASSED tests/conformance/test_conformance.py::test_yaml_case[properties::add_property_builtin_name]
PASSED tests/conformance/test_conformance.py::test_yaml_case[properties::add_property_defined_on_descendant]
PASSED tests/conformance/test_conformance.py::test_yaml_case[properties::is_clear_property_builtin]
PASSED tests/conformance/test_conformance.py::test_yaml_case[properties::clear_property_builtin]
PASSED tests/conformance/test_conformance.py::test_yaml_case[properties::clear_property_on_definer]

FAILED tests/conformance/test_conformance.py::test_yaml_case[properties::add_property_invalid_owner]
FAILED tests/conformance/test_conformance.py::test_yaml_case[properties::add_property_not_owner]
FAILED tests/conformance/test_conformance.py::test_yaml_case[properties::is_clear_property_works]
FAILED tests/conformance/test_conformance.py::test_yaml_case[properties::is_clear_property_with_read_permission]
FAILED tests/conformance/test_conformance.py::test_yaml_case[properties::is_clear_property_wizard_bypasses_read]

================ 6 passed, 5 failed in 0.72s =================
```

## Conclusion

Significant progress made with 6 out of 11 targeted tests now passing. The core validation logic is in place for:
- Built-in property detection
- Permission string validation
- Ancestor/descendant chain checking
- Basic permission enforcement

Remaining issues are primarily related to:
- Special object validation ($nothing)
- Permission vs ownership logic
- Read permission checking for property introspection

The foundation is solid and the remaining fixes should be straightforward once the permission model is better understood.
