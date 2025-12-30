# Task: Build Execution Tracing for Barn Server

## Context

Barn is a Go implementation of a MOO server. We have a critical bug: conformance tests pass, but when you connect interactively, nothing works - no welcome banner, no response to commands.

We need execution tracing to understand what's happening during the login flow.

## Reference Implementation

cow_py (Python MOO server) already has sophisticated tracing in `~/code/cow_py/src/cow_py/cli/debug.py`. Study it for design inspiration:
- `debug run` - Runs a verb with tracing, call traces, variable watching
- `debug trace` - Step-by-step execution tracing with filters
- `debug server-trace` - Traces server execution during login flow

Key files in cow_py:
- `~/code/cow_py/src/cow_py/cli/debug.py` - The debug CLI commands
- `~/code/cow_py/src/moo_interp/debugger.py` - The debugger/tracer implementation

## Objective

Add a `--trace` or `--debug` flag to Barn that logs:
1. Every verb call: `#obj:verb(args)` with caller and player context
2. Every verb return: the result value
3. Every exception: error code and call stack
4. Every `notify()` call: target player and message
5. Connection events: new connection, login attempt, login success/fail

The trace should go to stderr or a log file so it doesn't interfere with normal server output.

## Key Files to Modify

- `cmd/barn/main.go` - Add --trace flag
- `server/scheduler.go` - Wrap CallVerb to log calls/returns
- `server/connection.go` - Log connection events, login flow
- `builtins/network.go` - Log notify() calls
- Create new file: `server/trace.go` or `debug/trace.go` for trace infrastructure

## Design Guidance

1. **Global trace flag**: A simple global bool that enables/disables tracing
2. **Structured output**: Use a consistent format like:
   ```
   [TRACE] CALL #0:do_login_command args=[] player=-2 caller=0
   [TRACE] CALL #10:connection_name_lookup args=[#-2] player=-2 caller=0
   [TRACE] RETURN #10:connection_name_lookup => 0
   [TRACE] NOTIFY player=-2 message="Welcome to ToastCore..."
   [TRACE] EXCEPTION #0:do_login_command E_VERBNF
   ```
3. **Don't break anything**: Tracing should be optional and have zero overhead when disabled

## Test Command

After implementing, we should be able to run:
```bash
./barn_test.exe -db toastcore.db -port 9400 --trace > /dev/null 2> trace.log &
sleep 2
./moo_client.exe -port 9400 -cmd "" -timeout 3
cat trace.log
```

And see exactly what verbs were called, what they returned, and where things went wrong.

## Output

Write your implementation plan to `./reports/build-barn-trace.md` first. Include:
1. Analysis of cow_py's tracing design
2. Proposed design for Barn
3. File-by-file changes needed
4. Testing strategy

After Q approves the plan, implement it.

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
