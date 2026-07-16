# Review Task: Range Loop Binding Grammar Correction

## Context

Phase 3.3 splits the nullable semantic `ForStmt` into distinct collection and
range loop statements. During implementation, the old parser was found to
accept `for value, index in [start..end]`, even though the approved semantic
contract gives only collection loops an optional index/key binding.

ToastStunt 2.7.3_5 was checked through the documented WSL oracle. Evaluating
`for x, i in [1..2] return x; endfor` returned
`{0, {"Line 1:  syntax error"}}`. Its `src/parser.y` likewise has separate
productions for one- and two-binding collection loops, but only a one-binding
range production (lines 147-185).

The current worktree changes `spec/grammar.md` so the optional second identifier
belongs only to the collection production. `spec/statements.md` already shows
the same concrete forms and the approved semantic-normalization contract already
requires distinct variants, with no index/key field on range loops.

## Files to Read

- `spec/grammar.md`
- `spec/statements.md`, especially Sections 4.1-4.4 and 13.1
- `spec/vm.md`, especially Section 12.4
- `plans/multilingual-verb-language-cleanup-plan.md`, especially Phase 3.3
- `parser/parser_stmt.go`
- `parser/parser_stmt_test.go`
- `verb/ir.go`
- the current Git diff

## Review Criteria

1. Does the grammar now match Toast and the existing statement examples?
2. Does it express that index/key bindings are collection-only without changing
   collection, range, label, break, or continue semantics?
3. Does the implementation directly construct distinct semantic loop variants
   without an adapter, compatibility type, nullable semantic discriminator, or
   unrelated language change?
4. Are the parser rejection and structural tests sufficient for this boundary?

## Output

Begin with exactly `APPROVE`, `CONCERNS`, or `REJECT`, then provide concise,
source-cited findings. Modify no production code, tests, specifications, plan,
prompt, or existing report. Write only the report path named by the invoking
command.
