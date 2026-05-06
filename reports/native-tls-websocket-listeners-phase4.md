# Native TLS/WebSocket Listeners - Phase 4 Report

Workflow used: `plans/native-tls-websocket-listeners-workstream.md`, Phase 4.

## Source Slices

Commits:

- `f0d3abe Add native TLS listener support`
- `534d989 Cover TLS listen builtin options`
- `1e8a1e9 Cover TLS listener login and eval`

Implemented:

- Added `tls` listener support using Go `crypto/tls`.
- TLS listeners bind TCP sockets, wrap accepted sockets with `tls.Server`, complete the TLS handshake, then use the existing TCP line transport.
- `listen(obj, port, ["protocol" -> "tls", "certificate" -> path, "key" -> path])` now creates TLS listeners.
- CLI `tls://:port?cert=...&key=...` specs now reach the same listener path.
- `listeners()` reports TLS listener metadata through the existing listener info fields.
- TLS handshake failure returns before creating a Barn connection.

## Verification

Working directory: `C:\Users\Q\code\barn`

Commands run:

```powershell
go test .\server .\builtins
```

Result:

```text
ok  	barn/server	0.279s
ok  	barn/builtins	(cached)
```

The focused Go tests cover:

- TLS listener creation from self-signed certificate/key files.
- `listeners()` TLS metadata.
- TLS listener certificate/key validation.
- TLS handshake feeding the existing line transport.
- TLS listener login and eval through the scheduler.
- `listen()` builtin option parsing for TLS.

Managed focused conformance:

```powershell
$env:UV_NO_SYNC='1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788 -K "server_admin or login"
```

Result:

```text
112 passed, 2585 deselected in 4.35s
```

Run directory: `reports\runs\20260506_093723`

Managed full TCP conformance:

```powershell
$env:UV_NO_SYNC='1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788
```

Result:

```text
2 failed, 2528 passed, 167 skipped in 42.63s
```

Run directory: `reports\runs\20260506_093744`

The two full-run failures match the Phase 0 baseline:

- `algorithms::crypt_invalid_salt`
- `math::ctime_with_int_arg_is_invarg`

## Exit Gate

Phase 4 exit gate passed:

- TLS is a native listener family.
- TLS shares TCP line semantics after handshake.
- TLS listener login/eval works with a self-signed certificate.
- TCP managed conformance remains unchanged from the Phase 0 baseline.
