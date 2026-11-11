# fix/notify-noflush-queue-bound

## Goal
Make Barn pass `builtins/notify_semantics.yaml::no_flush_notify_returns_zero_when_unread_output_queue_is_full`
(Toast `[1, 1, 1]`, Barn `[1, 1, 0]`) with the smallest production change.

## State
- Test (moo-conformance-tests main): sets max_queued_output 2048, notifies a
  65536-char line with no_flush, then repeats up to 2000 times; expects the
  first to return 1 and some later one to return 0. The third element
  (`zero_at > 1`) was 0 on Barn because Barn's notify always returned 1.
- Toast `enqueue_output` (network.cc, read via WSL): if output_length != 0 and
  output_length + length > max_queued_output, push_output, then
  to_flush = output_length + length - max; to_flush > 0 && !flush_ok -> return 0
  and the line is not queued. bf_notify returns that (server.cc:2949).
- Fix: `builtins/network.go` builtinNotify returns 0 without queuing when
  no_flush, queued output for the target is nonzero, and the line's byte length
  (with CRLF) exceeds max_queued_output. `builtins/connection.go`
  queuedOutputLength is the shared count (connection buffer + this task's
  deferred notifications to the target), also used by buffered_output_length.
- Go tests added in builtins/network_test.go; they fail without the fix and pass
  with it, also under -race. go vet ./..., go build ./... pass.
- PR opened; waiting on CI.

## Decisions and why
- Residual after Toast's push is taken as 0: Barn writes accepted output
  synchronously (WriteOutput blocks rather than queuing unread bytes), and
  Barn's deferred/no_flush buffers would all be pushed by Toast's push_output
  to a reading peer. So the line fits only when it is within max_queued_output.
  Counting the full pending bytes instead would return 0 in Barn where Toast
  pushes and returns 1 (several small no_flush lines summing past the max).
- Deferred-until-commit delivery is unchanged for every line that is accepted.

## Surprises
- None so far.

## Next step
Watch CI on the PR; merge on green.
