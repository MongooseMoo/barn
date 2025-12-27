# Task: Fix verb argument spec loading from database

## Context

The `verb_args()` builtin returns `{"", "", ""}` for all verbs because the argument specs aren't being loaded from the database.

In `db/reader.go` around line 527:
```go
// Preps
_, err = readInt(r)  // DISCARDED - THIS IS THE BUG
```

The verb argspec is being read but thrown away.

## MOO Database Format

Looking at the toastcore.db file, verb entries look like:
```
co*nnect @co*nnect   <- verb name(s)
2                     <- owner (#2)
93                    <- perms + argspec encoding
-1                    <- prep value (-1 = none)
```

The encoding (from LambdaMOO/ToastStunt):
- `perms & 0x7` = basic permissions (r=1, w=2, x=4)
- `perms & 0x8` = debug flag (d)
- `(perms >> 4) & 0x3` = dobj spec (0=none, 1=any, 2=this)
- `(perms >> 6) & 0x3` = iobj spec (0=none, 1=any, 2=this)

The prep value:
- -2 = any
- -1 = none
- 0+ = specific preposition index

Preposition table (0-indexed):
```
0: with/using
1: at/to
2: in front of
3: in/inside/into
4: on top of/on/onto/upon
5: out of/from inside/from
6: over
7: through
8: under/underneath/beneath
9: behind
10: beside
11: for/about
12: is
13: as
14: off/off of
```

## Files to Modify

- `db/reader.go` - parse and store argspec in verb.ArgSpec

## Implementation

1. In the verb reading loop (around line 520-535):
   - Extract dobj from `(perms >> 4) & 0x3`
   - Extract iobj from `(perms >> 6) & 0x3`
   - Read the prep value (currently discarded)
   - Convert to strings: 0="none", 1="any", 2="this"
   - Convert prep index to string (or "none"/"any" for -1/-2)
   - Store in `verb.ArgSpec`

2. Also check the other verb reading location around line 715-747.

## Verification

After fix:
```bash
barn.exe -db toastcore.db -eval "verb_args(#10, 3)"
```
Should return something like `{"any", "none", "any"}` instead of `{"", "", ""}`.

## Output

Write results to `./reports/fix-verb-argspec-loading.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
