# MOO Property Built-ins

## Overview

Functions for managing object properties.

---

## 1. Property Access

### 1.1 Direct Access

```moo
obj.property_name         // Read property
obj.property_name = val   // Write property
obj.(expr)                // Dynamic name read
obj.(expr) = val          // Dynamic name write
```

**Examples:**
```moo
player.name               => "Alice"
player.name = "Bob";
propname = "score";
player.(propname)         => 100
```

---

## 2. Property Listing

### 2.1 properties

**Signature:** `properties(object) → LIST`

**Description:** Returns list of property names defined on object (not inherited).

**Examples:**
```moo
properties($thing)   => {"name", "description", ...}
```

**Errors:**
- E_INVIND: Invalid object
- E_PERM: Object not readable

---

## 3. Property Information

### 3.1 property_info

**Signature:** `property_info(object, name) → LIST`

**Description:** Returns property metadata.

**Returns:** `{owner, perms}` where:
- `owner`: Object that owns the property
- `perms`: Permission string (see valid formats below)

**Permission string format:**

Valid permission strings (order matters):
- `""` (empty) - no permissions
- `"r"` - read only
- `"w"` - write only
- `"rw"` - read and write
- `"rwc"` - read, write, and chown

**Invalid combinations** (all return E_INVARG):
- `"c"` alone - chown requires write permission
- `"rc"` - chown requires write permission
- `"wc"` - incorrect ordering (must be "rwc")
- Any out-of-order strings: `"wrc"`, `"crw"`, `"cwr"`, `"rcw"`, etc.

**Permission characters:**
| Char | Meaning |
|------|---------|
| r | Readable |
| w | Writable |
| c | Change owner allowed (requires 'w') |

**Examples:**
```moo
property_info($thing, "name")   => {#wizard, "rw"}
```

**Errors:**
- E_INVIND: Invalid object
- E_PROPNF: Property not found

**Note on built-in properties:** Calling `property_info()` on built-in properties (name, owner, location, contents, etc.) returns E_PROPNF. Built-in properties are not tracked in the property system.

---

### 3.2 set_property_info

**Signature:** `set_property_info(object, name, info) → none`

**Description:** Changes property metadata.

**Parameters:**
- `object`: Target object
- `name`: Property name
- `info`: `{owner, perms}` or just `perms` string

**Examples:**
```moo
set_property_info(obj, "secret", {player, "r"});
set_property_info(obj, "public", "rw");
```

**Errors:**
- E_PERM: Not owner/wizard
- E_PROPNF: Property not found

---

## 4. Property Management

### 4.1 add_property

**Signature:** `add_property(object, name, value, info) → none`

**Description:** Adds a new property to object.

**Parameters:**
- `object`: Target object
- `name`: Property name
- `value`: Initial value
- `info`: `{owner, perms}` or just `perms` string

**Examples:**
```moo
add_property(obj, "score", 0, "rw");
add_property(obj, "secret", "", {player, "r"});
```

**Errors:**
- E_PERM: Not owner/wizard
- E_INVARG: Property already exists
- E_INVARG: Invalid property name

---

### 4.2 delete_property

**Signature:** `delete_property(object, name) → none`

**Description:** Removes property from object.

**Behavior details:**
- If property is defined on this object: removes the property definition
- If property is inherited (no local value): succeeds as a no-op
- If property has a local override of inherited property: removes the override, reverts to inherited value
- E_PROPNF only if property doesn't exist anywhere in inheritance chain

**Examples:**
```moo
delete_property(obj, "temporary");  // Remove defined property

// Inherited property handling
parent.x = 10;
child.x = 99;  // Local override
delete_property(child, "x");  // Removes override, child.x reverts to 10

delete_property(child, "x");  // No-op (now inheriting), still succeeds
```

**Errors:**
- E_PERM: Not owner/wizard
- E_PROPNF: Property not found in object or inheritance chain

---

## 5. Property Inheritance

### 5.1 clear_property

**Signature:** `clear_property(object, name) → none`

**Description:** Clears property value to inherit from parent.

**Semantics:**
- Property still exists on object but has no local value
- Reading searches up the parent chain for a non-clear value
- Useful for "resetting" to default

**Multi-level inheritance:**
- If parent's property is also clear, search continues to grandparent
- Continues recursively until a non-clear value is found
- If all ancestors have clear properties, raises E_PROPNF

**Writing to clear properties:**
- Writing to a clear property sets a local value (un-clears it)
- The property now has its own value instead of inheriting
- Does not write through to parent

**Idempotency:**
- Calling `clear_property()` on an already-clear property succeeds silently (no-op)
- Only raises E_PROPNF if property doesn't exist anywhere in inheritance chain

**Examples:**
```moo
// Simple clear
obj.description = "Custom";
clear_property(obj, "description");
// obj.description now returns parent's value

// Multi-level clear
grandparent.x = 99;
clear_property(parent, "x");    // parent.x inherits from grandparent
clear_property(child, "x");     // child.x inherits through parent to grandparent
child.x  => 99

// Writing to clear property
clear_property(obj, "name");
obj.name = "New";  // Un-clears, sets local value
// obj.name is now "New", not inherited

// Idempotent
clear_property(obj, "prop");
clear_property(obj, "prop");  // Succeeds, no error
```

**Errors:**
- E_PERM: Not owner/wizard
- E_PROPNF: Property not found in object or any ancestor

---

### 5.2 is_clear_property

**Signature:** `is_clear_property(object, name) → BOOL`

**Description:** Tests if property is cleared (inheriting).

**Examples:**
```moo
is_clear_property(obj, "name")   => false (has own value)
is_clear_property(obj, "desc")   => true  (inheriting)
```

**Errors:**
- E_PROPNF: Property not found

---

## 6. Built-in Properties

Every object has these read-only or system-managed properties:

| Property | Type | Description |
|----------|------|-------------|
| `.name` | STR | Object name |
| `.owner` | OBJ | Object owner |
| `.location` | OBJ | Container |
| `.contents` | LIST | Contained objects |
| `.programmer` | BOOL | Programmer flag |
| `.wizard` | BOOL | Wizard flag |
| `.r` | BOOL | Read flag |
| `.w` | BOOL | Write flag |
| `.f` | BOOL | Fertile flag |

---

## 7. Error Handling

| Error | Condition |
|-------|-----------|
| E_INVIND | Invalid object |
| E_PROPNF | Property not found |
| E_PERM | Permission denied |
| E_TYPE | Invalid name type |
| E_INVARG | Invalid argument |

---

## 8. Permission Model

### 8.1 Read Permission

Property readable if:
- Caller owns the object
- Caller is wizard
- Property has 'r' permission

**Inherited property permissions:**
When accessing an inherited property (e.g., `child.prop` where `prop` is defined on `parent`):
- Permission checks use the property definition from the ancestor where it's defined
- The object owner's permissions are NOT checked
- Example: If `parent.prop` has no 'r' flag, reading `child.prop` requires owning parent or being wizard, even if you own child

### 8.2 Write Permission

Property writable if:
- Caller owns the object
- Caller is wizard
- Property has 'w' permission

### 8.3 Chown Permission

Property owner changeable if:
- Caller owns the object
- Caller is wizard
- Property has 'c' permission

---

## Go Implementation Notes

```go
type Property struct {
    Name    string
    Value   Value
    Owner   int64
    Perms   PropertyPerms
    Clear   bool
}

type PropertyPerms uint8

const (
    PROP_READ  PropertyPerms = 1 << 0
    PROP_WRITE PropertyPerms = 1 << 1
    PROP_CHOWN PropertyPerms = 1 << 2
)

func builtinProperties(args []Value) (Value, error) {
    objID := int64(args[0].(ObjValue))
    obj := db.GetObject(objID)
    if obj == nil {
        return nil, E_INVIND
    }

    if !canRead(callerPerms(), obj) {
        return nil, E_PERM
    }

    names := make([]Value, 0, len(obj.Props))
    for name := range obj.Props {
        names = append(names, StringValue(name))
    }
    return &MOOList{data: names}, nil
}

func builtinPropertyInfo(args []Value) (Value, error) {
    objID := int64(args[0].(ObjValue))
    name := string(args[1].(StringValue))

    prop, err := db.FindProperty(objID, name)
    if err != nil {
        return nil, E_PROPNF
    }

    perms := ""
    if prop.Perms&PROP_READ != 0 {
        perms += "r"
    }
    if prop.Perms&PROP_WRITE != 0 {
        perms += "w"
    }
    if prop.Perms&PROP_CHOWN != 0 {
        perms += "c"
    }

    return &MOOList{data: []Value{
        ObjValue(prop.Owner),
        StringValue(perms),
    }}, nil
}

func builtinClearProperty(args []Value) (Value, error) {
    objID := int64(args[0].(ObjValue))
    name := string(args[1].(StringValue))

    obj := db.GetObject(objID)
    prop, ok := obj.Props[name]
    if !ok {
        return nil, E_PROPNF
    }

    if !canModify(callerPerms(), obj) {
        return nil, E_PERM
    }

    prop.Clear = true
    prop.Value = nil  // Will inherit from parent
    return nil, nil
}
```
