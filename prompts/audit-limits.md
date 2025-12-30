# Task: Audit Limits Implementation in ToastStunt and cow_py

## Objective
Research where limit checking happens in ToastStunt (C++) and cow_py (Python) to produce a precise implementation guide for barn (Go).

## Context
MOO servers enforce limits on value sizes:
- `max_string_concat` - max bytes for string operations
- `max_list_value_bytes` - max bytes for list values
- `max_map_value_bytes` - max bytes for map values

When exceeded, operations raise E_QUOTA (catchable if `max_concat_catchable` is set).

## Research Tasks

### 1. Find `value_bytes` implementation
- ToastStunt: Search `~/src/toaststunt/` for `value_bytes` or equivalent size calculation
- cow_py: Search `~/code/cow_py/src/` for same
- Document: How is size calculated for each type (int, str, list, map, obj, etc.)?

### 2. Find all limit check locations
For each codebase, find WHERE limit checks happen:

**String operations:**
- String concatenation (`+` or `strcat`)
- `tostr()`, `toliteral()`
- `strsub()`, `substitute()`
- `encode_binary()`, `decode_binary()`
- `encode_base64()`, `decode_base64()`
- `random_bytes()`

**List operations:**
- `setadd()`, `setremove()`
- `listinsert()`, `listappend()`, `listset()`, `listdelete()`
- List literal construction `{a, b, c}`
- List append/prepend with `@`
- Index assignment `list[i] = value`
- Range assignment `list[i..j] = value`

**Map operations:**
- `mapdelete()`
- Map literal construction `["a" -> 1]`
- Index assignment `map[key] = value`
- Range assignment on maps

### 3. Document the check pattern
For each location found:
- What limit is checked? (string/list/map)
- When is it checked? (before operation, after?)
- What error is raised? (E_QUOTA, E_MAXREC?)
- Is it catchable?

## Output Format
Write findings to `./reports/audit-limits.md` with:

```markdown
# Limits Audit Report

## value_bytes Implementation
[How to calculate size for each type]

## Check Locations

### String Limits
| Operation | ToastStunt Location | cow_py Location | Check Details |
|-----------|--------------------|-----------------|--------------|
| concat    | file:line          | file:line       | checks max_string_concat before... |

### List Limits
[Same table format]

### Map Limits
[Same table format]

## Implementation Checklist for Barn
- [ ] Implement value_bytes() in builtins/
- [ ] Add check to X in file Y
- [ ] ...
```

## Reference Paths
- ToastStunt: `~/src/toaststunt/`
- cow_py: `~/code/cow_py/src/`
- barn: `~/code/barn/`

## DO NOT
- Write any Go code
- Modify any files except the report
- Make assumptions - find the actual code
