# Task Report: Add -eval flag to CLI

## Status: Complete

## Implementation Summary

Successfully added the `-eval` flag to the barn CLI for evaluating MOO expressions directly without starting the server.

## Changes Made

### File: `cmd/barn/main.go`

1. **Added imports**:
   - `barn/parser` - for parsing MOO expressions
   - `barn/vm` - for evaluating expressions

2. **Added `-eval` flag**:
   - Added flag declaration: `evalExpr := flag.String("eval", "", "Evaluate a MOO expression (e.g., \"1 + 2\")")`
   - Updated `isInspection` check to include `-eval`
   - Added handler call in inspection block

3. **Implemented `evalExpression()` function**:
   - Parses the expression using `parser.NewParser()` and `ParseExpression(0)`
   - Creates an evaluator with the database store using `vm.NewEvaluatorWithStore(store)`
   - Creates a fresh task context using `types.NewTaskContext()`
   - Evaluates the expression and prints results
   - Success: prints `=> <value>` in MOO literal format
   - Error: prints `Error: <error_code>`

## Test Results

All test cases from the prompt specification passed:

### Arithmetic
```bash
$ barn.exe -db toastcore.db -eval "1 + 2"
=> 3
```

### List operations
```bash
$ barn.exe -db toastcore.db -eval "{1, 2, 3}"
=> {1, 2, 3}

$ barn.exe -db toastcore.db -eval "{1, 2, 3}[2]"
=> 2
```

### Database builtins
```bash
$ barn.exe -db toastcore.db -eval "verbs(#10)"
=> {"blacklisted graylisted redlisted spooflisted", "record_connection", ...}

$ barn.exe -db toastcore.db -eval "verb_args(#10, 3)"
=> {"", "", ""}
```

### Literals
```bash
$ barn.exe -db toastcore.db -eval "#10"
=> #10

$ barn.exe -db toastcore.db -eval '"hello"'
=> "hello"

$ barn.exe -db toastcore.db -eval "E_VERBNF"
=> E_VERBNF
```

### Error handling
```bash
$ barn.exe -db toastcore.db -eval "1 / 0"
Error: E_DIV
```

## Implementation Notes

1. **Pattern consistency**: Followed the same pattern as other inspection flags (`-verb-code`, `-obj-info`, etc.)

2. **Database integration**: Uses the same database loading mechanism as other inspection flags, ensuring the evaluator has access to all database objects, verbs, and properties.

3. **Evaluator setup**: Used `vm.NewEvaluatorWithStore()` which:
   - Creates a fresh environment
   - Registers all builtins (object, property, verb, eval, pass)
   - Sets up the verb caller callback
   - Provides full database access

4. **Output format**: Uses the `Value.String()` method which provides proper MOO literal formatting:
   - Strings are quoted and escaped
   - Lists use `{...}` syntax
   - Maps use `[...]` syntax
   - Objects use `#N` syntax
   - Error codes show as `E_CODE`

5. **Error handling**: Distinguishes between:
   - Parse errors (malformed syntax)
   - Runtime errors (E_DIV, E_TYPE, etc.)

## Usage Examples

The `-eval` flag is useful for:
- Quick debugging of expressions
- Testing builtin functions
- Inspecting database objects
- Verifying MOO semantics
- REPL-style exploration

```bash
# Test a builtin
barn.exe -db toastcore.db -eval "typeof(#10)"

# Check object properties
barn.exe -db toastcore.db -eval "parent(#10)"

# Test string operations
barn.exe -db toastcore.db -eval '"hello" + " " + "world"'

# Verify arithmetic
barn.exe -db toastcore.db -eval "2 ^ 10"
```

## Completeness

- ✅ All test cases from prompt pass
- ✅ Error handling works correctly
- ✅ Database integration functional
- ✅ Output format matches MOO literal syntax
- ✅ Pattern consistent with existing inspection flags
- ✅ No file modified errors encountered
