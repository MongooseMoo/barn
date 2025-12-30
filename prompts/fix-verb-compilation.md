# Task: Fix Verb Compilation for Player Commands

## Context
Barn is a Go MOO server. There's a bug where player commands fail with "[verb has no code]" because verbs aren't being compiled before task dispatch.

## The Bug
In `server/connection.go`, the `dispatchCommand` function checks `match.Verb.Program` but verbs are lazy-compiled only in `vm/verbs.go` via `evaluator.CallVerb()`.

Two code paths exist:
1. `Scheduler.CallVerb()` → `evaluator.CallVerb()` - compiles verbs (used for server hooks)
2. `dispatchCommand()` → `CreateVerbTask()` - does NOT compile verbs (used for player commands)

## Files to Modify
- `server/connection.go` - the `dispatchCommand` function around line 362-413

## The Fix
Before checking/using `match.Verb.Program`, compile the verb if needed:

```go
// Compile verb if needed (lazy compilation)
if match.Verb.Program == nil && len(match.Verb.Code) > 0 {
    program, errors := db.CompileVerb(match.Verb.Code)
    if len(errors) > 0 {
        conn.Send(fmt.Sprintf("Verb compile error: %s", errors[0]))
        return nil
    }
    match.Verb.Program = program
}
```

This should go BEFORE the existing check at line 404:
```go
if match.Verb.Program == nil || len(match.Verb.Program.Statements) == 0 {
```

## Test Command
After fixing, build and test:
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9201 &
sleep 2
(echo "connect wizard"; sleep 1; echo "; 1 + 1"; sleep 1) | nc -w 5 localhost 9201
```

Expected output should include:
```
-=!-^-!=-
{1, 2}
-=!-v-!=-
```

## Output
Write status to `./reports/fix-verb-compilation.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./server/connection.go`, `C:/Users/Q/code/barn/server/connection.go`
4. NEVER use cat, sed, echo - always Read/Edit/Write
