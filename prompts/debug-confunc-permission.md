# Task: Debug Why confunc Permission Check Fails in Barn

## The Problem

`#0:user_connected` calls `user.location:confunc(user)` to show the room.

`#3:confunc` (generic room) exists and does `this:look_self()`.

But `#3:confunc` has a permission check:
```moo
if ((((cp = caller_perms()) == player) || $perm_utils:controls(cp, player)) || (caller == this))
```

Toast shows the room. Barn doesn't. The permission check is likely failing in Barn.

## What To Investigate

1. In Barn's user_connected flow, what is:
   - `player` (the global variable)
   - `caller_perms()` inside confunc
   - `caller` inside confunc

2. Check how Barn sets these values when user_connected runs:
   - Does `player` get set to the wizard after switch_player?
   - Or is it still the negative connection ID?

3. Check server/scheduler.go CallVerb and how it sets:
   - The player variable
   - caller_perms context

4. Compare with how Toast sets these (from source at ~/src/toaststunt/)

## Key Files
- `server/scheduler.go` - CallVerb, callUserConnected
- `server/connection.go` - where user_connected is triggered
- `vm/` - caller_perms() builtin
- `builtins/` - set_task_perms implementation

## Output

Write to `./reports/debug-confunc-permission.md`:
1. What values Barn has for player/caller_perms/caller in confunc
2. Why the permission check fails
3. The fix needed
