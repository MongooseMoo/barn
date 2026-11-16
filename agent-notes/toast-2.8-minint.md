# toast-2.8-minint

## Goal

Match stock ToastStunt 2.8 (03af4fa) integer division, modulus and power
edges. The conformance test `builtins/math.yaml::minint_division_edge_case_64bit`
expected `-9223372036854775807 / -1` to be `-9223372036854775807`; Toast 2.8
gives `9223372036854775807`.

## State

Done on branch `toast-2.8-minint` (Barn) with the companion
moo-conformance-tests branch `toast-2.8-minint`.

- `vm/op_arith.go`: `executeDiv` divides int/int with plain Go `/`; the
  `MININT` constant and its `/ -1` special case are gone. `executePow` decides
  an int base with a negative int exponent through `intPowNegative`.
- `vm/int_division_edges_test.go` covers the edges; `vm/promote_numbers_test.go`
  `TestPromoteNumbers_PromoteMinIntDivStillInt` now expects the negated result.
- Conformance: the renamed `minint_divided_by_neg_one_negates_64bit` plus new
  division, modulus, negation and `(-1) ^ n` tests pass on WSL Toast 2.8 and on
  Barn through the managed harness.

## Decisions and why

- Toast source, not observation, set the expectations:
  - `src/include/structures.h`: `MININT` is still `-9223372036854775807`; it
    did not change in 2.8.
  - Toast a32843c (`do_divide`): any divisor of -1 returns
    `(Num)(0 - (UNum)a)`, a wrapping negation, so `INT64_MIN / -1` is
    `INT64_MIN`. The Go spec defines `math.MinInt64 / -1` as `math.MinInt64`
    with no panic, so Barn's plain `/` matches and needs no special case.
  - Toast a32843c (`do_modulus`): `d == -1` gives 0, otherwise `n % d` with a
    sign fix-up, with no overflow. Barn's `executeMod` and the `fkMod` fast path
    in `vm/vm.go` already did exactly this, so they are unchanged.
  - Toast cdd6d38 (`do_power`): base -1 with a negative exponent is
    `(b & 1) ? -1 : 1`; base 1 is 1; base 0 is E_DIV; any other base is 0.
    Barn used `int64(math.Pow(a, b))`, which got small exponents right but
    rounded odd exponents beyond 2^53 to even (e.g. `(-1) ^ -9223372036854775807`
    gave 1). `intPowNegative` uses the exact parity.
  - Unary minus (`OP_UNARY_MINUS` in execute.cc) is unchanged; Go negation
    wraps the same way.
- One int/int path for every profile, following stock 2.8 (team-lead
  decision). The WSL checkout `/root/src/toaststunt-mongoose` (72e3c7f) is
  stale. The live fork branch, MongooseMoo/toaststunt `mongoose` (upstream
  2.8.0 merged in fork PR #17), matches stock 2.8 under PROMOTE_NUMBERS:
  `do_divide` uses `(Num)(0 - (UNum)a)` (src/numbers.cc ~482), `do_modulus` uses
  the `(d == -1) ? 0 : n % d` fix-up (~417), and `do_power` with a negative
  int exponent returns float `pow()` (~512-515), which Barn's promote path
  already models. Its non-PROMOTE `do_power` (~732) still has the pre-cdd6d38
  inverted `(b & 1) ? 1 : -1`; Barn's strict path follows stock 2.8 there.

## Surprises

- `BenchmarkVM/list_iter_1M` fails on origin/master before this change with
  `compile failed: [Line 1:  syntax error]` (`vm/perf_bench_test.go`). Not
  touched here. No `BenchmarkVM` case exercises `/` or `^`.

## Next step

Publish both branches (conformance first, then Barn).
