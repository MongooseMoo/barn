# Task: Fix CLI verb lookup to support wildcard matching

## Context

The barn CLI has flags for inspecting the database:
- `-verb-code "#10:connect"` - should dump verb code

The problem: MOO verbs have wildcard names like `co*nnect @co*nnect`. When I run:
```
barn.exe -db toastcore.db -verb-code "#10:connect"
```
It fails with "verb not found" because it's doing exact string matching, not wildcard matching.

## Objective

Fix the CLI verb lookup to match verbs using MOO's wildcard pattern matching:
- `co*nnect` matches: `co`, `con`, `conn`, `conne`, `connec`, `connect`
- `*` means "zero or more characters can go here"

## Files to Modify

- `cmd/barn/main.go` - the CLI entry point, has the `-verb-code` flag handling

## How MOO wildcard matching works

A verb name like `co*nnect` means:
- Must start with `co`
- Must end with `nnect`
- Any characters (including none) can appear where `*` is

So `connect` matches `co*nnect` because:
- Starts with `co` ✓
- Ends with `nnect` ✓

Multiple names separated by space are aliases: `co*nnect @co*nnect` means two patterns that both work.

## Implementation approach

1. Look at how `db.Store.FindVerb()` works - it likely already does wildcard matching
2. If so, use that instead of direct map lookup in the CLI
3. If not, the matching logic needs to be added

## Test

After fix:
```
barn.exe -db toastcore.db -verb-code "#10:connect"
```
Should dump the `co*nnect` verb's code.

## Output

Write results to `./reports/fix-verb-wildcard-lookup.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
