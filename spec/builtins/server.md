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

### 1.2 memory_usage

**Signature:** `memory_usage() → LIST`

**Description:** Returns memory statistics.

**Returns:** List of memory metrics. Format is implementation-defined.

**Implementation Note:** May raise E_FILE if memory statistics are not available in the current implementation.

**Examples:**
```moo
memory_usage()  => {block_size, nused, nfree}
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

### 4.1 reset_max_object

**Signature:** `reset_max_object() → INT`

**Description:** Resets the maximum object ID counter to the highest valid object.

**Permissions:** Wizard only.

**Returns:** New maximum object ID.

**Behavior:**
- Scans all objects to find highest valid ID
- Sets counter to that value
- Next `create()` will use ID + 1

**Use case:** Reclaim IDs after mass object recycling.

**Examples:**
```moo
reset_max_object()  => 1523
```

**Errors:**
- E_PERM: Caller is not a wizard

---

### 4.2 renumber

**Signature:** `renumber(object) → OBJ`

**Description:** Changes an object's ID to the lowest available.

**Parameters:**
- `object` (OBJ): Object to renumber

**Permissions:** Wizard only.

**Returns:** New object ID.

**Behavior:**
- Finds lowest unused object ID
- Moves object to that ID
- Updates all references

**Examples:**
```moo
renumber(#9999)  => #42
```

**Errors:**
- E_PERM: Caller is not a wizard
- E_INVARG: Invalid object

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

### 5.5 buffered_output_length

**Signature:** `buffered_output_length(player) → INT`

**Description:** Returns bytes pending in output buffer.

**Parameters:**
- `player` (OBJ): Connected player

**Returns:** Number of bytes buffered for output.

**Examples:**
```moo
buffered_output_length(#5)  => 1234
```

**Errors:**
- E_INVARG: Player not connected

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

**Signature:** `listen(object, point [, options]) → INT`

**Description:** Creates a network listener.

**Parameters:**
- `object` (OBJ): Object to receive connections
- `point` (INT): Port number
- `options` (MAP, optional): Configuration map with keys:
  - `"print-messages"` (INT): 1 to send system messages, 0 to suppress (default: implementation-defined)
  - `"ipv6"` (INT): 1 for IPv6, 0 for IPv4 (default: 0)
  - `"interface"` (STR): Interface name or address to bind to (default: all interfaces)

**Returns:** Port number (INT) on success.

**Permissions:** Wizard only.

**Examples:**
```moo
listen(#0, 9999)                                       => 9999
listen(#0, 8080, ["print-messages" -> 1])             => 8080
listen(#0, 7777, ["ipv6" -> 1, "interface" -> "::"])  => 7777
```

**Errors:**
- E_PERM: Caller is not a wizard
- E_INVARG: Invalid parameters
- E_TYPE: Options is not a map

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

**Returns:** List of MAP values, each containing:
- `"object"` (OBJ): Object receiving connections
- `"port"` (INT): Port number
- `"print-messages"` (INT): 1 if system messages enabled, 0 otherwise
- `"ipv6"` (INT): 1 if IPv6, 0 if IPv4
- `"interface"` (STR): Interface name or address

**Examples:**
```moo
listeners()  => {["interface" -> "Empiricist", "ipv6" -> 1, "object" -> #0, "port" -> 7777, "print-messages" -> 1]}
```

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
