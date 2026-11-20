# fix/yielded-command-resume

## Goal
Over Barn's real TCP server, bench_players.py's first `look` on Mongoose never
completed after PR #585 (yin() on a root-VM task yields for 0 s once the slice
has run >= 10000 ticks). Find the cause, reproduce it with a deterministic Go
test on the real InputProcessor path, fix it Toast-faithfully, mutation-check.

## State
Fixed and committed on this branch (no push, no PR). The lead runs
TestMongooseRealWorkload and bench_players.py.

## Cause
The yielded command was not the problem: a yielded command task resumes
through the real dispatch loop (TestYieldedCommandResumesWithoutFurtherInput
passes on master too). The hang was the login before it.

- server/input_processor.go HandleConnection fixed each line's identity
  (`player`, the negative connection id before login) BEFORE its blocking
  read, and processInput routed on that value (input.Player < 0 ->
  processPreLogin).
- With #585, Mongoose's do_login_command slice yields at yin() before its
  switch_player. processInput for the password line returns at the yield; the
  read loop captures -connID and blocks; the resumed login slice switches the
  connection to #2 and prints "Welcome!"; the bench's next line,
  `OUTPUTPREFIX ===BP-PREFIX===`, is then processed as pre-login input: a new
  do_login_command answers "I don't understand that. Valid commands at this
  point are ..." and loginPlayer logs "already logged in via switch_player"
  (the hang run's server log has exactly that line, 8 ms after the switch;
  the pre-#585 run has none).
- The bench's completed_command only sends its OOB probe after it has seen the
  prefix tag, so it waited for `look` until its 30 s timeout. Barn was not
  hung: the goroutine dump shows the $physics rearm sweep (forked at boot,
  bg_seconds 30, forks do not split at yin) running, and the probe was never
  sent.
- Toast: tasks.cc run_ready_tasks (~line 1766) chooses do_command_task or
  do_login_task from tq->player when the queued line runs; switch_player sets
  tq->player (tasks.cc:2978).

## Fix
- server/connection.go: Connection.inputPlayer() (player once logged in, else
  -ID). HandleConnection uses it in place of its GetPlayer/IsLoggedIn pair.
- server/input_processor.go processInput: for a line whose ConnID names a live
  connection, refresh input.Player from that connection before routing.
- Test: server/yielded_command_test.go TestLineAfterYieldedLoginIsACommand
  (do_login_command runs 12000 ticks, yin(), switch_player; the next line
  `ping` must reach #2's command verb). Fails on master 3/3 ("no pong", line
  routed with player=-2, same "already logged in via switch_player" warning).
- Mutation: refresh removed -> the new test fails; a scratch in-process
  Mongoose diag (real login conversation through HandleConnection, bench's
  prefix/suffix + probe) fails 2/2 with OUTPUTPREFIX routed as player=-2.
  Restored -> both pass.

## Identity issue seen on the pre-#585 build (NOT the same defect, not fixed)
After a switch_player login whose do_login_command task suspended (Mongoose
reads username/password with read()), the connection's ConnectionTime stays
zero: ConnectionManager.SwitchPlayer (server/connection_manager.go:931-981)
never sets it, and loginPlayer, which does (input_login.go:254/274), is
skipped because input_processor.go:663 returns when the login task's
SuspendGeneration != 0. ConnectedPlayers (connection_manager.go:604) omits a
player whose connection has a zero ConnectionTime unless show_all. Scratch
diag after this fix: logged in as #2, connected_players() from #36 returns {}.
In the #585 hang run the misrouted OUTPUTPREFIX accidentally ran loginPlayer,
which set ConnectionTime, masking this. With this fix the bench's identity
check ("roster players not connected after login") is expected to fail on
Mongoose until that is fixed. Toast's connected_players()/switch_player
behaviour for this case is not yet read.

## Decisions and why
- Refresh at processing time, not only by moving the capture after the read:
  a line read while the login is still yielded and processed after it switches
  would still be stale. Processing time is when Toast decides.
- Kept TestYieldedCommandResumesWithoutFurtherInput (passes on master): it is
  the only test that drives a #585 yield through the real InputProcessor loop
  with the bench's completion probe.

## Surprises
- The brief said no goroutine ran MOO during the hang; the dump shows the
  rearm sweep running.
- A server-package harness without server.go's SetTaskOutputFlusher never
  sends a command task's OUTPUTSUFFIX.
- Scratch diag sources (not committed) are in the session scratchpad
  yield-diag/ folder.

## ConnectionTime at switch_player (lead, same branch)
Toast's player_switched (server.cc) sets new_h->connection_time =
time(nullptr), and bf_connected_players lists a handle only when show_all or
connection_time != 0. Barn's SwitchPlayer never set ConnectionTime, so a
Mongoose login (do_login_command read()s, suspends, so loginPlayer is
skipped) left the player out of connected_players(). Fixed in SwitchPlayer;
TestSwitchPlayerConnectsForConnectedPlayers, mutation caught.

## Next step
Rerun bench_players.py (Barn 1p+16p, then Toast vs Barn on an idle machine);
PR; CI; merge.
