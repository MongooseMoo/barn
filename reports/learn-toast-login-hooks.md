# Toast Login Hook Behavior Investigation

## Summary

**For a fresh login (first-time connection), Toast calls `user_connected`, NOT `user_reconnected`.**

## Evidence from ToastStunt Source Code

### 1. Normal Login Flow (C:\Users\Q\src\toaststunt\src\server.cc, lines 1854-1855)

When `do_login_command` returns a player object:

```c
call_notifier(new_id, new_h->listener,
              is_newly_created ? "user_created" : "user_connected");
```

- If player was just created → `user_created`
- If player already existed → `user_connected`
- No check for existing connections at this point

### 2. Reconnection Case (same file, lines 1830-1838)

When a player already has an active connection and connects again:

```c
if (existing_listener == new_h->listener)
    call_notifier(new_id, new_h->listener, "user_reconnected");
else {
    new_h->disconnect_me = true;
    call_notifier(new_id, existing_listener, "user_client_disconnected");
    new_h->disconnect_me = false;
    call_notifier(new_id, new_h->listener, "user_connected");
}
```

**Key insight:** `user_reconnected` is ONLY called when:
1. The player already has an active connection (existing_h != NULL)
2. Both connections are on the same listener

Otherwise, it's `user_connected`.

### 3. switch_player() Behavior (C:\Users\Q\src\toaststunt\src\server.cc, lines 1027-1032)

After `switch_player()` completes, in the main loop:

```c
if (h->switched) {
    if (h->switched != h->player && is_user(h->switched))
        call_notifier(h->switched, h->listener, "user_disconnected");
    if (is_user(h->player))
        call_notifier(h->player, h->listener,
                      h->switched == h->player ? "user_reconnected" : "user_connected");
    h->switched = 0;
}
```

**Logic:**
- `h->switched` stores the OLD player objid
- `h->player` is the NEW player objid
- If `switched == player` → reconnection to same player → `user_reconnected`
- If `switched != player` → switching to different player → `user_connected`

## Official Documentation

From `ChangeLog-LambdaMOO.txt` (lines 630-637):

```
:user_connected(USER)
    When #0:do_login_command() returns USER, a previously-existing
    valid player object for which no active connection already
    existed.

:user_reconnected(USER)
    When #0:do_login_command() returns USER, a previously-existing
    valid player object for which there was already an active
    connection.
```

## Semantics

| Hook | When Called |
|------|-------------|
| `user_created` | New player object just created (objid > max_object() before login) |
| `user_connected` | Existing player, NO active connection already |
| `user_reconnected` | Existing player, HAS active connection already |
| `user_disconnected` | Player disconnecting or being switched away from |

## Barn Implication

**Barn's current behavior is CORRECT:**

When `do_login_command` calls `switch_player(-1, wizard)`:
- Old player: -1 (not logged in)
- New player: wizard objid (e.g., #3)
- These are different → should call `user_connected`

This matches Toast's logic: fresh login = `user_connected`.

## The Real Problem

If Barn calls `user_connected` but "The First Room" doesn't appear, the bug is NOT in which hook is called. The bug must be:

1. **Hook not being called at all** (timing issue, main loop not processing)
2. **Hook called but verb fails silently** (error in user_connected verb)
3. **Hook called, verb succeeds, but output not sent** (notify() not working)
4. **Hook called too early** (before connection is fully established)

The difference between Barn and Toast is likely in WHEN/HOW the hook is processed, not WHICH hook.

## Next Steps

To debug why "The First Room" doesn't appear:

1. Add logging to confirm `user_connected` is actually being called
2. Verify the verb `#0:user_connected` exists and runs without errors
3. Check if output from the verb reaches the player
4. Compare timing: Toast processes the hook in the main event loop after switch_player returns, not immediately during switch_player
