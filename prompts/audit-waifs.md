# Task: Audit Waif Implementation in ToastStunt and cow_py

## Objective
Research how waifs work in ToastStunt (C++) and cow_py (Python) to produce an implementation guide for barn (Go).

## Context
Waifs are lightweight, garbage-collected object-like values. They:
- Have a "class" object that defines their properties/verbs
- Store property values locally
- Are created via `new_waif()` builtin
- Cannot be parents/children of other objects
- Cannot have player/wizard/programmer flags
- Owner cannot be changed after creation

## Research Tasks

### 1. Find `new_waif()` Implementation
- ToastStunt: Search `~/src/toaststunt/src/` for `new_waif`, `bf_new_waif`, waif creation
- cow_py: Search `~/code/cow_py/src/` for same
- Document: What parameters? How is class determined? How is owner set?

### 2. Find Waif Value Type
- How is a waif value represented?
- How does it store property values?
- How does it reference its class object?

### 3. Find Waif Property Access
- How does property get/set work on waifs?
- Which properties come from class vs stored locally?
- What restrictions exist?

### 4. Find Waif Verb Calls
- How does verb lookup work on waifs?
- Does it just delegate to class object?

### 5. Find Waif Restrictions
- What operations are forbidden on waifs?
- `valid()` - always returns 0?
- `parents()` / `children()` - E_INVARG?
- `.wizard`, `.programmer`, `.player` flags - E_PERM?
- `.owner` - read-only after creation?

### 6. Check Existing Barn Implementation
- Look at `~/code/barn/types/waif.go`
- What's already implemented?
- What's missing?

## Reference Paths
- ToastStunt: `~/src/toaststunt/src/`
- cow_py: `~/code/cow_py/src/`
- barn: `~/code/barn/`
- barn waif type: `~/code/barn/types/waif.go`

## Output Format
Write findings to `./reports/audit-waifs.md` with:

```markdown
# Waif Audit Report

## new_waif() Implementation
- Parameters: ...
- Class determination: ...
- Owner: ...
- ToastStunt location: file:line
- cow_py location: file:line

## Waif Value Structure
- Class reference: ...
- Property storage: ...
- ToastStunt: file:line
- cow_py: file:line

## Property Access
- Get: ...
- Set: ...
- Restrictions: ...

## Verb Calls
- Lookup: ...
- Dispatch: ...

## Forbidden Operations
| Operation | Expected Result | ToastStunt | cow_py |
|-----------|-----------------|------------|--------|
| valid(waif) | 0 | file:line | file:line |
| parents(waif) | E_INVARG | ... | ... |
| ...

## Implementation Plan for Barn
1. [ ] Implement new_waif() builtin
2. [ ] Add waif property get/set in vm/properties.go
3. [ ] Add waif verb lookup
4. [ ] Add restrictions for forbidden operations
5. [ ] ...
```

## DO NOT
- Write any Go code
- Modify any files except the report
- Make assumptions - find the actual code
