# Task: Learn Which Hook Toast Calls on Fresh Login

## Question

When a player does `connect wizard` on Toast (fresh login, not reconnection), does Toast call:
- `#0:user_connected`
- `#0:user_reconnected`
- Both?
- Neither?

## Why This Matters

Barn calls `user_connected` after switch_player, but "The First Room" doesn't appear.
Toast shows "The First Room" on fresh login.

We need to know what Toast actually does to match its behavior.

## Investigation Options

1. **Look at ToastStunt source code** - it's at `~/src/toaststunt/`
   - Search for user_connected, user_reconnected
   - Find where switch_player triggers hooks
   - Check what hook is called for fresh logins

2. **Check MOO documentation/spec**
   - What's the intended semantic for user_connected vs user_reconnected?

3. **Empirical test** (if needed)
   - Could modify database verbs to log which is called
   - But source code analysis is cleaner

## Output

Write to `./reports/learn-toast-login-hooks.md`:
1. What hook Toast calls on fresh login (user_connected or user_reconnected)
2. Evidence (source code location or documentation)
3. Brief explanation of the semantics
