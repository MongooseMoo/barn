# HTTP binary-mode transition delta analysis

## Scope and state

Reviewing only the new binary-mode wake condition, the real blocked-reader
regression and its coder evidence. The rest of PR 648 is unchanged and was
previously reviewed. No implementation changes or publication by this analyst.

The advertised role file under .Codex/AGENTS.md.d is absent; the inherited
gauntlet protocol supplies the analyst responsibilities.

## Initial observations

The production change removes the truthy-only wake condition after storing the
new binary option. The regression uses actual TCPTransport, HandleConnection,
the runtime builtin setter, and the subsequent intrinsic eval statement.

## Blocking finding: default binary-off now disconnects WebSockets

The new unconditional binary-option wake also runs when an already-text
WebSocket connection executes `set_connection_option(player, "binary", 0)`.
This has a real production path: ConnectionManager accepts WebSockets and
constructs WebSocketTransport at server/connection_manager.go:393-424; the normal
connection handler and intrinsic eval expose the builtin to that connection.

The changed branch at builtins/network.go:1359 invokes Connection.WakeInputReader
(server/connection.go:243-250). Its WebSocket path invokes WakeReader, which
cancels the context of the in-flight read (server/websocket_transport.go:125-132).
ReadLine returns that error directly at :54-56. HandleConnection only retries
net.Error timeouts (server/input_processor.go:179-199), so an ordinary
context.Canceled read logs an error, returns, runs disconnect processing and
closes the connection. The existing blockingWSConn fixture explicitly returns
ctx.Err() at server/review_server_test.go:117-121, and the existing wake test
asserts interruption, not a usable subsequent connection.

Previously a binary value of zero skipped this wake entirely. The new TCP test
cannot detect this regression. A real blocked WebSocket regression exercising
the actual setter is needed before accepting the new branch. A general
WebSocket read retry is insufficient without checking the actual websocket
library's cancellation contract; this review does not select a transport design.

This is source-backed behavior, not an executed WebSocket reproduction by this
analyst. Parent notified immediately. No source change or test run performed.

## Other observations and limits

TCP text ReadInput preserves its partial builder on timeout, while the binary
chunk's initial Peek returns before mutation. Existing deadline reset precedes
mode selection on the next loop. The minimal TCP wake fixes the demonstrated
blocked-read mechanism without new framing state or locks.

The new test registers its socket cleanup after SwitchPlayer and the enabling
Eval. An early fatal failure in either setup step can leave the newly created
pipe open. This is a small fixture cleanup gap; register socket cleanup directly
after acquisition independently of goroutine joins. No production defect is
inferred from that test-only path.

Managed comparison and required CI remain outside this source review. The
WebSocket regression above blocks accepting the new delta unchanged.

## Real wake/shutdown owner trace

Whole-repository Go-source search shows only one production caller of
Connection.WakeInputReader: the binary-option builtin. Its other caller is the
direct review_server_test assertion. WebSocketTransport.WakeReader is called
only by that Connection method in production and directly by concurrency tests.

Actual shutdown calls Server.shutdownWithCheckpoint ->
ConnectionManager.CloseConnections -> conn.closeForShutdown
(connection_manager.go:245) -> transport.CloseNow (connection.go:213-214).
It does not use either wake method. The old review test's claimed shutdown
contract is therefore not the real shutdown call graph.

The smallest existing-owner correction to consider is restricting
Connection.WakeInputReader to BinaryTransport, because HandleConnection selects
a different input reader only for that capability. WebSocket has no ReadChunk;
both mode values use ReadLine and need no mode-switch interruption. This can
use the existing capability and method rather than adding another exported
method or state owner. A real builtin/WebSocket regression should verify the
unchanged text connection and supersede the direct test's unsupported generic
shutdown framing. Actual CloseNow shutdown coverage already lives in
connection_shutdown_test.go. This is a recommendation to parent, not an
implemented correction or verified outcome.

## Revised delta review: findings resolved in source

Read the final connection guard, both real transport regressions, changed
direct transport wake test, and literal worker RED/GREEN output. The existing
Connection.WakeInputReader now returns before waking a non-BinaryTransport.
That matches HandleConnection's capability dispatch, so WebSocket ReadLine is
no longer cancelled by either binary option value. No new capability interface,
method, state owner or synchronization was introduced.

The new WebSocket regression goes through the actual builtin Eval and real
WebSocketTransport/HandleConnection, observes the parked read context, tests
both zero and one, and requires the subsequent actual eval result `{1, 42}`.
Its original implementation without the capability guard failed both subcases
with context cancellation and disconnect; the coder report preserves the RED.
The existing transport-level cancellation/deadline assertions are retained by
calling WebSocketTransport.WakeReader directly. Actual shutdown CloseNow
coverage is unchanged. This repairs the misleading test target rather than
loosening its transport assertion.

TCP pipe cleanup is now registered immediately after acquisition, before the
fallible SwitchPlayer/Eval setup. Later cleanup still closes the connection and
joins both started readers before the fixture stops the processor/runtime.
The new WebSocket fixture also registers close before fallible setup and joins
the input reader while processing remains active.

Worker's scoped combined GREEN output:

```text
PASS TestDisablingBinaryModeWakesBlockedReader
PASS TestBinaryModeDoesNotInterruptWebSocketReader/0
PASS TestBinaryModeDoesNotInterruptWebSocketReader/1
PASS TestReview_WebSocketWakeReaderInterruptsBlockedRead
ok github.com/MongooseMoo/barn/server 1.583s
test exit: 0
```

No remaining actionable defect found in this revised delta. This is a source
and regression-evidence finding, not a merge verdict. No tests rerun by this
analyst; required exact-head CI and final-source managed HTTP remain unverified
here. No implementation edits or publication performed.
