# Realistic corpus review, 2026-09-17

The corrected realistic corpus and the tight-loop baseline were run with five interleaved samples per workload on WSL Debian. Both lanes use the same Test.db fixture. All returned values matched; every realistic workload has a Toast median in the requested 20-200 ms range. These are serial microbenchmarks, not multiplayer goodput measurements.

The initial corpus run exposed five workloads outside the target range. Their iteration counts were adjusted before the recorded run. The callers_chain mismatch comment was stale: the recorded values match after #305. This is one local comparison, with no machine isolation or cross-run confidence interval; ratios identify profiling candidates rather than proving a root cause.

## Reproduction

From this checkout:

```powershell
python scripts/bench_differ.py --corpus experiments/corpus-real.txt --repeats 5 --out .tmp/realistic
python scripts/bench_differ.py --repeats 5 --barn-linux .tmp/realistic/barn_linux --out .tmp/tight
```

## Profiling order

1. Protected-builtin dispatch (15.15x Toast): inspect dispatch, frame setup and finalization together before attributing the gap to lookup.
2. Map updates (7.95x for 100 keys, 4.54x for the small map): investigate copy/allocation cost.
3. Call-stack introspection, growing property lists and set operations (3.44-3.68x).
4. General verb/string/recursion work (roughly 2-3x), then tight arithmetic loops (roughly 1.5x).

The error-idiom and local exception rows are faster than Toast here, so they do not lead the next serial profiling pass. This updates the plan's measurement priority; it is not evidence that any specific implementation change is sufficient.

## Realistic corpus


- db: `C:\Users\Q\code\moo-conformance-tests\src\moo_conformance\_db\Test.db` sha256 `1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`
- toast: `/root/src/toaststunt/build-release/moo` (WSL Debian)
- barn: linux/amd64 cross-build sha256 `f66ef68646c134e1c864581e977af6e97bec1f398b1f25b52bbdccf7bce11f30` from `C:\Users\Q\code\barn-review-307` @ 968a3d0
- repeats: 5 (interleaved); timing = in-MOO ftime(1) bookends around eval()
- lane wall clock: toast 6.7s, barn 18.4s

| workload | toast ms (med/min) | barn ms (med/min) | barn/toast | n | values |
|---|---|---|---|---|---|
| protected_valid_wrapper | 60.16 / 58.95 | 911.15 / 864.09 | 15.15x | 5/5 | match |
| map_get_put_100 | 36.94 / 35.25 | 293.67 / 258.54 | 7.95x | 5/5 | match |
| map_small_get_put | 60.40 / 57.28 | 274.35 / 262.99 | 4.54x | 5/5 | match |
| callers_chain | 39.18 / 37.39 | 144.24 / 140.59 | 3.68x | 5/5 | match |
| prop_write_growing_list | 69.60 / 66.63 | 252.80 / 217.36 | 3.63x | 5/5 | match |
| list_member_setadd | 53.57 / 52.21 | 184.05 / 173.31 | 3.44x | 5/5 | match |
| raise_through_verb | 72.10 / 68.68 | 205.92 / 188.89 | 2.86x | 5/5 | match |
| string_mix | 66.73 / 65.48 | 182.14 / 169.58 | 2.73x | 5/5 | match |
| recursion_40 | 48.20 / 46.38 | 119.40 / 114.79 | 2.48x | 5/5 | match |
| verb_chain_6deep | 75.72 / 68.58 | 164.30 / 160.79 | 2.17x | 5/5 | match |
| for_in_list_1000 | 71.98 / 71.61 | 146.66 / 143.64 | 2.04x | 5/5 | match |
| sysobj_verb_calls | 60.33 / 57.90 | 119.19 / 115.39 | 1.98x | 5/5 | match |
| sysprop_reads | 75.41 / 72.83 | 146.64 / 141.01 | 1.94x | 5/5 | match |
| valid_typeof_loop | 95.95 / 91.62 | 152.12 / 145.28 | 1.59x | 5/5 | match |
| inherited_prop_6anc | 134.35 / 131.60 | 174.25 / 164.91 | 1.30x | 5/5 | match |
| try_except_raise | 128.47 / 122.09 | 68.58 / 63.72 | 0.53x | 5/5 | match |
| catch_propnf_loop | 111.65 / 107.78 | 28.83 / 26.07 | 0.26x | 5/5 | match |

## Tight-loop baseline


- db: `C:\Users\Q\code\moo-conformance-tests\src\moo_conformance\_db\Test.db` sha256 `1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`
- toast: `/root/src/toaststunt/build-release/moo` (WSL Debian)
- barn: linux/amd64 cross-build sha256 `f66ef68646c134e1c864581e977af6e97bec1f398b1f25b52bbdccf7bce11f30` from `C:\Users\Q\code\barn-review-307` @ 968a3d0
- repeats: 5 (interleaved); timing = in-MOO ftime(1) bookends around eval()
- lane wall clock: toast 12.6s, barn 6.9s

| workload | toast ms (med/min) | barn ms (med/min) | barn/toast | n | values |
|---|---|---|---|---|---|
| noop | 0.00 / 0.00 | 0.01 / 0.01 | 2.26x | 5/5 | match |
| list_index_1M | 56.20 / 54.76 | 113.01 / 111.98 | 2.01x | 5/5 | match |
| prop_access_1M | 117.48 / 113.17 | 195.02 / 192.06 | 1.66x | 5/5 | match |
| nested_loop_2500x2500 | 154.77 / 152.46 | 246.51 / 243.26 | 1.59x | 5/5 | match |
| float_arith_5M | 143.03 / 142.20 | 217.09 / 216.19 | 1.52x | 5/5 | match |
| int_arith_5M | 130.46 / 128.31 | 196.40 / 194.72 | 1.51x | 5/5 | match |
| builtin_abs_200k | 17.27 / 16.93 | 25.44 / 24.69 | 1.47x | 5/5 | match |
| builtin_tostr_1M | 314.92 / 310.29 | 295.79 / 291.92 | 0.94x | 5/5 | match |
| string_concat_50k | 28.27 / 27.44 | 6.64 / 6.35 | 0.24x | 5/5 | match |
| list_append_30k | 1475.29 / 1453.85 | 6.06 / 4.53 | 0.00x | 5/5 | match |

## Remaining multiplayer blocker

The TCP driver still counts OUTPUTSUFFIX as completion. A mocked suspended command is counted as successful without a terminal acknowledgement. No multiplayer report is produced or endorsed here. Local process-ownership cleanup regressions use mocked calls and send no real signals.
