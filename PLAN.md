# PLAN.md - Go MOO Server Implementation

## How To Use This Plan

- Each **Phase** requires human approval before proceeding (marked with **GATE**)
- Each **Layer** has a test verification step - tests MUST pass before continuing
- **Handoff points** document state for session resume
- Read **spec refs** BEFORE implementing
- Implementation details come from specs, not this plan

## Progress Tracking

**CRITICAL:** Update this section as you work. Check boxes when complete. This is your resume point if interrupted.

### Current State
- **Active Phase:** Phase 6: Exception Handling (Layers 6.1-6.3, 6.5 COMPLETE)
- **Active Layer:** At Phase 6 GATE (6.4 deferred)
- **Last Test Run:** Exception handling tests passing; go build ./... passing
- **Note:** Layer 6.4 (Catch Expression) deferred - requires backtick/quote delimiter support

### Phase Checklist

- [x] **Phase 0: Foundation**
  - [x] 0.1 Project Setup
  - [x] 0.2 Conformance Test Runner
  - [x] 0.3 Execution Context
  - [ ] GATE APPROVED
- [x] **Phase 1: Types & Literal Parsing**
  - [x] 1.1 Value Interface + Lexer Foundation
  - [x] 1.2 INT Type + Integer Literals
  - [x] 1.3 FLOAT Type + Float Literals
  - [x] 1.4 STR Type + String Literals
  - [x] 1.5 ERR Type + Error Literals
  - [x] 1.6 OBJ Type + Object Literals
  - [x] 1.7 BOOL Type + Boolean Keywords
  - [x] 1.8 LIST Type + List Literals
  - [x] 1.9 MAP Type + Map Literals
  - [ ] GATE APPROVED
- [x] **Phase 2: Operators & Expressions**
  - [x] 2.1 AST Node Types + Operator Tokens
  - [x] 2.2 Unary Operators
  - [x] 2.3 Binary Operators (Arithmetic)
  - [x] 2.4 Comparison Operators
  - [x] 2.5 Logical Operators
  - [x] 2.6 Bitwise Operators
  - [x] 2.7 Ternary + Parentheses
  - [ ] GATE APPROVED
- [x] **Phase 3: Tree-Walk Evaluator**
  - [x] 3.1 Evaluator Scaffold
  - [x] 3.2 Expression Evaluation
  - [x] 3.3 Variable Binding
  - [x] 3.4 Type Conversion Builtins
  - [ ] GATE APPROVED
- [x] **Phase 4: Collections & Indexing**
  - [x] 4.1 List Indexing
  - [x] 4.2 Range Expressions
  - [x] 4.3 String Indexing
  - [x] 4.4 Map Indexing
  - [ ] 4.5 Splice Operator
  - [x] GATE APPROVED
- [ ] **Phase 5: Control Flow**
  - [ ] 5.1 If/Elseif/Else
  - [ ] 5.2 While Loops
  - [ ] 5.3 For Loops (List)
  - [ ] 5.4 For Loops (Range)
  - [ ] 5.5 For Loops (Map)
  - [ ] 5.6 Break/Continue
  - [ ] 5.7 Named Loops
  - [ ] GATE APPROVED
- [x] **Phase 6: Exception Handling**
  - [x] 6.1 Error Raising
  - [x] 6.2 Try/Except
  - [x] 6.3 Try/Finally
  - [ ] 6.4 Catch Expression (deferred - not needed for gate)
  - [x] 6.5 Scatter Assignment
  - [ ] GATE APPROVED
- [ ] **Phase 7: Core Builtins**
  - [ ] 7.1 String Builtins
  - [ ] 7.2 List Builtins
  - [ ] 7.3 Math Builtins
  - [ ] 7.4 Algorithm Builtins
  - [ ] 7.5 Map Builtins
  - [ ] GATE APPROVED
- [ ] **Phase 8: Object System**
  - [ ] 8.1 In-Memory Object Store
  - [ ] 8.2 Property Access
  - [ ] 8.3 Property Inheritance
  - [ ] 8.4 Object Creation/Recycling
  - [ ] 8.5 Object Builtins
  - [ ] 8.6 Property Builtins
  - [ ] GATE APPROVED
- [ ] **Phase 9: Verb System**
  - [ ] 9.1 Verb Storage and Lookup
  - [ ] 9.2 Verb Compilation
  - [ ] 9.3 Verb Calls
  - [ ] 9.4 Dollar Notation
  - [ ] 9.5 Verb Builtins
  - [ ] GATE APPROVED
- [ ] **Phase 10: Advanced Features**
  - [ ] 10.1 JSON Builtins
  - [ ] 10.2 eval() Builtin
  - [ ] 10.3 Anonymous Objects/Functions
  - [ ] 10.4 WAIFs
  - [ ] GATE APPROVED
- [ ] **Phase 11: Bytecode VM** (optional)
  - [ ] 11.1 Opcode Definitions
  - [ ] 11.2 Compiler
  - [ ] 11.3 VM Execution Loop
  - [ ] 11.4 Replace Tree-Walk
  - [ ] GATE APPROVED
- [ ] **Phase 12: Server Infrastructure**
  - [ ] 12.1 Database Reader
  - [ ] 12.2 Task Scheduler
  - [ ] 12.3 Server Main Loop
  - [ ] 12.4 Connection Handling
  - [ ] 12.5 Network Builtins
  - [ ] COMPLETE

## Prerequisites

- Go 1.21+
- Access to `barn/spec/` (language specification)
- Access to `cow_py/tests/conformance/` (test suite - see note below)
- Reference: `notes/architecture_synthesis.md` for directory structure
- Reference: `notes/go_interpreter_patterns.md` for parser/evaluator patterns

**Note on test files:** Conformance tests live in `cow_py/tests/conformance/`, NOT in barn/. This is intentional - tests are shared between the Python (cow_py) and Go (barn) implementations. The Go test runner (Phase 0.2) loads these external YAML files. Do not copy tests into barn/; both implementations validate against the same test suite.

## Test Verification

Tests run via cow_py's conformance test runner. During Go implementation, create a Go test runner that executes the same YAML test files.

```bash
# Python runner (reference)
cd ~/code/cow_py && uv run pytest tests/conformance/ -v

# Go runner (to be built in Phase 0)
cd ~/code/barn && go test ./conformance/... -v
```

---

## Test → Layer Mapping

| Layer | Test Files | Total | Active |
|-------|-----------|-------|--------|
| 1.x Types & Literals | basic/value.yaml | 28 | 28 |
| 2.x Operators | basic/arithmetic.yaml, language/equality.yaml | 20 | 20 |
| 3.x Evaluator | builtins/primitives.yaml (subset) | ~10 | ~10 |
| 4.x Indexing | language/index_and_range.yaml, basic/list.yaml | 137 | 135 |
| 5.x Control | language/looping.yaml | 29 | 19 |
| 6.x Exceptions | language/moocode_parsing.yaml | 62 | 50 |
| 7.x Builtins | basic/string.yaml, builtins/*.yaml | 400+ | 350+ |
| 8.x Objects | basic/object.yaml, basic/property.yaml, builtins/objects.yaml | 184 | 167 |
| 10.x Advanced | builtins/json.yaml, language/anonymous.yaml, language/waif.yaml | 219 | 156 |
| 12.x Server | server/*.yaml | 58 | 0 |

**Total:** 1,110 tests (905 active, 205 skipped)

---

## Phase 0: Foundation

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 1**

No MOO tests yet - this phase sets up the project structure and test runner.

### Layer 0.1: Project Setup

**Spec Refs:** `notes/architecture_synthesis.md` (Directory Structure section)

#### Tasks
1. Initialize Go module: `go mod init barn`
2. Create directory structure per architecture_synthesis.md:
   - `cmd/barn/` - CLI entry point
   - `types/` - MOO value types
   - `parser/` - Lexer and parser
   - `eval/` - Tree-walk evaluator
   - `builtins/` - Builtin function registry
   - `db/` - Object store and database
   - `conformance/` - Test runner
3. Create placeholder `cmd/barn/main.go`

#### Done Criteria
- [ ] `go build ./...` succeeds
- [ ] Directory structure matches architecture_synthesis.md

### Layer 0.2: Conformance Test Runner

**Spec Refs:** `notes/conformance_test_format.md` (lines 120-180: schema definition)

#### Dependencies
- YAML library: `go get gopkg.in/yaml.v3`
- Test files location: `../cow_py/tests/conformance/` (relative to barn/)

#### Test Schema (from conformance_test_format.md)
```go
type TestCase struct {
    Name        string            `yaml:"name"`
    Description string            `yaml:"description,omitempty"`
    Skip        interface{}       `yaml:"skip,omitempty"`      // bool or string
    SkipIf      string            `yaml:"skip_if,omitempty"`
    Permission  string            `yaml:"permission,omitempty"` // programmer|wizard
    Code        string            `yaml:"code,omitempty"`       // expression (wrapped in return)
    Statement   string            `yaml:"statement,omitempty"`  // explicit statements
    Verb        string            `yaml:"verb,omitempty"`       // #0:verb_name
    Setup       *SetupBlock       `yaml:"setup,omitempty"`
    Teardown    *SetupBlock       `yaml:"teardown,omitempty"`
    Expect      Expectation       `yaml:"expect"`
}

type Expectation struct {
    Value    interface{} `yaml:"value,omitempty"`    // exact match
    Error    string      `yaml:"error,omitempty"`    // E_TYPE, E_DIV, etc.
    Type     string      `yaml:"type,omitempty"`     // int, str, list, etc.
    Match    string      `yaml:"match,omitempty"`    // regex
    Contains interface{} `yaml:"contains,omitempty"` // list/string contains
}
```

#### Tasks
1. Create `conformance/schema.go` - Types above + YAML unmarshaling
2. Create `conformance/loader.go` - Walk `../cow_py/tests/conformance/**/*.yaml`
3. Create `conformance/runner.go` - Execute test: parse code, evaluate, check expectation
4. Create `conformance/conformance_test.go` - Go test integration with `t.Run()`

#### Done Criteria
- [ ] `go get gopkg.in/yaml.v3` succeeds
- [ ] Loader finds all 27 YAML files
- [ ] Loader parses all 1,110 test cases without YAML errors
- [ ] `go test ./conformance/... -v` runs, shows "0 passed, 1110 skipped" (no interpreter yet)

#### Handoff State
```
Files Created: go.mod, cmd/barn/main.go, conformance/*.go, types/, parser/, eval/, builtins/, db/
Test Status: 0/1110 passing, 1110 skipped (framework only, no interpreter)
Next: Layer 0.3 - Execution Context
```

### Layer 0.3: Execution Context + Foundational Types

**Rationale:** Pass context through the evaluator from day 1. Avoids rewriting every Eval() signature when adding tick limits (Phase 5) and permissions (Phase 9).

**Note:** This layer defines foundational types (ObjID, ErrorCode, Value) that later layers build upon. These are minimal stubs that get fleshed out in Phase 1.

#### Tasks
1. Create `types/base.go` - Foundational type definitions:
   ```go
   // ObjID represents a MOO object reference
   // -1 = nothing, -2 = ambiguous, 0+ = valid object
   type ObjID int64

   const (
       ObjNothing   ObjID = -1
       ObjAmbiguous ObjID = -2
   )

   // ErrorCode represents a MOO error type (E_TYPE, E_DIV, etc.)
   type ErrorCode int

   // Error codes - values from spec/errors.md
   const (
       E_NONE    ErrorCode = 0
       E_TYPE    ErrorCode = 1
       E_DIV     ErrorCode = 2
       E_PERM    ErrorCode = 3
       E_PROPNF  ErrorCode = 4
       E_VERBNF  ErrorCode = 5
       E_VARNF   ErrorCode = 6
       E_INVIND  ErrorCode = 7
       E_RECMOVE ErrorCode = 8
       E_MAXREC  ErrorCode = 9
       E_RANGE   ErrorCode = 10
       E_ARGS    ErrorCode = 11
       E_NACC    ErrorCode = 12
       E_INVARG  ErrorCode = 13
       E_QUOTA   ErrorCode = 14
       E_FLOAT   ErrorCode = 15
       E_FILE    ErrorCode = 16
       E_EXEC    ErrorCode = 17
   )

   // Value is the interface all MOO values implement
   // Stub here - Layer 1.1 adds full methods
   type Value interface{}
   ```

2. Create `types/context.go`:
   ```go
   type TaskContext struct {
       TicksRemaining int64       // Infinite loop protection
       Player         ObjID       // Current player
       Programmer     ObjID       // Effective permissions
       ThisObj        ObjID       // Current `this`
       Verb           string      // Current verb name
   }

   func NewTaskContext() *TaskContext {
       return &TaskContext{TicksRemaining: 30000} // Default tick limit
   }
   ```

3. Create `types/result.go` - Unified control flow:
   ```go
   type ControlFlow int
   const (
       FlowNormal ControlFlow = iota
       FlowReturn
       FlowBreak
       FlowContinue
       FlowException  // MOO error being raised
   )

   type Result struct {
       Val   Value
       Flow  ControlFlow
       Error ErrorCode  // Only set when Flow == FlowException
   }

   func Ok(v Value) Result { return Result{Val: v, Flow: FlowNormal} }
   func Err(e ErrorCode) Result { return Result{Flow: FlowException, Error: e} }
   ```

#### Done Criteria
- [ ] All types compile: ObjID, ErrorCode, Value, TaskContext, Result
- [ ] Error code constants match spec/errors.md
- [ ] Unit tests verify default tick limit

#### Handoff State
```
Files Created: types/base.go, types/context.go, types/result.go
Next: Phase 1 - Types & Literal Parsing
```

---

## Phase 1: Types & Literal Parsing

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 2**

This phase builds types AND their literal syntax together. Each layer adds a type and teaches the parser to recognize its literal form. By the end, the parser can handle all MOO literals and basic/value.yaml tests pass.

### Layer 1.1: Value Interface + Lexer Foundation

**Spec Refs:**
- `spec/types.md` (lines 1-30: type code table)
- `spec/grammar.md` (lines 1-50: token types)
- `notes/go_interpreter_patterns.md` (parser approach: hand-written Pratt parser for expressions)

**Note:** Layer 0.3 defined `type Value interface{}` as a stub in `types/base.go`. This layer replaces it with the full interface (keep in same file or move to `types/value.go`).

#### Tasks
1. Update `types/base.go` (or create `types/value.go`) - replace Value stub:
   ```go
   type Value interface {
       Type() TypeCode
       String() string    // MOO literal representation
       Equal(Value) bool  // Deep equality
       Truthy() bool      // MOO truthiness rules
   }
   ```
2. Create `types/typecode.go` - enum matching spec/types.md:
   - TYPE_INT=0, TYPE_OBJ=1, TYPE_STR=2, TYPE_ERR=3, TYPE_LIST=4
   - TYPE_FLOAT=9, TYPE_MAP=10, TYPE_WAIF=13, TYPE_BOOL=14
3. Create `parser/token.go` - Token struct with type, value, position
4. Create `parser/lexer.go` - Lexer skeleton with Scan() method

#### Done Criteria
- [ ] TypeCode enum matches spec/types.md exactly
- [ ] Lexer can be instantiated with source string
- [ ] Unit tests verify type codes

### Layer 1.2: INT Type + Integer Literals

**Spec Refs:** `spec/types.md` (lines 31-45: INT section), `spec/grammar.md` (integer literals)

#### Tasks
1. Create `types/int.go` - IntValue as int64
2. Add lexer rule for integer literals: `-?[0-9]+`
3. Create `parser/parser.go` with ParseLiteral() method
4. Parse integer literals into IntValue

#### Done Criteria
- [ ] Lexer tokenizes: `42`, `-5`, `0`, `9223372036854775807`
- [ ] Parser: `ParseLiteral("42")` returns `IntValue(42)`
- [ ] IntValue(0).Truthy() == false
- [ ] IntValue(1).Truthy() == true

### Layer 1.3: FLOAT Type + Float Literals

**Spec Refs:** `spec/types.md` (lines 46-65: FLOAT section), `spec/grammar.md` (float literals)

#### Tasks
1. Create `types/float.go` - FloatValue as float64
2. Add lexer rule for floats (all valid forms):
   - With decimal: `3.14`, `-0.5`, `.5`
   - With exponent: `1e10`, `1E-5`, `3.14e+2`
   - Combined: `1.5e-3`
   - Regex: `-?([0-9]+\.?[0-9]*|[0-9]*\.[0-9]+)([eE][+-]?[0-9]+)?`
3. Extend parser to handle float literals
4. Handle NaN/Infinity: these raise E_FLOAT per spec

#### Done Criteria
- [ ] Lexer tokenizes: `3.14`, `-0.5`, `1e10`, `1.5e-3`
- [ ] Parser: `ParseLiteral("3.14")` returns `FloatValue(3.14)`
- [ ] FloatValue(0.0).Truthy() == false
- [ ] NaN/Infinity detection ready (will raise E_FLOAT in evaluator)

### Layer 1.4: STR Type + String Literals

**Spec Refs:**
- `spec/types.md` (lines 66-95: STR section, escape sequences table)
- `spec/grammar.md` (string literals)

#### Escape Sequences (from spec/types.md)
| Escape | Meaning |
|--------|---------|
| `\\` | backslash |
| `\"` | double quote |
| `\n` | newline |
| `\t` | tab |
| `\r` | carriage return |
| `\xHH` | hex byte |

#### Tasks
1. Create `types/str.go` - StrValue (wrapper around Go string)
2. Add lexer rule for strings: `"([^"\\]|\\.)*"`
3. Implement escape sequence processing in lexer
4. Implement 1-based indexing methods (for later use)

#### Done Criteria
- [ ] Lexer tokenizes: `"hello"`, `"with \"quotes\""`, `"line\nbreak"`
- [ ] Parser: `ParseLiteral("\"hello\"")` returns `StrValue("hello")`
- [ ] Escape sequences decoded correctly
- [ ] Empty string `""` works

### Layer 1.5: ERR Type + Error Literals

**Spec Refs:** `spec/errors.md` (all 18 error codes with names and values)

#### Error Codes (from spec/errors.md)
| Code | Value | Meaning |
|------|-------|---------|
| E_NONE | 0 | No error |
| E_TYPE | 1 | Type mismatch |
| E_DIV | 2 | Division by zero |
| E_PERM | 3 | Permission denied |
| E_PROPNF | 4 | Property not found |
| E_VERBNF | 5 | Verb not found |
| E_VARNF | 6 | Variable not found |
| E_INVIND | 7 | Invalid index |
| E_RECMOVE | 8 | Recursive move |
| E_MAXREC | 9 | Max recursion |
| E_RANGE | 10 | Range error |
| E_ARGS | 11 | Wrong args |
| E_NACC | 12 | Move refused |
| E_INVARG | 13 | Invalid argument |
| E_QUOTA | 14 | Quota exceeded |
| E_FLOAT | 15 | Float error |
| E_FILE | 16 | File error |
| E_EXEC | 17 | Exec error |

#### Tasks
1. Create `types/error.go` - ErrValue with error code
2. Add lexer rule for error literals: `E_[A-Z]+`
3. Map error names to codes per table above
4. ErrValue.Truthy() returns true (errors are truthy)

#### Done Criteria
- [ ] Lexer tokenizes: `E_TYPE`, `E_DIV`, `E_RANGE`
- [ ] Parser: `ParseLiteral("E_TYPE")` returns `ErrValue(E_TYPE)`
- [ ] All 18 error codes defined and parseable
- [ ] ErrValue.String() returns `"E_TYPE"` format

### Layer 1.6: OBJ Type + Object Literals

**Spec Refs:** `spec/types.md` (lines 96-115: OBJ section)

#### Special Objects
| Literal | Value | Meaning |
|---------|-------|---------|
| #-1 | NOTHING | No object |
| #-2 | AMBIGUOUS | Ambiguous match |
| #-3 | FAILED_MATCH | Match failed |

#### Tasks
1. Create `types/obj.go` - ObjValue as int64
2. Add lexer rule for object literals: `#-?[0-9]+`
3. Define NOTHING, AMBIGUOUS, FAILED_MATCH constants

#### Done Criteria
- [ ] Lexer tokenizes: `#0`, `#123`, `#-1`
- [ ] Parser: `ParseLiteral("#42")` returns `ObjValue(42)`
- [ ] ObjValue.String() returns `"#42"` format
- [ ] Special object constants defined

### Layer 1.7: BOOL Type + Boolean Keywords

**Spec Refs:** `spec/types.md` (BOOL section), `spec/grammar.md` (keywords)

#### Tasks
1. Create `types/bool.go` - BoolValue as bool
2. Add lexer keywords: `true`, `false`
3. Note: These are keywords, not identifiers

#### Done Criteria
- [ ] Lexer tokenizes `true` as KEYWORD_TRUE, `false` as KEYWORD_FALSE
- [ ] Parser: `ParseLiteral("true")` returns `BoolValue(true)`
- [ ] BoolValue(false).Truthy() == false
- [ ] BoolValue(true).Truthy() == true

### Layer 1.8: LIST Type + List Literals

**Spec Refs:**
- `spec/types.md` (lines 116-145: LIST section)
- `spec/grammar.md` (list literals)

#### Design Decision: Copy-on-Write with Interface Abstraction
Use **immutable slices** for COW semantics. When modifying, create a new slice. This is simpler than refcounting and idiomatic Go.

**Critical:** Wrap collections in interfaces to allow optimization later (e.g., persistent data structures for large lists). All access goes through methods, not direct slice indexing.

#### Tasks
1. Create `types/list.go`:
   ```go
   // MooList abstracts list storage - allows swapping implementation later
   type MooList interface {
       Len() int
       Get(index int) Value        // 1-based MOO index
       Set(index int, v Value) MooList  // Returns new list (COW)
       Append(v Value) MooList
       Slice(start, end int) MooList
       Elements() []Value          // For iteration
   }

   // sliceList is the concrete implementation (private)
   type sliceList struct { elements []Value }

   func (s *sliceList) Len() int { return len(s.elements) }
   func (s *sliceList) Get(i int) Value { return s.elements[i-1] } // 1-based
   func (s *sliceList) Set(i int, v Value) MooList {
       newElems := make([]Value, len(s.elements))
       copy(newElems, s.elements)
       newElems[i-1] = v
       return &sliceList{newElems}
   }
   // ... etc

   type ListValue struct { data MooList }

   func NewList(elements []Value) ListValue {
       return ListValue{data: &sliceList{elements: elements}}
   }
   ```
2. Add lexer tokens: `{`, `}`, `,`
3. Parse list literals: `{expr, expr, ...}` (recursive for nested lists)
4. Implement 1-based indexing methods
5. Implement deep equality for nested lists

#### Done Criteria
- [ ] Lexer tokenizes: `{`, `}`, `,`
- [ ] Parser: `ParseLiteral("{1, 2, 3}")` returns ListValue([1,2,3])
- [ ] Nested lists work: `{1, {2, 3}, 4}`
- [ ] Empty list: `{}` works
- [ ] ListValue.Equal() uses deep comparison

### Layer 1.9: MAP Type + Map Literals

**Spec Refs:**
- `spec/types.md` (lines 146-175: MAP section)
- `spec/grammar.md` (map literals)

#### Key Type Restrictions (from spec/types.md)
Valid key types: INT, FLOAT, STR, OBJ, ERR
Invalid key types: LIST, MAP (raise E_TYPE)

#### Tasks
1. Create `types/map.go`:
   ```go
   // MooMap abstracts map storage - allows swapping implementation later
   type MooMap interface {
       Len() int
       Get(key Value) (Value, bool)
       Set(key, val Value) MooMap  // Returns new map (COW)
       Delete(key Value) MooMap
       Keys() []Value
       Pairs() [][2]Value  // For iteration
   }

   // goMap is the concrete implementation using Go's map (private)
   // Key is stringified Value (since Go maps need comparable keys)
   type goMap struct {
       pairs map[string]mapEntry  // key hash -> entry
   }
   type mapEntry struct { key, val Value }

   func (m *goMap) Len() int { return len(m.pairs) }
   func (m *goMap) Get(k Value) (Value, bool) {
       if e, ok := m.pairs[keyHash(k)]; ok { return e.val, true }
       return nil, false
   }
   func (m *goMap) Set(k, v Value) MooMap {
       newPairs := make(map[string]mapEntry, len(m.pairs)+1)
       for h, e := range m.pairs { newPairs[h] = e }
       newPairs[keyHash(k)] = mapEntry{k, v}
       return &goMap{newPairs}
   }
   // ... etc

   type MapValue struct { data MooMap }

   func NewMap(pairs [][2]Value) MapValue {
       m := &goMap{pairs: make(map[string]mapEntry)}
       for _, p := range pairs {
           m.pairs[keyHash(p[0])] = mapEntry{p[0], p[1]}
       }
       return MapValue{data: m}
   }
   ```
2. Add lexer tokens: `[`, `]`, `->`
3. Parse map literals: `["key" -> value, ...]`
4. Validate key types (no LIST/MAP keys)
5. Implement keyHash() for comparable key lookup

#### Done Criteria
- [ ] Lexer tokenizes: `[`, `]`, `->`
- [ ] Parser: `ParseLiteral("[\"a\" -> 1]")` returns MapValue
- [ ] Nested values work: `["x" -> {1, 2}]`
- [ ] Empty map: `[]` works
- [ ] Key type validation ready (E_TYPE for invalid)

### Phase 1 Gate: Literal Parsing Tests

#### Gate Verification
```bash
go test ./conformance/... -run "basic/value" -v
# Expected: 28/28 tests pass (literal value tests)
```

The basic/value.yaml tests only test literal values (no operators). With the literal parser complete, these should pass.

#### Done Criteria
- [ ] All 9 types implemented with literal parsing
- [ ] Lexer handles all literal syntaxes
- [ ] Parser.ParseLiteral() works for all types
- [ ] basic/value.yaml: 28/28 passing

#### Handoff State
```
Files Created:
  types/*.go (9 files: value, typecode, int, float, str, error, obj, bool, list, map)
  parser/token.go, parser/lexer.go, parser/parser.go
Test Status: 28/28 basic/value.yaml passing
Next: Phase 2 - Operators & Expressions
```

---

## Phase 2: Operators & Expressions

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 3**

Phase 1 built the lexer and literal parser. This phase adds operators and full expression parsing.

### Layer 2.1: AST Node Types + Operator Tokens

**Spec Refs:** `spec/grammar.md` (Expression grammar), `spec/operators.md` (Precedence table)

#### Tasks
1. Create `parser/ast.go` - AST node types:
   - LiteralExpr (wraps Values from Phase 1)
   - BinaryExpr (left, op, right)
   - UnaryExpr (op, operand)
   - TernaryExpr (cond, then, else)
   - IdentifierExpr (variable names)
2. Extend lexer with operator tokens:
   - Arithmetic: `+`, `-`, `*`, `/`, `%`, `^`
   - Comparison: `==`, `!=`, `<`, `>`, `<=`, `>=`
   - Logical: `&&`, `||`, `!`
   - Bitwise: `&.`, `|.`, `^.`, `~`, `<<`, `>>`
   - Other: `?`, `|` (ternary), `in`
3. All AST nodes implement Expr interface with Position() method

#### Done Criteria
- [ ] All operator tokens lexed correctly
- [ ] AST node types defined for all expression forms
- [ ] Position tracking works for error messages

### Layer 2.2: Unary Operators

**Spec Refs:** `spec/operators.md` (Unary Operators section)

#### Tasks
1. Parse unary minus (-) with correct precedence
2. Parse logical not (!)
3. Parse bitwise not (~)

#### Done Criteria
- [ ] `-5` parses as UnaryExpr(MINUS, 5)
- [ ] `!true` parses as UnaryExpr(NOT, true)
- [ ] `~0` parses as UnaryExpr(BITNOT, 0)

### Layer 2.3: Binary Operators (Arithmetic)

**Spec Refs:** `spec/operators.md` (Arithmetic section, Precedence table lines 50-80)

#### Precedence (high to low per spec)
1. `^` (power, RIGHT associative)
2. `*`, `/`, `%` (multiplicative, left associative)
3. `+`, `-` (additive, left associative)

#### Tasks
1. Implement Pratt parser or precedence climbing
2. Parse arithmetic expressions with correct precedence
3. Handle right-associativity for `^`

#### Done Criteria
- [ ] `1 + 2 * 3` parses as `1 + (2 * 3)` (precedence)
- [ ] `2 ^ 3 ^ 2` parses as `2 ^ (3 ^ 2)` (right associative)
- [ ] `10 / 3 % 2` parses left-to-right

### Layer 2.4: Comparison Operators

**Spec Refs:** `spec/operators.md` (Comparison section)

#### Tasks
1. Parse `==`, `!=`, `<`, `>`, `<=`, `>=`
2. Parse `in` operator
3. Comparison has lower precedence than arithmetic

#### Done Criteria
- [ ] `1 + 1 == 2` parses as `(1 + 1) == 2`
- [ ] `x in {1, 2, 3}` parses correctly

### Layer 2.5: Logical Operators

**Spec Refs:** `spec/operators.md` (Logical section)

#### Tasks
1. Parse `&&` (AND, higher precedence)
2. Parse `||` (OR, lower precedence)
3. Both are left-associative, short-circuit in evaluator (not parser)

#### Done Criteria
- [ ] `a || b && c` parses as `a || (b && c)`
- [ ] `a && b || c && d` parses as `(a && b) || (c && d)`

### Layer 2.6: Bitwise Operators

**Spec Refs:** `spec/operators.md` (Bitwise section)

#### Tasks
1. Parse `&.` (bitwise AND), `|.` (bitwise OR), `^.` (bitwise XOR)
2. Parse `<<`, `>>` (shift operators)
3. Note: MOO uses `.` suffix to distinguish from logical ops

#### Done Criteria
- [ ] `5 &. 3` parses correctly
- [ ] `1 << 4` parses correctly

### Layer 2.7: Ternary + Parentheses

**Spec Refs:** `spec/operators.md` (Ternary section)

#### Tasks
1. Parse ternary: `expr ? true_expr | false_expr`
2. Parse parenthesized expressions: `(expr)`
3. Ternary has very low precedence (above assignment only)

#### Done Criteria
- [ ] `x ? 1 | 2` parses correctly
- [ ] `(1 + 2) * 3` respects parentheses
- [ ] Nested ternaries: `a ? b ? 1 | 2 | 3` parses correctly

### Phase 2 Gate: Expression Parsing Tests

#### Gate Verification
```bash
go test ./conformance/... -run "basic/arithmetic|language/equality" -v
# Expected: 20/20 tests pass
```

Note: These tests require both parsing AND evaluation. If Phase 3 (evaluator) isn't complete, run unit tests on parser AST output instead.

#### Done Criteria
- [ ] All operators parse with correct precedence
- [ ] AST structure is correct for complex expressions
- [ ] Parser handles all test cases in basic/arithmetic.yaml

#### Handoff State
```
Files Modified: parser/ast.go, parser/lexer.go (operators), parser/parser.go (expression parsing)
Test Status: Parser unit tests pass, expression ASTs correct
Next: Phase 3 - Tree-Walk Evaluator
```

---

## Phase 3: Tree-Walk Evaluator

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 4**

### Layer 3.1: Evaluator Scaffold

**Spec Refs:** `notes/go_interpreter_patterns.md` (Tree-Walk section)

**Critical:** Use Result type and TaskContext from Layer 0.3. All Eval methods return Result (not Value), accept *TaskContext. This unifies error propagation, control flow (break/continue/return), and tick limits into one mechanism.

#### Tasks
1. Create `eval/eval.go`:
   ```go
   type Evaluator struct { env *Environment }

   // Every Eval method takes context, returns Result (not Value)
   func (e *Evaluator) Eval(node ast.Node, ctx *TaskContext) Result {
       ctx.TicksRemaining--
       if ctx.TicksRemaining <= 0 {
           return Err(E_MAXREC)  // Or E_TICKS if defined
       }
       // dispatch to specific node type...
   }
   ```
2. Create `eval/environment.go` - Variable scoping/lookup
3. Implement visitor pattern over AST nodes
4. Propagate Result.Flow through expression trees (if child returns non-Normal, bubble up)

#### Done Criteria
- [ ] Evaluator can walk AST nodes
- [ ] Environment handles variable get/set
- [ ] All Eval methods use Result, not raw Value
- [ ] Tick counting works (returns error after N evals)

### Layer 3.2: Expression Evaluation

**Spec Refs:** `spec/operators.md` (all operator semantics)

#### Tasks
1. Implement literal evaluation (return value directly)
2. Implement binary operator evaluation per spec
3. Implement unary operator evaluation
4. Implement short-circuit for && and ||

#### Done Criteria
- [ ] 1 + 2 evaluates to 3
- [ ] true && false evaluates to false (short-circuit)
- [ ] Type errors raise E_TYPE per spec

### Layer 3.3: Variable Binding

**Spec Refs:** `spec/statements.md` (Assignment section)

#### Tasks
1. Implement variable assignment
2. Implement variable lookup
3. Handle undefined variable (E_VARNF)

#### Done Criteria
- [ ] x = 5; return x; evaluates to 5
- [ ] Undefined variable raises E_VARNF

### Layer 3.4: Type Conversion Builtins

**Spec Refs:** `spec/builtins/types.md`

#### Tasks
1. Create `builtins/registry.go` - Builtin function registry
2. Implement typeof(value)
3. Implement tostr(value)
4. Implement toint(value)
5. Implement tofloat(value)

#### Done Criteria
- [ ] typeof(42) == 0 (INT type code)
- [ ] tostr(42) == "42"
- [ ] toint("42") == 42
- [ ] toint("abc") raises E_TYPE

#### Phase 3 Gate Verification
```bash
go test ./conformance/... -run "builtins/primitives" -v
# Expected: subset of primitives tests pass (type conversion)
```

#### Handoff State
```
Files Created: eval/*.go, builtins/registry.go, builtins/types.go
Test Status: Type conversion tests passing
Next: Phase 4 - Collections & Indexing
```

---

## Phase 4: Collections & Indexing

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 5**

### Layer 4.1: List Indexing

**Spec Refs:** `spec/types.md` (LIST indexing), `spec/operators.md` (Index operator)

#### Tasks
1. Implement list[index] evaluation
2. Implement 1-based indexing
3. Raise E_RANGE for out-of-bounds

#### Done Criteria
- [ ] {1,2,3}[1] == 1
- [ ] {1,2,3}[0] raises E_RANGE
- [ ] {1,2,3}[4] raises E_RANGE

### Layer 4.2: Range Expressions

**Spec Refs:** `spec/operators.md` (Range section)

#### Tasks
1. Parse range expressions: expr[start..end]
2. Implement ^ (first) and $ (last) anchors
3. Implement range slicing for lists

#### Done Criteria
- [ ] {1,2,3,4,5}[2..4] == {2,3,4}
- [ ] {1,2,3}[^..$] == {1,2,3}
- [ ] {1,2,3}[$..^] == {3,2,1} (reversed)

### Layer 4.3: String Indexing

**Spec Refs:** `spec/types.md` (STR indexing)

#### Tasks
1. Implement string[index] returning single character
2. Implement string[start..end] returning substring
3. Strings are byte sequences per spec (not Unicode codepoints) - index by byte position

#### Done Criteria
- [ ] "hello"[1] == "h"
- [ ] "hello"[2..4] == "ell"

### Layer 4.4: Map Indexing

**Spec Refs:** `spec/types.md` (MAP indexing)

#### Tasks
1. Implement map[key] lookup
2. Raise E_RANGE for missing keys
3. Handle nested map access

#### Done Criteria
- [ ] ["a" -> 1]["a"] == 1
- [ ] ["a" -> 1]["b"] raises E_RANGE

### Layer 4.5: Splice Operator

**Spec Refs:** `spec/operators.md` (Splice section)

#### Tasks
1. Parse @expr in list contexts
2. Implement splice evaluation (flatten list into parent list)

#### Done Criteria
- [ ] {1, @{2,3}, 4} == {1,2,3,4}
- [ ] @non_list raises E_TYPE

#### Phase 4 Gate Verification
```bash
go test ./conformance/... -run "language/index_and_range|basic/list" -v
# Expected: 135/137 tests pass
```

#### Handoff State
```
Files Modified: eval/eval.go (indexing), parser/parser.go (ranges), types/list.go, types/str.go
Test Status: 135/137 indexing tests passing
Next: Phase 5 - Control Flow
```

---

## Phase 5: Control Flow

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 6**

### Layer 5.1: If/Elseif/Else

**Spec Refs:** `spec/statements.md` (If section)

#### Tasks
1. Parse if/elseif/else/endif statements
2. Implement conditional evaluation
3. Handle truthy/falsy per spec/types.md

#### Done Criteria
- [ ] if (1) return 1; else return 2; endif == 1
- [ ] Elseif chains work correctly

### Layer 5.2: While Loops

**Spec Refs:** `spec/statements.md` (While section)

#### Tasks
1. Parse while/endwhile
2. Implement loop evaluation
3. Handle infinite loop protection (tick limit)

#### Done Criteria
- [ ] while (x < 5) x = x + 1; endwhile works
- [ ] Infinite loops hit tick limit

### Layer 5.3: For Loops (List Iteration)

**Spec Refs:** `spec/statements.md` (For section - list iteration)

#### Tasks
1. Parse for x in (list) ... endfor
2. Implement list iteration

#### Done Criteria
- [ ] for x in ({1,2,3}) sum = sum + x; endfor works

### Layer 5.4: For Loops (Range Iteration)

**Spec Refs:** `spec/statements.md` (For section - range iteration)

#### Tasks
1. Parse for x in [start..end] ... endfor
2. Implement range iteration

#### Done Criteria
- [ ] for x in [1..5] ... works
- [ ] Negative step (5..1) works

### Layer 5.5: For Loops (Map Iteration)

**Spec Refs:** `spec/statements.md` (For section - map iteration)

#### Tasks
1. Parse for k, v in (map) ... endfor
2. Implement key-value iteration

#### Done Criteria
- [ ] for k, v in (["a" -> 1]) ... works

### Layer 5.6: Break/Continue

**Spec Refs:** `spec/statements.md` (Break/Continue section)

#### Tasks
1. Parse break and continue statements
2. Implement loop exit/skip behavior

#### Done Criteria
- [ ] break exits loop immediately
- [ ] continue skips to next iteration

### Layer 5.7: Named Loops

**Spec Refs:** `spec/statements.md` (Named loops section)

#### Tasks
1. Parse loop labels: while name (cond) ...
2. Parse break name, continue name
3. Implement named loop exit/skip

#### Done Criteria
- [ ] break outer exits outer loop from nested loop
- [ ] continue inner continues correct loop

#### Phase 5 Gate Verification
```bash
go test ./conformance/... -run "language/looping" -v
# Expected: 19/29 tests pass (10 skipped require advanced features)
```

#### Handoff State
```
Files Modified: parser/*.go (statements), eval/eval.go (control flow)
Test Status: 19/29 looping tests passing
Next: Phase 6 - Exception Handling
```

---

## Phase 6: Exception Handling

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 7**

### Layer 6.1: Error Raising

**Spec Refs:** `spec/errors.md` (Error raising), `spec/statements.md`

#### Tasks
1. Implement raise(error) builtin or implicit raising
2. Propagate errors up call stack

#### Done Criteria
- [ ] E_TYPE propagates correctly
- [ ] Unhandled errors terminate task

### Layer 6.2: Try/Except

**Spec Refs:** `spec/statements.md` (Try/Except section)

#### Tasks
1. Parse try/except/endtry
2. Implement error catching by code
3. Implement ANY catch

#### Done Criteria
- [ ] try 1/0; except E_DIV ... catches division error
- [ ] try x; except ANY ... catches all errors

### Layer 6.3: Try/Finally

**Spec Refs:** `spec/statements.md` (Try/Finally section)

#### Tasks
1. Parse try/finally/endtry
2. Implement finally always runs
3. Handle finally with return/break/continue

#### Done Criteria
- [ ] finally block runs on success
- [ ] finally block runs on error
- [ ] finally block runs on return

### Layer 6.4: Catch Expression

**Spec Refs:** `spec/operators.md` (Catch expression section)

#### Tasks
1. Parse catch expression: `expr ! codes => default`
2. Implement inline error handling

#### Done Criteria
- [ ] 1/0 `! E_DIV => 0` == 0
- [ ] 1/0 `! ANY => 0` == 0
- [ ] 1/1 `! E_DIV => 0` == 1

### Layer 6.5: Scatter Assignment

**Spec Refs:** `spec/statements.md` (Scatter section)

#### Tasks
1. Parse scatter: {a, b, ?c, @rest} = list
2. Implement required, optional, rest patterns

#### Done Criteria
- [ ] {a, b} = {1, 2}; works
- [ ] {a, ?b} = {1}; works (b = 0)
- [ ] {a, @rest} = {1, 2, 3}; rest == {2, 3}

#### Phase 6 Gate Verification
```bash
go test ./conformance/... -run "language/moocode_parsing" -v
# Expected: 50/62 tests pass
```

#### Handoff State
```
Files Modified: parser/*.go, eval/eval.go (exceptions)
Test Status: 50/62 parsing tests passing
Next: Phase 7 - Core Builtins
```

---

## Phase 7: Core Builtins

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 8**

### Layer 7.1: String Builtins

**Spec Refs:** `spec/builtins/strings.md`

#### Tasks
1. Implement length(str)
2. Implement strsub(str, start, end)
3. Implement index(str, substr)
4. Implement rindex(str, substr)
5. Implement strcmp(str1, str2)
6. Implement uppercase/lowercase
7. Implement explode/implode
8. Implement match/rmatch

#### Done Criteria
- [ ] All string builtins per spec
- [ ] basic/string.yaml passes

### Layer 7.2: List Builtins

**Spec Refs:** `spec/builtins/lists.md`

#### Tasks
1. Implement listappend/listinsert/listset/listdelete
2. Implement setadd/setremove
3. Implement length(list)

#### Done Criteria
- [ ] All list builtins per spec
- [ ] basic/list.yaml passes

### Layer 7.3: Math Builtins

**Spec Refs:** `spec/builtins/math.md`

#### Tasks
1. Implement abs, min, max
2. Implement sqrt, sin, cos, tan
3. Implement log, exp, log10
4. Implement random, random_bytes
5. Implement ceil, floor, round

#### Done Criteria
- [ ] All math builtins per spec
- [ ] builtins/math.yaml passes

### Layer 7.4: Algorithm Builtins

**Spec Refs:** `spec/builtins/lists.md` (sort, reverse, etc.)

#### Tasks
1. Implement sort(list, ?comparator)
2. Implement reverse(list)
3. Implement unique(list)

#### Done Criteria
- [ ] builtins/algorithms.yaml passes

### Layer 7.5: Map Builtins

**Spec Refs:** `spec/builtins/maps.md`

#### Tasks
1. Implement mapkeys(map)
2. Implement mapvalues(map)
3. Implement mapdelete(map, key)

#### Done Criteria
- [ ] All map builtins per spec
- [ ] builtins/map.yaml passes

#### Phase 7 Gate Verification
```bash
go test ./conformance/... -run "basic/string|builtins/string_operations|builtins/algorithms|builtins/map|builtins/math" -v
# Expected: 350+/400+ tests pass
```

#### Handoff State
```
Files Created: builtins/strings.go, builtins/lists.go, builtins/math.go, builtins/maps.go
Test Status: Core builtins passing
Next: Phase 8 - Object System
```

---

## Phase 8: Object System

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 9**

**Design Decision: ObjID Discipline**
Objects reference each other by ObjID (integer), NOT Go pointers. This matches the LambdaMOO database format (Phase 12) and avoids painful serialization issues later.

**Note:** ObjID was already defined in Layer 0.3 (`types/base.go`). Use it here.

```go
// ObjID already defined in types/base.go

type Object struct {
    ID       ObjID
    Parent   ObjID        // NOT *Object
    Owner    ObjID        // NOT *Object
    Children []ObjID      // NOT []*Object
    // ...
}
```

All cross-object references use ObjID + store.Get(id) lookup. This is slightly slower but makes persistence trivial.

### Layer 8.1: In-Memory Object Store

**Spec Refs:** `spec/objects.md` (Object structure)

#### Tasks
1. Create `db/object.go` - Object struct with ObjID references (not pointers)
2. Create `db/store.go` - In-memory object storage (map[ObjID]*Object)
3. Implement object lookup by ID

#### Done Criteria
- [ ] Objects can be stored and retrieved by ID
- [ ] Object structure matches spec

### Layer 8.2: Property Access

**Spec Refs:** `spec/objects.md` (Properties), `spec/operators.md` (Property access)

#### Tasks
1. Parse obj.property syntax
2. Parse obj.(expr) dynamic property access
3. Implement property get/set

#### Done Criteria
- [ ] obj.name returns property value
- [ ] obj.missing raises E_PROPNF

### Layer 8.3: Property Inheritance

**Spec Refs:** `spec/objects.md` (Inheritance)

#### Tasks
1. Implement parent chain traversal
2. Implement property inheritance lookup
3. Handle clear_property semantics

#### Done Criteria
- [ ] Child inherits parent properties
- [ ] Child can override parent properties

### Layer 8.4: Object Creation/Recycling

**Spec Refs:** `spec/builtins/objects.md` (create, recycle)

#### Tasks
1. Implement create(parent) builtin
2. Implement recycle(obj) builtin
3. Handle object ID allocation

#### Done Criteria
- [ ] create(#1) returns new child of #1
- [ ] recycle(obj) marks object invalid

### Layer 8.5: Object Builtins

**Spec Refs:** `spec/builtins/objects.md`

#### Tasks
1. Implement valid(obj)
2. Implement parent(obj), children(obj)
3. Implement move(obj, dest)
4. Implement chparent(obj, new_parent)

#### Done Criteria
- [ ] All object builtins per spec

### Layer 8.6: Property Builtins

**Spec Refs:** `spec/builtins/properties.md`

#### Tasks
1. Implement properties(obj)
2. Implement property_info(obj, name)
3. Implement add_property/delete_property
4. Implement is_clear_property/clear_property

#### Done Criteria
- [ ] All property builtins per spec

#### Phase 8 Gate Verification
```bash
go test ./conformance/... -run "basic/object|basic/property|builtins/objects|builtins/create" -v
# Expected: 167/184 tests pass
```

#### Handoff State
```
Files Created: db/object.go, db/store.go, builtins/objects.go, builtins/properties.go
Test Status: Object system tests passing
Next: Phase 9 - Verb System
```

---

## Phase 9: Verb System

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 10**

### Layer 9.1: Verb Storage and Lookup

**Spec Refs:** `spec/objects.md` (Verbs section)

#### Tasks
1. Extend Object with verb storage
2. Implement verb lookup by name
3. Implement verb inheritance lookup

#### Done Criteria
- [ ] Verbs stored on objects
- [ ] Verb lookup follows inheritance

### Layer 9.2: Verb Compilation

**Spec Refs:** `spec/grammar.md`, `spec/vm.md` (optional for tree-walk)

#### Tasks
1. Parse verb code as MOO statements
2. Store compiled AST on verb

#### Done Criteria
- [ ] Verb code parses correctly
- [ ] Verbs can be added programmatically

### Layer 9.3: Verb Calls

**Spec Refs:** `spec/operators.md` (Verb call section)

#### Tasks
1. Parse obj:verb(args) syntax
2. Implement verb invocation
3. Set up verb call context (this, caller, args, etc.)

#### Done Criteria
- [ ] obj:verb() invokes verb
- [ ] this, caller, args set correctly

### Layer 9.4: Dollar Notation

**Spec Refs:** `spec/operators.md` (Dollar notation)

#### Tasks
1. Parse $name as #0.name lookup
2. Parse $name:verb() as #0.name:verb()

#### Done Criteria
- [ ] $system resolves to #0.system
- [ ] $string_utils:explode() works

### Layer 9.5: Verb Builtins

**Spec Refs:** `spec/builtins/verbs.md`

#### Tasks
1. Implement verbs(obj)
2. Implement verb_info(obj, name)
3. Implement add_verb/delete_verb
4. Implement set_verb_code
5. Implement verb_code (decompilation)

#### Done Criteria
- [ ] All verb builtins per spec

#### Phase 9 Gate Verification
```bash
# Verb-related tests across multiple files
go test ./conformance/... -v
# Check that verb-dependent tests now pass
```

#### Handoff State
```
Files Modified: db/object.go (verbs), builtins/verbs.go
Test Status: Verb system working
Next: Phase 10 - Advanced Features
```

---

## Phase 10: Advanced Features

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 11**

### Layer 10.1: JSON Builtins

**Spec Refs:** `spec/builtins/json.md`

#### Tasks
1. Implement parse_json(str)
2. Implement generate_json(value)
3. Handle MOO-specific mappings per spec

#### Done Criteria
- [ ] builtins/json.yaml passes (120 tests)

### Layer 10.2: eval() Builtin

**Spec Refs:** `spec/builtins/exec.md` (eval section)

#### Tasks
1. Implement eval(str) - parse and execute MOO code
2. Handle eval errors correctly

#### Done Criteria
- [ ] eval("1 + 2") == 3
- [ ] eval("bad syntax") raises E_PARSE

### Layer 10.3: Anonymous Objects/Functions

**Spec Refs:** `spec/types.md` (ANON section)

#### Tasks
1. Parse anonymous function syntax
2. Implement anonymous function evaluation
3. Handle closures if specified

#### Done Criteria
- [ ] language/anonymous.yaml passes

### Layer 10.4: WAIFs

**Spec Refs:** `spec/types.md` (WAIF section)

#### Tasks
1. Implement WAIF type
2. Implement WAIF creation and access
3. Handle WAIF-specific behavior

#### Done Criteria
- [ ] language/waif.yaml passes

#### Phase 10 Gate Verification
```bash
go test ./conformance/... -run "builtins/json|language/anonymous|language/waif" -v
# Expected: 156/219 tests pass (some require server features)
```

#### Handoff State
```
Files Created: builtins/json.go, builtins/eval.go, types/anon.go, types/waif.go
Test Status: Advanced features passing
Next: Phase 11 - Bytecode VM (optional optimization)
```

---

## Phase 11: Bytecode VM (Optimization)

**GATE: REQUIRES HUMAN APPROVAL BEFORE PHASE 12**

This phase is optional - tree-walk interpreter is sufficient for correctness.

### Layer 11.1: Opcode Definitions

**Spec Refs:** `spec/vm.md` (Opcodes section)

#### Tasks
1. Create `vm/opcodes.go` - Opcode enum
2. Define all opcodes per spec

### Layer 11.2: Compiler

**Spec Refs:** `spec/vm.md` (Compilation section)

#### Tasks
1. Create `vm/compiler.go` - AST to bytecode
2. Implement constant pool
3. Implement jump target resolution

### Layer 11.3: VM Execution Loop

**Spec Refs:** `spec/vm.md` (Execution section)

#### Tasks
1. Create `vm/vm.go` - Bytecode interpreter
2. Implement stack operations
3. Implement all opcodes

### Layer 11.4: Replace Tree-Walk

#### Tasks
1. Switch evaluator to use VM
2. Ensure all tests still pass

#### Phase 11 Gate Verification
```bash
go test ./conformance/... -v
# Expected: All previously passing tests still pass
```

#### Handoff State
```
Files Created: vm/*.go
Test Status: No regressions
Next: Phase 12 - Server Infrastructure
```

---

## Phase 12: Server Infrastructure

**GATE: FINAL PHASE**

### Layer 12.1: Database Reader

**Spec Refs:** `spec/database.md`

#### Tasks
1. Create `db/reader.go` - Parse LambdaMOO database files
2. Support v4 and v17 formats per spec
3. Load objects, properties, verbs from database

#### Done Criteria
- [ ] Can load a standard MOO database file

### Layer 12.2: Task Scheduler

**Spec Refs:** `spec/tasks.md`

#### Tasks
1. Create `server/task.go` - Task struct and lifecycle
2. Implement task queue and scheduling
3. Implement tick counting and limits
4. Implement fork/suspend/resume

#### Done Criteria
- [ ] Tasks execute with tick limits
- [ ] fork() creates delayed tasks

### Layer 12.3: Server Main Loop

**Spec Refs:** `spec/server.md`

#### Tasks
1. Create `server/server.go` - Main server loop
2. Implement checkpoint/dump_database
3. Implement shutdown hooks

#### Done Criteria
- [ ] Server starts and runs main loop
- [ ] Server can checkpoint database

### Layer 12.4: Connection Handling

**Spec Refs:** `spec/login.md`

#### Tasks
1. Implement TCP listener
2. Implement connection to player mapping
3. Implement do_login_command flow

#### Done Criteria
- [ ] Clients can connect via telnet
- [ ] Login flow works per spec

### Layer 12.5: Network Builtins

**Spec Refs:** `spec/builtins/network.md`

#### Tasks
1. Implement notify(player, message)
2. Implement read(player)
3. Implement connected_players()

#### Done Criteria
- [ ] server/*.yaml passes

#### Phase 12 Gate Verification
```bash
go test ./conformance/... -v
# Expected: All 905 active tests pass

# Integration test
./barn -db test.db
# Server runs and accepts connections
```

#### Handoff State
```
Files Created: db/reader.go, server/*.go
Test Status: 905/905 active tests passing (205 known skips)
Status: IMPLEMENTATION COMPLETE
```

---

## Skipped Tests Roadmap

**205 tests currently skipped in conformance suite:**

| Category | Count | Phase to Address |
|----------|-------|------------------|
| server/exec.yaml | 20 | Phase 12 |
| server/limits.yaml | 38 | Phase 12 |
| language/waif.yaml (partial) | 25 | Phase 10 |
| language/anonymous.yaml (partial) | 27 | Phase 10 |
| builtins/create.yaml (multi-user) | 15 | Phase 12 (test infra) |
| builtins/objects.yaml (decompile) | 17 | Phase 9 |
| language/looping.yaml (decompile) | 10 | Phase 9 |
| builtins/math.yaml (64-bit) | 15 | Platform-specific |
| Other | 38 | Various |

Tests should be unskipped as their required features are implemented.

---

## Success Criteria

1. **All 905 active conformance tests pass**
2. **Server can load a MOO database and run**
3. **Clients can connect and execute verbs**
4. **Performance reasonable for typical workloads**

When all phases complete, Barn is a functional MOO server.
