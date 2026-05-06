# Native TLS/WebSocket Listeners - Phase 3 Report

Workflow used: `plans/native-tls-websocket-listeners-workstream.md`, Phase 3.

## Source Slices

Commits:

- `9767df7 Add startup listener spec parser`
- `c8d25fe Unify startup listener specs`
- `8fc295f Cover startup listener spec selection`

Implemented:

- Added `server.ParseListenSpec()` for startup listener URLs:
  - `tcp://:7777`
  - `tls://:7778?cert=...&key=...`
  - `ws://:7779/moo`
  - `wss://:7780/moo?cert=...&key=...`
- Preserved `-port N` as shorthand for one TCP listener.
- Added repeatable `-listen` CLI flag.
- Rejected explicit `-port` combined with `-listen` as ambiguous.
- Replaced the one-listener startup path with `ConnectionManager.StartListeners([]ListenerSpec)`.
- Startup listeners and MOO `listen()` now both reach the same `addListener()` registration path.

## Verification

Working directory: `C:\Users\Q\code\barn`

Commands run:

```powershell
go test .\cmd\barn .\server
```

Result:

```text
ok  	barn/cmd/barn	0.494s
ok  	barn/server	0.098s
```

The affected Go tests cover:

- `-port` shorthand spec creation.
- repeatable `-listen` parsing.
- explicit `tcp://` startup listener specs.
- multiple TCP startup listeners binding through `StartListeners`.
- invalid listener specs.
- ambiguous `-port` plus `-listen`.

Attempted broader Go package check:

```powershell
go test .\cmd\barn .\server .\conformance
```

Result:

```text
cmd/barn and server passed; conformance failed because the external conformance YAML directory was not present in this checkout.
```

Managed focused conformance:

```powershell
$env:UV_NO_SYNC='1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788 -K "server_admin or login"
```

Result:

```text
112 passed, 2585 deselected in 4.35s
```

Run directory: `reports\runs\20260506_093008`

Managed full TCP conformance:

```powershell
$env:UV_NO_SYNC='1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788
```

Result:

```text
2 failed, 2528 passed, 167 skipped in 42.75s
```

Run directory: `reports\runs\20260506_093026`

The two full-run failures match the Phase 0 baseline:

- `algorithms::crypt_invalid_salt`
- `math::ctime_with_int_arg_is_invarg`

## Exit Gate

Phase 3 exit gate passed:

- There is one production listener registration path.
- `-port N` remains a TCP startup shorthand.
- Repeatable startup listener specs are parsed and validated.
- TCP managed conformance remains unchanged from the Phase 0 baseline.
