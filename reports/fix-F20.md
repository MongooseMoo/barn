# Fix F20 — unparse of `for value, index in (expr)` produced garbage range

## The bug
`parser/unparse.go` `ForStmt` branch tested `s.Index != ""` FIRST and emitted a
range using the index var as range-start and `len(s.Body)` as range-end:
`for L x in [k..1]`. A for-with-index loop is a *container* loop with two bound
variables, not a range loop.

## Correct MOO syntax (authority: ToastStunt grammar)
`C:/Users/Q/src/toaststunt/src/parser.y`:
- value only: `tFOR tID tIN '(' expr ')'` — `for x in (expr)` (parser.y:147-159)
- value + key/index: `tFOR tID ',' tID tIN '(' expr ')'` — `for value, index in (expr)`
  (parser.y:160-174). `$2`=value → `s.list.id`, `$4`=index → `s.list.index`.
  Comma BETWEEN the two ids; iterable wrapped in `( )`.
- range: `tFOR tID tIN '[' expr tTO expr ']'` — `for x in [a..b]` (parser.y:175-187)

## How Barn parses it (field mapping)
`parser/parser_stmt.go:225-240,313-322`: first id → `ForStmt.Value`, then if a
comma follows, the second id → `ForStmt.Index`; container expr → `Container`.
So unparse must emit `Value, Index` order — matches Toast `$2,$4`.

## The fix
`parser/unparse.go`: branch on `s.Container != nil` first (it is the container
form whether or not an index var is present); within it, if `s.Index != ""`
emit `value, index in (container)`, else `value in (container)`. The range
branch (`Container == nil`) is unchanged. Label still prepended when present.

## Evidence
- Red test now passes: `TestReview_UnparseForWithIndexVar`
  - input `for L x, k in (mylist)\nreturn x;\nendfor`
  - unparse → `for L x, k in (mylist)\n  return x;\nendfor`
  - round-trip: re-parse + re-unparse identical (asserted in test).
- `go test ./parser/... -run 'Unparse|For' -v` → PASS
- `go vet ./parser/` → clean
- Full `go test ./parser/...`: only 2 FAILs remain, both unrelated pre-existing
  intentionally-red review tests (`TestReview_EIntrptLiteralRejected`,
  `TestReview_BreakLabelAsIdentExpr`) — different findings, untouched by this change.

## Before / after
- Before: `for L x in [k..1]` (wrong: range syntax, index as start, body-count as end)
- After:  `for L x, k in (mylist)` (correct container+key syntax, stable round-trip)

## Commit
COMMIT_HASH_PLACEHOLDER (branch review/branch-stocktake-2026-06-25)
