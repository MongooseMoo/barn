# MOO Server Administration Built-ins

## Overview

Functions for server administration, monitoring, and control.

---

## 1. Server Information

### 1.1 server_version

**Signature:** `server_version([detail]) → ANY`

**Description:** Without arguments, returns the implementation's version string. An empty string or non-string argument returns the complete version information list.

**Returns:** The information list contains `major`, `minor`, `release`, `ext`, `string`, `os`, `features`, `options`, and `source`. A nonempty string selects a key or slash-separated nested path; one trailing slash is accepted. Unknown keys and traversal through a non-list value raise `E_INVARG`. Version numbers and source metadata describe the running implementation.

The `options` group lists every compile option the build knows about, as
`{name, value}` pairs, whether or not the option is set: a defined flag
option's value is `{0}`, an undefined option's value is `#-1`, and an integer
option (compiled limits such as `MAX_QUEUED_OUTPUT`, `DEFAULT_MAX_STACK_DEPTH`,
`DEFAULT_FG_TICKS`, `DEFAULT_BG_TICKS`, `DEFAULT_FG_SECONDS`,
`DEFAULT_BG_SECONDS`, `DEFAULT_MAX_STRING_CONCAT`, `MIN_STRING_CONCAT_LIMIT`,
`DEFAULT_MAX_LIST_VALUE_BYTES`, `MIN_LIST_VALUE_BYTES_LIMIT`,
`DEFAULT_MAX_MAP_VALUE_BYTES`, `MIN_MAP_VALUE_BYTES_LIMIT`) reports its
compiled INT value. So `server_version("options/ONLY_32_BITS")` is `#-1` on a
64-bit build rather than raising `E_INVARG`.

**Examples:**
```moo
server_version("string") == server_version()  => 1
server_version("source/vcs")  // Version-control system, or "unknown"
```

---

### 1.2 memory_usage

**Signature:** `memory_usage() → LIST`

**Description:** Returns the server process's memory statistics.

**Returns:** A list of five FLOATs read from `/proc/self/statm`, in pages:
`{size, resident, share, text, data}` (total program size, resident set,
shared pages, text, data + stack). On macOS, which has no `/proc`, only the
resident set size is available: it is reported in bytes as element 2 and the
other four elements are `0.0`.

**Errors:**
- E_FILE: `/proc/self/statm` cannot be opened (macOS: the resident-size query fails)
- E_NACC: `/proc/self/statm` cannot be parsed

No permission check: any programmer may call it.

---

### 1.3 usage

**Signature:** `usage() → LIST`

**Description:** Returns system load and process resource usage.

**Permissions:** Wizard only (`E_PERM` otherwise). Never raises any other error.

**Returns:** A 10-element list:
1. `{load1, load5, load15}`: system load averages as INTs in `sysinfo()`
   fixed-point form (load × 65536), not FLOATs; all 0 if `sysinfo()` fails
2. User CPU time (FLOAT seconds)
3. System CPU time (FLOAT seconds)
4. Minor page faults (INT)
5. Major page faults (INT)
6. Block input operations (INT)
7. Block output operations (INT)
8. Voluntary context switches (INT)
9. Involuntary context switches (INT)
10. Signals received (INT)

Elements 2-10 come from `getrusage(RUSAGE_SELF)`.

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

**Description:** Requests a database checkpoint.

**Permissions:** Wizard only.

**Behavior:**
- Only records the request and returns 0 at once (does not block)
- The server performs the checkpoint after the calling task's run has
  finished, so `#0:checkpoint_started()`, the checkpoint itself and
  `#0:checkpoint_finished()` observe every write the caller made after
  calling `dump_database()`

**Examples:**
```moo
dump_database();
```

**Errors:**
- E_PERM: Caller is not a wizard

---

### 2.3 load_server_options

**Signature:** `load_server_options() → none`

**Description:** Refreshes the server's cached option values from the
properties of the `$server_options` object (`#0.server_options` must hold a
valid object; each option is a property on it).

**Permissions:** Wizard only.

**Behavior:**
- Re-reads every cached option: each `protect_<builtin>` flag, each
  `protect_<builtin-property>` flag, and the cached miscellaneous options
  (`max_list_value_bytes`, `max_map_value_bytes`, `max_string_concat`,
  `max_concat_catchable`, `include_rt_vars`, `max_queued_output`, ...).
- An option of the wrong type reads as its default; out-of-range values are
  clamped to the compiled limits. Nothing is rejected: the call never raises
  `E_INVARG` and returns 0.
- Uncached options (for example `fg_ticks`, `bg_ticks`,
  `pcre_match_max_iterations`, `sqlite_max_handles`) are read from
  `$server_options` each time they are used and need no reload.
- The refresh takes effect in the calling task before it finishes: a lowered
  `max_string_concat` makes the next concatenation in the same task raise
  `E_QUOTA`, and `yin()` in the same task validates against a raised
  `fg_ticks`. The exception is `max_stack_depth`, which is sampled when a task
  starts: raising it and reloading does not deepen the running task's stack
  (recursing past the old limit still raises `E_MAXREC`), but tasks started
  afterwards get the new depth.

**Examples:**
```moo
add_property($server_options, "max_string_concat", 100000, {player, "r"});
load_server_options();  // Apply new limit
```

**Errors:**
- E_PERM: Caller is not a wizard

### 2.4 Protected Builtins

When `$server_options.protect_<name>` is true (as cached by
`load_server_options()`), every call to builtin `<name>` from a frame whose
`this` is not `#0` is redirected to `#0:bf_<name>(@args)`, and the builtin
call returns whatever that verb returns. The redirect does not depend on the
caller's wizard bit: a wizard's eval frame and a verb on an ordinary object
both get the wrapper's result. The wrapper itself (`this == #0`) reaches the
real builtin.

If `#0:bf_<name>` does not exist (or the call fails), a wizard falls through to
the real builtin and a non-wizard gets `E_PERM`. `E_MAXREC` from the wrapper
call is returned as `E_MAXREC`.

```moo
// with $server_options.protect_valid = 1 and
// #0:bf_valid = "return valid(args[1]) + 10;"  then load_server_options():
{valid(#0), valid(#-1)}   // wizard eval
=> {11, 10}
```

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

**Description:** Moves an object to the lowest free object number below its
current one.

**Parameters:**
- `object` (OBJ): Object to renumber

**Permissions:** Wizard only.

**Returns:** New object ID, or `object` itself when no lower number is free.

**Behavior:**
- Scans numbers from `#0` up to (not including) `object` for the first one with
  no numbered object (never created, or recycled) and moves the object there
- Anonymous objects take no object number, so a live or recycled anonymous
  object never becomes the target: with no numbered hole below it, the object
  keeps its number
- Rewrites the parent/child and location/contents hierarchies, the player
  list, and the owners of objects, verbs and properties to the new number; an
  object owned by the number taken over gets owner `#-1`. Other references
  (property values, verb code) are not rewritten
- The renumbered object is usable at its new number in the same task

**Examples:**
```moo
renumber(#9999)  => #42
```

**Errors:**
- E_INVARG: Invalid object (checked first)
- E_PERM: Caller is not a wizard

---

## 5. Connection Management

### 5.1 connected_players

**Signature:** `connected_players([include_all]) → LIST`

**Description:** Returns list of connected player objects.

**Parameters:**
- `include_all` (BOOL, optional): Include unlogged connections (as negative IDs)

**Returns:** List of player object IDs. Without `include_all`, only
connections that have a connection time are listed: a connection gets one when
it logs in, or when `switch_player()` moves it to a player (even if
`do_login_command` has not returned). A connection being booted or
disconnected is left out at once, before its disconnect is processed.
`connected_seconds()` follows the same rule (`E_INVARG` for a connection with
no connection time).

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

**Signature:** `notify(conn, message [, no_flush [, no_newline]]) → INT`

**Description:** Queues `message` as output on `conn`'s connection, followed by
a newline (sent as CR LF) unless `no_newline` is true. On a connection in
binary mode the message is decoded from MOO binary string form (`~XX`
escapes) and sent as raw bytes with no newline.

**Parameters:**
- `conn` (OBJ): Target player or connection
- `message` (STR): Message to send
- `no_flush` (optional): If true, the line may be refused instead of discarding older queued output
- `no_newline` (optional): If true, no line terminator is added

**Returns:** 1 when the line is queued. 1 also when `conn` has no connection
(the message is dropped). 0 when `no_flush` is true, output is already queued,
and the line would push the queue past `max_queued_output` even after the
server has written what the peer accepts; the line is not queued. Without
`no_flush`, older queued output is discarded to make room and the call
returns 1.

**Permissions:** The caller's permissions must be a wizard or `conn` itself;
otherwise `E_PERM`, checked before looking up the connection (so a
non-wizard notifying a disconnected other target also gets `E_PERM`).

**Examples:**
```moo
notify(player, "Hello, world!")   => 1
```

**Errors:**
- E_PERM: Caller is neither a wizard nor `conn`
- E_INVARG: Binary-mode connection and `message` is not a valid binary string

---

### 5.5 buffered_output_length

**Signature:** `buffered_output_length([conn]) → INT`

**Description:** With `conn`, returns the number of bytes queued for output on
that connection: the wire bytes of each queued line, including its CR LF
terminator (`notify(c, "abc", 1)` adds 5; with `no_newline` it adds 3), and
the decoded byte count of binary output (`"~41~42"` adds 2). Within one task
the count accumulates across successive `notify()` calls.

With no argument, returns the server's output capacity, the cached
`max_queued_output` option (default 65536). It follows `load_server_options()`
in the same task. A positive value below 2048 is raised to 2048; zero and
negative values are kept as given; a non-integer value reads as the default.

**Examples:**
```moo
buffered_output_length(#5)  => 1234
buffered_output_length()    => 65536
```

**Errors:**
- E_INVARG: `conn` not connected
- E_PERM: Caller is neither a wizard nor `conn` (checked after the connection lookup)

---

## 6. Network

### 6.1 open_network_connection

**Signature:** `open_network_connection(host, port [, options]) → OBJ`

**Description:** Opens an outbound network connection (only in builds with
`OUTBOUND_NETWORK`; otherwise every call raises `E_PERM`).

**Parameters:**
- `host` (STR): Hostname or IP address
- `port` (INT): Port number
- `options` (MAP, optional), keys matched without regard to case:
  - `"ipv6"`: true resolves the host as IPv6; otherwise IPv4 only
  - `"listener"` (OBJ): object that handles the connection; a non-object raises `E_TYPE` with message `"listener should be an object"` and the value
  - `"tls"`: true makes the connection a TLS client (`E_PERM`, `"TLS is not enabled"`, if the server has no TLS context)
  - `"tls_verify"`: read but has no effect

**Permissions:** Wizard only (checked before the options).

**Returns:** The new connection's negative object number (for use with
`read()`, `notify()`). The connect, including any TLS handshake, must finish
within `$server_options.outbound_connect_timeout` seconds (default 5).

**Connection behavior:** Without `"listener"` the connection's handler is `#0`,
it gets no initial empty input line, and it does not print connection
messages; with `"listener" -> OBJ` that object handles it (using that
listener's print-messages setting, or none for an object that is not a
listener) and it gets one initial empty input line, as an accepted connection
does. Outbound connections are never closed by `connect_timeout`.
`connection_info()` reports them with `"outbound" -> 1`. Connecting to one of
the server's own listeners gives two connections: the dialled end (returned)
and the accepted end.

**Errors:**
- E_PERM: Caller is not a wizard; outbound connections disabled (`"Outbound network connections are disabled."`)
- E_TYPE: `host` not STR or `port` not INT; `"listener"` not an object
- E_INVARG: `"getaddrinfo error"` (value: resolver error text) when the host has no address in the selected family; `"Connection failure"` (value: `strerror` text) when the connect is refused, unreachable, the address is unavailable, or it times out (including a SIGALRM-driven timeout); `"TLS connection failed"` (value: TLS error text)
- E_QUOTA: socket setup failure, or another connect failure (`"Connection failure"`)

---

### 6.2 listen

**Signature:** `listen(object, point [, options]) → INT`

**Description:** Creates a network listener.

**Parameters:**
- `object` (OBJ): Object to receive connections
- `point` (INT): Port number
- `options` (MAP, optional): Configuration map; keys are matched without regard to case:
  - `"print-messages"`: true to send system messages (default: 0)
  - `"ipv6"`: true for IPv6 (default: IPv4)
  - `"interface"` (STR): Interface name or address to bind to (default: all interfaces); a non-string value is ignored
  - `"tls"`: true makes the listener TLS (`E_INVARG`, `"TLS is not enabled"`, if the server has no TLS context). `"certificate"` and `"key"` (STR paths) are only used with `"tls"`; a non-string value raises `E_INVARG` with `"Certificate path should be a string"` / `"Private key path should be a string"`

Options are read before the permission check, so an option error is reported
even to a non-wizard.

**Returns:** The listener's canonical descriptor (the port number for a TCP port) on success.

**Permissions:** Wizard only.

**Examples:**
```moo
listen(#0, 9999)                                       => 9999
listen(#0, 8080, ["print-messages" -> 1])             => 8080
listen(#0, 7777, ["ipv6" -> 1, "interface" -> "::"])  => 7777
```

**Errors:**
- E_PERM: Caller is not a wizard (message `"Permission denied"`)
- E_INVARG: Option error (above); invalid object, or a listener with that descriptor already exists (`"Invalid argument"`)
- E_QUOTA: `"Failed to listen on port"`
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
- `"tls"` (INT): 1 if TLS (builds with TLS support)

With one argument, `listeners(find)` returns only the listeners whose object
(when `find` is an OBJ) or descriptor equals `find`.

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
