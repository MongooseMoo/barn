# Native TLS/WebSocket Listeners: Phase 1 Verification

Date: 2026-05-06

## Workflow Used

Requested workflow:

- `plans/native-tls-websocket-listeners-workstream.md`

Actual Phase 1 workflow:

1. Inspected existing listener/login lifecycle source.
2. Found current source already had listener-object `do_login_command`, `argstr`, `user_created`, and `user_client_disconnected` support.
3. Implemented the missing accepting-listener source port tracking for connection metadata.
4. Added focused scheduler tests for listener-owned login dispatch and listener-owned login hooks.
5. Ran focused Go tests.
6. Ran the managed focused listener/admin/login conformance slice.
7. Reran the full managed TCP baseline because this phase touches connection/login surfaces.

## Commits

- `9027465 Track accepting listener port on connections`
- `c7127cf Cover listener-owned login hooks`

## Production Change

Connections now store the accepting listener port when created from a listener record.

Affected behavior:

- `connection_name(player)` method `0` uses the connection's accepting listener port when available.
- `connection_info(player)["source_port"]` uses the connection's accepting listener port when available.
- Synthetic/test connections retain the primary listener port fallback.

## Focused Tests Added

File:

- `server/scheduler_login_test.go`

Coverage:

- `TestDoLoginCommandDispatchesOnListenerWithArgstr`
  - Proves the accepting listener object, not `#0`, handles `do_login_command`.
  - Proves the original command text reaches the verb as `argstr`.
  - Proves quoted-word parsing reaches `args`.
- `TestLoginPlayerRunsListenerCreatedAndConnectedHooks`
  - Proves listener-owned `user_created` runs.
  - Proves listener-owned `user_connected` runs.

## Verification Commands

Focused Go tests:

```powershell
go test .\server .\builtins
```

Result:

```text
ok  	barn/server	(cached)
ok  	barn/builtins	(cached)
```

Managed focused conformance:

```powershell
$env:UV_NO_SYNC = '1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788 -K "server_admin or login"
```

Result:

```text
112 passed, 2585 deselected in 4.40s
```

Recorded run:

- `reports/runs/20260506_091356/summary.json`

Managed full TCP baseline:

```powershell
$env:UV_NO_SYNC = '1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788
```

Result:

```text
2 failed, 2528 passed, 167 skipped in 42.73s
```

Recorded run:

- `reports/runs/20260506_091416/summary.json`

Failures:

- `algorithms::crypt_invalid_salt`
- `math::ctime_with_int_arg_is_invarg`

These are unchanged from Phase 0 and are not listener/TLS/WS failures.

## Phase 1 Exit Status

Phase 1 is complete:

- Custom listener objects can own login flow.
- Listener-owned login hook tests exist.
- Connection metadata now uses the accepting listener port.
- TCP managed baseline is unchanged from Phase 0.

Next unchecked phase:

- Phase 2: Target Listener Representation.
