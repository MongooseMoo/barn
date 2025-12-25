# MOO Object Management Built-ins

## Overview

Functions for creating, modifying, and querying objects.

---

## 1. Object Creation

### 1.1 create

**Signature:** `create(parent [, owner [, anonymous]]) → OBJ`
**Signature:** `create(parents_list [, owner [, anonymous]]) → OBJ`

**Description:** Creates a new object.

**Parameters:**
- `parent`: Single parent object, or list of parents
- `owner`: Owner of new object (default: caller)
- `anonymous`: If true, object is garbage-collected

**Behavior:**
1. **Allocate new object ID**: Sequential allocation from `max_object() + 1`. Recycled object slots are NOT automatically reused (use `recreate()` to explicitly reuse a slot).
2. Set parent(s)
3. Set owner
4. **Copy inherited properties**: All properties from the entire inheritance chain are copied. Clear properties remain clear (inherit dynamically). Non-clear properties are copied as independent values (not references).
5. **Call `initialize` verb** if defined on the new object or inherited:
   - Called with no arguments: `new_obj:initialize()`
   - Context variables: `this` = new object, `player` = creator, `caller` = creator
   - If `initialize` raises an error, object creation is NOT rolled back
   - The object exists but may be in an invalid state after error

**Examples:**
```moo
obj = create($thing);                    // Single parent
obj = create($thing, player);            // Specify owner
obj = create({$thing, $container});      // Multiple parents
anon = create($thing, $nothing, 1);      // Anonymous object
```

**Errors:**
- E_PERM: Caller can't create children of parent
- E_INVARG: Invalid parent (recycled, not fertile)
- E_QUOTA: Object quota exceeded

---

### 1.2 recreate

**Signature:** `recreate(object, parent) → OBJ`

**Description:** Recreates a recycled object.

**Wizard only.**

**Examples:**
```moo
recreate(#100, $thing)   // Reuse object slot #100
```

**Errors:**
- E_PERM: Not a wizard
- E_INVARG: Object not recycled

---

### 1.3 recycle

**Signature:** `recycle(object) → none`

**Description:** Destroys an object.

**Behavior (in order):**
1. **Call `:recycle` verb** if defined on object or inherited (object still in pre-destruction state)
2. **Move to $nothing**: Remove from location's contents, set location to #-1
3. **Clear properties**: Remove all property values
4. **Remove verbs**: Delete all verb definitions
5. **Clear parent/children links**: Remove from parent's children list
6. **Mark slot as recycled**: Set RECYCLED (1024) and INVALID (512) flags

**Important:** The `:recycle` verb sees the object in its original state (location, properties, verbs all intact). Changes in steps 2-6 happen AFTER the recycle verb completes.

**Reference handling:** Existing references to the recycled object (in variables, properties, lists) remain as the old object ID. `valid(ref)` returns false. Any operation on the recycled object raises E_INVIND.

**Examples:**
```moo
recycle(old_object);
valid(old_object)   => 0
```

**Errors:**
- E_PERM: Not owner or wizard
- E_INVARG: Already recycled

---

## 2. Object Validation

### 2.1 valid

**Signature:** `valid(object) → BOOL`

**Description:** Tests if object exists and is not recycled.

**Behavior:**
- Returns false if object ID is negative (all sentinels: #-1, #-2, #-3, etc.)
- Returns false if object ID exceeds `max_object()`
- Returns false if object was recycled (RECYCLED flag set)
- Returns true only if object is a valid, non-recycled object

**Examples:**
```moo
valid(#0)        => true
valid(#-1)       => false (negative ID - $nothing sentinel)
valid(#-2)       => false (negative ID - $failed_match sentinel)
valid(#99999)    => false (if doesn't exist or exceeds max_object)
```

**Errors:** None (always returns boolean)

---

### 2.2 typeof

**Signature:** `typeof(value) → INT`

**Description:** Returns type code. For objects, returns TYPE_OBJ (1).

---

## 3. Inheritance

### 3.1 parent

**Signature:** `parent(object) → OBJ`

**Description:** Returns first parent of object.

**Examples:**
```moo
parent($room)     => $nothing (or root object)
parent(my_obj)    => $thing
```

**Errors:**
- E_INVIND: Invalid object

---

### 3.2 parents

**Signature:** `parents(object) → LIST`

**Description:** Returns list of all direct parents.

**Examples:**
```moo
parents(obj_with_one_parent)    => {#parent}
parents(obj_with_two_parents)   => {#parent1, #parent2}
```

**Errors:**
- E_INVIND: Invalid object

---

### 3.3 children

**Signature:** `children(object) → LIST`

**Description:** Returns list of direct children.

**Examples:**
```moo
children($thing)   => {#obj1, #obj2, ...}
```

**Errors:**
- E_INVIND: Invalid object

---

### 3.4 ancestors (ToastStunt)

**Signature:** `ancestors(object) → LIST`

**Description:** Returns list of all ancestors (parents, grandparents, etc.).

**Order:** Breadth-first, parents before grandparents.

**Examples:**
```moo
// If obj -> $thing -> $root
ancestors(obj)   => {$thing, $root}
```

---

### 3.5 descendants (ToastStunt)

**Signature:** `descendants(object) → LIST`

**Description:** Returns list of all descendants (children, grandchildren, etc.).

---

### 3.6 chparent

**Signature:** `chparent(object, new_parent) → none`

**Description:** Changes object's parent (single inheritance).

**Errors:**
- E_PERM: Not owner/wizard
- E_INVARG: Would create cycle
- E_INVARG: New parent not fertile

---

### 3.7 chparents

**Signature:** `chparents(object, parents_list) → none`

**Description:** Changes object's parents (multiple inheritance).

**Examples:**
```moo
chparents(obj, {$thing, $container});
```

**Errors:**
- E_PERM: Not owner/wizard
- E_INVARG: Would create cycle
- E_INVARG: Parent not fertile

---

### 3.8 isa

**Signature:** `isa(object, parent) → BOOL`

**Description:** Tests if object inherits from parent (directly or indirectly).

**Examples:**
```moo
isa($room, $thing)    => true (if $room inherits from $thing)
isa(obj, obj)         => true (object is its own ancestor)
```

---

## 4. Object Properties (Built-in)

### 4.1 Flags

**Signature:** `set_object_flag(object, flag, value) → none`
**Signature:** `object_flag(object, flag) → BOOL`

**Flags:**
| Flag | Property | Description |
|------|----------|-------------|
| "user" | `.player` | Is a player |
| "programmer" | `.programmer` | Can program |
| "wizard" | `.wizard` | Full access |
| "read" | `.r` | Readable |
| "write" | `.w` | Writable |
| "fertile" | `.f` | Can have children |

**Examples:**
```moo
// Using properties
obj.wizard = 1;           // Requires caller to be wizard
if (obj.programmer) ...

// Using functions (equivalent)
set_object_flag(obj, "wizard", 1);  // Requires caller to be wizard
if (object_flag(obj, "programmer")) ...
```

**Privilege escalation prevention:**
- Setting the `wizard` flag requires caller to be a wizard
- Ownership of the object is NOT sufficient
- Raises E_PERM if non-wizard attempts to set wizard flag
- This prevents privilege escalation attacks

---

### 4.2 Name and Owner

```moo
obj.name              // Object name (STR)
obj.owner             // Owner object (OBJ)
```

**To change owner:**
```moo
chown(object, new_owner);
```

**Wizard only for chown.**

---

## 5. Location and Contents

### 5.1 Location Properties

```moo
obj.location          // Container (OBJ)
obj.contents          // Contained objects (LIST)
```

---

### 5.2 move

**Signature:** `move(what, where) → none`

**Description:** Moves object to new location.

**Behavior:**
1. Remove from old location's contents
2. Set location to new container
3. Add to new container's contents
4. Call `exitfunc` on old location
5. Call `enterfunc` on new location

**Examples:**
```moo
move(ball, player);           // Pick up ball
move(player, $entrance);      // Teleport player
move(obj, $nothing);          // Remove from world
```

**Errors:**
- E_PERM: No permission
- E_RECMOVE: Moving into self/descendant
- E_INVIND: Invalid object

---

### 5.3 locations (ToastStunt)

**Signature:** `locations(object) → LIST`

**Description:** Returns chain of containers.

**Examples:**
```moo
// If ball in box in room
locations(ball)   => {#box, #room}
```

---

### 5.4 contents

**Signature:** `contents(object) → LIST`

**Description:** Same as `object.contents`.

---

### 5.5 occupants (ToastStunt)

**Signature:** `occupants(location) → LIST`

**Description:** Returns all objects inside (recursive).

---

## 6. Player Management

### 6.1 is_player

**Signature:** `is_player(object) → BOOL`

**Description:** Tests if object is a player.

---

### 6.2 set_player_flag

**Signature:** `set_player_flag(object, value) → none`

**Description:** Sets/clears player status.

**Wizard only.**

---

### 6.3 players

**Signature:** `players() → LIST`

**Description:** Returns list of all player objects.

---

### 6.4 connected_players

**Signature:** `connected_players() → LIST`

**Description:** Returns list of currently connected players.

---

## 7. Object Information

### 7.1 max_object

**Signature:** `max_object() → OBJ`

**Description:** Returns highest allocated object ID.

**Behavior:**
- Returns the high-water mark of object allocation
- Includes recycled objects (returns highest ID ever allocated, not highest valid)
- Example: If #1, #2, #3 exist and #2 is recycled, returns #3
- New objects created via `create()` get `max_object() + 1`

---

### 7.2 reset_max_object (ToastStunt)

**Signature:** `reset_max_object() → OBJ`

**Description:** Reclaims recycled object slots at end.

**Wizard only.**

---

### 7.3 object_bytes

**Signature:** `object_bytes(object) → INT`

**Description:** Returns memory used by object.

---

## 8. Anonymous Objects

### 8.1 Creation

```moo
anon = create($thing, $nothing, 1);
```

### 8.2 Characteristics

- No persistent ID
- Garbage collected when unreferenced
- Cannot be stored in database
- Useful for temporary structures

### 8.3 is_anonymous (ToastStunt)

**Signature:** `is_anonymous(object) → BOOL`

---

## 9. Waif Objects

### 9.1 new_waif

**Signature:** `new_waif() → WAIF`

**Description:** Creates lightweight object.

**Properties accessed via `.:` syntax:**
```moo
w = new_waif();
w.:name = "example";
value = w.:name;
```

---

### 9.2 waif_stats (ToastStunt)

**Signature:** `waif_stats() → LIST`

**Description:** Returns waif memory statistics.

---

## 10. Error Handling

| Error | Condition |
|-------|-----------|
| E_INVIND | Invalid/recycled object |
| E_PERM | Permission denied |
| E_INVARG | Invalid argument |
| E_RECMOVE | Recursive move |
| E_QUOTA | Quota exceeded |

---

## Go Implementation Notes

```go
type Object struct {
    ID        int64
    Name      string
    Owner     int64
    Parents   []int64
    Children  []int64
    Location  int64
    Contents  []int64
    Flags     ObjectFlags
    Props     map[string]*Property
    Verbs     map[string]*Verb
    Recycled  bool
    Anonymous bool
}

func builtinCreate(args []Value) (Value, error) {
    parent, ok := args[0].(ObjValue)
    if !ok {
        // Check for list of parents
        parents, ok := args[0].(*MOOList)
        if !ok {
            return nil, E_TYPE
        }
        // Multiple inheritance path
        return createWithParents(parents, args[1:])
    }

    // Single parent path
    parentObj := db.GetObject(int64(parent))
    if parentObj == nil || parentObj.Recycled {
        return nil, E_INVARG
    }
    if !parentObj.Flags.Has(FLAG_FERTILE) && !callerIsWizard() {
        return nil, E_PERM
    }

    newObj := &Object{
        ID:      db.NextObjectID(),
        Parents: []int64{int64(parent)},
        Owner:   callerPerms(),
    }

    // Inherit properties
    for name, prop := range parentObj.Props {
        newObj.Props[name] = prop.Copy()
    }

    db.AddObject(newObj)

    // Call initialize if exists
    if initVerb := db.FindVerb(newObj.ID, "initialize"); initVerb != nil {
        vm.CallVerb(newObj.ID, "initialize", nil)
    }

    return ObjValue(newObj.ID), nil
}

func builtinMove(args []Value) (Value, error) {
    what := int64(args[0].(ObjValue))
    where := int64(args[1].(ObjValue))

    // Check for recursive move
    if isDescendant(where, what) {
        return nil, E_RECMOVE
    }

    whatObj := db.GetObject(what)
    oldLoc := whatObj.Location

    // Remove from old location
    if oldLoc != NOTHING {
        oldLocObj := db.GetObject(oldLoc)
        oldLocObj.Contents = removeFrom(oldLocObj.Contents, what)
    }

    // Add to new location
    whatObj.Location = where
    if where != NOTHING {
        whereObj := db.GetObject(where)
        whereObj.Contents = append(whereObj.Contents, what)
    }

    // Call hooks
    if oldLoc != NOTHING {
        callVerbIfExists(oldLoc, "exitfunc", what)
    }
    if where != NOTHING {
        callVerbIfExists(where, "enterfunc", what)
    }

    return nil, nil
}
```
