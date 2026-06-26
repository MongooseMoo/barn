# Fix F29 — E_INTRPT rejected as an unknown error literal

## Toast source (authority; WSL oracle down)
- `C:/Users/Q/src/toaststunt/src/include/structures.h:70-74` defines `enum error { ... }`.
  Header comment (`:65-69`) states the order defines the numeric equivalents and they are
  DB-stored. The enum, in order:
  `E_NONE=0, E_TYPE=1, E_DIV=2, E_PERM=3, E_PROPNF=4, E_VERBNF=5, E_VARNF=6, E_INVIND=7,
  E_RECMOVE=8, E_MAXREC=9, E_RANGE=10, E_ARGS=11, E_NACC=12, E_INVARG=13, E_QUOTA=14,
  E_FLOAT=15, E_FILE=16, E_EXEC=17, E_INTRPT=18`.
- **`E_INTRPT` exists at `structures.h:73`, value 18 (last element), canonical spelling
  `E_INTRPT`.** Toast's compiler accepts every enum name as an error literal, so `E_INTRPT`
  is a valid literal; `toint(E_INTRPT)` == 18 and `tostr(E_INTRPT)` == "E_INTRPT".

## Barn before/after
- Barn **already had** the constant: `types/errorcode.go:26` `E_INTRPT ErrorCode = 18`, plus
  `String()` (:68), `Message()` (:115), and `ErrorFromString()` (:161) cases. Value already
  matched Toast.
- The **only** gap: `parser/parser_error.go` `errorNames` map listed E_NONE..E_EXEC (18
  entries) but omitted `E_INTRPT`. `isErrorName("E_INTRPT")` returned false, so the parser
  (`parser/parser.go:630`, `:691`, `parser/parser_stmt.go:551`) rejected the literal with
  "unknown error code".

## Fix
- Added `"E_INTRPT": {}` to `errorNames` (`parser/parser_error.go`). No other table needed
  changes — value resolution flows through `types.ErrorFromString` (used by
  `bytecode/parser_literals.go:33`, `vm/error.go:41`, `builtins/json.go:342`), which already
  knew E_INTRPT. No other error codes perturbed.

## Round-trip verified
Parser accepts `E_INTRPT`, records `LiteralErr` with `ErrorName=="E_INTRPT"`, resolves to
`types.E_INTRPT` == 18; `String()`/unparse emit `E_INTRPT`.

## Test
- `parser/fix_f29_test.go` `TestFixF29_EIntrptParsesToCode18`: asserts `isErrorName`,
  successful parse, AST `LiteralErr`/`ErrorName=="E_INTRPT"`, and numeric value 18.
- Analyst red test `parser/review_test.go` `TestReview_EIntrptLiteralRejected` now passes.

### Fail-against-old (line temporarily removed)
```
--- FAIL: TestFixF29_EIntrptParsesToCode18  isErrorName("E_INTRPT") is false
--- FAIL: TestReview_EIntrptLiteralRejected  parser rejected it: syntax error
```

### Green gate
```
ok  barn/parser   0.400s
ok  barn/types    (cached)
ok  barn/vm       0.904s
go vet ./parser/  (clean)
```

Commit: <filled at commit time>
