# Server Specification Patch Resolution Report

**Date:** 2025-12-24

**Summary:** Resolved 26 gaps identified in verification report. Most gaps were real missing details that needed clarification. Some were intentionally left implementation-defined with explicit notes added.

---

## spec/server.md (8 gaps)

### 1. Server Started Hook Timing
**RESOLUTION:** Patched
**CHANGE:** Clarified step 2 to specify "Initialize network listeners (create sockets, bind ports, but do not accept yet)" and step 4 to "Begin accepting connections on initialized listeners". This makes clear that listeners are created but not accepting before server_started() is called.

### 2. Task Scheduling Details
**RESOLUTION:** Intentional
**CHANGE:** Added explicit note: "Scheduler picks next ready task (implementation-defined: FIFO, priority, or other)" and "Tasks with identical expiry times are scheduled in creation order". This preserves Go implementation flexibility while specifying deterministic behavior for same-time tasks.

### 3. Connection Object Representation
**RESOLUTION:** Intentional (already noted in login.md)
**CHANGE:** Already marked as implementation-defined in login.md. Cross-reference maintained. Connection type is deliberately flexible (negative INT, OBJ, or other identifier).

### 4. Checkpoint Atomicity During Runtime
**RESOLUTION:** Intentional
**CHANGE:** Added note: "Database consistency during checkpoint is implementation-defined (may use copy-on-write, fork, or snapshot)". This allows Go to choose appropriate concurrency strategy without over-specifying.

### 5. Panic Shutdown "Best-Effort" Dump
**RESOLUTION:** Patched
**CHANGE:** Clarified to "best-effort: try to write current state, may be partial or incomplete". Makes clear it's an attempt without guarantees.

### 6. Player Disconnection Task Handling
**RESOLUTION:** Patched
**CHANGE:** Changed from implementation choice to specified behavior: "Associated foreground tasks are killed. Associated background tasks continue running (implementation may allow configuration via server_options)". This matches typical MOO behavior while allowing configuration.

### 7. Task Error "Logged" Location
**RESOLUTION:** Patched
**CHANGE:** Clarified "Error logged to server log (implementation-defined: file, stderr, logging system)". Specifies destination category while allowing implementation flexibility.

### 8. Shutdown "Implementation-Defined Message"
**RESOLUTION:** Patched
**CHANGE:** Reordered shutdown sequence to clarify: shutdown_started() is called first, then "Notify connected players (message parameter from shutdown(), or default implementation-defined message)". Makes clear the message source priority.

---

## spec/login.md (6 gaps)

### 1. Connection Object Type
**RESOLUTION:** Intentional
**CHANGE:** Updated to "Connection identifier (implementation-defined type: may be negative INT, OBJ, or other unique identifier)". This is deliberately flexible - Go can choose what works best.

### 2. Connection Timeout Hook
**RESOLUTION:** Patched
**CHANGE:** Clarified to "user_disconnected(connection) called if do_login_command was ever invoked for this connection". Makes the condition explicit and testable.

### 3. Reconnection Boot Behavior
**RESOLUTION:** Patched
**CHANGE:** Changed from implementation choice to specified: "Boot existing connection (send disconnect message, close socket)". This is standard behavior across MOO servers.

### 4. Boot Message Content/Delivery
**RESOLUTION:** Patched
**CHANGE:** Clarified sequence: "Send disconnect message to player (implementation-defined default, may be overridden by MOO code)". Specifies that there IS a message, format is flexible.

### 5. Output Flush Timing
**RESOLUTION:** Patched
**CHANGE:** Added cross-reference: "When buffer exceeds max_queued_output limit from server_options (default: 65536 bytes)". Links to server_options spec.

### 6. Connection Switching
**RESOLUTION:** Noted
**CHANGE:** Clarified to "Some servers support a switch_player(old, new) builtin to change which player object a connection represents (implementation-optional)". Makes clear this is not required.

---

## spec/builtins/server.md (7 gaps)

### 1. server_version() Format
**RESOLUTION:** Intentional
**CHANGE:** Updated to "Version identifier string (format implementation-defined, conventionally 'Name Major.Minor.Patch')". Gives guidance without mandating format.

### 2. memory_usage() Return Format
**RESOLUTION:** Intentional
**CHANGE:** Expanded with examples: "Format is implementation-defined. Common implementations return {total_bytes, used_bytes, free_bytes} or a map of metric names to values." Provides guidance with examples.

### 3. shutdown() Hook Calling
**RESOLUTION:** Patched
**CHANGE:** Added to behavior section: "Hook calling order: #0:shutdown_started() is called before stopping connections". Explicit ordering specified.

### 4. dump_database() Blocking
**RESOLUTION:** Patched
**CHANGE:** Clarified: "Returns immediately (does not block). Triggers asynchronous checkpoint sequence. Hooks are called during checkpoint execution (may occur after function returns)". Makes timing explicit.

### 5. load_server_options() Validation
**RESOLUTION:** Patched
**CHANGE:** Added validation section: "Type mismatch (e.g., string for integer option): raises E_INVARG. Out-of-range values (e.g., negative timeout): raises E_INVARG. Unknown option keys: silently ignored." Complete error handling specified.

### 6. server_log() Authorization
**RESOLUTION:** Patched
**CHANGE:** Clarified: "Wizard only by default. May be configured via server_options['protect_server_log'] (0=wizard-only, 1=all)." Follows standard protect_* pattern.

### 7. Connection Name for Unlogged
**RESOLUTION:** Patched
**CHANGE:** Updated signature to accept "OBJ or connection identifier" and added example with negative ID. Makes clear it works on unlogged connections.

---

## spec/objects.md Section 12 (5 gaps)

### 1. Hook Error Handling
**RESOLUTION:** Patched (in login.md)
**CHANGE:** Updated hook error text to: "Errors in lifecycle hooks are logged to server log (implementation-defined format: may include traceback, error message, hook name) but do not abort the operation. The connection proceeds normally. MOO code cannot check hook execution status - errors are visible only in server log."

### 2. server_options Defaults
**RESOLUTION:** Patched
**CHANGE:** Added note in table: "dump_interval and checkpoint_interval are aliases for the same setting. Implementations should accept both names." Resolves naming ambiguity.

### 3. server_options Type Validation
**RESOLUTION:** Patched
**CHANGE:** Added to minimal database section: "Type validation: Setting server_options to invalid values (wrong types, out-of-range) will cause load_server_options() to raise E_INVARG." Links to builtin spec.

### 4. Minimal Database Wizard Count
**RESOLUTION:** Patched
**CHANGE:** Updated to "At least one wizard object (recommended but not strictly required)" with explanation that wizard is needed for wizard-only builtins. Makes requirement practical, not absolute.

### 5. Bootstrap Sequence Tooling
**RESOLUTION:** Patched
**CHANGE:** Completely rewrote section to clarify: "No builtin bootstrap mechanism is required. Creating a minimal database from scratch requires manual database file creation or external tooling." Lists what implementations MAY provide. Makes clear MOO code cannot bootstrap itself.

---

## Summary Statistics

**Total gaps resolved:** 26
- **Patched (real gaps):** 19
- **Intentional (explicitly noted):** 6
- **Already handled:** 1

**Specification changes:**
- spec/server.md: 7 edits
- spec/login.md: 7 edits
- spec/builtins/server.md: 7 edits
- spec/objects.md: 4 edits

**Pattern analysis:**
- Most gaps were timing/ordering ambiguities (8 gaps)
- Error handling details were underspecified (5 gaps)
- Type flexibility vs. specification (4 gaps)
- Cross-referencing between specs (3 gaps)
- Default values and validation (6 gaps)

**No over-specification:** All "implementation-defined" notes are for areas where Go should have flexibility (scheduling algorithms, log formats, connection representation). Core behavioral contracts are now explicit.
