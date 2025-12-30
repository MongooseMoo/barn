# Fix add_property() E_INVARG Error - Investigation Report

## Summary

**STATUS: NO BUG FOUND**

The `add_property()` builtin works correctly. The issue was a **syntax error in the test command** - unescaped quotes inside the eval string literal.

## Investigation

### Test 1: Direct add_property call
```moo
; return add_property(#0, "temp", 0, { #3, "rwc" });
```
**Result:** `{1, 0}` - **SUCCESS**

### Test 2: Original failing test from prompt
```moo
; return eval("add_property(#0, "temp", 0, { #3, "rwc" }); return 0;");
```
**Result:** `{0, E_INVARG}` - **FAILS**

### Test 3: With properly escaped quotes
```moo
; return eval("add_property(#0, \"temp\", 0, { #3, \"rwc\" }); return 0;");
```
**Result:** `{1, {1, 0}}` - **SUCCESS**

### Test 4: Verification that properties were created
```moo
; return properties(#0);
```
**Result:** `{1, {"temp", "temp3", "temp5"}}` - All test properties exist

## Root Cause

The original test command used **unescaped quotes** inside an eval string literal:

```bash
# WRONG - parser sees: eval("add_property(#0, "
printf 'connect programmer\n; return eval("add_property(#0, "temp", ...)");\n'
```

The MOO parser treats `"add_property(#0, "` as the string (stops at second quote), then encounters `temp` as an unexpected identifier, causing a parse error. When `ParseProgram()` fails, `EvalString()` returns `E_INVARG` (line 497 in `vm/eval.go`).

## Correct Usage

Quotes inside eval strings must be escaped:

```bash
# CORRECT - parser sees: eval("add_property(#0, \"temp\", ...)")
printf 'connect programmer\n; return eval("add_property(#0, \\"temp\\", 0, { #3, \\"rwc\\" }); return 0;");\n'
```

Or use a variable to avoid nested quotes:

```moo
; x = "add_property(#0, \"temp\", 0, { #3, \"rwc\" });";
; return eval(x);
```
**Result:** `{1, {1, 0}}` - SUCCESS

## Parser Validation

Tested the parser directly with various inputs:

1. `{ #3, "rwc" }` as literal → **parses OK**
2. `return { #3, "rwc" };` as statement → **parses OK**
3. `add_property(#0, "temp", 0, { #3, "rwc" }); return 0;` → **parses OK**
4. `eval("add_property(#0, "temp", ...)")` → **parse error: expected ')' after function args, got IDENTIFIER**

The parser is working correctly. It stops at the first unescaped closing quote.

## Conclusion

**No code changes needed.** The `add_property()` builtin and the parser are both working correctly.

The issue was in the **test command syntax** - unescaped quotes created a malformed MOO expression that the parser correctly rejected.

## Files Analyzed

- `C:\Users\Q\code\barn\builtins\properties.go` (lines 174-252) - add_property implementation
- `C:\Users\Q\code\barn\vm\builtin_eval.go` (lines 8-48) - eval() implementation
- `C:\Users\Q\code\barn\vm\eval.go` (lines 490-515) - EvalString() method
- `C:\Users\Q\code\barn\parser\parser_list.go` - list literal parsing
- `C:\Users\Q\code\barn\parser\parser_obj.go` - object literal parsing

All implementations are correct and match MOO specification.
