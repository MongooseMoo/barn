# Task: Detect Divergences in Server Builtins

## Context

We need to verify Barn's server builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all server builtins.

## Files to Read

- `spec/builtins/server.md` - expected behavior specification
- `builtins/system.go` or similar - Barn implementation

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Server Information
- `server_version()` - version string
- `memory_usage()` - memory statistics (if exists)
- `db_disk_size()` - database size (if exists)

### Server Control
- `shutdown()` - shutdown server (DO NOT ACTUALLY TEST)
- `dump_database()` - checkpoint database (BE CAREFUL)

### Connection Management
- `connected_players()` - list of connected players
- `connection_name()` - get connection info
- `connection_option()` / `connection_options()` - get/set options
- `set_connection_option()` - set option
- `buffered_output_length()` - check output buffer

### Network
- `open_network_connection()` - outbound connections
- `listen()` / `unlisten()` - control listeners
- `listeners()` - list active listeners

### Player Commands
- `boot_player()` - disconnect player
- `notify()` - send text to player
- `read()` - read from player
- `force_input()` - inject input

### Logging
- `server_log()` - write to server log

## Edge Cases to Test

- Invalid player objects
- Non-connected players
- Permission checks (wizard-only functions)
- Invalid connection options

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'server_version()'
./toast_oracle.exe 'connected_players()'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return server_version();"
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return connected_players();"

# Check conformance tests
grep -r "server_version\|connected_players\|notify\|boot_player" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-server.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY major server builtin SAFELY
- DO NOT test shutdown() or dump_database() destructively
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
