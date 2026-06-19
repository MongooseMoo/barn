# CODER report — rebase feat/bytecode-package onto master 507b3b7

Worktree `C:/Users/Q/code/barn-bytecode`. Backup tag `backup-pre-rebase-aa4de1e` = old tip aa4de1e.

## The 3 perf commits (0bc99b3..507b3b7), read before resolving
- `027e358` vm-bench harness + first wins: adds vm/perf_bench_test.go (BenchmarkVM);
  executeAdd operand-order, IntValue.String/valueToStr strconv, list append/extend bulk copy.
  Touches builtins/types.go, types/int.go, vm/op_arith.go, vm/op_list.go, vm/perf_bench_test.go.
- `fb438f5` O(1) list size: caches ValueBytes size on the list, incremental Append/Concat.
  Touches builtins/limits.go, types/list.go, types/value_bytes.go, vm/op_list.go.
- `507b3b7` frame cache + inline dispatch: adds vm.frame field + pushFrame/popFrame helpers;
  CurrentFrame() = O(1) field read; INLINES Step() into executeLoop (new code using bare
  OpCode()/CountsTick()). Touches vm/op_verb.go, vm/registry.go, vm/stack.go, vm/vm.go.

## Git mechanism
`git rebase --onto 507b3b7 0bc99b3 feat/bytecode-package`. Replayed 2 commits (the spike
extraction 1be12b9 -> 3b20d14, and my work aa4de1e -> 7979d6f). merge-base now = 507b3b7 (confirmed).

## Conflict resolution
git auto-merged with NO textual conflict markers, BUT auto-merge silently took the perf
commit's NEW inlined executeLoop verbatim (bare `OpCode()`/`CountsTick()`), which my extraction
had removed the aliases for. Build caught it:
```
vm\vm.go:260:10: undefined: OpCode
vm\vm.go:262:7: undefined: CountsTick
```
This is the documented "preserve both" case: KEEP the perf inlined dispatch loop, RETARGET the
two moved symbols to bytecode.X (same pattern already in the old Step() at vm.go:399-403, which
uses bytecode.OpCode / bytecode.CountsTick, and Execute(op bytecode.OpCode)). NOT ambiguous —
resolved by editing vm.go:260/262 to bytecode.OpCode / bytecode.CountsTick. Perf logic (the
inlined loop, vm.frame cache, pushFrame/popFrame) fully preserved.

## Conflicts resolved (all the same "preserve perf, retarget moved type" pattern; NONE ambiguous)
1. vm/vm.go inlined executeLoop (perf 507b3b7 new code): bare `OpCode()`/`CountsTick()` ->
   `bytecode.OpCode`/`bytecode.CountsTick`. Perf inlined-dispatch loop kept verbatim.
2. vm/perf_bench_test.go (perf 027e358 new bench harness): `*Program` -> `*bytecode.Program`,
   `NewCompilerWithRegistry` -> `bytecode.NewCompilerWithRegistry`, added barn/bytecode import.
   BenchmarkVM logic unchanged.

## Perf preservation verified
- vm/vm.go diff vs 507b3b7 = ONLY type retargets (OpCode/CountsTick/Execute sig/Program field).
  pushFrame/popFrame/vm.frame/inlined loop/O(1) CurrentFrame do NOT appear as +/- => identical.
- fb438f5 O(1) list: types/list.go cachedBytes + value_bytes.go present; op_list uses
  list.Append/list.Concat (incremental). 027e358: strconv.FormatInt in types/int.go present;
  perf_bench_test.go present.

## Gate output (all green)
**go build ./...** exit 0.

**go vet ./...** — only the 2 known pre-existing findings:
```
cmd\moo_client\main.go:53:25: address format "%s:%d" does not work with IPv6 ...
vm\stack.go:49:15: method ReadByte() byte should have signature ReadByte() (byte, error)
```

**go test ./...** — only the 2 known pre-existing fixture failures (barn/conformance missing
../cow_py/tests/conformance; barn/db/format missing mongoose7_snapshot.db). All else PASS
(bytecode, vm, builtins, server, db/store, types).

**db/store parser-free:** `go list -deps ./db/store | grep parser` -> EMPTY (grep exit 1).

**Conformance (managed harness from this worktree):**
```
================ 3871 passed, 131 skipped in 142.42s ================
```
EXACTLY 3871 passed / 0 failed / 131 skipped.

**Cache benchmark (warm << cold, hit != recompile):**
```
BenchmarkCompileVerbCold-32     733729     1580 ns/op    3750 B/op   27 allocs/op
BenchmarkCompileVerbWarm-32   45696703      28.71 ns/op     0 B/op    0 allocs/op
```

**Perf non-regression — BenchmarkVM, 507b3b7 baseline vs rebased branch (ns/op):**
```
bench                baseline        rebased
int_arith_1M         97,202,892      98,260,525
float_arith_1M       98,911,475      97,552,533
string_concat_10k    16,076,553      13,122,841
list_append_10k    1,005,278,200*  1,481,610,350*   (*single-shot noise; see below)
list_index_1M        254,202,880    191,165,683
tostr_200k           129,563,300     87,915,323
nested_1k            129,987,738     97,395,425
```
allocs/op identical to baseline on every bench (deterministic; differences are <0.1% from b.N).
Most ns/op equal-or-faster on rebased. The one apparent outlier (list_append_10k) is a
high-variance O(n^2) ~1.6GB-per-op memory-bound workload that ran only 1-2 iters; re-ran
focused -benchtime=5x -count=3:
```
rebased:  {1039.97M, 1406.77M, 1404.16M} ns/op   allocs ~89,9xx
baseline: {1263.46M, 1514.23M, 1629.87M} ns/op   allocs ~89,9xx
```
Ranges overlap heavily and rebased trends LOWER. VERDICT: NO regression. Expected — the rebase
touched only type references in the VM, no executed logic changed, so the perf profile is
identical to baseline within measurement noise.

## VERDICT: rebase clean (2 mechanical retargets, no ambiguity, all perf preserved), all gates green.

## Final state
- New base: 507b3b7 (confirmed via merge-base).
- New branch tip: `521e5d7` ("Rebase onto 507b3b7: retarget perf hot-path code to bytecode package").
  History: 521e5d7 (rebase fixes) -> 7979d6f (extraction+cache) -> 3b20d14 (spike) -> 507b3b7 (master).
  Merge squashes, so granularity is immaterial.
- Backup tag `backup-pre-rebase-aa4de1e` points at the pre-rebase tip (aa4de1e) for safety.
- NOT merged (verifier re-gates next). Main tree, master, spike branches untouched.
- Nothing stopped on: no ambiguous conflicts; every conflict was the expected
  "keep perf code, retarget moved type to bytecode.X" pattern, caught by the compiler and
  verified against master's diff (perf logic byte-identical).
