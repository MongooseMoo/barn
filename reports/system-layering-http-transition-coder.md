# HTTP binary-mode transition repair

## Goal and ownership

Reproduce the published HTTP failure mechanism through the real connection
reader and builtin option setter, then make the existing binary-mode wake
symmetric. Final scope is builtins/network.go, server/connection.go, one focused
server regression file, the obsolete connection-wake review test and its stale
WebSocket owner comment. No publication, no new synchronization owner, and no
CR/telnet changes.

## Prerequisite evidence

Read the focused canonical Toast XML: 26 tests, zero failures/errors/skips.
Matching schema-2 admission records four passed probes. Raw files remain in
.tmp/pr648-toast-http-focused.xml and its matching admission JSON.

## State

The new real TCPTransport/net.Pipe regression reproduced both the missing wake
and the resulting syntax error. It selects binary before HandleConnection starts,
waits for the actual underlying Read, invokes the real builtin through rt.Eval
to disable binary, then sends the real MCT statement shape with LF.

## RED

Command: `go test ./server -run '^TestDisablingBinaryModeWakesBlockedReader$' -count=1 -v`

```text
=== RUN   TestDisablingBinaryModeWakesBlockedReader
binary_input_transition_test.go:78: disabling binary mode left the old binary read blocked
binary_input_transition_test.go:89: first text statement output = "{0, {\"Parse error: syntax error\"}}", open=true; want {1, 42}
--- FAIL: TestDisablingBinaryModeWakesBlockedReader (1.02s)
FAIL github.com/MongooseMoo/barn/server 2.500s
test exit: 1
```

## Selected owner repair

Remove only the truthy condition from the existing binary-mode wake. Both mode
directions must interrupt the already-blocked read so HandleConnection chooses
the newly stored mode. No read loop, framing, parser or transport semantics
changed.

## GREEN

Same explicit test command after the one-line repair:

```text
=== RUN   TestDisablingBinaryModeWakesBlockedReader
--- PASS: TestDisablingBinaryModeWakesBlockedReader (0.02s)
PASS
ok github.com/MongooseMoo/barn/server 1.504s
test exit: 0
```

The expected `{1, 42}` is emitted by actual intrinsic eval after the real reader
reselects text mode. The test joins both the connection-reader and peer-output
reader on close. It uses channel observations rather than sleeps. The RED path
continues after reporting the absent wake, proving the next actual statement
produces a syntax error as well as the missing interruption.

## Initial TCP-only refactor review and limits (superseded below)

The existing setter already stores the new mode before waking the reader; its
existing wake/deadline owner and existing timeout/reselect loop are sufficient.
The repair removes an asymmetric condition rather than adding a second mode
state, callback or synchronization mechanism. This initial conclusion omitted
the WebSocket interaction discovered by the analyst below. At this stage
`gofmt` ran only on the two literal Go files;
`git diff --check` produced no output for the owned changes.

The parent has independently reproduced all nine original failures with the
frozen pre-repair Barn binary. Post-repair managed HTTP and full exact-head CI
remain parent-owned and unverified here. No commit, push, public post or merge
was performed by this worker. Other dirty notes and raw evidence were preserved.

## Revised whole-transport plan

The independent analyst found that waking every transport on binary0 cancels
an unchanged WebSocket read. Root selected the bounded correction in the
existing connection owner: WakeInputReader does nothing for transports that
do not implement BinaryTransport, because HandleConnection uses their same
ReadLine strategy in both modes. No new method/interface/field was added.

Whole-tree source search finds the only production call to WakeInputReader in
builtins/network.go. Actual shutdown uses Connection.closeForShutdown, which
cancels the connection and calls transport.CloseNow when supported. The old
review test's claim that shutdown uses Connection.WakeInputReader was false.
That test now directly exercises WebSocketTransport.WakeReader, retaining its
explicit context-cancellation and stamped-deadline assertions. The transport's
WakeReader implementation is unchanged. Existing CloseNow shutdown tests were
not edited.

The TCP fixture now registers both pipe closes immediately on acquisition,
independently of later goroutine-join cleanup. The new WebSocket regression
uses a controlled wsConn with the real WebSocketTransport/HandleConnection and
actual builtin Eval. It captures the blocked read context, applies binary0 or
binary1, verifies that context remains live, and sends the next actual eval
message. No sleep or new production synchronization was introduced.

### WebSocket RED before connection guard

Command: `go test ./server -run '^TestBinaryModeDoesNotInterruptWebSocketReader$' -count=1 -v`

```text
=== RUN   TestBinaryModeDoesNotInterruptWebSocketReader/0
binary_input_transition_test.go:184: binary option cancelled the unchanged WebSocket read: context canceled
binary_input_transition_test.go:193: binary option disconnected the WebSocket before its next statement
=== RUN   TestBinaryModeDoesNotInterruptWebSocketReader/1
binary_input_transition_test.go:184: binary option cancelled the unchanged WebSocket read: context canceled
binary_input_transition_test.go:193: binary option disconnected the WebSocket before its next statement
--- FAIL: TestBinaryModeDoesNotInterruptWebSocketReader (0.03s)
FAIL github.com/MongooseMoo/barn/server 1.551s
test exit: 1
```

### Combined GREEN after guard and direct review-test correction

Command: `go test ./server -run '^(TestDisablingBinaryModeWakesBlockedReader|TestBinaryModeDoesNotInterruptWebSocketReader|TestReview_WebSocketWakeReaderInterruptsBlockedRead)$' -count=1 -v`

```text
--- PASS: TestDisablingBinaryModeWakesBlockedReader (0.02s)
--- PASS: TestBinaryModeDoesNotInterruptWebSocketReader (0.02s)
    --- PASS: TestBinaryModeDoesNotInterruptWebSocketReader/0 (0.01s)
    --- PASS: TestBinaryModeDoesNotInterruptWebSocketReader/1 (0.01s)
--- PASS: TestReview_WebSocketWakeReaderInterruptsBlockedRead (0.00s)
PASS
ok github.com/MongooseMoo/barn/server 1.583s
test exit: 0
```

Final scoped diff check produced no output. Required CI and the managed HTTP
selection on this final production guard remain parent-owned. The earlier
TCP-only post-repair managed result does not verify this final source.

The parent's final source review also found the stale production WakeReader
comment naming Connection.WakeInputReader/graceful shutdown as callers. Updated
only that comment to the explicit transport cancellation contract and actual
mode/shutdown owners. No WebSocket implementation changed after the combined
GREEN run; no extra test run was made for this comment-only correction.
