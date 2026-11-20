# test/verified-bytecode-fuzz

## Goal
Make `bytecode.VerifyProgram` reject stack-unsafe bytecode before restored
suspended tasks reach the VM's unchecked loop. Reproducer:
`vm/testdata/fuzz/FuzzRestoreVMSnapshotDoesNotPanic/1d7519eb41cfc9de`
(`OP_RETURN` on an empty stack passed verification, then panicked in
`vm/stack.go` Pop).

## State
- `bytecode/verify.go`: `StackHeights(program) ([]int, error)` verifies and
  returns the entry height of each reachable instruction (-1 elsewhere);
  `VerifyProgram` is a wrapper. Per-opcode `stackEffect` and `successors`.
- `vm/persistence.go` `validateVMSnapshot`: each frame's saved stack must match
  the analysed height at its saved IP, and each handler's StackDepth must match
  the height at its handler IP.
- Passing: bytecode unit tests; FuzzCompileVerifiable seeds and the 11,006-verb
  local corpus (with the full builtin registry applied locally, not committed);
  10-minute campaigns of FuzzCompileVerifiable and
  FuzzRestoreVMSnapshotDoesNotPanic; targeted vm, db/format, engine tests.

## Decisions and why
- Stack effects come from `vm/*.go`, not the opcode comments.
- A non-top frame's caller height at its saved IP is `len(Stack)+1` when the
  callee's return pushes onto it (`vm/stack.go` Return: eval frames always,
  frames with a move/recycle continuation, otherwise unless DiscardReturn);
  `len(Stack)` otherwise. The top frame resumes with exactly its stack (builtin
  suspend pushed its 0 before yielding, `vm/op_misc.go`; fork popped its delay).
- Handler entry height equals the height at TRY_EXCEPT/TRY_FINALLY: unwinding
  sets SP to the depth recorded there (`vm/vm.go` handleErrorWithReason,
  `vm/stack.go` Return).
- FORK's only successor is the instruction after its body; the body runs as
  its own extracted program and is verified separately when restored.
- Control reaching the end of code (a fork body ending the program) is
  rejected.

## Surprises
- Non-debug error recovery (`vm/vm.go` executeLoop pushes the error value after
  a failed instruction) leaves a height that depends on how many operands the
  instruction had popped before failing. The verifier models the ordinary
  path only. OP_GET_PROP_KEYED (pops 1, pushes 2) pops the object inside
  executeGetPropAccess before failing, so a non-debug frame continues with one
  value where two are expected: compiled `return {99, #0.p["k"], 77};` in a
  non-debug frame with #0 invalid panics "stack underflow" (checked with a
  throwaway vm test, not committed). Compiler output, so not a verifier gap;
  unfixed here.

## Next step
Lead review and publication.
