# Verification Task: Range Loop Binding Spec Integration

Independently verify the completed Phase 3.3 loop-normalization slice and its
specification correction. This is verification, not design or implementation.

Read only:

- `prompts/review-spec-range-loop-binding.md`
- `reports/codex-spec-review-range-loop-binding.md`
- `reports/agy-spec-review-range-loop-binding.md`
- `spec/grammar.md` lines 61-81
- `spec/statements.md` Sections 4.1-4.4 and 13.1
- `spec/vm.md` Section 12.4
- `plans/multilingual-verb-language-cleanup-plan.md` Phase 3.3
- `verb/ir.go` loop statement definitions
- `parser/parser_stmt.go` loop parser
- `parser/parser_stmt_test.go` loop tests
- `parser/unparse.go` loop cases
- `bytecode/compiler.go` loop dispatch and lowering

Verify all of the following:

1. The grammar, statement specification, semantic IR, parser, unparser, and
   compiler agree that only collection loops may bind an index/key variable.
2. Collection and range loops are distinct sealed semantic statements and the
   nullable `ForStmt` path is absent.
3. Labels, value variables, collection index/key variables, bodies, break
   values, and continue behavior still reach the same compiler bookkeeping.
4. The tests and both external reviews cover the changed boundary.
5. No adapter, alias, compatibility type, or unrelated multilingual surface was
   introduced.

Begin with exactly `PASS` or `FAIL`, then give concise source-cited evidence.
Do not modify any file. Return the report as the final response; the invoking
CLI will save it to `reports/spec-verification-range-loop-binding.md`.
