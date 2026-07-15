# Barn VM measured candidate scout

Date: 2026-07-15

This is an ideation artifact, not an experiment record. No candidate was run,
no triage budget was consumed, and neither sealed holdout row was opened.

## 1. Repository identity and search record

- Repository: `git@github.com:MongooseMoo/barn.git`, working tree
  `C:\Users\Q\code\barn`, branch `master`, HEAD
  `d9c9f4dfe7d9fe8ffe0168f8e6a6738ad04f7a7d`. These values were observed with
  `git remote get-url origin`, `git branch --show-current`, and
  `git rev-parse HEAD` before source investigation.
- After the mandatory checkpoint append, tracked state showed only that exact
  authorized scout addition to `notes-barn-performance-campaign.md`; `git diff`
  confirmed there was no other tracked modification. The complete pre-existing untracked
  inventory was enumerated with `git ls-files --others --exclude-standard` and
  left untouched.
- Campaign authority searched: `experiments/INDEX.md:3-41,44-69,81-116`,
  `reports/barn-performance-prior-art-2026-07-15.md:36-82,201-305,307-379`, and
  `reports/barn-vm-current-profile-2026-07-15.md:39-162`.
- Production/compiler source searched: `vm/vm.go:15-136,251-368,427-686`,
  `vm/stack.go:9-63`, `bytecode/opcodes.go:17-139,224-254`,
  `bytecode/compiler.go:100-176,1273-1373,1688-1768`,
  `bytecode/program.go:69-102`, `vm/op_misc.go:10-79`,
  `builtins/registry.go:16-53,349-474`, `vm/op_arith.go:39-157`,
  `types/str.go:9-135`, `vm/op_list.go:9-101`, `types/list.go:8-220`, and
  `builtins/types.go:24-83`.
- Development benchmark source searched, excluding holdout results and
  execution: `vm/perf_bench_test.go:26-32,37-69`. Compilation and store setup
  are outside the timer; each timed iteration creates a task context and VM and
  runs a precompiled program (`vm/perf_bench_test.go:37-69`).
- Correctness contracts searched: `vm/list_inplace_aliasing_test.go:9-63`,
  `vm/string_inplace_aliasing_test.go:10-89`,
  `vm/op_arith_routing_test.go:9-72`, `vm/dispatch_falloff_test.go:10-63`,
  `builtins/dispatch_characterization_test.go:12-16,53-245`,
  `types/value_struct_test.go:71-210`,
  `bytecode/source_compilation_contract_test.go:14-128`, and
  `bytecode/program_test.go:5-82`.
- Git history searched with `git log --all` over every named production and
  benchmark path. Commit bodies/diffs were checked for the kept VM lineage
  (`027e358`, `fb438f5`, `507b3b7`, `6094276`, `a03f4b2`, `8c43bee`,
  `30d4288`, `1ef32f1`, `e0cdce8`, `c6d81e7`, `5bf93a1`, `fd17fa3`, and
  `cf67990`) and the adjacent reports/ledgers named below. The repository's
  consolidated kept results are independently listed at
  `reports/barn-performance-prior-art-2026-07-15.md:309-336`.
- Adjacent reports/ledgers searched: `reports/perf-c1-scout.md:1-267`,
  `reports/perf-c4-scout.md:1-303`, `reports/perf-c5-profiler.md:1-229`,
  `experiments/2026-06-22-builtin-call-performance.md:1-61`,
  `reports/perf-c2-final-analyst.md:43-184`, and
  `experiments/2026-06-24-commit-dominated-concurrency-ledger.md:857-897`.

## 2. Current measured source shapes and constraints

The current profile is one aggregate, separately collected CPU/allocation view
over the seven development rows. It does not label samples by row, and its
single diagnostic run is not comparative timing evidence
(`reports/barn-vm-current-profile-2026-07-15.md:137-149`). Candidate row effects
below are therefore hypotheses to kill with isolated development probes.

### CPU dispatch, byte reads, ticks, and stack

- `executeLoop` is 22.92% flat/99.09% cumulative and `Execute` is 12.40% flat;
  `ReadByte`, `ReadShort`, and `CurrentFrame` add 9.03%, 1.96%, and 2.36% flat
  respectively (`reports/barn-vm-current-profile-2026-07-15.md:54-68`). The loop
  already caches `cur := vm.frame`, but `Execute` cases still call methods which
  reload `vm.frame` and `Program.Code` (`vm/vm.go:272-282,427-460`;
  `vm/stack.go:48-62`). Frame/IP state must remain correct across call, return,
  error unwind, yield, and resume (`vm/vm.go:214-249,282-360`).
- `CountsTick` is 3.16% flat in the aggregate profile
  (`reports/barn-vm-current-profile-2026-07-15.md:62-66`). Its switch recognizes
  exactly builtin calls, verb calls, loop backedges, fused range-next, and
  `pass()` (`bytecode/opcodes.go:246-254`); the loop increments ticks, synchronizes
  context-visible remaining ticks, and later enforces the limit
  (`vm/vm.go:278-281,350-359`). Tick identity and observability are correctness
  constraints, not bookkeeping that can simply disappear.
- `Pop` and `Push` are 3.30% and 2.03% flat
  (`reports/barn-vm-current-profile-2026-07-15.md:61-66`). Their underflow and
  stack-growth behavior is explicit (`vm/stack.go:9-26`). They are a secondary
  cost inside the broader decode/dispatch family rather than an independent
  first-round target.
- The removed per-op end-of-code check cannot be revisited: the hot loop now
  depends on every compiler path and extracted fork ending in a frame-popping
  terminator (`vm/vm.go:251-271`; `bytecode/compiler.go:100-176`;
  `bytecode/program.go:69-102`), guarded at `vm/dispatch_falloff_test.go:10-63`.

### Builtin call/dispatch

- `executeCallBuiltin` is 18.06% cumulative, `CallByID` 14.83%, and registry
  `dispatch` 13.06%; registry dispatch owns 5.80% flat
  (`reports/barn-vm-current-profile-2026-07-15.md:58-68,78-87`). Fixed arguments
  already borrow the VM stack without allocating (`vm/op_misc.go:25-34`), but
  the call path first indexes registry metadata for line synchronization and
  then indexes/bounds-checks the same entry again in `CallByID`
  (`vm/op_misc.go:36-47`; `builtins/registry.go:384-405`).
- Any consolidation must keep invalid-ID `E_VERBNF`, identical name/ID argument
  validation, protected redirection before validation, `#0` bypass, wizard
  fallthrough, non-wizard denial, and callers/task-stack line synchronization
  (`builtins/registry.go:399-474`;
  `builtins/dispatch_characterization_test.go:90-245`). Runtime protected state
  cannot be frozen at registration because `load_server_options()` replaces it
  (`reports/barn-performance-prior-art-2026-07-15.md:338-342`).

### String construction and append

- `appendRep` owns 29.53% of allocation space and 41.98% of allocated objects;
  `NewStr`, `strconv.FormatInt`, and `Builder.WriteString` own another 10.17%,
  2.42%, and 0.79% of space plus 15.41%, 11.00%, and 3.61% of objects
  (`reports/barn-vm-current-profile-2026-07-15.md:93-115`). The `tostr` row's
  exact baseline is 12.21 MiB and 599.9k allocations/op
  (`experiments/INDEX.md:60-66`).
- Single-argument `tostr(i)` still creates a Builder, formats through a temporary
  Go string, materializes the Builder string, checks the final size, and boxes it
  through `NewStr` (`builtins/types.go:27-60`; `types/str.go:21-24`). Zero- and
  multi-argument concatenation, every `valueToStr` case, the limit refresh/check,
  and exact float/object/error/list/map rendering must remain unchanged
  (`builtins/types.go:24-83`).
- `strRep` stores an immutable string or growable bytes plus a shared watermark;
  every non-empty append returns a new header even when it reuses backing
  capacity (`types/str.go:9-19,64-95`). The new header is load-bearing: string
  aliases captured in variables, lists, maps, keys, and properties must retain
  their old contents (`vm/string_inplace_aliasing_test.go:10-89`). Only
  uncommitted backing bytes may be written and the shared frontier must advance
  monotonically (`reports/perf-c4-scout.md:219-269`).

### List append

- `sliceList.append` owns 55.64% of allocation space and 27.89% of allocated
  objects (`reports/barn-vm-current-profile-2026-07-15.md:93-115`). The
  `list_append_10k` baseline is 1.398 MiB and 10.05k allocations/op, while its
  ±16% timing interval makes small timing deltas unsuitable for cheap triage
  (`experiments/INDEX.md:60-69`).
- List byte size is already cached and append is already amortized O(n): the
  frontier path writes one uncommitted slot, advances the shared watermark, and
  returns a new header; the fallback forces a copy with `[:n:n]`
  (`types/list.go:47-58,82-100`). The O(n²) premise is explicitly dead, while
  the residual one-header-per-append cost was left unexecuted
  (`reports/perf-c4-scout.md:8-36,125-133`).
- Any representation reduction must preserve byte-size accounting and every
  alias divergence case (prior binding, nested capture, three-way share, slice,
  trailing splice, and reassignment chain) at
  `vm/list_inplace_aliasing_test.go:9-63`.

### VM construction

- `NewVM` preallocates stack capacity 256 and frame capacity 16
  (`vm/vm.go:81-94`) but owns only 0.67% of aggregate allocation space
  (`reports/barn-vm-current-profile-2026-07-15.md:99-104`). Smaller initial VM
  capacities and an earlier VM-pooling form were already rejected
  (`experiments/2026-06-24-commit-dominated-concurrency-ledger.md:874-897`).

## 3. Explicit prior-art eligibility

| Nearby idea | Status | Current reason |
|---|---|---|
| Remove the per-op end-of-code check and use cached `vm.frame` | already implemented | Kept by `fd17fa3`; current invariant is visible at `vm/vm.go:251-282` and guarded at `vm/dispatch_falloff_test.go:10-63` (`reports/barn-performance-prior-art-2026-07-15.md:327-328`). |
| Replace the opcode switch with computed-goto/another dispatcher | dead | The measured switch was already a jump table and only 1.5% flat; the report explicitly says not to lead here (`reports/perf-c5-profiler.md:145-149,217`). No cause of death changed. |
| Thread the cached frame/code/IP through operand decode | eligible | It was recommended but not completed (`reports/perf-c5-profiler.md:174-179`); current `ReadByte`/`ReadShort`/`CurrentFrame` still total 13.35% flat (`reports/barn-vm-current-profile-2026-07-15.md:60-66`). |
| Make `CountsTick` a constant-time metadata lookup | eligible | It was recommended but remains the switch at `bytecode/opcodes.go:246-254`; the new profile still assigns it 3.16% flat (`reports/barn-vm-current-profile-2026-07-15.md:62-66`). |
| Numeric-first self-accumulation routing | already implemented | Kept by `c6d81e7` (`reports/barn-performance-prior-art-2026-07-15.md:325-328`); current numeric-first branches are `vm/op_arith.go:110-128`. |
| Replace `OP_STRING_APPEND` with `OP_ADD` and delete the duplicate handler | blocked | Observable float-overflow and `PROMOTE_NUMBERS` behavior still differs and requires Toast verification (`reports/perf-c1-scout.md:229-238`). Its blocker has not changed. |
| O(1) list byte-size caching | already implemented | Kept by `fb438f5`; current cache is `types/list.go:47-58` (`reports/barn-performance-prior-art-2026-07-15.md:314-320`). |
| O(n) watermark-backed list/string append | already implemented | Kept by `30d4288`/`1ef32f1`; O(n²) is explicitly false on current shape (`reports/perf-c4-scout.md:8-35`). |
| Refcount- or compiler-alias-based zero-allocation in-place append | dead | Go `Value` copies have no maintained uniqueness/refcount, and a missed alias corrupts MOO value semantics (`reports/perf-c4-scout.md:213-254`). No cause of death changed. |
| Reduce residual list/string header size while preserving the watermark protocol | eligible | This narrower constant-factor work was not executed (`reports/barn-performance-prior-art-2026-07-15.md:343-345`), and the new profile now measures the two header-producing append owners at 85.17% of allocation space (`reports/barn-vm-current-profile-2026-07-15.md:99-100`). |
| Fixed-argc builtin stack-window arguments | already implemented | Kept by `e0cdce8`; current fixed args alias the VM stack (`vm/op_misc.go:25-34`; `reports/barn-performance-prior-art-2026-07-15.md:323-328`). |
| ID-indexed builtin entry, lock-free protected set, inline validation | already implemented | Kept by `cf67990` (`reports/barn-performance-prior-art-2026-07-15.md:327-328`); current entry layout is `builtins/registry.go:16-39`. |
| Registration-time protected-builtin boolean | dead | Runtime options mutate the protected set, the recorded original cause of death (`reports/barn-performance-prior-art-2026-07-15.md:338-342`). |
| Consolidate the remaining line-sync and call entry lookups | eligible | The prior ceremony was removed, but the current source still indexes the ID entry separately at `vm/op_misc.go:36-47` and `builtins/registry.go:384-405`; the current profile still names the call/dispatch cluster (`reports/barn-vm-current-profile-2026-07-15.md:61-68`). |
| Interface-value de-boxing | already implemented | Kept by `5bf93a1`, reducing numeric allocations to 11/op (`reports/barn-performance-prior-art-2026-07-15.md:325-328`). |
| Shrink the 24-byte `Value` globally | dead | No copy center surfaced, and prior profiling ranked it behind measured structural work (`reports/perf-c5-profiler.md:196-199,215-217`). The current profile still supplies no contrary evidence. |
| Single-argument `tostr` bypass of Builder concatenation | eligible | The current source always constructs the Builder (`builtins/types.go:27-45`), while current exact baseline/profile evidence names Builder, formatting, and string boxing (`experiments/INDEX.md:65`; `reports/barn-vm-current-profile-2026-07-15.md:101-115`). |
| Smaller VM stack/frame capacities or VM pooling | dead | Previously rejected (`experiments/2026-06-24-commit-dominated-concurrency-ledger.md:874-897`) and `NewVM` is now only 0.67% of allocation space (`reports/barn-vm-current-profile-2026-07-15.md:99-104`). |
| GC tuning or PGO | blocked | Prior work explicitly deferred these until structural fixes/reprofiling (`reports/perf-c5-profiler.md:204-215`); the aggregate development profile is not a representative production PGO input and does not prove GC tuning is the first lever (`reports/barn-vm-current-profile-2026-07-15.md:137-149`). |

## 4. Candidate hypotheses

### C1 — Carry the cached frame/code/IP through VM-owned decode

- Evidence/owner: `ReadByte` 9.03%, `ReadShort` 1.96%, and `CurrentFrame` 2.36%
  flat beside `Execute` 12.40% (`reports/barn-vm-current-profile-2026-07-15.md:58-68`);
  owner `vm/vm.go:251-282,427-686` with operand helpers at `vm/stack.go:48-62`.
- Expected rows: all seven; strongest directional signal expected in
  `int_arith_1M`, `float_arith_1M`, and `list_index_1M` because they execute long
  opcode streams. This row attribution is intentionally unconfirmed because the
  aggregate profile is unlabeled (`reports/barn-vm-current-profile-2026-07-15.md:137-145`).
- Killing probe: one isolated `int_arith_1M` CPU profile before source work; kill
  if combined decode/frame samples are below 8% or line attribution shows the
  reloads are already compiler-elided. No holdout.
- Prediction: a frame/code/IP-local implementation lowers `int_arith_1M`
  `sec/op` by at least 3%, with unchanged B/op and allocs/op.
- Guards: frame/IP write-back on call/return/error/yield/resume; terminator tests
  (`vm/dispatch_falloff_test.go:18-63`), general bytecode execution, and no stack
  or tick semantic change.
- Prior art/cost: eligible unfinished portion of the C5 recommendation
  (`reports/perf-c5-profiler.md:174-179`), after the separate bounds-check slice
  already landed. Estimated implementation cost: medium-high.

### C2 — Replace `CountsTick`'s switch with immutable opcode metadata

- Evidence/owner: 3.16% aggregate flat CPU
  (`reports/barn-vm-current-profile-2026-07-15.md:62-66`); exact owner
  `bytecode/opcodes.go:246-254`.
- Expected rows: all loop rows and both builtin-heavy rows; no allocation row.
- Killing probe: one isolated `int_arith_1M` CPU profile with `pprof list` for
  `CountsTick`; kill if its flat share is below 2% in that row.
- Prediction: table/bit metadata lowers targeted-row `sec/op` by at least 1%
  and leaves B/op/allocs/op unchanged.
- Guards: the counted set must remain exactly `CALL_BUILTIN`, `CALL_VERB`,
  `LOOP`, `FOR_RANGE_NEXT`, and `PASS` (`bytecode/opcodes.go:247-253`); tick
  limit and context-visible remaining ticks remain unchanged (`vm/vm.go:278-281,350-359`).
- Prior art/cost: eligible recommendation never implemented
  (`reports/perf-c5-profiler.md:177-179`). Estimated cost: low.

### C3 — Resolve builtin line-sync metadata and dispatch entry once

- Evidence/owner: builtin call/registry path is 18.06%/14.83%/13.06%
  cumulative and 9.39% flat across its three named nodes
  (`reports/barn-vm-current-profile-2026-07-15.md:61-68,80-83`); owner is the
  single builtin-call surface `vm/op_misc.go:10-79` plus
  `builtins/registry.go:384-436`.
- Expected rows: `builtin_abs_200k` and `tostr_200k` (which also calls
  `length()` each iteration); other rows only through incidental builtins.
- Killing probe: one isolated `builtin_abs_200k` CPU profile; kill if the two ID
  entry resolutions and bounds checks are not visible or the whole registry
  cluster is below 5% flat.
- Prediction: one entry resolution lowers both builtin rows by at least 2%
  `sec/op`, with no B/op/allocs/op change.
- Guards: invalid IDs, line-sync for only `callers`/`task_stack`, protected
  redirect-before-validation, raw redirected args, wizard behavior, and exact
  `E_ARGS`/`E_TYPE`/`E_VERBNF` results
  (`builtins/dispatch_characterization_test.go:90-245`).
- Prior art/cost: eligible residual after `cf67990`; it does not retry the dead
  registration-time cache. Estimated cost: medium.

### C4 — Fast-path one-argument `tostr` without a concatenation Builder

- Evidence/owner: exact row baseline 599.9k allocs/op and 12.21 MiB/op
  (`experiments/INDEX.md:65`); Builder, `FormatInt`, and `NewStr` are current
  allocation leaders (`reports/barn-vm-current-profile-2026-07-15.md:101-115`);
  owner `builtins/types.go:27-60`.
- Expected rows: `tostr_200k` only.
- Killing probe: two `-benchmem` samples of `tostr_200k` around only the
  single-argument path; kill if allocs/op fall by less than 150k or timing does
  not move in the predicted direction. No profile is needed first.
- Prediction: at least 150k fewer allocs/op (25% of the baseline) and at least
  3% lower `sec/op`, with no B/op regression.
- Guards: keep the zero-argument empty string, multi-argument concatenation,
  `valueToStr` rendering, `UpdateContextLimits`, and final string-limit check
  exactly as at `builtins/types.go:27-83`.
- Prior art/cost: eligible narrower source shape after builtin ceremony was
  already fixed; it is not the broader pending string-box redesign
  (`reports/barn-performance-prior-art-2026-07-15.md:323-328,348-350`). Estimated
  cost: low.

### C5 — Compact the append-heavy `strRep` header without changing COW

- Evidence/owner: `appendRep` owns 29.53% allocation space/41.98% objects and
  `NewStr` another 10.17%/15.41%
  (`reports/barn-vm-current-profile-2026-07-15.md:99-115`); owner
  `types/str.go:9-95`.
- Expected rows: `string_concat_10k` primarily, then `tostr_200k` through
  `NewStr`.
- Killing probe: one isolated `string_concat_10k` allocation profile or two
  `-benchmem` samples after a representation-only prototype; kill if B/op falls
  by less than 15% or allocs/op rises.
- Prediction: at least 15% lower B/op on `string_concat_10k`, at least 5% lower
  B/op on `tostr_200k`, and no allocs/op increase; timing must be non-regressing.
- Guards: never mutate an existing header; write only uncommitted backing bytes,
  preserve the shared monotonic frontier, materialized-string bytes, quota
  length, and all alias/property/map-key cases
  (`types/str.go:64-95`; `vm/string_inplace_aliasing_test.go:10-89`).
- Prior art/cost: eligible constant-factor remainder, not the dead O(n²),
  refcount, or compiler-alias ideas (`reports/perf-c4-scout.md:245-269`).
  Estimated cost: medium-high.

### C6 — Compact the append-heavy `sliceList` header without changing COW

- Evidence/owner: `sliceList.append` owns 55.64% allocation space/27.89% objects
  (`reports/barn-vm-current-profile-2026-07-15.md:99-114`); owner
  `types/list.go:8-100`.
- Expected rows: `list_append_10k`; no claim is made for the sealed list holdout.
- Killing probe: two `-benchmem` samples of `list_append_10k`, judging B/op and
  allocs/op rather than its noisy timing; kill below a 10% B/op reduction or on
  any allocation-count increase (`experiments/INDEX.md:62,68-70`).
- Prediction: at least 10% lower B/op, unchanged-or-lower allocs/op, and no
  directional timing regression above 3%.
- Guards: cached byte size, 1-based element semantics, shallow element identity,
  frontier-only writes, forced-copy fallback, and all divergence cases
  (`types/list.go:47-100`; `vm/list_inplace_aliasing_test.go:9-63`).
- Prior art/cost: eligible constant-factor remainder after `fb438f5` and
  `30d4288`, not a retry of O(n²) growth (`reports/perf-c4-scout.md:125-133,245-269`).
  Estimated cost: medium-high.

## 5. Ranked non-dominated set

CPU and allocation objectives remain separate; no synthetic combined score is
used.

| Front | Rank | Candidate | Expected campaign effect | Probe / implementation cost | Dominance judgment |
|---|---:|---|---|---|---|
| CPU, broad | 1 | C2 tick metadata | Small broad CPU reduction | low / low | Non-dominated cheapest measured CPU probe. |
| CPU, broad | 2 | C1 cached-frame decode | Medium-to-large broad CPU reduction | low profile / medium-high | Non-dominated on breadth and measured ceiling. |
| CPU, builtin | 1 | C4 single-arg `tostr` | Large allocation reduction plus focused CPU | very low / low | Non-dominated cheapest dual-axis focused candidate. |
| CPU, builtin | 2 | C3 one builtin entry resolution | Small-to-medium focused CPU reduction | low / medium | Non-dominated because it benefits two builtin rows without allocation/representation risk. |
| Allocation, string | 1 | C5 compact `strRep` | Large share of aggregate object/byte churn | medium / medium-high | Non-dominated broad string representation lever, but follows C4 because its probe is costlier. |
| Allocation, list | 1 | C6 compact `sliceList` | Largest aggregate allocation-space owner | medium / medium-high | Non-dominated list-specific lever; assess B/op because timing baseline is noisy. |

Direct stack-op specialization is dominated by C1: it attacks 5.33% flat with
similar VM correctness risk while C1 addresses the larger decode/frame cluster
(`reports/barn-vm-current-profile-2026-07-15.md:60-66`). VM construction/capacity
work is dominated and dead at 0.67% allocation space
(`reports/barn-vm-current-profile-2026-07-15.md:99-104`).

## 6. Excluded from the first round

- C5/C6: allocation evidence is strong, but each requires a representation
  prototype and exhaustive alias guards; cheaper C4 tests the string-allocation
  direction first (`vm/string_inplace_aliasing_test.go:10-89`;
  `vm/list_inplace_aliasing_test.go:9-63`).
- `OP_STRING_APPEND` deletion: blocked on unchanged Toast semantic questions,
  not a performance triage candidate (`reports/perf-c1-scout.md:229-238`).
- Computed-goto/switch replacement, global `Value` shrink, refcount/alias-analysis
  append, smaller VM capacities, and VM pooling: dead for the unchanged causes
  recorded in the eligibility table.
- GC tuning and PGO: blocked until structural candidates change the profile and
  a representative input exists (`reports/perf-c5-profiler.md:204-215`).
- `list_index_1M`-specific collection/layout work: the aggregate profile does not
  isolate its owner, so there is no measured one-surface hypothesis yet
  (`reports/barn-vm-current-profile-2026-07-15.md:137-145`).

## 7. Pre-existing red full gate

The full Go correctness gate remains red at the pre-existing scheduler
ID-collision test `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`;
all other listed packages passed in the campaign frame
(`experiments/INDEX.md:81-97`). This scout did not diagnose, fix, execute, or
promote that failure, and it is not a candidate.

## 8. Recommended first triage round

Use four of the campaign's eight triage probes, in this order. These are future
proposals; this scout did not consume them.

1. **C4, `tostr_200k` two-sample `-benchmem` probe.** Cheapest falsification,
   exact alloc baseline, isolated row, and a predicted ≥150k allocs/op removal
   (`experiments/INDEX.md:65`; `builtins/types.go:27-60`).
2. **C2, isolated `int_arith_1M` CPU profile.** Confirm or kill the current
   3.16% aggregate tick predicate before a tiny source slice
   (`reports/barn-vm-current-profile-2026-07-15.md:62-66`).
3. **C3, isolated `builtin_abs_200k` CPU profile.** Confirm that the remaining
   double entry resolution is material after the already-kept C5 ceremony fix
   (`vm/op_misc.go:36-47`; `builtins/registry.go:384-405`).
4. **C1, isolated `int_arith_1M` decode line profile.** Confirm the aggregate
   `ReadByte`/`ReadShort`/`CurrentFrame` shares remain local-frame reload work
   before paying the medium-high implementation cost
   (`reports/barn-vm-current-profile-2026-07-15.md:60-66`;
   `vm/stack.go:48-62`).

No candidate belongs in `experiments/INDEX.md` until a manager authorizes and a
worker actually executes its triage probe. Neither holdout row may be opened by
these probes (`experiments/INDEX.md:24-41`).
