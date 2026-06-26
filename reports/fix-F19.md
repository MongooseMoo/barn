# Fix F19 — list-literal expression rejected as a statement

## Finding
`{x, y};` at statement position was rejected with "syntax error". `reports/review-frontend.md`
BUG-5. Red test: `parser/review_test.go:30 TestReview_ListExprAsStatementMistakenForScatter`.

Root cause: `parser/parser_stmt.go looksLikeScatter()` committed to scatter parsing for any
`{` followed by IDENTIFIER / `?` / `@`, with **no `=` lookahead and no backtracking**. So any
list expression beginning with an identifier used as a statement was sent to
`parseScatterStatement`, which then failed ("scatter must be followed by '='").

## How ToastStunt distinguishes scatter vs list-literal
A brace-group is fundamentally a **list literal**; it is a scatter target only when a
top-level `=` follows the matching `}`. From `C:/Users/Q/src/toaststunt/src/parser.y`:

- `parser.y:630` — `expr: '{' arglist '}'` → reduces to `EXPR_LIST` (the list literal).
- `parser.y:466-487` — `expr: expr '=' expr` → if the LHS is an `EXPR_LIST`, it is
  *reinterpreted* as `EXPR_SCATTER`. The `{...}` only becomes scatter because an `=` follows.
- `parser.y:488-495` — `expr: '{' scatter '}' '=' expr` → dedicated production for scatter
  with optional (`?x`), default (`?x = e`) and rest (`@x`) targets.

The LALR parser chooses between "reduce `{...}` to a list" and "shift into scatter
assignment" using one token of lookahead on `=`. Net rule: **scatter iff a top-level `=`
follows the matching `}`.**

## Oracle cross-check (WSL strict master, conformance Test.db)
```
{1, 2}                         => {1, 2}      # bare list literal statement
{1, 2}[1]                      => 1           # list literal, indexed (no '=')
{a, b} = {1, 2}                => {1, 2}      # scatter assignment
{?c = 5, @d} = {7, 8, 9}       => {7, 8, 9}   # optional/default/rest scatter
```
Toast accepts list-literal expression statements AND scatter assignment, exactly per the rule.

## The fix
Rewrote `looksLikeScatter()` (`parser/parser_stmt.go`) to do proper lookahead instead of a
one-token guess. It clones the lexer (`Lexer` is a pure value struct — no shared mutable
state, so a copy is an independent cursor), then scans the brace group tracking `()`,`[]`,`{}`
nesting starting at depth 1 (the opening `{` is already current). When depth returns to 0
(the matching `}`), it returns true **iff the next token is `TOKEN_ASSIGN`**.

- `{x, y};`      → next token `;` → not scatter → expression statement (FIXED)
- `{a, b} = rhs` → next token `=` → scatter statement (preserved, incl. `?`/`@`/defaults)
- `{a, b}[1];`   → next token `[` → expression statement (IndexExpr over ListExpr)

The scatter statement path (`parseScatterStatement` → `compileScatter`) is untouched, so all
optional/rest/default scatter behavior and its error messages are preserved. Statement-start
`{...} = rhs` still routes through the full scatter path (not the limited
`AssignExpr{Target: ListExpr}` expression path).

## Verification
Before: `TestReview_ListExprAsStatementMistakenForScatter` FAILED ("{x, y}; ... rejected: syntax error").
After:
```
go test ./parser/... -run 'Scatter|List|Statement' -v   # all PASS incl. the F19 test
go test ./vm/...      -run 'Scatter|List'                # scatter executes: all PASS
go test ./bytecode/...                                   # ok
go vet ./parser/                                         # clean
```
Remaining `./parser/...` and `./vm/...` failures (E_INTRPT, UnparseForWithIndexVar,
BreakLabelAsIdentExpr, MapIn*) are unrelated pre-existing intentionally-red review tests for
other findings; none touch scatter/list parsing.

Commit: <FILLED AFTER COMMIT>
