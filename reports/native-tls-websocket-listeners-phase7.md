# Native TLS and WebSocket Listener Workstream - Phase 7

Workflow used: `plans/native-tls-websocket-listeners-workstream.md`, Phase 7.

## Result

Phase 7 is complete.

Implemented native `wss` listeners using the same HTTP/WebSocket acceptor as
`ws`, over a TLS-wrapped listener:

- `listen(..., ["protocol" -> "wss", "path" -> ..., "certificate" -> ...,
  "key" -> ...])` creates WSS listeners
- CLI `wss://...?...` startup specs use the shared listener registration path
- `listeners()` reports `protocol` as `wss`, preserves path metadata, and marks
  `TLS` as true
- failed TLS handshake does not create a Barn connection
- WSS path, origin, login/eval, and shutdown behavior match WS semantics

## Verification

Commands:

```powershell
go test .\server -run "TestWSS"
go test .\server .\builtins
```

Working directory:

```text
C:\Users\Q\code\barn
```

Results:

```text
go test .\server -run "TestWSS": ok
go test .\server .\builtins: ok
```

## Notes

`go test .\server .\builtins` left untracked file I/O scratch artifacts under
`builtins/testdata/fileio`; these are generated diagnostics and were not
committed.
