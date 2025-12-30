# Task: Learn Why Server Hooks Don't Show Tracebacks

## Role
You are the LEARNER. Investigate only. Do NOT write code fixes.

## The Problem

Player commands show tracebacks when they error (we verified this with `look me`).

Server hooks like `do_login_command` do NOT show tracebacks when they error - they fail silently.

Something during login fails (we don't see "The First Room" that Toast shows), but no traceback appears.

## What To Investigate

1. How does `do_login_command` get called? (server/scheduler.go `CallVerb`?)

2. Where does error handling differ between:
   - Player commands (tracebacks work)
   - Server hooks (tracebacks missing)

3. Find the exact code path difference - where does one send tracebacks and the other doesn't?

4. Check: Is there a try/catch or error suppression in the server hook path?

## Key Files
- `server/scheduler.go` - task execution, CallVerb, CreateVerbTask
- `server/connection.go` - where do_login_command gets triggered
- `task/` - task types, error handling

## Output

Write findings to `./reports/learn-server-hook-tracebacks.md`:
1. How player command errors flow to traceback (the working path)
2. How server hook errors flow (the broken path)
3. The EXACT difference - file:line where they diverge
4. What needs to change (but don't implement it)

The FIXER agent will read your report and implement the fix.
