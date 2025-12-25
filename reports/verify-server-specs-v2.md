# Server Specification Verification Report
**Date:** 2025-12-24
**Task:** Verify each specification is implementable without guessing

---

## spec/server.md
**STATUS:** GAPS
**GAPS FOUND:** 6

1. **Checkpoint trigger signals:** "Signal | External signal (implementation-defined)" - which signals? SIGTERM? SIGHUP? Undefined.

2. **Task scheduling algorithm:** "implementation-defined: FIFO, priority, or other" - no clear algorithm specified. "Tasks with identical expiry times are scheduled in creation order" is clear, but base scheduling is underspecified.

3. **Background task continuation on disconnect:** "Associated background tasks continue running (implementation may allow configuration via server_options)" - which server_option key? Not defined in §5.3 table.

4. **Database consistency during checkpoint:** "implementation-defined (may use copy-on-write, fork, or snapshot)" - no specific mechanism required, cannot implement without choosing one.

5. **Checkpoint hooks error handling:** "Logged but do not abort server" - what log format? Where? stderr? file? syslog? Undefined.

6. **Shutdown default message:** "default implementation-defined message" (§4.1) - no default message specified.

---

## spec/login.md
**STATUS:** GAPS
**GAPS FOUND:** 5

1. **Connection identifier type:** "implementation-defined type: may be negative INT, OBJ, or other unique identifier" - which type should be used? No guidance.

2. **Return value for string:** "String: Send this message to connection (optional in some servers)" - is this supported or not? Conditional behavior creates ambiguity.

3. **Timeout behavior:** "`#0:user_disconnected(connection)` called if `do_login_command` was ever invoked for this connection" - ambiguous when this happens. Does initial connection count? Or only after first command?

4. **Hook error log format:** "implementation-defined format: may include traceback, error message, hook name" - what format? Where logged?

5. **Boot message:** "Send disconnect message to player (implementation-defined default, may be overridden by MOO code)" (§7.2) - no default message specified, no mechanism to override defined.

---

## spec/database.md
**STATUS:** PASS
**GAPS FOUND:** 0

This specification is implementable. Python reference implementation (`lambdamoo-db-py`) provides concrete format. All value encodings, section orders, and structures are well-defined. While streaming vs. full-load is left to implementation, both are viable strategies that can be chosen without guessing format details.

---

## spec/builtins/server.md
**STATUS:** GAPS
**GAPS FOUND:** 4

1. **server_version format:** "format implementation-defined, conventionally 'Name Major.Minor.Patch'" - convention noted but not required. Implementor cannot know if deviation breaks MOO code expectations.

2. **memory_usage return format:** "Format is implementation-defined" with two different examples (list vs map) - which one? No clear choice.

3. **shutdown hook calling order ambiguity:** "Hook calling order: `#0:shutdown_started()` is called before stopping connections" - but §2.1 shutdown sequence says "Run #0:shutdown_started(message)" as step 1 before step 2 "Stop accepting new connections". Unclear if this means before stopping acceptance or before closing all connections.

4. **Logging destination:** "Use Go's `log` package or structured logging (e.g., `slog`)" - implementation note, but `server_log()` doesn't specify where logs go (file? stderr? syslog?). MOO code may expect to read log files.

---

## spec/objects.md (Section 12 only - System Objects)
**STATUS:** GAPS
**GAPS FOUND:** 3

1. **maxint/minint exact values:** "Maximum integer value (2^63-1 for 64-bit)" - assumes 64-bit, but no requirement stated. Could be 32-bit? 128-bit? Spec should mandate bit width.

2. **server_options aliases:** "dump_interval and checkpoint_interval are aliases for the same setting. Implementations should accept both names." - which is canonical? What happens if both are set to different values? Undefined.

3. **Bootstrap tooling:** "No builtin bootstrap mechanism is required" but "MOO code cannot bootstrap itself" - spec states no requirement but also states it's impossible from MOO code. Implementor must guess whether to provide tooling or require manual database creation. Not a technical gap but a process gap.

---

## Summary

- **spec/server.md:** 6 gaps - underspecified behaviors around signals, scheduling, logging, checkpoint mechanisms
- **spec/login.md:** 5 gaps - connection identifiers, optional features, message defaults
- **spec/database.md:** 0 gaps - complete specification with reference implementation
- **spec/builtins/server.md:** 4 gaps - format ambiguities, logging destination unclear
- **spec/objects.md §12:** 3 gaps - integer sizes, configuration ambiguities, bootstrap process

**Total gaps: 18**

**Implementable without gaps:** spec/database.md only.
