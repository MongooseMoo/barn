# Spec Patch Batch 1 - Summary

**Date:** 2026-01-03
**Based on:** divergence-math.md, divergence-strings.md, divergence-lists.md

## Changes Made

### 1. spec/builtins/math.md

**Removed:** Section 5 "Bitwise Operations" (entire section with 6 functions)
- Removed: bitand, bitor, bitxor, bitnot, bitshl, bitshr
- Reason: These are language operators, not builtins (implemented in VM evaluator)
- New home: operators.md (see below)

**Marked ToastStunt-only sections:**
- Section 5 "Special Values" → "Special Values (ToastStunt)"
  - floatinfo(), intinfo()
- Section 6 "Advanced Math" → "Advanced Math (ToastStunt)"
  - cbrt, log2, hypot, fmod, remainder, copysign, ldexp, frexp, modf, isinf, isnan, isfinite

**Renumbered sections:**
- Old Section 5 (Bitwise) → removed
- Old Section 6 (Special Values) → Section 5
- Old Section 7 (Advanced Math) → Section 6
- Old Section 8 (Error Handling) → Section 7

### 2. spec/operators.md

**Added:** Function forms for bitwise operators (section 8)
- Added function form documentation for: bitor(), bitxor(), bitand(), bitnot()
- Added function form documentation for: bitshl() [ToastStunt], bitshr() [ToastStunt]
- Added examples showing both operator and function forms
- Clarified error conditions (E_INVARG for negative shift counts)

**Reasoning:** Bitwise operations are available both as operators (`&.`, `|.`, `^.`, `~`, `<<`, `>>`) and as callable functions. The operators belong in operators.md, but the function forms should also be documented there since they're just alternate syntax for the same VM operations.

### 3. spec/builtins/lists.md

**Marked verb implementations (not builtins):**
- Section 2.3: indexc → [Verb]
- Section 4.2: slice → [Verb]
- Section 4.3: rotate → [Verb]
- Section 5 heading: "Set Operations" → "Set Operations (MOO Verbs - Not Builtins)"
  - 5.1 intersection → [Verb]
  - 5.2 union → [Verb]
  - 5.3 diff → [Verb]
- Section 6 heading: "Aggregation" → "Aggregation (MOO Verbs - Not Builtins)"
  - 6.1 sum → [Verb]
  - 6.2 avg → [Verb]
  - 6.3 product → [Verb]
- Section 7 heading: "Searching" → "Searching (MOO Verbs - Not Builtins)"
  - 7.1 assoc → [Verb]
  - 7.2 rassoc → [Verb]
  - 7.3 iassoc → [Verb]
- Section 8 heading: "Utility" → "Utility (MOO Verbs - Not Builtins)"
  - 8.1 make_list → [Verb]
  - 8.2 flatten → [Verb]
  - 8.4 count → [Verb]

**Added clarifying notes:**
- Sections 5-8: Added note explaining these return E_VERBNF without verb implementations
- Section 8.3 (unique): Added note clarifying this IS a ToastStunt builtin

**Reasoning:** Testing revealed these functions are NOT implemented as builtins in either Toast or Barn - they return E_VERBNF. They're provided as MOO verb implementations in standard databases. Only `unique()` and core list operations (listappend, listinsert, etc.) are actual builtins.

### 4. spec/builtins/strings.md

**No changes made.**

Divergence report showed no issues - spec is accurate for implemented builtins.

## Summary Statistics

**Files modified:** 3
**Files unchanged:** 1
**Sections removed:** 1 (bitwise operators from math.md)
**Sections added:** 0 (added to existing section in operators.md)
**Functions reclassified:** 16 (15 list verbs + indexc)
**Functions marked ToastStunt-only:** 15 (math advanced functions)

## Verification

Changes are minimal and targeted:
- No new content added beyond what divergence reports documented
- No sections rewritten, only annotations/markers added
- Preserved all existing structure and formatting
- All changes directly justified by divergence report findings

## Key Findings from Divergence Reports

1. **Math:** Bitwise operators are VM operators, not builtins (confirmed by checking registry.go)
2. **Math:** 15 advanced math functions documented but not implemented in Barn (ToastStunt extensions)
3. **Strings:** All implemented builtins work correctly, no spec changes needed
4. **Lists:** 16 "builtin" functions are actually MOO verbs, return E_VERBNF without implementations

## Impact

These changes make the specs more accurate:
- Developers know which functions are builtins vs verbs
- Clear distinction between core MOO, ToastStunt extensions, and verb implementations
- Bitwise operators properly documented as language operators with function call syntax
- Reduces confusion when functions return E_VERBNF (expected for verbs, not builtins)
