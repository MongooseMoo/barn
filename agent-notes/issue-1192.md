# Issue 1192 — checkpointing self-add assignments

## Goal

Restore the caller operand in a suspended self-add and verify both checkpoint directions.

## State

On the original Toast checkpoint, the unmodified compiler failed with `native frame 4: native operand 0: native continuation lacks 0 value from *ir.LiteralExpr`. With the change, `barn offline-checkpoint` reported `read: 76 queued, 31 suspended or interrupted tasks, 0 connections`. The output file contains 76 queued and 31 suspended tasks. The private checkpoint was used locally.

## Decisions and why

Compile the left identifier through the same path as ordinary addition while retaining the self-add opcode and the original variable slot. This records the read at the point it happened. Reconstructing a literal or reading the current local would give the wrong answer if the variable changed before suspension.

The managed cross-engine test places two tasks in one checkpoint. One changes a string local while evaluating an argument to suspend. The other accumulates integers and suspends in a child verb during the second loop iteration inside a try. The final result and call counts check both the saved value and exactly-once execution.

## Surprises

The literal in the error belongs to an earlier assignment. The list self-append shortcut already records the prefix it holds, and the indexed-assignment shortcut explicitly marks a variable base when it must survive evaluation of another operand. Issue 942 concerned a temporary for exception codes and already has a retained fuzz seed.

## Next step

Run the new case in the PR's cross-engine checkpoint job.
