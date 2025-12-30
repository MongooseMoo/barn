# Task: Fix Map Operations

## Context
5 tests failing for map operations: mapdelete, range operations, first/last index.

## Objective
Fix map operations to pass conformance tests.

## Failing Tests

- mapdelete_removes_entry - mapdelete should remove entry
- mapdelete_chain - chained mapdelete operations
- first_last_index - first() and last() on maps
- ranged_set_invalid_range_2 - range validation
- ranged_set_invalid_range_3 - range validation
- ranged_set_merge_existing_key - range set merging
- inverted_ranged_set_in_loop - inverted range in loop

## Investigation

1. Find map-related builtins:
```bash
grep -rn "mapdelete\|builtinMapDelete" /c/Users/Q/code/barn/builtins/
```

2. Run a specific failing test to see the error:
```bash
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_map.py::test_mapdelete_removes_entry --transport socket --moo-port 9300 -v -s
```

3. Test manually:
```bash
./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; x = [\"a\" -> 1, \"b\" -> 2]; return mapdelete(x, \"a\");"
```

## Verification

```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
# restart server

cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_map.py --transport socket --moo-port 9300 -v
```

## After Fix Verified

Commit:
```bash
git add builtins/*.go vm/*.go
git commit -m "Fix map operations: mapdelete, ranges, first/last

Fix mapdelete to properly remove entries, fix range operations
for maps, fix first()/last() for map keys."
```

## Output
Write status to `./reports/fix-map-operations.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
5. If all formats fail, STOP and report - do not use bash workarounds
