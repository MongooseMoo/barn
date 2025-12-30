# Fix Plan: index_and_range::decompile_with_index_operators

## Test Summary

**Test:** `index_and_range::decompile_with_index_operators`

**Input code:**
```moo
o = create($nothing);
add_verb(o, {player, "xd", "foobar"}, {"this", "none", "this"});
set_verb_code(o, "foobar", {"return \"foobar\"[^ + 2 ^ 2 .. $ - #0.off];"});
vc = verb_code(o, "foobar");
recycle(o);
return vc;
```

**Expected output:**
```
["return \"foobar\"[^ + 2 ^ 2..$ - $off];"]
```

**Actual Barn output:**
```
["return \"foobar\"[^ + 2 ^ 2 .. $ - #0.off];"]
```

**Differences:**
1. Range operator spacing: ` .. ` should be `..` (no spaces)
2. System object property format: `#0.off` should be `$off`

## Root Cause Analysis

Barn's `verb_code()` builtin (in `builtins/verbs.go:267-310`) currently returns the **original source code** from `verb.Code[]` without any transformation:

```go
func builtinVerbCode(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
    // ... permission checks ...

    // Convert source lines to list
    lines := make([]types.Value, len(verb.Code))
    for i, line := range verb.Code {
        lines[i] = types.NewStr(line)
    }
    return types.Ok(types.NewList(lines))
}
```

However, ToastStunt's `verb_code()` (in `src/verbs.cc`) calls `unparse_program()` to **decompile** the compiled bytecode back to normalized source code:

```c
code = new_list(0);
unparse_program(db_verb_program(h), lister, &code, parens, indent, MAIN_VECTOR);
```

This decompilation process:
1. Walks the compiled AST
2. Produces normalized source code
3. Applies formatting conventions (no spaces around `..`, etc.)
4. Converts `#0.property` to `$property` syntax sugar

**Barn is missing the entire decompilation/unparsing subsystem.**

## Implementation Plan

### Step 1: Create AST Unparser (`parser/unparse.go`)

Create a new file that walks AST nodes and produces formatted source strings:

**Key functions needed:**
- `UnparseProgram(stmts []Stmt) []string` - Entry point, returns lines of code
- `unparseStmt(stmt Stmt, indent int) string` - Format a statement
- `unparseExpr(expr Expr, precedence int) string` - Format an expression with proper parenthesization
- Helper functions for each expression type

**Critical formatting rules:**

1. **Range expressions** (RangeExpr):
   - Format: `base[start..end]` with NO spaces around `..`
   - Example: `"foobar"[^ + 1..$]` not `"foobar"[^ + 1 .. $]`

2. **Property references** (PropertyExpr):
   - If object is `#0` (system object), use `$property` syntax
   - Otherwise use `obj.property` or `#N.property` syntax
   - Example: `#0.off` → `$off`

3. **Index marker expressions** (IndexMarkerExpr):
   - `^` for TOKEN_CARET (first)
   - `$` for TOKEN_DOLLAR (last)

4. **Binary operators** - Need proper precedence to avoid unnecessary parens:
   - Exponentiation (`^`) binds tighter than addition
   - Example: `^ + 2 ^ 2` correctly produces 1 + 4 = 5, not (1+2)^2 = 9

5. **Object literals**:
   - `#N` format for object IDs

6. **String literals**:
   - Escape special characters properly
   - Use double quotes

**Precedence table needed:**
```
Highest:
  Property access, indexing, verb calls
  Unary: -, !, ~
  Exponentiation: ^
  Multiplicative: *, /, %
  Additive: +, -
  Bitwise shifts: <<, >>
  Relational: <, <=, >, >=, ==, !=, in
  Bitwise: &, |
  Logical: &&, ||
  Ternary: ? |
Lowest:
  Assignment: =
```

### Step 2: Integrate Unparser into verb_code()

Modify `builtins/verbs.go:builtinVerbCode()`:

```go
func builtinVerbCode(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
    // ... existing validation ...

    // Decompile from AST if program exists
    var lines []string
    if verb.Program != nil && len(verb.Program.Statements) > 0 {
        lines = parser.UnparseProgram(verb.Program.Statements)
    } else {
        // Fallback to original source if not compiled
        lines = verb.Code
    }

    // Convert to Value list
    result := make([]types.Value, len(lines))
    for i, line := range lines {
        result[i] = types.NewStr(line)
    }

    return types.Ok(types.NewList(result))
}
```

### Step 3: Handle Optional Parameters

The full signature is: `verb_code(object, verb-desc [, fully-paren [, indent]])`

- `fully_paren` - If true, add extra parentheses for clarity
- `indent` - If true, indent nested statements

For the failing test, both default to false/simple formatting.

Initially implement without these options, then add support later if needed.

### Step 4: Test Coverage

**Unit tests for unparser** (`parser/unparse_test.go`):
- Range expressions with various operators
- Property access on #0 vs other objects
- Index markers (^ and $)
- Binary operators with correct precedence
- String escaping
- Object literal formatting

**Integration test:**
Run the failing conformance test:
```bash
cd C:/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9300 -k "decompile_with_index_operators" -v
```

Should produce:
```
["return \"foobar\"[^ + 2 ^ 2..$ - $off];"]
```

## Files to Modify

### New Files
1. `C:\Users\Q\code\barn\parser\unparse.go` - AST to source decompiler
2. `C:\Users\Q\code\barn\parser\unparse_test.go` - Unit tests

### Modified Files
1. `C:\Users\Q\code\barn\builtins\verbs.go` - Update `builtinVerbCode()` to use unparser
2. `C:\Users\Q\code\barn\db\verbs.go` - Potentially add helper to get program from verb

## Implementation Details

### Property Expression Special Case

When unparsing a PropertyExpr:
```go
func unparsePropertyExpr(e *PropertyExpr) string {
    // Check if base is #0
    if lit, ok := e.Expr.(*LiteralExpr); ok {
        if obj, ok := lit.Value.(types.ObjValue); ok {
            if obj.ID() == 0 {
                // Use $property syntax for system object
                return "$" + e.Property
            }
        }
    }

    // Otherwise use obj.property syntax
    base := unparseExpr(e.Expr, precedenceProperty)
    return base + "." + e.Property
}
```

### Range Expression Formatting

When unparsing a RangeExpr:
```go
func unparseRangeExpr(e *RangeExpr) string {
    base := unparseExpr(e.Expr, precedenceIndex)
    start := unparseExpr(e.Start, precedenceRange)
    end := unparseExpr(e.End, precedenceRange)
    // No spaces around ..
    return base + "[" + start + ".." + end + "]"
}
```

## Estimated Complexity

**Medium**

**Reasoning:**
- Need to create a complete AST walker (~300-500 lines)
- Precedence handling requires careful thought
- Special cases for property syntax and formatting
- Must handle all expression types in parser/ast.go
- Testing requires understanding of MOO operator precedence

**Time estimate:** 4-6 hours
- 2-3 hours: Implement unparser with basic expressions
- 1-2 hours: Handle all edge cases (properties, ranges, operators)
- 1 hour: Testing and debugging

## Dependencies

None - this is a standalone feature addition.

## Risks

1. **Operator precedence errors** - Could produce code that parses differently
   - Mitigation: Comprehensive test suite, compare with toast_oracle

2. **String escaping bugs** - Could break on special characters
   - Mitigation: Use existing types.Value.String() methods where possible

3. **Incomplete AST coverage** - Some expression types might be missed
   - Mitigation: Walk through all types in parser/ast.go systematically

## Verification Strategy

1. Run failing test - should pass
2. Run all index_and_range tests - should still pass
3. Test with toast_oracle:
   ```bash
   ./toast_oracle.exe 'o = create($nothing); add_verb(o, {#2, "x", "test"}, {"this", "none", "this"}); set_verb_code(o, "test", {"return #0.foo;"}); return verb_code(o, "test");'
   ```
   Should return `["return $foo;"]`

4. Add tests for edge cases:
   - Nested ranges: `x[1..3][2..2]`
   - Complex expressions in ranges: `x[^ + 2 ^ 2..$ - 1]`
   - Property chains: `#0.foo.bar` (probably stays as-is)
   - Dynamic properties: `obj.(prop)` (parentheses required)

## Future Enhancements

After basic implementation works:
1. Support `fully_paren` parameter for extra clarity
2. Support `indent` parameter for formatted output
3. Add line number tracking for multi-line verbs
4. Optimize for readability (e.g., line breaking for long expressions)
