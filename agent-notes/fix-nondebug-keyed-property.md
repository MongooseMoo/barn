# Non-debug obj.prop[key] stack shape

## Goal
A failing property read inside `obj.prop[key]` in a verb without the debug
flag must yield the error as a value, as Toast does, without corrupting the
operand stack. Found by agent `stack-verifier` while building the bytecode
stack verifier (fuzzing expansion, barn master checkout
agent-notes/fuzzing-expansion.md); confirmed on master.

## State
vm/op_property.go executeGetPropKeyed pushes the object back when
executeGetPropAccess fails (it pops the object first). Conformance test
language/nondebug_keyed_property_error.yaml (moo-conformance-tests branch
test/nondebug-keyed-property): stock WSL Toast 3 passed with admission;
Barn master both cases fail; this branch passes them plus all "prop" tests
(579 passed, 21 skipped).

## Evidence
- Master, non-debug verb `return {99, #0.p["k"], 77};`: task killed,
  "stack underflow" (vm/op_list.go:66). Debug verb: E_PROPNF, correct.
- Nested in a debug caller `return {"a", "b", this:keyed(), "c"};`, master
  returns {"a", "b", "E_TYPE", 77, {"a", "b", "E_TYPE", 77}, "c"}: the inner
  frame popped the caller's values. Toast: {"a", "b", {99, E_TYPE, 77}, "c"}.

## Decisions and why
- Restore the [obj, value] shape on the error path rather than change
  OP_INDEX_KEYED: the non-debug loop pushes the error as the value, so the
  next instructions see the normal shape and index the error (E_TYPE), which
  is Toast's result.

## Next step
PR CI. Merge before the conformance data PR, since Barn CI runs its main.
