# Fix exp displayed digits (issue #1110)

## Goal

Make Barn's `exp()` print ToastStunt's displayed digits (Toast uses host libm
exp, glibc 2.36 on the WSL oracle), keep the #1096 edge behaviour, and
establish what remains different for the other math functions. Constraint
(Q on #1096): no glibc-derived code or data, glibc source not opened, no other
C library ported; code is derived from mathematics or Go's BSD stdlib.

## State

Built and measured. exp, sin, cos, tan, asin, acos, log and log10 are
correctly rounded (`builtins/math_exp.go`, `builtins/math_cr.go`); sinh and
cosh use the correctly rounded exp for |x| >= 22 (`builtins/math_edges.go`).
Suite rows: `builtins/math_displayed_digits.yaml` (41 rows). Gates below.
Committed locally on both branches; nothing pushed.

### Measurement (2026-11-21)

Sweep: 27,732 distinct arguments (seeded generator
`expdigits-gen.py` in the session scratchpad): 8,000 uniform in
[-745.2, 709.8], 6,000 in [-30, 30], 4,000 log-uniform magnitudes
1e-20..709.7 both signs, 2,000 in [-1, 1], 1,000 in the subnormal-result
range [-745.13, -708.39], 1,000 in [700, 709.78], tenths, quarters and
integers across the range, 1,000 tiny magnitudes 1e-300..1e-5, and the edges
(0.0, -0.0, 1.5, 709.78, 709.782712893384, -708.3964185322641, -745.0,
-745.1332191019411, -745.13321910194122, +-1e-300, +-5e-324, the issue's
arguments). Each line evaluated
``{i, v = `exp(X) ! ANY', typeof(v) == FLOAT ? {tostr(v), floatstr(v, 17, 1)} | toliteral(v)}``
(18 significant digits identify the double exactly).

- Toast: stock WSL `/root/src/toaststunt/build-release/moo` (SHA-256
  61dd1799d82f75e33cb9c42fe48bd1966199ad83fda2f6298e3a7f872cc613dd, Debian
  GLIBC 2.36-9+deb12u14), `moo -e -p 0` on a private copy of the suite's
  `_db/Test.db` (2035 bytes, SHA-256
  1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e).
- Barn: `bin/barn eval --db <copy> --log-dir= --file`.
- Correctly rounded reference: mpmath (`uv run --with mpmath`) at 300 and
  600 bits, rounded to nearest-even at quantum max(2^(e-52), 2^-1074); the two
  precisions agree on every argument.
- Go candidates: `go1.27.1 windows/amd64`.

| candidate | displayed-digit mismatches with Toast | exact-double mismatches |
|---|---|---|
| `Exp2(x*Log2E)` (master; `math.Exp` below 2^-1022) | 13,413 | 19,321 |
| `math.Exp` | 202 | 3,045 |
| correctly rounded | 2 | 22 |
| master `bin/barn eval` (same as the first row) | 13,413 | 19,321 |

Toast's (glibc 2.36's) exp differs from the correctly rounded double on 22 of
27,732 arguments, by one ulp each; on 2 of them the 15 displayed digits
differ:

| argument | Toast | correctly rounded |
|---|---|---|
| `exp(478.4707143724247)` | 6.26889512279811e+207 (6.268895122798105e+207) | 6.2688951227981e+207 (6.268895122798104e+207) |
| `exp(11.060668048606715)` | 63619.0382830428 (63619.038283042755) | 63619.0382830427 (63619.03828304275) |

So no exp written from the mathematics can reach zero: reaching it means
reproducing glibc's particular rounding errors, which needs glibc's algorithm.
The correctly rounded exp is the best candidate under the constraint.

Built (`builtins/math_exp.go`, see "Decisions"); `bin/barn eval` of the fixed
branch on the same 27,732 lines: 2 displayed-digit mismatches (the two above,
Barn's `tostr` string compared with Toast's), 22 exact-double mismatches, and
its double equals the mpmath correctly rounded double on every argument.
`exp(1.5)` prints 4.48168907033806.

Benchmark `BenchmarkMathExp` (1024 arguments uniform in [-700, 700],
`go test -bench`, 5 runs, windows/amd64): new 21.56-21.95 ns/op; master's
Exp2 form in the same loop 21.55-21.64 ns/op; `math.Exp` 10.94-11.14 ns/op.

### Other functions: sweep on master behaviour (2026-11-21)

Same method, per-function samples (`expdigits-gen.py FUNC`): sin/cos/tan
10,000 (5,000 in [-10, 10], 2,000 in [-1e6, 1e6], 2,000 log-uniform
1e-10..1e22, multiples of 0.7853981633974483); asin/acos 8,817 ([-1, 1],
1 - 10^-u near both ends, tiny); sinh/cosh 10,000 ([-710.4, 710.4],
[-30, 30], [-1, 1], tiny); tanh 9,000; asinh 9,000 (log-uniform to 1e300);
log 8,972 and log10 9,587 (log-uniform 1e-323..1e308 including subnormals,
(0, 10], 1 +- 10^-u, and powers of ten for log10); cbrt 9,000 (log-uniform
1e-320..1e308 both signs, [-1000, 1000]). Barn = Go's `math` (plus the #1096
edge helpers). "Barn off" = Toast's double is the correctly rounded one and
Barn's is not; "Toast off" = the reverse.

| function | displayed mismatches | Barn off | Toast off | neither | CR candidate's displayed mismatches |
|---|---|---|---|---|---|
| sin | 394 | 394 | 0 | 0 | 0 |
| cos | 426 | 426 | 0 | 0 | 0 |
| tan | 609 | 601 | 1 | 7 | 2 |
| asin | 730 | 729 | 1 | 0 | 1 |
| acos | 1,217 | 1,216 | 0 | 1 | 1 |
| log | 11 | 11 | 0 | 0 | 0 |
| log10 | 104 | 92 | 12 | 0 | 38 |
| sinh | 130 | 105 | 16 | 9 | 70 |
| cosh | 73 | 61 | 12 | 0 | 47 |
| tanh | 93 | 16 | 74 | 3 | 92 |
| asinh | 1 | 1 | 0 | 0 | 123 |
| cbrt | 410 | 28 | 373 | 9 | 398 |

sinh/cosh by region: for |x| >= 22 Toast's |result| equals the correctly
rounded e^|x|/2 in the displayed digits on all 4,922 arguments (doubles differ
on 7, the glibc exp misroundings); Barn's 41 mismatches there come from Go's
`math.Sinh`/`Cosh` using `math.Exp`. Below 22 Toast is not correctly rounded
(sinh: 4,392 of 5,078 arguments differ from e^|x|/2 in the display, as they
must; against the correctly rounded sinh Toast is off on 16 of Barn's 89).

### After the fix (same samples, fixed `bin/barn eval`)

| function | displayed mismatches before | after | after: Barn off | after: Toast off | after: neither |
|---|---|---|---|---|---|
| exp | 13,413 | 2 | 0 | 2 | 0 |
| sin | 394 | 0 | 0 | 0 | 0 |
| cos | 426 | 0 | 0 | 0 | 0 |
| tan | 609 | 2 | 0 | 2 | 0 |
| asin | 730 | 1 | 0 | 1 | 0 |
| acos | 1,217 | 1 | 0 | 1 | 0 |
| log | 11 | 0 | 0 | 0 | 0 |
| log10 | 104 | 38 | 0 | 38 | 0 |
| sinh | 130 | 89 | 64 | 16 | 9 |
| cosh | 73 | 32 | 20 | 12 | 0 |
| tanh | 93 | 93 (unchanged) | 16 | 74 | 3 |
| asinh | 1 | 1 (unchanged) | 1 | 0 | 0 |
| cbrt | 410 | 410 (unchanged) | 28 | 373 | 9 |

For exp, sin, cos, tan, asin, acos, log and log10 Barn's double equals the
mpmath correctly rounded double on every swept argument (checked double by
double). The linux GOAMD64=v3 build (`bin/barn-expdigits-linux-v3`, run in
WSL) prints byte-identical transcripts to the windows build for exp, sin,
cos, tan, asin, acos, log, log10 and cosh; sinh differs in the last bits on
some |x| < 1 arguments, which is Go's own `math.Sinh` (Go fuses inside its
math package under v3, as the #1096 note saw for `math.Tan`), with the same
89 displayed mismatches.

Remaining differences (all with Toast's double not the correctly rounded
one, or Go's sinh/cosh/tanh/asinh/cbrt kept):

- exp: `exp(478.4707143724247)` Toast 6.26889512279811e+207, Barn
  6.2688951227981e+207; `exp(11.060668048606715)` Toast 63619.0382830428,
  Barn 63619.0382830427.
- tan 2, asin 1, acos 1: Toast off by one ulp, e.g.
  `asin(0.45724339199736885)` Toast 0.474893114418887, Barn (correctly
  rounded) 0.474893114418888.
- log10 38: Toast off, e.g. `log10(3.472426992603786)` Toast
  0.540633123511848, Barn 0.540633123511849 (correctly rounded
  0.5406331235118486).
- sinh 89 / cosh 32, all with |x| < 22 (Go's `math.Sinh`/`Cosh`): Barn off
  on 64 / 20, e.g. `sinh(0.7022425930146845)` is "neither" (Toast
  0.761400444987478, Barn 0.761400444987477, correctly rounded
  0.7614004449874775); Toast off on 16 / 12, e.g. `sinh(7.449846364294103)`
  Toast 859.799175876657, Barn and correctly rounded 859.799175876656. A
  correctly rounded sinh/cosh would leave 70 / 47 (Toast is not correctly
  rounded there), so it does not remove the mismatches; not built.
- tanh 93: Toast off on 74, Barn off on 16, neither 3 (correctly rounded
  tanh would leave 92); not built.
- asinh 1: `asinh(2.9896516318465434)` Toast 1.8151689282752, Barn
  1.81516892827519; a correctly rounded asinh would leave 123 (Toast's and
  Go's asinh agree with each other and not with the correctly rounded value),
  so not built.
- cbrt 410: Toast off on 373 (Go's `math.Cbrt` is the correctly rounded one
  there), Barn off on 28, neither 9; correctly rounded would leave 398; not
  built.

### Gates (2026-11-21)

- Managed stock WSL Toast, suite branch, `make conformance-toast
  CONFORMANCE_SUITE=<suite worktree> K="_digits_"`: 42 passed (41 rows +
  capability_admission).
- Unfixed Barn (origin/master builtins via `go build -overlay`, linux
  binary), CI form with `-k "capability_admission or _digits_ or math or
  exp_float"`: all 41 new rows FAIL; 212 passed, 2 skipped (every existing
  math row, including `exp_float_formatting_fifteen_digits`, passes).
- Fixed Barn, full managed suite with the suite branch (CI form, no -k):
  13,931 passed, 327 skipped, rc 0.
- gofmt clean; `go vet ./...` rc 0; `staticcheck ./...` rc 0;
  `golangci-lint run ./...` "0 issues."; `go build ./...` rc 0;
  `go test ./...` 40 packages ok, rc 0.
- `go test ./vm -bench=BenchmarkVM -ldflags=-funcalign=64`, master overlay vs
  fixed, 2 x 3 runs interleaved (CPU load 31% from other work): geomean
  +0.92%, spreads up to +-13%; the two rows with p < 0.05 (string_concat_10k
  +14%, nested_1k +2.4%) rerun 5 x 2 interleaved: p = 0.165 and 0.529, no
  difference. No VM workload calls a math builtin; the package's init adds
  0.5-1.1 ms per process.

### Per-call cost through the VM, first build (2026-11-21, after lead review)

`BenchmarkVMMath` (new, `vm/perf_bench_test.go`): 200k MOO iterations of
`y = y + step; x = f(y);` per builtin, plus `float_abs` as the loop
baseline. Master = origin/master builtins via `go build -overlay`; both with
`-ldflags=-funcalign=64`; 3 rounds of `-count=2` interleaved; CPU load at
the three rounds 63%, 51%, 54% (other sessions). benchstat sec/op:

| workload | master | first build | delta |
|---|---|---|---|
| float_abs_200k | 30.41m | 30.37m | ~ (p=0.699) |
| exp_200k | 38.48m | 40.65m | +5.6% (p=0.015) |
| log_200k | 40.07m | 40.88m | ~ (p=0.394) |
| log10_200k | 38.42m | 40.86m | +6.4% (p=0.002) |
| sin_200k | 37.51m | 45.73m | ~ (p=0.065, master +-35%) |
| cos_200k | 37.59m | 46.03m | +22.4% (p=0.002) |
| tan_200k | 36.97m | 48.02m | +29.9% (p=0.002) |
| asin_200k | 38.68m | 48.98m | +26.6% (p=0.002) |
| acos_200k | 39.05m | 47.90m | +22.7% (p=0.002) |

A MOO iteration of the loop costs about 150 ns, so a 30-50 ns builtin
difference shows.

### Can Go's result be accepted by a cheap bound? (lead's first route)

No. Read in `$(go env GOROOT)/src/math` (go1.27.1):

- `sin.go:36-41` (sin and cos) and `tan.go:33-37` state only peak relative
  errors observed over random trials (2.1e-16, 2.9e-16 over +-1.07e9), not a
  proven bound; `asin.go` and `log10.go` state no accuracy at all.
- `log.go:63-65` and `exp.go:84-86` claim "less than 1 ulp" from an error
  analysis, but that is the pure-Go code; amd64 runs the assembly
  `archLog`/`archExp` (`log_asm.go`, `exp_asm.go`), which has no stated
  bound (and `archLog` is wrong on subnormals).
- Even a proven bound of 1/2 + e ulp on the returned double gives no
  information about where the true value sits relative to the rounding
  midpoints, so the returned double alone can never prove that it is the
  correctly rounded one. A first test needs an error estimate computed
  alongside the result, which is what the double-double path is.

So the route taken is to make the double-double path itself cheap (next
section).

### Making the paths cheaper (2026-11-22)

Step 1, cutting work in the existing double-double paths (sin/cos compute
only the value needed, log without divisions via a 128/j reciprocal table
and a two-product, tan with one division, asin/acos by a table grid in y
instead of Go's asin plus a Newton step), `BenchmarkMathCR` ns/op: log 27.0
(was 29.8), log10 31.2 (31.6), sin 46.4 (45.4), cos 47.4 (45.9), tan 64-82
(55.7; noisy run), asin 51.0 then 36.6 (65.7), acos 48-53 then 36.5 (66.3).
sin/cos did not move: a profile showed the time is latency along serial
two-sums and long tail sums, not the number of values computed.

Step 2, a lean first level (`builtins/math_cr_fast.go`, about 2^-65
instead of 2^-75: the leading product and sum exact, the smaller terms in
float64 without renormalising) with its own Ziv test, falling back to the
2^-75 path and then math/big. A prototype sin first level fell back on 60
of 200,000 arguments in [-10, 10]; its first version also returned a wrong
double that passed its test (the reduction tail's second-order term
`rl (cos t - 1)` and `S t rl` were missing from value and bound), fixed
before use. tan's first version divided by the leading part of an
unnormalised cosine (error 2^-30), caught by the fast-vs-slow test, fixed
by renormalising both parts first. `BenchmarkMathCR` ns/op (Barn / Go):
exp 18.3 / 15.3 (master's Exp2 form 21.6), log 19.8 / 16.0, log10 23.9 /
16.6, sin 23.3 / 13.2, cos 23.8 / 13.2, tan 33.0 / 15.3, asin 36.5 / 16.2,
acos 36.5 / 16.4.

`BenchmarkVMMath`, master vs this build, 3 rounds x 2 interleaved,
funcalign=64, CPU load 7%, 14%, 5%: float_abs ~ (p=0.132), exp ~ (p=0.937),
log ~ (p=0.937), log10 +5.6%, sin +6.7%, cos +4.1%, tan +11.1%, asin +12.3%,
acos +10.8% (all p=0.002). One MOO call of a math builtin costs about 47 ns
over the float_abs loop on master.

Step 3, asin/acos first level with per-interval Taylor coefficients
(`asinTable`: for y_i = i/64, asin(sin y_i + d) = y_i + a1 d + ... + a12 d^12,
a1 = 1/cos y_i and the rest from the recurrence of (1-x^2) f'' = x f',
computed at init with math/big), no square root or division for
|x| <= 1/2; its first version dropped the d^2 cross term in the argument's
low part (2^-62.6, caught by the fast-vs-slow test), fixed. One merged
sin/cos table row, the reduction tail not renormalised while |k| <= 2^14,
and table indices and k read from the bits of v + 1.5*2^52 instead of
float-int-float conversions. `BenchmarkMathCR`, gated on CPU load (19%
before, 9% after), ns/op Barn / Go: exp 18.8 / 14.9 (master's Exp2 form
21.6), log 21.2 / 15.1, log10 25.5 / 15.4, sin 23.1 / 13.7, cos 23.5 /
13.5, tan 34.8 / 15.6, asin 29.7 / 16.4, acos 29.0 / 16.8. The sweep
transcripts of this build (exp, sin, cos, tan, asin, acos, log, log10, sinh,
cosh) are byte-identical to the verified ones.

### Table construction at first use (lead item 3)

Tried: `sync.Once` around `initMathTables`, called at the top of each of the
eight functions (the fast path is one inlined atomic load). A/B against the
init version (overlay), 3 rounds x `-count=3` interleaved, CPU load 100%,
50%, 59% (could not get a quiet machine): exp ~ (p=0.982), log ~ (0.339),
log10 -0.9% (0.023), sin +1.8% (0.005), cos ~ (0.258), asin ~ (0.214),
acos +4.8% (0.000), geomean +1.2%. That is measurable per call, so the
tables stay at package init (0.5-1.1 ms once per process).

### sin/cos/tan at |x| >= 2^30 (lead item 5)

Before (math/big path for every such argument), `-benchtime=2000x`, load
62%: sin 31.3 us, cos 32.3 us, tan 29.4 us for 2^30..2^40; 33.8 / 34.0 /
39.5 us for 2^40..2^100; 146.8 / 139.3 / 138.9 us for 2^100..2^1023 (Go's
`math.Sin` 22-25 ns). A defect on the same path, so fixed:
`builtins/math_trig_reduce.go` reduces |x| >= 2^30 by Payne and Hanek's
method (written from the method: the 53-bit mantissa times a 192-bit window
of 2/pi's bits, quadrant and 190 fraction bits; the bits of 2/pi are
computed with math/big on the first such argument, not at init), then the
2^-75 path and Ziv's test; math/big only when the reduced argument is below
2^-37 or the test fails. After, load 100%: sin 86 / 91 / 295 ns, cos 86 /
92 / 189 ns, tan 107 / 403 / 383 ns for the three ranges (Go: sin 22 / 25
/ 72 ns, tan 27 / 40 / 154 ns at the same load). The fast-vs-slow test now
also draws sin/cos/tan arguments log-uniform in 1e9..1e308, and the sweep
transcripts (350 sin arguments at 1e9 and above) are identical.

## Decisions and why

- Correctly rounded rather than anything else: it is the only target that
  can be written from the mathematics without reproducing glibc's own
  rounding errors, and on the sweep it matches Toast's double on all but 22
  of 27,732 exp arguments (2 in the display). Not reachable under the
  constraint: the remaining 2 (they are glibc's misroundings).
- Lead's step 4 ("fix where Barn is off and a fix removes the mismatches"):
  sin, cos, tan, asin, acos, log and log10 were Barn-off on nearly every
  mismatch, and correctly rounded versions remove them (log10 leaves 38
  where Toast is off). sinh/cosh for |x| >= 22 equal e^|x|/2 to 2^-63, and
  Toast there equals the correctly rounded e^|x|/2 on every swept argument,
  so they use the new exp. tanh, asinh, cbrt and sinh/cosh below 22 are kept
  as Go's: Toast is not correctly rounded there, so a correctly rounded
  version does not remove the mismatches (counts in the table above).
- How each piece was derived (no C library source was opened or ported;
  glibc's source was not opened at all):
  - exp: argument reduction x = k ln2/64 + r with k the nearest integer,
    e^x = 2^(k>>6) 2^((k&63)/64) e^r, written from the identity itself (no
    paper was read for this task). x - k*hi(ln2/64) is exact in one FMA
    (argument in `math_exp.go`); e^r - 1 by its Taylor series to degree 8,
    r and r^2/2 exact by two-sum/two-product.
  - log: x = 2^e m, m in [sqrt(1/2), sqrt(2)), F = j/128 nearest m,
    u = (m-F)/F, ln x = e ln2 + ln F + ln(1+u) with the Mercator series to
    u^10; m - F exact, u as a double-double.
  - log10 = ln x times 1/ln 10 as a double-double.
  - sin/cos/tan: x = k pi/2 + r with pi/2 as four float64 terms
    (|x| < 2^30; x - k*P0 exact in one FMA), then r = c + t with c = i/64
    and the angle-addition formulas with sin/cos(i/64) tables and Taylor
    series for sin t, cos t - 1; tan = sin/cos divided in double-double.
    |x| >= 2^30 goes to math/big.
  - asin: one Newton step on sin y = s from Go's `math.Asin`, using the
    double-double sin/cos; for |x| > 1/2, asin x = pi/2 - 2 asin(sqrt((1-x)/2))
    (1 - x and the halving exact). acos: pi/2 - asin x for |x| <= 1/2,
    2 asin(sqrt((1-x)/2)) and pi - 2 asin(sqrt((1+x)/2)) outside.
  - Error-free transformations: two-sum (Knuth), fast two-sum (Dekker),
    two-product with `math.FMA`.
  - Rounding: Ziv's strategy. The fast path's error bound is checked by
    rounding both ends of the error interval; otherwise math/big decides at
    128, 256, ... bits. math/big versions: exp by (e^(y/2^s))^(2^s) and
    Taylor; ln by Newton on exp; sin/cos by reduction with pi from Machin's
    formula and Taylor; asin by Newton on sin; tan = sin/cos; acos = pi/2 -
    asin. The loop ends because these values are transcendental for the
    arguments that reach it (Lindemann-Weierstrass), so never a midpoint.
  - Constants and tables are computed at package init with math/big from
    exact-rational series: ln 2 = 2 atanh(1/3), ln 10 = 3 ln 2 + 2 atanh(1/9),
    pi/2 by Machin, ln(j/128) by steps 2 atanh(1/(2j+-1)), 2^(j/64) by six
    square roots of 2 and products, sin/cos(i/64) by rotation from the
    Taylor series of 1/64. Init takes 0.5-1.1 ms (measured 3 times).
  - Go's standard library was not copied; `math.Asin` is called as the
    Newton starting point and `math.Log`/`math.Frexp` start the math/big
    Newton iteration.
- Products that must not be fused are wrapped in `float64(...)`; fusing the
  others only makes them more accurate, and the Ziv test makes the result the
  same either way (the linux GOAMD64=v3 build printed identical transcripts).
- Suite rows: expected values only from Toast. Each row's argument has
  Toast's double equal to the mpmath correctly rounded double (300 and 600
  bits agree) and the exact value within 0.2 ulp of it (mpmath distance,
  `expdigits-pick.py`), so a libm accurate to better than 0.7 ulp, such as
  ubuntu-24.04's on the suite CI, returns the same double. Each row also
  fails on unfixed Barn. The issue's 700.0, 709.5 and 132.78155163351704
  meet the criteria and are included; 450.7359251584104 (0.44 ulp) and
  709.78 (0.42 ulp) do not and are left out.

## Surprises

- Go's math is off far more often than the #1096 sweep showed once the
  samples are uniform over the natural domains: acos 1,217 of 8,817, asin
  730, tan 609, cos 426, sin 394 displayed mismatches, all but a handful
  with Toast correctly rounded.
- glibc 2.36's exp is not correctly rounded on 22 of 27,732 arguments;
  its asinh agrees with Go's (and not with the correctly rounded value) on
  all but 1.
- mpmath's `cbrt` of a negative number is the complex principal root; the
  reference uses sign(x) cbrt(|x|).
- A Claude Code safety check refused a WSL script that removed a `mktemp`
  directory by variable; the v3 run used a fixed `/tmp/expdigits-v3`
  directory, left in place.

## Next step

Lead: publish (suite PR first; its 41 rows fail on Barn master, so the Barn
PR should merge with or right after it).

## Barn PR body

Title: builtins: correctly rounded exp, log, log10, sin, cos, tan, asin, acos

Fixes #1110

`exp()` printed different digits from ToastStunt on about half of ordinary
arguments: `exp(700.0)` gave 1.01423205473497e+304 where Toast gives
1.014232054735e+304. Barn computed `math.Exp2(x * math.Log2E)`, whose rounded
product loses accuracy as |x| grows. A sweep of 27,732 arguments against the
stock WSL Toast oracle found 13,413 displayed-digit mismatches for that form,
202 for Go's `math.Exp`, and 2 for a correctly rounded exp (Toast's glibc
2.36 exp misrounds 22 of them by one ulp, 2 visibly).

The same sweep method on the other math builtins found Go's math off where
Toast is correctly rounded: sin 394, cos 426, tan 601, asin 729, acos 1,216,
log 11, log10 92 mismatches (8,800 to 10,000 arguments each).

- `builtins/math_exp.go`, `builtins/math_cr.go`: exp, log, log10, sin, cos,
  tan, asin and acos are correctly rounded. Each computes a double-double
  approximation with an error bound (table-driven argument reduction,
  error-free two-sum/two-product with `math.FMA`, Taylor and Mercator
  series) and uses Ziv's rounding test; when the rounding is not decided, and
  for subnormal exp results and |x| >= 2^30 trig arguments, math/big decides
  it. Constants and tables are computed at init from exact-rational series
  (0.5-1.1 ms). No C library code was used or ported.
- `builtins/math_edges.go`: sinh and cosh for |x| >= 22 are e^|x|/2 from the
  new exp (Toast matches the correctly rounded e^|x|/2 on every swept
  argument there).
- After the fix, Barn's double equals the correctly rounded double on every
  swept argument for these eight functions; the displayed mismatches left are
  Toast's own misroundings (exp 2, tan 2, asin 1, acos 1, log10 38). tanh,
  asinh, cbrt and sinh/cosh below 22 keep Go's results: Toast is not
  correctly rounded there. Details and counts:
  `agent-notes/fix-exp-displayed-digits.md`.

Cost per call (`BenchmarkMathCR`, windows/amd64): exp 21.7 ns (old Exp2 form
21.6, `math.Exp` 13.3), log 29.8 (14.1), log10 31.6 (14.7), sin 45.4 (13.3),
cos 45.9 (13.0), tan 55.7 (15.3), asin 65.7 (16.1), acos 66.3 (16.3).

Conformance: MongooseMoo/moo-conformance-tests `test/exp-displayed-digits`
(41 rows, all fail on Barn master).

## Suite PR body

Title: Pin displayed digits of exp, log, log10, trig and sinh/cosh

Adds `builtins/math_displayed_digits.yaml`: 41 `tostr(f(x))` rows for exp
(18, from -598.7 to 709.5, including the arguments in barn#1110: 700.0,
709.5, 132.78155163351704), sin, cos (4 each), tan, asin, acos, sinh, cosh
(2 each), log (1, subnormal argument) and log10 (4).

Expected values were observed on the stock WSL Toast oracle. Each row was
chosen so that Toast's double is the correctly rounded one (mpmath at 300
and 600 bits) and the exact value lies within 0.2 ulp of it, so any C
library accurate to better than 0.7 ulp returns the same double.
