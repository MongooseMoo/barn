# Issue 423: uncaught-error handler ordering

## Goal
Diagnose issue 423 in this isolated worktree. Establish Toast behavior, capture a deterministic Barn failure, publish red CI, then make the smallest fix and establish green CI. Complete 20 managed conformance runs under load. Evaluate an explicit Elle negative control for the demonstrated defect.

## State
PR #482 is draft. Red commit 700b54ff, Elle CI run 39566782997: parent dispatched=1 while handler held, parent observed {194,0}, test failed with exit 1. Focused authored healthy test passed locally: parent dispatched=0; handler and parent both observed {194,1}. This reproduces the historical defect by a checker-tag-only fault, not on healthy master.
Green changes pending: remove temporary red workflow step, require healthy pass and specific fault rejection in scripts/check_elle.py, retain raw logs/results, and run the unchanged conformance test in 20 managed sessions under two CPU load workers in CI. Managed WSL Toast completed: 2 passed, 13170 deselected in 325.42s; session 4685. Issue remains open.

## Decisions and why
Q approved proceeding with stronger coverage after learning the fix already exists. Smallest coverage change: about 100 lines for a real VM/scheduler regression, a compile-away fault at settledLocked (about 4 lines), and about 30 lines in the existing Elle driver to require both healthy success and a specific negative-control failure. Keep serializable checking unchanged: task ordering is a separate assertion alongside Elle.
Smallest intended change: one deterministic engine regression and the production ordering correction it demonstrates; size unknown until reproduction. Existing Elle checks request serializable histories, which may not detect scheduler ordering violations. Do not strengthen the whole model without evidence.

## Surprises
The remembered reports/toast-oracle-wsl.md does not exist. Current Makefile documents the managed conformance-toast target and includes capability admission automatically for focused selection.

Current master already contains cd84cb267811b43cf3c3ceb61a2c6cf18f56c891, whose message explicitly identifies this exact [194,0] failure. Verified implementation: task/task.go settledLocked retains terminal tasks while executionActive; task/manager.go Unfinished uses that index. Existing task/manager_test.go TestUnfinishedKeepsFinishedTaskUntilItsSliceEnds tests both killed and completed states. No remaining production defect demonstrated. Toast execute.cc:3171 synchronously invokes do_server_verb_task for the uncaught error handler.

## Next step
Publish green changes and read exact-head PR CI results. Preserve failures and stop on an unexpected build/test/harness failure. No production behavior fix is intended. Update PR description and task record with final evidence; do not merge without authorization.

Managed oracle command: wsl -d Debian --exec bash -lc 'set -euo pipefail; cd /mnt/c/Users/Q/code/barn-issue-423; make conformance-toast K=tick_exhaustion_extended_opcode_does_not_test_the_budget'. Changes now include the new engine test, task/task.go checker-tag fault, and temporary Elle workflow red step. A developer checkpoint also exists at C:/Users/Q/code/barn/notes-issue-423-checkpoint.md.
