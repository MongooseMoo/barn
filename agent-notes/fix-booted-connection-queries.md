# Fix: connection builtins answer for a booted connection

## Goal

`builtins/network_matrix.yaml::open_network_connection_loopback_records_outbound_state_and_boots_cleanly`
failed once on barn PR 1034 (job 136805645968): `expected value 1, but got 0` at its last step,
`try connection_info({conn}); return 0; except (E_INVARG) return 1; endtry`, after `boot_player({conn})`
and a poll that saw `{conn}` leave `connected_players(1)`. Make every connection builtin Toast gates on
`!h || h->disconnect_me` treat a booted connection as gone at once, and add Toast-first conformance coverage.

## State

Done and committed locally: Barn branch `fix/booted-connection-queries`
(worktree `C:/Users/Q/code/barn-booted-connection-queries`) and suite branch
`test/booted-connection-queries` (worktree `C:/Users/Q/code/moo-conformance-tests-booted-connection-queries`).
Not pushed. The suite test is Toast-green, Barn-red before the fix and Barn-green after it.

## Findings

### Toast (upstream `src/server.cc`)

- `boot_player` (1824-1830) sets `h->disconnect_me = true` at the call.
- `bf_connected_players` (2795, 2802) skips such handles. `bf_connection_info` (3066-3070),
  `bf_connected_seconds` (2820), `bf_idle_seconds` (2838), `bf_connection_name` (2859),
  `bf_name_lookup` (2938-2943, permission check first, then `!h || h->disconnect_me` -> E_INVARG),
  `bf_set_connection_option` (3010-3015, permission first) and `bf_connection_options` (3037) all treat
  `!h || h->disconnect_me` as no connection.
- `bf_notify` (2950-2985): permission check, then `if (h && !h->disconnect_me) send ... else { if
  (in_emergency_mode) emergency_notify(...); r = 1; }`. A booted target gets nothing and the result is 1,
  before any binary-mode decoding or queue-limit check.
- `bf_buffered_output_length` (3285) checks only `!h`; not changed.

### Barn before the fix

- `server/connection_manager.go` `BootPlayer` closes the connection (cancels its ctx) but leaves it in
  `playerConns` until the disconnect event is processed asynchronously. `ConnectedPlayers` already skips a
  connection whose ctx is done; `GetConnection`, reached through `builtins/network.go` `resolveConnection`,
  still returns it. This window is what the CI failure hit under load.
- Inside a transaction `builtinBootPlayer` does not boot at all: it queues `PendingEffectBootPlayer`, applied
  by `FlushPendingEffects` at commit. So in the booting task itself, `connected_players` still listed the
  connection and every connection builtin still answered (the suite test showed `connected_players` too:
  `[1, 1, 1, 0, 0, ...]`).
- `notify` used `resolveConnection`: for a booted target it went on to binary decoding (E_INVARG on an
  invalid binary line, where Toast returns 1) and the no-flush queue check (0, where Toast returns 1).
- `connection_name_lookup` called `cm.ConnectionNameLookup`, which uses `GetConnection` and does a reverse
  DNS lookup for a booted connection. It also had no permission check (Toast: E_PERM for a non-wizard on
  another player, before the lookup).
- The failure log prints no port numbers; nothing in it involves port selection (suite PR 249).

### Fix

- `builtins/network.go` `resolveLiveConnection`: nil when this task has a pending boot of the target
  (`pendingBoot`, `builtins/pending_effects.go`) or the connection's `Done()` is closed; otherwise
  `resolveConnection`. Used by connection_info, connected_seconds, idle_seconds, connection_name,
  connection_name_lookup, set_connection_option, notify (`builtins/network.go`) and connection_options
  (`builtins/connection.go`).
- `connected_players` skips players with a pending boot in this task.
- `connection_name_lookup` checks permission first, as Toast does.

### Tests

- `builtins/booted_connection_test.go`:
  - `TestConnectionQueriesTreatBootedConnectionAsGone`: closed-lifetime connection still returned by the
    manager; each of the seven query/option builtins must raise E_INVARG.
  - `TestNotifyToBootedConnectionReturnsOne`: binary mode, invalid line `~zz`, returns 1.
  - `TestConnectionNameLookupChecksPermissionFirst`: E_PERM.
  - `TestBootingTaskSeesItsPendingBoot`: snapshot transaction, `boot_player` queues one effect, then
    `connection_info` is E_INVARG and `connected_players` is `{}`.
  - All four fail with the production files stashed (or the `pendingBoot` checks disabled) and pass with them.
- `builtins/connection_option_lookup_test.go` `TestClosedOptionMutationSkipsWakeAndHeldDrain`: the setter on
  a closed connection was expected to return a normal 0; Toast's `bf_set_connection_option` says E_INVARG,
  so only the result assertion changed. The protections it exists for (no option bag recreated, no input
  wake, held commands not drained) are unchanged and still asserted.
- `server/connection_option_identity_test.go` `TestAdmittedOptionSetterCannotRecreateDisconnectedBag`
  protects a setter that saw a live connection and then lost it before mutating (a Barn-only window; Toast
  is single-threaded). It paused inside `GetConnection`, which is now before the liveness check, so the
  setter saw the disconnect and returned E_INVARG. The pause moved to the connection's second `Done()` call
  (the first is the liveness check, the second the lifetime handed to the mutation), so the disconnect
  again lands after admission and before mutation; its assertions (normal 0, no bag recreated) are unchanged.
- Suite: `builtins/network_matrix.yaml` `boot_player_makes_connection_gone_within_the_booting_task`. One
  task: checks the connection is listed and answers, calls `boot_player`, then with no wait checks
  `connected_players(1)`, connection_info, connected_seconds, idle_seconds, connection_name,
  connection_name_lookup, connection_options and set_connection_option all treat it as gone. Outbound
  loopback connection, so it skips when `option.OUTBOUND_NETWORK` is off. (On Toast an outbound connection
  has `connection_time == 0`, so connected_seconds raises E_INVARG even before the boot; that element is
  weak and kept for completeness.)

## Runs

- `go test ./builtins ./server -count=1`: ok (47.3 s, 18.8 s). `gofmt -l builtins server`: clean.
  `go vet ./builtins ./server`: clean. `golangci-lint run ./builtins/... ./server/...`: 0 issues.
- Managed harness, `network_matrix.yaml` selector, `-k "capability_admission or (network_matrix)"`:
  - Stock WSL Toast (`/root/src/toaststunt/build-release/moo {db} {db}.new -p {port}`,
    `--target-profile-manifest=profiles/toast/stock-wsl-testdb.json`): 5 passed, 2 skipped (load 9.80).
  - Barn without the production change: 1 failed (`[1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0]`), 4 passed,
    2 skipped (load 7.34).
  - Barn with the fix: 5 passed, 2 skipped (load 8.75).
  - The 2 skips are the disabled-outbound rows (`skip_if: option.OUTBOUND_NETWORK`).
- Full managed suite on fixed Barn (suite branch `test/booted-connection-queries`, canonical CI command,
  no `-k`): 13769 passed, 325 skipped, 0 failed, 23 min 14 s (load 9.85 at start, 29.01 at end).

## Decisions and why

- A task-local view of pending boots (`pendingBoot`) rather than booting immediately: boot_player is
  deferred to commit so a conflict retry cannot boot twice; `pendingServerOptions` already gives the loading
  task its own view the same way.
- `pendingBoot` compares the object number as given: `boot_player(#2)` then `connection_info(#-5)` for the
  same connection is not matched. Not shown by any test or caller; recorded only.

## Surprises

- The deferred boot meant the same-task suite test failed on Barn even with the closed-connection fix; the
  first fix alone did not cover the Toast behaviour.

## Next step

Lead publishes both branches. Suite PR first (Toast-green, Barn-red until the Barn PR lands).

## Barn PR body

Title: Treat a booted connection as gone in connection builtins

Toast's boot_player sets `disconnect_me` at the call, and connected_players, connection_info,
connected_seconds, idle_seconds, connection_name, connection_name_lookup, set_connection_option,
connection_options and notify all check it (server.cc). In Barn the boot closed the connection but left it
registered until its disconnect event was processed, and inside a transaction the boot was only queued for
commit. So connection_info and the rest kept answering for a booted connection: once after the boot (the
network_matrix flake on PR 1034) and always within the booting task.

- `resolveLiveConnection` treats a closed connection, or one this task has booted, as gone; the builtins
  above use it, and connected_players skips this task's pending boots.
- notify to a booted connection returns 1 without decoding or queue checks, as Toast does.
- connection_name_lookup checks permission before the lookup (Toast bf_name_lookup).
- Two existing tests follow Toast's E_INVARG for a closed connection; their bag, wake and held-input
  protections are unchanged.

Tests: `builtins/booted_connection_test.go`; suite test
`builtins/network_matrix.yaml::boot_player_makes_connection_gone_within_the_booting_task` (suite PR),
Toast-green, Barn-red before this change, Barn-green after.

## Suite PR body

Title: Cover connection builtins within the task that boots the connection

Adds `boot_player_makes_connection_gone_within_the_booting_task` to `builtins/network_matrix.yaml`. In one
task it boots an outbound loopback connection and checks, with no wait, that connected_players(1),
connection_info, connected_seconds, idle_seconds, connection_name, connection_name_lookup,
connection_options and set_connection_option all treat it as gone. Toast's boot_player sets
`disconnect_me` at the call and each of these builtins checks it (server.cc).

Managed runs: stock WSL Toast passes; Barn fails before the matching Barn PR
(`[1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0]`) and passes with it.
