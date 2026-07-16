# Barn VM current development profile

Date: 2026-07-15

## 1. Commit, worktree, and commands

Collection ran from `C:\Users\Q\code\barn` on branch `master` at
`c87229f66a4cf6e45234275f1eed53db8722aa77`. That commit is a verified linear
descendant of campaign frame commit
`7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644`. Before collection, the only
tracked modification was the authorized campaign checkpoint history in
`notes-barn-performance-campaign.md`; unrelated untracked state was preserved.

CPU collection, completed with real exit code 0 in retained session `73512`:

```powershell
go test ./vm -run='^$' -bench='^BenchmarkVM/(int_arith_1M|float_arith_1M|string_concat_10k|list_append_10k|list_index_1M|builtin_abs_200k|tostr_200k)$' -benchtime=3s -count=1 -cpu=1 -outputdir='C:\Users\Q\AppData\Local\Temp\barn-performance-profile-c87229f-20260715' -cpuprofile='C:\Users\Q\AppData\Local\Temp\barn-performance-profile-c87229f-20260715\vm-dev-cpu.prof'
```

Allocation collection, completed with real exit code 0 in retained session
`71570`:

```powershell
go test ./vm -run='^$' -bench='^BenchmarkVM/(int_arith_1M|float_arith_1M|string_concat_10k|list_append_10k|list_index_1M|builtin_abs_200k|tostr_200k)$' -benchtime=3s -count=1 -cpu=1 -outputdir='C:\Users\Q\AppData\Local\Temp\barn-performance-profile-c87229f-20260715' -memprofile='C:\Users\Q\AppData\Local\Temp\barn-performance-profile-c87229f-20260715\vm-dev-mem.prof'
```

The retained profiles were read with these commands, each of which exited 0:

```powershell
go tool pprof -top -nodecount=30 experiments/2026-07-15-vm-dev-cpu.prof | Tee-Object -FilePath experiments/2026-07-15-vm-dev-cpu-top.txt
go tool pprof -top -cum -nodecount=30 experiments/2026-07-15-vm-dev-cpu.prof | Tee-Object -FilePath experiments/2026-07-15-vm-dev-cpu-cum.txt
go tool pprof -alloc_space -top -nodecount=30 experiments/2026-07-15-vm-dev-mem.prof | Tee-Object -FilePath experiments/2026-07-15-vm-dev-alloc-space-top.txt
go tool pprof -alloc_objects -top -nodecount=30 experiments/2026-07-15-vm-dev-mem.prof | Tee-Object -FilePath experiments/2026-07-15-vm-dev-alloc-objects-top.txt
```

CPU and allocation profiles were collected separately. Neither holdout row was
opened.

## 2. Sample duration and counts

The CPU profile covers 28.27 seconds and contains 27.57 seconds of sampled CPU
time, or 97.52% of wall duration. The flat view's displayed top 30 nodes account
for 23.39 seconds (84.84%) of the total; the cumulative view's displayed top 30
nodes account for 19.89 seconds (72.14%). Both commands dropped nodes whose
cumulative contribution was at most 0.14 seconds.

The allocation command used one process, one CPU, one measured run, and a
three-second benchtime per selected row. Its observed benchmark iteration counts
were 100 integer-arithmetic, 100 float-arithmetic, 5,236 string-concatenation,
3,406 list-append, 43 list-index, 262 builtin-abs, and 97 `tostr` iterations.
The allocation-space profile estimates 8,824.04 MB total allocation churn; the
allocation-object profile estimates 127,252,720 allocated objects.

## 3. CPU flat Barn-owned functions

| Barn-owned function | Flat CPU | Cumulative CPU |
|---|---:|---:|
| `barn/vm.(*VM).executeLoop` | 22.92% | 99.09% |
| `barn/vm.(*VM).Execute` | 12.40% | 72.87% |
| `barn/vm.(*VM).ReadByte` | 9.03% | 11.39% |
| `barn/builtins.(*Registry).dispatch` | 5.80% | 13.06% |
| `barn/vm.(*VM).Pop` | 3.30% | 3.30% |
| `barn/bytecode.CountsTick` | 3.16% | 3.16% |
| `barn/vm.(*VM).CurrentFrame` | 2.36% | 2.36% |
| `barn/vm.(*VM).Push` | 2.03% | 5.01% |
| `barn/vm.(*VM).ReadShort` | 1.96% | 1.99% |
| `barn/vm.(*VM).executeCallBuiltin` | 1.81% | 18.06% |
| `barn/builtins.(*Registry).CallByID` | 1.78% | 14.83% |

These are aggregate percentages across all seven selected development rows.

## 4. CPU cumulative Barn-owned callers

| Barn-owned caller | Flat CPU | Cumulative CPU |
|---|---:|---:|
| `barn/vm.BenchmarkVM.func1` | 0% | 99.31% |
| `barn/vm.(*VM).Run` | 0% | 99.13% |
| `barn/vm.(*VM).executeLoop` | 22.92% | 99.09% |
| `barn/vm.(*VM).Execute` | 12.40% | 72.87% |
| `barn/vm.(*VM).executeCallBuiltin` | 1.81% | 18.06% |
| `barn/builtins.(*Registry).CallByID` | 1.78% | 14.83% |
| `barn/builtins.(*Registry).dispatch` | 5.80% | 13.06% |
| `barn/vm.(*VM).ReadByte` | 9.03% | 11.39% |
| `barn/vm.(*VM).executeStringAppend` | 0.80% | 10.01% |
| `barn/vm.(*VM).executeListAppend` | 0.11% | 8.96% |
| `barn/types.(*sliceList).append` | 0.87% | 7.29% |
| `barn/types.(*strRep).appendRep` | 0.51% | 5.69% |
| `barn/builtins.builtinTostr` | 0.073% | 4.93% |

High cumulative percentages here describe inclusive call paths; they do not
mean that the caller itself owns the same flat cost.

## 5. Allocation leaders

Allocation-space leaders:

| Function | Allocated space | Share |
|---|---:|---:|
| `barn/types.(*sliceList).append` | 4,909.26 MB | 55.64% |
| `barn/types.(*strRep).appendRep` | 2,605.40 MB | 29.53% |
| `barn/types.NewStr` | 897.54 MB | 10.17% |
| `internal/strconv.FormatInt` | 213.50 MB | 2.42% |
| `strings.(*Builder).WriteString` | 70.00 MB | 0.79% |
| `barn/vm.NewVM` | 59.33 MB | 0.67% |
| `barn/types.(*strRep).str` | 48.47 MB | 0.55% |

Allocation-object leaders:

| Function | Allocated objects | Share |
|---|---:|---:|
| `barn/types.(*strRep).appendRep` | 53,415,015 | 41.98% |
| `barn/types.(*sliceList).append` | 35,490,400 | 27.89% |
| `barn/types.NewStr` | 19,607,083 | 15.41% |
| `internal/strconv.FormatInt` | 13,992,149 | 11.00% |
| `strings.(*Builder).WriteString` | 4,587,590 | 3.61% |

The listed allocation-space nodes account for 99.77% of estimated allocated
space. The listed allocation-object nodes account for 99.87% of estimated
allocated objects.

## 6. Runtime and testing overhead versus Barn-owned cost

The testing harness is visible as an inclusive wrapper: `testing.(*B).runN` is
99.35% cumulative CPU and `testing.(*B).launch` is 97.97%, both with 0% flat CPU
in the displayed cumulative view. `barn/vm.BenchmarkVM.func1` is likewise a
0%-flat wrapper at 99.31% cumulative. These entries identify the benchmark call
chain, not self-cost leaders.

Runtime CPU entries include `runtime.tryDeferToSpanScan` at 3.23% flat,
`runtime.mallocgc` at 0.98% flat and 12.95% cumulative,
`runtime.newobject` at 9.43% cumulative, and write-barrier paths at 4.57%
cumulative. Standard-library allocation leaders are
`internal/strconv.FormatInt` and `strings.(*Builder).WriteString`. The larger
flat CPU and allocation shares listed in sections 3 and 5 are attributed to
Barn-owned symbols rather than the testing wrapper.

## 7. Profile limitations and blended-workload ambiguity

These are aggregate profiles from seven benchmarks executed in one test
process. They do not label samples by benchmark row. The rows ran different
iteration counts, and their operation sizes and allocation rates differ, so an
aggregate percentage cannot be assigned to one row without a separately
labelled or isolated profile. Allocation profiles are sampled estimates rather
than exact allocation ledgers. The single `-count=1` collection is diagnostic
attribution, not comparative timing evidence and not a candidate keep gate.

The profiles cover only the development rows named in the command. They provide
no evidence about either sealed holdout row, socket/server behavior, scheduler
concurrency, database load, or deployment RSS.

## 8. Evidence map for a later scout

| Measured cost | Concrete symbol/source location |
|---|---|
| VM execution loop and opcode dispatch | `vm/vm.go:252` (`executeLoop`), `vm/vm.go:428` (`Execute`) |
| Bytecode reads and stack operations | `vm/stack.go:10` (`Push`), `vm/stack.go:20` (`Pop`), `vm/stack.go:49` (`ReadByte`) |
| Opcode count test | `bytecode/opcodes.go:247` (`CountsTick`) |
| Builtin call path | `vm/op_misc.go:10` (`executeCallBuiltin`), `builtins/registry.go:401` (`CallByID`), `builtins/registry.go:426` (`dispatch`) |
| String-append path | `vm/op_arith.go:106` (`executeStringAppend`), `types/str.go:75` (`appendRep`) |
| List-append path | `vm/op_list.go:59` (`executeListAppend`), `types/list.go:82` (`append`) |
| String construction and `tostr` | `types/str.go:22` (`NewStr`), `builtins/types.go:27` (`builtinTostr`), `builtins/types.go:49` (`valueToStr`) |
| Per-run VM construction | `vm/vm.go:82` (`NewVM`) |

This map reports measured ownership and source locations only. No candidate,
ranking, or optimization proposal is part of this instrumentation deliverable.
