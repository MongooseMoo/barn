# Issue 559: active output deadlines

## Goal

Implement issue #559 on branch `fix/output-deadlines-559`: a peer that stops reading must not indefinitely block active TCP/WebSocket output, including boot and recycle banners. Preserve framing, serialization, disconnect hooks and shutdown joins. Root owns all publication and merge actions.

## State

Implementation and focused regression run are committed locally. Work is isolated in `C:/Users/Q/code/barn-output-deadlines-559`. The change touches the two transports, boot/recycle hook ordering and the new output regression file. Existing PR CI supplies the broader checks; no CI steps were run locally as a publication gate. The post-commit checkpoint remains uncommitted as requested by the checkpoint hook.

## Decisions and why

Smallest production change: one shared 30-second active-write allowance, applied as a TCP write deadline around WriteString/Flush and a WebSocket context deadline around Write. TCP clears its write deadline after each operation. This allowance is an operation policy, not a read or idle-session deadline; 30 seconds tolerates ordinary brief stalls while providing a finite upper bound. Initial size: roughly 12 added production lines and two moved hook-selection lines, with focused regressions separate.

Return the original write/deadline error through existing callers. Do not retry/reset the TCP bufio.Writer after partial output: its sticky error already prevents replaying partly delivered bytes. Do not add a queue, worker, flag, retry layer or new shared package.

Set boot/recycle's already-selected disconnect hook before the banner. The pinned coder/websocket implementation closes its socket when a write context expires (`conn.go:171-177`), so selecting the hook after Send can allow reader cleanup to observe the wrong hook. Root explicitly approved this in-scope move. Keep transport closure in its existing owner.

## Surprises

One read-only module search put --glob after the -- path separator and returned errors for those literal paths. The corrected search puts every option before --. No source or test conclusion depends on the rejected portion of that probe.

## Next step

Return the committed branch and raw verification evidence to root for publication and PR CI. Root owns all external publication and merge actions; it can include this post-commit checkpoint when preparing publication.

## PR draft

Bound active TCP and WebSocket writes to 30 seconds so a non-reading peer cannot indefinitely hold normal output or boot/recycle before closure. Preserve existing selected disconnect hooks when a WebSocket write deadline closes the socket. Closes #559.

Validation: `go test ./server -run '^(TestTCPOutputDeadline|TestWebSocketOutputDeadline)' -count=1 -timeout 30s -v` passes. Existing PR CI remains pending publication.

## Verification evidence

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
