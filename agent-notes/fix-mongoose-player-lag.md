# Mongoose player lag

## Goal

Merge PR #614, use the existing Mongoose player harness to reproduce intermittent
command delays, diagnose them and fix the verified cause. No live deployment.

## State

The diagnostics PR's first queue run failed the checkpoint_finished suspend
callback conformance case with a socket timeout. Unit and Elle checks succeeded.
Preserved queue artifacts under the diagnostics worktree's
.tmp/pr614-queue-failure. The exact CI suite revision passed the focused managed
stock Toast check and Barn check plus five more Barn repetitions. The queue log
shows a checkpoint hook read before the caller's hook definition was visible;
separate PR #623 addresses that checkpoint ordering race. The queue-only timeout
cause remains unverified. Resubmitted #614 through the full queue (CI run
42333068808). This branch will fast-forward after merge, before measurements.
No new workload capture or lag-cause verdict yet.

## Decisions and why

Use scripts/bench_players.py: it already connects real players over TCP, selects
the native command mix and waits for terminal acknowledgements. The in-process
harness bypasses socket first-output instrumentation, so no adapter is needed.
Before capturing, add an optional --sound input copied and hashed alongside the
checkpoint, select debug logging when BARN_RUNTIME_DIAGNOSTICS=1, and correct the
bootstrap default to the installed Mongoose Toast path documented in
docs/mongoose-performance.md. These address observed missing fixture input,
disabled diagnostic records and an absent executable. Test these harness paths
with scripts/test_bench_players.py, then run 1/4/16 actual-player cohorts.

Retain failed cohorts. Separate socket first output from harness terminal time
(which includes the completion probe). Inspect exceptions, runtime phases and
profiles before changing runtime policy. $sh.handle is separate planned work.

## Surprises

Historical October 11 evidence has exit_code=1 and is not a valid baseline.
The old TCP harness creates an empty sound directory and starts Barn at info.
The configured Toast executable is missing; the documented September build is
installed. No evidence yet establishes the cause of current intermittent lag.

Harness preparation: scripts/test_bench_players.py first failed on the missing
--sound argument and missing debug launch option, then passed after their fixes.
The optional catalog copy records source/copy hashes; the Toast default matches
the installed documented Mongoose build. These edits are local, not published.

Selected the newest checkpoint in the Mongoose hub:
C:/Users/Q/projects/mongoose/db/mongoose.db.new.2026-11-05T0034 (278104681 bytes,
SHA256 28E8179503B246F50FFD5147AC10E43B47CCB297EF0846B171A67A921B8CF37F).
Catalog C:/Users/Q/code/barn/files/sqlite/sound.sqlite has SHA256
0FCDC8D7F909D3E57A801EC1089B69C86FB390B4DC1D85331E25E68E00639318;
read-only schema inspection found AssetFiles and 123338 rows. The harness records
and verifies both copies before the server starts. Use 1/4/16 players, GOGC=400,
GOMAXPROCS=32, default 60s settle, 2s warmup and 30s measurement per level.

## Next step

Resolve the queue conformance failure, finish the merge, verify two parents and
fast-forward this branch; then capture the real player workload.
