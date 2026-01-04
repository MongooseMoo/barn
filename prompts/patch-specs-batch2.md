# Task: Patch Specs Based on Batch 2 Divergence Reports

## Context

Batch 2 divergence detection is complete. Three reports need spec patches:
- `reports/divergence-types.md` - Found 4 spec gaps (functions not in Toast)
- `reports/divergence-objects.md` - Clean (no changes needed)
- `reports/divergence-properties.md` - Clean but needs documentation clarifications

## Objective

Update the spec files based on verified divergence findings.

## Files to Read

1. `reports/divergence-types.md` - Types divergence findings
2. `reports/divergence-objects.md` - Objects divergence findings
3. `reports/divergence-properties.md` - Properties divergence findings
4. `spec/builtins/types.md` - Current types spec
5. `spec/builtins/properties.md` - Current properties spec

## Changes Required

### 1. spec/builtins/types.md

**Remove or mark as non-existent:**
- `toerr()` - Section 6 - Does NOT exist in Toast
- `tonum()` - Section 7 - Does NOT exist in Toast (not an alias)
- `typename()` - Section 10 - Does NOT exist in Toast
- `is_type()` - Section 11 - Does NOT exist in Toast

**Fix accuracy issues:**
- Section for `tostr()`: Note that with no args returns "" (not E_ARGS)
- Section for `tostr()`: Note that `tostr({1,2})` returns "{list}" not "{1, 2}"
- Section for `tostr()`: Note that `tostr([])` returns "[map]" not "[]"

### 2. spec/builtins/objects.md

**No changes needed** - Report status is clean.

### 3. spec/builtins/properties.md

**Add permission string documentation:**
```
Valid permission strings (order matters):
- "" (empty) - no permissions
- "r" - read only
- "w" - write only
- "rw" - read and write
- "rwc" - read, write, and chown

Invalid combinations:
- "c" alone (chown requires write)
- "rc" (chown requires write)
- "wc" (valid but should document)
- Out of order: "wrc", "crw", etc. → E_INVARG
```

**Clarify delete_property:**
Current spec says: "Cannot delete inherited properties; use clear_property instead."
Actual behavior (both servers):
- `delete_property(child, inherited_prop)` succeeds (no-op)
- `delete_property(child, local_override)` removes override

**Clarify property_info on built-ins:**
- `property_info(#0, "name")` → E_PROPNF
- Built-in properties (name, owner, location, etc.) not tracked in property system

## Output

Write summary to: `reports/patch-batch2-summary.md`

## CRITICAL

- Only make changes documented in divergence reports
- Preserve existing spec structure
- Use [Not Implemented] or similar markers for non-existent functions
- Do NOT add new content beyond what's in the reports
- Do NOT modify any Go code
