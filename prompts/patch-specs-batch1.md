# Task: Patch Specs Based on Divergence Reports (Batch 1)

## Context

Three divergence reports have been generated comparing Barn vs Toast behavior for math, strings, and lists builtins. Your job is to update the specs based on verified findings.

## Input Reports

1. `reports/divergence-math.md` - Status: divergences_found
2. `reports/divergence-strings.md` - Status: clean
3. `reports/divergence-lists.md` - Status: clean

## Spec Files to Potentially Update

- `spec/builtins/math.md`
- `spec/builtins/strings.md`
- `spec/builtins/lists.md`
- `spec/operators.md` (for bitwise operators)

## Actions Required

### 1. Math Spec (`spec/builtins/math.md`)

**Remove bitwise operators section** - they are language operators, not builtins:
- bitand, bitor, bitxor, bitnot, bitshl, bitshr
- These belong in `spec/operators.md`, not `spec/builtins/math.md`

**Mark ToastStunt-only functions clearly** - the following are NOT in core MOO:
- frandom, floatinfo, intinfo, cbrt, log2, hypot, fmod
- remainder, copysign, ldexp, frexp, modf, isinf, isnan, isfinite

### 2. Operators Spec (`spec/operators.md`)

**Add bitwise operators section** if not already present:
- `&` or `bitand(a, b)` - bitwise AND
- `|` or `bitor(a, b)` - bitwise OR
- `^` or `bitxor(a, b)` - bitwise XOR
- `~` or `bitnot(a)` - bitwise NOT
- `<<` or `bitshl(a, n)` - left shift
- `>>` or `bitshr(a, n)` - arithmetic right shift

### 3. Lists Spec (`spec/builtins/lists.md`)

**Mark ToastStunt extensions as verb implementations**:
- The following are NOT builtins in either Toast or Barn:
- intersection, union, diff, sum, avg, product
- assoc, rassoc, iassoc, make_list, flatten, count
- rotate, slice, indexc
- These return E_VERBNF - they're MOO verbs, not builtins

### 4. Strings Spec (`spec/builtins/strings.md`)

**No changes needed** - spec is accurate for implemented builtins.
Possibly add note about Unicode handling (length returns bytes, not chars).

## Process

1. Read each spec file
2. Make minimal, targeted edits based on divergence report findings
3. Preserve existing structure and formatting
4. Add clear [ToastStunt] or [Verb] markers where appropriate
5. Do NOT add new content beyond what's documented in reports

## Output

Write a summary of changes made to: `reports/patch-batch1-summary.md`

## CRITICAL

- Make minimal changes - only what the reports justify
- Do NOT rewrite entire sections
- Do NOT add undocumented features
- Preserve existing formatting style
- Test nothing - only update documentation
