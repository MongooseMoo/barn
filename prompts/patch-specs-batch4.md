# Task: Patch Specs for Batch 4 Divergences

## Context

Divergence detection completed for batch 4 (maps, json, server). JSON is clean. Maps and server need spec patches.

## Reports to Read

- `reports/divergence-maps.md` - Multiple spec errors found
- `reports/divergence-json.md` - CLEAN, no patches needed
- `reports/divergence-server.md` - Some functions not implemented

## Patches Required

### 1. spec/builtins/maps.md

**mapdelete missing key behavior (Section 2.3 area)**
- CURRENT: Spec claims mapdelete returns original map unchanged if key doesn't exist
- CORRECT: Both implementations return E_RANGE for missing keys
- Evidence: Conformance test `mapdelete_missing_key` expects E_RANGE
- ACTION: Update spec to document E_RANGE behavior

**ToastStunt builtins don't exist (Sections 3.1-3.2, 4.1-4.2)**
- mapmerge, mapslice, mklist, mkmap are marked "(ToastStunt)" but don't exist in Toast
- ACTION: Mark all four as `[Not Implemented]` or remove entirely

**`in` operator doesn't work with maps (Section 9)**
- CURRENT: Spec claims `in` operator tests for key presence in maps
- CORRECT: Both implementations return 0 (false) even when key exists
- Evidence: `"a" in ["a" -> 1]` returns 0 on both Toast and Barn
- ACTION: Remove or clearly mark as NOT WORKING

**Key types table wrong (Section 6)**
- CURRENT: Spec claims LIST and MAP can be map keys
- CORRECT: Both implementations reject list/map keys with E_TYPE
- Evidence: `[{1, 2} -> "pair"]` returns E_TYPE on both
- ACTION: Remove LIST and MAP from valid key types table

**DO NOT CHANGE:**
- Key ordering (ERR vs FLOAT position) - this is a likely_barn_bug, spec follows Toast
- Map indexing behavior - Toast is broken, Barn is correct, spec documents intended behavior

### 2. spec/builtins/server.md

**Functions not implemented (add [Not Implemented] markers)**
- memory_usage() - Not implemented in either server
- buffered_output_length(player) - Not implemented in either server
- reset_max_object() - Not implemented in either server
- renumber(object) - Not implemented in either server

**DO NOT CHANGE:**
- connected_players() documentation - Barn has a bug, spec is correct

### 3. spec/builtins/json.md

**NO CHANGES NEEDED** - Report is clean, all 40+ behaviors match

## Style Guide

For [Not Implemented] markers, follow the pattern from time.md:

```markdown
### function_name() [Not Implemented]

**Signature:** `function_name(args)`

> **Note:** This function is documented in ToastStunt but not implemented.

Description of what it would do if implemented.
```

## Output

Apply patches directly to the spec files. Be surgical - only change what's documented above.

## CRITICAL

- Do NOT invent new behaviors - only document what's verified in reports
- Do NOT change behaviors that are "likely_barn_bug" - spec follows Toast
- Do NOT change behaviors that are "likely_toast_bug" - spec documents intended behavior
- Preserve existing spec structure and formatting
