# Investigation: Why "The First Room" Doesn't Appear on Login

## Problem Statement

Toast shows "The First Room" after `connect wizard`. Barn shows welcome messages and ANSI notification but no room description.

## Login Flow Trace

### 1. Entry Point: #0:do_login_command

Server task called on new connection. Key steps:
- Line 34: `$login:(args[1])(@listdelete(args, 1))` - calls `$login:connect("wizard")`
- Line 37: `switch_player(player, retval)` - switches connection to authenticated player

### 2. Authentication: #10:connect (Login Commands)

The `co*nnect` verb (lines 1-117) handles:
- Name matching (line 17)
- Password verification (lines 29-48)
- Guest handling (lines 64-72)
- Connection limits (lines 100-106)
- Returns player object on success (line 113)

### 3. Connection Switch

After successful auth, `switch_player()` is called in Barn's `connection.go`:
- Marks connection as `alreadyLoggedIn = true`
- Calls `callUserConnected(player)` instead of `callUserReconnected(player)`
- This is correct per spec - first login uses user_connected

### 4. Hook: #0:user_connected

The verb `"user_created user_connected"` on #0 (lines 1-17):
```moo
user = args[1];
set_task_perms(user);
try
  user.location:confunc(user);  // Line 9: Call confunc on location
  user:confunc();                 // Line 10: Call confunc on player
except id (ANY)
  [error handling]
endtry
```

### 5. Player confunc Chain

#6:confunc (generic player) calls:
- `this:("@last-connection")()` - logs connection time
- `$news:check()` - checks for news

### 6. Room Display: #3:enterfunc (generic room)

Called when player is moved into room. Key line:
```moo
if (is_player(object) && (object.location == this))
  player = object;
  this:look_self(player.brief);  // Line 4: Display room
endif
```

## Actual Behavior in Barn

### Client Output
```
Welcome to the ToastCore database.
[welcome message continues]
ANSI Version 2.6 is currently active.
Your previous connection was before we started keeping track.
There is new news.  Type `news' to read all news or `news new' to read just new news.
[Connection closes]
```

### Server Log
```
2025/12/29 19:44:48 New connection from [::1]:26791 (ID: 2)
2025/12/29 19:44:48 Switched connection 2 from player -2 to 2
2025/12/29 19:44:48 Connection 2 already logged in as player 2 via switch_player
2025/12/29 19:44:58 Connection 2 read error: EOF
2025/12/29 19:44:58 user_disconnected error: E_PERM
```

**Key observations:**
1. No "user_connected error" - verb was found and called
2. ANSI/news messages appear - confunc IS running
3. Connection closes with EOF - premature disconnection
4. Room description is missing

## Root Cause

The `#0:user_connected` verb IS being called and IS running. Evidence:
- No "user_connected error" in logs (would appear if verb not found)
- ANSI notification appears (from player confunc)
- News notification appears (from player confunc)

**But `user.location:confunc(user)` on line 9 is not being called, OR it's failing silently.**

### Why location:confunc Isn't Working

When `user_connected` calls `user.location:confunc(user)`, this should:
1. Call confunc on the room (player's location)
2. Room's confunc doesn't exist, so fails
3. **OR** player.location is invalid/not set

Actually, wait - looking at #0:user_reconnected, I see it does:
```moo
move(user, $nothing);
move(user, user.home);  // This triggers enterfunc
```

But #0:user_connected does:
```moo
user.location:confunc(user);  // Just calls confunc, no move
user:confunc();
```

**The problem:** `user.location:confunc` tries to call a verb that doesn't exist. Rooms don't have confunc. The correct approach is user_reconnected's method: **move the player** to trigger enterfunc.

### Why This Works on Toast

Need to verify: Does Toast's user_connected actually work, or does Toast take a different path?

Let me check if Toast goes through user_connected or user_reconnected for initial login.

## The Actual Bug

**Hypothesis:** Barn's connection.go logic is wrong. It calls user_connected when `alreadyLoggedIn` is true, but this happens on EVERY switch_player call, including the first login from do_login_command.

Looking at connection.go lines 382-394:
```go
if alreadyLoggedIn {
    log.Printf("Connection %d already logged in as player %d via switch_player", conn.ID, player)
    cm.callUserConnected(player)
    return
}

if reconnection {
    existingConn.Send("You have been disconnected (reconnected elsewhere)")
    existingConn.Close()
    cm.callUserReconnected(player)
} else {
    cm.callUserConnected(player)
}
```

The logic is:
1. If `alreadyLoggedIn`, call user_connected and RETURN (skip everything else)
2. Otherwise, if `reconnection`, call user_reconnected
3. Otherwise, call user_connected

**The bug:** `alreadyLoggedIn` is set by switch_player, which is called during normal login. This causes the code to call user_connected and return early, skipping the room display logic.

## Solution

The issue is that `alreadyLoggedIn` doesn't distinguish between:
1. **First login via switch_player** (should call user_reconnected per ToastCore's expectations)
2. **Already connected from another connection** (should disconnect old connection)

**Option 1:** Fix user_connected verb in database
Add move logic like user_reconnected has:
```moo
move(user, $nothing);
move(user, user.home);  // Triggers enterfunc -> look_self
```

**Option 2:** Fix Barn's login logic
Call user_reconnected instead of user_connected for switch_player logins, since ToastCore's user_connected doesn't handle room display.

**Option 3:** Investigate why user.location:confunc fails
The verb tries to call confunc on the location, but rooms don't have confunc. This should trigger an E_VERBNF exception, which the try/except would catch and display error messages. But no error appears - why?

## Next Steps

1. Test with Toast: Does `connect wizard` call user_connected or user_reconnected?
2. Check if Toast's database has a different user_connected implementation
3. Verify if the issue is:
   - Wrong hook being called
   - Hook implementation doesn't handle room display
   - Hook is failing silently

## Confirmed Root Cause

Verified that #62 (The First Room) **does NOT have a confunc verb**. When user_connected calls:
```moo
user.location:confunc(user);  // Line 9
```

This should fail with E_VERBNF (verb not found), which the try/except would catch and display:
```moo
user:tell("Confunc failed: ", id[2], ".");
[traceback output]
```

**But no error message appears in the client output!** This means either:
1. The exception is being caught but tell() isn't working
2. user.location is invalid (#0 or $nothing) so the verb call doesn't happen
3. The verb call is silently failing

Let me verify player.location on fresh login...

Actually, checking the code flow: when switch_player is called during do_login_command, the player hasn't been moved into a room yet. The player.location is likely #0 or invalid.

## The Real Issue

**user_connected expects the player to already be in their location**, but switch_player happens BEFORE the player is moved into the room.

Compare with **user_reconnected**:
```moo
move(user, $nothing);     // Clear location first
move(user, user.home);    // Move to home, triggers enterfunc
```

This explicitly handles placement, while user_connected assumes location is already set.

## Recommended Fix

**Option 1: Call user_reconnected for all logins**

Modify `connection.go` line 383-394 to call `user_reconnected` for switch_player logins:
```go
if alreadyLoggedIn {
    log.Printf("Connection %d already logged in as player %d via switch_player", conn.ID, player)
    cm.callUserReconnected(player)  // Changed from callUserConnected
    return
}
```

**Option 2: Fix the database's user_connected verb**

Update #0:user_connected to match user_reconnected:
```moo
user = args[1];
set_task_perms(user);
move(user, $nothing);
move(user, user.home);  // This triggers enterfunc -> look_self
user:confunc();
```

**Recommendation:** Option 1 is simpler and matches actual MOO server behavior. ToastStunt likely calls user_reconnected for all successful logins, regardless of whether it's a "reconnection" in the traditional sense.
