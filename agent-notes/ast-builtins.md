# AST builtins: parse_ast, verb_ast, validate_ast, unparse_ast

## Goal

Add the Mongoose ToastStunt fork's four syntax-tree builtins to Barn with
output identical to the fork (node shapes, key order through `toliteral`,
errors, messages, permissions, `unparse_ast` text). Widened by team-lead and Q
(2026-11-14): correct the one formatter (`parser/format.go`) to Toast's
unparser, so `verb_code` and the text `set_verb_code` stores change toward
Toast too. Reference: fork `src/ast_map.cc`, `src/unparse.cc`, `src/verbs.cc`,
`docs/Features/ast.md`, `test/tests/test_ast.rb` at `mongoosemoo/mongoose`.

## State

- Tests: `tests/mongoose-conformance/builtins/{parse_ast,verb_ast,validate_ast,unparse_ast,verb_code_layout}.yaml`.
  Expected values come from Toast: the AST files from the fork built at 0ff6fd7
  (manifest `profiles/toast/mongoose-0ff6fd7-wsl-testdb.json`, now on master),
  `verb_code_layout.yaml` from stock Toast (aecc51e), which gave identical text.
- All four builtins in `builtins/ast.go`; unparse_ast prints through
  `parser.FormatUncheckedWith` (no ir.Validate: trees may nest 1000 deep).
- Formatter commits, one per rule: indent argument; consequent/caught/assigned
  value unbracketed; splice operand unbracketed; `$name(...)`; `$name =`;
  integer postfix base (`5 .x`); map operand bracketed; `%.15g` floats via
  `internal/floattext` (shared with `types`); negated numbers not forced into
  brackets in fully-paren mode; else + nested if (`parser.ElseIfArms`).
- Left on purpose (evidence in the work-10 report): `(^)` before a dot (Toast's
  own `l[^.x]` does not recompile), unsigned wrapped int literals, `- -5` not
  folded (fuzz round-trip keeps the IR). `x.:name`: no parser change
  (team-lead's decision). Their test cases carry a declared `skip:` naming
  the difference.
- The AST tests declare `requires: features: [ast]`; the fork Toast manifest
  and Barn's mongoose profiles carry `feature.ast: true`, and Barn's manifest
  computes it from the registry (`profile/manifest.go`), as `runtime.arch_bits`
  is computed for the 64-bit feature.
- Branch merged with origin/master; a pull request against master is open.

## Decisions and why

- One formatter, Toast's rules (team-lead and Q). FormatOptions carries only
  call arguments (fully-paren, indent) and data about the program (which
  IfStmts are elseif arms); nil arms means the source is unknown.
- `elseif` versus `else` + nested `if`: read from the source at each lowered
  IfStmt's position (the parser keeps the `elseif` keyword's position); the IR
  stays normalized, as `parser/AGENTS.md` requires.
- Variable spellings come from `compiler.NativeVariableNames`.
- parse_ast compiles with the set_verb_code compiler and raises its
  diagnostics, without set_verb_code's Barn-only missing-semicolon retry.
- Float spelling lives in `internal/floattext` because `types` imports only the
  standard library and `parser` may not import `types`.

## Surprises

- Toast's own `l[^.x]` (from `l[(^).x]`) is a syntax error on Toast.
- The managed-toast worktree that held the fork manifest was removed after its
  PR merged; the manifest is now `profiles/toast/mongoose-0ff6fd7-wsl-testdb.json`.

## Next step

Watch the pull request's CI and fix anything red on this branch.
