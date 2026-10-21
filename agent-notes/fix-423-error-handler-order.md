# Issue 423: uncaught-error handler ordering

## Goal
Diagnose issue 423 in this isolated worktree. Establish Toast behavior, capture a deterministic Barn failure, publish red CI, then make the smallest fix and establish green CI. Complete 20 managed conformance runs under load. Evaluate an explicit Elle negative control for the demonstrated defect.

## State
Coverage implementation added: real fork/handler/parent test, checker-tag-only finished-task-index fault, and temporary red demonstration step in Elle CI. Focused authored test passed locally: parent dispatched=0 while handler held; handler and parent both observed {194,1}. Next publish the deliberate fault in red CI, then replace the demonstration step with permanent negative-control checking in scripts/check_elle.py.
User authorized implementation with "Make it so!". Branch starts at 30700d51. No implementation changes or Barn reproduction yet. Managed WSL Toast completed: 2 passed, 13170 deselected in 325.42s; session 4685, command below. Issue remains open.

## Decisions and why
Q approved proceeding with stronger coverage after learning the fix already exists. Smallest coverage change: about 100 lines for a real VM/scheduler regression, a compile-away fault at settledLocked (about 4 lines), and about 30 lines in the existing Elle driver to require both healthy success and a specific negative-control failure. Keep serializable checking unchanged: task ordering is a separate assertion alongside Elle.
Smallest intended change: one deterministic engine regression and the production ordering correction it demonstrates; size unknown until reproduction. Existing Elle checks request serializable histories, which may not detect scheduler ordering violations. Do not strengthen the whole model without evidence.

## Surprises
The remembered reports/toast-oracle-wsl.md does not exist. Current Makefile documents the managed conformance-toast target and includes capability admission automatically for focused selection.

Current master already contains cd84cb267811b43cf3c3ceb61a2c6cf18f56c891, whose message explicitly identifies this exact [194,0] failure. Verified implementation: task/task.go settledLocked retains terminal tasks while executionActive; task/manager.go Unfinished uses that index. Existing task/manager_test.go TestUnfinishedKeepsFinishedTaskUntilItsSliceEnds tests both killed and completed states. No remaining production defect demonstrated. Toast execute.cc:3171 synchronously invokes do_server_verb_task for the uncaught error handler.

## Next step
Implement the deterministic ordering test and the existing-tag fault. Publish a red PR CI run with the fault enabled, then teach the existing Elle driver to require that precise rejection and publish green. No production behavior fix is intended.

Managed oracle command: wsl -d Debian --exec bash -lc 'set -euo pipefail; cd /mnt/c/Users/Q/code/barn-issue-423; make conformance-toast K=tick_exhaustion_extended_opcode_does_not_test_the_budget'. Changes now include the new engine test, task/task.go checker-tag fault, and temporary Elle workflow red step. A developer checkpoint also exists at C:/Users/Q/code/barn/notes-issue-423-checkpoint.md.
