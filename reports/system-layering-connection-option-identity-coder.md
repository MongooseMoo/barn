# Connection option identity implementation

## Goal and state

Implement the selected physical-connection option key and disconnect cleanup.
The selected store-key change and first implementation's focused evidence are
recorded below. Its ten-sample timing evidence detected extra input/held-input
cost. Parent selected the explicit producer-ID revision at the end of this
report; that revision and the selected omitted-caller correction are implemented.
Focused correctness and controlled residual timing costs are recorded below.
Parent retained the direct design with those costs explicit, then selected a
stale-setter lifetime correction after independent analysis. That correction
has behavioral RED/GREEN and related focused evidence at the end. The focused
window is released; no publication or merge is authorized to this worker.

## Evidence

The regression compiles/runs a real MOO task that sets binary on player two and
switches to player three, then queries both in the following task. Before repair:

```text
options after committed switch: {0, E_INVARG}; want {1, E_INVARG}
FAIL github.com/MongooseMoo/barn/server 1.467s
```

Two earlier compile probes corrected Go raw-string syntax and the Value.ErrCode
accessor before this behavioral RED. They are not counted as the witness.
After the selected identity change:

```text
--- PASS: TestSwitchPlayerPreservesCommittedConnectionOptions (0.01s)
ok github.com/MongooseMoo/barn/server 1.039s
```

The Session option store keeps its existing mutex. Socket identity comes from
the existing immutable Connection.ID, including actual callers already holding
a connection. Disconnect removes only that physical entry after manager unlock.
Held command/HTTP state remains player-owned. Same-task deferred views remain
outside this correction.

## Initial lock and caller audit (superseded by the selected revision below)

`Connection.ConnectionID` returns immutable `ID` without `Connection.mu`.
Notify, both option getters and setter reuse the connection already resolved by
their existing owner. HandleConnection and processInput use the existing
physical connection/event ID. ForceInput reuses its existing single connection
lookup, moved ahead of option classification, instead of looking up again.

The player-owned HTTP helpers call heldInputEnabled while holding their HTTP
mutex; heldInputEnabled now resolves the player through ConnectionManager.mu
before taking the Session option read lock. ConnectionManager has no Session
owner/reference. Its callbacks (including SwitchPlayer's event producer) run
after manager unlock. processDisconnect releases manager.mu before dropping the
physical option entry and before CloseHeldHTTPInput; no manager-to-HTTP inversion
was found in these real owners. The held command/HTTP maps remain player-keyed.

## Focused fixture evidence

```text
go test ./builtins -run '^(TestSingleConnectionOption.*|TestSessionConnectionAndHeldInputStateIsIsolatedAndClosed|TestFlushCommandMatchesCaseInsensitivelyAndClearsPendingCommands)$' -count=1 -v
ok github.com/MongooseMoo/barn/builtins 1.447s
```

The fakes now expose explicit physical IDs, including the embedded engine
snapshot/output fixture. HTTP retained-buffer and waiter fixtures now own an
actual connection when enabling held input. No player-to-ID fallback exists.

```text
go test ./server -run '^(TestSwitchPlayerPreservesCommittedConnectionOptions|TestConnectionOptionsFollowPreloginIdentityAndResetOnReconnect|TestStaleDisconnectForgetsOnlyItsPhysicalOptions|TestConnectionOptionReadDoesNotWaitForOutputMutex)$' -count=1 -v
--- PASS: TestSwitchPlayerPreservesCommittedConnectionOptions (0.02s)
--- PASS: TestConnectionOptionsFollowPreloginIdentityAndResetOnReconnect (0.01s)
--- PASS: TestStaleDisconnectForgetsOnlyItsPhysicalOptions (0.03s)
--- PASS: TestConnectionOptionReadDoesNotWaitForOutputMutex (0.02s)
ok github.com/MongooseMoo/barn/server 1.050s

go test ./engine -run '^TestRunTaskReleasesSnapshotBeforeBlockedCommittedOutput$' -count=1 -v
--- PASS: TestRunTaskReleasesSnapshotBeforeBlockedCommittedOutput (0.02s)
ok github.com/MongooseMoo/barn/engine 1.019s
```

A later focused builtin selection passed the scalar option, independent snapshot,
session-close, notify-permission, flush, retained HTTP-buffer, and HTTP-waiter
cases, but exposed two remaining NoHost held-input fixtures:

```text
network_http_test.go:254: expected held input to be intercepted
network_http_test.go:294: expected partial input to be intercepted
FAIL github.com/MongooseMoo/barn/builtins 1.374s
```

Both fixtures now expose explicit physical connections rather than creating a
player option without a connection. Their repaired focused run:

```text
go test ./builtins -run '^(TestPrepareHTTPReadReturnsZeroAfterInvalidBinaryInput|TestKilledHTTPReadClearsBufferAndAllowsFreshParse)$' -count=1 -v
--- PASS: TestPrepareHTTPReadReturnsZeroAfterInvalidBinaryInput (0.01s)
--- PASS: TestKilledHTTPReadClearsBufferAndAllowsFreshParse (0.02s)
ok github.com/MongooseMoo/barn/builtins 0.943s
```

## Repeated performance evidence and limitation

The Go benchmarking statistics skill was used for repeated sample comparison.
Both sides ran the same edited benchmark file on Windows/amd64, Ryzen 5950X,
default GOMAXPROCS 32. Measurements were serialized after the binary-input worker
released its timing window. No CPU governor was pinned. The benchmark fake
manager does not model production manager-lock contention.

```text
go test ./builtins -run '^$' -bench '^(BenchmarkSingleConnectionOption|BenchmarkConnectionOptionConsumers)$' -benchmem -count=10 -benchtime=200ms
before: ok github.com/MongooseMoo/barn/builtins 46.905s
after:  ok github.com/MongooseMoo/barn/builtins 44.348s
benchstat reports/system-layering-option-identity-before.txt reports/system-layering-option-identity-after.txt
```

Baseline temporarily restored only the original three builtin production files
through apply_patch, adapting typed test-only arguments for compilation; selected
contents were retained and restored exactly before the after run. The benchmark
host/physical-ID fixture was identical on both sides. No branch reset, stash or
other worker's edits were used. Two final HTTP fixture repairs do not affect
these benchmark bodies.

| Consumer | Before ns/op | After ns/op | benchstat |
| --- | ---: | ---: | --- |
| Truthy default | 12.10 | 12.92 | +6.78%, p=0.000 |
| Truthy stored | 27.08 | 28.72 | +6.06%, p=0.000 |
| Held default | 12.16 | 15.71 | +29.10%, p=0.000 |
| Held stored | 25.57 | 30.98 | +21.20%, p=0.000 |
| Notify default | 38.04 | 38.17 | no significant difference, p=0.468 |
| Notify stored binary | 110.6 | 112.6 | +1.76%, p=0.011 |
| Getter default | 25.77 | 26.66 | +3.49%, p=0.000 |
| Getter stored | 40.85 | 41.59 | +1.84%, p=0.000 |
| Ordinary input default | 39.69 | 47.33 | +19.25%, p=0.000 |
| Ordinary input stored | 68.13 | 79.15 | +16.17%, p=0.000 |

Every row has ten samples per side. All byte/allocation counts are unchanged:
scalar/getter/input/default-notify zero, binary-notify 16 B and two allocations.
Snapshot defaults remain 896 B/nine allocations and stored snapshots 400 B/two.
Snapshot timings have wide 12–18% median confidence intervals and move strongly
between capture windows; do not use their apparent improvement or the mixed
suite geomean to claim safety. Raw samples and complete benchstat output are in
the adjacent before/after/benchstat text files.

The selected player-owned held-input helper necessarily gains real connection
resolution. Its additional measured cost is reported without substituting a new
design. No all-input or overall performance non-regression is established.

## Next step

Parent stopped the first implementation after the measured regression, then
selected the revision below. The original raw correctness and timing evidence
remain intact. Broad CI/isolation verdicts and publication remain unrun by this
worker. Same-task pending lifecycle views remain unfixed.

## Existing identity at real held-input producers

Read-only inventory for parent's revised plan; no alternate implementation:

- server/input_processor.go:403 calls HandleHeldInput with an InputEvent already
  carrying ConnID. processInput also resolves the physical connection earlier
  when refreshing input.Player.
- server/input_processor.go:495 calls HandleHeldInput from ForceInput after its
  existing single lookup has already computed connID at lines 486–490.
- builtins/network.go:1456 calls prepareHTTPRead after resolveConnection returned
  conn. The physical ID is available there without another manager lookup.
- collectHTTPWakeupsLocked has one production caller, HandleHeldInput at line
  645. It can receive the identity already available at the input producer;
  prepareHTTPRead's immediate-completion held-input probes can similarly receive
  its resolved caller's ID.
- CancelHTTPReadTask scans the player-keyed HTTP states and probes hold-input
  after removing a task at line 751. Current httpHeldInput has buffer,
  bufferCapacity, invalidCount and waiters; waiter has task and kind. Neither
  carries a physical connection ID. Its only production caller is
  engine/task_rows.go:31 after KillAtRow, passing row.ID; task t is in scope there,
  but no physical connection ID is established by that caller.

The held HTTP map remains player-owned. Threading an existing producer's ID
through these calls, or adding identity metadata to its existing state, requires
parent's revised written plan before code changes.

## Revised plan for parent selection: carry producer identity, resolve only cancellation

Status: selected by parent; implementation authorized, timing requires a
serialized window after focused correctness. This supersedes the first design's requirement
that every internal held-input probe resolve the player through the manager.
The physical option store, its mutex, disconnect cleanup and builtin interface
remain exactly as selected. Only input/HTTP option-read data flow changes.

### Exact signatures and production producers

1. Change heldInputEnabled to `heldInputEnabled(connectionID int64) bool`.
   Its body calls ConnectionOptionTruthy(connectionID, "hold-input") directly.
   It must not resolve a player, infer an ID from a player, or use a fallback.
2. Change the existing public method to
   `HandleHeldInput(player types.ObjID, connectionID int64, line string, atFront bool) (bool, []string)`.
   Read flush-command and hold-input using that ID. Delete both manager lookups
   currently performed by this method and its heldInputEnabled call. Continue
   indexing heldCommands and heldHTTPInput by player, and preserve parsing,
   waiter consumption, atFront, flushing and command enqueue behavior.
3. Change the internal collector to
   `collectHTTPWakeupsLocked(player types.ObjID, connectionID int64, state *httpHeldInput) []httpWake`.
   Its sole production caller is HandleHeldInput; pass the same input producer's
   ID. Keep the existing option-read position and all empty-state checks.
4. Change the read preparation method to
   `prepareHTTPRead(player types.ObjID, connectionID int64, kind string, t *taskpkg.State) (types.Value, bool)`.
   Both immediate-completion/invalid-input held probes use the supplied ID.
   builtinReadHTTP already owns a connection from resolveConnection; pass
   `conn.ConnectionID()` with its existing player target. Admission, permission,
   task selection, suspension and queue ordering remain unchanged.
5. In processInput, call HandleHeldInput with `input.Player, input.ConnID`.
   The InputEvent is the existing physical producer. Do not add a manager lookup
   or modify how queued input refreshes input.Player.
6. In ForceInput, call HandleHeldInput with its already-computed connID from the
   existing one connection lookup. For a nonexistent physical connection the
   existing connID zero has default false/empty options; retain the existing
   synthetic negative-lane assignment only at its existing queue boundary.
   Do not manufacture a physical ID from the player for option reads.
7. Keep `CancelHTTPReadTask(taskID int64)` and the engine/task_rows.go caller
   unchanged. Only this cold cancellation path lacks a producer physical ID.
   Inline the current manager resolution there at the existing hold-input probe:
   start held=false; if a manager and actual current player connection exist,
   read hold-input using that actual conn.ConnectionID(); otherwise remain false.
   This is the same selection the current helper makes, not a fallback. Keep
   its existing waiter scan, buffer clearing and deletion checks in place.

No change to getOrCreateHeldHTTPInput(player), httpHeldInput/httpReadWaiter
fields, player-keyed maps, HasPendingHTTPRead, CloseHeldHTTPInput,
ClearAllHeldHTTPInput, Session.Close, or any queue/worker signature. No new
interface, index, cache, alias, wrapper, notification scan or cancellation scan.

### Why not put a connection ID on player-owned HTTP state

The real replacement producer is ConnectionManager.SwitchPlayer: it changes
playerConns to the replacement, then unlocks and closes the displaced socket.
It does not notify Session of a state rebind. The old disconnect intentionally
skips CloseHeldHTTPInput when replacementActive is true. A last-producer ID on
the retained player state would therefore still identify the displaced socket
until another input/read_http call arrived. CancelHTTPReadTask can run in that
interval from the task-row observer and would query the wrong option bag.

Keeping that metadata current would require a new lifecycle rebind operation
and coordinating it with manager publication/HTTP state locking. Clearing state
on mismatch would discard existing player-owned waiters/buffer; rejecting an
old producer would change queue behavior. Neither is needed to remove measured
hot-path lookups. Explicit IDs on existing producers plus unchanged current
resolution on cold cancellation avoid all of those additions.

### Replacement, reconnect, and teardown audit

- A normal player switch retains one physical ID; subsequent input/read probes
  use that ID directly. The option bag follows the socket as already proven.
  No held-buffer/command map entry migrates to another player.
- A replacement socket has its own option defaults. A replacement input/read
  producer uses that replacement ID while retaining current player-owned HTTP
  state and waiter policy. Cancellation between replacement and its first new
  input still resolves the replacement through the existing manager mapping.
- Already-queued input from a displaced socket retains its own input.ConnID.
  The revised plan reads that physical producer's options; it does not introduce
  filtering, redirect queues, clear buffers, or suppress existing player-owned
  waiter delivery. This is an explicit consequence of physical option identity;
  stale input delivery policy is not redesigned here.
- A stale disconnect still deletes only its own physical option entry and
  preserves replacement HTTP state through the existing replacementActive guard.
  A real current disconnect still closes player-owned HTTP state after manager
  unlock. Reconnect after completed teardown gets a fresh ID and defaults.
- Empty-state retention/deletion, held command draining, HTTP waiter kills,
  ClearAllHeldHTTPInput and Session.Close retain their existing ownership and
  cleanup behavior. No metadata exists to become stale after those operations.
- Ordinary input/read HTTP paths lose the HTTP-to-manager lock edge for their
  option probes. Cold cancellation retains its existing HTTP -> manager ->
  Session option lock order; processDisconnect retains manager unlock before
  Session calls. No new lock is introduced.

### Fixtures and bounded verification after selection

Update every direct caller to pass its actual fixture ID: builtin
connection_option_lookup_test.go (consumer benchmark), http_buffer_consumption_test.go,
network_http_test.go, network_test.go, network_waiter_test.go,
session_runtime_test.go, and server/input_flush_test.go. The last file's stale
disconnect fixture already holds `replacement`, so pass replacement.ID rather
than a player conversion. Raw store-only tests may still use explicit IDs
without creating a transport. No optional variadic parameter or legacy shim.

Add a fixture assertion that ordinary HandleHeldInput and immediate HTTP read
preparation do not call the connection manager when given the producer ID. Add
cancellation coverage where a replacement is installed before any new producer
updates player-owned HTTP state: cancellation must observe replacement hold
mode, preserve surviving waiters, retain the existing rule that cancelling a
matched waiter clears buffered bytes/invalid input, and permit normal teardown.
An unrelated cancellation must preserve existing bytes. Check empty-state
retention with replacement hold-input on and deletion with it off. Add an
ordinary input case with player numeric value different from connection ID. Keep the
four physical-ID server regressions and stale-disconnect held-input test.

Run only the explicitly named edited-file tests locally. After correctness,
repeat the same consumer benchmarks with ten samples and unchanged workloads;
serialize captures, use an interleaved/affinity-controlled comparison for the
small notify delta, and retain raw samples and allocations. The original raw
before/after/benchstat artifacts remain untouched. No non-regression claim
precedes measurements. Parent owns selection, broad CI and publication.

### Stopped: caller inventory omitted builtinRead

The revision's claim that every physical-ID producer was inventoried was wrong.
The search covered HandleHeldInput, prepareHTTPRead and collector calls, but not
all heldInputEnabled callers. Focused builtin/server builds both failed before
running tests:

```text
builtins\connection.go:39:76: cannot use player (variable of int64 type "github.com/MongooseMoo/barn/types".ObjID) as int64 value in argument to ctx.Session.heldInputEnabled
FAIL github.com/MongooseMoo/barn/builtins [build failed]
FAIL github.com/MongooseMoo/barn/server [build failed]
```

Repository-wide heldInputEnabled search now shows one additional production
caller, builtinRead at builtins/connection.go:39. It already calls
`cm.GetConnection(player)` for admission at line 36 and discards the result.

Proposed exact addition to the selected producer plan, pending parent selection:
keep its nil-manager admission check, retain its existing GetConnection result,
reject nil as before, and pass conn.ConnectionID() to heldInputEnabled. Do not
add another lookup, use a player cast, or change read admission/permission/
suspension semantics. Add a focused read() nonblocking fixture with target player
different from physical ID, checking hold-input rejection and exactly one
existing admission lookup. Re-run the previously named focused tests after the
correction. The typed helper signature is the mechanical build guard that
exposed the omitted producer. Parent selected this correction; it is now
implemented without another lookup or changed nil/permission/error order.

### Revised focused correctness

The repository-wide heldInputEnabled caller audit now includes builtinRead
admission, collector, HandleHeldInput, both prepareHTTPRead probes, cancellation,
and direct benchmark/allocation fixtures. Input/read producers pass physical IDs
they already own; cancellation alone resolves the current player connection.

```text
--- PASS: TestHeldInputUsesProducerIDWithoutManagerLookup (0.01s)
--- PASS: TestReadUsesAdmittedPhysicalIDWithoutSecondLookup (0.02s)
--- PASS: TestHTTPCancellationUsesReplacementBeforeNewInput (0.02s)
ok github.com/MongooseMoo/barn/builtins 1.687s
```

That explicit builtin selection also ran the scalar option/allocation, retained
buffer, malformed/front insertion, cancellation, waiter storage, flush, session
isolation, and invalid/killed HTTP fixtures named above.

```text
--- PASS: TestSwitchPlayerPreservesCommittedConnectionOptions (0.02s)
--- PASS: TestConnectionOptionsFollowPreloginIdentityAndResetOnReconnect (0.01s)
--- PASS: TestStaleDisconnectForgetsOnlyItsPhysicalOptions (0.02s)
--- PASS: TestConnectionOptionReadDoesNotWaitForOutputMutex (0.01s)
--- PASS: TestFlushCommandReportsAndDiscardsPendingInput (0.01s)
--- PASS: TestSupersededConnectionDisconnectPreservesReplacementHeldInput (0.01s)
ok github.com/MongooseMoo/barn/server 1.541s
```

The real flush fixture uses player 424244 with physical ID 2 and now includes a
ForceInput command/receipt between ordinary input and flushing, asserting FIFO
output. The existing stale-disconnect fixture passes the actual replacement.ID.
All original benchmark artifacts remain unchanged. Exclusive timing has been
requested for alternating affinity-controlled prebuilt base/candidate binaries.

### Revised controlled measurement: stopped on remaining regressions

Parent granted one exclusive capture window after other workers released it.
Exactly ten balanced pairs ran: odd pairs base/candidate, even pairs
candidate/base. Both Windows/amd64 benchmark binaries were compiled once, then
left fixed throughout all twenty processes. GOMAXPROCS was 1 and each benchmark
shell's processor affinity mask was 4, inherited by its child. Every invocation
reported mask 4 and exit 0. No CPU governor/turbo control was applied.

The before binary restored the original three builtin production files. The
same benchmark workloads and connection fixtures remained in place; temporary
test-only adaptations removed the extra physical-ID arguments for original
signatures, restored original typed player arguments, and made new test fixture
ID constants untyped so original signatures compiled. Tests were disabled.
No extra branch checkout or copied implementation was used. Before compiling
the candidate, all selected source/fixture contents were restored; the source
fingerprints recorded before temporary restoration match those at candidate
compilation. Complete source and binary SHA256 fingerprints are in the manifest.

```text
go version go1.27.1 windows/amd64
go test ./builtins -c -o C:/Users/Q/AppData/Local/Temp/barn-option-identity-base.test.exe
go test ./builtins -c -o C:/Users/Q/AppData/Local/Temp/barn-option-identity-candidate.test.exe
both builds: exit 0
each fixed binary: -test.run=^$ -test.bench=^(BenchmarkSingleConnectionOption|BenchmarkConnectionOptionConsumers)$ -test.benchmem -test.count=1 -test.benchtime=200ms
benchstat reports/system-layering-option-identity-revised-base.txt reports/system-layering-option-identity-revised-candidate.txt
```

| Case | Base ns/op | Selected ns/op | benchstat, ten samples per side |
| --- | ---: | ---: | --- |
| Truthy default | 12.09 | 12.18 | no significant difference, p=0.101 |
| Held default | 12.14 | 12.30 | +1.40%, p=0.001 |
| Snapshot default | 647.2 | 644.2 | no significant difference, p=0.247 |
| Truthy stored | 27.05 | 27.18 | +0.46%, p=0.022 |
| Held stored | 25.54 | 25.75 | +0.82%, p=0.002 |
| Snapshot stored | 531.8 | 521.9 | -1.85%, p=0.000 |
| Notify default | 38.15 | 38.93 | no significant difference, p=0.143 |
| Getter default | 25.86 | 26.38 | no significant difference, p=0.383 |
| Ordinary input default | 39.83 | 41.55 | +4.32%, p=0.001 |
| Notify stored binary | 130.0 | 135.2 | +3.96%, p=0.000 |
| Getter stored | 41.02 | 41.01 | no significant difference, p=0.853 |
| Ordinary input stored | 70.48 | 69.38 | -1.57%, p=0.000 |

Every allocation/byte count is identical to its base case. Raw affinity/order,
samples and complete statistics are saved as
system-layering-option-identity-revised-manifest.txt,
system-layering-option-identity-revised-base.txt,
system-layering-option-identity-revised-candidate.txt, and
system-layering-option-identity-revised-benchstat.txt. The earlier sequential
before/after files remain untouched and should not be combined statistically
with this different affinity/GOMAXPROCS capture.

The larger manager-lookup overhead is removed structurally, with focused tests
showing zero hot manager probes. Remaining measured regressions still prevent
a performance-neutral claim. In particular, default input and stored binary
notify are significantly slower in this selected compiled pair. Source analysis
has not isolated their cause; do not attribute the residual to a particular
instruction, compiler layout, clock, or allocation. Default getter confidence
interval is wide (24%) and some baseline samples are noisy despite interleaving.

Per the hard constraint, no alternate implementation, additional timing or Go
compilation follows this result. The exclusive window was released to parent.
Parent owns the next design decision. The selected code, focused correctness
evidence, source fingerprints, binaries and raw measurement artifacts remain
available; no publication or merge has occurred.

## Selected stale-setter cleanup correction: regression first

Parent selected the analyst's cause-level correction after reading
system-layering-connection-option-identity-analyst.md. Expose the real existing
Connection context lifetime as Done(); cancel natural disconnect after manager
unlock and before Forget; check that captured channel under the existing option
mutex in the private setter, returning false without mutation if closed. The
builtin preserves normal zero and skips wake/drain when mutation is skipped.
Reads remain ID-only; no new lock, closed-ID index or Session-wide closed state.

The existing server identity test file now contains the deterministic real
producer regression. A manager wrapper delegates to the real GetConnection,
captures that actual result and pauses before returning it to the admitted
builtin. The test suppresses the target disconnect hook, invokes real
processDisconnect, releases/joins the setter, and checks normal zero and no
physical-ID binary override. Idempotent release/join cleanup is registered
before the goroutine starts. This test was run before the production correction.

```text
go test ./server -run '^TestAdmittedOptionSetterCannotRecreateDisconnectedBag$' -count=1 -v
connection_option_identity_test.go:113: admitted setter recreated the disconnected physical connection's binary option bag
--- FAIL: TestAdmittedOptionSetterCannotRecreateDisconnectedBag (0.02s)
FAIL github.com/MongooseMoo/barn/server 1.062s
```

Existing live-setter-then-disconnect tests cover the inverse ordering: live
binary mutation is visible, and subsequent physical cleanup removes it.
The selected production correction followed that behavioral RED. Real Done()
returns the existing ctx.Done without an output/metadata lock. Natural
processDisconnect invokes the existing cancel after manager unlock and before
the existing Forget call. The sole production private-setter caller supplies
that actual ID and channel. The private setter checks Done while holding its
existing option mutex and reports whether it mutated. A closed lifetime returns
false before map access/creation; builtin then returns its normal zero before
input wake/drain. Existing validation and permission order stay in place.

The ordering covers both schedules: if live mutation wins the option mutex,
disconnect's subsequent locked Forget deletes it; if cancellation wins, the
closed channel is checked under that mutex and prevents recreation. Cancellation
after an open check but before mutation still leads to the subsequent locked
delete. No tombstone, extra lock or closed Session semantics was added.

```text
same exact regression command after correction:
--- PASS: TestAdmittedOptionSetterCannotRecreateDisconnectedBag (0.02s)
ok github.com/MongooseMoo/barn/server 1.020s
```

The new raw-store test additionally proves that closed lifetime does not update
an existing bag and cannot recreate a forgotten bag, inspecting actual entry
absence. The admitted builtin fixture verifies normal zero and no input wake,
held-command drain or forced-input call on skipped mutation. Recording and
benchmark connection fakes explicitly expose nil/never-cancelled Done channels;
the cancelled fixture overrides Done with its controlled closed channel. Raw
store/benchmark-only setter calls explicitly pass nil. All concrete ConnectionID
implementations were audited and adapted; compilation needed no later fake fix.

```text
--- PASS: TestConnectionOptionLifetimeOrdersMutationAndForget (0.01s)
--- PASS: TestClosedOptionMutationSkipsWakeAndHeldDrain (0.02s)
ok github.com/MongooseMoo/barn/builtins 1.161s
ok github.com/MongooseMoo/barn/server 1.553s
ok github.com/MongooseMoo/barn/engine 1.425s
```

These exact named selections, including scalar/allocation/session, seven real
server identity/flush cases, and the engine blocked-output fixture, are saved
with full commands and observed output in the lifetime-builtins-green,
lifetime-server-green and lifetime-engine-green text files. Full behavioral RED
is in stale-setter-red.txt. Only explicit selections from edited test files ran.
No broader tests, race detector, conformance suite, CI or isolation verdict is
claimed. The focused compilation/test window was released to parent.

Original benchmark evidence and residual costs are retained. No performance
capture was selected or run for this lifetime correction; the earlier exact
source fingerprints describe the measured pre-lifetime revision, not this final
source. Hot option-read source is unchanged by this cold lifetime correction,
but the exact final binary was not benchmarked; no performance-neutral claim
is earned. Normal server shutdown already cancels sockets before Session.Close;
arbitrary embedding-host post-close concurrency remains outside this fix.
Parent owns required PR CI/final review and any publication. No commit, push or
publication was performed by this worker.

## Parent fixture refinement and publication evidence

Parent replaced the manually held Connection.mu fixture with the existing real
TCPTransport/shutdownPeer blocked SendNotification producer. PR 646 separates
the metadata/output mutexes, so testing physical Write entry keeps the fixture
meaning stable across default integration. Cleanup closes the transport and
joins both workers; the test asserts the option result while output remains
blocked, then joins the writer with its closed error.

Observed explicit parent test output excerpt:

```text
go test ./server -run '^TestConnectionOptionReadDoesNotWaitForOutputMutex$' -count=1 -v
--- PASS: TestConnectionOptionReadDoesNotWaitForOutputMutex (0.01s)
ok github.com/MongooseMoo/barn/server 1.473s
```

The production lifetime delta has independent source analysis with no new
actionable finding. Final PR checks/verifier remain outstanding. Lossless
system-layering-option-identity-evidence.json contains twelve raw captures with
their original decoded UTF-8 SHA256s checked, including all failed and revised
timing evidence. Those timings precede the cold lifetime guard; exact final
binary performance remains unmeasured and no neutrality claim is made.
