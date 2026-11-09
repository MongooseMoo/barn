# Output byte accounting implementation

## Goal

Correct buffered_output_length to count MOO output bytes for both task-local
pending notifications and the connection queue, retaining constant-time queued
length queries and existing acceptance, framing and draining behavior.

## State

Scoped source and regression inspection completed. Managed oracle evidence
supplied by the parent establishes four count-versus-byte failures on Barn and
passes on stock Toast. Focused edited regressions failed against the original
implementation as expected and pass after the scoped implementation.

RED command (inside verified Debian WSL, from this worktree):

```text
go test ./server ./builtins -run "^(TestFlushResumesAfterPartialWriteFailure|TestBufferedOutputLengthTracksQueuedBytes|TestFlushClearsDeliveredNotificationsAndPreservesRetry|TestBufferedOutputLengthIncludesPendingNotifications)$" -count=1
```

Raw RED excerpts:

```text
connection_buffer_test.go:78: buffered bytes after Flush = 3, want 42
connection_buffer_test.go:78: buffered bytes after Flush = 2, want 29
connection_buffer_test.go:78: buffered bytes after Flush = 1, want 13
connection_test.go:46: buffered bytes after failed Flush() = 2, want 13
connection_test.go:123: BufferedOutputLength before Flush = 2, want 15
network_test.go:273: buffered_output_length after notify 1 = 1, error E_NONE, want 5
network_test.go:273: buffered_output_length after notify 1 = 1, error E_NONE, want 3
network_test.go:273: buffered_output_length after notify 1 = 1, error E_NONE, want 4
network_test.go:273: buffered_output_length after notify 1 = 1, error E_NONE, want 2
network_test.go:273: buffered_output_length after notify 1 = 1, error E_NONE, want 0
network_test.go:273: buffered_output_length after notify 1 = 1, error E_NONE, want 2
FAIL github.com/MongooseMoo/barn/server 0.007s
FAIL github.com/MongooseMoo/barn/builtins 0.095s
```

## Decisions and why

Use one PendingNotification byte-length method for the decoded payload and
captured newline choice. Maintain the connection total at existing queue
mutation sites, subtracting only successfully delivered entries. No additional
queue, transport policy or effect lifecycle is introduced.

The implementation uses task.PendingNotification.ByteLength for MOO payload and
captured CRLF bytes, excluding transport frame/TLS overhead. Connection updates
outputBytes under its existing mutex at both append sites and only after each
successful delivery. Failed entries and suffixes retain their bytes; delivered
slots are still cleared. WebSocket validation remains before enqueue. The
builtin sums the same method for matching task-local pending notifications.

Focused GREEN output from the identical command:

```text
ok github.com/MongooseMoo/barn/server 0.006s
ok github.com/MongooseMoo/barn/builtins 0.100s
diff_check_exit=0
```

The changed regressions cover exact queued text bytes, partial failure/retry
suffixes, reference clearing, pending accumulation, CRLF/no-newline, UTF-8 byte
length, empty framed/unframed output, decoded binary payloads, connection-option
changes after capture and exclusion of another recipient's notifications.
No full local test suite, managed oracle, broad build or benchmark was run.
No speedup or measured non-regression is claimed.

## Next step

Return the scoped diff to the parent for managed post-fix oracle, analyst review
and PR CI. No commit, push, PR, review or merge was performed by this worker.
