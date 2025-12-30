# Task: Fix ctime() Format to Match MOO Standard

## Problem

Barn's `ctime()` returns a 27-character string with timezone:
```
"Sun Dec  28 18:45:46 2025 MST"
```

Traditional MOO format is 24 characters without timezone:
```
"Sun Dec 28 18:45:46 2025"
```

This breaks MOO code like `#61:description` in toastcore.db that does:
```moo
raw = ctime(this.last_news_time);
date = raw[1..10] + "," + raw[20..24];
```

## Root Cause

In `builtins/system.go`, the ctime implementation uses:
```go
t.Format("Mon Jan  2 15:04:05 2006 MST")
```

The `MST` should be removed.

## Fix

Change the format to:
```go
t.Format("Mon Jan  2 15:04:05 2006")
```

## Location

`builtins/system.go` - `builtinCtime` function (around line 280)

## Test

```bash
cd ~/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9500 > server.log 2>&1 &
sleep 3
./moo_client.exe -port 9500 -timeout 5 -cmd "connect wizard" -cmd "; return {ctime(), length(ctime())};"
# Should return 24-char string without timezone
```

## Expected Result

```
=> {"Sun Dec 28 18:45:46 2025", 24}
```

## Deliverable

1. Fix the ctime format in builtins/system.go
2. Verify with test
3. Write brief report to ./reports/fix-ctime-format.md

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
