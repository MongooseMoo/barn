# MOO Login Flow Specification

## Overview

Player connections go through authentication before entering the command loop. The login system is entirely MOO-programmable via hooks on `#0`.

---

## 1. Connection States

```
New Connection → Unlogged → Logged In → Disconnected
                    ↓
                 Timeout
```

| State | Description |
|-------|-------------|
| Unlogged | Fresh connection, not yet authenticated |
| Logged In | Associated with a player object |
| Disconnected | Connection closed |

---

## 2. Unlogged Connection Handling

### 2.1 Command Routing

All input from unlogged connections goes to:

```moo
#0:do_login_command(connection, line)
```

**Parameters:**
- `connection`: Connection identifier (implementation-defined type: may be negative INT, OBJ, or other unique identifier)
- `line`: Raw input string from user

**Return value:**
- Player object (`#N`): Login successful, associate connection with this player
- `0` or negative: Login failed, connection remains unlogged
- String: Send this message to connection (optional in some servers)

### 2.2 Example Login Verb

```moo
// #0:do_login_command
{conn, line} = args;
if (index(line, "connect ") == 1)
    // Parse "connect username password"
    parts = $string_utils:explode(line);
    if (length(parts) >= 3)
        player = $login:authenticate(parts[2], parts[3]);
        if (valid(player))
            return player;  // Success!
        endif
    endif
    notify(conn, "Invalid username or password.");
endif
return 0;  // Stay unlogged
```

### 2.3 Timeout

Unlogged connections timeout after `connect_timeout` seconds (default: 300).

On timeout:
1. `#0:user_disconnected(connection)` called if `do_login_command` was ever invoked for this connection
2. Connection closed
3. Timeout message sent (implementation-defined)

---

## 3. Login Success

When `do_login_command` returns a valid player object:

### 3.1 Simple Login (New Connection)

1. Associate connection with player object
2. Call `#0:user_connected(player)`
3. Enter command loop

### 3.2 Reconnection (Player Already Connected)

If player is already connected elsewhere:

1. Boot existing connection (send disconnect message, close socket)
2. Associate new connection with player
3. Call `#0:user_reconnected(player)`
4. Enter command loop

---

## 4. User Lifecycle Hooks

All hooks are verbs on `#0` (system object).

### 4.1 user_connected

```moo
#0:user_connected(player)
```

**Called:** After successful login for a fresh connection.

**Typical use:**
- Send welcome message
- Announce arrival to other players
- Initialize session state

### 4.2 user_reconnected

```moo
#0:user_reconnected(player)
```

**Called:** When player reconnects while already connected (replaces old connection).

**Typical use:**
- Send "reconnected" message
- Announce reconnection

### 4.3 user_disconnected

```moo
#0:user_disconnected(player)
```

**Called:** When connection closes (voluntary or timeout).

**Typical use:**
- Announce departure
- Save session state
- Clean up resources

### 4.4 Hook Errors

Errors in lifecycle hooks are logged to server log (implementation-defined format: may include traceback, error message, hook name) but do not abort the operation. The connection proceeds normally. MOO code cannot check hook execution status - errors are visible only in server log.

---

## 5. Command Loop

Once logged in, input is parsed and dispatched:

### 5.1 Parsing

```
command     → verb rest
            → verb dobj rest
            → verb dobj prep iobj
```

### 5.2 Dispatch

1. Parse command line
2. Find matching verb on dobj, player's location, or player
3. Create foreground task
4. Execute verb with context variables set

### 5.3 Context Variables

| Variable | Value |
|----------|-------|
| `player` | The connected player object |
| `this` | Object the verb is defined on |
| `caller` | Object that called this verb |
| `verb` | Verb name as string |
| `args` | Argument list |
| `argstr` | Original argument string |
| `dobj` | Direct object |
| `dobjstr` | Direct object string |
| `iobj` | Indirect object |
| `iobjstr` | Indirect object string |
| `prepstr` | Preposition string |

---

## 6. Output

### 6.1 notify()

```moo
notify(player, message [, no_flush])
```

Sends message to player's connection.

### 6.2 Output Buffering

Output is buffered and flushed:
- At end of task
- When buffer exceeds `max_queued_output` limit from server_options (default: 65536 bytes)
- On explicit flush (if supported)

---

## 7. Disconnection

### 7.1 Voluntary

Player types `@quit` or similar (handled by MOO code calling `boot_player()`).

### 7.2 boot_player()

```moo
boot_player(player)
```

Forcibly disconnects a player.

**Permissions:** Wizard, or player disconnecting self.

**Sequence:**
1. Send disconnect message to player (implementation-defined default, may be overridden by MOO code)
2. Call `#0:user_disconnected(player)`
3. Close connection

### 7.3 Connection Lost

If network connection drops:
1. Detect on next I/O attempt
2. Call `#0:user_disconnected(player)`
3. Clean up connection state

---

## 8. Multiple Connections

### 8.1 Same Player

Most servers allow only one connection per player. On reconnection:
- Old connection is booted
- New connection takes over
- `user_reconnected` called (not `user_connected`)

### 8.2 Connection Switching

Some servers support a `switch_player(old, new)` builtin to change which player object a connection represents (implementation-optional).

---

## 9. Go Implementation Notes

### 9.1 Connection Representation

Each connection needs:
- Network socket/channel
- Player object ID (0 if unlogged)
- Input buffer
- Output buffer
- Connection time
- Last activity time

### 9.2 Goroutine Model

```go
// Per-connection goroutine
func handleConnection(conn net.Conn) {
    c := &Connection{socket: conn}

    // Unlogged phase
    for !c.loggedIn {
        line := c.readLine()
        player := callDoLoginCommand(c, line)
        if player > 0 {
            c.player = player
            c.loggedIn = true
        }
    }

    // Notify login
    callUserConnected(c.player)

    // Command loop
    for {
        line := c.readLine()
        dispatchCommand(c.player, line)
    }
}
```

### 9.3 Graceful Shutdown

On server shutdown, all connections should:
1. Receive shutdown message
2. Have `user_disconnected` called
3. Be closed cleanly
