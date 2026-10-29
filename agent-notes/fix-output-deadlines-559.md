# Issue 559: active output deadlines

## Goal

Implement issue #559 on branch `fix/output-deadlines-559`: a peer that stops reading must not indefinitely block active TCP/WebSocket output, including boot and recycle banners. Preserve framing, serialization, disconnect hooks and shutdown joins. Root owns all publication and merge actions.

## State

The initial implementation is published as PR #563. Root verified a review finding: an ordinary TCP output timeout leaves an open socket with a permanently failed buffered writer. That repair is complete locally in `C:/Users/Q/code/barn-output-deadlines-559`, and its authored regression selection passes. Root owns repair publication and review of the actual delta. Broader results belong to PR CI, not a local CI rerun.

## Decisions and why

Smallest production change: one shared 30-second active-write allowance, applied as a TCP write deadline around WriteString/Flush and a WebSocket context deadline around Write. TCP clears its write deadline after success and closes its socket after terminal output failure. This allowance is an operation policy, not a read or idle-session deadline; 30 seconds tolerates ordinary brief stalls while providing a finite upper bound.

Return the original write/deadline error through existing callers and close the TCP socket on terminal output failure. My prior choice to keep transport closure only in the administrative caller was wrong: ordinary notification callers return/set E_INVARG without closing, while bufio.Writer retains its failure permanently. A failed output stream cannot remain a live authenticated connection. Do not retry/reset it or replay partial output. Successful writes clear their temporary deadline; failure cleanup must not replace the original I/O error with a close/deadline error. Do not add a queue, worker, flag, retry layer or new shared package.

Set boot/recycle's already-selected disconnect hook before the banner. Both terminal TCP output failure and the pinned WebSocket dependency's context expiration can close a socket while Send is still returning. Selecting the hook afterward can let reader cleanup observe the wrong hook. Root explicitly approved this in-scope move. The existing transport owner retires its failed socket; ordinary connection/input cleanup observes that closure.

## Surprises

One read-only module search put --glob after the -- path separator and returned errors for those literal paths. The corrected search puts every option before --. No source or test conclusion depends on the rejected portion of that probe.

## Next step

Return the scoped repair commit to root for publication and review of the actual delta. Keep prior unchanged WebSocket/hook coverage and existing required CI; do not claim the repair is published or merged from this local result.

## PR draft

Bound active TCP and WebSocket writes to 30 seconds so a non-reading peer cannot indefinitely hold normal output or boot/recycle before closure. Retire TCP sockets on terminal output failure and preserve the original error. Preserve existing selected disconnect hooks when write failure closes the socket. Closes #559.

Validation: `go test ./server -run '^(TestTCPOutputDeadline|TestWebSocketOutputDeadline)' -count=1 -timeout 30s -v` passes after the TCP retirement repair. The repair's existing PR CI remains pending root publication.

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
