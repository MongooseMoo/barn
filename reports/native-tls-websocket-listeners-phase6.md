# Native TLS and WebSocket Listener Workstream - Phase 6

Workflow used: `plans/native-tls-websocket-listeners-workstream.md`, Phase 6.

## Result

Phase 6 is complete.

Implemented native `ws` listeners with:

- `github.com/coder/websocket` as the maintained WebSocket dependency
- HTTP server based upgrade handling
- exact path matching and `426 Upgrade Required` for non-WebSocket HTTP
  requests
- default open origin policy matching `spec/server.md`
- WebSocket transport that bypasses the telnet byte parser
- one text message as one MOO input line
- one logical output message as one WebSocket text message
- rejection of embedded CR/LF and binary messages
- ping/pong handled at transport level
- pre-login timeout behavior that can still send the timeout message
- listener metadata and `listen()` descriptor coverage
- connection manager shutdown that closes listeners, connections, and outbound
  sockets using configurable shutdown timing fields

## Verification

Commands:

```powershell
go test .\server .\builtins
$env:UV_NO_SYNC='1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788 -K "server_admin or login"
$env:UV_NO_SYNC='1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788
```

Working directory:

```text
C:\Users\Q\code\barn
```

Results:

```text
go test .\server .\builtins: ok
focused managed conformance: 112 passed, 2585 deselected
full managed conformance: 2 failed, 2528 passed, 167 skipped
```

The full managed conformance failures are unchanged from the Phase 0 baseline:

```text
algorithms::crypt_invalid_salt
math::ctime_with_int_arg_is_invarg
```

Managed run directories:

```text
reports\runs\20260506_095953
reports\runs\20260506_100007
```

## Notes

`go test .\server .\builtins` produced untracked file I/O scratch artifacts
under `builtins/testdata/fileio`; these are generated diagnostics and were not
committed.
