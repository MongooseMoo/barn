# Issue 559: active output deadlines

## Goal

Implement issue #559 on branch `fix/output-deadlines-559`: a peer that stops reading must not indefinitely block active TCP/WebSocket output, including boot and recycle banners. Preserve framing, serialization, disconnect hooks and shutdown joins. Root owns all publication and merge actions.

## State

PR #563 contains the active-write bound and TCP retirement repair. Root supplied the next CI failure: the existing shutdown test observes two physical TCP closes when explicit shutdown and write-failure cleanup both close the same socket. The existing TCP transport now owns idempotent physical closure, and the authored regression selection passes locally. Root owns repair publication and broader PR CI.

## Decisions and why

Smallest production change: one shared 30-second active-write allowance, applied as a TCP write deadline around WriteString/Flush and a WebSocket context deadline around Write. TCP clears its write deadline after success and closes its socket after terminal output failure. This allowance is an operation policy, not a read or idle-session deadline; 30 seconds tolerates ordinary brief stalls while providing a finite upper bound.

Return the original write/deadline error through existing callers and close the TCP socket on terminal output failure. My prior choice to keep transport closure only in the administrative caller was wrong: ordinary notification callers return/set E_INVARG without closing, while bufio.Writer retains its failure permanently. A failed output stream cannot remain a live authenticated connection. The subsequent direct `t.conn.Close()` failure cleanup also bypassed the shared closure owner and duplicated a shutdown close. Route both terminal failure and explicit Close through TCPTransport.Close, with a sync.Once and stored close result there. Close must not take the write mutex: it has to interrupt a writer already holding that mutex. This concrete existing race justifies roughly seven added production lines and one redirected cleanup call, not a new lifetime framework.

Do not retry/reset the buffered stream or replay partial output. Successful writes clear their temporary deadline; failure cleanup must not replace the original I/O error with a close/deadline error. Repeated Close returns the original physical close result. Preserve the existing shutdown test's one-close assertion unchanged.

Set boot/recycle's already-selected disconnect hook before the banner. Both terminal TCP output failure and the pinned WebSocket dependency's context expiration can close a socket while Send is still returning. Selecting the hook afterward can let reader cleanup observe the wrong hook. Root explicitly approved this in-scope move. The existing transport owner retires its failed socket; ordinary connection/input cleanup observes that closure.

## Surprises

One read-only module search put --glob after the -- path separator and returned errors for those literal paths. The corrected search puts every option before --. No source or test conclusion depends on the rejected portion of that probe.

## Next step

Return the idempotent-close repair commit to root for publication and existing PR CI. The unmodified shutdown test and its one-close assertion remain unchanged; its CI result after publication is not yet known here.

## PR draft

Bound active TCP and WebSocket writes to 30 seconds so a non-reading peer cannot indefinitely hold normal output or boot/recycle before closure. Retire TCP sockets on terminal output failure and preserve the original error. Preserve existing selected disconnect hooks when write failure closes the socket. Closes #559.

Validation: `go test ./server -run '^(TestTCPOutputDeadline|TestWebSocketOutputDeadline)' -count=1 -timeout 30s -v` passes after TCP retirement and idempotent-close repairs. The latest repair's existing PR CI remains pending root publication.

## Initial verification evidence

The same authored test selection failed before the production change:

```text
output did not return after the peer stopped reading
successful write deadlines = [], want allowance then clearing
WebSocket write context has no deadline
hook during boot write = "user_client_disconnected", want "user_disconnected"
hook during recycle write = "user_client_disconnected", want ""
FAIL github.com/MongooseMoo/barn/server 5.459s
```

After the change, all five top-level tests passed:

```text
--- PASS: TestTCPOutputDeadlineInterruptsPartialWriteWithoutReplay (0.10s)
--- PASS: TestTCPOutputDeadlinePreservesFramingAndClearsAfterSuccess (0.00s)
--- PASS: TestWebSocketOutputDeadlineIsBoundedAndReleased (0.00s)
--- PASS: TestTCPOutputDeadlineLetsBootAndRecycleClose (0.20s)
--- PASS: TestWebSocketOutputDeadlinePreservesAdministrativeDisconnectHook (0.00s)
PASS
ok github.com/MongooseMoo/barn/server 2.979s
```

The TCP tests use actual blocked net.Pipe writes, shorten only the fake connection's applied deadline, verify the finite production deadline was supplied and cleared, and prove timed-out partial bytes are not replayed. The WebSocket tests exercise the transport boundary with the existing fake connection: deadline supplied, timer context canceled on return, error/framing preserved, and selected hook visible while Write runs. They do not establish a live stalled-WebSocket measurement; the pinned dependency's context-triggered socket closure was inspected directly.

`gofmt -d` on the four changed Go files and `git diff --check` produced no output. No broad unit/conformance/race run or server launch was performed locally. Broader results remain unknown until the PR's existing CI runs.

## Checkpoint

- Production change uses the existing transport and connection owners, with no additional service or configuration surface.
- Baseline regressions failed for the missing deadlines and hook-selection ordering; patched regressions passed.
- Partial TCP errors remain sticky; the existing administrative caller closes the transport after Send returns.
- No publication has occurred; root will publish the local commit and read PR CI.
- Next: finish the scoped commit and hand root the branch and verification evidence.

## Post-commit checkpoint

- The scoped source, regression tests and task note are committed on `fix/output-deadlines-559`.
- Authored deadline regressions pass; broader PR CI remains pending root publication.
- Production diff is 13 insertions and 3 deletions across the three existing owners.
- This checkpoint is uncommitted as the hook requested; no external publication occurred.
- Next: root inspects the branch, includes the checkpoint if desired, publishes the PR and monitors existing CI.

## TCP retirement repair

Root verified the PR #563 finding: the deadline stopped a failed output operation, but ordinary notification callers did not close the socket, and the sticky bufio.Writer failure made its future output unusable. The earlier policy kept closure only in the administrative path; that was insufficient. The Decisions section was corrected before repairing the source.

TCP WriteOutput now closes its own socket on any terminal output/deadline failure, waking connection input cleanup. It clears the write deadline only after success. Deferred cleanup preserves the original I/O/deadline error when Close fails; no buffered writer reset, output replay or retry was introduced.

The updated actual net.Pipe timeout regression also checks reader wakeup, peer closure and no subsequent writes. The fake reports a Close failure to prove the original timeout survives cleanup. Deadline arming and clearing failures also retire their connection and preserve their own error. WebSocket/hook source and existing coverage are unchanged.

The first repair run failed because I expected SetReadDeadline to succeed after the peer was closed; net.Pipe returned `io: read/write on closed pipe`. The test now requires that closed-pipe result and then EOF instead of treating successful closure as a fixture failure.

The authored selection then passed:

```text
--- PASS: TestTCPOutputDeadlineInterruptsPartialWriteWithoutReplay (0.10s)
--- PASS: TestTCPOutputDeadlinePreservesFramingAndClearsAfterSuccess (0.00s)
--- PASS: TestTCPOutputDeadlineSetupFailureClosesConnection (0.00s)
--- PASS: TestWebSocketOutputDeadlineIsBoundedAndReleased (0.00s)
--- PASS: TestTCPOutputDeadlineLetsBootAndRecycleClose (0.20s)
--- PASS: TestWebSocketOutputDeadlinePreservesAdministrativeDisconnectHook (0.00s)
PASS
ok github.com/MongooseMoo/barn/server 3.003s
```

## Repair checkpoint

- Cause corrected in the existing transport policy and owner: terminal TCP output failure retires the socket.
- Focused regressions assert reader wakeup, peer closure, original errors and no replay; all six top-level tests pass.
- The repair changes transport.go, its existing output regression file and this living note only.
- Root owns publication, PR CI and review of the actual repair delta; no merge is claimed.
- Next: hand root the scoped local repair commit.

## Physical-close checkpoint

- Root's existing shutdown CI failure is double physical closure after terminal write cleanup races shutdown.
- The existing owner policy now requires one sync.Once-protected TCP close result, independent of the write mutex.
- Authored regressions reproduce the current defect: physical closes=3 after failure plus repeated Close, with no active writer.
- Existing shutdown assertions remain unchanged; no unmodified shutdown-suite rerun was performed locally.
- Next: redirect failure cleanup through the idempotent TCP close owner, then run the authored regression selection.

## Idempotent physical-close repair

Root supplied the existing CI failure without changing the shutdown assertion:

```text
--- FAIL: TestCloseConnectionsBoundsAllBannerWrites/TCP
connection_shutdown_test.go:150: peer 0 closes=2 active writers=0
```

The cause was the added terminal-error cleanup calling the raw socket Close while normal shutdown also called TCPTransport.Close. Both now use the existing TCPTransport close owner, protected by one sync.Once and a stored physical-close error. Close does not acquire the writer mutex, so shutdown still interrupts blocked writes. Existing WebSocket ownership is unchanged.

The actively authored partial-write test now counts physical closes and checks repeated Close returns the stored error. A new authored case reuses the existing stalled shutdown peer and proves explicit Close interrupts a write holding the write mutex, followed by error cleanup and repeated Close without another physical socket close. Before the owner change, both cases reproduced the defect:

```text
physical closes after failure and repeated Close = 3, want one
physical closes=3 active writers=0, want one close and no blocked writer
FAIL github.com/MongooseMoo/barn/server 2.645s
```

After the repair, the full authored selection passed:

```text
--- PASS: TestTCPOutputDeadlineInterruptsPartialWriteWithoutReplay (0.10s)
--- PASS: TestTCPOutputDeadlineCloseInterruptsWriterOnce (0.00s)
--- PASS: TestTCPOutputDeadlinePreservesFramingAndClearsAfterSuccess (0.00s)
--- PASS: TestTCPOutputDeadlineSetupFailureClosesConnection (0.00s)
--- PASS: TestWebSocketOutputDeadlineIsBoundedAndReleased (0.00s)
--- PASS: TestTCPOutputDeadlineLetsBootAndRecycleClose (0.20s)
--- PASS: TestWebSocketOutputDeadlinePreservesAdministrativeDisconnectHook (0.00s)
PASS
ok github.com/MongooseMoo/barn/server 2.839s
```

No unmodified shutdown-suite or broad CI step was run locally. `git diff -- server/connection_shutdown_test.go` returned no output. Root must publish and read the existing PR CI to establish that test's repaired result.

## Idempotent-close checkpoint

- TCP terminal errors and explicit Close share one physical close result in their existing owner.
- Close remains independent of the writer mutex and retains the original physical close error.
- Authored regressions reproduced duplicate closes before the fix; all seven top-level authored tests pass after it.
- Existing shutdown assertions are unchanged; broader CI remains root's publication follow-up.
- Next: commit this scoped repair and return the branch to root.
