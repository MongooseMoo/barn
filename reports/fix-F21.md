# Fix F21 — `break <label>` stored in wrong AST field

## The bug
`parseBreakStatement` never set `BreakStmt.Label`. `break myloop;` parsed the
loop name as an arbitrary expression into `BreakStmt.Value`, while
`continue myloop;` correctly set `ContinueStmt.Label`. The compiler partly
compensated (`findLoopByTarget`-via-Value), so `break nonexistent;` *silently*
compiled as a break-with-value instead of raising "Invalid loop name" the way
`continue nonexistent;` does.

## Authority — ToastStunt (the spec)
`break`/`continue` take only an optional **loop name** (no value expression):
- `C:/Users/Q/src/toaststunt/src/parser.y:241-252` — `tBREAK ';'` and
  `tBREAK tID ';'`; both call `check_loop_name(..., LOOP_BREAK)`. There is no
  `break expr` production.
- `parser.y:253-264` — the symmetric `continue` productions.
- `parser.y:1187-1209` — `check_loop_name`: an unknown name is rejected with
  `error("Invalid loop name in `break' statement: ", name)` (`:1205-1206`) /
  `... `continue' statement` ...` (`:1207-1208`). Break and continue validate
  identically.

So Barn's `break expr;`/`Value` mechanism was non-standard; Toast has no such
thing. The fix makes break a mirror of continue.

## Barn's continue path (the in-repo reference)
- Parser `parser/parser_stmt.go` `parseContinueStatement`: reads an optional
  `TOKEN_IDENTIFIER` into `ContinueStmt.Label`.
- AST `parser/ast.go`: `ContinueStmt{ Pos, Label }`.
- Compiler `bytecode/compiler.go` `compileContinue`: `loop :=
  findLoopByTarget(n.Label)`; if `loop == nil && n.Label != ""` →
  `fmt.Errorf("Invalid loop name")`. `findLoopByTarget` matches a loop by its
  explicit label OR loop-variable/index name (matching Toast, which
  `push_loop_name`s the loop variable id).

## Changes (break now mirrors continue)
1. `parser/ast.go` — removed `BreakStmt.Value`; `BreakStmt` is now
   `{ Pos, Label }`, doc cites parser.y:241-252.
2. `parser/parser_stmt.go` `parseBreakStatement` — parses an optional
   `TOKEN_IDENTIFIER` into `Label` (byte-for-byte the continue shape); no longer
   parses an arbitrary expression.
3. `parser/unparse.go` — dropped the dead `s.Value` branch; emits
   `break;` / `break LABEL;`.
4. `bytecode/compiler.go` `compileBreak` — now `loop :=
   findLoopByTarget(n.Label)`; unknown named loop → `Invalid loop name`; else
   emits the forward jump. Removed the `findLoopByTarget`-via-Value
   compensation and the `OP_SET_VAR loop.ResultVar` break-value store (both
   dead now that `Value` is gone). Loops still initialise their result slot to
   0 as before.

## Confirmation
- `break nonexistent;` now errors at compile time with "Invalid loop name"
  (`bytecode/break_label_test.go` `TestBreakUnknownLoopNameIsCompileError`),
  matching the continue baseline (`TestContinueUnknownLoopNameIsCompileError`).
- Labeled break/continue still run: `break i;` from an inner loop exits the
  outer loop named `i`; `continue i;` continues it; plain `break;`/`continue;`
  unchanged (`vm/break_label_test.go`, 4 tests).

## Test output
- `go test ./parser/... -run 'Break|Continue|Loop|Label' -v` →
  `TestReview_BreakLabelAsIdentExpr` PASS (was the red test).
- `go test ./vm/... -run 'Break|Continue|Labeled|Plain' -v` → 4 PASS.
- `go test ./bytecode/... -run 'Break|Continue|Loop|Label' -v` → 3 PASS.
- `go vet ./parser/` → exit 0.

### Before / after (full package runs)
`go test ./parser/... ./vm/... ./bytecode/...` — before and after, the only
failures are **pre-existing unrelated RED tests** for other findings:
`parser TestReview_EIntrptLiteralRejected` (E_INTRPT literal) and
`vm TestReview_MapInChecksValuesNotKeys` / `...ValueFoundAsKey_ReturnsZero`
(map `in` scans values not keys). None touch break/continue. The F21 red test
flipped red→green; `bytecode` is fully green.

(The `vm/stack.go:49 ReadByte` vet note is pre-existing and outside the prompt's
`go vet ./parser/` gate.)

## Commit
`<filled in below>` on `review/branch-stocktake-2026-06-25`.
