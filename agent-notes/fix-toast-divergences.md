# fix/toast-divergences

## Goal

Make Barn pass the Toast-verified conformance tests from moo-conformance-tests
branch `jepsen/toast-divergences` (PR 190), the prerequisite for Jepsen PR E
(`agent-notes/jepsen-expansion-plan.md`). Oracle record and verdicts:
`reports/jepsen-toast-divergence-oracle.md` in the main checkout.

## State

Fixed, each with a Go test that fails without the fix:

- `delete_property` checks object write permission before the lookup (Toast
  `property.cc:246-251`); `clear_property` reports E_PROPNF before permissions
  (`property.cc:276-279`); `set_connection_option` checks permission before the
  connection (`server.cc:2982-2988`).
- An indefinitely suspended task is written with start time `-1`, the value
  Toast's `%d` prints for its 64-bit `INTNUM_MAX` (`tasks.cc:1304-1307,1918`), so it
  resumes with its wake value after a restart (`db/format/writer_vm.go`).
- `Server.Start` starts task dispatch after start scripts, checkpointed-connection
  hooks and `#0:server_started` (`server/server.go`).
- `open_network_connection` honors `$server_options.outbound_connect_timeout`,
  raises `{E_INVARG, "Connection failure", strerror}`, and SIGALRM fails the
  earliest pending connect when it is due before the checkpoint timer instead of
  checkpointing (`server/connection_manager.go`, `server/signals_unix.go`).

Open: `notify(..., 1)` returning 0 on a full unread queue (Toast
`network.cc:1347-1354`) needs a design decision, because Barn defers task output to
commit and has no bounded per-connection queue.

## Decisions and why

- The indefinite-suspend change is in the writer only, so a Barn checkpoint and a
  Toast checkpoint hold the same bytes and Barn's existing loader handles both.
- Dispatch starts after `server_started`, not behind a new gate: the runtime already
  runs server hooks synchronously on the caller's goroutine, and the input queue is
  buffered, so nothing before it needs the dispatcher.
- `connection_options()` keeps E_INVARG before E_PERM: Toast's
  `bf_connection_options` checks the handle first (`server.cc:3003-3011`).

## Surprises

- Barn's slog file sink escapes `"` inside a `server_log` message, so a log check
  containing quotes never matches on Barn; the conformance test logs without quotes.
- After a successful dial Barn waits for its own listener to accept the socket
  (`OpenNetworkConnection` matching loop), so a real outbound connection to another
  host still fails. Not reached by these tests.

## Next step

Resolve the notify design question, merge PR 190, rerun this PR's CI against the
new conformance main, merge.
