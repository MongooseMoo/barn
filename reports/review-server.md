# Server/Command/Config Layer Review

## Pre-existing Baseline

All tests pass before this review:
```
ok  barn/server   (all existing tests green)
ok  barn/command
ok  barn/config
ok  barn/profile
?   barn/trace    [no test files]
```

---

## Architecture Summary

The network/command/config layer is divided as follows:

- **server/**: A `Server` owns a `ConnectionManager` (listeners, connections) and an
  `InputProcessor` (single-goroutine event loop). All MOO verb execution runs on the
  `InputProcessor.run()` goroutine; connection goroutines feed it via a capped channel
  (`inputQueue`, cap 256), blocking on a `Done` channel for each event.

- **Shutdown ordering**: `CloseListeners` → `CloseConnections` (waits on `connectionWG`)
  → `input.Stop()` → checkpoint → `scheduler.Stop()` → `backgroundWG.Wait()`. Ordering
  is correct: verb tasks for the final checkpoint run before the scheduler stops.

- **Login flow**: Two paths exist. The synchronous path (`callDoLoginCommand`) is used
  as a fallback; the async resumable path (`dispatchLoginCommand`) is used when
  `do_login_command` exists and supports `read()`. Both paths interpret the same
  `types.Result` via separate near-identical functions.

- **Command parsing**: Token-by-token with quote stripping; preposition detection is
  O(words × preps × aliases). Verb dispatch searches player → location → dobj → iobj.
  Wildcard verb names (`look*s`) are correctly implemented.

- **WebSocket**: Uses `github.com/coder/websocket`; transport-level and text-level
  locking are separated. `WakeReader()` is implemented as a no-op.

- **Config**: Minimal — two boolean options (OUTBOUND\_NETWORK, PROMOTE\_NUMBERS).
  Parser rejects unknown keys as hard errors (intentional design, but diverges from Toast).

---

## CONFIRMED Bugs (red tests in `server/review_server_test.go`)

### CONFIRMED-1 — Security hole: hardcoded wizard fallback in `callDoLoginCommand` [CRITICAL]

**Location**: `server/input_login.go:47`

**Code**: When the listener handler object has no `do_login_command` verb, the function
returns `types.ObjID(2), nil` — the wizard player. Every unauthenticated connection
gets instant wizard access with no password check.

**Red test output**:
```
--- FAIL: TestReview_FallbackLoginReturnsWizardWithNoLoginHandler (0.00s)
    review_server_test.go:43: callDoLoginCommand with no do_login_command verb returned player #2 (wizard):
        security hole — unauthenticated connections get instant wizard access
        (input_login.go:47 hardcoded fallback `return types.ObjID(2), nil`)
```

**Note**: This path is only reachable when the synchronous (non-resumable) login
fallback is taken — i.e., when `dispatchLoginCommand` falls back because the handler
exists but has no `do_login_command`. In a correctly configured server with a full
database this is dead code, but it is a live bug for any misconfigured or minimal setup.

---

### CONFIRMED-2 — `ConnectionNameLookup` always returns `nil` error [MEDIUM]

**Location**: `server/connection_manager.go:739`

**Code**: The function signature is `(string, error)`. Line 739 is always
`return resolved, nil`. The error from `net.LookupAddr` is assigned to `err` (line 733)
but never used in the return. The error from `net.SplitHostPort` (line 722) is shadowed
by the subsequent `err` from `LookupAddr`. Callers expecting error propagation get a
silent partial result on any address that fails DNS lookup.

**Red test output**:
```
--- FAIL: TestReview_ConnectionNameLookupNeverErrors (0.00s)
    review_server_test.go:65: ConnectionNameLookup returned nil error for an invalid
        remote address: errors from net.SplitHostPort and net.LookupAddr are silently
        discarded (connection_manager.go:739 always returns `resolved, nil`)
```

---

### CONFIRMED-3 — `WebSocketTransport.WakeReader()` is a no-op, blocking graceful per-connection shutdown [MEDIUM]

**Location**: `server/websocket_transport.go:85`

**Code**: `func (t *WebSocketTransport) WakeReader() {}` — implements the `WakeReader`
interface but does nothing. `Connection.WakeInputReader()` checks for the `WakeReader`
interface, finds it on `WebSocketTransport`, calls it, and **returns early** without
falling through to `SetReadDeadline(time.Now())`. Consequence: a goroutine blocked in
`WebSocketTransport.ReadLine()` cannot be interrupted by `conn.Close()` or
`conn.cancel()`. Per-connection graceful shutdown hangs for WebSocket connections until
the underlying TCP socket closes (which only happens at full `httpServer.Close()` time,
not at individual connection close time).

The fix is one of: remove `WakeReader()` from `WebSocketTransport` (so fallthrough to
`SetReadDeadline` occurs), or implement it as `t.SetReadDeadline(time.Now())`.

**Red test output**:
```
--- FAIL: TestReview_WebSocketWakeInputReaderDoesNotSetDeadline (0.00s)
    review_server_test.go:92: WakeInputReader on a WebSocketTransport left readDeadline
        at zero: WakeReader() is a no-op (websocket_transport.go:85) and satisfies the
        WakeReader interface, preventing fallthrough to SetReadDeadline(time.Now()).
        Result: conn.Close() cannot interrupt a blocked WebSocket read.
```

---

## SUSPECTED Bugs (code-based, no red test)

### SUSPECTED-1 — `OpenNetworkConnection` polling loop never resolves [CRITICAL]

**Location**: `server/connection_manager.go:679-715`

After `net.Dial` succeeds, the function polls `cm.connections` (sleeping 10 ms) for up
to 2 seconds, looking for a new entry with a `connID` not present before the dial. But
for *outbound* connections no code path calls `NewConnectionFromTransport` or
`handleNewConnection` — those are only invoked from the accept loop for *inbound*
connections. The `cm.connections` map never gains the new entry. The poll always
exhausts its 2-second window, closes the client socket, and returns
`"timed out waiting for outbound connection to register"`. The `outboundClients` map is
dead code (the connID condition that populates it is never true). The `open_network_connection()`
builtin is non-functional.

### SUSPECTED-2 — OOB silently discarded when `IsOutOfBand && disable-oob` [HIGH]

**Location**: `server/input_processor.go:201-208`

```go
if input.IsOutOfBand || (oob && !disableOOB) {
    if !disableOOB {
        p.processOutOfBand(input)
    }
    return   // ← silently dropped when IsOutOfBand && disableOOB
}
```

A transport-level OOB frame (telnet IAC, WebSocket binary) sets `input.IsOutOfBand=true`
independently of the `#$#` text prefix. When `disable-oob` is set, the outer `if` still
fires (because `IsOutOfBand` is true), `processOutOfBand` is skipped, and the event is
discarded with no logging. The `disable-oob` connection option is documented as
suppressing the MOO-protocol OOB text prefix, not transport-layer negotiation. Telnet
WILL/WONT/DO/DONT commands are thus silently eaten when the option is set.

### SUSPECTED-3 — `ForceInput` re-enters `processInput` from the input goroutine [HIGH]

**Location**: `server/input_processor.go:291-293`

When `ForceInput` is called for a pre-login player that has a `connID`, it calls
`p.processInput(evt)` directly, synchronously, on the same goroutine that is the
consumer of `inputQueue`. If that nested `processInput` call ever sends to
`p.inputQueue` (e.g. from a login verb path that enqueues work), and the queue is full
(cap 256 with 256 concurrent connections backed up), the goroutine deadlocks — it is
both the blocked producer and the only consumer. Current code paths through pre-login
`processPreLogin` → `dispatchLoginCommand` avoid this specific channel write, so no
immediate deadlock, but the pattern has no re-entrancy guard and is one code change away
from locking up.

---

## ARCHITECTURAL Findings

### ARCH-1 — `OpenNetworkConnection` architecture is absent, not just broken [CRITICAL]

There is a `outboundClients map[int64]net.Conn` data structure and a polling loop that
expects outbound connections to self-register, but no code ever performs that
registration. The feature cannot be fixed by a small patch; it requires designing an
outbound connection handler that calls `NewConnectionFromTransport` on the dialed socket
and routes it through `handleConnection`.

### ARCH-2 — Duplicate login result interpretation [MEDIUM]

`callDoLoginCommand` (synchronous path) and `interpretLoginResult` (async/resumable
path) in `server/input_login.go` contain near-identical logic: check for
`FlowException`, extract `ObjValue`, validate the player flag, fall back to
`conn.GetPlayer()`. Any logic change to one must be manually applied to the other. Should
be a single shared function.

### ARCH-3 — Singleton `task.GetManager()` as global state [MEDIUM]

`task.GetManager()` returns a process-global singleton. Tests in `server/` must call
`resetTaskManager()` via `t.Cleanup()` to prevent cross-test contamination. Adding
`t.Parallel()` to any of these tests without further isolation would cause non-
deterministic failures. The global task manager is also a namespace conflict: connIDs
restart at 2 in every new `ConnectionManager`, so tasks from different `ConnectionManager`
instances in the same process share the same negative-player namespace.

### ARCH-4 — `inputQueue` backpressure cliff [HIGH]

`inputQueue` has capacity 256. Each connection goroutine blocks after enqueueing an event,
waiting on a `Done` channel. If 256+ connections simultaneously flood input (or if the
scheduler is slow for a burst), the 257th connection goroutine blocks forever in
`EnqueueInput`. This is a DoS vector: a slow verb (e.g. via `suspend()`) stalls the
consumer, and enough concurrent connections exhaust the queue and hang.

### ARCH-5 — config parser rejects unknown keys (Toast incompatibility) [LOW]

`config/parser.go:61` returns an error for any unrecognized option key. Toast ignores
unknown keys. This is an intentional design choice (the existing test
`TestParseRejectsInvalidConfig` asserts this behavior), but it is a compatibility hazard:
any future Barn option addition breaks existing `.conf` files until users update them.

### ARCH-6 — `.pr` (2 chars after prefix) accepted as `.program` abbreviation [LOW]

`command/intrinsic.go:47-54`: `matchesProgramIntrinsic` accepts `.pr` alone (the
remainder is empty, and `strings.HasPrefix("ogram", "")` is true). Toast behavior for
the minimum abbreviation length is unconfirmed. The existing test explicitly asserts
`.pr` is valid (`intrinsic_test.go:10`), so this may be intentional, but it differs from
what most MOO users would expect.

### ARCH-7 — `tokenizeCommandWords` uses byte-level iteration on UTF-8 input [LOW]

`command/command.go:114-151`: `ch := input[i]` reads one byte at a time.
`unicode.IsSpace(rune(ch))` is passed a single byte — multi-byte Unicode spaces (NBSP,
etc.) are never treated as word separators. High bytes (128-254) in the character filter
can misinterpret UTF-8 continuation bytes as separate characters. MOO is traditionally
ASCII; Toast accepts UTF-8 but Barn's tokenizer is not Unicode-safe.

---

## Race Detector

`go test -race ./server/... ./command/... ./config/... ./profile/...` with the new test
file finds **no data races** beyond the three intentionally-failing review tests.

---

## Test File Created

`server/review_server_test.go` — three failing tests:
- `TestReview_FallbackLoginReturnsWizardWithNoLoginHandler` (CONFIRMED-1)
- `TestReview_ConnectionNameLookupNeverErrors` (CONFIRMED-2)
- `TestReview_WebSocketWakeInputReaderDoesNotSetDeadline` (CONFIRMED-3)
