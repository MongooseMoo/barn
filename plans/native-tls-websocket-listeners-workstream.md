# Native TLS and WebSocket Listener Workstream

## Goal

Make Barn's network listener surface native and extensible so startup listeners and the MOO `listen()` builtin can create:

- `tcp`: current TCP/telnet line protocol
- `tls`: TCP/telnet line protocol wrapped in TLS
- `ws`: WebSocket over HTTP
- `wss`: WebSocket over HTTPS/TLS

This workstream is a plan only. It does not treat an external websocket-to-TCP proxy as acceptable completion.

## Control Rules

1. ToastStunt is the oracle for TCP/TLS listener behavior where Toast has behavior.
2. ToastStunt is not an oracle for WebSocket behavior; every user-visible WS/WSS semantic must be specified in `spec/` before implementation.
3. The existing TCP byte stream is the regression floor. TCP conformance must remain unchanged after every refactor slice.
4. Implement one listener family at a time in this order: listener identity fixes, shared listener spec, TCP refactor, TLS, WS, WSS.
5. Do not add WS outbound support to `open_network_connection()` in this workstream.
6. Do not add compatibility wrappers around the old listener interface. Change the listener interface, update all callers, then delete old production paths.

## Dependency Order

The phases below are already topologically ordered:

1. Phase 0 establishes baseline evidence and known semantics.
2. Phase 1 fixes listener identity bugs that every protocol would inherit.
3. Phase 2 introduces the target listener representation without adding protocols.
4. Phase 3 proves the new representation preserves TCP.
5. Phase 4 adds TLS, which reuses line semantics and validates the protocol split.
6. Phase 5 specifies WS semantics before code because Toast has no WS oracle.
7. Phase 6 adds WS.
8. Phase 7 adds WSS using the already-proven TLS and WS pieces.
9. Phase 8 finishes docs, CLI compatibility, and full verification.

Do not begin a phase until all previous phase exit gates pass or the user explicitly defers the blocked item.

## Phase 0: Baseline and Source Confirmation

Deliverables:

- Record the current TCP conformance baseline using the managed conformance command.
- Record focused listener/admin test status for `listen()`, `listeners()`, `unlisten()`, `connection_name()`, `connection_info()`, login hooks, and notifier hooks.
- Confirm ToastStunt listener behavior from source:
  - `src/network.cc`: listener records, accept path, TLS options.
  - `src/server.cc`: `listen()`, `listeners()`, `unlisten()`, `server_new_connection`, listener object dispatch.
  - `src/tasks.cc`: login task and `read_http()` behavior.

Exit gate:

- Baseline commands, working directory, and results are recorded in a report.
- Any uncertain Toast behavior is verified against Toast before Barn-side implementation.

## Phase 1: Listener Identity and Login Lifecycle

Rationale:

WebSocket listeners are not useful if custom listener objects cannot own login behavior. Barn currently stores listener object identity but does not fully dispatch through it.

Deliverables:

- `do_login_command` dispatches on the accepting listener object, not always `#0`.
- Login argument handling matches Toast where currently known:
  - parsed words use Toast-compatible command parsing.
  - argstr preserves the original input line.
- Login lifecycle tracks newly-created players so `user_created` vs `user_connected` can be selected.
- Notifier coverage includes:
  - `user_connected`
  - `user_reconnected`
  - `user_created`
  - `user_disconnected`
  - `user_client_disconnected`
- Cross-listener reconnection semantics match Toast where verified.
- `print-messages` controls connect/full/refuse messages where Toast uses it.
- `connection_name()` and `connection_info()` use the actual accepting listener's source port instead of the primary listener port.

Owned files likely involved:

- `server/scheduler_login.go`
- `server/scheduler.go`
- `server/connection.go`
- `server/connection_manager.go`
- `builtins/network.go`
- focused tests under `server/` and `builtins/`

Verification:

- Focused Go tests for listener-object login dispatch.
- Focused Go tests for each notifier path.
- Managed conformance targeted at server/admin/login behavior.
- Full TCP conformance baseline rerun if targeted behavior touches shared login scheduling.

Exit gate:

- Custom listener objects can own login flow.
- TCP behavior remains at or better than Phase 0 baseline.

## Phase 2: Target Listener Representation

Deliverables:

- Add a single internal `ListenSpec` representation with:
  - `Protocol`: `tcp`, `tls`, `ws`, `wss`
  - `Port`
  - `Interface`
  - `Object`
  - `PrintMessages`
  - `Path`
  - `TLSCertificatePath`
  - `TLSKeyPath`
  - future-safe fields for origin/proxy policy, default disabled
- Replace port-only listener identity with a key that can distinguish protocol and path.
- Update listener records to store protocol metadata and canonical listener description.
- Update `ListenerInfos()` and `listeners()` to expose protocol metadata while preserving existing TCP behavior for in-db code.
- Update `unlisten()` to accept the canonical descriptor returned by `listen()`, while preserving integer TCP port unlisten.

Owned files likely involved:

- `server/connection_manager.go`
- `builtins/network.go`
- `builtins/signatures.go`
- `builtins/function_signatures_generated.go`
- `server/server.go`
- tests under `server/` and `builtins/`

Verification:

- `listen(obj, 0)` returns a usable TCP descriptor.
- `listeners()` can filter by object, legacy port, and canonical descriptor.
- `unlisten(desc)` removes exactly the requested listener.
- Duplicate listener rejection is correct for protocol/key collisions.

Exit gate:

- The new representation exists, old port-only production assumptions are removed, and TCP behavior is unchanged.

## Phase 3: Startup Listener Unification

Deliverables:

- Startup listener creation and `listen()` use the same `ListenSpec` path.
- Existing `-port N` remains as shorthand for one `tcp` listener.
- Add repeatable startup listener syntax, for example:
  - `-listen tcp://:7777`
  - `-listen tls://:7778?cert=...&key=...`
  - `-listen ws://:7779/moo`
  - `-listen wss://:7780/moo?cert=...&key=...`
- Reject ambiguous or invalid listener specs with explicit startup errors.

Owned files likely involved:

- `cmd/barn/main.go`
- `server/server.go`
- `server/connection_manager.go`
- CLI parsing tests if the repo has a local pattern for them

Verification:

- Startup with only `-port` behaves like Phase 0.
- Startup with one explicit `tcp://` listener behaves like `-port`.
- Startup with multiple TCP listeners creates listener entries for each.
- Invalid specs fail before database mutation or listener startup.

Exit gate:

- There is exactly one production listener registration path.

## Phase 4: Native TLS Listener

Deliverables:

- Add `tls` listener support using Go `crypto/tls`.
- TLS listeners wrap accepted sockets with `tls.Server`, perform handshake, then use the existing TCP line transport.
- `listen(obj, port, ["protocol" -> "tls", "certificate" -> path, "key" -> path])` creates TLS listeners.
- CLI `tls://` listeners use the same code path.
- `listeners()` reports TLS metadata.
- `connection_name()` and `connection_info()` remain line/TCP-compatible while indicating protocol where the existing return shape permits it.

Semantic decisions:

- TLS certificate and key paths are wizard/startup-only inputs.
- Certificate reload is out of scope unless explicitly added later.
- TLS failure before login closes the socket without creating a logged-in player.

Verification:

- Self-signed cert integration test connects with a TLS client.
- Login/eval works through TLS.
- `listen()`/`listeners()`/`unlisten()` round trip for TLS.
- Connect timeout works after TLS handshake.
- Shutdown closes TLS listener and active TLS connections cleanly.
- TCP conformance remains unchanged.

Exit gate:

- TLS is a native listener family and shares all line semantics with TCP.

## Phase 5: WebSocket Spec Before Code

Rationale:

ToastStunt does not define WS semantics. Barn must specify them before implementation.

Deliverables:

- Add or update `spec/server.md` with WS/WSS extension semantics:
  - listener option keys and canonical descriptors
  - `listeners()` map fields
  - `unlisten()` accepted descriptors
  - HTTP path matching
  - origin policy default
  - non-WS HTTP request response
  - connection naming and connection info fields
  - input framing rule
  - output framing rule
  - text vs binary frame policy
  - high-byte and invalid UTF-8 behavior
  - ping/pong behavior
  - close behavior
  - timeout behavior before login
  - interaction with `notify(..., no_newline)`
  - interaction with OOB `#$#`
  - interaction with `flush_command`
  - interaction with connection `binary` option

Recommended first-cut semantics:

- One complete WS text message is one MOO input line.
- Embedded CR/LF in a WS message is rejected with connection close or split only if the spec chooses that explicitly.
- Server output sends one WS message per logical `notify`/flush line.
- `notify(..., no_newline)` affects TCP/TLS only unless the spec defines a WS analogue.
- Client-supplied forwarding headers are ignored unless a trusted-proxy option is added later.
- Ping/pong frames do not surface as MOO input.

Verification:

- Spec review before implementation.
- Tests in later phases must cite these decisions.

Exit gate:

- No WS/WSS production code begins until the spec decisions above are written.

## Phase 6: Native WebSocket Listener

Deliverables:

- Add a WS acceptor based on an HTTP server and a maintained Go WebSocket library.
- The WS acceptor performs HTTP upgrade and then creates a Barn connection through the normal connection manager lifecycle.
- WS listener records include path and origin policy.
- `listen(obj, port, ["protocol" -> "ws", "path" -> "/moo"])` creates WS listeners.
- CLI `ws://` listeners use the same code path.
- `listeners()` reports WS metadata.
- `unlisten()` shuts down the HTTP server/listener cleanly.

Design constraints:

- Do not construct WS from a raw `net.Conn` by hand-rolling HTTP upgrade.
- Do not run WS payload through the telnet IAC state machine.
- Do not use an external proxy as the implementation.
- Do not broaden `open_network_connection()` to WS in this workstream.

Verification:

- Unit/integration test dials WS and completes login.
- WS one-message-one-line behavior matches spec.
- Embedded newline behavior matches spec.
- Ping/pong frames do not become input.
- Non-WS HTTP request receives the specified response.
- Origin policy accepts and rejects as specified.
- `listen()`/`listeners()`/`unlisten()` round trip for WS.
- Shutdown closes WS listener and active WS connections cleanly.
- TCP and TLS tests still pass.

Exit gate:

- WS is native, listener-owned, and specified.

## Phase 7: Native WSS Listener

Deliverables:

- Add WSS using the same HTTP/WS acceptor over a TLS listener/config.
- `listen(obj, port, ["protocol" -> "wss", "path" -> "/moo", "certificate" -> path, "key" -> path])` creates WSS listeners.
- CLI `wss://` listeners use the same code path.
- `listeners()` reports WSS metadata.

Verification:

- Self-signed cert WSS integration test connects and logs in.
- WSS path/origin behavior matches WS.
- TLS handshake failure does not create a leaked Barn connection.
- Shutdown closes WSS listener and active WSS connections cleanly.
- TCP/TLS/WS tests still pass.

Exit gate:

- WSS is just WS over the native TLS listener machinery, not a separate special-case stack.

## Phase 8: Final Verification and Documentation

Deliverables:

- Update `README.md` or server docs with startup listener examples.
- Update any conformance or managed harness docs if startup syntax changes.
- Add a short browser smoke artifact only if the repo has an accepted location for manual smoke tests.
- Record final verification report with:
  - exact commands
  - working directories
  - test results
  - known deferred semantics, if any

Verification:

- Full Go test suite with `uv run` only where Python tooling is involved; Go commands use the repo's established Go workflow.
- Managed TCP conformance command.
- Focused TLS, WS, WSS integration tests.
- Manual browser smoke for WS/WSS if feasible.

Exit gate:

- All phases complete or explicitly deferred by the user.
- No old production listener path coexists with the target listener path unless explicitly documented as a compatibility surface.

## Completion Definition

The workstream is complete only when:

- Startup and `listen()` use one listener registration path.
- TCP behavior remains compatible with the original baseline.
- TLS listeners are native and line-compatible with TCP.
- WS and WSS listeners are native and specified.
- `listen()`/`listeners()`/`unlisten()` work for every protocol.
- Connection lifecycle, login dispatch, notifier hooks, and connection metadata respect the accepting listener.
- Tests cover TCP regression, TLS, WS, WSS, shutdown, timeout, and listener round trips.
