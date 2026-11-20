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

### 1.2 Name Resolution

Property names are compared case-insensitively everywhere (reads, writes and
every property builtin). A name is resolved in this order (Toast
`db_properties.cc` `db_find_property()`):

1. The built-in property names `name`, `owner`, `programmer`, `wizard`, `r`,
   `w`, `f`, `a`, `location`, `last_move`, `contents`, in any case.
2. The object's own property definitions, in definition order.
3. Each ancestor's definitions, in ancestor order.

The first matching slot wins. A loaded database can give an object a
definition whose name an ancestor also defines; the object's own definition
is then found first (conformance
`server/startup_repeated_ancestor_propname.yaml`).

A property definition name read from a database keeps every character but
the line's terminating newline, so a trailing carriage return is part of the
name and survives later checkpoints (conformance
`server/startup_propdef_trailing_cr.yaml`).

---

## 2. Property Listing

### 2.1 properties

**Signature:** `properties(object) → LIST`

**Description:** Returns the names of the properties defined on the object
(not inherited), in definition order. Deleting a definition removes its name
(matched case-insensitively) and a later `add_property()` appends at the end,
so deleting and re-adding a name moves it to the end. Anonymous objects are
accepted.

**Examples:**
```moo
properties($thing)   => {"name", "description", ...}
```

**Errors:**
- E_TYPE: Argument is not an object
- E_INVARG: Invalid object
- E_PERM: Object not readable

---

## 3. Property Information

### 3.1 property_info

**Signature:** `property_info(object, name) → LIST`

**Description:** Returns property metadata.

**Returns:** `{owner, perms}` where:
- `owner`: Object that owns the property
- `perms`: Permission string (see valid formats below)

**Permission characters:**
| Char | Meaning |
|------|---------|
| r | Readable |
| w | Writable |
| c | Each descendant's slot is owned by that descendant's owner (§8.3) |

The returned `perms` string is lowercase in the order `r`, `w`, `c`.

**Examples:**
```moo
property_info($thing, "name")   => {#wizard, "rw"}
```

**Errors (in the order checked):**
- E_TYPE: `object` is not an object
- E_INVARG: Invalid object
- E_PROPNF: Property not found, or a built-in property (name, owner,
  location, contents, etc.); built-in properties have no owner or perms
- E_PERM: Property not readable

Anonymous objects are accepted.

---

### 3.2 set_property_info

**Signature:** `set_property_info(object, name, info) → 0`

**Description:** Changes property metadata, and optionally renames the
property. Returns 0.

**Parameters:**
- `object`: Target object (numbered; an anonymous object or a WAIF is
  `E_TYPE`)
- `name`: Property name
- `info`: `{owner, perms}` or `{owner, perms, new_name}`, validated as in
  §3.3

**Rename** (three-element `info`, Toast `db_properties.cc`
`db_rename_propdef()`):
- Only the object that defines the property can rename it. For a name the
  object merely inherits, the call is `E_INVARG` and nothing changes (not
  even the owner or perms).
- If `new_name` differs from the old name other than by case, it must not
  name a built-in property (in any case) or a property defined on the
  object, any ancestor or any descendant; otherwise `E_INVARG` and nothing
  changes. A change of case alone, or the same name, is allowed.
- The definition keeps its place in `properties()`. Every descendant's slot
  (anonymous descendants included) follows the new name with its value,
  owner, perms and clear state. The old name is then free to define again.
- WAIF values of the class and its descendant classes keep their value for a
  renamed `:` property under the new name (see `spec/objects.md` §10.3).
- The new owner and perms from the same call are applied after the rename.

**Examples:**
```moo
set_property_info(obj, "secret", {player, "r"});
set_property_info(obj, "old", {player, "rw", "new"});   // rename
```

**Errors (in the order checked):**
- E_TYPE: `object` is not a numbered object
- E_INVARG: Invalid object
- E_TYPE / E_INVARG: Bad `info` (§3.3), checked before the property is
  looked up
- E_PROPNF: Property not found, or built-in
- E_PERM: Property not writable (not the property owner, not a wizard, and
  no `w` flag), or a non-wizard names an owner different from the
  property's current owner. The `w` flag lets a non-owner change the perms
  while keeping the owner.
- E_INVARG: Rename refused (above)

(Toast `property.cc` `set_prop_info()`; conformance
`builtins/verb_prop_builtin_precedence.yaml`,
`builtins/owner_change_permission.yaml`,
`builtins/property_rename_and_numbered_object_checks.yaml`.)

---

### 3.3 Property Info Validation

`add_property()` and `set_property_info()` validate `info` the same way
(Toast `property.cc` `validate_prop_info()`):

1. It must be a list of two or three elements, `{OBJ, STR}` or
   `{OBJ, STR, STR}`; any other shape or element type (including an
   anonymous object as owner, or a bare perms string) is `E_TYPE`.
2. The owner must be valid, else `E_INVARG`.
3. Each perms character must be `r`, `w` or `c`, in either case and in any
   order and combination (`""`, `"c"`, `"rc"`, `"cwr"` are all valid); any
   other character is `E_INVARG`.

---

## 4. Property Management

### 4.1 add_property

**Signature:** `add_property(object, name, value, info) → 0`

**Description:** Adds a new property definition to the object. Returns 0.

**Parameters:**
- `object`: Target object (numbered; anonymous is `E_TYPE`)
- `name`: Property name
- `value`: Initial value
- `info`: `{owner, perms}`, validated as in §3.3. A three-element info list
  is `E_TYPE`.

**Behavior** (Toast `db_properties.cc` `db_add_propdef()`):
- The name must not already resolve on the object (a built-in name, or a
  definition on the object or any ancestor) and must not be defined on any
  descendant; otherwise `E_INVARG`.
- The definition is appended to the object's `properties()`.
- The object's own slot holds `value`. Every existing descendant, permanent
  or anonymous (including anonymous children of permanent descendants),
  gets a clear slot that reads through to the inherited value.
- Slot owner: without `c`, every slot is owned by `owner`. With `c`, each
  slot is owned by the owner of the object holding it, including the
  defining object's own slot (§8.3).
- WAIF values of the class keep the values they already hold.

**Examples:**
```moo
add_property(obj, "secret", "", {player, "r"});
```

**Errors (in the order checked):**
- E_TYPE / E_INVARG: Bad `info` (§3.3)
- E_TYPE: `info` has a third element, or `object` is not a numbered object
- E_INVARG: Invalid object
- E_PERM: Object not writable by the caller, or a non-wizard names an owner
  other than itself (checked before the name clash)
- E_INVARG: Name already defined (above)

---

### 4.2 delete_property

**Signature:** `delete_property(object, name) → 0`

**Description:** Removes a property definition from the object that defines
it. Returns 0.

**Behavior details** (Toast `db_properties.cc` `db_delete_propdef()`):
- Only a definition on the object itself is deleted, found by a
  case-insensitive name match. A name the object only inherits is
  `E_PROPNF`; deleting cannot remove a child's override of an inherited
  property.
- The slot is removed from the object and from every descendant, permanent
  or anonymous, including descendants that had written their own value in
  this or an earlier task. Afterwards reading the name on any of them is
  `E_PROPNF` (unless another definition of it is inherited).
- WAIF values of the class lose their value for a deleted `:` property;
  re-adding the property gives them the class value.

**Examples:**
```moo
delete_property(obj, "temporary");  // Remove defined property

parent.x = 10;            // parent defines x
child.x = 99;             // child's override
delete_property(child, "x");    // E_PROPNF: child does not define x
delete_property(parent, "x");   // removes x from parent and child
```

**Errors (in the order checked):**
- E_TYPE: `object` is not a numbered object
- E_INVARG: Invalid object
- E_PERM: Object not writable by the caller (checked before the lookup, so a
  missing property on an unwritable object is `E_PERM`)
- E_PROPNF: Property not defined on the object

(Conformance `builtins/delete_property_descendants.yaml`,
`builtins/property_definition_anonymous_descendants.yaml`,
`builtins/properties.yaml:delete_property_missing_on_unwritable_object_checks_permission_first`.)

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
- Continues recursively until a non-clear value is found (the definer's slot
  is never clear, so the search always ends there at the latest)

**Writing to clear properties:**
- Writing to a clear property sets a local value (un-clears it)
- The property now has its own value instead of inheriting
- Does not write through to parent

**Idempotency:**
- Calling `clear_property()` on an already-clear property succeeds silently (no-op)

**Definer:** The object that defines the property cannot clear it: its slot
always holds a value, and `clear_property()` there is `E_INVARG`.

Clearing changes only the value; the slot keeps its owner and perms.
Anonymous objects are accepted.

**Examples:**
```moo
// Simple clear (description defined on an ancestor of obj)
obj.description = "Custom";
clear_property(obj, "description");
// obj.description now returns parent's value

// Multi-level clear (x defined on grandparent)
grandparent.x = 99;
clear_property(parent, "x");    // parent.x inherits from grandparent
clear_property(child, "x");     // child.x inherits through parent to grandparent
child.x  => 99

// Writing to clear property
obj.description = "New";  // Un-clears, sets local value

// Idempotent
clear_property(obj, "prop");
clear_property(obj, "prop");  // Succeeds, no error
```

**Errors (in the order checked, Toast `property.cc` `bf_clear_prop()`):**
- E_TYPE: `object` is not an object
- E_INVARG: Invalid object
- E_PROPNF: Property not found (looked up before any permission check)
- E_PERM: A built-in property, or the slot is not writable by the caller
- E_INVARG: The object defines the property

---

### 5.2 is_clear_property

**Signature:** `is_clear_property(object, name) → BOOL`

**Description:** Tests if property is cleared (inheriting). A built-in
property is never clear (returns 0). A slot an object has never written is
clear, including the slot every existing descendant gets from
`add_property()` on an ancestor.

**Examples:**
```moo
is_clear_property(obj, "name")   => false (has own value)
is_clear_property(obj, "desc")   => true  (inheriting)
```

**Errors (in the order checked):**
- E_INVARG: `object` is not an object, or is invalid
- E_PROPNF: Property not found
- E_PERM: Property (not built-in) not readable

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
| `.a` | BOOL | Anonymous flag (children of this object may be anonymous) |
| `.last_move` | MAP | Last move record (0 under the `clear_last_move` server option) |

Built-in names are matched in any case and take precedence over any defined
property (§1.2). They cannot be defined, renamed to, cleared (`E_PERM`), or
queried with `property_info()` (`E_PROPNF`).

---

## 7. Error Handling

| Error | Condition |
|-------|-----------|
| E_TYPE | Object argument of the wrong type (an anonymous object where a numbered one is required), or a malformed info list |
| E_INVARG | Invalid object; bad perms character or invalid owner; name clash |
| E_PROPNF | Property not found |
| E_PERM | Permission denied |

The property builtins report an invalid object as `E_INVARG`, not
`E_INVIND`.

---

## 8. Permission Model

Every object carries its own slot for each property it defines or inherits,
and each slot has its own owner and perms. Permission checks use the slot on
the object being accessed (Toast `db_properties.cc` `db_property_allows()`),
not the definer's slot and not the object's owner.

### 8.1 Read Permission

Property readable if:
- Caller owns the slot (the property owner shown by `property_info()` on
  this object)
- Caller is wizard
- The slot has 'r' permission

### 8.2 Write Permission

Property writable if:
- Caller owns the slot
- Caller is wizard
- The slot has 'w' permission

`set_property_info()` needs write permission on the slot; a non-wizard may
not change the owner (§3.2).

### 8.3 The `c` Flag

`c` decides who owns the slots a definition creates on objects other than
the one named in `info`: with `c`, a slot is owned by the owner of the
object that holds it; without `c`, by the property owner. The rule is
applied whenever a slot is created (Toast `db_properties.cc`
`insert_prop2()`):

- by `add_property()`, for the defining object's own slot and for every
  existing descendant, permanent or anonymous;
- by `create()`, for each slot the new object inherits;
- by `chparent()`/`chparents()`, for each new slot of the moved object and
  of its descendants.

(Conformance `builtins/properties.yaml:chown_flag_*`,
`builtins/property_definition_anonymous_slot_owner.yaml`.)

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
