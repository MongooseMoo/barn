# MOO Database Format Specification

## Overview

MOO databases are text-based files containing all persistent state: objects, properties, verbs, players, and suspended tasks.

**Reference implementation:** `~/src/lambdamoo-db-py/` provides a Python reader/writer.

---

## 1. Format Versions

| Version | Name | Key Features |
|---------|------|--------------|
| 4 | LambdaMOO Classic | Original format |
| 17 | ToastStunt | Maps, WAIFs, booleans, anonymous objects |

Barn targets **version 17** (ToastStunt) for full feature support.

---

## 2. File Structure

### 2.1 Header

```
** LambdaMOO Database, Format Version 17 **
```

### 2.2 Sections (v17 order)

1. Players list
2. Pending finalizations
3. Clocks (obsolete, count = 0)
4. Queued tasks
5. Suspended tasks
6. Interrupted tasks
7. Active connections
8. Object count
9. Objects (with properties and verb metadata)
10. Anonymous objects
11. Verb count
12. Verb code

---

## 3. Value Encoding

Values are type-tagged. Type code on one line, value on next (or inline for simple types).

### 3.1 Type Codes

| Code | Type | Format |
|------|------|--------|
| 0 | INT | Integer on next line |
| 1 | OBJ | `#N` on next line |
| 2 | STR | String on next line |
| 3 | ERR | Error code (integer) on next line |
| 4 | LIST | Length, then N values |
| 5 | CLEAR | No additional data |
| 6 | NONE | No additional data |
| 9 | FLOAT | Float on next line |
| 10 | MAP | Count, then N key-value pairs |
| 12 | ANON | Anonymous object ID |
| 13 | WAIF | WAIF structure |
| 14 | BOOL | 0 or 1 |

### 3.2 Examples

```
0
42
```
(INT 42)

```
2
Hello, world!
```
(STR "Hello, world!")

```
4
3
0
1
0
2
0
3
```
(LIST {1, 2, 3})

---

## 4. Object Format

### 4.1 Object Header

```
#123
Object Name
flags_int
owner_objnum
location_value
last_move_value
contents_list
parents_list
children_list
verb_count
```

### 4.2 Verb Metadata (per verb)

```
verb_name
owner_objnum
perms_int
preps_int
```

### 4.3 Properties

```
propdefs_count        # Properties DEFINED on this object
propname1
propname2
...
total_prop_count      # Including inherited
value1
owner1
perms1
value2
owner2
perms2
...
```

### 4.4 Object Flags

| Bit | Flag | Description |
|-----|------|-------------|
| 0 | USER | Is a player object |
| 1 | PROGRAMMER | Has programmer privileges |
| 2 | WIZARD | Has wizard privileges |
| 4 | READ | Object is readable |
| 5 | WRITE | Object is writable |
| 7 | FERTILE | Can be parent of new objects |
| 8 | ANONYMOUS | Is anonymous object |
| 9 | INVALID | Marked invalid |
| 10 | RECYCLED | Marked recycled |

---

## 5. Verb Code Section

After all objects, verb code appears:

```
#objnum:verbindex
line1
line2
...
.
```

The `.` terminates each verb's code.

---

## 6. Task Persistence

### 6.1 Queued Tasks

```
N queued tasks
```

Each task:
```
unused firstline id starttime
activation...
rtenv...
code...
.
```

### 6.2 Suspended Tasks

```
N suspended tasks
id starttime [type]
task_local_value
vm...
```

### 6.3 VM Structure

```
top_of_stack
activation1
activation2
...
```

---

## 7. Recycled Objects

Recycled objects appear as:

```
# 123 recycled
```

These IDs are tracked for potential reuse.

---

## 8. Compatibility Requirements

### 8.1 Read Support (Required)

Barn MUST read:
- LambdaMOO v4 databases
- ToastStunt v17 databases

Use lambdamoo-db-py as reference for parsing.

### 8.2 Write Support (Required)

Barn MUST write v17 format for:
- Checkpoints
- Panic dumps
- Manual database dumps

### 8.3 Round-Trip Integrity

A database read and immediately written must produce functionally equivalent output (whitespace may differ).

---

## 9. Go Implementation Notes

### 9.1 Encoding

- Use `latin-1` encoding (as original MOO does)
- Line endings: `\n` (Unix style)

### 9.2 Atomic Writes

```go
// Write to temp, rename for atomicity
tmpFile := dbPath + ".tmp"
// ... write to tmpFile ...
os.Rename(tmpFile, dbPath)
```

### 9.3 Large Databases

Stream parsing recommended - don't load entire file to memory.

---

## 10. Reference

The authoritative implementation is:
- **Reader:** `lambdamoo_db/reader.py`
- **Writer:** `lambdamoo_db/writer.py`
- **Types:** `lambdamoo_db/enums.py`
- **Structures:** `lambdamoo_db/database.py`

When in doubt, match lambdamoo-db-py behavior.
