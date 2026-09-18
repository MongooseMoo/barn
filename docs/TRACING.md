# Barn Execution Tracing Guide

## Quick Start

Enable tracing when starting the server:

```bash
./barn.exe -db MyGame.db -port 7777 --trace 2> trace.log
```

## Command-Line Flags

### `--trace`
Enable execution tracing (logs to stderr by default).

### `--trace-filter <pattern>`
Filter traced verbs using glob patterns. Multiple patterns can be comma-separated.

Examples:
```bash
# Trace all verbs
./barn.exe --trace

# Trace only login-related verbs
./barn.exe --trace --trace-filter "do_login*,user_*"

# Trace only a specific verb
./barn.exe --trace --trace-filter "look"

# Multiple patterns
./barn.exe --trace --trace-filter "do_*,user_*,get_*"
```

## Output Format

### Connection Events
```
[TRACE] CONN NEW conn=2 player=#-2 127.0.0.1:54321
[TRACE] CONN LOGIN conn=2 player=#8
[TRACE] CONN DISCONNECT conn=2 player=#8
```

### Verb Calls
```
[TRACE] CALL #0:do_login_command args=["connect", "wizard"] player=#-2 caller=#-2
[TRACE] RETURN #0:do_login_command => #8
```

### Exceptions
```
[TRACE] CALL #0:user_connected args=[#8] player=#8 caller=#8
[TRACE] EXCEPTION #0:user_connected E_VERBNF
```

### Notify Calls
```
[TRACE]   NOTIFY #8 "Welcome to the game!"
[TRACE]   NOTIFY #8 "You are in a dark room."
```

Note: Notify calls are indented to show they occurred during verb execution.

## Use Cases

### Debug Login Issues
```bash
./barn.exe -db game.db --trace --trace-filter "do_login*,user_*" 2> login_trace.log
```

This traces:
- `do_login_command` - Initial connection handler
- `user_connected` - Post-login hook
- `user_reconnected` - Reconnection handler
- `user_disconnected` - Disconnect hook

### Debug Command Processing
```bash
./barn.exe -db game.db --trace --trace-filter "look,@describe" 2> command_trace.log
```

### Full Execution Trace
```bash
./barn.exe -db game.db --trace 2> full_trace.log
```

Warning: Full traces can be very verbose in active databases.

### Real-Time Monitoring
```bash
# Terminal 1: Start server with trace
./barn.exe -db game.db --trace

# Terminal 2: Connect and test
telnet localhost 7777
```

The trace output will appear in terminal 1 as commands execute.

## Pattern Matching

Trace filters use Go's `filepath.Match` glob syntax:

- `*` - Matches any sequence of characters
- `?` - Matches any single character
- `[abc]` - Matches any character in the set
- `[a-z]` - Matches any character in the range

Examples:
```bash
# Match all "do_" verbs
--trace-filter "do_*"

# Match all "user_" verbs
--trace-filter "user_*"

# Match verbs starting with any single letter followed by "ook"
--trace-filter "?ook"  # Matches: look, book, cook, etc.

# Match verbs starting with letters a-g
--trace-filter "[a-g]*"
```

## Performance

When tracing is **disabled** (default):
- Zero overhead (simple boolean check)
- No string formatting or I/O

When tracing is **enabled**:
- Minimal overhead (mutex lock + fprintf)
- Output is buffered by OS
- Filter checks are fast (glob pattern matching)

## Tips

1. **Always redirect stderr to a file** for later analysis:
   ```bash
   ./barn.exe --trace 2> trace.log
   ```

2. **Use filters to reduce noise** in busy databases:
   ```bash
   ./barn.exe --trace --trace-filter "do_*,user_*"
   ```

3. **Grep for specific events**:
   ```bash
   grep "EXCEPTION" trace.log
   grep "CONN" trace.log
   grep "do_login_command" trace.log
   ```

4. **Watch for patterns**:
   ```bash
   tail -f trace.log | grep "EXCEPTION"
   ```

5. **Count verb invocations**:
   ```bash
   grep "CALL" trace.log | cut -d: -f2 | cut -d' ' -f1 | sort | uniq -c
   ```

## Example Session

```bash
$ ./barn.exe -db toastcore.db --trace 2> trace.log &
$ telnet localhost 7777
Trying 127.0.0.1...
Connected to localhost.
connect wizard
*** Connected ***
look
You see a room.
quit
*** Disconnected ***
$ grep "^\[TRACE\]" trace.log
[TRACE] CONN NEW conn=2 player=#-2 127.0.0.1:54321
[TRACE] CALL #0:do_login_command args=[] player=#-2 caller=#-2
[TRACE]   NOTIFY #-2 "Welcome to ToastCore."
[TRACE] RETURN #0:do_login_command => 0
[TRACE] CALL #0:do_login_command args=["connect", "wizard"] player=#-2 caller=#-2
[TRACE] RETURN #0:do_login_command => #8
[TRACE] CONN LOGIN conn=2 player=#8
[TRACE] CALL #0:user_connected args=[#8] player=#8 caller=#8
[TRACE] RETURN #0:user_connected => 0
[TRACE] CALL #8:look args=[] player=#8 caller=#8
[TRACE]   NOTIFY #8 "You see a room."
[TRACE] RETURN #8:look => 0
[TRACE] CONN DISCONNECT conn=2 player=#8
[TRACE] CALL #0:user_disconnected args=[#8] player=#8 caller=#8
[TRACE] RETURN #0:user_disconnected => 0
```

## See Also

- [Implementation Report](reports/build-barn-trace.md) - Technical details
- [CLAUDE.md](CLAUDE.md) - Project documentation
