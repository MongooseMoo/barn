# C5 fix #2 — Cut per-builtin-call ceremony (coder report)

**Date:** 2026-06-27
**Branch:** `perf/c5-builtin` (off master `617505f`)
**Host:** AMD Ryzen 9 5950X (32 threads), Windows, Go amd64

## Mission
Profiler fix #2: every builtin call paid (1) two map lookups in `CallByID`,
(2) an `RWMutex` lock + map lookup in `IsProtectedBuiltin`, and (3) a per-call
validation-wrapper closure (`Register.func1`, 36% cum) — while the real work
(`formatBase10`) was 1.6%. This owned the `tostr_200k` +14.6% post-C2 regression
and all builtin-heavy workloads. All three cut, behavior + error codes identical.

## Changes (file:line)

### 1. `CallByID`: ID-indexed slice, one lookup not two — `builtins/registry.go`
- Added `builtinEntry{name, fn (raw), sig, hasSig, lineSync}` (registry.go ~16).
- `Registry` now has `entries []*builtinEntry` indexed by ID; removed the
  `byID`, `idToName`, `lineSyncByID` maps and the `nextID` counter.
- `Register` (~338) appends one entry at `id = len(entries)`; the funcs map keeps
  the validating closure (see #3 rationale).
- `CallByID` (~376): `if id<0 || id>=len(entries) { return E_VERBNF }` then a
  single slice index — was `r.byID[id]` + `r.idToName[id]` (two `mapaccess`).
- `CallByName` (~388): resolves via `nameToID` then indexes `entries` (one map
  lookup, same as before; behavior identical).
- `NeedsLineSyncByID` (~362): bounds-checked slice index instead of a map lookup;
  out-of-range still returns false (was a missing-key map read → false).

### 2. `IsProtectedBuiltin`: lock-free — `builtins/protected.go`
- Replaced `struct{ sync.RWMutex; set map[string]bool }` with
  `atomic.Pointer[map[string]bool]` (+ `init()` publishing an empty map).
- `IsProtectedBuiltin` (~30): `return (*protectedBuiltins.Load())[name]` — one
  atomic load + read-only map index. No lock, no atomic-add round-trip.
- `LoadProtectedBuiltinsFromStore` (~41): builds a fresh map and publishes it
  with a single `protectedBuiltins.Store(&next)` (both the nil-store and
  populated paths).

### 3. Fold the validation closure into dispatch — `builtins/registry.go`
- `dispatch` (~394) now takes the `*builtinEntry` and validates inline:
  `maybeProtectedRedirect(e.name…)` FIRST, then (if `e.hasSig`)
  `validateKnownFunctionArgs(e.name, e.sig, args)`, then `e.fn(ctx, args)`.
- The hot `CallByID`/`CallByName` path no longer calls through the registration
  closure (`Register.func1`); it calls the raw `e.fn` after inline validation.
- The funcs map still stores the validating closure so `Get()`/`Has()`/
  `call_function()` (which invoke the returned fn directly) keep identical
  arg-validation behavior — they are not on the hot path.

## Protectedness mutability determination + lock-free reasoning

**Is the protected set runtime-mutable? YES.** The writer is
`LoadProtectedBuiltinsFromStore`, called from (a) `server.go:157` at boot and
(b) `builtins/system.go:629` inside the `load_server_options()` builtin — so a
running MOO can toggle `$server_options.protect_<name>` and refresh the set at
runtime. `experiments/2026-06-22-builtin-call-performance.md:37` records that a
prior attempt to cache protectedness at *registration* was reverted precisely
because it is database/runtime state. Therefore a fixed per-entry bool is
**incorrect**; the lock-free design must support concurrent writes.

**Approach: `atomic.Pointer[map[string]bool]` snapshot (swap-on-write,
load-on-read).** Memory-model contract: the map a pointer references is never
mutated after `Store`; a refresh builds a brand-new map and atomically swaps the
pointer. `atomic.Pointer.Load` establishes a happens-before edge with the
`Store` that published the map, so a reader observes either the old or the new
fully-initialized map — never a half-built one and never a torn read. Readers
need no lock. This preserves the exact observable semantics of the old RWMutex
version (which builtins are protected, and that a refresh takes effect
atomically) while removing the per-call `RLock` atomics. Validated by a
dedicated concurrent read/write test under `-race` (see below).

## RED → GREEN tests — `builtins/dispatch_characterization_test.go`

Characterization tests pin the observable behavior + error codes. All written
first and confirmed GREEN on the unmodified base code, then kept GREEN through
the refactor:

- `TestCallByIDInvalidIDErrors` — id `-1`, `1<<30`, `1<<20` → `E_VERBNF`.
- `TestCallByIDValidatesArgCountAndType` — `sqlite_close()`/`(1,2)` → `E_ARGS`;
  `sqlite_close("x")` → `E_TYPE` (validation runs before the body).
- `TestCallByNameValidatesArgs` — same validation via name; unknown → `ok=false`.
- `TestProtectedBuiltinNonWizardDeniedWizardFallsThrough` — protected `abs`,
  `this != #0`, no `#0:bf_abs`: non-wizard → `E_PERM`; wizard → falls through and
  runs the real builtin (`abs(-5)==5`).
- `TestProtectedBuiltinRedirectsToWrapperVerb` — with `#0:bf_abs` present, the
  call redirects through the verb caller and returns its result.
- `TestProtectedBuiltinThisZeroRunsRealBuiltin` — `this == #0` always runs the
  real builtin even when protected.
- `TestProtectedBuiltinConcurrentReadWrite` — 8 reader goroutines hammering
  `IsProtectedBuiltin` while a writer swaps the snapshot; asserts no data race
  under `-race`.

Protected state is driven through the real public loader
(`LoadProtectedBuiltinsFromStore` against a store whose `#0.server_options`
carries `protect_abs=1`), so the tests exercise the actual writer/reader contract
and survive the refactor unchanged.

## Gate output (raw)

### `go test ./builtins -count=1`
```
ok  	barn/builtins	1.001s
```

### `go test ./builtins -race -count=1`
```
ok  	barn/builtins	1.158s
```

### `go test ./vm -count=1`
```
ok  	barn/vm	1.116s
```

### Conformance (managed mode)
```
uv run --project ../moo-conformance-tests moo-conformance \
  --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
...
================ 3988 passed, 131 skipped in 164.85s (0:02:44) ================
```
**3988 passed / 131 skipped / 0 failed** — exact target, no regression.

### benchstat (C0 baseline `perf-baseline-vm-20260627.txt` vs `perf-c5-builtin-after.txt`, count=10)

```
                        │   baseline    │           after            │
                        │    sec/op     │   sec/op      vs base      │
VM/int_arith_1M-32          59.33m ± 6%    52.90m ±34%  -10.84% (p=0.000)
VM/float_arith_1M-32        68.67m ± 3%    53.06m ±16%  -22.73% (p=0.000)
VM/string_concat_10k-32     1.331m ±11%    1.004m ±24%  -24.60% (p=0.002)
VM/list_append_10k-32       2.225m ±15%    1.724m ±15%  -22.49% (p=0.004)
VM/list_index_1M-32         139.2m ±15%    143.6m ± 6%        ~ (p=0.075)
VM/builtin_abs_200k-32      30.15m ± 1%    20.47m ±12%  -32.10% (p=0.000)   <-- target DOWN
VM/tostr_200k-32            57.57m ± 1%    53.18m ± 5%   -7.64% (p=0.000)   <-- target DOWN, < baseline
VM/nested_1k-32             55.65m ±10%    45.08m ± 9%  -19.00% (p=0.000)
VM/list_iter_1M-32          95.15m ±10%    74.32m ± 7%  -21.89% (p=0.000)
geomean                     29.20m         23.90m       -18.15%
```

**Gate targets met:**
- `tostr_200k`: **-7.64%** (p=0.000), now **53.18m < 57.57m C0 baseline** — the
  +14.6% post-C2 regression is **resolved to below baseline**.
- `builtin_abs_200k`: **-32.10%** (p=0.000) — down hard.
- **No significant sec/op regression.** `list_index_1M` shows +3% but
  `p=0.075` (benchstat marks it `~`, not significant); every other workload is
  faster. allocs/op are flat-or-down everywhere (the `list_append_10k` +5.37%
  B/op is a pre-existing C2-era artifact unrelated to builtin dispatch — its
  allocs/op are -66%).

**Caveat (honest):** the locked baseline is the **C0** snapshot, so these deltas
are *cumulative* over C1+C2+C5#1+C5#2, not this commit in isolation. The fix-#2
specific result is the one the gate asks for: `tostr`/`builtin_abs` are down and
`tostr` is now ≤ baseline (regression retired). The dispatch-ceremony removal is
the remaining lever for builtin-heavy workloads after the C2 de-box.

## Process note
The first benchmark run was launched in the background; it died partway (file
froze mid-`string_concat_10k`, no writer process alive — likely killed while
competing with the conformance run). I did **not** run benchstat on the
truncated data; I re-ran the full benchmark in the foreground (exit 0, 120.97s,
all 9 workloads) and used that. No disk failure occurred.

## Commit
`perf(c5): cut per-builtin-call ceremony` — commit **`e4ede2d`** on branch
`perf/c5-builtin` (parent `617505f`). This report's hash annotation was added in
the immediate follow-up doc commit.
