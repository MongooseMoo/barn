# C5 Coder — Fix #1: Collapse the VM dispatch loop

**Branch:** `perf/c5-dispatch` off master `6278e38`
**Date:** 2026-06-27
**Scope:** remove the per-opcode end-of-code bounds check (`vm/vm.go:256`, profiled at 13.4% of CPU) by relying on the compiler's terminal halt opcode; simplify the loop condition. Behavior unchanged.

---

## 1. What changed

### vm/vm.go — `executeLoop` (hot path), ~line 249
Removed the per-opcode bounds check and the `if/else` it gated. Before:

```go
for len(vm.Frames) > 0 {
    cur := vm.frame
    var err error
    if cur.IP >= len(cur.Program.Code) {   // <- 13.4% of total CPU, profiler vm.go:256
        vm.Return(types.NewInt(0))
    } else {
        op := bytecode.OpCode(cur.Program.Code[cur.IP])
        cur.IP++
        if bytecode.CountsTick(op) { vm.Ticks++; vm.syncContextTicks() }
        err = vm.Execute(op)
    }
    if err != nil { ...
```

After:

```go
for vm.frame != nil {                      // <- pointer test, not len(slice) reload
    cur := vm.frame
    op := bytecode.OpCode(cur.Program.Code[cur.IP])   // <- direct fetch, no bounds check
    cur.IP++
    if bytecode.CountsTick(op) { vm.Ticks++; vm.syncContextTicks() }
    err := vm.Execute(op)
    if err != nil { ...
```

Two changes, both behavior-preserving:
1. **Loop condition `len(vm.Frames) > 0` → `vm.frame != nil`.** `pushFrame`/`popFrame` (vm.go:35–49) maintain the invariant `vm.frame == nil ⟺ len(vm.Frames) == 0`, so this is exactly equivalent and replaces a slice-header load + length compare with a single pointer compare.
2. **Dropped the `IP >= len(Code)` bounds check** and fetch `Code[IP]` directly. A 20-line comment documents the *terminator invariant* it now relies on.

### Terminator approach (no new opcode needed)
The "halt opcode" already exists: the compiler **always** emits a terminal frame-popping opcode at the end of every program:
- `bytecode/compiler.go` `Compile` (line 121/123) emits `OP_RETURN` (loop-valued program) or `OP_RETURN_NONE`.
- `CompileStatements` (line 162/164/167) emits `OP_RETURN`/`OP_RETURN_NONE`, including the **empty-program** case → `OP_RETURN_NONE` (line 167).
- `CompileVerbBytecode` (line 198) routes through `CompileStatements`.
- `Program.ExtractForkBody` (`bytecode/program.go:76`) appends `OP_RETURN_NONE` to fork sub-programs.

`OP_RETURN_NONE` calls `vm.Return(NewInt(0))`, which pops the frame and yields the MOO default `0`. So execution can never run off the end of `Code`; it always reaches the terminator first. **No compiler change was required** — the invariant already held universally; this fix only deletes the now-provably-dead runtime check and documents the dependency.

### Coverage audit (every program-compilation path terminates)
- Verbs / eval / top-level: `Compile` / `CompileStatements` — terminate. ✔
- Fork bodies: `ExtractForkBody` — appends `OP_RETURN_NONE`. ✔
- Lambdas: **no lambda-function compile path exists** in Barn. The `lambda`/`anonymous` hits in the codebase are anonymous *object* GC (`vm/anonymous_gc.go`), unrelated. ✔
- Raw hand-built `Program{}` literals reaching the VM: **none** — `grep "Program{"` across `vm/*_test.go` = 0; production `&Program{}` construction exists only in the compiler and `ExtractForkBody`. ✔
- DB-loaded verbs: `CompileVerbBytecode` is source-hash keyed and recompiles on miss (immutable cached `*Program`), so loaded verbs are compiler output → terminated. ✔

### What I deliberately did NOT do (and why)
Sub-fix #2 (thread `ip` as a loop local) was **not** applied. Operand decoding (`ReadByte`/`ReadShort`) and jumps mutate `vm.frame.IP` from ~50 handler call-sites across `op_*.go`. Caching `ip` in the loop would desynchronize from those mutations unless every handler signature is rewritten to take/return the local — too invasive to guarantee "behavior must not change." `cur := vm.frame` is already cached for the fetch; the pinned 13.4% win is the bounds check, which is fully captured here. `Execute` is left intact (merging it is high-risk for zero correctness margin). The `switch` is untouched (profiler proved it is already a jump table at 1.5%).

### Tests added — `vm/dispatch_falloff_test.go`
Characterization tests that pin the fall-off-end behavior the change depends on:
- `TestFallOffEndReturnsZero` — `"x = 5; y = x + 10;"` (no trailing return) → `0`.
- `TestEmptyProgramReturnsZero` — `""` → `0`.
- `TestExprStmtNoReturnFallsOffToZero` — `"1 + 2;"` → `0`.
- `TestEveryCompiledProgramEndsWithTerminator` — structural guard: 6 program shapes (empty, assignment, bare expr, explicit return, trailing loop, conditional) each end in `OP_RETURN`/`OP_RETURN_NONE`. Fails loudly if a future compiler change drops the terminator (which would otherwise become an OOB read).

**RED→GREEN:** all four pass on unmodified master (characterization), and stay green after the change:
```
$ go test ./vm -run 'TestFallOffEndReturnsZero|TestEmptyProgramReturnsZero|TestExprStmtNoReturnFallsOffToZero|TestEveryCompiledProgramEndsWithTerminator' -count=1
ok  	barn/vm	0.875s
```

---

## 2. Gates

### go test ./vm ./bytecode
```
ok  	barn/vm	1.074s
ok  	barn/bytecode	0.396s
```

### go test ./vm -race
```
ok  	barn/vm	1.178s
```

### go build ./... + go vet
```
BUILD_OK
vm\stack.go:49:15: method ReadByte() byte should have signature ReadByte() (byte, error)
```
The single vet note is **pre-existing** (an io.ByteReader naming-convention hint on `ReadByte`, untouched by this change).

### Conformance (managed mode)
```
uv run --project ../moo-conformance-tests moo-conformance \
  --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"

================ 3988 passed, 131 skipped in 160.33s (0:02:40) ================
```
**3988 passed / 131 skipped / 0 failed — target met, no regression.**

---

## 3. benchstat

### 3a. vs C0 baseline `experiments/perf-baseline-vm-20260627.txt` (the campaign baseline)
> Note: this baseline predates C2 (it shows ~2M allocs/op vs master's 11), so the ns/op delta below is the **combined** C2+C5 improvement over C0; the alloc collapse is C2's.

```
                        │   baseline (C0)   │   perf-c5-dispatch-after   │
                        │      sec/op       │    sec/op      vs base     │
VM/int_arith_1M-32          59.33m ±  6%      33.66m ±  8%  -43.27%  (p=0.000)
VM/float_arith_1M-32        68.67m ±  3%      33.48m ±  4%  -51.24%  (p=0.000)
VM/string_concat_10k-32    1331.5µ ± 11%      785.6µ ±  7%  -41.00%  (p=0.000)
VM/list_append_10k-32       2.225m ± 15%      1.218m ± 15%  -45.25%  (p=0.000)
VM/list_index_1M-32        139.19m ± 15%      80.41m ±  3%  -42.23%  (p=0.000)
VM/builtin_abs_200k-32      30.15m ±  1%      21.35m ±  8%  -29.20%  (p=0.000)
VM/tostr_200k-32            57.57m ±  1%      50.69m ± 35%  -11.95%  (p=0.023)
VM/nested_1k-32             55.65m ± 10%      44.66m ±  9%  -19.74%  (p=0.000)
VM/list_iter_1M-32          95.15m ± 10%      55.04m ±  6%  -42.15%  (p=0.000)
geomean                     29.20m            18.30m        -37.32%
```
**int_arith_1M −43.27%, nested_1k −19.74%** — both down significantly; no workload regresses.

### 3b. ISOLATED — my C5 change alone (vs current master `6278e38`, same 4-bench subset, captured back-to-back under identical conditions)
```
                     │ master 6278e38 │   perf-c5 (after)   │
                     │     sec/op     │   sec/op    vs base │
VM/int_arith_1M-32      35.04m ± 3%     31.51m ± 1%  -10.07% (p=0.000)
VM/float_arith_1M-32    35.09m ± 1%     33.76m ± 1%   -3.79% (p=0.002)
VM/nested_1k-32         33.13m ± 6%     31.02m ± 4%   -6.35% (p=0.000)
VM/list_iter_1M-32      55.66m ± 4%     53.93m ± 3%   -3.11% (p=0.007)
geomean                 38.80m          36.53m        -5.87%
```
**Isolated C5 contribution: int_arith −10.07%, nested −6.35%, all workloads down, none regress.**

The real-world wall-clock win (~10% on the hottest arithmetic loop) is more modest than the profiler's 25–40% prediction because the profiler's 13.4% was measured *under cpuprofiling*, where `asyncPreempt` sampling inflates a tight non-preemptible loop. Removing the single hottest line (a pointer-chase + slice-len + branch on every opcode) yields a clean, significant, regression-free improvement across every VM workload.

**Methodology note (recorded for the campaign):** an initial isolation that compared the *full-suite* after-run against a *subset* master-run spuriously reported nested_1k at **+34.83%**. That was a machine-state/thermal confound from comparing benchstat files captured under different run conditions, NOT a real regression — the controlled same-subset rerun above shows −6.35%. Only compare benchstat files captured back-to-back with identical bench sets.

Artifacts: `experiments/perf-c5-dispatch-after.txt` (full suite), `experiments/perf-c5-master-6278e38.txt` + `experiments/perf-c5-after-subset.txt` (isolation).

---

## 4. Commit
Branch `perf/c5-dispatch` off master `6278e38`. Single commit:
`perf(c5): collapse VM dispatch loop hot path` — authoritative hash via
`git -C C:/Users/Q/code/barn log --oneline -1 perf/c5-dispatch`.
