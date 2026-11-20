# Fix nightly fuzz #643: CALL_VERB leaves ip on its operand after an error

## Goal
Fix the FuzzRestoreVMSnapshotDoesNotPanic panic from the 2026-11-07 nightly run
(issue #643, run 42363293768) and land it. Q asked to unfuck the nightly fuzz
and land the actual fixes.

## State
Fixed in vm/op_verb.go executeCallVerbStatic; the three failing inputs from the
run's artifact (58fc22730842cbf4, 80f20469ef533fe5, 9d84d726cc791dd2) are
committed under vm/testdata/fuzz/FuzzRestoreVMSnapshotDoesNotPanic and pass.
All three panicked at vm/vm.go:806 before the fix.

## Cause
executeCallVerbStatic validated the name constant before reading the argc
byte. When the constant is not a string, it returned the error with ip on the
argc operand. A frame without the 'd' flag (vm/vm.go:977; a restored snapshot
frame defaults to VerbDebug false) pushes the error and continues at ip, so
the operand byte was executed as an opcode and control ran past the final
RETURN: `IMM; CALL_VERB 49 0; RETURN` with constant 49 a list. Both
OP_CALL_VERB and OP_CALL_VERB_WIDE go through this function. Property
handlers in vm/op_property.go read all operands before any failing check.

## Decisions and why
Read argc first so ip is at the next instruction whenever the handler errors.
No general "advance ip on error" change: FORK's successor is past its body,
so a generic rule would need the verifier's successor logic in the VM.

## Surprises
The nightly repair workflow (claude-fix.yml) failed for both issues within
0.7 s with zero turns and zero cost: the Claude call was refused before any
work, consistent with the OAuth token's usage being exhausted (error text is
hidden by the action). The fuzz campaign and issue reporting worked.

## Next step
CI on the PR, then merge.
