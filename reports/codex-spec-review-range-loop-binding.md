APPROVE

No blocking findings.

- Grammar limits the optional index/key binding to collection loops, matching the existing collection and range forms ([grammar.md](/C:/Users/Q/code/barn/spec/grammar.md:64), [statements.md](/C:/Users/Q/code/barn/spec/statements.md:152), [statements.md](/C:/Users/Q/code/barn/spec/statements.md:226)).
- Distinct semantic variants encode the required fields without nullable discrimination or compatibility types ([ir.go](/C:/Users/Q/code/barn/verb/ir.go:342), [vm.md](/C:/Users/Q/code/barn/spec/vm.md:614)).
- The parser directly constructs those variants and rejects a second range binding while preserving labels and bodies ([parser_stmt.go](/C:/Users/Q/code/barn/parser/parser_stmt.go:259), [parser_stmt.go](/C:/Users/Q/code/barn/parser/parser_stmt.go:323)).
- Compiler dispatch, loop bookkeeping, and unparsing preserve collection/range, label, break, and continue behavior ([compiler.go](/C:/Users/Q/code/barn/bytecode/compiler.go:493), [compiler.go](/C:/Users/Q/code/barn/bytecode/compiler.go:2038), [compiler.go](/C:/Users/Q/code/barn/bytecode/compiler.go:2130), [unparse.go](/C:/Users/Q/code/barn/parser/unparse.go:86)).
- Tests cover the critical boundary: distinct structural lowering, preservation of collection bindings, range-label preservation, rejection of range index bindings, and labeled collection round-tripping ([parser_stmt_test.go](/C:/Users/Q/code/barn/parser/parser_stmt_test.go:94), [parser_stmt_test.go](/C:/Users/Q/code/barn/parser/parser_stmt_test.go:120), [review_test.go](/C:/Users/Q/code/barn/parser/review_test.go:40)).