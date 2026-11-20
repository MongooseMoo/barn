# Verifier accepts empty fork bodies

## Goal
The first nightly fuzz run (workflow fuzz.yml, run dispatched 2026-11-02)
failed FuzzCompileVerifiable: the compiler's bytecode for
`fork A00000(0)endfork` is rejected by bytecode.VerifyProgram with "fork body
[8,8) is not on instruction boundaries".

## State
bytecode/verify.go verifyFork rejects only a negative body length. Minimized
input committed as compiler/testdata/fuzz/FuzzCompileVerifiable/c8c000a2685f3ac5:
fails without the fix, passes with it.

## Evidence
- The compiler always emits OP_FORK_LOCAL_WIDE and patches the body length
  to the compiled body size, 0 for an empty body
  (compiler/lower_control.go compileFork).
- The runtime accepts length 0: Program.forkBodyEnd rejects only negative
  lengths (bytecode/program.go:184) and ExtractForkBody appends
  OP_RETURN_NONE, so the forked task runs a lone return.
- The `bodyLength <= 0` rule dates from "Validate restored VM bytecode"
  (#260), so every empty fork was rejected, including in a suspended task's
  checkpoint, which would then fail to load.

## Next step
PR CI.
