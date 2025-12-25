# LambdaMOO Database Format Exploration

Based on thorough exploration of the `lambdamoo-db` Python package (version 0.1.10).

## 1. What Is This?

The `lambdamoo-db-py` package is a **Python library for parsing, reading, and writing LambdaMOO database files**. It handles:

- Loading MOO database files into Python objects
- Serializing Python objects back to MOO database format
- Exporting databases to JSON or flat file structures
- Preserving round-trip fidelity

**Key files:**
- `lambdamoo_db/database.py` - Data models
- `lambdamoo_db/reader.py` - Parser (550+ lines)
- `lambdamoo_db/writer.py` - Serializer (360+ lines)

## 2. Database Format

Text-based format with strict version control.

### Version History
```
DBV_Prehistory (0)    - Before format versions
DBV_Exceptions (1)    - Added try/except/finally/endtry
DBV_BreakCont (2)     - Added break/continue
DBV_Float (3)         - Added float type
DBV_BFBugFixed (4)    - Bug fix in built-in function overrides
DBV_NextGen (5)       - Next-generation format
DBV_TaskLocal (6)     - Task local values
DBV_Map (7)           - MAP type
DBV_FileIO (8)        - E_FILE error
DBV_Exec (9)          - E_EXEC error
DBV_Interrupt (10)    - E_INTRPT error
DBV_This (11)         - 'this' verification
DBV_Iter (12)         - Map iterator
DBV_Anon (13)         - Anonymous objects
DBV_Waif (14)         - Waif objects
DBV_Last_Move (15)    - last_move property
DBV_Threaded (16)     - Threading
DBV_Bool (17)         - Boolean type (Current)
```

**Supported versions for reading: 4 and 17**

### File Structure (Version 17)
```
[version string]           "** LambdaMOO Database, Format Version 17 **"
[players section]
[pending finalization]
[clocks]
[queued tasks]
[suspended tasks]
[interrupted tasks]
[connections]
[objects section]          <- LARGEST SECTION
[anonymous objects]
[verbs section]
```

## 3. MOO Value Types

15 different types:

```python
class MooTypes(IntEnum):
    INT = 0              # Integer values
    OBJ = 1              # Object references (#12345)
    STR = 2              # Strings
    ERR = 3              # Error codes (E_PERM, E_RANGE, etc.)
    LIST = 4             # Lists (heterogeneous arrays)
    CLEAR = 5            # CLEAR sentinel value
    NONE = 6             # None/nil value
    _CATCH = 7           # Internal: catch continuation
    _FINALLY = 8         # Internal: finally continuation
    FLOAT = 9            # Floating point numbers
    MAP = 10             # Dictionary/maps
    ANON = 12            # Anonymous objects
    WAIF = 13            # Waif objects
    BOOL = 14            # Boolean true/false
```

## 4. Objects Structure

### MooObject Schema
```python
@attrs.define
class MooObject:
    id: int                          # Object ID (#0, #1, #2, ...)
    name: str                        # Object name
    flags: ObjectFlags               # User/programmer/wizard flags
    owner: int                       # Owner object ID
    location: int                    # Where object is located
    parents: list[int]               # Parent object IDs (inheritance)
    children: list[int]              # Child object IDs
    last_move: int                   # Last move timestamp
    contents: list[int]              # Objects contained in this
    verbs: list[Verb]                # Defined verbs
    properties: list[Property]       # Object properties
    propdefs_count: int              # Properties DEFINED (not inherited)
    anon: bool                       # Anonymous object flag
```

### Object Flags
```python
class ObjectFlags(IntFlag):
    USER = 1              # User-player object
    PROGRAMMER = 2        # Can write programs
    WIZARD = 4            # Administrator privileges
    READ = 16             # Object is readable
    WRITE = 32            # Object is writable
    FERTILE = 128         # Can have children
    ANONYMOUS = 256       # Anonymous object
    INVALID = 512         # Invalid/deleted
    RECYCLED = 1024       # Recycled slot
```

## 5. Properties

### Property Schema
```python
@attrs.define
class Property:
    propertyName: str                # e.g., "name", "age"
    value: Any                       # Type-tagged value
    owner: int                       # Who set this property
    perms: PropertyFlags             # Read/write/clear permissions
```

### Property Resolution
1. Read `propdefs_count` property **names** from the database
2. Read total property **values** with owner/perms
3. First `propdefs_count` entries are defined; rest are inherited
4. Properties are resolved through parent chain if needed

## 6. Verbs

### Verb Schema
```python
@attrs.define
class Verb:
    name: str              # Verb name (e.g., "say", "do_something")
    owner: int             # Who owns/created this verb
    perms: int             # Verb permission bits
    preps: int             # Preposition bitmap
    object: int            # Which object this verb lives on
    code: list[str] | None # MOO code lines (None = no program)
```

### Verb Code Storage
- `None` = No program defined
- `[]` = Empty program
- `[...lines...]` = Actual MOO source lines

**Format:**
```
#12345:0                 # Object 12345, verb index 0
[code lines]
.                        # Period terminates code
```

## 7. Serialization Logic

### Reading Process
1. **Version Detection** - Read format version
2. **Sequential Reading**:
   - Players list
   - Pending values
   - Clocks
   - Task queues
   - Objects (largest section)
   - Anonymous objects
   - Verbs

### Type-Tagged Value Reading
```python
def readValue(db, known_type=None) -> Any:
    val_type = readInt()  # Type tag
    match val_type:
        case MooTypes.STR: return readString()
        case MooTypes.OBJ: return readObjnum()
        case MooTypes.INT: return readInt()
        case MooTypes.LIST: return readList()
        case MooTypes.MAP: return readMap()
        # ... 15 types total
```

## 8. Go Implementation Considerations

### Data Structures Needed
```go
type MooObject struct {
    ID          int
    Name        string
    Flags       ObjectFlags
    Owner       int
    Location    int
    Parents     []int
    Children    []int
    LastMove    int64
    Contents    []int
    Verbs       []Verb
    Properties  []Property
    PropDefsCnt int
    IsAnon      bool
}

type Verb struct {
    Name   string
    Owner  int
    Perms  int
    Preps  int
    Object int
    Code   []string
}

type Property struct {
    Name  string
    Value MooValue
    Owner int
    Perms PropertyFlags
}
```

### Critical Constraints
1. **Object ID continuity** - Recycled slots must be preserved
2. **Parent resolution** - Multi-parent support exists
3. **Property inheritance** - Properties inherited from parents
4. **Verb code format** - Stored as text lines
5. **Type coherence** - Type tags are mandatory
6. **Version evolution** - Must handle v4 and v17 formats

### Testing Requirements
- Round-trip test (load → save → load) must produce identical output
- Inheritance chain must be walkable
- Recycled objects must not break numbering
- Property lookup must traverse parent chain correctly
