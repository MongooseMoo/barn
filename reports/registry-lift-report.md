# Registry Lift Report

## Workflow Used

Executed `prompts/registry-lift.md` directly.

## Changed

- Added `TaskContext.Registry interface{}` in `types/context.go`; `NewTaskContext()` still leaves `Store` and `Registry` nil and documents that callers must populate them.
- Lifted all store/registry-backed builtins to the canonical signature:
  - `ctx.Store.(*db.Store)` casts added to 50 builtins.
  - `ctx.Registry.(*Registry)` casts added to 5 builtins: `create`, `recycle`, `recreate`, `call_function`, `function_info`.
  - Missing store/registry context returns `E_INVARG`.
- Collapsed builtin registration into `builtins.NewRegistry()`:
  - Deleted `RegisterObjectBuiltins`, `RegisterPropertyBuiltins`, `RegisterVerbBuiltins`, `RegisterCryptoBuiltins`, and `RegisterSystemBuiltins`.
  - Removed all callers of those deleted methods.
- Simplified VM registry construction:
  - `vm.BuildVMRegistry()` no longer takes a store.
  - VM-aware `eval()` now reads the store from `ctx.Store`.
- Populated context dependencies at runtime entry points:
  - `cmd/barn/main.go`
  - `conformance/runner.go`
  - `server/scheduler.go`
  - `vm/eval.go`
  - `vm/eval_stmt.go`
  - `vm/vm.go`
  - selected VM parity helpers now use the complete `NewRegistry()` path.

## Deviations

- Also migrated `call_function` and `function_info` to `ctx.Registry`; they were existing registry-backed builtins outside the prompt's store-count list but violated the same single-signature goal.
- Added `Evaluator.GetStore()` so conformance context setup can explicitly populate `ctx.Store`.
- `task.NewTaskFull()` still cannot populate store/registry by itself because `task` does not own those dependencies; scheduler task-creation sites set them immediately after construction, and `runTask` backfills before execution.

## Preserved Registration-Time State

- No deleted category method installed `SetVerbCaller`, `SetRunGCFunc`, or waif registration callbacks.
- `RegisterSystemBuiltins` did contain behavior beyond simple registration:
  - `recreate()` post-processing copied inherited properties, linked parent children, and called `:initialize`; this was moved into `builtinRecreate`.
  - `set_task_perms` was re-registered with store access; this is now the canonical `builtinSetTaskPerms`.
- Existing scheduler wiring for `SetVerbCaller` and `SetRunGCFunc` remains in `server.NewScheduler`.

## Verification

- `go build ./...`: passed.
- `go test ./...`: failed. Failures included broad-suite issues outside this migration surface or in files not edited for this work:
  - build failures in `types/result_test.go` for `Break` call arity.
  - build failure in `conformance/conformance_test.go` for `server.NewServer` call arity.
  - runtime failures in `builtins`, `parser`, and `vm` packages.
- Server build for conformance: `go build -o barn.exe ./cmd/barn/` passed.
- Managed conformance:
  - Command: `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"`
  - Result: `2 failed, 3786 passed, 103 skipped in 68.88s`.
  - Prior baseline found in `reports/phase0-baseline-report.md`: `3 failed, 2570 passed, 183 skipped`.

## Notes

- Static scans after the lift found no remaining `Register{Object,Property,Verb,Crypto,System}Builtins` references.
- Static scans found no remaining `builtinX(ctx, args, store...)` or `builtinX(ctx, args, store, registry)` signatures.
