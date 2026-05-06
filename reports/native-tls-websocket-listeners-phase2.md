# Native TLS/WebSocket Listeners - Phase 2 Report

Workflow used: `plans/native-tls-websocket-listeners-workstream.md`, Phase 2.

## Source Slice

Commit: `f9b5557 Introduce listener descriptors`

Implemented:

- Added `builtins.ListenerSpec` with protocol, port, interface, object, print-messages, path, and TLS certificate/key fields.
- Added `builtins.ListenerDescriptor` and changed the connection manager interface to accept descriptors instead of raw ports.
- Replaced `ConnectionManager.listeners` port-only keys with `{protocol, port, path}` listener keys.
- Stored listener protocol metadata on listener records and exposed it through `ListenerInfos()` and `listeners()`.
- Kept TCP compatibility: `listen(obj, port)` still returns an integer port for TCP, and `unlisten(port)` still works.
- Added descriptor map support for `listeners(desc)` and `unlisten(desc)`.
- Rejected non-TCP protocols at registration in this phase; TLS/WS/WSS construction is later workstream scope.

## Verification

Working directory: `C:\Users\Q\code\barn`

Commands run:

```powershell
go test .\server .\builtins
```

Result:

```text
ok  	barn/server	0.099s
ok  	barn/builtins	(cached)
```

Managed focused conformance:

```powershell
$env:UV_NO_SYNC='1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788 -K "server_admin or login"
```

Result:

```text
112 passed, 2585 deselected in 4.35s
```

Run directory: `reports\runs\20260506_092235`

Managed full TCP conformance:

```powershell
$env:UV_NO_SYNC='1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788
```

Result:

```text
2 failed, 2528 passed, 167 skipped in 42.38s
```

Run directory: `reports\runs\20260506_092253`

The two full-run failures match the Phase 0 and Phase 1 baseline:

- `algorithms::crypt_invalid_salt`
- `math::ctime_with_int_arg_is_invarg`

## Exit Gate

Phase 2 exit gate passed:

- New listener representation exists.
- Old port-only production listener identity has been removed.
- TCP managed conformance is unchanged from the Phase 0 baseline.
