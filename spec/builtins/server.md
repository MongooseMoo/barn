# MOO Server Administration Built-ins

## Overview

Functions for server administration, monitoring, and control.

---

## 1. Server Information

### 1.1 server_version

**Signature:** `server_version() → STR`

**Description:** Returns server version string.

**Returns:** Version identifier string (format implementation-defined, conventionally "Name Major.Minor.Patch").

**Examples:**
```moo
server_version()  => "Barn 1.0.0"
```

---

### 1.2 memory_usage [Not Implemented]

**Signature:** `memory_usage() → LIST`

> **Note:** This function is documented in MOO specifications but not implemented in ToastStunt or Barn.

**Description:** Would return memory statistics.

**Returns:** Would return list of memory metrics. Format is implementation-defined. Common implementations would return `{total_bytes, used_bytes, free_bytes}` or a map of metric names to values.

**Examples:**
```moo
memory_usage()  => {10485760, 6291456, 4194304}  // Total, used, free
memory_usage()  => [["total" -> 10485760], ["used" -> 6291456]]  // Map format
```

---

## 2. Server Control

### 2.1 shutdown

**Signature:** `shutdown([message [, panic]]) → none`

**Description:** Initiates server shutdown.

**Parameters:**
- `message` (STR, optional): Shutdown message (logged, sent to players)
- `panic` (BOOL, optional): If true, panic shutdown (emergency dump)

**Permissions:** Wizard only.

**Behavior:**
- Graceful (default): Calls `#0:shutdown_started(message)`, stops accepting connections, checkpoints, notifies players, clean shutdown
- Panic: Emergency dump, immediate exit
- Hook calling order: `#0:shutdown_started()` is called before stopping connections

**Examples:**
```moo
shutdown();                           // Graceful shutdown
shutdown("Maintenance");              // With message
shutdown("Emergency!", 1);            // Panic shutdown
```

**Errors:**
- E_PERM: Caller is not a wizard

---

### 2.2 dump_database

**Signature:** `dump_database() → none`

**Description:** Forces immediate database checkpoint.

**Permissions:** Wizard only.

**Behavior:**
- Returns immediately (does not block)
- Triggers asynchronous checkpoint sequence
- Hooks `#0:checkpoint_started()` and `#0:checkpoint_finished()` are called during checkpoint execution (may occur after function returns)

**Examples:**
```moo
dump_database();
```

**Errors:**
- E_PERM: Caller is not a wizard

---

### 2.3 load_server_options

**Signature:** `load_server_options() → none`

**Description:** Reloads server options from `#0.server_options`.

**Permissions:** Wizard only.

**Behavior:**
- Re-reads `#0.server_options` map
- Validates option values (type checks, range checks)
- Updates running configuration
- Takes effect immediately

**Validation:**
- Type mismatch (e.g., string for integer option): raises E_INVARG
- Out-of-range values (e.g., negative timeout): raises E_INVARG
- Unknown option keys: silently ignored

**Examples:**
```moo
#0.server_options["bg_ticks"] = 50000;
load_server_options();  // Apply new limit
```

**Errors:**
- E_PERM: Caller is not a wizard
- E_INVARG: Invalid option type or value

---

## 3. Logging

### 3.1 server_log

**Signature:** `server_log(message [, level]) → none`

**Description:** Writes message to server log.

**Parameters:**
- `message` (STR): Message to log
- `level` (INT, optional): Log level (0=info, 1=warning, 2=error)

**Permissions:** Wizard only by default. May be configured via `server_options["protect_server_log"]` (0=wizard-only, 1=all).

**Examples:**
```moo
server_log("Player count: " + tostr(length(connected_players())));
server_log("Quota exceeded for #123", 1);  // Warning
```

**Errors:**
- E_PERM: Caller not authorized to log

---

## 4. Object Management

### 4.1 reset_max_object [Not Implemented]

**Signature:** `reset_max_object() → INT`

> **Note:** This function is documented in MOO specifications but not implemented in ToastStunt or Barn.

**Description:** Would reset the maximum object ID counter.

**Permissions:** Wizard only.

**Returns:** Would return new maximum object ID.

**Behavior:**
- Would scan all objects to find highest valid ID
- Would set counter to that value
- Next `create()` would use ID + 1

**Use case:** Reclaim IDs after mass object recycling.

**Examples:**
```moo
reset_max_object()  => 1523
```

---

### 4.2 renumber [Not Implemented]

**Signature:** `renumber(object) → OBJ`

> **Note:** This function is documented in MOO specifications but not implemented in ToastStunt or Barn.

**Description:** Would change an object's ID to the lowest available.

**Parameters:**
- `object` (OBJ): Object to renumber

**Permissions:** Wizard only.

**Returns:** Would return new object ID.

**Behavior:**
- Would find lowest unused object ID
- Would move object to that ID
- Would update all references (expensive operation)

**Examples:**
```moo
renumber(#9999)  => #42
```

---

## 5. Connection Management

### 5.1 connected_players

**Signature:** `connected_players([include_all]) → LIST`

**Description:** Returns list of connected player objects.

**Parameters:**
- `include_all` (BOOL, optional): Include unlogged connections (as negative IDs)

**Returns:** List of player object IDs.

**Examples:**
```moo
connected_players()     => {#5, #23, #107}
connected_players(1)    => {#5, #23, #107, -1, -2}  // -1, -2 are unlogged
```

---

### 5.2 connection_name

**Signature:** `connection_name(player) → STR`

**Description:** Returns connection identifier (usually IP/hostname).

**Parameters:**
- `player` (OBJ or connection identifier): Connected player or unlogged connection

**Returns:** Connection identifier string (typically IP address or hostname).

**Examples:**
```moo
connection_name(#5)  => "192.168.1.100"
connection_name(-1)  => "192.168.1.200"  // Unlogged connection
```

**Errors:**
- E_INVARG: Player/connection not connected

---

### 5.3 boot_player

**Signature:** `boot_player(player) → none`

**Description:** Forcibly disconnects a player.

**Parameters:**
- `player` (OBJ): Player to disconnect

**Permissions:** Wizard, or disconnecting self.

**Behavior:**
- Calls `#0:user_disconnected(player)`
- Closes connection
- Sends boot message

**Examples:**
```moo
boot_player(#troublemaker);
```

**Errors:**
- E_PERM: Not authorized to boot this player
- E_INVARG: Player not connected

---

### 5.4 notify

**Signature:** `notify(player, message [, no_flush]) → none`

**Description:** Sends message to player's connection.

**Parameters:**
- `player` (OBJ): Target player
- `message` (STR): Message to send
- `no_flush` (BOOL, optional): Don't flush buffer immediately

**Examples:**
```moo
notify(player, "Hello, world!");
```

**Errors:**
- E_INVARG: Player not connected

---

### 5.5 buffered_output_length [Not Implemented]

**Signature:** `buffered_output_length(player) → INT`

> **Note:** This function is documented in MOO specifications but not implemented in ToastStunt or Barn.

**Description:** Would return bytes pending in output buffer.

**Parameters:**
- `player` (OBJ): Connected player

**Returns:** Would return number of bytes buffered.

**Examples:**
```moo
buffered_output_length(#5)  => 1234
```

---

## 6. Network

### 6.1 open_network_connection

**Signature:** `open_network_connection(host, port) → OBJ`

**Description:** Opens outbound network connection.

**Parameters:**
- `host` (STR): Hostname or IP address
- `port` (INT): Port number

**Permissions:** Wizard only.

**Returns:** Connection object (for use with `read()`, `notify()`).

**Errors:**
- E_PERM: Caller is not a wizard
- E_INVARG: Invalid host or port
- E_QUOTA: Connection limit reached

---

### 6.2 listen

**Signature:** `listen(object, point [, print_messages]) → none`

**Description:** Creates a network listener.

**Parameters:**
- `object` (OBJ): Object to receive connections
- `point` (ANY): Port number or descriptor
- `print_messages` (BOOL, optional): Send system messages to connections

**Permissions:** Wizard only.

**Errors:**
- E_PERM: Caller is not a wizard
- E_INVARG: Invalid parameters

---

### 6.3 unlisten

**Signature:** `unlisten(point) → none`

**Description:** Removes a network listener.

**Parameters:**
- `point` (ANY): Port or descriptor to stop listening on

**Permissions:** Wizard only.

---

### 6.4 listeners

**Signature:** `listeners() → LIST`

**Description:** Returns list of active listeners.

**Returns:** List of `{object, point, print_messages}` tuples.

---

## 7. Error Summary

| Error | Conditions |
|-------|------------|
| E_PERM | Non-wizard calling privileged function |
| E_INVARG | Invalid object, player not connected |
| E_QUOTA | Resource limit exceeded |

---

## 8. Go Implementation Notes

### 8.1 Permissions

```go
func requireWizard(caller Objid) error {
    if !isWizard(caller) {
        return E_PERM
    }
    return nil
}
```

### 8.2 Shutdown

Use `context.Context` cancellation to signal shutdown across goroutines.

### 8.3 Logging

Use Go's `log` package or structured logging (e.g., `slog`).
