# Go Interpreter Patterns Research

## Key Resources

### Books
- **"Writing An Interpreter In Go"** by Thorsten Ball - Tree-walk interpreter
- **"Writing A Compiler In Go"** by Thorsten Ball - Bytecode compiler + VM

### Implementation Approaches

#### 1. Tree-Walk Interpreter
- Parse source -> AST -> Walk and execute
- Simple, straightforward
- Slower execution
- Good for initial implementation

#### 2. Bytecode VM
- Parse source -> AST -> Compile to bytecode -> VM executes
- Faster execution
- More complex (compiler + VM)
- Traditional approach: big switch statement over opcodes

### Go Parsing Options

#### 1. **Participle** (github.com/alecthomas/participle)
- Idiomatic Go - uses struct tags like encoding/json
- Runtime parser, no code generation
- Good for Go-first development

```go
type Expression struct {
    Left  *Term     `@@`
    Op    string    `@("+" | "-")`
    Right *Term     `@@`
}
```

#### 2. **pigeon** (github.com/mna/pigeon)
- PEG parser generator
- Generates Go code from grammar file
- More similar to Lark approach
- Supports left recursion (experimental)

#### 3. **pointlander/peg**
- Packrat parser generator
- Well-established
- Good for complex grammars

#### 4. Hand-written Recursive Descent
- Full control
- More boilerplate
- What Thorsten Ball uses in his books
- Pratt parser for expressions (operator precedence)

## MOO Grammar Analysis

From moo_interp's `parser.lark`:

### Statement Types
- if/elseif/else/endif
- for/endfor (with range and list iteration)
- while/endwhile
- fork/endfork
- try/except/finally/endtry
- return, break, continue

### Expression Precedence (low to high)
1. Assignment (`=`)
2. Ternary (`? |`)
3. Catch (`` `expr ! codes => default' ``)
4. Splicer (`@`)
5. Logical OR (`||`)
6. Logical AND (`&&`)
7. Bitwise OR (`|.`)
8. Bitwise XOR (`^.`)
9. Bitwise AND (`&.`)
10. Comparison (`==`, `!=`, `<`, `>`, `<=`, `>=`, `in`)
11. Shift (`<<`, `>>`)
12. Additive (`+`, `-`)
13. Multiplicative (`*`, `/`, `%`)
14. Power (`^`)
15. Unary (`!`, `~`, `-`)
16. Postfix (property access, indexing, verb calls)

### Literals
- Integers: `42`, `-5`
- Floats: `3.14`, `1e-5`
- Strings: `"hello"`
- Objects: `#0`, `#-1`
- Lists: `{1, 2, 3}`
- Maps: `["key" -> value]`
- Errors: `E_TYPE`, `E_PERM`

### Special MOO Features
- Dollar notation: `$system`, `$string_utils`
- Object properties: `obj.prop`, `obj.(expr)`
- Verb calls: `obj:verb(args)`, `obj:(expr)(args)`
- Scatter assignment: `{a, b, ?c} = {1, 2}`
- List splice: `@list`
- Range indexing: `str[1..5]`, `list[^..$]`

## Recommended Approach for Go MOO

### Phase 1: Parser
**Option A: Participle** - Good if we want idiomatic Go
- Struct-based grammar definition
- Easy to test and iterate
- May need customization for MOO quirks

**Option B: pigeon/PEG** - Good if we want to stay close to Lark grammar
- Can adapt the existing Lark grammar
- Generated parser code

**Option C: Hand-written** - Maximum control
- Pratt parser for expressions
- Recursive descent for statements
- More work but most flexible

### Phase 2: AST
Define Go types for each node:
```go
type Node interface {
    Pos() Position
}

type Expr interface {
    Node
    exprNode()
}

type Stmt interface {
    Node
    stmtNode()
}

type BinaryExpr struct {
    Left  Expr
    Op    Token
    Right Expr
    pos   Position
}

type IfStmt struct {
    Condition Expr
    Then      []Stmt
    ElseIfs   []ElseIfClause
    Else      []Stmt
    pos       Position
}
```

### Phase 3: Interpreter Options

**A. Tree-Walk (simpler, start here)**
```go
type Interpreter struct {
    env *Environment
}

func (i *Interpreter) Eval(expr Expr) (Value, error) {
    switch e := expr.(type) {
    case *BinaryExpr:
        left, _ := i.Eval(e.Left)
        right, _ := i.Eval(e.Right)
        return i.evalBinary(e.Op, left, right)
    case *Literal:
        return e.Value, nil
    // ...
    }
}
```

**B. Bytecode VM (faster, Phase 2)**
```go
type OpCode byte

const (
    OP_CONST OpCode = iota
    OP_ADD
    OP_SUB
    OP_CALL
    // ...
)

type VM struct {
    chunk  *Chunk
    ip     int
    stack  []Value
}

func (vm *VM) Run() (Value, error) {
    for {
        op := vm.chunk.Code[vm.ip]
        vm.ip++
        switch op {
        case OP_CONST:
            idx := vm.chunk.Code[vm.ip]
            vm.ip++
            vm.stack = append(vm.stack, vm.chunk.Constants[idx])
        case OP_ADD:
            b := vm.stack[len(vm.stack)-1]
            vm.stack = vm.stack[:len(vm.stack)-1]
            a := vm.stack[len(vm.stack)-1]
            vm.stack[len(vm.stack)-1] = add(a, b)
        // ...
        }
    }
}
```

## Recommendations

1. **Start with tree-walk interpreter** - Get semantics right first
2. **Use Participle for parsing** - Idiomatic, testable, flexible
3. **Define clear AST types** - Foundation for both approaches
4. **Make conformance tests pass** - Spec-driven development
5. **Optimize to bytecode later** - If performance needed

## Sources
- [Writing An Interpreter In Go](https://interpreterbook.com/)
- [Writing A Compiler In Go](https://compilerbook.com/)
- [Participle Parser Library](https://github.com/alecthomas/participle)
- [pigeon PEG Generator](https://github.com/mna/pigeon)
- [Crafting Interpreters in Go](https://www.chidiwilliams.com/posts/notes-on-crafting-interpreters-go)
