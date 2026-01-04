# Task: Detect Divergences in Math Builtins

## Context

We need to verify Barn's math builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all math builtins.

## Files to Read

- `spec/builtins/math.md` - expected behavior specification
- `builtins/math.go` - Barn implementation

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

From the spec, test these builtins thoroughly:

### Basic Arithmetic
- `abs()` - edge cases: MIN_INT, 0, negative, float
- `min()` / `max()` - edge cases: single arg, mixed types, empty
- `random()` - error conditions (max <= 0, min > max)
- `frandom()` - error conditions

### Integer Operations
- `floatstr()` - precision bounds, scientific notation
- `ceil()` / `floor()` / `trunc()` - negative values, zero, at boundaries

### Trigonometric
- `sin()` / `cos()` / `tan()` - at asymptotes, special values
- `asin()` / `acos()` / `atan()` - domain errors (|x| > 1)
- `sinh()` / `cosh()` / `tanh()`

### Exponential/Logarithmic
- `sqrt()` - **IMPORTANT**: negative argument (E_FLOAT vs E_INVARG?)
- `exp()` - overflow
- `log()` / `log10()` / `log2()` - zero, negative values
- `cbrt()` - negative values

### Bitwise
- `bitand()` / `bitor()` / `bitxor()` / `bitnot()` - negative values
- `bitshl()` / `bitshr()` - large shift counts, negative counts

### Special
- `floatinfo()` / `intinfo()` - structure of returned list
- `isinf()` / `isnan()` / `isfinite()` - with various inputs

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'sqrt(-1)'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return sqrt(-1);"

# Check conformance tests
grep -r "sqrt\|abs\|floor\|ceil" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-math.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY builtin listed above
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
