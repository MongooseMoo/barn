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
- parse_ast: done in `builtins/ast.go`; 29 of 30 parse_ast cases pass on Barn.
  `waif_property_spelling` fails: Barn's parser rejects `a.:w` outright (see Surprises);
  making it parse is a language change outside this task.
- verb_ast: done; 5 of 5 cases pass on Barn. It parses the stored source like
  verb_code does. Barn's set_verb_code stores the formatter's output, which
  turns `else` + nested `if` into `elseif`, so for a verb programmed under Barn
  verb_ast reports elseif arms where Toast reports the nested if.
- Shared pieces added (no behaviour change): `compiler.NativeVariableNames`,
  `parser.{Binary,Unary}Operator{Spelling,FromSpelling}`, `parser.IsIdentifier`,
  `types.TypeCode.ToastName` (moved from vm), `readableDescribedVerb` (verb_code's checks).

## Decisions and why

- Share the existing parser and formatter: Barn's `verb_code` reparses stored
  source and prints it with `parser.FormatMOO`; the builtins convert between
  `ir.Program` and the map tree on top of that path. No second parser or unparser.
- Compare trees through `toliteral()` in the tests so key order is pinned too.
- `elseif` versus `else` + nested `if`: told apart from the source text at the
  nested IfStmt's position (the parser records the `elseif` token position), so
  no IR change was needed for code-to-tree.
- Variable spellings come from `compiler.NativeVariableNames`, the existing
  reproduction of Toast's name table (yacc reduction order, builtin spellings).
- Errors: parse_ast compiles with the same compiler as set_verb_code and raises
  its diagnostics; it does not apply set_verb_code's Barn-only missing-semicolon
  retry (Toast has none).

## Surprises

- Barn's IR lowers `elseif` into `Else: [IfStmt]`, so it cannot tell `elseif`
  from `else` + nested `if`; the fork keeps them apart in both directions.
- Barn's parser has no waif `x.:name` spelling; the fork compiles it as `x.(":name")`.
- Barn's formatter spells floats with `FormatFloat('f', -1)`; Toast prints
  `1e+20`, `1.23456789012346e+17` (15 significant digits).
- Barn's formatter has no indent-off mode.

## Next step

validate_ast, then unparse_ast, committing each.
