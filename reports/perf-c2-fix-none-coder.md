# C2 Fix — Result.Val zero-value-vs-None blocker (coder)

Branch: `perf/c2-value-unbox`. Working dir: C:\Users\Q\code\barn.

## Status: DONE — conformance back to 3988/0, allocs still ~zero.

## RESULT SUMMARY
- RED tests written first, confirmed failing, now pass.
- Unit: `go test ./types ./vm ./builtins ./scheduler` all ok.
- Race: `go test ./vm -race` ok.
- Conformance: **3988 passed / 131 skipped / 0 failed** (158.55s). Exact baseline. No test weakened/skipped.
- benchstat sanity: int_arith_1M = **11 allocs/op** (8816 B/op), NOT back to ~2.0M. De-box preserved.
- Commit: see bottom.

## EXACT FIXES (file:line, before -> after)

### 1. types/result.go — Err() (primary, covers 28 builtin files)
before: `return Result{Flow: FlowException, Error: e}`  (Val = zero Value{} = int 0 post-de-box)
after:  `return Result{Flow: FlowException, Error: e, Val: None}`
Rationale: vm.HandleError (vm/vm.go:720) gates on Val.IsNone() to build the standard
{code,message,value,traceback} list. Zero Value{} is int 0 (not None) post-de-box, so the
branch was skipped and builtin errors surfaced as bare int 0.

### 2. Sibling FlowException Result literals omitting Val (same hazard)
- scheduler/call_verb.go:48 (E_VERBNF, verb not found): added `Val: types.None`
- scheduler/call_verb.go:60 (E_VERBNF, compile error):   added `Val: types.None`
- scheduler/eval.go:150 (E_INVARG):                       added `Val: types.None`

### 3. ActivationFrame literals omitting ThisValue (the 2nd conformance failure)
task/task.go ToList(): `thisVal = NewObj(a.This); if !a.ThisValue.IsNone() { thisVal = a.ThisValue }`.
Eval frame left ThisValue at zero Value{} (int 0), so `this` rendered as 0 instead of #-1.
Fixed the 3 literals that omitted ThisValue to `types.None`:
- vm/registry.go:135 (eval'd-code activation) — the failing test
- scheduler/task_load.go:88 (loaded/suspended top frame)
- scheduler/task_runtime.go:140 (runtime top frame)
The other 4 literals (op_verb.go:245/456, call_verb.go:93, task_factory.go:271) already set
ThisValue; traceback.go:88 already sets types.None. ToList was deliberately NOT changed —
a primitive `this` can legitimately be integer 0, so int 0 cannot be treated as "unset";
the construction sites are the correct fix locus.

## ORIGINAL STATUS NOTES (kept for trail)

## Root cause (confirmed from analyst report + source read)
Post-de-box, zero `types.Value{}` is integer 0 (tag TYPE_INT), not nil/None.
- `types/result.go:60` `Err(e)` returns `Result{Flow: FlowException, Error: e}` leaving `.Val` at zero `Value{}` = int 0.
- `vm/op_misc.go:57` packs `result.Val` into `VMException{Value: result.Val}`.
- `vm/vm.go:720` `HandleError` gates on `exceptionValue.IsNone()` to build `{code,msg,0,traceback}`. int 0 is neither None nor LIST -> both branches skipped -> builtin error surfaces as bare int 0.

`None = Value{tag: tagNone}`, `IsNone()` checks `tag == tagNone` (types/value.go:55,63).

## Fix plan
1. PRIMARY: `types/result.go` `Err(e)` -> set `Val: None` explicitly.
2. AUDIT of literal `Result{Flow: FlowException}` sites (grep results):
   - builtins/tasks.go:423 — sets Val (exceptionList). FINE.
   - vm/vm.go:317 — sets Val (NewStr). FINE.
   - vm/vm.go:338 — sets Val (NewStr). FINE.
   - scheduler/call_verb.go:48 — NO Val (E_VERBNF). HAZARD candidate.
   - scheduler/call_verb.go:60 — NO Val (E_VERBNF). HAZARD candidate.
   - scheduler/eval.go:150 — NO Val (E_INVARG). HAZARD candidate.
   Checking whether the scheduler sites feed HandleError's IsNone gate before deciding to patch.

## TDD: RED CONFIRMED FAILING
- types/result_none_test.go::TestErrResultValIsNone -> "Err(E_TYPE).Val.IsNone() = false (Val=0, type=INT)"
- vm/builtin_error_value_test.go::TestBuiltinRaisedErrorValueIsErrorList -> "caught value = 0 (type INT), want LIST"

## Fix being applied
- types/result.go Err(e): add Val: None.
- scheduler/call_verb.go:48,60 + scheduler/eval.go:150: add Val: types.None (same hazard, FlowException w/ no Val).

## Applied fix -> RED tests green; unit+race green.
Conformance: 3987 passed / 1 failed / 131 skipped (was 1383 failed). HUGE improvement.

REMAINING 1 FAILURE (same de-box hazard class, traceback):
  error_traceback::exception_traceback_direct_eval_frame_fields
  expected ['#-1', ''], got [0, '']  -> object #-1 field rendered as int 0.
  Investigating vm/traceback.go buildTraceback — a frame object field defaulting
  to zero Value{} (int 0) instead of NewObj(-1)/#-1.

ROOT CAUSE #2 (same hazard): task/task.go ToList() does
  thisVal := NewObj(a.This); if !a.ThisValue.IsNone() { thisVal = a.ThisValue }
The eval-frame ActivationFrame literal (vm/registry.go:135) omitted ThisValue, so
post-de-box it was int 0 (not None) -> ToList used int 0 as `this` instead of #-1.
NOTE: ToList must NOT be hardened to treat int 0 as unset — a primitive `this` can
legitimately be an integer. The construction sites are the correct fix locus.
Audited all 8 ActivationFrame literals: 4 set ThisValue, traceback.go:88 sets None,
3 omitted it -> fixed to types.None:
  - vm/registry.go:135 (eval frame) [the conformance failure]
  - scheduler/task_load.go:88 (loaded/suspended task top frame)
  - scheduler/task_runtime.go:140 (runtime task top frame)
Traceback fix proof = conformance (eval-frame test); no runBytecodeProgram eval path.
