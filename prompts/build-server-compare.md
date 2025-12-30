# Task: Build Server Comparison Tool (Toast vs Barn)

## Context

Barn is a Go implementation of a MOO server. We have a critical bug: conformance tests pass, but when you connect interactively, nothing works. Toast (the C++ reference) works fine.

We need tooling to compare what Toast sends to clients vs what Barn sends (or doesn't send).

## Available Resources

1. **Toast executable**: `~/src/toaststunt/build-win/Release/moo.exe` (the C++ reference server)
2. **toast_moo.exe**: `~/code/barn/toast_moo.exe` (wrapper script for toast)
3. **toast_oracle.exe**: `~/code/barn/toast_oracle.exe` (runs expressions in Toast emergency mode)
4. **cow_py server-trace**: `uv run python -m cow_py.cli debug server-trace` (Python reference tracing)
5. **moo_client.exe**: `~/code/barn/moo_client.exe` (Go client for testing)
6. **Database**: `~/code/barn/toastcore.db` (the database both servers should use)

## Objective

Build a CLI tool `cmd/compare_servers/main.go` that:
1. Starts Toast server on port A
2. Starts Barn server on port B
3. Connects to both with identical input
4. Captures and compares output from both
5. Shows differences clearly

Output should be like:
```
=== Server Comparison: Login Flow ===
Input: [connect]

Toast (port 9500):
  Welcome to ToastCore MOO!
  ...

Barn (port 9501):
  [no output received]

DIFFERENCE: Barn produced no output for empty login command
```

## Design Options

### Option A: External comparison tool (recommended)
Create `cmd/compare_servers/main.go` that:
- Uses os/exec to start both servers
- Connects via TCP to both
- Captures output with timeouts
- Compares and diffs

### Option B: Test-based comparison
Add a comparison test in `server/compare_test.go` that:
- Starts both servers programmatically
- Runs scenarios
- Asserts output matches

Recommend Option A because it's more flexible and can be run manually.

## Key Scenarios to Test

1. **Empty connect**: Connect, send nothing, wait for welcome banner
2. **Login flow**: `connect wizard` or `connect guest`
3. **Invalid command**: Send garbage after login
4. **EVAL command**: `OUTPUTPREFIX >>>` then `;1+1`

## Implementation Steps

1. First, manually verify Toast works:
   ```bash
   # Start Toast
   ~/src/toaststunt/build-win/Release/moo.exe toastcore.db NUL 9500 &
   sleep 2
   # Connect and see welcome
   ./moo_client.exe -port 9500 -timeout 5
   ```

2. Build the comparison tool that automates this

3. Run comparisons to identify specific differences

## Test Command

After implementing:
```bash
go build -o compare_servers.exe ./cmd/compare_servers/
./compare_servers.exe -db toastcore.db -toast-port 9500 -barn-port 9501 -scenario "login"
```

## Output

Write your implementation plan to `./reports/build-server-compare.md` first. Include:
1. Manual verification that Toast works (capture actual output)
2. Proposed tool design
3. Implementation plan
4. List of comparison scenarios

After Q approves the plan, implement it.

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
