# Task: Investigate "look self" Difference Between Toast and Barn

## Context
Barn is a Go implementation of a MOO server. ToastStunt is the reference C++ implementation. When running "connect wizard" followed by "look self" on the toastcore database, the two servers behave differently.

## Objective
1. Run "connect wizard" then "look self" on BOTH servers using toastcore_barn.db
2. Capture and compare the exact output from each
3. Identify the root cause of the difference
4. Document findings

## Test Procedure

### Step 1: Test with Toast (Reference)
```bash
cd /c/Users/Q/code/barn

# Start Toast on port 9700
./toast_moo.exe -l 9700 toastcore_barn.db > toast_9700.log 2>&1 &
sleep 2

# Send commands
./moo_client.exe -port 9700 -timeout 5 -cmd "connect wizard" -cmd "look self"

# Kill the server
taskkill //F //IM toast_moo.exe 2>/dev/null || true
```

### Step 2: Test with Barn
```bash
cd /c/Users/Q/code/barn

# Build barn if needed
go build -o barn_test.exe ./cmd/barn/

# Start Barn on port 9300
./barn_test.exe -db toastcore_barn.db -port 9300 > barn_9300.log 2>&1 &
sleep 2

# Send commands
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard" -cmd "look self"

# Kill the server
taskkill //F //IM barn_test.exe 2>/dev/null || true
```

### Step 3: Compare Outputs
- Note exact differences in output
- Check server logs for errors
- Identify which builtin/verb is behaving differently

## Files to Check If Investigating Code
- `builtins/` - Go builtin implementations
- `vm/` - VM execution
- Server logs: `toast_9700.log`, `barn_9300.log`

## Output
Write findings to `./reports/investigate-look-self-diff.md` with:
1. Toast output (exact)
2. Barn output (exact)
3. Difference analysis
4. Root cause identification
5. Recommended fix (if applicable)

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
