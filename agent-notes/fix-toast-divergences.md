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
- A suspended task saved with a negative start time (older 64-bit Toast wrote an
  indefinite suspension's `INTNUM_MAX` through `%d` as `-1`) loads as indefinite,
  as the CI Toast oracle MongooseMoo/toaststunt `eaaa1972` reads it
  (`engine/task_load.go`). Barn already kept its own indefinite suspensions
  suspended across a restart, which the conformance test pins.
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

- An earlier version of this branch wrote `-1` to make indefinite suspensions resume
  after restart, matching the WSL stock binary (`aecc51e9`). The conformance CI oracle
  (`eaaa1972`) fixed that `%d` bug and keeps the task suspended, so the writer change
  was dropped and only the reader compatibility kept.
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
