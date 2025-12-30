# Task: Fix create() Permission Check for Wizards

## Context
Barn is a Go MOO server. The `create()` builtin is returning E_PERM even when the caller is a wizard.

## The Bug
```
; return player.wizard;
{1, 1}  // Player IS wizard

; return parent(create(#2));
{0, E_PERM}  // But create fails with permission error
```

Wizards should be able to create objects from any parent.

## Files to Check
- `builtins/objects.go` - contains the create builtin (around line 11 and 83)

## What to Fix
Find why the permission check fails for wizards. The code comments say:
- "Wizards can create from any object"
- "Non-wizards can only create from objects they own OR that have the fertile flag"

Check how the wizard status is being retrieved from ctx (TaskContext). The player's wizard flag is `player.wizard` but the permission check may be looking at the wrong thing.

## Test Command
After fixing, test manually:
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9225 &
sleep 2
printf 'connect wizard\n; return parent(create(#2));\n' | nc -w 3 localhost 9225
```

Expected: `{1, #2}` (success, parent is #2)

## Output
Write status to `./reports/fix-create-perm.md`

## CRITICAL: Do NOT modify tests
The tests are correct. Only fix the Go implementation.

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./builtins/objects.go`, `C:/Users/Q/code/barn/builtins/objects.go`
4. NEVER use cat, sed, echo - always Read/Edit/Write
