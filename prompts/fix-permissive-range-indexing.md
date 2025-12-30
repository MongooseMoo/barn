# Task: Fix Permissive Range Indexing

## Problem

Barn throws Range error where Toast returns empty list for out-of-bounds ranges.

**Toast behavior:**
```
;{1}[1..4]
=> Range error (strict - end beyond length)

;{1}[$..$-4]
=> {}  (permissive - backwards/empty range returns empty list)
```

**Barn behavior:**
Both cases throw Range error. Barn is too strict.

## The Bug

In `#46:messages_in_seq` line 6:
```moo
return caller.messages[msgs[1]..msgs[2] - 1];
```

When `msgs[2] - 1` results in a value less than `msgs[1]` (backwards range), Toast returns `{}` but Barn throws Range error.

## What Toast Does

Toast's range indexing `list[start..end]`:
- If `end < start`: return empty list `{}`
- If `start > length`: return empty list `{}`
- If `end > length`: clamp to length (or return what's available)
- Only error on truly invalid cases

## Where to Fix

Look in `vm/eval.go` or `vm/eval_expr.go` for range/slice operations on lists. The code that handles `list[start..end]` syntax needs to be more permissive.

## Test

```bash
cd ~/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db toastcore_barn.db -port 9950 > server.log 2>&1 &
sleep 3

# These should return {} not Range error:
./moo_client.exe -port 9950 -timeout 5 -cmd "connect wizard" -cmd "; return {1}[\$..\$-4];"
./moo_client.exe -port 9950 -timeout 5 -cmd "connect wizard" -cmd "; return {1, 2, 3}[3..1];"

# After fix, news command should get further:
./moo_client.exe -port 9950 -timeout 10 -cmd "connect wizard" -cmd "news"
```

## Compare with Toast Oracle

```bash
./toast_oracle.exe '{1, 2, 3}[3..1]'
./toast_oracle.exe '{1, 2, 3}[5..10]'
./toast_oracle.exe '{1}[$..$-4]'
```

## Output

Write findings to `./reports/fix-permissive-range-indexing.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
