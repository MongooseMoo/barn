# Phase 3 Progress Notes

## Plan: 9 tasks to eliminate tree-walker delegation from bytecode VM

## Pre-existing failures (NOT from this work)
- `TestTryExcept/catch_with_error_variable_-_returns_error_VALUE`
- `TestEvalErrors/toint("abc")`

## Task Status
- [x] Task 1: Map Indexing + Dollar Marker (B1, B2, B3, B4) — commit 9453b5b
- [x] Task 2: Splice in Call Arguments (A1, A2) — commit 51ade45
- [x] Task 3: Tick Counting + Nested Index Assignment (E2, A3) — commit 42c2d66
- [x] Task 4: Native Verb Calls — Infrastructure — commit 427828d
- [x] Task 5: Native Verb Calls — Rewrite executeCallVerb() — commit aa95f52
- [x] Task 6: Native Verb Calls — Edge Cases — commit 1c80bcc and Verification
- [x] Task 7: Waif Support + Property Permissions (B5, B6) — commit f5049cc
- [x] Task 8: Line Number Tracking (E1) — commit 5f157e6
- [x] Task 9: Dead Opcode Cleanup + Size Limits (C, B8) — commit 3635fb1

## Completed Tasks

### Task 1: Map Indexing + Dollar Marker (B1, B2, B3, B4) — DONE
- Commit: `9453b5b`
- 29 new parity tests
- B2: Map indexing — moved int assertion inside list/string cases, added MapValue case
- B3: Map index assignment — already worked via setAtIndex once B2 unblocked
- B4: Map range assignment — added MapValue case with position-based pair splicing
- B1: Dollar marker — added indexContextVar temp, DUP+LENGTH pre-computation
- 2 pre-existing failures only, no regressions

### Task 2: Splice in Call Arguments (A1, A2) — DONE
- Commit: `51ade45`
- 7 new parity tests
- A1: Builtin args — incremental list build with OP_LIST_APPEND/EXTEND, argc=0xFF sentinel
- A2: Verb args — same pattern for compileVerbCall
- No regressions

### Task 3: Tick Counting + Nested Index Assignment (E2, A3) — DONE
- Commit: `42c2d66`
- 10 new parity tests
- E2: Added OP_LOOP to CountsTick(), removed dead opcodes from CountsTick
- A3: Nested index assignment via temp variable desugaring, arbitrary depth
- No regressions

### Task 4: Native Verb Calls — Infrastructure — DONE
- Commit: `427828d`
- 10 new tests
- 4A: CompileStatements() method on Compiler, VarNames populated by declareVariable()
- 4B: BytecodeCache any field on db.Verb struct
- 4C: CompileVerbBytecode() lazy compilation helper
- 4D: Cross-frame exception unwinding in HandleError() (loops through frames)
- No regressions

### Task 5: Native Verb Calls — Rewrite executeCallVerb() — DONE
- Commit: `aa95f52`
- 9 new test cases across 5 test functions
- Rewrote executeCallVerb() with native frame push
- setLocalByName() helper populates this/verb/caller/args/player in new frame
- Tree-walker fallback for verbs the bytecode compiler can't handle yet
- StackFrame extended with IsVerbCall, SavedThisObj, SavedVerb for context restore
- Return() and HandleError() restore context + pop activation frames on verb frame exit
- Tests cover: builtin vars, nested calls, recursion, cross-frame errors, cross-frame catch
- No regressions

### Task 6: Native Verb Calls — Edge Cases — DONE
- Commit: `1c80bcc`
- 16 new test cases, 10 new test verbs
- Zero bugs found — Task 5 implementation handles all edge cases correctly
- Covers: finally blocks, deep unwinding (A→B→C), unhandled exceptions, args access, return types, E_VERBNF, E_PERM, player propagation, nested finally
- No regressions

### Task 7: Waif Support + Property Permissions (B5, B6) — DONE
- Commit: `f5049cc`
- 13 new test cases
- B5: Waif property read (vmGetWaifProp) + write (vmSetWaifProp) with special properties, fallback to class
- B6: checkPropertyReadPerm/checkPropertyWritePerm — wizard bypass, owner check, flag check
- No regressions

### Task 8: Line Number Tracking (E1) — DONE
- Commit: `5f157e6`
- 3 new tests
- trackLine() method records LineEntry when source line changes during compilation
- CurrentLine() and annotateError() in vm.go for line numbers in error messages
- No regressions

### Task 9: Dead Opcode Cleanup + Size Limits (C, B8) — DONE
- Commit: `3635fb1`
- 9 dead opcodes marked with DEAD_ prefix in OpCodeNames and comments
- Size limit checks added to executeAdd, executeListAppend, executeListExtend, executeRangeSet
- Uses builtins.CheckStringLimit/CheckListLimit/CheckMapLimit → E_QUOTA
- No regressions

## PHASE 3 COMPLETE — ALL 9 TASKS DONE
