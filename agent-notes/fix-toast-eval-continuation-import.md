# fix/toast-eval-continuation-import

## Goal

Issue #885. Barn would not boot a private ChatMUD Toast checkpoint: 8 saved tasks paused inside code run by eval() failed with "native builtin continuation requires conversion". Q asked for the issue to be filed and fixed, using the ChatMUD database only as a private oracle.

## State

Done:
- Import: the eval frame's own continuation (builtin `eval`, pc 2, no data) is recognised when the caller is paused at an `eval(...)` call. It is dropped, because Barn already models it as `IsEvalFrame`.
- Export: an `IsEvalFrame` frame is written with builtin `eval`, pc 2, as Toast does.
- Added the public Toast-written fixture `engine/testdata/toast-suspended-eval.db`, its producer `tests/mongoose-conformance/server/suspended_eval_checkpoint.yaml`, and the regression test `TestToastSuspendedEvalImportReturnsEvalResult`.

## Decisions and why

- **Toast stores the mark on the callee.** `bf_eval` pushes the activation (`setup_activ_for_eval`) and then returns a call package. `OP_BI_FUNC_CALL` stores `bi_func_pc`/`bi_func_id` on `RUN_ACTIV`, which is now the eval activation. On return the unwinder calls `bf_eval(next=2)` -> `{1, value}`.
- **The mark is only dropped when the caller's call site is eval().** An eval mark anywhere else, at another pc, or with saved data still refuses, as before.
- **The fixture came from lisdude/toaststunt 20668a7 on Linux (OVH).** The pinned WSL oracle wasn't available there. bf_eval's continuation is unchanged LambdaMOO code.

## Surprises

- A Barn-only restart test passed before the fix, because Barn read back its own output. The mismatch only shows across engines. With Barn writing and Toast resuming (a wrapper that boots Barn first and Toast on restart), Toast's eval() returned a bare `73` instead of `{1, 73}`.

## Next step

Q reviews and merges. The pinned WSL Toast oracle and staticcheck/golangci-lint were not run locally; CI runs the linters.
