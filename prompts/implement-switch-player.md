# Task: Implement switch_player Builtin

## Context
2 tests failing because `switch_player()` builtin is not implemented.

## Objective
Implement `switch_player(player, connection)` builtin that switches which player is controlling a connection.

## ToastStunt Reference

```bash
grep -n "bf_switch_player" /c/Users/Q/src/toaststunt/src/functions.cc
```

### switch_player(player, connection)
- Requires wizard permissions
- Switches the controlling player for a network connection
- Non-wizards get E_PERM
- Programmers (non-wizards) also get E_PERM

## Tests Expecting

From test names:
- non_wizard_gets_E_PERM - non-wizard calling switch_player gets E_PERM
- programmer_cannot_switch_player - programmer (non-wizard) gets E_PERM

## Implementation

```go
func builtinSwitchPlayer(e *Evaluator, args []Value) (Value, error) {
    // Check wizard permissions -> E_PERM if not
    // Get player object from args[0]
    // Get connection object from args[1]
    // Perform the switch
    // Return 0 on success
}
```

## Verification

```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
# restart server

cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_switch_player.py --transport socket --moo-port 9300 -v
```

## After Fix Verified

Commit:
```bash
git add builtins/*.go
git commit -m "Implement switch_player() builtin

Switches the controlling player for a network connection.
Requires wizard permissions, returns E_PERM for non-wizards."
```

## Output
Write status to `./reports/implement-switch-player.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
5. If all formats fail, STOP and report - do not use bash workarounds
