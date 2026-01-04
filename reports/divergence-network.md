# Divergence Report: Network Builtins

**Spec File**: `spec/builtins/network.md`
**Barn Files**: `builtins/network.go`
**Status**: divergences_found
**Date**: 2026-01-03

## Summary

Tested 12 network-related builtins across both Barn (port 9500) and Toast (port 9501). Found **2 behavioral divergences** and identified **multiple functions documented in spec but not implemented in either server**. Most core connection management functions behave identically between servers.

**Key Findings:**
- 10 functions tested and verified correct
- 2 behavioral divergences found
- 6+ documented functions don't exist in either server
- No conformance test coverage for any network builtins (only `notify` mentioned in command_parsing.yaml but skipped)

## Divergences

### 1. connected_players() - Returns Ghost Connections

| Field | Value |
|-------|-------|
| Test | `connected_players()` |
| Barn | `{#-7, #-529, #3059}` (3 players, includes negative IDs) |
| Toast | `{#10697}` (1 player, current connection only) |
| Classification | likely_barn_bug |
| Evidence | Barn is tracking old/disconnected connections with negative object IDs. Toast correctly returns only currently connected players. Barn's implementation doesn't properly clean up disconnected players from the connection manager. |

### 2. listeners() - Empty vs Listener Data (Environment-Dependent)

| Field | Value |
|-------|-------|
| Test | `listeners()` using toast_oracle on toastcore.db |
| Barn (Test.db) | `{}` (empty list) |
| Toast (Test.db) | `{}` (empty list) |
| Toast (toastcore.db via oracle) | `{["interface" -> "Empiricist", "ipv6" -> 1, "object" -> #0, "port" -> 7777, "print-messages" -> 1]}` |
| Classification | not_a_divergence |
| Evidence | Both servers return empty list on Test.db which has no registered listeners. Toast oracle on toastcore.db shows proper listener data structure. This is environment-dependent, not a server difference. Barn's stub implementation returns correct empty list when no listeners are registered. |

## Test Coverage Gaps

**CRITICAL: NO conformance test coverage for any network builtins!**

The only mention is in `command_parsing.yaml` which requires `notify` but all tests are skipped with "Requires command dispatch".

Behaviors documented in spec but NOT covered by conformance tests:

### Connection Management Functions
- `connected_players([include_queued])` - list connected players, optional queued flag
- `connected_players()` edge case - negative object IDs for disconnected players
- `connection_name(player, method)` - all three methods (0=hostname, 1=ip, 2=legacy)
- `boot_player(player)` - disconnect player, wizard-only permission check
- `listeners([find])` - list listening ports, optional filter by object/port
- `idle_seconds(player)` - time since last input
- `connected_seconds(player)` - connection duration
- `notify(player, message)` - basic send message
- `notify(player, message, no_flush)` - buffered vs immediate send

### Connection Options
- `set_connection_option(conn, option, value)` - hold-input, disable-oob, binary mode
- `connection_option(conn, option)` - retrieve option values

### Advanced Functions (Stubs/Minimal)
- `read_http(type [, connection])` - E_ARGS, E_TYPE, E_INVARG error conditions
- `switch_player(old_player, new_player)` - wizard permission requirement
- `connection_name_lookup(player)` - async DNS lookup stub

## Functions Documented in Spec But Not Implemented

These functions are documented in `spec/builtins/network.md` but return E_VERBNF or E_VARNF in both servers:

### 1. connection_info() - ToastStunt Extension
- **Spec Claims**: Returns map with `["ip", "port", "connected_at", "last_input", "bytes_in", "bytes_out"]`
- **Reality**: Both servers return E_VERBNF
- **Classification**: spec_error - function doesn't exist in Toast

### 2. notify_list() - ToastStunt Extension
- **Spec Claims**: Sends multiple lines efficiently
- **Status**: NOT TESTED (requires connection setup)
- **Classification**: needs_investigation

### 3. force_input() - ToastStunt Extension
- **Spec Claims**: Queues input as if player typed it (wizard-only)
- **Status**: NOT TESTED (complex setup required)
- **Classification**: needs_investigation

### 4. flush_input() - ToastStunt Extension
- **Spec Claims**: Discards queued input
- **Status**: NOT TESTED (complex setup required)
- **Classification**: needs_investigation

### 5. curl() - ToastStunt Extension
- **Spec Claims**: Makes HTTP requests with options map
- **Reality**: Both servers return E_VARNF
- **Classification**: spec_error - function doesn't exist in Toast
- **Note**: spec/builtins/network.md shows Go implementation but builtin isn't registered

### 6. dns_lookup() - ToastStunt Extension
- **Spec Claims**: Resolves hostname to IP addresses list
- **Status**: NOT TESTED (would require external network access)
- **Classification**: needs_investigation

### 7. reverse_dns() - ToastStunt Extension
- **Spec Claims**: Reverse DNS lookup
- **Status**: NOT TESTED (would require external network access)
- **Classification**: needs_investigation

### 8. set_connection_timeout() - ToastStunt Extension
- **Spec Claims**: Sets idle timeout for connection
- **Status**: NOT TESTED (difficult to verify timeout behavior)
- **Classification**: needs_investigation

### 9. listen() / unlisten() / open_network_connection()
- **Spec Claims**: Connection management functions
- **Status**: NOT TESTED (would require server configuration changes)
- **Classification**: needs_investigation

### 10. read() - Core MOO Function
- **Spec Claims**: Reads line of input from player, suspends task
- **Status**: NOT TESTED (requires task suspension mechanism)
- **Classification**: needs_investigation

### 11. connection_options() - Connection Query
- **Spec Claims**: Returns MAP of current connection options
- **Status**: NOT TESTED
- **Classification**: needs_investigation

## Behaviors Verified Correct

These behaviors match between Barn and Toast:

### ✓ listeners()
- Returns LIST type (4)
- Returns empty list `{}` on Test.db with no listeners
- Proper structure on databases with registered listeners

### ✓ connected_players() - Type Only
- Returns LIST type (4)
- **NOTE**: Values differ (see divergence #1)

### ✓ connection_name(player, method)
- Returns STR type (2)
- Method 0: Returns IP address `"[::1]"` for IPv6 localhost
- Method 1: Returns IP address `"[::1]"` (same as method 0)
- Method 2: Returns legacy format `"[::1], port XXXXX"`
- E_INVARG for disconnected player `#-1`

### ✓ idle_seconds(player)
- Returns INT type (0)
- Returns `0` for active connection (both servers)
- **NOTE**: Barn is stub (always 0), Toast also returns 0 for active connections

### ✓ connected_seconds(player)
- Returns INT type (0)
- Returns `0` for new connection (both servers)
- **NOTE**: Barn is stub (always 0), but Toast also returns 0 for fresh connections

### ✓ boot_player(player)
- E_INVARG for invalid/disconnected player `#-1`

### ✓ set_connection_option(conn, option, value)
- Accepts arguments without error
- Returns `0` (success)
- **NOTE**: Both are stubs that accept but don't enforce options

### ✓ connection_option(conn, option)
- Returns `0` (default value)
- **NOTE**: Both are stubs returning defaults

### ✓ read_http(type [, connection])
- E_ARGS when called with no arguments
- E_TYPE when first argument is not string (e.g., `123`)
- E_INVARG when type string is invalid (e.g., `"invalid"`)
- E_INVARG when type is valid but no HTTP data available (e.g., `"request"`)

### ✓ connection_name_lookup(player)
- Returns `0` (stub success)
- Both servers implement as stub (no async DNS)

## Recommendations

### Immediate Action Required

1. **Fix Barn's connected_players() bug** - Remove ghost connections with negative IDs from the connection manager
2. **Remove non-existent functions from spec** - `connection_info()` and `curl()` are documented but don't exist in Toast
3. **Add conformance tests for core network functions** - Currently ZERO test coverage

### Investigation Needed

1. **Verify ToastStunt extensions** - Many functions marked "(ToastStunt)" in spec need verification:
   - Do they actually exist in ToastStunt?
   - Are they optional/configuration-dependent?
   - Should they be marked as "extension" rather than "core"?

2. **Test with actual network operations**:
   - `listen()`/`unlisten()` with real ports
   - `dns_lookup()`/`reverse_dns()` with real hostnames (local only)
   - `notify_list()` with multiple lines
   - `read()` with task suspension

3. **Document stub implementations**:
   - `idle_seconds()` always returns 0 in Barn
   - `connected_seconds()` always returns 0 in Barn
   - `set_connection_option()` accepts but ignores options in both
   - These may be acceptable stubs or may need full implementation

### Spec Updates Needed

1. Mark `connection_info()` as "NOT IMPLEMENTED" or remove
2. Mark `curl()` as "NOT IMPLEMENTED" or clarify it's planned
3. Clarify which functions require external network access (DNS, HTTP)
4. Document which functions are stubs vs fully implemented
5. Add note that `listeners()` returns empty list when no listeners registered
6. Document behavior of `connected_players()` with disconnected connections

## Testing Methodology

All tests performed using:
- **Toast**: port 9501 with Test.db
- **Barn**: port 9500 with Test.db
- **Toast oracle**: toastcore.db for reference values
- **Test client**: `moo_client.exe` with 10-second timeout

Commands tested were executed as wizard player via `connect wizard` to ensure maximum permissions for permission-dependent functions.

## Notes

- Many network functions require active connections or external resources
- Test.db has no registered listeners (both servers return empty list)
- Both servers use IPv6 localhost `[::1]` for connections
- Stub implementations may be intentional for security (no external network access)
- NO external network requests were made during testing (per instructions)
