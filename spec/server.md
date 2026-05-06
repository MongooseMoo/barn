# MOO Server Lifecycle Specification

## Overview

A MOO server manages object persistence, player connections, and task execution. This spec defines the required behaviors, not implementation details.

---

## 1. Startup Sequence

### 1.1 Initialization Order

1. Load database from disk
2. Initialize network listeners (create sockets, bind ports, but do not accept yet)
3. Call `#0:server_started()` (no arguments)
4. Begin accepting connections on initialized listeners
5. Enter main loop

### 1.2 Server Started Hook

```moo
#0:server_started()
```

**Called:** Once, after database loaded, before connections accepted.

**Purpose:** Initialize runtime state, start background tasks, log startup.

**No return value expected.**

---

## 2. Main Loop

### 2.1 Responsibilities

The server continuously:
1. Accept new connections
2. Process incoming commands
3. Run ready tasks (scheduled, resumed, forked)
4. Send output to players
5. Handle checkpoints when requested
6. Clean up disconnected/recycled players

### 2.2 Task Scheduling

Tasks run cooperatively:
- One task runs until it suspends, completes, or exceeds limits
- Scheduler picks next ready task (implementation-defined: FIFO, priority, or other)
- Forked tasks run after their delay expires
- Tasks with identical expiry times are scheduled in creation order

---

## 3. Checkpoints (Database Persistence)

### 3.1 Trigger Methods

| Method | Description |
|--------|-------------|
| Timer | Periodic interval (configurable) |
| Builtin | `dump_database()` called |
| Signal | External signal (implementation-defined) |

### 3.2 Checkpoint Hooks

**Before checkpoint:**
```moo
#0:checkpoint_started()
```

**After checkpoint:**
```moo
#0:checkpoint_finished(success)
```
- `success`: 1 if successful, 0 if failed

### 3.3 Semantics

- Checkpoints persist all objects, properties, verbs, and queued tasks
- Checkpoint must be atomic (write to temp file, rename)
- Server continues running during checkpoint
- Database consistency during checkpoint is implementation-defined (may use copy-on-write, fork, or snapshot)
- If checkpoint fails, server continues (does not abort)

---

## 4. Shutdown

### 4.1 Graceful Shutdown

Triggered by:
- `shutdown([message])` builtin
- OS termination signal (SIGTERM, SIGINT equivalent)

**Sequence:**
1. Run `#0:shutdown_started(message)` if defined
2. Stop accepting new connections
3. Notify connected players (message parameter from shutdown(), or default implementation-defined message)
4. Flush final checkpoint
5. Close all connections
6. Exit cleanly

### 4.2 Panic Shutdown

Triggered by:
- `panic([message])` builtin (wizard only)
- Unrecoverable internal error

**Sequence:**
1. Log error with stack trace if available
2. Attempt emergency database dump (best-effort: try to write current state, may be partial or incomplete)
3. Exit immediately (non-zero status)

**No hooks called during panic.**

---

## 5. System Object (#0)

### 5.1 Required Properties

| Property | Type | Description |
|----------|------|-------------|
| `server_options` | MAP | Server configuration (see §5.3) |
| `maxint` | INT | Maximum integer value |
| `minint` | INT | Minimum integer value |

### 5.2 Required Verbs

| Verb | Signature | When Called |
|------|-----------|-------------|
| `server_started` | `()` | After DB load, before connections |
| `checkpoint_started` | `()` | Before checkpoint begins |
| `checkpoint_finished` | `(success)` | After checkpoint completes |
| `user_connected` | `(player)` | Player successfully logged in |
| `user_disconnected` | `(player)` | Player connection closed |
| `user_reconnected` | `(player)` | Player reconnected (same object) |
| `do_login_command` | `(player, command)` | Pre-login command processing |

### 5.3 Server Options

`#0.server_options` is a map controlling server behavior:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `bg_ticks` | INT | 30000 | Background task tick limit |
| `bg_seconds` | INT | 3 | Background task time limit |
| `fg_ticks` | INT | 60000 | Foreground task tick limit |
| `fg_seconds` | INT | 5 | Foreground task time limit |
| `max_stack_depth` | INT | 50 | Maximum call stack depth |
| `connect_timeout` | INT | 300 | Seconds before unlogged connection times out |
| `checkpoint_interval` | INT | 3600 | Seconds between automatic checkpoints |

---

## 6. Connection Lifecycle

### 6.1 States

```
Connect → Unlogged → Logged In → Disconnected
```

### 6.2 Unlogged Connections

New connections start unlogged:
- Commands routed to `#0:do_login_command(conn, line)`
- Login verb returns player object on success
- Timeout if no login within `connect_timeout` seconds

### 6.3 Logged-In Connections

After successful login:
- `#0:user_connected(player)` called
- Commands parsed and dispatched to verbs
- Output sent via `notify(player, message)`

### 6.4 Disconnection

When connection closes:
- `#0:user_disconnected(player)` called
- Pending output discarded
- Associated foreground tasks are killed
- Associated background tasks continue running (implementation may allow configuration via server_options)

---

## 7. Network Listeners

### 7.1 Listener Protocols

Barn listeners support four native protocols:

| Protocol | Transport | Input Model |
|----------|-----------|-------------|
| `tcp` | TCP byte stream | Telnet-style line input |
| `tls` | TLS over TCP | Telnet-style line input |
| `ws` | WebSocket over HTTP | Message input |
| `wss` | WebSocket over HTTPS/TLS | Message input |

Startup listener specs and the MOO `listen()` builtin use the same listener
representation. The `-port N` startup option is shorthand for one
`tcp://:N` listener.

### 7.2 Listener Options and Descriptors

`listen(object, port, [options])` accepts an optional map for extended
listener options:

| Key | Type | Applies To | Description |
|-----|------|------------|-------------|
| `protocol` | STR | all | `tcp`, `tls`, `ws`, or `wss`; default is `tcp` |
| `path` | STR | `ws`, `wss` | HTTP request path; default is `/` |
| `certificate` | STR | `tls`, `wss` | TLS certificate file path |
| `key` | STR | `tls`, `wss` | TLS private key file path |
| `print-messages` | INT | all | Whether listener connect/full/refuse messages are printed |

The canonical listener descriptor is:

| Protocol | Descriptor Returned by `listen()` |
|----------|-----------------------------------|
| `tcp` | INT port |
| `tls` | MAP with `protocol` and `port` |
| `ws` | MAP with `protocol`, `port`, and `path` |
| `wss` | MAP with `protocol`, `port`, and `path` |

The canonical descriptor identifies exactly one listener. `unlisten()` accepts
the canonical descriptor. For compatibility, `unlisten(port)` means
`unlisten(["protocol" -> "tcp", "port" -> port])`.

`listeners([find])` returns a list of maps with these fields for every protocol:

| Key | Type | Description |
|-----|------|-------------|
| `object` | OBJ | Listener object that owns login processing |
| `port` | INT | Bound local port |
| `protocol` | STR | `tcp`, `tls`, `ws`, or `wss` |
| `path` | STR | WebSocket path, or empty string for non-WebSocket listeners |
| `print-messages` | INT | Listener message flag |
| `ipv6` | INT | Nonzero when the listener is IPv6 |
| `interface` | STR | Bound interface text |
| `TLS` | INT | Nonzero for `tls` and `wss` |

`listeners(find)` accepts a listener object, a legacy integer port, or a
canonical descriptor map.

### 7.3 WebSocket HTTP Semantics

For `ws` and `wss`, only requests whose path exactly equals the listener path
are eligible for upgrade. Matching is byte-for-byte after the HTTP server has
parsed the path. Query strings are ignored for path matching. Path matching is
case-sensitive. No prefix matching is performed.

The default origin policy is open: WebSocket upgrade requests are accepted
regardless of the `Origin` header. Client-supplied forwarding headers such as
`Forwarded`, `X-Forwarded-For`, and `X-Real-IP` are ignored by connection
metadata unless a future trusted-proxy option explicitly enables them.

A non-WebSocket HTTP request to a WebSocket listener receives `426 Upgrade
Required` and does not create a Barn connection. A WebSocket upgrade request for
the wrong path receives `404 Not Found` and does not create a Barn connection.

### 7.4 WebSocket Connection Metadata

After a successful WebSocket upgrade, the connection enters the normal Barn
connection lifecycle. The listener object owns `do_login_command()` and the
same login, reconnect, disconnect, and notifier hooks used by TCP and TLS.

`connection_name(player, 0)` keeps the legacy format:

```moo
"port <listen-port> from <remote-host>, port <remote-port>"
```

For `connection_info(player)`, `source_port` is the accepting listener's local
port and destination address fields are derived from the HTTP request's remote
socket address. The `protocol` field remains the IP family string (`IPv4` or
`IPv6`) for compatibility; listener protocol is reported through `listeners()`.

### 7.5 WebSocket Input Framing

One complete WebSocket text message is one MOO input line. The message payload
is delivered without adding or requiring CRLF framing.

Text messages containing `\r` or `\n` are invalid input. The server closes the
WebSocket with a policy/protocol error and does not split the payload into
multiple MOO commands.

Binary WebSocket messages are invalid input. The server closes the WebSocket
with an unsupported-data or policy/protocol error and does not expose the bytes
to MOO code.

Valid UTF-8 text payloads may contain high-byte Unicode code points. Invalid
UTF-8 text payloads are invalid input and close the WebSocket before the
payload is delivered.

WebSocket ping and pong control frames are transport-level events. They do not
surface as MOO input, do not affect idle command text, and do not call login or
command verbs.

Client close frames close the Barn connection and trigger the normal disconnect
lifecycle. Server-initiated disconnects send a WebSocket close frame when the
connection is still writable, then close the underlying connection.

The `connect_timeout` server option applies after a successful WebSocket
upgrade. A connection that remains unlogged past the timeout is closed like an
unlogged TCP or TLS connection.

### 7.6 WebSocket Output Framing

Each logical output line sent to a WebSocket connection is one WebSocket text
message. TCP/TLS newline bytes are not appended to the WebSocket payload.

`notify(player, message, no_flush, no_newline)` keeps its TCP/TLS meaning for
byte-stream transports. For WebSocket transports, `no_newline` is ignored
because WebSocket output is message-framed, not newline-framed. The `no_flush`
argument still buffers output; flushing sends each buffered logical message as
its own WebSocket text message.

Out-of-band lines beginning with `#$#` are application text on WebSocket
connections. They are sent and received as ordinary text messages and are not
implemented as WebSocket control frames.

The `flush-command` connection option is matched against a complete incoming
WebSocket text message. When it matches, held commands are flushed exactly as
they are for TCP/TLS.

The `binary` connection option does not change WebSocket framing. WebSocket
connections continue to accept only complete valid UTF-8 text messages as MOO
input and send text messages as output.

---

## 8. Error Handling

### 8.1 Task Errors

Unhandled errors in tasks:
- Foreground: Error message sent to player via notify()
- Background: Error logged to server log (implementation-defined: file, stderr, logging system), task aborts silently

### 8.2 Hook Errors

Errors in server hooks (`server_started`, `checkpoint_finished`, etc.):
- Logged but do not abort server
- Server continues with default behavior

### 8.3 Database Corruption

If database cannot be loaded:
- Log error
- Exit with non-zero status
- Do not start network listeners

---

## 9. Go Implementation Notes

### 9.1 Natural Mappings

| MOO Concept | Go Idiom |
|-------------|----------|
| Main loop | `for { select { ... } }` |
| Shutdown signal | `context.Context` cancellation |
| Checkpoint trigger | Channel send |
| Connection handling | Goroutine per connection |
| Task scheduling | Goroutine + scheduler |
| Graceful shutdown | `sync.WaitGroup` for cleanup |

### 9.2 Atomicity

Use `os.Rename` for atomic checkpoint (write temp, rename).

### 9.3 Signals

Use `signal.Notify` for OS signals, but internal shutdown via context.

---

## 10. Differences from ToastStunt

This spec intentionally omits:
- Fork-based checkpointing (Go can checkpoint in-process or via goroutine)
- Signal-specific behavior (use Go's signal handling)
- C++ memory management details
- Platform-specific networking

The spec defines **observable behavior**, not implementation.
