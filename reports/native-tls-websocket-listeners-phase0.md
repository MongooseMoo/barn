# Native TLS/WebSocket Listeners: Phase 0 Baseline

Date: 2026-05-06

## Workflow Used

Requested workflow:

- `plans/native-tls-websocket-listeners-workstream.md`

Actual Phase 0 workflow:

1. Reread the workstream.
2. Ran the documented managed conformance command.
3. The first managed run was blocked by a `uv` dependency metadata-name mismatch.
4. Searched the exact `uv` error string before choosing a workaround.
5. Reran the same managed script with `UV_NO_SYNC=1`, using the existing environment.
6. Ran the focused listener/admin/login slice with the same managed script and `UV_NO_SYNC=1`.
7. Confirmed the relevant ToastStunt and Barn listener surfaces from source.

## Managed Command

Documented command from `README.md`:

```powershell
.\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788
```

Working directory:

```text
C:\Users\Q\code\barn
```

Expected result:

- Build Barn.
- Copy `Test_conf.db` to `Test_run.db`.
- Start Barn on TCP port `7788`.
- Run `uv run pytest --pyargs moo_conformance --moo-port=7788 -v`.
- Write logs under `reports/runs/`.

Actual first result:

- Barn built and started.
- `uv` failed before pytest collection:

```text
error: Failed to generate package metadata for `moo-conformance-tests==0.1.0 @ directory+../moo-conformance-tests`
  Caused by: Package metadata name `moo-conformance` does not match given name `moo-conformance-tests`
```

Recorded run:

- `reports/runs/20260506_090701/summary.json`

Cause verified locally:

- Barn `pyproject.toml` depends on `moo-conformance-tests`.
- `..\moo-conformance-tests\pyproject.toml` declares project name `moo-conformance`.
- This is a local path dependency mismatch. I did not edit dependency metadata because local path dependency pins are not acceptable as a committed fix.

## Baseline Run

Command:

```powershell
$env:UV_NO_SYNC = '1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788
```

Result:

```text
2 failed, 2528 passed, 167 skipped in 42.92s
```

Recorded run:

- `reports/runs/20260506_090753/summary.json`
- `reports/runs/20260506_090753/pytest.log`

Failures:

- `algorithms::crypt_invalid_salt`
- `math::ctime_with_int_arg_is_invarg`

These are the Phase 0 TCP baseline failures for this workstream. They are not listener/TLS/WS failures.

## Focused Listener/Admin/Login Slice

Command:

```powershell
$env:UV_NO_SYNC = '1'; .\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788 -K "server_admin or login"
```

Result:

```text
112 passed, 2585 deselected in 4.33s
```

Recorded run:

- `reports/runs/20260506_090853/summary.json`
- `reports/runs/20260506_090853/pytest.log`

Covered focused areas include:

- `connection_name()`
- `connection_info()`
- `listeners()`
- `notify()`
- `connected_players()`
- connection options
- related server/admin behavior selected by the suite's `server_admin or login` pattern

## Source Confirmation

ToastStunt source surfaces read:

- `C:\Users\Q\src\toaststunt\src\network.cc`
  - `nlistener` records store listener handle, resolved name, IP address, fd, port, and TLS metadata behind `USE_TLS`.
  - Accepted sockets become `nhandle` records through `make_new_connection()`.
  - `network_make_listener()` creates listener records and `network_listen()` starts accepting.
  - No native WebSocket implementation was found in this file.
- `C:\Users\Q\src\toaststunt\src\server.cc`
  - `server_new_connection()` assigns negative pre-login players and stores the listener object.
  - `bf_listen()` accepts options including `print-messages`, `ipv6`, `interface`, and TLS options when built with TLS.
  - `bf_listeners()` returns listener maps with object, port, print-messages, ipv6, interface, and TLS where enabled.
- `C:\Users\Q\src\toaststunt\src\tasks.cc`
  - `do_login_task` dispatches login work through the task queue handler, which is the listener object.
  - `read_http()` support is task/parser oriented, not WebSocket support.

Barn source surfaces read:

- `server/connection_manager.go`
  - Current listener records are TCP listener records keyed by port.
  - Accepted connections create `TCPTransport` and then normal Barn `Connection` values.
  - Dynamic `AddListener()` and `RemoveListener()` already exist for TCP.
- `server/transport.go`
  - `Transport` is line-oriented.
  - `TCPTransport` performs telnet IAC filtering and CR/LF line handling.
- `server/scheduler_login.go`
  - Current source already dispatches `do_login_command` through `conn.ListenerObject()`.
  - Current source already passes argstr through `CallVerbWithArgstr`.
  - Current source includes `user_created` and `user_client_disconnected` helpers.

## Phase 0 Exit Status

Phase 0 is complete enough to proceed:

- Baseline command and workaround are recorded.
- Focused listener/admin/login status is recorded.
- ToastStunt TCP/TLS listener source behavior is confirmed.
- ToastStunt has no WebSocket oracle in `network.cc`; WS/WSS semantics must be specified before implementation.

Next unchecked phase:

- Phase 1: Listener Identity and Login Lifecycle.
