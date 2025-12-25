# MOO_INTERP Codebase Exploration Report

## 1. Project Overview

**MOO-Interp** is a complete Python implementation of a MOO interpreter. It implements:
- Full parser for MOO language (using Lark/Earley)
- Bytecode compiler (AST → stack-based VM instructions)
- Stack-based virtual machine
- 100+ built-in functions
- Special MOO data types with 1-based indexing
- Full exception handling (try/catch/finally)
- Verb calling and object inheritance
- Database integration with lambdamoo-db

**Key Files:**
- `moo_interp/parser.lark` - Grammar definition (216 lines)
- `moo_interp/moo_ast.py` - AST nodes, compiler (1,629 lines)
- `moo_interp/vm.py` - Virtual machine (1,741 lines)
- `moo_interp/builtin_functions.py` - Built-ins (2,514 lines)
- `moo_interp/opcodes.py` - Opcode definitions (143 lines)

## 2. Architecture

3-phase pipeline:
```
MOO Source → Parser → AST → Compiler → Bytecode → VM → Result
```

## 3. Parser Architecture

Uses Lark's Earley parser. Grammar in `parser.lark`.

### Language Constructs
**Statements:**
- If/ElseIf/Else, For loops, While loops
- Fork (async), Try/Except/Finally
- Break/Continue (with labels), Return

**Expressions (Precedence low→high):**
1. Assignment (`=`)
2. Ternary (`? |`)
3. Catch (`` `expr ! error` ``)
4. Splicer (`@`)
5. Logical (`||`, `&&`)
6. Bitwise (`|.`, `^.`, `&.`, `>>`, `<<`)
7. Comparison (`<`, `>`, `<=`, `>=`, `==`, `!=`, `in`)
8. Arithmetic (`+`, `-`, `*`, `/`, `%`, `^`)
9. Unary (`!`, `~`, `-`)
10. Postfix (property, index, verb call)

**Literals:**
- Numbers, floats, strings, booleans
- Objects (`#0`, `#123`)
- Lists (`{1, 2, 3}`), Maps (`["key" -> value]`)

## 4. AST Nodes (moo_ast.py)

### Main API
```python
def parse(text: str) -> VerbCode        # Parse to AST
def compile(tree, ...) -> StackFrame    # Compile to bytecode
def run(frame, ...) -> VM               # Execute
```

### Key Node Classes
- **Literals:** StringLiteral, NumberLiteral, FloatLiteral, BooleanLiteral, ObjnumLiteral, _List, Map
- **Operators:** BinaryExpression, _UnaryExpression, _Ternary
- **Control:** _IfStatement, ForStatement, WhileStatement, ReturnStatement
- **Access:** Identifier, _Property, _Index, _Range
- **Calls:** _FunctionCall, _VerbCall, DollarVerbCall

## 5. Opcodes (opcodes.py)

### Regular Opcodes (51 variants)
- Control: `OP_IF`, `OP_WHILE`, `OP_FOR_*`, `OP_JUMP`
- Arithmetic: `OP_ADD`, `OP_MINUS`, `OP_MULT`, `OP_DIV`
- Comparison: `OP_EQ`, `OP_NE`, `OP_LT`, `OP_GT`
- Variables: `OP_PUSH`, `OP_PUT`, `OP_IMM`
- Lists/Maps: `OP_MAKE_EMPTY_LIST`, `OP_MAP_CREATE`
- Functions: `OP_BI_FUNC_CALL`

### Extended Opcodes (25 variants)
- Bitwise: `EOP_BITOR`, `EOP_BITAND`, `EOP_BITXOR`
- Control: `EOP_CATCH`, `EOP_TRY_EXCEPT`, `EOP_SCATTER`

### Optimized Numbers
Integers -10 to 192 encoded as single opcode (no operand).

## 6. Virtual Machine (vm.py)

### VM Structure
```python
class VM:
    stack: List[Any]                  # Value stack
    call_stack: List[StackFrame]      # Call frames
    result: MOOAny                    # Final result
    state: VMOutcome                  # DONE/ABORTED/BLOCKED
    db: MooDatabase                   # Object storage
```

### Stack Frame
```python
class StackFrame:
    prog: Program                     # Bytecode
    ip: int                           # Instruction pointer
    rt_env: List[Any]                 # Local variables
    this: int                         # Current object
    player: int                       # Player object
    verb: str                         # Verb name
    loop_stack: List[Any]             # Loop state
    exception_stack: List[Any]        # Exception handlers
```

### Execution Model
- Stack-based operations
- Frame-based function calls
- Error handling via exception stack

## 7. MOO Data Types

### MOOString
- 1-based indexing
- Mutable
- Methods: find, index, split, etc.

### MOOList
- 1-based indexing
- Reference counting for copy-on-write
- Methods: append, insert, delete

### MOOMap
- Dictionary with string/number keys
- Reference counting

## 8. Built-in Functions (165 total)

**Type Conversion:** tostr, toint, tofloat, toliteral

**Math:** min, max, floor, ceil, sqrt, sin, cos, etc.

**String:** length, explode, index, strsub, strcmp

**List/Map:** listappend, listdelete, mapkeys, mapvalues

**Object/Property:** property_info, add_property, delete_property

**File I/O:** file_open, file_read, file_write

**Time:** time, ftime, ctime

**JSON:** generate_json, parse_json

## 9. Error Codes

```python
E_NONE = 0      # No error
E_TYPE = 1      # Type mismatch
E_DIV = 2       # Division by zero
E_PERM = 3      # Permission denied
E_PROPNF = 4    # Property not found
E_VERBNF = 5    # Verb not found
E_VARNF = 6     # Variable not found
E_INVIND = 7    # Invalid index
E_RANGE = 10    # Range error
E_ARGS = 11     # Wrong number of arguments
E_INVARG = 13   # Invalid argument
```

## 10. Key Implementation Details

### Short-Circuit Evaluation
`||` and `&&` generate special bytecode to avoid unnecessary evaluation.

### Copy-on-Write
Lists and maps track reference count. `shallow_copy()` for modifications.

### Variable Management
- Local variables in `CompilerState.var_names`
- Runtime environment indexed by variable position

### Verb Inheritance
`_find_verb()` walks up inheritance chain with visited set.

## 11. Go Porting Considerations

1. **Parser:** Need Earley parser or PEG alternative
2. **AST Generation:** Manual in Go (no Lark equivalent)
3. **Dynamic Typing:** MOO is dynamic, Go is static
4. **Copy-on-Write:** Requires reference tracking
5. **Built-in Registry:** Use reflection or code generation
6. **Database:** Port lambdamoo-db format

## 12. File Summary

| File | Lines | Purpose |
|------|-------|---------|
| `parser.lark` | 216 | Grammar |
| `moo_ast.py` | 1,629 | AST + compiler |
| `vm.py` | 1,741 | Virtual machine |
| `builtin_functions.py` | 2,514 | Built-ins |
| `opcodes.py` | 143 | Opcodes |
| `string.py` | 100 | MOOString |
| `list.py` | 70 | MOOList |
| `map.py` | 44 | MOOMap |
| `errors.py` | 36 | Error codes |
