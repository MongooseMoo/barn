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
cause remains unverified. The second queue run passed (CI 42333068808, Elle
42333068721); #614 merged at 2026-11-07T03:25:55Z with two parents. Integrated
the remote default branch into this branch and built cmd/barn successfully.
Started the actual-player TCP harness under .tmp/player-lag-baseline with the
fixture/settings below. Bootstrap completed, then control login failed with
WinError 10054 before any cohort. Preserved run.json, checkpoint and logs.
The login task waited 1.623622s for the commit gate, but the reset cause is being
verified separately. Actual dump code $telnet:send_raw enables binary, sends raw
Telnet bytes, then disables binary before login. That wakes the concurrent TCP
reader by expiring its deadline; HandleConnection currently interprets any
unauthenticated timeout as login expiry and returns. Added the exact sequence
as a candidate managed regression. Stock Toast passed it with canonical admission
(2 passed), evidence .tmp/prelogin-toast.xml and prelogin-toast-admission.json.
The deterministic TestPreloginBinaryOutputDoesNotExpireLogin then failed with
"binary output expired the prelogin reader: write pipe: i/o timeout". After the
deadline comparison fix, its race run passed (ok server 1.259s). Built the fixed
cmd/barn binary at .tmp/player-lag-barn-fixed.exe for the next actual-player run.
No new flag/queue/lock. This fixes the capture/login defect introduced by
read-ahead; the original reported lag still needs player-cohort evidence.

The second run .tmp/player-lag-reader-fixed logged in successfully, then failed
Control's bracketed look. The log classifies all three bracket/look inputs as
login at 20:37:36.424 before the switch to player #2 in that same millisecond.
Actual $account:login_as_player prints Welcome before end_login_music (which can
suspend), record_connection and switch_player. The harness wrongly treats that
early text as player readiness. Reuse its existing prefix/suffix/look exchange
as bounded synchronization: retry it within the existing 30s deadline until an
actual prefix followed by suffix arrives. Do not add a login protocol or alter
MOO login behavior. Added a regression with one dropped exchange, failing on the
old arm_bracket. The repair passes scripts/test_bench_players.py. HTTP captures
attempted after this failed run ended were refused; those are not profiles.
The third capture .tmp/player-lag-cohorts now has 16 accounts from 524 real player
candidates and has verified its 1-player cohort connected with no extras. A 90s
whole-process CPU capture has started against its private debug endpoint; its
window includes settle/background work and must not be attributed entirely to
one command or cohort. New PR publication is pending Q's reply to the question
tool; no new PR has been posted.

The cohort run finished exit 0. All command failures and traceback counts are
zero. Results (terminal times include the OOB probe): 1 player 491 commands,
p99 210.37ms/max 322.05ms; 4 players 842 commands, p99 373.41ms/max 2744.16ms;
16 players 1063 commands, p99 2399.94ms/max 5712.16ms. Direct command records
distinguish early print from later completion: home command 1196 first printed
at 37.8787ms and finished at 2735.6964ms; setup home 3009 first printed at
57.2422ms and finished at 10411.9618ms through 1067 slices. No SQL slot/work
delay was measured for 3009; its resume_queue maximum was 3.5113ms. Original
first-output delay 911 (@who) was 284.398ms with VM attempts totaling only
9.0726ms; the native slice took 272.9007ms, so the missing interval is outside
the VM attempt and requires lock/lease evidence, not a VM CPU claim.

Retained 90s Go CPU and MOO profiles plus vars-before/stacks-before. CPU total
206.03s over 90s: NextTaskWake 38.58s cumulative (18.73%), PendingReadier 33.78s
(16.40%); task-labelled CPU #9655:arm 60.55s, #0:server_started 47.89s. Labels
are root tasks. MOO profile shows background physics arm/rearm, declaration and
validation. These are whole-process windows, not command-exclusive CPU.

The 30s HTTP trace request was interrupted by normal benchmark teardown (curl
56); retain the truncated go.trace. Go trace's viewer explicitly reports an
incomplete final generation and recovers the complete prefix (0 B reported
lost), so this is bounded-prefix evidence, not a complete 30s capture. Raw
trace-derived blocking/scheduler profiles are retained. Blocking prefix:
acquireTaskExecution 65.71s aggregate across goroutines, of which VMStartMu.Lock
at runtime.go:274 accounts for 60.53s and s.mu.Lock for 5.18s. Investigate actual
VMStartMu holders and admission/root/GC ownership before changing lock order or
scheduling. Source shows the runtime mutex held while releaseTaskExecution waits
for scheduler.ReleaseExecution, and while queueTask/handOffSuspended wait to
enqueue. NextWake/Ready scan the same scheduler under its mutex. That lets a
long scheduler scan hold up the runtime mutex indirectly; acquireTaskExecution
then retains VMStartMu waiting for it and all other VM starts line up there.

Bounded repair to test: remove scheduler waits from the runtime-mutex critical
sections. Registration and saved-VM publication stay under runtime.mu; enqueue
after unlocking while the physical execution lease still prevents early resume.
For final physical release, acquire scheduler.mu before runtime.mu and change
the lease map/active flag while holding both, preserving the atomic handoff.
No change to readiness order, admission weights, MVCC validation or VM budget.
Add a deterministic regression that holds a scheduler scan, begins a background
handoff, and proves an unrelated VM can still acquire its execution lease.
Run the new regression with race detection, then repeat the same real-player
capture and obtain CI/Elle verdicts before any merge. This is a hypothesis-backed
lock-convoy repair; a latency improvement is not established yet.
TestSchedulerHandoffLeavesUnrelatedVMStartsAvailable first failed for both
releaseTaskExecution and queueTask with "blocked an unrelated VM start". After
moving the scheduler waits outside runtime.mu, the new file's race run passed
(ok engine 1.339s). Final release now takes scheduler.mu before runtime.mu and
clears the physical-active flag under both; queue registration and saved-VM
publication unlock runtime.mu before heap operations. Task-row observers are
called after store publication locks are released. Repeat the same cohort
capture before accepting the performance hypothesis; CI/Elle are still pending.

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

The paired handoff-repair run finished exit 0 in .tmp/player-lag-handoff-fixed.
Terminal p99/max: 1 player 246.01/563.91ms; 4 players 264.76/443.67ms;
16 players 744.69/1282.68ms. Throughput 17.8/54.6/105.5 completed/s. Baseline
16-player p99/max was 2399.94/5712.16ms and throughput 34.3/s. This is one
instrumented pair, not a clean speedup proof: both worlds retain the same
background #9655:arm E_MAXREC and player #71 home E_NACC application errors
(35 baseline, 82 candidate); the native handler hides the latter from the TCP
traceback counter. One-player first-output p99/max worsened from 33.98/284.40ms
to 42.21/487.14ms (includes setup/warmup); latency is not universally fixed.

Q asked whether the E_MAXREC also occurs on Toast. Barn's originating-task
census classifies task 1787475465 as tick_budget, not frame recursion. Ran the
same checkpoint/catalog through the WSL Mongoose Toast harness in
.tmp/player-lag-toast-maxrec: no corresponding tick-exhaustion/task error in
the fresh server log. Absence alone is insufficient. Its shutdown produced a
PANIC-named checkpoint, and moodb rejected that output with duplicate
server-equivalent map key; preserve it without loosening the parser. A second
Toast harness is active in .tmp/player-lag-toast-task-check with a longer settle
window to query queued_tasks directly before shutdown. Source moodb inspection
lists the original saved task as queued with when=109290153959993s. Confirm
actual Toast scheduling before investigating a Barn restore/time mismatch.

The live Toast probe did return the exact task as queued:
{1787475465,416145977,0,30000,#249,#9655,"arm",1,#9655,1436}.
The probe used an early return inside Control.eval, violating that helper's
last-return tagging contract; it raised ControlEvalError after retaining the
real untagged result above. Do not repeat that caller shape.

Critical fixture finding: using the existing moodb Reader.read_v17_header on
the source and private copies shows that offline Toast bootstrap changed task
1787475465's timestamp from 109290153959993 to 416145977 before BOTH engines
ran. The installed Mongoose Toast's tasks.cc read_task_queue reads st into C int
using %d. The queued count stays 1080. This makes the prior TCP measurements
observations of an altered scheduling workload, not an original-world speedup
proof. Mark all earlier throughput/tail comparisons accordingly. Do not infer a
Barn restore/time defect: task_load.go uses time.Unix and readiness checks the
absolute future deadline. The error has not been reproduced on Toast, but the
bootstrap discrepancy prevents a clean original-checkpoint parity verdict.

Read-only partial moodb inspection of #24.traceback_log (parsing valid header
and objects through its ancestors, without suppressing the later duplicate-key
error) found no retained E_MAXREC rows in either Toast output. This is bounded
history, not proof none ever occurred. The post-run task header no longer holds
the target; absence cannot distinguish normal completion, cancellation or error.

Ran the existing TestMongooseRealWorkload on the original checkpoint and catalog
with 1/4/16 players, GOGC=400, GOMAXPROCS=32, 2s warmup/8s measurement; it passed
in 55.16s. Evidence .tmp/player-lag-original-input/ has manifest, JSON logs and
Go/MOO profiles. Its code does not load persisted queued/suspended tasks, so this
run cannot decide the saved-task MaxRec question. Console diagnostics were huge;
do not claim missing per-cohort stdout as retained. It is a player-code/startup
service capture, not full server replay or socket response evidence.

Next: preserve original task scheduling in any further TCP replay. Existing
wizard credentials avoid the lossy bootstrap, but MONGOOSE_LOGIN_SCRIPT is absent.
Do not build a new bootstrap layer or accept rewritten scheduling as equivalent.
New PR publication still awaits Q's pending approval. Do not push/post or claim
CI/isolation coverage for these local fixes. Keep all captures and manifests.
