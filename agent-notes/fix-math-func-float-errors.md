# fix-math-func-float-errors

## Goal

Resolve Barn issue #1096: the 18 functions Toast's `MATH_FUNC` macro defines
(sqrt, cbrt, sin, cos, tan, asin, acos, sinh, cosh, tanh, acosh, asinh, atanh,
exp, log, log10, ceil, floor) give Toast's errors (overflow, underflow, poles,
domain) and Toast's results at the edges Go's math gets wrong. Barn branch
`fix/math-func-float-errors`; suite branch `test/math-func-float-errors`.

## State

Reworked to Q's design (errors only, no port). The Barn branch was rebuilt
from origin/master: it contains no glibc-derived code or data. Committed
locally on both branches (Barn: one commit on origin/master; suite: two
commits), nothing pushed. Gate results below.

### How Toast decides (origin/master `src/numbers.cc:553`)

`MATH_FUNC(name)`: `errno = 0`, call libm, `EDOM` -> E_INVARG, other errno or
`!IS_REAL(result)` (`IS_REAL(x)` is `-DBL_MAX <= x <= DBL_MAX`,
`src/include/my-math.h:24`) -> E_FLOAT, else the float. The oracle binary
`/root/src/toaststunt/build-release/moo` links Debian glibc 2.36's libm.

### Observation sweep (2026-11-21)

18 functions x 325 arguments (special values plus a 250-value sweep), plus
1500 random arguments for 11 of them, run as
`{v = \`f(x) ! ANY', typeof(v) == FLOAT ? floatstr(v, 17, 1) | 0}` through Toast
emergency mode (`moo -e`, stock build) and `bin/barn eval` on the suite's
Test.db. Classes where unchanged Barn differed:

- Overflow: sinh and cosh returned `inf.0` for |x| > 710.4758600739439
  (Toast E_FLOAT). The issue's case.
- Early overflow: sinh/cosh at 709.5 <= |x| <= 710.47 returned `inf.0`; Toast
  is finite (`cosh(710.4)` = 1.66636428328065e+308).
- Underflow: `exp(-745.0)` was E_FLOAT; Toast 4.94065645841247e-324.
- Subnormal arguments: `log(1e-310)` Barn -709.085081548895, Toast
  -713.801378828154; `log10(1e-310)` Barn -307.951738116602, Toast -310.0.
- Last displayed digits: many functions (see "Unfiled findings").
- Domain errors, poles, negative zero: already matched.
- NaN and infinite arguments: not producible on Toast (`tofloat("inf")`,
  `tofloat("nan")`, `tofloat("1e400")`, `parse_json("1e400")` are E_INVARG;
  `1e308 * 10.0` is E_FLOAT; `1e400` is a parse error); Barn agrees.

### What the Barn change does

- `mathFuncResult` (`builtins/math.go`) mirrors `MATH_FUNC` for all 18
  builtins: NaN result -> E_INVARG, infinite result -> E_FLOAT. exp also
  returns E_FLOAT for a zero result (underflow). The per-function range checks
  it replaces gave the same errors.
- `builtins/math_edges.go`, written from the mathematics:
  - `mathLog`, `mathLog10`: for a positive subnormal x, `math.Frexp` gives
    x = m * 2^e with m in [0.5, 1) (Frexp normalises subnormals), so
    ln(x) = ln(m) + e*ln(2) and log10(x) = log10(m) + e*log10(2), with
    log10(2) as the constant `math.Ln2 / math.Ln10`. Other arguments go to
    `math.Log` / `math.Log10` unchanged.
  - `mathExp`: keeps Barn's existing `math.Exp2(x*math.Log2E)` (it is finite up
    to x*log2(e) < 1024, x about 709.78, the true threshold), and for a result
    below 2^-1022 uses `math.Exp`, which is gradual in the subnormal range
    (`math.Exp(-745)` = 5e-324, `math.Exp(-746)` = 0) where Exp2 reached zero
    early.
  - `mathSinh`, `mathCosh`: for |x| > 709, sinh(x) = sign(x)*(e^|x| -
    e^-|x|)/2 and e^-|x| is below 2^-2000 of e^|x|, so both equal e^|x|/2 to
    double precision; computed as (e^(|x|/2)/2) * e^(|x|/2), where |x|/2 is
    exact and far from Go's early exp overflow, so the product overflows only
    past the float range. Below 709 they use `math.Sinh` / `math.Cosh`.
- `internal/fuzzseed/structured.go`: `floatLiterals` gains 710.4, 745.0 and
  1e-310.

### Go's own math at the edges (amd64)

Command (scratch `e1/main.go`, `go run`, `go version go1.27.1 windows/amd64`;
an earlier probe on go1.26.0 printed the same values):

```go
fmt.Println(math.Log10(1e-310), math.Log(1e-310), math.Exp(709.5), math.Exp(709.4))
```

Output:

```
-307.9517381166022 -709.0850815488945 +Inf 1.2260423226426727e+308
```

Toast: `log10(1e-310)` = -310.0, `log(1e-310)` = -713.801378828154,
`exp(709.5)` = 1.35498631931463e+308.

Responsible Go sources (`$(go env GOROOT)/src/math`):

- `log_amd64.s:34-45` (`archLog`, selected by `log_asm.go:9`
  `haveArchLog = true`): the exponent is `(bits>>52)&0x7FF - 0x3FE` and the
  raw mantissa is ORed with 0.5, with no subnormal normalisation, so a
  subnormal input is treated as if its exponent were -1022. `log10.go:16-18`
  computes `Log(x) * (1/Ln10)` and inherits the error.
- `exp_amd64.s:56` and `:99-103` (`archExp`, `exp_asm.go:9`
  `haveArchExp = true`): `k = round(x*log2(e))`; the `ldexp` step returns +Inf
  when `k + 0x3FF >= 0x7FF`, i.e. k >= 1024, from x of about 709.44, although
  `fr * 2^k` with fr < 1 is still finite.

### Gates (2026-11-21, reworked branch)

- Managed stock WSL Toast, suite branch (`make conformance-toast K=mathfunc_`):
  `66 passed, 14180 deselected` (65 rows + capability_admission).
- Managed Barn, reworked branch, `-k "capability_admission or (mathfunc_ or
  math)"`: `276 passed, 2 skipped, 13968 deselected` (new rows plus every
  existing math suite row).
- gofmt clean; `go vet ./...` rc 0; `staticcheck ./...` rc 0;
  `golangci-lint run ./...` "0 issues."; `go build ./...` rc 0;
  `go test ./...` every package ok, rc 0.

## Unfiled findings: last-digit differences from Toast's libm

Removed from the suite PR (Toast-run values; Barn values from the reworked
build). Go's math and glibc's libm round differently; Q's design keeps Go's
results here.

| Input | Toast | Barn |
|---|---|---|
| `cbrt(709.79)` | 8.92024177054545 | 8.92024177054546 |
| `cbrt(-745.2)` | -9.06617884726421 | -9.06617884726422 |
| `cbrt(-0.3)` | -0.669432950082169 | -0.66943295008217 |
| `exp(709.78)` | 1.79282279439452e+308 | 1.79282279439455e+308 |
| `exp(132.78155163351704)` | 4.63762013563591e+57 | 4.63762013563593e+57 |
| `exp(398.88134969770834)` | 1.70596103654619e+173 | 1.70596103654613e+173 |

Not last-digit only (Barn's `Exp2(x*Log2E)` loses accuracy as |x| grows,
since the rounding error of the product is multiplied by x), also removed
and open for a decision:

| Input | Toast | Barn |
|---|---|---|
| `exp(700.0)` | 1.014232054735e+304 | 1.01423205473497e+304 |
| `exp(450.7359251584104)` | 5.65099719667398e+195 | 5.65099719667416e+195 |
| `exp(709.7)` | 1.65498402768026e+308 | 1.65498402768031e+308 |
| `exp(709.5)` | 1.35498631931463e+308 | 1.35498631931457e+308 |

Measured over 1825 exp rows (325 sweep + 1500 random in (-746, 710) and wide
magnitudes): `Exp2(x*Log2E)` differs from Toast in the 15 displayed digits on
914, `math.Exp` on 20 (one of them `exp(709.78)`, Go's early overflow).
`math.Exp` would break the existing conformance row `tostr(exp(1.5))`
(`audit/divergence_introspection_runtime.yaml:90`): Toast 4.48168907033806,
Go `math.Exp` 4.481689070338065 -> 4.48168907033807; the Exp2 form was chosen
for that row. Kept as is in this branch.

Other last-digit differences seen in the sweep (not in the suite PR, never
were): sin 7, cos 5, tan 6, asin 4, acos 1, tanh 4, asinh 1, log 4 (three of
them subnormal, e.g. `log(3.8286538584e-313)` Toast -719.366620838733, Barn
-719.366620838732), log10 12, sinh 15, cosh 15 (e.g. `sinh(-709.5)` Toast
-6.77493159657316e+307, Barn -6.77493159657317e+307), cbrt 66 of 1825 rows
(raw lists in the scratch sweep files, not committed).

## Decisions and why

- Q, 2026-11-21 (via the lead): "Errors only, no port". No glibc-derived code
  goes into Barn. An earlier version of this branch ported glibc 2.36's exp,
  log, log10, sinh, cosh, tanh, asinh, acosh, atanh, cbrt, expm1 and log1p and
  matched the oracle's libm bit for bit; it was removed and the branch rebuilt
  from origin/master, so it is not in the history.
- Go's math stays for ordinary results; Barn's own code covers only the two Go
  defects (subnormal log/log10, early exp overflow for sinh/cosh) and exp's
  subnormal range.
- exp keeps `Exp2(x*Log2E)` in its ordinary range because an existing
  conformance row (exp(1.5)) depends on it; replacing it with `math.Exp` is the
  open decision above.
- Conformance rows whose expected value Go's math cannot meet in the displayed
  digits were removed from the suite PR, not weakened or skipped, and listed
  above.
- Products in the edge helpers are wrapped in `float64(...)` so GOAMD64=v3 does
  not fuse them (linux v3 and v1 builds gave the same sweep results).
- Fuzz: `floatLiterals` gains 710.4, 745.0, 1e-310 so generated programs reach
  the sinh/cosh/exp overflow boundary, exp's subnormal range (with unary
  minus) and subnormal log/log10 arguments. 3.0e300 already reached sinh/cosh
  overflow (that is how #986 found the issue). `scripts/fuzz_compute.cjs` is a
  workflow scheduler and generates no programs.

## Surprises

- `parse_json("1e-400")`: Toast E_INVARG, Barn 0.0. Outside this issue.
- Go's amd64 assembly `math.Exp` overflows early and `math.Log` mishandles
  subnormals (sources above).
- Go's `math.Tan` gave one more display mismatch with Toast in the GOAMD64=v3
  linux build than in v1 (Go fuses inside its math package under v3).

## Next step

Lead: decide the exp ordinary-range question; then push both branches and
open the PRs below (conformance first).

## Draft PR bodies

### moo-conformance-tests: `test/math-func-float-errors`

Title: Pin MATH_FUNC float errors and edge results

Adds `builtins/math_func_float_errors.yaml`: 65 rows for the 18 functions
ToastStunt defines with `MATH_FUNC` (`src/numbers.cc`): overflow
(`sinh(3e300)`, `cosh(710.48)`, `exp(710.0)`) and underflow to zero
(`exp(-746.0)`) are E_FLOAT; poles (`log(-0.0)`, `log10(-0.0)`,
`atanh(-1.0)`) are E_FLOAT; domain edges (`sqrt(-1e-300)`,
`asin(1.0000000000000002)`, `acosh(0.9999999999999999)`) are E_INVARG; finite
results just below the sinh/cosh overflow threshold (`cosh(710.4)` =
1.66636428328065e+308), subnormal exp results (`exp(-745.0)` =
4.94065645841247e-324), subnormal log/log10 arguments (`log10(1e-310)` =
-310.0), large arguments (`sin(1e22)`, `cos(1e300)`) and negative zero
(`toliteral(sqrt(-0.0))` = "-0.0").

Expected values were observed on the stock WSL Toast oracle. NaN and infinite
arguments cannot be produced in MOO (`tofloat("inf")`, `tofloat("nan")` and
`parse_json("1e400")` are E_INVARG, `1e308 * 10.0` is E_FLOAT), so no row
covers them.

### barn: `fix/math-func-float-errors`

Title: builtins: MATH_FUNC errors and edge results match Toast

Fixes #1096

`sinh(3e300)` and `cosh(3e300)` returned `inf.0`; Toast raises E_FLOAT. A sweep
of the 18 `MATH_FUNC` builtins against Toast also found: sinh/cosh returned
`inf.0` for 709.5 <= |x| <= 710.47 (Go's amd64 `math.Exp` returns +Inf from x of
about 709.44, `src/math/exp_amd64.s`), `exp(-745.0)` was E_FLOAT instead of the
smallest subnormal, and `log`/`log10` of subnormal arguments were far off
(`log10(1e-310)` gave -307.95, Toast -310.0; `src/math/log_amd64.s` does not
normalise subnormals).

- `mathFuncResult` maps a NaN result to E_INVARG and an infinite one to
  E_FLOAT for all 18 builtins, as `MATH_FUNC` does from errno.
- log/log10 of a subnormal: x = m*2^e via `math.Frexp`, ln(m) + e*ln(2).
- sinh/cosh for |x| > 709: e^|x|/2 as (e^(|x|/2)/2) * e^(|x|/2).
- exp below 2^-1022: `math.Exp`, which is gradual in the subnormal range.
- The structured fuzz generator's float literals gain 710.4, 745.0, 1e-310.

Go's math still rounds some ordinary results differently from Toast's libm in
the last displayed digit; those are listed in
`agent-notes/fix-math-func-float-errors.md` and not changed here.

Conformance: MongooseMoo/moo-conformance-tests `test/math-func-float-errors`.
