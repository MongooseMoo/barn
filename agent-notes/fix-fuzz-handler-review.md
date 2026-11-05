# Saved snapshot fuzz input

## Goal
Deliver the existing #608 verifier fix for the 2026-11-05 snapshot fuzz failure, after checking the saved input and compiler contracts.

## State
Q authorized assessment and delivery with "Make it so!". Reviewed the open PR diff against origin/master and the real source. No production changes were added to the original proposal. The committed f10bcff0e9a20440 input matches artifact 13408519086 byte for byte; both SHA-256 digests are f10bcff0e9a204407a1f36049548711b350c692f4d95e7ef5996d4b3d087160a. The artifact was read in memory, without executing its bytecode.

The original PR head's Build & unit tests, MOO conformance suite, and Serializable histories and negative control checks reported pass. Its review-thread query returned no threads and no submitted reviews. Adding this task record requires checks on the final PR head before merge. No local Go tests were run in this review.

## Decisions and why
The exact input decodes to TRY_FINALLY 0, an immediate, and RETURN. vm/control.go executeTryFinally installs the absolute target; vm/stack.go Return removes the handler and jumps to that target. Re-entering its TRY installs it again. bytecode/opcodes.go assigns TRY_FINALLY an unchecked tick, and vm/vm.go chargeTicks does not test tick exhaustion for unchecked charges.

compiler/lower_control.go compileTry emits TRY_FINALLY_WIDE before its body, then patches its target to the later finalizer. It also patches each TRY_EXCEPT_LOCAL_WIDE handler to a later emitted handler body. compiler/lower_expressions.go compileCatch emits the protected expression before patching its handler to a later address. The shared verifier restriction therefore agrees with both compiler producers. Narrow and wide handler target decoding uses the existing absolute-target implementation in bytecode/verify.go. END_FINALLY keeps its existing boundary check because its operand is the matching finalizer identity, including at the end of that finalizer.

The existing compiler-output verifier test includes try/except, try/finally and catch expressions. The new negative cases cover self-targeted narrow finally/except handlers and a backwards wide finally handler; the exact fuzz input supplies the restored-VM regression.

## Surprises and limits
The companion FuzzSourceExecution campaign terminated after the automated repair agent issued a broad process-name kill. PR #609 guards that workflow separately; merge the guard before the snapshot fix.

PR #608 is not a general execution-boundedness proof for forged bytecode. The reported LOOP 3; RETURN_NONE example remains outside this handler restriction: branchTarget maps the LOOP back to itself, successors propagates an unchanged stack height to that same address, and OP_LOOP carries no default tick. vm/vm.go's loop dispatch jumps without a checkpoint. The restore fuzz target sets a numeric seconds limit through NewState but never calls SetExecutionDeadline; task/task.go SecondsLeft otherwise subtracts SecondsUsed, which this standalone fixture does not advance. These observations are source analysis, not a fresh runtime reproduction. A deadline alone would not stop a zero-tick cycle because the clock checkpoint depends on ticks. No claim that the handler restriction resolves that broader limitation is warranted; preserve the existing PR body's explicit limitation. Changing general verifier cycle admission or runtime deadline enforcement is a separate design from this scoped handler repair.

## Next step
Push this task record to #608, wait for final-head checks, and normally merge after the process guard. Verify remote merge topology and issue #606 closure, then remove only the task-owned worktree and branch.
