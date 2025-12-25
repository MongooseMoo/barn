# ToastStunt Repository Exploration Report

## What is ToastStunt?

A modern, high-performance C++ fork of LambdaMOO:
- **Origin:** LambdaMOO (Xerox PARC, 1992)
- **First Fork:** Stunt (Todd Sundsted)
- **Current:** ToastStunt (lisdude) - C++ port with modern features

**Location:** `C:/Users/Q/code/mongoose/toaststunt/`
**~40,000+ lines** of C++ source

## Interpreter Architecture

Pipeline:
```
MOO Source → Parser (parser.y) → AST (ast.h) → CodeGen (code_gen.cc)
→ Bytecode (opcode.h) → VM (execute.cc) → Database/Network
```

### Key Components

| File | Size | Purpose |
|------|------|---------|
| `execute.cc` | 4,166 lines | Main VM interpreter |
| `code_gen.cc` | 46KB | AST → bytecode |
| `parser.y` | - | Bison grammar |
| `network.cc` | 57KB | Network I/O |
| `list.cc` | 51KB | List operations |

## Parser/Grammar

- **Bison** (yacc) for syntax
- **gperf** for keyword lookup
- Full MOO 1.8.3+ syntax + extensions

### Control Flow
```moo
if (cond)
elseif (cond)
else
endif

for x in (list)
endfor

while (cond)
endwhile

fork (seconds)
endfork

try
except E_TYPE (err)
finally
endtry
```

## Opcode/Bytecode System

~100 distinct opcodes, stack-based VM.

### Categories
1. **Control** (1 tick): IF, WHILE, FORK, FOR_RANGE
2. **Arithmetic** (1 tick): ADD, MINUS, MULT, DIV, MOD
3. **Comparison** (1 tick): EQ, NE, LT, LE, GT, GE, IN
4. **Logical** (1 tick): AND, OR, NOT
5. **Variables** (0-1 tick): PUSH, PUT, IMM
6. **Lists/Maps** (0 tick): MAKE_EMPTY_LIST, MAP_CREATE
7. **Extended**: CATCH, TRY_EXCEPT, SCATTER

### Tick System
- Non-zero tick opcodes consume 1 "tick"
- Players get configurable ticks/second (default 10,000)
- Prevents DOS and infinite loops

### Optimizations
- Numbers -10 to 143 encoded directly in opcode
- Zero-tick operations for fast paths

## Built-in Functions (~26 categories)

### Registration Modules
1. `register_collection()` - Lists, arrays
2. `register_list()` - List operations (51KB)
3. `register_map()` - Map operations (29KB)
4. `register_numbers()` - Math
5. `register_functions()` - Strings, types
6. `register_objects()` - Object creation
7. `register_property()` - Property ops
8. `register_verbs()` - Verb management
9. `register_execute()` - Task control
10. `register_tasks()` - Task inspection
11. `register_server()` - Server config
12. `register_fileio()` - File I/O (45KB)
13. `register_sqlite()` - SQLite (threaded)
14. `register_pcre()` - Regex (JIT)
15. `register_crypto()` - Hashing
16. `register_argon2()` - Password hashing
17. `register_yajl()` - JSON
18. `register_curl()` - HTTP requests
19. `register_exec()` - Subprocess
20. `register_background()` - Thread pool
21. `register_waif()` - Lightweight objects
22. ... and more

### Function Signature
```c
typedef package (*bf_type)(Var args, Byte nargs, void *data, Objid caller);

// Returns:
// BI_RETURN    - Normal return
// BI_RAISE     - Error raised
// BI_CALL      - Nested verb call
// BI_SUSPEND   - Waiting for I/O
// BI_KILL      - Task killed
```

## VM Execution (execute.cc)

### Activation Frame
```c
typedef struct {
    Program *prog;           // Compiled code
    Var *rt_env;             // Local variables
    Var *base_rt_stack;      // Stack base
    Var *top_rt_stack;       // Stack top
    unsigned pc;             // Program counter
    Var _this;               // Object context
    Objid player;            // Player object
    Objid progr;             // Programmer
} activation;
```

### Execution Outcomes
- `OUTCOME_DONE` - Completed
- `OUTCOME_ABORTED` - Error/killed
- `OUTCOME_BLOCKED` - Waiting I/O

## Type System

```c
// Simple types (stored directly)
TYPE_INT    // 64-bit integer (default) or 32-bit
TYPE_OBJ    // Object ID
TYPE_ERR    // Error code
TYPE_BOOL   // Boolean

// Complex types (allocated separately)
TYPE_STR    // String
TYPE_LIST   // List
TYPE_FLOAT  // Float
TYPE_MAP    // Map/dictionary
TYPE_WAIF   // Lightweight object
TYPE_ANON   // Anonymous object
```

### Error Codes
```c
E_NONE = 0,   E_TYPE = 1,   E_DIV = 2,    E_PERM = 3,
E_PROPNF = 4, E_VERBNF = 5, E_VARNF = 6,  E_INVIND = 7,
E_RECMOVE = 8, E_MAXREC = 9, E_RANGE = 10, E_ARGS = 11,
E_NACC = 12,  E_INVARG = 13, E_QUOTA = 14, E_FLOAT = 15,
E_FILE = 16,  E_EXEC = 17,  E_INTRPT = 18
```

## Database System

### Object Structure
- Object ID (64-bit)
- Parents (multiple inheritance)
- Properties (key-value with permissions)
- Verbs (compiled bytecode)
- Owner, flags

### Persistence
- Periodic checkpointing
- Fork-based non-blocking saves
- Version tracking

## Network

- IPv4/IPv6
- TLS/SSL
- Telnet protocol
- HAProxy support
- TCP keep-alive

## Test Suite

**Location:** `test/`
- Ruby + Test::Unit
- Pre-built Test.db
- Comprehensive coverage

## Modern Features (ToastStunt-specific)

1. 64-bit integers (default)
2. SQLite integration (threaded)
3. PCRE with JIT
4. Argon2id passwords
5. TLS/IPv6 networking
6. Thread pools
7. HTTP parsing
8. Anonymous objects (GC)
9. Waif objects
10. Task profiling

## Dependencies

- YAJL (JSON)
- PCRE (regex)
- SQLite3
- Nettle (crypto)
- libcurl
- Argon2
- OpenSSL
- Aspell

## Go Porting Considerations

1. **Parser:** Use Go parser generator (pigeon/PEG) or hand-write
2. **Bytecode:** Define similar opcode set
3. **VM:** Stack-based execution with frames
4. **Types:** Go interfaces for MOO values
5. **Builtins:** Registry pattern with reflection
6. **Concurrency:** Goroutines instead of threads
7. **Database:** Port lambdamoo-db format
