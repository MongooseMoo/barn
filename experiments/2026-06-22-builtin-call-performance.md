# Builtin Call Performance

Date: 2026-06-22

Status: measured on experiment branch; source change promoted.

Experiment branch: `experiment/verb-call-performance`

Evidence commits:
- `e0cdce8` source/config delta
- record commit: this commit

Hypothesis: The benchmark row named `verb_call_200k` is dominated by fixed-argc builtin call overhead because the workload is `abs(-i)`, not object verb dispatch. Avoiding per-call argument-slice allocation and unnecessary builtin-call bookkeeping should improve that row without changing MOO semantics.

Single variable: Optimize fixed-argc `OP_CALL_BUILTIN` dispatch by borrowing the argument window from the VM stack, capturing known builtin signatures at registration time, and syncing task line numbers only for builtins that expose them.

Baseline:
- Command: `wsl --cd /mnt/c/Users/Q/code/moo-conformance-tests --exec bash -lc 'bash bench/run_bench.sh'`
- Result: `verb_call_200k`: Toast `8.69ms`, Barn `52.94ms`, `barn/toast=6.1x`.
- Local command: `go test ./vm -run '^$' -bench 'BenchmarkVM/(builtin_abs_200k|int_arith_1M|nested_1k)' -benchmem -count=3`
- Local result: `BenchmarkVM/builtin_abs_200k`: `45.6-46.7ms/op`, `9602858-9602862 B/op`, `999484 allocs/op`.
- Telemetry: `go test ./vm -run '^$' -bench 'BenchmarkVM/builtin_abs_200k$' -benchmem -cpuprofile .tmp\profiles\verb-call-baseline.cpu -memprofile .tmp\profiles\verb-call-baseline.mem -count=1`; allocation profile showed `PopN`, `builtinAbs`, `executeNeg`, and accumulator add as dominant allocation sites. CPU profile showed `CallByID`, dispatch, signature validation, and line syncing in the hot path.

Experiment result:
- Command: `wsl --cd /mnt/c/Users/Q/code/moo-conformance-tests --exec bash -lc 'bash bench/run_bench.sh'`
- Result: `verb_call_200k`: Toast `8.90ms`, Barn `31.11ms`, `barn/toast=3.5x`.
- Neighboring rows: `builtin_tostr_1M` Barn `282.37ms` vs prior `479.65ms`; `prop_access_1M` Barn `174.72ms` vs prior `269.62ms`.
- Local command: `go test ./vm -run '^$' -bench 'BenchmarkVM/(builtin_abs_200k|int_arith_1M|nested_1k)' -benchmem -count=3`
- Local result: `BenchmarkVM/builtin_abs_200k`: `32.1-33.7ms/op`, `6402847-6402868 B/op`, `799484 allocs/op`.
- Telemetry: post-slice profiles under `.tmp\profiles\verb-call-stackargs.*` and `.tmp\profiles\verb-call-registry.*` confirmed `PopN` allocation was removed from the fixed-argc builtin call path.

Failure analysis:
- Profiler or operational command: `wsl --cd /mnt/c/Users/Q/code/barn --exec bash -lc 'go build -o /tmp/barn_conformance ./cmd/barn && cd /mnt/c/Users/Q/code/moo-conformance-tests && uv run moo-conformance --server-command "/tmp/barn_conformance -db {db} -port {port}"'`
- Compared against: baseline managed conformance `3871 passed, 131 skipped`.
- Dominant cost before: fixed-argc builtin calls allocated a fresh `[]types.Value` via `PopN` and repeated signature/profiling bookkeeping per call.
- Dominant cost after: argument slice allocation is gone for fixed-argc builtins; remaining cost is general VM/interface boxing and dynamic protected-builtin checking.
- Interpretation: shrank. A discarded attempt to cache `IsProtectedBuiltin` at registration time improved benchmarks further but was invalid because protected builtins are database/runtime state refreshed by `load_server_options()`.
- Next target from evidence: general VM value boxing in `executeNeg`, accumulator add, and builtin return values, or a principled dynamic protected-builtin cache updated by `LoadProtectedBuiltinsFromStore`.

Fast contracts:
- `go test ./types ./builtins ./bytecode ./vm`
- `git diff --check`

Metric gate:
- `go test ./vm -run '^$' -bench 'BenchmarkVM/(builtin_abs_200k|int_arith_1M|nested_1k)' -benchmem -count=3`
- `wsl --cd /mnt/c/Users/Q/code/moo-conformance-tests --exec bash -lc 'bash bench/run_bench.sh'`
- `wsl --cd /mnt/c/Users/Q/code/barn --exec bash -lc 'go build -o /tmp/barn_conformance ./cmd/barn && cd /mnt/c/Users/Q/code/moo-conformance-tests && uv run moo-conformance --server-command "/tmp/barn_conformance -db {db} -port {port}"'`

Outcome: positive.

Decision: promote.

Generated diagnostics:
- `.tmp\profiles\verb-call-baseline.cpu`
- `.tmp\profiles\verb-call-baseline.mem`
- `.tmp\profiles\verb-call-stackargs.cpu`
- `.tmp\profiles\verb-call-stackargs.mem`
- `.tmp\profiles\verb-call-registry.cpu`
- `.tmp\profiles\verb-call-registry.mem`

These generated diagnostics were not committed.
