# cow_py Repository Exploration Report

## 1. What Is This?

**cow_py** is a Python MOO (MUD Object Oriented) server implementation. It's the server component that:
- Accepts network connections
- Dispatches commands to verbs
- Executes MOO code through moo_interp
- Manages database persistence

**Repository Location:** `C:/Users/Q/code/cow_py`

## 2. Architecture

Layered design:
```
Network Layer (gevent StreamServer)
    ↓
Connection Management (socket ↔ player mapping)
    ↓
Command Parsing (input → verb + args)
    ↓
Verb Dispatch (find verb in object hierarchy)
    ↓
Task Execution (runs verb through moo_interp VM)
    ↓
Database Persistence (saves to disk)
```

## 3. Key Components

| File | Lines | Purpose |
|------|-------|---------|
| `server.py` | 556 | Main server (gevent StreamServer) |
| `connection.py` | - | Connection management |
| `scheduler.py` | - | Task queuing |
| `task_runner.py` | 292 | Executes verbs through moo_interp |
| `command.py` | - | Parses player input |
| `verbs.py` | - | Verb lookup in hierarchy |
| `persistence.py` | - | Database autosave |
| `context.py` | - | ExecutionContext (player, this, caller, args) |
| `server_builtins.py` | 1535 | Server-specific builtins |

## 4. Connection Flow

1. **Accept** → `MooServer._handle_connection()` spawns greenlet
2. **Connection Created** → `ConnectionManager.new_connection()` assigns ID
3. **Login** → Login verb sets `connection.player`
4. **Input Loop** → socket → `parse_command()` → `find_verb()` → Task → `TaskRunner.run()`
5. **Output** → `notify()` builtin → messages queued → sent to socket

## 5. Interpreter Integration

Depends on **moo_interp** (separate repo).

**Integration Points:**
1. `moo_interp.moo_ast.compile()` - verb code → bytecode
2. `moo_interp.vm.VM` - executes bytecode
3. Context variables: `player, this, caller, verb, args, argstr, dobj, dobjstr, iobj, iobjstr, prepstr`

## 6. Database Handling

**Three layers:**
1. **lambdamoo-db** - Reads/writes `.db` files
2. **Persistence** - Dirty-flag tracking, autosave
3. **Server** - Passes DB to TaskRunner for verb lookup

## 7. Conformance Tests (CRITICAL for TDD)

**Location:** `C:/Users/Q/code/cow_py/tests/conformance/`

**Stats:** 27 YAML files, ~8,359 lines

### Categories
```
basic/      (6 files): arithmetic, list, object, property, string, value
builtins/   (13 files): create, json, map, primitives, algorithms, etc.
language/   (6 files): anonymous, equality, looping, waif, moocode_parsing
server/     (2 files): exec, limits
```

### YAML Schema
```yaml
name: suite_name
description: "..."

setup:
  permission: wizard
  code: |
    $test_obj = create($nothing);

teardown:
  permission: wizard
  code: |
    recycle($test_obj);

tests:
  - name: test_name
    permission: programmer
    skip: false
    skip_if: "condition"

    # ONE of:
    code: "1 + 1"              # Expression
    statement: |               # Statements
      x = 5;
      return x * 2;
    verb: "#0:verb_name"       # Verb call

    # Expected - ONE of:
    expect:
      value: 2
      error: E_DIV
      type: int
      match: "regex.*"
      contains: "needle"
      range: [1, 100]
      notifications: ["msg"]
```

### Error Codes
```
E_TYPE, E_DIV, E_PERM, E_PROPNF, E_VERBNF, E_VARNF, E_INVIND,
E_RANGE, E_ARGS, E_INVARG, E_RECMOVE, E_MAXREC, E_NACC, E_QUOTA,
E_FLOAT, E_FILE, E_EXEC, E_INTRPT
```

## 8. Test Infrastructure

**Pipeline:**
```
conftest.py (pytest fixtures)
    ↓
test_conformance.py (test discovery)
    ↓
runner.py (YamlTestRunner)
    ↓
transport.py (DirectTransport or SocketTransport)
```

**Key Files:**
- `conftest.py` - Fixtures, YAML discovery
- `schema.py` - Dataclasses (MooTestSuite, MooTestCase, Expectation)
- `runner.py` - Test execution
- `transport.py` - Direct (in-process) or Socket (TCP)

**Running:**
```bash
# All tests
uv run pytest tests/conformance/ -v

# Specific category
uv run pytest tests/conformance/ -k "arithmetic"

# Against server
uv run pytest tests/conformance/ --transport=socket --moo-port=7777
```

## 9. Server Builtins

**Core** (from moo_interp):
- Math, strings, lists, maps, JSON, eval

**Server** (from server_builtins.py):
```
notify(player, message)
connected_players()
task_id()
queued_tasks()
kill_task(task_id)
boot_player(player)
valid(object)
create(parent, [owner])
recycle(object)
parent(object)
children(object)
move(object, location)
property_info(object, property)
add_property(object, name, value, perms)
delete_property(object, property)
```

## 10. Dependencies

```python
dependencies = [
    "gevent>=25.9.1",          # Cooperative multitasking
    "lambdamoo-db",            # DB read/write
    "moo-interp",              # Interpreter
    "click>=8.0",              # CLI framework
    "jmespath>=1.0",           # JSON path queries
]
```

## 11. Go Implementation Patterns

**What to replicate:**
1. **Command Parsing** - Input → verb + args
2. **Verb Dispatch** - Recursive object hierarchy search
3. **Task/Context Management** - Execution state
4. **Builtin Registry** - Pluggable from multiple sources
5. **Database Integration** - Object/property/verb lookup
6. **Permission System** - Wizard/programmer/user levels
7. **Concurrency** - Goroutines instead of greenlets

## 12. Key Files for Reference

- `tests/conformance/schema.py` - Test data structures
- `tests/conformance/runner.py` - Test execution
- `tests/conformance/CONVERTING.md` - Converting Ruby tests
- `src/cow_py/server.py` - Server architecture
