# Output byte accounting scout

## Confirmed producers

The managed stock oracle XML contains five tests with no failures; the Barn
comparison contains admission success and four count-versus-byte failures.
builtinBufferedOutputLength adds one per task-local pending notification in
builtins/connection.go. Connection.BufferedOutputLength returns the slice length
in server/connection.go. Production notification creation is confined to
builtinNotify and Connection.Buffer. builtinNotify already decodes binary output
and captures newline framing before deferring the notification.

## Proposed direct correction

Put the payload-plus-optional-CRLF byte calculation on task.PendingNotification,
its existing shared owner. Keep Connection.BufferedOutputLength constant time
by maintaining its cached byte total at both append sites and after each
successful flush entry. A failed entry and its suffix retain their bytes.
Sum the same method for matching task-local pending notification effects.
Do not change queue acceptance, physical draining or effect ordering here.

## State

Scout completed and stopped. All outputBuffer mutations are confined to
Connection construction, its two enqueue sites and flushLocked. Tests retain
backing slices but do not mutate queue contents. Existing tests require byte
expectations: failed flush leaves 13 bytes for three/four plus CRLF; first and
second queue entries total 15 bytes. The retry/clearing suite's three entries
contain 13, 16 and 13 bytes; surviving suffix totals are 42, 29, 13 or zero.

TCP appends CRLF when newline is requested. WebSocket sends text-frame payloads
without adding CRLF, and rejects invalid UTF-8 before enqueue. The shared
calculation represents MOO logical output bytes, excluding WebSocket framing,
TLS overhead and physical socket progress. Preserve WebSocket framing and its
validation-before-enqueue invariant. No local tests were run by the scout.
