# MOO Object Model Specification

## Overview

MOO is an object-oriented language with prototype-based inheritance. Objects contain properties (data) and verbs (code).

---

## 1. Object Structure

### 1.1 Object Components

| Component | Description |
|-----------|-------------|
| ID | Unique integer identifier (#0, #1, ...) |
| Name | String name property |
| Owner | Object that owns this object |
| Parents | List of parent objects (inheritance) |
| Children | List of child objects |
| Location | Object this is contained in |
| Contents | Objects contained in this |
| Flags | Permission and state flags |
| Properties | Named values with permissions |
| Verbs | Named code blocks with permissions |

### 1.2 Object IDs

```moo
#0      // System object (root)
#1      // First created object
#-1     // Common "nothing" sentinel
#-2     // Waif class marker
```

**Special objects:**
- `#0` - System object, accessed via `$`
- `$nothing` - Typically `#-1`
- `$failed_match` - Typically `#-2`
- `$ambiguous_match` - Typically `#-3`

---

## 2. Object Flags

### 2.1 Flag Types

| Flag | Value | Description |
|------|-------|-------------|
| USER | 1 | Is a player object |
| PROGRAMMER | 2 | Can write/edit code |
| WIZARD | 4 | Full administrative access |
| READ | 16 | Object is readable |
| WRITE | 32 | Object is writable |
| FERTILE | 128 | Can be used as parent |
| ANONYMOUS | 256 | Anonymous (garbage-collected) |
| INVALID | 512 | Object has been invalidated |
| RECYCLED | 1024 | Object slot is recycled |

### 2.2 Permission Hierarchy

```
WIZARD > PROGRAMMER > USER > (no flags)
```

**Wizard:** Can do anything
**Programmer:** Can write verbs, create objects
**User:** Can login, execute commands
**None:** Limited to public access

---

## 3. Inheritance

### 3.1 Single Inheritance

```moo
child = create(parent);
// child inherits from parent
```

### 3.2 Multiple Inheritance

```moo
child = create({parent1, parent2});
// child inherits from both parents
```

### 3.3 Inheritance Resolution

**Algorithm:** Breadth-first search, left-to-right order.

For property/verb lookup with parents `{A, B}` where A has parent X and B has parent Y:

**Search order:** obj → A → B → X → Y

**Implementation:**
1. Start with queue containing [obj]
2. Pop from front of queue (FIFO)
3. Check for property/verb on current object
4. If found, return it (first match wins)
5. If not found, append current object's parents to end of queue
6. Repeat until queue is empty or match found
7. Track visited objects to prevent infinite loops in cycles

**Diamond inheritance example:**
```
    D
   / \
  B   C
   \ /
    A
```

If A has parents {B, C}, and both B and C inherit from D:
- Search order: A → B → C → D
- If both B and D define property `x`, B's value is found first
- D is visited only once despite being reachable through both paths

**Key properties:**
- First match wins (left-to-right precedence)
- Each object visited at most once (cycle-safe)
- All immediate parents checked before any grandparents (breadth-first)

### 3.4 Related Functions

| Function | Purpose |
|----------|---------|
| `parent(obj)` | Get first parent |
| `parents(obj)` | Get all parents (list) |
| `children(obj)` | Get all children |
| `ancestors(obj)` | Get all ancestors |
| `descendants(obj)` | Get all descendants |
| `chparent(obj, new_parent)` | Change single parent |
| `chparents(obj, parents_list)` | Change parent list |
| `isa(obj, parent)` | Check inheritance |

---

## 4. Properties

### 4.1 Property Definition

Properties are named values attached to objects.

```moo
// Access
value = obj.property_name
value = obj.(expr)          // Dynamic name

// Assignment
obj.property_name = value
obj.(expr) = value
```

### 4.2 Property Permissions

| Permission | Bit | Description |
|------------|-----|-------------|
| Read | r | Property can be read |
| Write | w | Property can be modified |
| Chown | c | Property owner can change |

### 4.3 Property Inheritance

**Defined properties:** Exist on the object itself
**Inherited properties:** Come from parent(s)

```moo
// Check where property is defined
property_info(obj, "name")
// Returns: {owner, perms} or E_PROPNF
```

### 4.4 Clear Properties

A property can be "cleared" to explicitly inherit from parent:

```moo
clear_property(obj, "name");
// Now reads from parent
is_clear_property(obj, "name");  // Returns 1
```

### 4.5 Property Operations

| Function | Purpose |
|----------|---------|
| `properties(obj)` | List all property names |
| `property_info(obj, name)` | Get owner and perms |
| `add_property(obj, name, value, info)` | Add new property |
| `delete_property(obj, name)` | Remove property |
| `set_property_info(obj, name, info)` | Change perms |
| `clear_property(obj, name)` | Clear to inherit |
| `is_clear_property(obj, name)` | Check if cleared |

### 4.6 Built-in Properties

Every object has these:

| Property | Type | Writable | Description |
|----------|------|----------|-------------|
| `.name` | STR | Yes | Object name |
| `.owner` | OBJ | Yes* | Object owner (*via chown) |
| `.location` | OBJ | No** | Container object (**via move) |
| `.contents` | LIST | No** | Contained objects (**via move) |
| `.programmer` | INT | Yes | Programmer flag (0/1) |
| `.wizard` | INT | Yes* | Wizard flag (0/1, *wizards only) |
| `.r` | INT | Yes | Read flag (0/1) |
| `.w` | INT | Yes | Write flag (0/1) |
| `.f` | INT | Yes | Fertile flag (0/1) |

**Mutability notes:**
- `.location` and `.contents` are read-only. Attempting to write raises E_PERM.
- These are modified only via `move()` operations.
- Flag properties (programmer, wizard, r, w, f) treat any non-zero value as true; only 0 clears the flag.

---

## 5. Verbs

### 5.1 Verb Definition

Verbs are named code blocks attached to objects.

```moo
// Call syntax
obj:verb_name(args)
obj:(expr)(args)      // Dynamic name
```

### 5.2 Verb Permissions

| Permission | Bit | Description |
|------------|-----|-------------|
| Read | r | Verb code can be read |
| Write | w | Verb code can be modified |
| Execute | x | Verb can be called |
| Debug | d | Debug info available |

### 5.3 Verb Arguments

Verbs can specify expected arguments:

| Specifier | Meaning |
|-----------|---------|
| `this` | Called on this object |
| `none` | No direct object |
| `any` | Any value accepted |

### 5.4 Verb Dispatch

1. Parse command: `verb dobj prep iobj`
2. Find verb on dobj's ancestor chain
3. Check verb argument specifiers match
4. Check execute permission
5. Call verb with context variables

### 5.5 Context Variables

Inside a verb, these are automatically set:

| Variable | Type | Description |
|----------|------|-------------|
| `this` | OBJ | Object verb is defined on |
| `player` | OBJ | Player who initiated action |
| `caller` | OBJ | Object that called this verb |
| `verb` | STR | Verb name as called |
| `args` | LIST | Arguments passed |
| `argstr` | STR | Original argument string |
| `dobj` | OBJ | Direct object |
| `dobjstr` | STR | Direct object string |
| `iobj` | OBJ | Indirect object |
| `iobjstr` | STR | Indirect object string |
| `prepstr` | STR | Preposition string |

### 5.6 Verb Operations

| Function | Purpose |
|----------|---------|
| `verbs(obj)` | List all verb names |
| `verb_info(obj, name)` | Get verb metadata |
| `verb_args(obj, name)` | Get argument specs |
| `verb_code(obj, name)` | Get verb source |
| `add_verb(obj, info, args)` | Add new verb |
| `delete_verb(obj, name)` | Remove verb |
| `set_verb_info(obj, name, info)` | Change metadata |
| `set_verb_args(obj, name, args)` | Change arg specs |
| `set_verb_code(obj, name, code)` | Change source |

---

## 6. Object Lifecycle

### 6.1 Creation

```moo
new_obj = create(parent);
new_obj = create(parent, owner);
new_obj = create({parent1, parent2});
```

**Semantics:**
1. Allocate new object ID
2. Set parent(s)
3. Set owner (caller if not specified)
4. Inherit properties from parent(s)
5. Call `initialize` verb if defined

### 6.2 Recycling

```moo
recycle(obj);
```

**Semantics:**
1. Call `recycle` verb if defined
2. Clear all properties
3. Remove all verbs
4. Remove from parent's children
5. Remove from location's contents
6. Mark object slot as recycled

### 6.3 Recycled Objects

- `valid(recycled_obj)` returns `0`
- Accessing recycled object raises `E_INVIND`
- Object ID may be reused later

### 6.4 Recreation

```moo
recreate(obj_id, parent);
```

**Semantics:**
- Recreate a recycled object slot
- Must be wizard to use
- Object gets new parent and properties

---

## 7. Location and Contents

### 7.1 Containment Model

Objects can contain other objects:
- `obj.location` - What contains this
- `obj.contents` - What this contains

### 7.2 Moving Objects

```moo
move(what, where);
```

**Semantics:**
1. Remove `what` from old location's contents
2. Set `what.location` to `where`
3. Add `what` to `where.contents`
4. Call `exitfunc` on old location (if defined)
5. Call `enterfunc` on new location (if defined)

**Errors:**
- `E_RECMOVE`: Can't move object into itself/descendant
- `E_PERM`: No permission to move

### 7.3 Related Functions

| Function | Purpose |
|----------|---------|
| `move(what, where)` | Move object |
| `contents(obj)` | Same as obj.contents |
| `locations(obj)` | Chain of containing objects |
| `occupants(location)` | All objects inside (recursive) |

---

## 8. Object Validity

### 8.1 Checking Validity

```moo
valid(obj)
```

**Returns:**
- `1` if object exists and is not recycled
- `0` if object is recycled or never existed

### 8.2 Object Tests

| Condition | Test |
|-----------|------|
| Exists | `valid(obj)` |
| Is player | `is_player(obj)` |
| Is programmer | `obj.programmer` |
| Is wizard | `obj.wizard` |
| Inherits from | `isa(obj, parent)` |

---

## 9. Anonymous Objects

### 9.1 Creation

```moo
anon = create(parent, $nothing, 1);  // Third arg = anonymous
```

### 9.2 Characteristics

- No persistent ID (garbage collected)
- Cannot be stored in database permanently
- Collected when no references remain
- Useful for temporary data structures

### 9.3 Cycle Detection

Anonymous objects with circular references are detected and collected.

---

## 10. Waif Objects

### 10.1 Creation

```moo
w = new_waif();
```

### 10.2 Characteristics

- Lightweight (less overhead than objects)
- Prototype-based
- Properties accessed via `.:` syntax
- Garbage collected

### 10.3 Waif Properties

```moo
w.:name = "example";
value = w.:name;
```

### 10.4 Waif Class

All waifs inherit from their "waif class" object, which provides:
- Default property values
- Verb implementations

---

## 11. Permission Model

### 11.1 Ownership

Every object has an owner:
- Owner has full control
- Owner can grant permissions to others

### 11.2 Permission Checks

| Action | Required |
|--------|----------|
| Read property | Owner, or `r` flag |
| Write property | Owner, or `w` flag |
| Call verb | Owner, or `x` flag |
| Read verb code | Owner, or `r` flag |
| Modify object | Owner or wizard |
| Create child | Owner/wizard, and `f` flag |

### 11.3 Caller Permissions

```moo
caller_perms()    // Returns permission object
set_task_perms(obj)  // Change task permissions
```

### 11.4 Wizard Bypass

Wizards can:
- Access any property
- Call any verb
- Modify any object
- Use privileged builtins

---

## 12. System Objects

### 12.1 System Object (#0)

The system object is accessed via `$` in MOO code. It is the root of the object hierarchy and contains server hooks.

**Required Properties:**

| Property | Type | Description |
|----------|------|-------------|
| `server_options` | MAP | Server configuration (see §12.4) |
| `maxint` | INT | Maximum integer value: 9223372036854775807 (2^63-1, 64-bit signed) |
| `minint` | INT | Minimum integer value: -9223372036854775808 (-2^63, 64-bit signed) |
| `nothing` | OBJ | "Nothing" sentinel (typically #-1) |
| `failed_match` | OBJ | Failed match sentinel (typically #-2) |
| `ambiguous_match` | OBJ | Ambiguous match sentinel (typically #-3) |

**Required Verbs:**

| Verb | Signature | When Called |
|------|-----------|-------------|
| `server_started` | `()` | After database load, before connections |
| `checkpoint_started` | `()` | Before database checkpoint |
| `checkpoint_finished` | `(success)` | After checkpoint (1=ok, 0=fail) |
| `user_connected` | `(player)` | Player successfully logged in |
| `user_disconnected` | `(player)` | Player connection closed |
| `user_reconnected` | `(player)` | Player reconnected (replaced connection) |
| `do_login_command` | `(conn, line)` | Command from unlogged connection |

### 12.2 Root Objects

Common base objects (conventional, not required):

```moo
$nothing      // #-1, parent for orphan objects
$room         // Base for locations
$thing        // Base for portable objects
$player       // Base for player objects
$exit         // Base for exit objects
$container    // Base for containers
```

### 12.3 Login Object

The login object (often `$login` or pointed to by `#0.login`) handles authentication.

**Typical Verbs:**

| Verb | Purpose |
|------|---------|
| `authenticate` | Verify username/password |
| `create_player` | Create new player object |
| `welcome_message` | Return welcome text |
| `connect_message` | Return "use connect or create" text |

### 12.4 Server Options

`#0.server_options` is a MAP controlling server behavior:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `bg_ticks` | INT | 30000 | Background task tick limit |
| `bg_seconds` | INT | 3 | Background task time limit |
| `fg_ticks` | INT | 60000 | Foreground task tick limit |
| `fg_seconds` | INT | 5 | Foreground task time limit |
| `max_stack_depth` | INT | 50 | Maximum call stack depth |
| `connect_timeout` | INT | 300 | Seconds before unlogged connection times out |
| `dump_interval` | INT | 3600 | Seconds between automatic checkpoints (alias: `checkpoint_interval`) |
| `checkpoint_interval` | INT | 3600 | Seconds between automatic checkpoints (alias: `dump_interval`) |
| `max_queued_output` | INT | 65536 | Max bytes buffered per connection |
| `name_lookup_timeout` | INT | 5 | DNS lookup timeout |
| `protect_*` | INT | 0/1 | Builtin function protection flags |

**Note:** `dump_interval` and `checkpoint_interval` are aliases for the same setting. Implementations should accept both names. If both are set, `checkpoint_interval` takes precedence.

### 12.5 Minimal Core Database

A minimal MOO database requires:

1. **#0** (system object) with:
   - `server_options` property (empty map is valid, server uses defaults)
   - `do_login_command` verb (can just return 0 to deny all logins)
   - `server_started` verb (can be empty)

2. **At least one wizard object** (recommended but not strictly required):
   - WIZARD flag set (for bootstrapping)
   - Allows use of wizard-only builtins to build the database

**Type validation:** Setting `server_options` to invalid values (wrong types, out-of-range) will cause `load_server_options()` to raise E_INVARG.

3. **Optional but conventional:**
   - `$nothing` (#-1) for sentinel
   - `$room` as location base
   - `$player` as player base

### 12.6 Bootstrap Sequence

**No builtin bootstrap mechanism is required.** Creating a minimal database from scratch requires manual database file creation or external tooling.

**Recommended bootstrap approach:**

1. Create empty database file with #0 (system object)
2. Add minimal required properties (`server_options` as empty map)
3. Add stub verbs for hooks (`do_login_command`, `server_started`)
4. Create wizard object for administration
5. Save database

**Implementations may provide:**
- Command-line tool to create minimal database
- Database generation utility
- Example minimal database file

**MOO code cannot bootstrap itself** - the server must load a database before executing MOO code.

---

## 13. Go Implementation Notes

### 13.1 Object Structure

```go
type Object struct {
    ID         int64
    Name       string
    Owner      int64
    Parents    []int64
    Children   []int64
    Location   int64
    Contents   []int64
    Flags      ObjectFlags
    Properties map[string]*Property
    Verbs      map[string]*Verb
    Anonymous  bool
}
```

### 13.2 Property Structure

```go
type Property struct {
    Name   string
    Value  Value
    Owner  int64
    Perms  PropertyPerms
    Clear  bool  // Inheriting from parent
}
```

### 13.3 Verb Structure

```go
type Verb struct {
    Name     string
    Owner    int64
    Perms    VerbPerms
    ArgSpec  VerbArgs
    Code     []string  // Source lines
    Compiled *Program  // Bytecode (cached)
}
```

### 13.4 Inheritance Resolution

```go
func (db *Database) FindProperty(obj int64, name string) (*Property, error) {
    visited := make(map[int64]bool)
    return db.findPropertyRecursive(obj, name, visited)
}

func (db *Database) findPropertyRecursive(obj int64, name string, visited map[int64]bool) (*Property, error) {
    if visited[obj] {
        return nil, nil // Cycle detection
    }
    visited[obj] = true

    o := db.GetObject(obj)
    if prop, ok := o.Properties[name]; ok && !prop.Clear {
        return prop, nil
    }

    for _, parent := range o.Parents {
        if prop, err := db.findPropertyRecursive(parent, name, visited); prop != nil {
            return prop, nil
        }
    }
    return nil, ErrPropNotFound
}
```
