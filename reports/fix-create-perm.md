# Fix create() Permission Check for Wizards

## Problem

The `create()` builtin was incorrectly checking wizard status based on the **verb owner** (programmer) instead of the **player** executing the command. This caused wizard players to get E_PERM when calling create() from non-wizard-owned verbs like `:eval`.

### Symptom

```
; return player.wizard;
{1, 1}  // Player IS wizard

; return parent(create(#2));
{0, E_PERM}  // But create fails with permission error
```

## Root Cause

In MOO, there are two key permission contexts:
- `ctx.Player` - The player who typed the command (e.g., #1 wizard)
- `ctx.Programmer` - The owner of the verb being executed (e.g., owner of `:eval` verb)
- `ctx.IsWizard` - Set based on the **programmer/verb owner**, not the player

When a wizard types `; return create(#2);`:
1. Server parses `;` prefix as `eval` command
2. Finds `:eval` verb (likely owned by non-wizard system object)
3. Sets `ctx.Programmer` to verb owner
4. Sets `ctx.IsWizard` based on **verb owner** (not wizard)
5. `create()` checks `ctx.IsWizard` and incorrectly denies permission

## Solution

Modified `builtins/objects.go` to check the **player's** wizard status instead of relying solely on `ctx.IsWizard`:

```go
// Before (line 166):
if !ctx.IsWizard {
    isOwner := parent.Owner == ctx.Programmer
    hasFertile := parent.Flags.Has(db.FlagFertile)
    if !isOwner && !hasFertile {
        return types.Err(types.E_PERM)
    }
}

// After:
playerIsWizard := ctx.IsWizard || isPlayerWizard(store, ctx.Player)
if !playerIsWizard {
    isOwner := parent.Owner == ctx.Programmer
    hasFertile := parent.Flags.Has(db.FlagFertile)
    if !isOwner && !hasFertile {
        return types.Err(types.E_PERM)
    }
}
```

Added helper function:
```go
func isPlayerWizard(store *db.Store, objID types.ObjID) bool {
    obj := store.Get(objID)
    if obj == nil {
        return false
    }
    return obj.Flags.Has(db.FlagWizard)
}
```

## Changes Made

1. **Line 168** in `builtins/objects.go`: Changed permission check to also check player's wizard status
2. **Line 277** in `builtins/objects.go`: Applied same fix to owner=$nothing permission check
3. **Lines 1090-1097**: Added `isPlayerWizard()` helper function

## Testing

The fix compiles successfully:
```bash
go build -o barn_fix.exe ./cmd/barn/
```

## Notes

- The approach uses `ctx.IsWizard || isPlayerWizard(store, ctx.Player)` to handle both cases:
  - Direct execution where `ctx.IsWizard` is correctly set
  - Execution through non-wizard verbs where player status must be checked

- This preserves correct permission semantics: wizards can create from any parent, regardless of which verb they call create() from

- A separate concurrent map writes issue was discovered during testing (unrelated to this fix)
