# Server Specification Verification Report

**Task:** Evaluate whether each specification provides sufficient detail for blind implementation without guessing.

**Date:** 2025-12-24

**Methodology:** Read each spec as if implementing from scratch, identifying any missing or ambiguous details that would require guessing or referencing external code.

---

## spec/server.md
**STATUS:** GAPS
**GAPS FOUND:** 8

### Missing Details:

1. **Server Started Hook Timing**: "After database loaded, before connections accepted" - but what about "initialize network listeners" (step 2)? Are listeners created before or after server_started()? Step order shows listeners at step 2, hook at step 3, but this contradicts "before connections accepted" if listeners are already up.

2. **Task Scheduling Details**: "Scheduler picks next ready task" - no algorithm specified. FIFO? Priority? Random? How are forked tasks with same expiry time ordered?

3. **Connection Object Representation**: `do_login_command(connection, line)` receives a "connection" - what type is this? OBJ? INT? STRING? The spec says "implementation-defined" but doesn't specify what MOO type the verb receives.

4. **Checkpoint Atomicity During Runtime**: "Server continues running during checkpoint" - does this mean concurrent writes can happen during checkpoint? Are there any locks or copy-on-write semantics? What's the consistency model?

5. **Panic Shutdown "Best-Effort" Dump**: What does "best-effort" mean exactly? Try to write full database? Write partial? Just object count? No guidance on what to attempt.

6. **Player Disconnection Task Handling**: "Associated tasks may continue or be killed (implementation choice)" - this is explicitly punting a behavioral decision. Two implementations could behave differently.

7. **Task Error "Logged" Location**: "Error logged" - where? To what? File? Server log? Specific format? How does MOO code access these logs?

8. **Shutdown "Implementation-Defined Message"**: What message? What format? Is this the shutdown message argument or something else? Does this go to notify() or some other channel?

---

## spec/login.md
**STATUS:** GAPS
**GAPS FOUND:** 6

### Missing Details:

1. **Connection Object Type**: Same as server.md - `do_login_command(connection, line)` receives "connection" but type is "implementation-defined". Can't implement MOO builtin signatures without knowing the type.

2. **Connection Timeout Hook**: "user_disconnected(connection) called (if connection had any state)" - when does a connection have state vs. not? Is this just "was do_login_command ever called"? Ambiguous.

3. **Reconnection Boot Behavior**: "Boot existing connection (implementation choice: immediate or graceful)" - another explicit choice point. Two implementations could differ.

4. **Boot Message Content/Delivery**: `boot_player()` "Send disconnect message" - what message? Where does it come from? Is this a hardcoded string? From a property? To the booted player or the booter?

5. **Output Flush Timing**: "When buffer exceeds limit" - what limit? Is this max_queued_output from server_options? Not cross-referenced.

6. **Connection Switching**: Section 8.2 mentions "some servers support switch_player(old, new)" - is this a builtin? A verb? What's the signature? What's the semantics? Too vague.

---

## spec/database.md
**STATUS:** PASS
**GAPS FOUND:** 0

### Assessment:

This spec is well-defined:
- File format structure is explicit and ordered
- Type codes are enumerated with exact formats
- Object/property/verb serialization is detailed
- Examples are provided for encoding
- References external implementation (lambdamoo-db-py) for ambiguities
- Round-trip integrity requirement is clear
- Atomic write protocol is specified

Could implement a reader/writer from this spec alone, using lambdamoo-db-py as oracle for edge cases as instructed.

---

## spec/builtins/server.md
**STATUS:** GAPS
**GAPS FOUND:** 7

### Missing Details:

1. **server_version() Format**: Returns "version identifier (e.g., 'Barn 1.0.0')" - is this a requirement or just an example? Is there a required format? Does MOO code expect to parse this?

2. **memory_usage() Return Format**: "List of memory metrics (format implementation-defined)" - completely unspecified. Can't write MOO code that uses this without guessing the structure.

3. **shutdown() Hook Calling**: Does shutdown() call #0:shutdown_started() before or after "Stop accepting new connections"? Order matters for MOO code behavior.

4. **dump_database() Blocking**: "Does not block (checkpoint may be asynchronous)" - so does it return before checkpoint_started() is called? Or just before checkpoint finishes? When do hooks run relative to function return?

5. **load_server_options() Validation**: What happens if server_options contains invalid values? Type errors? Out-of-range integers? Does it raise E_INVARG? Silently ignore? Clamp to valid range?

6. **server_log() Authorization**: "Wizard only (or configurable)" - configurable how? Via a server_options key? A property? What's the exact check?

7. **Connection Name for Unlogged**: Can connection_name() be called on unlogged connections (negative IDs from connected_players(1))? Spec doesn't say.

---

## spec/objects.md (Section 12 Only - System Objects)
**STATUS:** GAPS
**GAPS FOUND:** 5

### Missing Details:

1. **Hook Error Handling**: "Errors in lifecycle hooks are logged but do not abort the operation" - what does "logged" mean? Where? How can MOO code check if a hook failed? Is there a traceback?

2. **server_options Defaults**: Table shows "Default" column but several keys from server.md are missing (like checkpoint_interval vs dump_interval - are these the same? Different names for same thing?).

3. **server_options Type Validation**: What happens if I set server_options["bg_ticks"] = "banana"? Type error at set time? At load_server_options() time? Silently ignored?

4. **Minimal Database Wizard Count**: "At least one player object" with wizard flag - can I have zero wizards? What happens on startup if no wizard exists? Can't call wizard-only builtins.

5. **Bootstrap Sequence Tooling**: "When starting with an empty database" - is there a builtin to create objects when no database exists? Or is this describing manual database file creation? How does #0 get created if there's no database to run MOO code in?

---

## Summary

**PASS:** 1/5 specs
**GAPS:** 4/5 specs

**Specs with gaps:**
- spec/server.md - 8 gaps (implementation choices, undefined types, missing algorithms)
- spec/login.md - 6 gaps (type ambiguity, behavioral choices, missing details)
- spec/builtins/server.md - 7 gaps (undefined formats, missing error handling)
- spec/objects.md (§12) - 5 gaps (error handling, validation, bootstrapping)

**Specs that pass:**
- spec/database.md - Complete specification with external reference for edge cases

**Common gap patterns:**
1. **Implementation-defined types** (connection object type appears in multiple specs)
2. **Explicit implementation choices** (reconnection boot, task continuation)
3. **Missing error handling** (type validation, hook failures)
4. **Undefined formats** (log messages, memory_usage return value)
5. **Ambiguous timing** (hook call order, async operations)

**Recommendation:** The database format spec demonstrates the level of precision needed. Other specs need similar detail for type signatures, error conditions, and ordering guarantees.
