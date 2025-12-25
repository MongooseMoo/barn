# MOO Server Lifecycle Specification

## Overview

A MOO server manages object persistence, player connections, and task execution. This spec defines the required behaviors, not implementation details.

---

## 1. Startup Sequence

### 1.1 Initialization Order

1. Load database from disk
2. Initialize network listeners
3. Call `#0:server_started()` (no arguments)
4. Begin accepting connections
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
- Scheduler picks next ready task
- Forked tasks run after their delay expires

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
- If checkpoint fails, server continues (does not abort)

---

## 4. Shutdown

### 4.1 Graceful Shutdown

Triggered by:
- `shutdown([message])` builtin
- OS termination signal (SIGTERM, SIGINT equivalent)

**Sequence:**
1. Stop accepting new connections
2. Notify connected players (implementation-defined message)
3. Run `#0:shutdown_started(message)` if defined
4. Flush final checkpoint
5. Close all connections
6. Exit cleanly

### 4.2 Panic Shutdown

Triggered by:
- `panic([message])` builtin (wizard only)
- Unrecoverable internal error

**Sequence:**
1. Log error with stack trace if available
2. Attempt emergency database dump (best-effort)
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
- Associated tasks may continue or be killed (implementation choice)

---

## 7. Error Handling

### 7.1 Task Errors

Unhandled errors in tasks:
- Foreground: Error message sent to player
- Background: Error logged, task aborts silently

### 7.2 Hook Errors

Errors in server hooks (`server_started`, `checkpoint_finished`, etc.):
- Logged but do not abort server
- Server continues with default behavior

### 7.3 Database Corruption

If database cannot be loaded:
- Log error
- Exit with non-zero status
- Do not start network listeners

---

## 8. Go Implementation Notes

### 8.1 Natural Mappings

| MOO Concept | Go Idiom |
|-------------|----------|
| Main loop | `for { select { ... } }` |
| Shutdown signal | `context.Context` cancellation |
| Checkpoint trigger | Channel send |
| Connection handling | Goroutine per connection |
| Task scheduling | Goroutine + scheduler |
| Graceful shutdown | `sync.WaitGroup` for cleanup |

### 8.2 Atomicity

Use `os.Rename` for atomic checkpoint (write temp, rename).

### 8.3 Signals

Use `signal.Notify` for OS signals, but internal shutdown via context.

---

## 9. Differences from ToastStunt

This spec intentionally omits:
- Fork-based checkpointing (Go can checkpoint in-process or via goroutine)
- Signal-specific behavior (use Go's signal handling)
- C++ memory management details
- Platform-specific networking

The spec defines **observable behavior**, not implementation.
