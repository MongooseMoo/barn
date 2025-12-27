# Task: Fix Database Reader and Build Inspection Tooling

## Problem

The `connect wizard` command fails because `$player_db:find_exact` is not found. When we inspect #39 (player_db), we see only 7 verbs and no parent. But #39 should inherit from #37 (Generic Database) which has `find_exact`.

The database reader is likely parsing parent relationships incorrectly.

## Objective

1. Fix the database reader to correctly parse object parents
2. Build better CLI tooling to inspect database contents
3. Add tests to verify database reading is correct

## Reference Implementations

Compare against these known-good implementations:

### lambdamoo-db-py (`~/src/lambdamoo-db-py/`)
- Python LambdaMOO database parser
- Check how it parses object parents in v4 format

### ToastStunt (`~/src/toaststunt/`)
- C++ MOO server
- Look at `db_io.cc` or similar for database reading
- Check how v4 format objects are parsed

## Current Code

The database reader is in `db/reader.go`. The `readObjectV4` function (around line 397) parses objects.

Current parsing order (lines ~460-493):
```
location
firstContent (skip)
neighbor (skip)
parent
firstChild (skip)
sibling (skip)
```

This may be wrong. Compare against reference implementations.

## Verification Steps

1. After fixing, run: `./barn.exe -db toastcore.db -obj-info "#39"`
   - Should show parent = #37 (or similar non-empty parent)

2. Run: `./barn.exe -db toastcore.db -eval 'parent($player_db)'`
   - Should NOT return #-1

3. Run: `./barn.exe -db toastcore.db -eval '$player_db:find_exact("wizard")'`
   - Should either find the wizard or return $failed_match, NOT E_VERBNF

## CLI Tooling to Add

Add these flags to `cmd/barn/main.go`:

1. `-dump-obj-raw "#39"` - Dump raw database fields for an object (for debugging)
2. `-verb-lookup "#39:find_exact"` - Show where a verb would be found (which parent)
3. `-ancestry "#39"` - Show full parent chain

## Tests to Add

Create `db/reader_test.go` with tests that:
1. Parse a known object and verify parent is correct
2. Parse an object with verbs and verify verb count matches
3. Verify verb inheritance lookup works

## Output

Write findings and status to `./reports/db-reader-fix.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
