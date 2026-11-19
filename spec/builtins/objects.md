# MOO Object Management Built-ins

## Overview

Functions for creating, modifying, and querying objects.

---

## 1. Object Creation

### 1.1 create

**Signature:** `create(parent [, owner] [, anonymous] [, init_args]) → OBJ`
**Signature:** `create(parents_list [, owner] [, anonymous] [, init_args]) → OBJ`

**Description:** Creates a new object.

**Parameters** (Toast `objects.cc` `bf_create()`): after the first argument,
up to three optional arguments are told apart by type, each kind at most
once: an OBJ is the owner, an INT is the anonymous flag, a LIST is the
argument list for `:initialize`. Any other type, or a second argument of the
same kind, is `E_TYPE`.
- `parent`: A numbered object (`#-1` allowed), or a list of numbered
  objects. An anonymous object is never a parent: `create(anon)` and
  `create({anon})` are `E_TYPE`.
- `owner`: Owner of the new object (default: the task's permissions). An
  owner of `#-1` makes a permanent object its own owner.
- `anonymous`: If true, the new object is anonymous (garbage-collected)
- `init_args`: Arguments passed to `:initialize` (default `{}`)

**Behavior:**
1. **Allocate new object ID**: Sequential allocation from `max_object() + 1`. Recycled object slots are NOT automatically reused (use `recreate()` to explicitly reuse a slot).
2. Set parent(s)
3. Set owner
4. **Inherited property slots**: The new object gets one slot for each
   property its ancestors define. Each slot is clear (holds no value and
   reads through to the ancestor's value); its owner follows the `c` rule
   (`spec/builtins/properties.md` §8.3).
5. **Call `initialize` verb** if defined on the new object or inherited:
   - Called with `init_args` (or no arguments): `new_obj:initialize(@init_args)`
   - Context variables: `this` = new object, `player` = creator, `caller` = creator
   - If `initialize` raises an error, object creation is NOT rolled back
   - The object exists but may be in an invalid state after error

**Examples:**
```moo
obj = create($thing);                    // Single parent
obj = create($thing, player);            // Specify owner
obj = create({$thing, $container});      // Multiple parents
anon = create($thing, 1);                // Anonymous object
```

**Errors (in the order checked):**
- E_TYPE: First argument is not an OBJ or a list of OBJs (an anonymous
  object, alone or in the list, is `E_TYPE`); bad optional argument types
- E_INVARG: Anonymous with owner `#-1`; invalid owner; invalid parent
- E_PERM: A non-wizard names another owner, or a parent does not allow it:
  creating a permanent child needs the parent's `f` flag, an anonymous child
  needs the parent's `a` flag (or ownership of the parent, or wizard)
- E_QUOTA: Object quota exceeded
- E_INVARG: The parents' property names conflict

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

**Behavior (in order,** Toast `objects.cc` `bf_recycle()`**):**
1. **Call `:recycle` verb** if defined on object or inherited (object still
   in pre-destruction state). Anonymous objects get `:recycle` too.
2. If the object is no longer valid when `:recycle` returns, the call is
   `E_INVARG`.
3. **Empty contents**: Each object it contains is moved to `#-1` (running
   the object's `:exitfunc`), then the object itself is moved out of its
   location.
4. **Reparent children**: Each child gets the recycled object's parents in
   place of the recycled object.
5. **Destroy**: Properties, verbs and the object are discarded; the owner's
   quota is credited. Values in the object's own slots are released, so an
   anonymous object held only there is recycled. A slot the object only
   inherited and never wrote is clear and holds no value, so recycling a
   child never recycles an anonymous object its parent holds, whether the
   property was defined before or after the child was created
   (conformance `builtins/recycle.yaml:recycling_created_child_leaves_parents_anonymous_value`,
   `recycling_child_leaves_anonymous_value_of_property_added_later`).

**Important:** The `:recycle` verb sees the object in its original state (location, properties, verbs all intact). Changes in steps 3-5 happen AFTER the recycle verb completes.

**Reference handling:** Existing references to the recycled object (in variables, properties, lists) remain as the old object ID. `valid(ref)` returns false. Any operation on the recycled object raises E_INVIND.

**Examples:**
```moo
recycle(old_object);
valid(old_object)   => 0
```

**Errors:**
- E_TYPE: Argument is not an object
- E_INVARG: Invalid object, or an anonymous object already recycled
- E_PERM: Not owner or wizard

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
valid(#-2)       => false (negative ID - $ambiguous_match sentinel)
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

**Description:** Returns list of direct children. Anonymous objects are
never listed, here or in `descendants()`, whether created under the parent
or moved there by `chparent()` (conformance
`builtins/objects.yaml:chparent_anonymous_not_added_to_parent_children`).

**Examples:**
```moo
children($thing)   => {#obj1, #obj2, ...}
```

**Errors:**
- E_INVIND: Invalid object

---

### 3.4 ancestors (ToastStunt)

**Signature:** `ancestors(object [, full]) → LIST`

**Description:** Returns list of all ancestors (parents, grandparents, etc.).
If `full` is true, `object` itself comes first.

**Order:** Depth-first pre-order, left to right. Each parent is followed by
its own ancestors before the next parent, and each ancestor appears once, at
its first occurrence. This is the order property and command-verb lookup use
(`spec/objects.md` §3.3; Toast `db_objects.cc` `db2_add_parents` via the
`DEFUNC` macro).

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

**Description:** Changes object's parent (single inheritance). Returns 0.

**Semantics** (shared with `chparents`, Toast `objects.cc`
`bf_chparent_chparents()`, `db_properties.cc` property relayout):
- `object` may be numbered or anonymous. An anonymous object that changes
  parent is still never listed in any parent's `children()` or
  `descendants()`.
- The new parent(s) must be numbered objects (`#-1` allowed); an anonymous
  object is `E_TYPE`.
- Property slots of the object and of each descendant: a slot for a
  property defined by an ancestor common to the old and new parents is kept,
  with its value (or clear state) and owner, including a value written
  earlier in the same task. Slots for properties only the old ancestry
  defined are dropped. Slots for properties only the new ancestry defines
  are added clear, owned per the `c` rule (`spec/builtins/properties.md`
  §8.3). A property of the same name from a different definer counts as a
  different property, so the object gets a clear slot of the new one.
  (Conformance `builtins/properties.yaml:chparent_keeps_*`,
  `chparent_same_name_from_other_definer_is_clear`,
  `chown_flag_chparent_*`.)

**Errors (in the order checked):**
- E_TYPE: `object` is not an object, or the new parent is not an OBJ (an
  anonymous object is not a parent)
- E_INVARG: Invalid object or invalid new parent
- E_PERM: Caller does not control `object`, or the new parent lacks `f` and
  is not owned by the caller (wizards bypass)
- E_RECMOVE: The new parent is `object` or one of its descendants
- E_INVARG: `object` or a descendant defines a property name that the new
  parent or its ancestors also define

---

### 3.7 chparents

**Signature:** `chparents(object, parents_list) → none`

**Description:** Changes object's parents (multiple inheritance).

**Examples:**
```moo
chparents(obj, {$thing, $container});
```

**Errors:** As for `chparent`, applied to each element of `parents_list`:
E_TYPE for a non-OBJ element (an anonymous object included), E_INVARG for an
invalid one, E_PERM, E_RECMOVE for a cycle, and E_INVARG for a property
name conflict.

---

### 3.8 isa

**Signature:** `isa(object, parent [, return_object]) → BOOL | OBJ`

**Description:** Tests if object inherits from parent (directly or indirectly).
`parent` may be a list, in which case the first matching parent counts. When
`return_object` is true, the result is the matching parent, or `#-1` when
none matches, instead of 1 or 0 (Toast `objects.cc` `bf_isa()`).

Flag arguments are Toast 2.8 `TYPE_FLAG`: an INT or a BOOL is accepted and read for truth; any other type (a FLOAT included) is `E_TYPE`, and `function_info()` reports the argument type as -3 (Toast `functions.cc` lines 268-269, `structures.h` line 123; registration at `objects.cc` lines 1381-1384 at origin/master; conformance `builtins/flag_args_accept_bool.yaml`).

**Examples:**
```moo
isa($room, $thing)    => true (if $room inherits from $thing)
isa(obj, obj)         => true (object is its own ancestor)
```

---

## 4. Object Properties (Built-in)

### 4.1 Flags

Object flags are accessed and modified via properties only. There are no `object_flag()` or `set_object_flag()` functions.

**Flags:**
| Property | Description |
|----------|-------------|
| `.player` | Is a player (use `is_player()` to test, `set_player_flag()` to change) |
| `.programmer` | Can program |
| `.wizard` | Full access (wizard-only to set) |
| `.r` | Readable |
| `.w` | Writable |
| `.f` | Can have children |

**Examples:**
```moo
// Reading flags
if (obj.wizard) ...
if (obj.programmer) ...

// Setting flags
obj.programmer = 1;       // Requires permissions
obj.wizard = 1;           // Requires caller to be wizard
obj.f = 1;                // Make object fertile
```

**Privilege escalation prevention:**
- Setting the `wizard` property requires caller to be a wizard
- Ownership of the object is NOT sufficient
- Raises E_PERM if non-wizard attempts to set wizard property
- This prevents privilege escalation attacks

---

### 4.2 Name and Owner

```moo
obj.name              // Object name (STR)
obj.owner             // Owner object (OBJ)
```

**Changing owner (wizard only):**
```moo
obj.owner = new_owner;
```

**Note:** There is no `chown()` function. Owner is changed via property assignment.

---

## 5. Location and Contents

### 5.1 Location Properties

```moo
obj.location          // Container (OBJ)
obj.contents          // Contained objects (LIST)
```

---

### 5.2 move

**Signature:** `move(what, where [, position]) → 0`

**Description:** Moves object to new location. Returns 0. `what` and `where`
must be numbered objects (OBJ); `position` (INT, default 0) places `what` at
that index of the new contents.

**Behavior** (Toast `objects.cc` `do_move()`):
1. Check: `what` invalid, `where` invalid and not `#-1`, or `position` < 0
   is `E_INVARG`; the caller must control `what`, else `E_PERM`.
2. Unless `where` is `#-1` or already `what`'s location, call
   `where:accept(what)`; a missing `:accept` counts as refusal. With a
   nonzero `position`, the caller must also control `where` (`E_PERM`).
   A refusal is `E_NACC` unless the caller is a wizard.
3. After `:accept` returns, both objects are checked again. If `what` is no
   longer valid, `where` (other than `#-1`) is no longer valid (for example
   `:accept` recycled it), or `what` is already in `where` with `position`
   0, `move()` returns 0 without moving and without calling `:exitfunc` or
   `:enterfunc` (conformance `builtins/move_accept_recycles_destination.yaml`).
4. If `where` is `what` or is inside `what`, `E_RECMOVE`.
5. Remove from old location's contents, set location, add to new contents.
6. If the location changed, call `oldloc:exitfunc(what)`.
7. Then, if both are still valid and `what` is still in `where`, call
   `where:enterfunc(what)`.

**Examples:**
```moo
move(ball, player);           // Pick up ball
move(player, $entrance);      // Teleport player
move(obj, $nothing);          // Remove from world
```

**Errors:**
- E_TYPE: `what` or `where` is not a numbered object
- E_INVARG: Invalid `what` or `where`, or negative `position`
- E_PERM: No permission
- E_NACC: `where:accept` refused (non-wizard caller)
- E_RECMOVE: Moving into self or into an object inside `what`

---

### 5.3 locations (ToastStunt)

**Signature:** `locations(object [, stop [, check_parent]]) → LIST`

**Description:** Returns chain of containers, innermost first. With `stop`,
the walk ends before the first location equal to `stop`, or (when
`check_parent` is true) the first location that `isa()` `stop`. An invalid
`object` is `E_INVIND` (Toast `objects.cc` `bf_locations()`).

Flag arguments are Toast 2.8 `TYPE_FLAG`: an INT or a BOOL is accepted and read for truth; any other type (a FLOAT included) is `E_TYPE`, and `function_info()` reports the argument type as -3 (Toast `functions.cc` lines 268-269, `structures.h` line 123; registration at `objects.cc` lines 1381-1384 at origin/master; conformance `builtins/flag_args_accept_bool.yaml`).

**Examples:**
```moo
// If ball in box in room
locations(ball)   => {#box, #room}
```

---

### 5.4 occupants (ToastStunt)

**Signature:** `occupants(objects [, parent [, player_flag [, inverse]]]) → LIST`

**Description:** Filters a list of objects by parent inheritance and optionally player flag.

**Parameters:**
- `objects`: LIST of objects to filter (must all be valid objects)
- `parent`: OBJ or LIST of OBJs - filter to objects that `isa()` one of these parents
- `player_flag`: INT or BOOL - if true, only return objects with the player flag set
- `inverse`: INT or BOOL - if true, invert the parent check (return objects NOT descended from parent)

Flag arguments are Toast 2.8 `TYPE_FLAG`: an INT or a BOOL is accepted and read for truth; any other type (a FLOAT included) is `E_TYPE`, and `function_info()` reports the argument type as -3 (Toast `functions.cc` lines 268-269, `structures.h` line 123; registration at `objects.cc` lines 1381-1384 at origin/master; conformance `builtins/flag_args_accept_bool.yaml`).

**Behavior by argument count:**
- 1 arg: returns all valid objects from the list (player flag check enabled)
- 2 args: filters by parent (isa check), player flag check enabled
- 3 args: filters by parent, uses explicit player_flag setting
- 4 args: filters by parent with optional inverse, uses explicit player_flag

**Examples:**
```moo
occupants(connected_players())           // All connected players
occupants(connected_players(), $prog)    // Connected players descended from $prog
occupants({#10, #20, #30}, $thing)       // Objects in list descended from $thing
occupants({#10, #20}, $thing, 0)         // Descended from $thing, any player flag
occupants({#10, #20}, $thing, 0, 1)      // NOT descended from $thing
```

**Returns:** E_TYPE if first arg not a list, or parent arg wrong type. E_INVARG if list contains non-objects or invalid objects.

**Notes:**
- Uses the background thread system when threading is enabled and may suspend.
- There is no `contents()` function. Use `object.contents` property to get direct contents.

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
- Recycled slots stay counted across a checkpoint and restart: a database
  stores each recycled slot as `#N recycled` and loading recreates it as an
  empty slot, so `max_object()` after a restart still covers recycled slots
  above the highest valid object, and the next `create()` does not reuse
  them (Toast `db_objects.cc` `db_last_used_objid()`,
  `dbpriv_new_recycled_object()`; `db_file.cc` reader). Anonymous objects
  take no object number and never affect `max_object()`.

---

### 7.2 reset_max_object (ToastStunt)

**Signature:** `reset_max_object() → INT`

**Description:** Reclaims recycled object slots at end of database and returns new max_object value.

**Wizard only.**

---

### 7.3 recycled_objects (ToastStunt)

**Signature:** `recycled_objects() → LIST`

**Description:** Returns, in ascending order, every object number from `#0`
to `max_object()` that is not a valid object.

**Examples:**
```moo
recycled = recycled_objects();   => {#100, #105, #112}
```

---

### 7.4 next_recycled_object (ToastStunt)

**Signature:** `next_recycled_object([start]) → OBJ | INT`

**Description:** Returns the first object number at or above `start`
(default `#0`), up to `max_object()`, that is not a valid object. `start`
itself is included. Returns the integer 0 if there is none (Toast
`objects.cc` `bf_next_recycled_object()`).

**Examples:**
```moo
first = next_recycled_object();      => #100
next = next_recycled_object(#101);   => #105
```

**Errors:**
- E_INVARG: `start` is negative (any negative object number, not only
  `#-1`) or greater than `max_object()` (conformance
  `builtins/next_recycled_object_negative_start.yaml`)

---

### 7.5 owned_objects (ToastStunt)

**Signature:** `owned_objects(owner) → LIST`

**Description:** Returns list of all objects owned by owner.

**Examples:**
```moo
my_objects = owned_objects(player);
wizard_objects = owned_objects(#2);
```

---

### 7.6 locate_by_name (ToastStunt)

**Signature:** `locate_by_name(name [, case_matters]) → LIST`

**Description:** Returns, in ascending order, every valid object whose `name`
contains `name` as a substring (Toast `objects.cc`
`locate_by_name_callback()`).

**Parameters:**
- `name`: String to search for
- `case_matters`: If true, the substring match is case-sensitive (default:
  false, case-insensitive)

Flag arguments are Toast 2.8 `TYPE_FLAG`: an INT or a BOOL is accepted and read for truth; any other type (a FLOAT included) is `E_TYPE`, and `function_info()` reports the argument type as -3 (Toast `functions.cc` lines 268-269, `structures.h` line 123; registration at `objects.cc` lines 1381-1384 at origin/master; conformance `builtins/flag_args_accept_bool.yaml`).

**Examples:**
```moo
objs = locate_by_name("system");          // Case-insensitive substring search
objs = locate_by_name("System", true);    // Case-sensitive
```

**Note:** Uses the background thread system when threading is enabled and may suspend.

---

### 7.7 object_bytes

**Signature:** `object_bytes(object) → INT`

**Description:** Returns memory used by object.

---

## 8. Anonymous Objects

### 8.1 Creation

```moo
anon = create($thing, 1);              // owner #-1 with anonymous is E_INVARG
```

### 8.2 Characteristics

- No persistent ID
- Garbage collected when unreferenced
- Cannot be stored in database
- Useful for temporary structures
- Never a parent: `create()`, `chparent()` and `chparents()` reject an
  anonymous object as a parent with `E_TYPE`, so an anonymous object has no
  children and never appears in `children()` or `descendants()`
- May itself change parent with `chparent()`, and is recycled with
  `recycle()` (which runs its `:recycle`)
- `add_property()`, `delete_property()`, `set_property_info()`,
  `add_verb()`, `delete_verb()`, `set_verb_info()`, `set_verb_args()`,
  `set_verb_code()`, `verb_args()` and `move()` require a numbered object and
  raise `E_TYPE` for an anonymous one. Reading builtins (`properties()`,
  `property_info()`, `verbs()`, `verb_info()`, `verb_code()`,
  `clear_property()`, `is_clear_property()`) accept it.
- Property definitions added to or deleted from an ancestor reach anonymous
  descendants as they reach permanent ones

**Note:** There is no `is_anonymous()` function. Check object type with `typeof(obj) == TYPE_ANON` or test if object is less than #0

---

## 9. Waif Objects

### 9.1 new_waif

**Signature:** `new_waif() → WAIF`

**Description:** Creates a lightweight object. This builtin is invoked from a
verb: the receiver of the calling verb becomes the WAIF class and the current
programmer becomes its owner. The class and owner do not change afterward,
and each call creates a distinct reference identity.

**Properties use ordinary `.` syntax and aliases share mutation:**
```moo
waif_class = create($waif);
add_property(waif_class, ":name", "", {player, ""});
w = waif_class:new();
alias = w;
alias.name = "example";
value = w.name;
```

**Errors:**
- `E_INVIND`: The receiver used as the WAIF class is recycled or invalid.
- `E_INVARG`: The receiver is an anonymous object.

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
| E_RECMOVE | Recursive move or recursive parenting |
| E_NACC | Move refused by `:accept` |
| E_QUOTA | Quota exceeded |
| E_TYPE | Anonymous object where a numbered object is required |

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
