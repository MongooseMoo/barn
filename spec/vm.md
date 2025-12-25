# MOO VM Architecture Specification

## Overview

The MOO virtual machine is a stack-based bytecode interpreter. This document specifies the VM architecture for the Go implementation.

---

## 1. Compilation Pipeline

```
MOO Source → Lexer → Parser → AST → Compiler → Bytecode → VM
```

| Stage | Input | Output |
|-------|-------|--------|
| Lexer | Source text | Token stream |
| Parser | Tokens | AST nodes |
| Compiler | AST | Bytecode program |
| VM | Bytecode | Execution result |

---

## 2. Bytecode Format

### 2.1 Program Structure

```go
type Program struct {
    Code      []byte      // Bytecode instructions
    Constants []Value     // Constant pool
    VarNames  []string    // Variable name table
    LineInfo  []LineEntry // Source line mapping
}
```

### 2.2 Instruction Format

Variable-length instructions:

```
[opcode: 1 byte] [operands: 0-N bytes]
```

Operand sizes:
- 1 byte: Small indices (0-255)
- 2 bytes: Medium indices (0-65535)
- 4 bytes: Large values

---

## 3. Opcode Categories

### 3.1 Stack Operations

| Opcode | Operands | Description |
|--------|----------|-------------|
| OP_PUSH | index | Push constant from pool |
| OP_POP | - | Discard top of stack |
| OP_DUP | - | Duplicate top of stack |
| OP_IMM | value | Push immediate small int |

### 3.2 Variable Operations

| Opcode | Operands | Description |
|--------|----------|-------------|
| OP_GET_VAR | index | Push local variable |
| OP_SET_VAR | index | Pop and store to local |
| OP_GET_PROP | - | Pop obj, push obj.prop |
| OP_SET_PROP | - | Pop value, obj; set obj.prop |

### 3.3 Arithmetic

| Opcode | Operands | Description |
|--------|----------|-------------|
| OP_ADD | - | Pop b, a; push a + b |
| OP_SUB | - | Pop b, a; push a - b |
| OP_MUL | - | Pop b, a; push a * b |
| OP_DIV | - | Pop b, a; push a / b |
| OP_MOD | - | Pop b, a; push a % b |
| OP_POW | - | Pop b, a; push a ^ b |
| OP_NEG | - | Pop a; push -a |

### 3.4 Comparison

| Opcode | Operands | Description |
|--------|----------|-------------|
| OP_EQ | - | Pop b, a; push a == b |
| OP_NE | - | Pop b, a; push a != b |
| OP_LT | - | Pop b, a; push a < b |
| OP_LE | - | Pop b, a; push a <= b |
| OP_GT | - | Pop b, a; push a > b |
| OP_GE | - | Pop b, a; push a >= b |
| OP_IN | - | Pop b, a; push a in b |

### 3.5 Logical

| Opcode | Operands | Description |
|--------|----------|-------------|
| OP_NOT | - | Pop a; push !a |
| OP_AND | offset | Short-circuit AND |
| OP_OR | offset | Short-circuit OR |

### 3.6 Bitwise

| Opcode | Operands | Description |
|--------|----------|-------------|
| OP_BITOR | - | Pop b, a; push a \|. b |
| OP_BITAND | - | Pop b, a; push a &. b |
| OP_BITXOR | - | Pop b, a; push a ^. b |
| OP_BITNOT | - | Pop a; push ~a |
| OP_SHL | - | Pop b, a; push a << b |
| OP_SHR | - | Pop b, a; push a >> b |

### 3.7 Control Flow

| Opcode | Operands | Description |
|--------|----------|-------------|
| OP_JUMP | offset | Unconditional jump |
| OP_JUMP_IF_FALSE | offset | Pop; jump if falsy |
| OP_JUMP_IF_TRUE | offset | Pop; jump if truthy |
| OP_RETURN | - | Pop and return |
| OP_RETURN_NONE | - | Return 0 |

### 3.8 Looping

| Opcode | Operands | Description |
|--------|----------|-------------|
| OP_FOR_RANGE | var, end_offset | Start range loop |
| OP_FOR_LIST | var, end_offset | Start list loop |
| OP_FOR_NEXT | start_offset | Next iteration |
| OP_BREAK | - | Exit loop |
| OP_CONTINUE | - | Next iteration |

### 3.9 Exception Handling

| Opcode | Operands | Description |
|--------|----------|-------------|
| OP_TRY_EXCEPT | handler_offset | Push exception handler |
| OP_END_EXCEPT | - | Pop exception handler |
| OP_TRY_FINALLY | finally_offset | Push finally handler |
| OP_END_FINALLY | - | Execute finally |
| OP_CATCH | offset, codes | Inline catch expression |
| OP_RAISE | - | Raise error |

### 3.10 Function/Verb Calls

| Opcode | Operands | Description |
|--------|----------|-------------|
| OP_CALL_BUILTIN | func_id, argc | Call builtin function |
| OP_CALL_VERB | argc | Pop obj; call obj:verb |
| OP_SCATTER | pattern | Scatter assignment |

### 3.11 Collection Operations

| Opcode | Operands | Description |
|--------|----------|-------------|
| OP_MAKE_LIST | count | Pop N items, make list |
| OP_MAKE_MAP | count | Pop N pairs, make map |
| OP_INDEX | - | Pop idx, coll; push coll[idx] |
| OP_INDEX_SET | - | Pop val, idx, coll; set coll[idx] |
| OP_RANGE | - | Pop end, start, coll; push slice |
| OP_LENGTH | - | Pop coll; push length |
| OP_SPLICE | - | Splice list |

---

## 4. Stack Frame

### 4.1 Structure

```go
type StackFrame struct {
    Program     *Program    // Bytecode
    IP          int         // Instruction pointer
    BasePointer int         // Stack base for this frame
    Locals      []Value     // Local variables
    This        int64       // Current object
    Player      int64       // Player context
    Verb        string      // Verb name
    Caller      int64       // Calling object
    LoopStack   []LoopState // Nested loop state
    ExceptStack []Handler   // Exception handlers
}
```

### 4.2 Local Variables

Variables stored by index (not name):

```go
// Compile time: map name → index
// Runtime: access by index
frame.Locals[varIndex]
```

### 4.3 Loop State

```go
type LoopState struct {
    Type      LoopType  // Range or List
    StartIP   int       // Loop body start
    EndIP     int       // After loop
    Label     string    // Optional name
    Iterator  any       // Current position
    End       any       // End value/index
}
```

### 4.4 Exception Handlers

```go
type Handler struct {
    Type       HandlerType  // Except or Finally
    HandlerIP  int          // Handler code location
    Codes      []ErrorCode  // Errors to catch (except)
    VarIndex   int          // Variable for error (except)
}
```

---

## 5. Operand Stack

### 5.1 Structure

The VM uses a single operand stack shared across all frames:

```go
type VM struct {
    Stack     []Value      // Operand stack
    SP        int          // Stack pointer
    Frames    []StackFrame // Call stack
    FP        int          // Frame pointer
}
```

### 5.2 Stack Operations

```go
func (vm *VM) Push(v Value) {
    vm.Stack[vm.SP] = v
    vm.SP++
}

func (vm *VM) Pop() Value {
    vm.SP--
    return vm.Stack[vm.SP]
}

func (vm *VM) Peek(offset int) Value {
    return vm.Stack[vm.SP-1-offset]
}
```

### 5.3 Frame Stack Management

On call:
```go
func (vm *VM) Call(prog *Program, args []Value) {
    frame := &StackFrame{
        Program:     prog,
        IP:          0,
        BasePointer: vm.SP - len(args),
        Locals:      make([]Value, prog.NumLocals),
    }
    // Copy args to locals
    copy(frame.Locals, args)
    vm.Frames = append(vm.Frames, frame)
}
```

On return:
```go
func (vm *VM) Return(value Value) {
    frame := vm.CurrentFrame()
    vm.SP = frame.BasePointer  // Restore stack
    vm.Frames = vm.Frames[:len(vm.Frames)-1]  // Pop frame
    vm.Push(value)  // Return value
}
```

---

## 6. Execution Loop

### 6.1 Main Loop

```go
func (vm *VM) Run() (Value, error) {
    for {
        frame := vm.CurrentFrame()
        if frame.IP >= len(frame.Program.Code) {
            break  // End of code
        }

        op := OpCode(frame.Program.Code[frame.IP])
        frame.IP++

        if err := vm.Execute(op); err != nil {
            if !vm.HandleError(err) {
                return nil, err
            }
        }

        // Tick counting
        if countsTick(op) {
            vm.Ticks++
            if vm.Ticks >= vm.TickLimit {
                return nil, ErrTicksExceeded
            }
        }
    }

    return vm.Pop(), nil
}
```

### 6.2 Opcode Dispatch

```go
func (vm *VM) Execute(op OpCode) error {
    switch op {
    case OP_PUSH:
        idx := vm.ReadByte()
        vm.Push(vm.CurrentFrame().Program.Constants[idx])

    case OP_ADD:
        b, a := vm.Pop(), vm.Pop()
        result, err := Add(a, b)
        if err != nil {
            return err
        }
        vm.Push(result)

    case OP_CALL_BUILTIN:
        funcID := vm.ReadByte()
        argc := vm.ReadByte()
        args := vm.PopN(int(argc))
        result, err := vm.CallBuiltin(funcID, args)
        if err != nil {
            return err
        }
        vm.Push(result)

    // ... more cases
    }
    return nil
}
```

---

## 7. Exception Handling

### 7.1 Error Propagation

```go
func (vm *VM) HandleError(err error) bool {
    frame := vm.CurrentFrame()

    for i := len(frame.ExceptStack) - 1; i >= 0; i-- {
        handler := frame.ExceptStack[i]

        if handler.Type == HandlerExcept {
            if handler.Matches(err) {
                frame.ExceptStack = frame.ExceptStack[:i]
                frame.IP = handler.HandlerIP
                if handler.VarIndex >= 0 {
                    frame.Locals[handler.VarIndex] = ErrorValue(err)
                }
                return true
            }
        }
    }

    // Propagate to caller
    if len(vm.Frames) > 1 {
        vm.Frames = vm.Frames[:len(vm.Frames)-1]
        return vm.HandleError(err)
    }

    return false  // Unhandled
}
```

### 7.2 Finally Execution

```go
func (vm *VM) ExecuteFinally(reason FinallyReason) {
    frame := vm.CurrentFrame()

    for i := len(frame.ExceptStack) - 1; i >= 0; i-- {
        handler := frame.ExceptStack[i]
        if handler.Type == HandlerFinally {
            savedIP := frame.IP
            savedReason := reason
            frame.IP = handler.FinallyIP

            // Execute finally block
            for frame.IP < handler.EndIP {
                vm.Step()
            }

            frame.IP = savedIP
            // Continue with saved reason
        }
    }
}
```

---

## 8. Short-Circuit Evaluation

### 8.1 AND Operator

```
OP_AND offset
```

Bytecode pattern:
```
<left expression>
OP_AND skip_offset    ; if false, jump to end
<right expression>
skip_offset:          ; result on stack
```

### 8.2 OR Operator

```
OP_OR offset
```

Bytecode pattern:
```
<left expression>
OP_OR skip_offset     ; if true, jump to end
<right expression>
skip_offset:          ; result on stack
```

---

## 9. Constant Pool

### 9.1 Constant Types

| Type | Description |
|------|-------------|
| Integer | Literal integers |
| Float | Literal floats |
| String | Literal strings |
| Object | Object IDs (#N) |
| Error | Error codes |
| List | Literal lists |
| Map | Literal maps |

### 9.2 Optimized Small Integers

Integers -10 to 143 encoded directly in opcode:

```go
const (
    OP_IMM_MIN = -10
    OP_IMM_MAX = 143
)

// OP_IMM_N encodes integer N directly
if n >= OP_IMM_MIN && n <= OP_IMM_MAX {
    emit(OP_IMM_BASE + n - OP_IMM_MIN)
} else {
    emit(OP_PUSH)
    emit(addConstant(IntValue(n)))
}
```

---

## 10. Line Number Mapping

### 10.1 Structure

```go
type LineEntry struct {
    StartIP int  // First IP for this line
    Line    int  // Source line number
}
```

### 10.2 Lookup

```go
func (p *Program) LineForIP(ip int) int {
    for i := len(p.LineInfo) - 1; i >= 0; i-- {
        if p.LineInfo[i].StartIP <= ip {
            return p.LineInfo[i].Line
        }
    }
    return 0
}
```

---

## 11. Builtin Function Registry

### 11.1 Registration

```go
type BuiltinFunc func(args []Value) (Value, error)

var builtins = map[string]BuiltinInfo{
    "length": {ID: 0, MinArgs: 1, MaxArgs: 1, Func: builtinLength},
    "tostr":  {ID: 1, MinArgs: 1, MaxArgs: -1, Func: builtinTostr},
    // ...
}
```

### 11.2 Calling Convention

```go
func (vm *VM) CallBuiltin(id int, args []Value) (Value, error) {
    info := builtinsByID[id]

    // Arg count validation
    if len(args) < info.MinArgs {
        return nil, ErrArgs
    }
    if info.MaxArgs >= 0 && len(args) > info.MaxArgs {
        return nil, ErrArgs
    }

    return info.Func(args)
}
```

---

## 12. Compilation

### 12.1 Compiler Structure

```go
type Compiler struct {
    program    *Program
    constants  map[Value]int  // Constant deduplication
    variables  map[string]int // Variable indices
    loops      []LoopContext  // For break/continue
    scopes     []Scope        // Variable scopes
}
```

### 12.2 Expression Compilation

```go
func (c *Compiler) compileExpr(expr Expr) {
    switch e := expr.(type) {
    case *IntLiteral:
        if e.Value >= OP_IMM_MIN && e.Value <= OP_IMM_MAX {
            c.emit(OP_IMM_BASE + e.Value - OP_IMM_MIN)
        } else {
            c.emitConstant(IntValue(e.Value))
        }

    case *BinaryExpr:
        c.compileExpr(e.Left)
        c.compileExpr(e.Right)
        c.emit(binaryOpcode(e.Op))

    case *VarExpr:
        idx := c.resolveVariable(e.Name)
        c.emit(OP_GET_VAR, idx)
    }
}
```

### 12.3 Statement Compilation

```go
func (c *Compiler) compileStmt(stmt Stmt) {
    switch s := stmt.(type) {
    case *IfStmt:
        c.compileExpr(s.Condition)
        elseJump := c.emitJump(OP_JUMP_IF_FALSE)
        c.compileBlock(s.Then)
        endJump := c.emitJump(OP_JUMP)
        c.patchJump(elseJump)
        if s.Else != nil {
            c.compileBlock(s.Else)
        }
        c.patchJump(endJump)

    case *ForStmt:
        c.beginLoop(s.Label)
        // ... compile loop body
        c.endLoop()
    }
}
```

---

## 13. VM State Serialization

For task suspension/resumption:

```go
type VMState struct {
    Stack      []Value
    Frames     []FrameState
    TaskLocal  map[Value]Value
}

type FrameState struct {
    ProgramID  int64
    IP         int
    Locals     []Value
    LoopStack  []LoopState
    ExceptStack []Handler
}

func (vm *VM) Serialize() *VMState { ... }
func (vm *VM) Deserialize(state *VMState) { ... }
```

---

## 14. Performance Considerations

### 14.1 Optimizations

| Optimization | Description |
|--------------|-------------|
| Constant folding | Evaluate constant expressions at compile time |
| Small int opcodes | Avoid constant pool for common integers |
| Direct dispatch | Use switch instead of function table |
| Stack caching | Keep top values in registers (advanced) |

### 14.2 Memory Layout

```go
// Pre-allocate stack
vm.Stack = make([]Value, 1024)

// Reuse frame objects
vm.framePool = sync.Pool{
    New: func() any { return &StackFrame{} },
}
```
