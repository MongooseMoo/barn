# TCP MAX_LINE_BYTES

## Goal
Close a TCP connection whose pending line or telnet command reaches Toast's
MAX_LINE_BYTES (5242880), as Toast does (network.cc:395, 476). Before this, an
unauthenticated client could grow `lineBuf` or `tCommand` in
`TCPTransport.ReadInput` without bound. Found while planning fuzzing work
(agent-notes/fuzzing-expansion.md on master's checkout).

## State
Fix in server/transport.go. Conformance test: moo-conformance-tests PR 166,
`server/max_line_bytes.yaml`. Stock WSL Toast: pass. Barn master: both cases
fail (no log line). This branch: both pass (`make conformance-k
K=max_line_bytes` against the PR 166 worktree).

## Decisions and why
- Check after each byte is read, before it is used: Toast checks only when
  input arrives (pull_input before a read; process_telnet_byte per byte).
  Toast reads 1024-byte chunks, so a line can exceed the limit by under a chunk
  if its terminator arrives in the same read; Barn checks per byte. Not
  reproduced; recorded as a known granularity difference.
- Log message text carries Toast's stable fragments ("closed for exceeding
  MAX_LINE_BYTES!", "exceeded MAX_LINE_BYTES in telnet command!"); remote
  address and size are typed attrs. The returned error takes the existing
  read-error close path (server/input_processor.go).
- WebSocket input already has a 1 MiB read limit; unchanged.

## Next step
Merge this PR before moo-conformance-tests PR 166, since Barn CI runs the
conformance repo's main.
