# AST builtins: parse_ast, verb_ast, validate_ast, unparse_ast

## Goal

Add the Mongoose ToastStunt fork's four syntax-tree builtins to Barn with
output identical to the fork (node shapes, key order through `toliteral`,
errors, messages, permissions, `unparse_ast` text). Nothing else.
Reference: fork `src/ast_map.cc`, `src/verbs.cc` (`bf_verb_ast`),
`docs/Features/ast.md`, `test/tests/test_ast.rb` at `mongoosemoo/mongoose`.

## State

- Tests: `tests/mongoose-conformance/builtins/{parse_ast,verb_ast,validate_ast,unparse_ast}.yaml`,
  58 cases. All pass on the fork Toast built at 0ff6fd7 (managed candidate lane,
  manifest `profiles/toast/mongoose-0ff6fd7-wsl-testdb.json` from the
  `managed-toast-live-commit` branch, used as both oracle and target).
  Expected values were copied from that Toast's output.
- Barn implementation: not started at this point.

## Decisions and why

- Share the existing parser and formatter: Barn's `verb_code` reparses stored
  source and prints it with `parser.FormatMOO`; the builtins convert between
  `ir.Program` and the map tree on top of that path. No second parser or unparser.
- Compare trees through `toliteral()` in the tests so key order is pinned too.

## Surprises

- Barn's IR lowers `elseif` into `Else: [IfStmt]`, so it cannot tell `elseif`
  from `else` + nested `if`; the fork keeps them apart in both directions.
- Barn's parser has no waif `x.:name` spelling; the fork compiles it as `x.(":name")`.
- Barn's formatter spells floats with `FormatFloat('f', -1)`; Toast prints
  `1e+20`, `1.23456789012346e+17` (15 significant digits).
- Barn's formatter has no indent-off mode.

## Next step

Implement parse_ast, then verb_ast, validate_ast, unparse_ast, committing each.
