# Barn Architecture Synthesis

## Overview

Barn is a Go MOO server. This document synthesizes findings from exploring:
- **moo_interp** - Python interpreter (parser, compiler, VM)
- **cow_py** - Python server (networking, tasks, dispatch)
- **ToastStunt** - C++ reference implementation
- **lambdamoo-db** - Database format

## Design Principles

1. **Spec-first, TDD** - Conformance tests ARE the specification
2. **Layered architecture** - Clear boundaries between components
3. **Go idioms** - Not a direct port; use Go's strengths
4. **Incremental** - Start simple, add complexity as tests demand

## Component Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         BARN                                 │
├─────────────────────────────────────────────────────────────┤
│  Network Layer (net/http, goroutines)                       │
├─────────────────────────────────────────────────────────────┤
│  Server Layer                                                │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────┐ │
│  │ Connection   │ │ Command      │ │ Task                 │ │
│  │ Manager      │ │ Parser       │ │ Scheduler            │ │
│  └──────────────┘ └──────────────┘ └──────────────────────┘ │
├─────────────────────────────────────────────────────────────┤
│  Interpreter Layer                                           │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────┐ │
│  │ Parser       │ │ Compiler     │ │ VM                   │ │
│  │ (MOO→AST)    │ │ (AST→Bytec.) │ │ (Execute)            │ │
│  └──────────────┘ └──────────────┘ └──────────────────────┘ │
│  ┌──────────────┐ ┌──────────────┐                          │
│  │ Builtins     │ │ Types        │                          │
│  │ Registry     │ │ (MOO values) │                          │
│  └──────────────┘ └──────────────┘                          │
├─────────────────────────────────────────────────────────────┤
│  Database Layer                                              │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────┐ │
│  │ Reader       │ │ Writer       │ │ Object Store         │ │
│  │ (v4, v17)    │ │              │ │ (in-memory)          │ │
│  └──────────────┘ └──────────────┘ └──────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## Implementation Phases

### Phase 1: Core Types & Parser (TDD Foundation)

**Goal:** Parse MOO expressions, run basic conformance tests.

**Components:**
1. **MOO Types** (`types/`)
   - `Value` interface for all MOO values
   - Concrete types: Int, Float, Str, Obj, List, Map, Err, Bool
   - 1-based indexing for strings/lists
   - Error codes enum

2. **Parser** (`parser/`)
   - Start with Participle (struct-tag based, idiomatic Go)
   - Fallback: hand-written Pratt parser for expressions
   - Grammar based on moo_interp's `parser.lark`

3. **AST** (`ast/`)
   - Node interface with position tracking
   - Expression nodes: Binary, Unary, Literal, Identifier, etc.
   - Statement nodes: If, While, For, Return, etc.

**Tests:** `basic/arithmetic.yaml`, `basic/value.yaml`

### Phase 2: Tree-Walk Interpreter

**Goal:** Execute simple programs, expand test coverage.

**Components:**
1. **Evaluator** (`eval/`)
   - Visitor pattern over AST
   - Environment for variable scoping
   - Basic arithmetic, comparisons, logic

2. **Builtins** (`builtins/`)
   - Registry pattern (map name → function)
   - Type conversion: `tostr`, `toint`, `tofloat`
   - String ops: `length`, `index`, `strsub`
   - List ops: `listappend`, `listdelete`

**Tests:** `basic/string.yaml`, `basic/list.yaml`, `builtins/primitives.yaml`

### Phase 3: Control Flow & Scoping

**Goal:** Loops, conditionals, exception handling.

**Components:**
1. Extended evaluator for:
   - `if/elseif/else/endif`
   - `for/endfor` (list iteration, range)
   - `while/endwhile`
   - `try/except/finally/endtry`
   - `break/continue` with labels

**Tests:** `language/looping.yaml`, `builtins/algorithms.yaml`

### Phase 4: Objects & Properties

**Goal:** Object model, property access, verb calls.

**Components:**
1. **Database** (`db/`)
   - In-memory object store
   - Property resolution (inheritance chain)
   - Verb lookup with inheritance

2. **Extended evaluator:**
   - Property access: `obj.prop`, `obj.(expr)`
   - Verb calls: `obj:verb(args)`
   - Dollar notation: `$system`

**Tests:** `basic/object.yaml`, `basic/property.yaml`

### Phase 5: Database Persistence

**Goal:** Read/write LambdaMOO database files.

**Components:**
1. **Reader** (`db/reader.go`)
   - Version 4 and 17 support
   - Type-tagged value parsing
   - Object/property/verb deserialization

2. **Writer** (`db/writer.go`)
   - Round-trip fidelity
   - Checkpoint support

### Phase 6: Bytecode VM (Optimization)

**Goal:** Replace tree-walk with faster bytecode execution.

**Components:**
1. **Opcodes** (`vm/opcodes.go`)
   - ~50 core opcodes (control, arithmetic, stack)
   - Extended opcodes for advanced features
   - Optimized number encoding

2. **Compiler** (`compiler/`)
   - AST → bytecode transformation
   - Constant pool
   - Jump target resolution

3. **VM** (`vm/`)
   - Stack-based execution
   - Call frames with local environments
   - Exception stack for try/catch

### Phase 7: Server Layer

**Goal:** Network server, player connections, command dispatch.

**Components:**
1. **Server** (`server/`)
   - TCP listener with goroutines
   - Connection → player mapping
   - Command parsing (verb lookup)

2. **Task Runner** (`server/task.go`)
   - Execute verbs through interpreter
   - Context: player, this, caller, args
   - Tick limiting

3. **Server Builtins** (`server/builtins.go`)
   - `notify`, `connected_players`
   - `create`, `recycle`, `move`
   - Task management

## Key Design Decisions

### 1. Parser Choice: Participle First

**Why:**
- Idiomatic Go (struct tags like `encoding/json`)
- No code generation step
- Easy to iterate and debug
- Can switch to hand-written if needed

**Risk mitigation:**
- MOO has unusual syntax (`` `expr ! err` ``, `{a, ?b, @c}`)
- May need custom tokenizer or fallback to Pratt parser for expressions

### 2. Tree-Walk Before Bytecode

**Why:**
- Simpler to implement and debug
- Get semantics right first
- Conformance tests don't care about speed
- Bytecode is optimization, not correctness

### 3. Interface-Based Values

```go
type Value interface {
    Type() Type
    String() string
    Equal(Value) bool
}

type Int struct {
    val int64
}

type List struct {
    elements []Value
    // 1-based indexing handled in accessor methods
}
```

**Why:**
- MOO is dynamically typed
- Interface allows type switches
- Each type can implement MOO-specific behavior

### 4. Builtin Registry

```go
type BuiltinFunc func(args []Value) (Value, error)

var builtins = map[string]BuiltinFunc{
    "length":     builtinLength,
    "tostr":      builtinTostr,
    // ...
}
```

**Why:**
- Easy to add builtins incrementally
- Can validate arg counts/types
- Matches moo_interp pattern

### 5. Conformance Test Driver

```go
type TestCase struct {
    Name       string
    Code       string      // expression
    Statement  string      // multi-line
    Expect     Expectation
}

func RunTest(tc TestCase, interp *Interpreter) error {
    result, err := interp.Eval(tc.Code)
    return tc.Expect.Check(result, err)
}
```

**Why:**
- YAML tests are language-agnostic
- Same tests for Python and Go
- Spec-driven development

## Go-Specific Patterns

### Error Handling

MOO errors (E_TYPE, E_DIV) are values, not Go errors:

```go
type MooError struct {
    Code ErrorCode
    Msg  string
}

// Implement Value interface
func (e MooError) Type() Type { return TypeError }
```

Runtime errors (parse failure, VM crash) are Go errors.

### 1-Based Indexing

```go
func (l *List) Get(idx int) (Value, error) {
    if idx < 1 || idx > len(l.elements) {
        return nil, E_RANGE
    }
    return l.elements[idx-1], nil
}
```

### Copy-on-Write

```go
type List struct {
    elements []Value
    refcount int32
}

func (l *List) Set(idx int, v Value) *List {
    if l.refcount > 1 {
        // Make a copy
        newList := l.shallowCopy()
        newList.elements[idx-1] = v
        return newList
    }
    l.elements[idx-1] = v
    return l
}
```

## Directory Structure

```
barn/
├── cmd/
│   └── barn/
│       └── main.go           # CLI entry point
├── types/
│   ├── value.go              # Value interface
│   ├── int.go, str.go, ...   # Concrete types
│   └── error.go              # MOO error codes
├── parser/
│   ├── lexer.go              # Tokenization
│   ├── parser.go             # Grammar → AST
│   └── ast.go                # AST node types
├── eval/
│   └── eval.go               # Tree-walk interpreter
├── builtins/
│   ├── registry.go           # Builtin registration
│   ├── strings.go            # String builtins
│   ├── lists.go              # List builtins
│   └── ...
├── vm/                        # Phase 6
│   ├── opcodes.go
│   ├── compiler.go
│   └── vm.go
├── db/
│   ├── types.go              # MooObject, Verb, Property
│   ├── reader.go             # Parse .db files
│   ├── writer.go             # Write .db files
│   └── store.go              # In-memory storage
├── server/                    # Phase 7
│   ├── server.go
│   ├── connection.go
│   └── builtins.go
├── conformance/
│   ├── runner.go             # YAML test runner
│   └── schema.go             # Test case types
├── notes/                     # This directory
├── reports/                   # Exploration reports
└── go.mod
```

## Testing Strategy

### Unit Tests

Each package has `*_test.go` files:
- `types/int_test.go` - integer operations
- `parser/parser_test.go` - grammar edge cases
- `builtins/strings_test.go` - builtin behavior

### Conformance Tests

```bash
go test ./conformance/... -v
```

Runs all YAML tests from `cow_py/tests/conformance/`:
- `basic/*.yaml` - Type and operator tests
- `builtins/*.yaml` - Builtin function tests
- `language/*.yaml` - Control flow tests

### Integration Tests

Full interpreter tests:
- Parse → compile → execute → verify result
- Database round-trip tests
- Server command dispatch tests

## Open Questions

1. **Parser fallback:** When does Participle fail for MOO syntax?
2. **Goroutine model:** One per connection? Task pool?
3. **Database locking:** How to handle concurrent access?
4. **Waif/Anon objects:** Defer to later phase?
5. **Thread pool builtins:** `background()` in ToastStunt

## Next Steps

1. Initialize Go module (`go mod init barn`)
2. Create `types/` package with Value interface
3. Write first conformance test runner (just loads YAML)
4. Implement Int type, pass `basic/arithmetic.yaml` subset
5. Iterate: add types/builtins as tests demand
