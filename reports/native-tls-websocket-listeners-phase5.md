# Native TLS and WebSocket Listener Workstream - Phase 5

Workflow used: `plans/native-tls-websocket-listeners-workstream.md`, Phase 5.

## Result

Phase 5 is complete.

`spec/server.md` now defines Barn's native WebSocket and WSS listener
semantics before WS/WSS production code:

- listener option keys and canonical descriptors
- `listeners()` fields and accepted filters
- `unlisten()` descriptor compatibility
- WebSocket path matching
- default open origin policy
- non-WebSocket HTTP response behavior
- connection naming and `connection_info()` compatibility
- one-text-message-to-one-MOO-line input framing
- one-logical-output-line-to-one-WebSocket-message output framing
- rejection policy for embedded CR/LF, binary messages, and invalid UTF-8
- ping/pong, close, and pre-login timeout behavior
- interactions with `notify(..., no_newline)`, OOB `#$#`, `flush-command`, and
  the `binary` connection option

## Verification

Command:

```powershell
git diff --check -- .\spec\server.md
```

Working directory:

```text
C:\Users\Q\code\barn
```

Result:

```text
passed with no output
```

## Commit

```text
7c834d7 Specify native websocket listener semantics
```
