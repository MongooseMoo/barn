# fix/parser-toast-expression-grammar

## Goal

Barn compiles and runs four grammar shapes exactly as ToastStunt does; 14 live
Mongoose verbs failed to compile without them. Success is defined by the
Toast-passing conformance file
`moo-conformance-tests-parser-syntax/src/moo_conformance/_tests/language/expression_codes_and_defaults.yaml`
(7 cases, unchanged).

1. except / catch-expression codes are an expression list (parser.y `codes: tANY | ne_arglist`).
2. A catch default is any expression, including assignment (parser.y `default: tARROW expr`).
3. A splice operand is any expression, including assignment (parser.y `'@' expr`).
4. `true` / `false` are built-in variable slots (sym_table.cc SLOT_TRUE/SLOT_FALSE), not literals.

## State

Implemented and committed locally on this branch; not pushed, no PR.
Verified: the managed conformance selection
`expression_codes_and_defaults or try_except or catch` passes on Barn (48 passed,
2 expected audit skips); the 14 listed Mongoose corpus verbs compile; the
parser/ir/compiler/bytecode/vm/db/format packages pass; FuzzParseFormatRoundTrip
ran 5 minutes clean.

## Decisions and why

- Toast semantics (execute.cc find_handler_activ and the FIN_RAISE loop in
  unwind_stack; code_gen.cc generate_codes): each arm's codes are evaluated as an
  ordinary argument list when the try is entered, in arm order, before the body;
  ANY matches everything; otherwise the raised code matches when
  `ismember(code, codes, 0)` (case-insensitive equality, bool/int equivalence).
- IR: `Codes []ir.Expr` (argument list, splices allowed) replaces `[]string`;
  `IsAny` stays. One IR representation.
- Bytecode: arms made only of error literals keep the existing OP_TRY_EXCEPT code
  bytes, so bytecode persisted in existing checkpoints executes unchanged and the
  common path is untouched. Any other arm evaluates its list into a compiler
  temporary before OP_TRY_EXCEPT, and its clause carries the two code bytes
  `{ExceptCodesInLocal (0xFF), slot}`. No new opcode, no operand-layout change,
  no stack-height change (bytecode/verify.go is untouched; another agent owns it).
  CompactInternalLocals remaps that slot; DynamicTicks does not charge the clause
  again because the list-building instructions already charged it.
- Persistence: nothing changes. Handler.Codes stays `[]ErrorCode` (the marker
  pair is written as two ints like any codes); the evaluated list lives in the
  internal local, which element 19 of the frame metadata already persists.
- Matching compares the raised code value (the error, or the raised value for
  raise() of a non-error, carried as E_NONE with the value first in the tuple), so
  `except (E_NONE)` no longer catches `raise("x")`, matching Toast.
  vm.listIndex is the one list-membership routine for both `in` and handlers.
- `true`/`false` are identifiers; BuiltinSlots.Set records their slots in
  TypeConstants (now carrying a Value instead of a TypeCode) so every frame starts
  them as Booleans (Toast eval_env.cc fill_in_rt_consts). ir.LiteralBool remains in
  the IR, but the parser no longer produces it.

## Surprises

- Barn already raised non-error codes: raise("x") carries Code E_NONE and the value
  in the exception tuple's first element.
- With config.Core, 52 corpus files still fail only on capability-gated builtins
  (network, sqlite); none fail on syntax.

## Next step

Lead: review, push and open the PR. The other agent's stack-height verifier needs
no knowledge of ExceptCodesInLocal (OP_TRY_EXCEPT's stack effect is unchanged),
but the verifier does not bounds-check code bytes, so handlerMatches checks the
slot against the frame's locals at match time.
