# Connection output lock analyst

## Scope and state

Read-only source analysis of the uncommitted fix/connection-output-lock change
against origin/master. No tests or compilation: the binary-input worker owns the
measurement window. No implementation edits or publication.

## Findings

No actionable correctness defect found in this bounded source delta. This is
not a merge verdict. Performance evidence and required CI remain outstanding.

The production diff adds outputMu, substitutes it in exactly SendNotification,
Buffer, Flush and BufferedOutputLength, and regroups loggedIn with existing
booleans. No output policy, error branch, notification content or task ownership
code changed.

### Output ownership and error accounting

Repository-wide Go search found outputBuffer/outputBytes only in connection.go
and sequential connection_buffer_test.go assertions. Initialization occurs
before publication. All later production accesses now require outputMu:
connection.go:114-143,147-162,321-324. flushLocked has only the two existing
callers, both holding outputMu. Successful entries still decrement byte count
and clear backing slots; failure still retains the failed entry and later
suffix. Existing partial-failure/retry coverage remains unchanged in
connection_buffer_test.go:39-112. There is no new race through an old metadata
lock guarding one overlooked queue access.

### Metadata, transports and lock order

The four output methods access no metadata protected by mu. They take outputMu
and then the existing transport output lock. No production path acquires both
connection locks. Prefix/suffix and programming fields retain mu in
input_processor.go:810-817,845-860,886-892; player/login/hook/listener getters
and setters retain mu in connection.go. Moving loggedIn changes layout only.

Close/closeForShutdown remain independent of both connection locks
(connection.go:196-208). TCPTransport.Close interrupts through its closeOnce
without taking its write mutex (transport.go:275-278), so replacement can reach
Close while the writer remains blocked. Ordinary WebSocket Close still has its
existing handshake behavior (websocket_transport.go:94-101); this change does
not establish a new WebSocket interruption bound.

Send and sendCommandResponse still bypass connection output serialization
(connection.go:83-95). This is preexisting; transport locks still serialize their
writes against notification writes. The change must not claim stronger global
FIFO than the existing notification queue supplies. Validation and diagnostic
Committed placement remain before outputMu, and per-write Output recording
remains in the same branches. Command diagnostics use their own lock and have
no connection metadata callback on the output recording path.

### Actual producers and regression quality

SwitchPlayer holds manager.mu while invoking setDisconnectHook and SetPlayer,
then unlocks before closing the displaced connection
(connection_manager.go:932-967). processDisconnect also holds manager.mu while
reading connection metadata (input_processor.go:539-548). The HandleConnection
defer waits for disconnect completion before closing transport (:112-120).
These production relationships support the bounded owner correction; they do
not prove concurrent runtime commits or eliminate a gate held during replay.

The new replacement test uses actual TCPTransport.WriteOutput with the existing
controlled net.Conn. It waits for physical Write entry while the notification
already owns the relevant connection output lock, then invokes real manager
replacement. The original source therefore cannot pass merely because the
replacement raced ahead of write entry. The test checks mapping publication,
physical close and the writer's closed error. The second test checks metadata
progress while the write remains unfinished, then closes and joins it.
Both register independent close plus worker joins before launching goroutines,
and buffered result channels prevent cleanup from waiting on an unread result.

### Unresolved evidence

The sequential samples show Notification +1.84% while the unchanged Send
control moves +9.38%. They cannot settle attribution or performance neutrality.
The planned interleaved fixed-binary comparison is still required. Constructor
and structure size evidence is coder-provided; this analyst performed no runtime
measurement. No tests, race detector, benchmark or build was run here because
the binary-input worker owns that window. Required PR CI remains unverified by
this source review.

## Remaining

Parent resolves interleaved performance evidence and full PR checks. Verifier
must inspect the final exact diff and evidence before any merge verdict. No
production implementation changed during this analysis.

## Delta review: skip empty notification queue flush

Reviewed only the new SendNotification guard at connection.go:121-126. It checks
len(outputBuffer) while outputMu is already held and invokes the unchanged
flushLocked only for a nonempty queue. No actionable semantic defect found.

For an empty queue, flushLocked previously executed no transport write, byte
adjustment or diagnostic call; it only assigned outputBuffer[:0] and returned
nil (now :149-165). Reslicing an already-zero-length nil or nonnil slice to zero
does not change its pointer, length or capacity. Skipping that assignment has no
observable queue/accounting/error effect. The direct current notification write
and its diagnostic still run. For a nonempty queue, the same helper and error
branch run before the current notification, preserving existing FIFO and
failed-suffix handling. NoFlush still returns before this guard, unchanged.

No runtime measurements or tests performed for this delta. Parent reports the
prior controlled notification comparison remained +1.67%; the new guard's
measurements are pending. This source equivalence does not establish performance
neutrality or clear required PR CI.
