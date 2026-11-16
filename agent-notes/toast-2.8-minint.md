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
- One int/int path for every profile. The Mongoose fork
  (`/root/src/toaststunt-mongoose`, at 72e3c7f) predates a32843c and cdd6d38:
  both its builds still return `MININT` for `MININT / -1`, would trap on
  `INT64_MIN / -1`, use the old `(n % d + d) % d` modulus, and its
  non-PROMOTE `do_power` has the inverted `(-1) ^ n` sign. Barn follows stock
  2.8 and does not model those. The PROMOTE_NUMBERS negative-exponent float
  path is unchanged and still matches the fork.

## Surprises

- `BenchmarkVM/list_iter_1M` fails on origin/master before this change with
  `compile failed: [Line 1:  syntax error]` (`vm/perf_bench_test.go`). Not
  touched here. No `BenchmarkVM` case exercises `/` or `^`.

## Next step

Publish both branches (conformance first, then Barn).
