# Task: Fix Server Hook Tracebacks

## Role
You are the FIXER. Read the learner's findings and implement the fix.

## Prerequisites

FIRST read `./reports/learn-server-hook-tracebacks.md` - the LEARNER agent wrote findings there.

If that file doesn't exist yet, WAIT and check again in a few seconds.

## The Problem

Server hooks (like `do_login_command`) don't show tracebacks when they error. They fail silently. This makes debugging impossible.

## Your Task

1. Read the learner's report to understand the exact code path difference
2. Implement the fix so server hooks show tracebacks like player commands do
3. Test by triggering a login and checking for any errors

## Test

```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db toastcore_barn.db -port 9300 &
sleep 3
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard"
```

If login has errors, they should now show as tracebacks.

## Output

Write to `./reports/fix-server-hook-tracebacks.md`:
1. What the learner found (brief summary)
2. What you changed (file:line)
3. Test results

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
