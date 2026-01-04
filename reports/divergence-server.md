# Divergence Report: Server Builtins

**Spec File**: `spec/builtins/server.md`
**Barn Files**: `builtins/system.go`, `builtins/network.go`, `server/connection.go`
**Status**: divergences_found
**Date**: 2026-01-03

## Summary

Tested 8 server builtin functions across Barn (port 9500) and Toast (port 9501). Found 1 significant divergence in `connected_players()` behavior. Identified 8 functions from the spec that are not implemented in either server. Most core connection management functions (notify, connection_name, listeners, server_log) behave identically.

**Key Finding**: Barn's `connected_players()` incorrectly includes unlogged connections (negative IDs) in the default result, while Toast only returns logged-in player objects.

## Divergences

### 1. connected_players() - Includes Unlogged Connections by Default

| Field | Value |
|-------|-------|
| Test | `connected_players()` |
| Barn | `{#-529, #2941, #-7}` (includes negative connection IDs) |
| Toast | `{#10618}` (only logged-in player) |
| Classification | likely_barn_bug |
| Evidence | Toast is the reference implementation. According to the spec, `connected_players([include_all])` should optionally include unlogged connections as negative IDs when `include_all` is true. Barn's `builtinConnectedPlayers()` in `builtins/network.go` ignores the optional parameter entirely and always returns ALL players from the `playerConns` map, which includes both positive (logged) and negative (unlogged) entries. Toast correctly filters to only logged-in players by default. |

**Root Cause**: In `builtins/network.go`, the function doesn't check the `include_all` parameter:
```go
func builtinConnectedPlayers(ctx *types.TaskContext, args []types.Value) types.Result {
    // ... no parameter checking
    players := globalConnManager.ConnectedPlayers()
    // Returns ALL players including negative IDs
```

**Additional Evidence**:
```
Test: {connected_players(), connected_players(0), connected_players(1)}
Barn:  {{#-529, #2941, #-7}, {#-7, #-529, #2941}, {#-7, #-529, #2941}}
Toast: {{#10618}, {#10618}, {#10618}}
```

All three variants return the same result on Barn (bug), while Toast correctly filters based on the parameter.

## Test Coverage Gaps

Behaviors documented in spec but NOT covered by conformance tests:

### Server Information
- `server_version()` - No conformance tests found
- `memory_usage()` - Not implemented in either server (both return E_VERBNF)

### Server Control
- `shutdown([message [, panic]])` - Cannot test destructively, no conformance tests
- `dump_database()` - High risk to test, no conformance tests
- `load_server_options()` - No conformance tests found

### Connection Management
- `connected_players([include_all])` - **Critical gap**: No tests for the include_all parameter behavior
- `connection_name(player [, method])` - No tests for different method values (0, 1, 2)
- `boot_player(player)` - No permission check tests found
- `notify(player, message [, no_flush])` - No tests for no_flush parameter or invalid player behavior
- `buffered_output_length(player)` - Not implemented in either server (both return E_VERBNF)

### Network Functions
- `open_network_connection(host, port)` - Not tested (requires wizard permissions)
- `listen(object, point [, print_messages])` - Not tested (wizard-only)
- `unlisten(point)` - Not tested (wizard-only)
- `listeners()` - Tested, both return empty list (no listeners configured)

### Logging
- `server_log(message [, level])` - No conformance tests, only manual verification

### Object Management (from spec)
- `reset_max_object()` - Not implemented in Barn or Toast
- `renumber(object)` - Not implemented in Barn or Toast

## Behaviors Verified Correct

The following behaviors match between Barn and Toast:

### server_version()
```
Test: server_version()
Toast: "2.7.3_2"
Barn:  "1.0.0-barn"
```
Both return version strings (format is implementation-defined per spec).

### connection_name(player [, method])
```
Test: connection_name(player)
Both:  "[::1]"

Test: connection_name(player, 2)
Barn:  "[::1], port 15564"
Toast: "[::1], port 63220"
```
Both support method parameter correctly (0=IP, 1=IP, 2="IP, port XXXX").

### notify(player, message)
```
Test: notify(player, "test message")
Both: Sends "test message" to player, returns 0
```
Both successfully send messages to connected players.

### listeners()
```
Test: listeners()
Both: {}
```
Both return empty list (no active listeners configured on test servers).

### server_log(message)
```
Test: server_log("test log message")
Both: Returns 0 (wizard-only, successful log write)
```
Both accept and log messages (wizard permission required).

## Functions Not Implemented (Both Servers)

These functions from the spec return E_VERBNF on both servers:

1. `memory_usage()` - Returns E_VERBNF
2. `buffered_output_length(player)` - Returns E_VERBNF
3. `reset_max_object()` - Not registered in either server
4. `renumber(object)` - Not registered in either server
5. `open_network_connection(host, port)` - Not tested (may exist but requires setup)
6. `listen(object, point)` - Not tested (may exist but requires wizard permissions)
7. `unlisten(point)` - Not tested (wizard-only)
8. `load_server_options()` - Implemented in Barn (`builtins/system.go:470`), not tested on Toast

## Edge Cases Not Tested

Due to safety concerns or complexity:

1. `shutdown()` - Cannot test destructively on running servers
2. `dump_database()` - Could disrupt running servers
3. `boot_player()` with permission checks (non-wizard trying to boot another player)
4. `notify()` to disconnected players (both fail silently per MOO convention)
5. Connection options (`set_connection_option`, `connection_option`) - Stubbed in Barn

## Recommendations

1. **Fix Barn Bug**: Modify `builtinConnectedPlayers()` to respect the `include_all` parameter
   - Default (no args or `include_all=0`): Return only logged-in players (positive ObjIDs)
   - `include_all=1`: Return both logged-in players AND unlogged connections (negative IDs)

2. **Add Conformance Tests**: Create tests for `connected_players()` with both parameter values

3. **Test Coverage**: Add conformance tests for:
   - `connection_name()` method parameter (0, 1, 2)
   - `notify()` with no_flush parameter
   - `server_log()` permission checks
   - `boot_player()` permission checks

4. **Document Unimplemented Functions**: Update spec or implementation notes for:
   - `memory_usage()` (optional function)
   - `buffered_output_length()` (not implemented)
   - `reset_max_object()` (not implemented)
   - `renumber()` (not implemented)

## Test Commands Used

```bash
# Build client
go build -o moo_client.exe ./cmd/moo_client/

# Toast oracle (reference)
./toast_oracle.exe 'server_version()'
./toast_oracle.exe 'connected_players()'

# Barn tests
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return server_version();"
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return connected_players();"
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return connection_name(player);"
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return connection_name(player, 2);"
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; notify(player, \"test message\");"
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return listeners();"
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return server_log(\"test\");"

# Toast tests (same commands with -port 9501)
./moo_client.exe -port 9501 -cmd "connect wizard" -cmd "; return connected_players();"
./moo_client.exe -port 9501 -cmd "connect wizard" -cmd "; return connection_name(player);"
# ... etc

# Critical divergence test
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return {connected_players(), connected_players(0), connected_players(1)};"
./moo_client.exe -port 9501 -cmd "connect wizard" -cmd "; return {connected_players(), connected_players(0), connected_players(1)};"
```

## Conformance Test Search

```bash
grep -r "server_version\|connected_players\|notify\|boot_player" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
# Result: Only one mention of "notify" in command_parsing.yaml
```

**Finding**: Virtually no conformance test coverage for server administration builtins. This is a significant gap in the test suite.
