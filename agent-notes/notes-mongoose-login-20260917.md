# Mongoose login checkpoint 2026-09-17
- Fresh fixture SHA256: 3B1F76F930D6E31EB6BA667C4EF15BB2A7E0B7EC93688C2B5870BE8278EECD4A; disposable runs under .tmp/mongoose-login-20260917.
- WSL Debian stock Toast accepts guest into Georgie's Guesthouse; Barn initially waits over 20 seconds behind #1007:s_run, then can log in.
- Built-in pprof shows InputProcessor.run waiting in ProcessReadyTasks; CPU profile also identifies waif finalization overhead. No performance fix yet.
- Added deterministic regression TestInputDispatchContinuesDuringBackgroundTask; fails with "background task blocked foreground input dispatch".
- Next: keep scheduler passes serial while allowing input dispatch, run regression/race checks, then repeat fresh guest login. Temporary Barn PID 109888; WSL Toast PID 1165001.

## Completed input-dispatch repair
- InputProcessor now joins one background pass asynchronously; input still dispatches in order and Stop waits for the pass. Source and regression remain uncommitted.
- `go test -race ./server -count=1`: `ok github.com/MongooseMoo/barn/server 2.046s`; managed scheduling/login selection with capability admission: `6 passed, 12972 deselected in 12.91s`.
- Fresh guest probe: send at 503 ms, welcome at 506 ms, room at 528 ms; look sent at 2003 ms, room at 2010 ms. Evidence: .tmp/mongoose-login-20260917/fixed-client.jsonl.
- Barn PID 65516 remains on 127.0.0.1:11485 using the disposable snapshot and numeric promotion. Startup pprof/expvar/trace supplied the diagnosis; no debug-tool change was necessary.
- Remaining observed issues: suspended-frame restoration warnings and sound-hook `Confunc failed: This database is not open.`; login and look work despite these. Background workload performance is not repaired by this dispatch change.

## Account-login comparison follow-up
- User rejected guest-only equivalence; requested fresh Mongoose-branch Toast, latest remote database, account login, and sound SQLite. Refetched snapshot still hashes 3B1F76F9...8EECD4A; replaced root mongoose.db.new with backup retained under .tmp/mongoose-account-20260917.
- Backed up remote sound.sqlite via SQLite backup API (integrity_check=ok), downloaded and installed under files/sqlite and disposable run directories; prior local sound database preserved.
- Fresh WSL branch worktree /root/src/toaststunt-mongoose-login-20260917 at 72e3c7f builds after configuring from build directory; PROMOTE_NUMBERS defined. Toast binary SHA256 a748a93644fe2b973cc85dfed902454a0a56c8b368afdc8104161ec76154d098.
- Toast shows banner and account prompts but rejects supplied q credentials; asked whether password postdates checkpoint. Barn delays welcome until background exclusive commit gate releases, potentially expiring login deadline; pprof proves EscalationLock wait at task_runtime.go:177.
- Additional live delta: Barn emits literal ~FF telnet strings where Toast emits raw bytes. Next: reduce notification behavior and fix, then address startup lock-duration/waif GC overhead. Barn PID 60860, WSL Toast session 8486; evidence under .tmp/mongoose-account-20260917.

## Reusable workflow and notification reduction
- Added scripts/fetch-mongoose.ps1, scripts/mongoose-login.ps1, docs/mongoose-login.md after user requested scripts for repetitive operations. PowerShell parser accepts both; live Toast probe exercised login script (use idle timeout longer than initial banner wait).
- New generic conformance file in sibling repository: builtins/notify_output.yaml. Barn red output is ['part','ial','raw~0A','bad~GG','done'] instead of ['partial','raw','invalid','done']; admission passes.
- Oracle managed full collection was slow in WSL p9 filesystem reads; interrupted collection and reran with --moo-suite-path=builtins/notify_output.yaml, admission included (session 60483, /tmp/barn-mongoose-notify-oracle-focused.log).
- No production changes yet in this follow-up. Intended notification fix must capture decoded bytes/newline mode when notify runs, preserve it through deferred effects, and support raw transport writes.
- Account password/checkpoint question remains pending; Toast rejected supplied password on downloaded snapshot. Startup gate contention and pending-waif linear scans remain unresolved.

## Notification fix verified; startup performance next
- Managed notification regression passed fresh Mongoose Toast (2 passed in 8.06s), failed original Barn, then passed fixed Barn (2 passed, 12977 deselected in 11.79s).
- notify now decodes binary escapes and captures newline mode before deferred publication; Connection.SendNotification and Transport.WriteOutput carry framing. Server/builtins race tests passed (2.556s/6.876s).
- Scripted fresh Barn now emits raw telnet bytes and banner; read-based guest login still stalled during background work in the last probe. PID 30704; transcript prefix barn-20260917-125153-910.
- Added pending-waif bookkeeping benchmark and identity/drain test. Baseline 4096 waifs visited twice: 172347800 ns/op. Next optimize quadratic global pending-root scans without changing finalization identities/order.
- Generic test remains in sibling conformance repo; preserve its other dirty files. Scripts/docs and production notification changes remain uncommitted.

## Background zero-delay suspension divergence
- Pending-root identity maps reduce the 4096-waif benchmark to 2286700 ns/op; VM race tests pass after pool fixture update.
- New generic background suspension reduction passes fresh Mongoose Toast but fails Barn: parent resumes ahead of its already-ready child.
- Runtime explicitly resumes forked suspend(0) inline up to 16 times while retaining the exclusive commit gate; remove this path and use existing scheduler handoff.
- Added scripts/test-mongoose-deltas.ps1 for repeatable managed comparisons. Account password remains rejected by Toast; user checkpoint clarification pending.
- Current Barn PID 125244; all follow-up changes uncommitted. Next rebuild, compare reductions, then probe real login and SQLite behavior.

## Suspension handoff repaired; elapsed execution time investigation
- Deleted inline forked suspend(0) resumes; managed Barn reductions now 3 passed in 2.57s, matching Toast. Updated old engine test that required inline execution.
- Fresh Barn PID 47160 shows welcome and username but guest read waits on commit gate; profile shows background VM execution, not a deadlock.
- Task.SecondsUsed is never advanced and VM has no elapsed-time enforcement; reduced seconds_left elapsed test now correctly uses steps/run rather than expression code.
- Barn elapsed reduction fails expected 1 actual 0; Toast session 41122 pending. Do not implement until oracle confirms.
- Scripts now documented in docs/mongoose-login.md. Account snapshot/password clarification still pending; notes remain uncommitted.

## Elapsed budget and live snapshot findings
- Toast elapsed-seconds reduction passes; deadline-backed SecondsLeft now passes Barn too. Engine/task/builtins Go tests pass. Live Barn PID 118412 reaches guest room after about 40 seconds.
- Both engines reject q credentials and both report sound database not open. Offline inspection proves #0:server_started skips SQL initialization unless listening on $network.port (7777).
- Database sets bg_ticks=20000000 and bg_seconds=30; scheduler s_run collects and reinserts due recurring jobs without yielding inside its collection loop.
- fetch-mongoose.ps1 executed successfully, latest database hash unchanged 3B1F76...CD4A; sound snapshot integrity ok. Fresh Toast at 17890 PID relay 74096 also reproduces guest sound failure.
- Extended notify reduction proves noflush must preserve queued order; Connection fixed, rebuild pending. New hard-seconds reduction fails Barn (overran), Toast session 60474 pending.

## Hard seconds limits and commit-gate waiting
- Implemented sampled VM seconds checks, preserved timeout prefix writes and timeout hook resource. All five focused managed cases pass Barn; matching Toast cases passed individually.
- Commit-gate waiting previously expired the login's context deadline; now VM owns the adjustable seconds deadline and irreversible-boundary waits are excluded and logged as gate_wait.
- Added committed-scope Go regressions for wire framing, deadline exclusion, and timeout prefix writes. Full Go suite passed; race engine/vm/task/server/builtins passed (engine 215.672s).
- Current Barn PID 70164 on 7777 (latest code), transcript barn-20260917-132112-248 reaches guest room but startup still waits about 30 seconds and SQL initialization follows login, causing first sound error.
- Toast suspended activations carry source/native PCs but no Barn bytecode; loader rejects them. This is an additional known compatibility limit, not repaired. Account password still rejected by oracle; no follow-up commits yet.

## Committed delivery and remaining limits
- Committed f1b5aa2 on master: notification framing, real suspension handoff, seconds budgets/gate wait exclusion, pending-root indexes, regressions, scripts, and docs. Notes/assets remain uncommitted; no push.
- Moved only our three new generic YAML reductions out of sibling checkout into tests/mongoose-conformance; both managed engines pass all five cases including admission using the checked-in fixtures.
- Full Go suite passed again after commit; focused new Go tests also passed under race. Warm guest probe barn-20260917-132451-345 reached welcome at 5504ms and room at 5510ms without sound error.
- Remaining responsiveness is intermittent, not only cold startup: q probe barn-20260917-132648-483 hit its short idle timeout before banner during background load. Earlier completed q probes on both engines said Access Denied.
- Barn PID 70164 remains on 7777; fresh reference Toast PID 1171932 remains at WSL 17890. Stopped older Toast PID 1168029. Full equivalence and Toast suspended-task import remain unfinished.

## Refreshed checkpoint authenticates q
- Refetched using fetch-mongoose.ps1; database SHA256 C44051F14006778F1C7185F8914FD08CA2CA629119C3BB5EC0D4C9DB3C0E039D, replacing the earlier 3B1F76 snapshot; sound backup integrity ok.
- Restarted fresh Mongoose Toast at 17890 and Barn PID 75880 on 7777. Both now accept q with the supplied password and enter Trojanovich plaza; glorp's Lab.
- First Barn probe barn-20260917-150351-719 reached banner at 21688ms, with first-login SQL error and a later service-startup logger scatter error. Authentication is fixed by this checkpoint; those runtime issues remain.
- Follow-up Barn login and look succeeded without the sound error after initialization; overlapping account probe exposed the expected character chooser. No source edits or new commit in this refresh turn.

## Workload branch and nested-budget reduction
- Created fix/mongoose-workload at f1b5aa2, retaining both previously authorized commits; no unstaged tracked work existed at branch creation. User authorizes continued fixes/commits here.
- Compared q look/inventory/who and wizard eval on both live engines. Barn wizard eval reports negative tick use (-18 versus Toast 5); plain nested eval reduction passes both.
- Found builtin-invoked initialize gets a fresh foreground VM budget and its work is not charged to caller. Generic initializer test fails Barn [0,0], original sharing test passed Toast; extended oracle session 56542 pending.
- Profile workload.cpu shows 18.42% cumulative frame finalization candidate scans and full-core background activity. Logger scatter error is a MOO call passing too many arguments, not yet a server fix.
- Current Barn PID 75880 on 7777 and Toast 17890 use new C44051 checkpoint. Next repair callback budget accounting, verify managed reductions and live behavior, then commit on branch.

## Callback accounting and workload sorting fixes
- Branch fix/mongoose-workload now contains 9ce6c8b (callback budget sharing; Toast reduction passed) and ba0b541 (replace quadratic transaction sorting with slices.Sort). Managed Barn deltas: 7 passed in 7.11s; store race tests passed.
- Sorting benchmark at 4096 IDs drops from 3708607 to 147201 ns/op; 8 IDs increases from 27.30 to 35.42 ns/op, zero allocations. Latest profile still shows 122.80% CPU and frame finalization scans at 15.80% cumulative.
- Negative wizard eval ticks remain: MOO subtracts fixed Toast overhead, while Barn opcode charging differs. Suspended Toast frame imports still warn about empty bytecode; not repaired.
- Barn rebuilt as PID 12168 on 7777. Exploration probe barn-20260917-154352-661 pending; monitoring script/docs uncommitted. Finish live evidence and commit tooling, preserving this note uncommitted.

## Completed workload pass
- Committed capture script, account milestones, timeout validation, and documentation as a6524f9 on fix/mongoose-workload; tracked working tree is clean. Existing untracked work and these notes remain untouched/uncommitted.
- Probe 154352 had no received output because BannerWait 30s exceeded reader Timeout 20s, not a proven server stall. New validation rejects that combination; subsequent normal-timing probe displayed welcome, authenticated q, and returned room/inventory/who.
- Subsequent movement probe also reported Confunc failed: Invalid argument through #1584:bf_exec, #1300:exec, #43:time_offset and #1749:confunc. Exact oracle comparison for that failure remains to do; do not claim full login-hook parity.
- Current Barn PID 12168 remains on 7777. Remaining work includes hook failure, background responsiveness, negative wizard tick display and suspended Toast task import; no claim of complete Mongoose equivalence.

## Time-offset dependency reproduced on Toast
- WSL Mongoose Toast also fails #43:time_offset("America/Denver") before installing executables/tz. Remote helper is a 317-byte Bash script using TZ="$1" date +%z; copied into disposable Toast cwd, then the same verb returns -21600.
- Implementing native Windows cmd/mongoose_tz with embedded IANA tzdata; startup builds tz.exe and Toast startup installs downloaded original helper. Existing exec suites test generic executables, not this workload dependency.
- Added explicit -VerifyTimeOffset smoke mode. First candidate used #43:time_offset("UTC"), which both engines reject through Mongoose's valid_timezone whitelist; revised smoke uses direct exec UTC plus known-valid America/Denver through the actual MOO wrapper.
- Helper unit tests pass. Source changes remain uncommitted on fix/mongoose-workload; next verify revised smoke on both engines, audit managed exec tests, and measure responsiveness hot paths.

## Time smoke success and conformance fixture audit
- Revised time smoke passes both engines with {"mongoose-time-offset", {0, "+0000", ""}, -21600}; Barn took 55.4863 seconds under background load. Toast confirms Mongoose bg_ticks=20000000, bg_seconds=30.
- Packaged exec audit: WSL Toast 21 passed; Windows Barn 18 passed, 3 failed suspension assertions. Windows sleep.bat uses bare ping under exec's restricted PATH; fixture diagnosis in progress.
- Sibling conformance checkout has unrelated dirty files. Created clean worktree .tmp/conformance-mongoose-workload on its own fix/mongoose-workload branch at 01f0b48. Older plugin skips 17 exec rows; newer installed plugin runs them. No fixture edit yet.
- Inlining scalar MayHoldFinalizable tag checks halves plain-frame benchmark from 155.1 to 77.12ns; types/vm tests passed, race/engine/server session51684 pending. Need finish fixture proof, rebuild live Barn, document evidence, and commit scoped changes.

## Time dependency repaired and commits verified
- Barn commits df45cc9 (inline scalar finalization check) and 1705e8d (native timezone helper, automatic installation, smoke verification, documented packaged/worktree conformance runner). Types/vm race and engine/server tests passed; rebuilt Barn managed deltas 7 passed in 7.12s.
- Conformance worktree commit 76aed8b fixes sleep.bat to use absolute SystemRoot ping and adds exec_fixture_delay.yaml. Red [1,0] versus expected [0,1]; repaired regression passes both engines. Existing 17 exec assertions plus regression/admission pass 19/19 on both when old plugin feature gates are removed only in a disposable probe, preserved under run/exec-probe-without-legacy-feature-gates.yaml.
- Fresh Barn PID 50684 on 7777: time smoke passes. Warm login also reaches MESSAGE OF THE DAY and full welcome/pages after the time hook; no time-offset error. Measured call still takes 17.8942s, so latency remains unresolved. Cold login can still precede SQL initialization.
- Latest CPU sample finalizable frame scan falls from 23.55% to 12.10% cumulative, not a controlled end-to-end speedup claim. Background MOO budget is 20M ticks/30s; atomic long slices and serial batch joining remain latency leads. Both changed tracked working trees clean; notes remain uncommitted.

## Task profiling and immutable-path verb caching
- Added pprof moo.task/moo.verb labels and debug slow-slice logs, plus capture script task report. Live sample attributes 83.79% CPU to #1007:s_run; first background slice takes 19.49s while login waits 15.37s.
- Implemented narrowly scoped transaction-local verb caching after unrelated writes: every visited object must remain unowned and pointer-identical; property/global memo gates unchanged. Read-set regression first failed cache size 0, then passed; benchmark 223.9-240.8ns to 125.5-126.6ns.
- Store tests and store race passed. Engine race session95659 pending. Unchanged packaged verb/dispatch suites pass WSL Toast 102/102; Barn session48592 still running.
- Rebuilt live Barn PID96112 on7777, probe barn-20260917-164709-258 pending. These source/tooling changes remain uncommitted on fix/mongoose-workload. Next finish tests/live evidence and commit; no conformance files changed this turn.

## Compact temporary locals and final verification
- Compiler hidden temporaries forced tiny verbs to allocate 256 locals; compaction now preserves named slots and instruction widths while remapping temporary operands. Scatter fixture uses 7 slots; lifecycle benchmark 927.9ns to 70.91ns.
- Full go test ./... passed; engine/store/compiler/bytecode/VM race tests passed. Unchanged managed verb suites: both 102 passed; loop/scatter/fork selection: both 49 passed, 9 skipped. Local seven-test managed session passed.
- Fresh Barn PID115872 on7777: first background slice 14.439s; time-offset smoke passed. Cold login still reports SQL confunc failure before initialization; remaining responsiveness is unresolved.
- No conformance files changed this turn. Next inspect final diffs and commit profiling, safe verb memoization, and local compaction separately on fix/mongoose-workload; leave these notes uncommitted.

## Claude Fable consultation 2026-09-18
- Prior fixes committed f776c7f, 591aeb8, 1219a0e. User requests external-agent Claude Fable recommendations followed by implementation; read external-agents skill, launched claude --model fable --print with prompt prompts/fable-mongoose-next-20260918.md.
- Claude session26659 still running; reports/fable-mongoose-next-20260918.md contains interim findings only. Exclusive commit gate blocks foreground behind resumed background slices; s_run runs scheduled jobs inline, needs per-job attribution.
- Existing Barn PID115872 still runs port7777. New profile workload-20260918-140632-905 attributes95.27%CPU to s_run; live account probe barn-20260918-140839-680 logs in and looks successfully, time helper valid but80.486s real.
- No implementation changes this turn yet. Wait for Fable final report, then implement bounded recommended fixes with tests/oracle evidence; do not weaken conformance assertions.

## Fable recommendation and Toast-first reproduction
- Fable session26659 finished exit0; final report recommends claim-at-dispatch, ticks/gate telemetry, remeasure duty and only then per-job optimization. No critical defect found in prior three commits. SQL startup ordering is Toast-equivalent; do not reorder it.
- Added external conformance fork/ready_sibling.yaml with test-owned driver, visibility and kill assertions. Managed WSL Toast passed2 including admission; unchanged Barn failed expected[1,1] actual[0,1]. Scheduler unit regression also failed: task1 claimed before dispatch.
- Implemented Ready selection without claims and runtime per-batch claim immediately before dispatch, dropping killed/already-claimed tasks; fixed stale ProcessReadyTasks comment. Engine tests session52349 pending.
- Next add engine kill regression and slice tick/gate metrics, rebuild and verify oracle regression, then fresh real workload measurement and scripted clean-login timing. Nothing committed this turn yet.

## Dispatch fix validation and remaining load
- Claim-at-dispatch regression now passes Barn2/2 and Toast2/2; engine sibling-kill test, full go test ./..., engine race188s, local7 managed tests pass. Broader fork selection3passed9skipped (existing gates). Real Mongoose1p+16p gate passed, zero failed/uncaught; output real-workload-test.txt.
- Fresh run .tmp/mongoose-fable-20260918 PID124696 port7777 since14:25:28. Reset boundaries collapse5.15/s to0; median slow slice~0.56s, but duty remains~99%. CPU profile14:31 still94.65%root s_run. Added ticks/gate_held logs and root-label docs.
- Added clean-login retry/timing switch (not runtime-tested yet), summarize-mongoose-workload.ps1 and measure-mongoose-cycles.ps1. All implementation/scripts uncommitted; Fable prompt/report untracked.
- Per-job probe via eval first3objects: Barn ticks71/201/15 versus WSL Toast382/1036/70, wall~0.5ms/~0.5ms/0 vs0.05ms/0.18ms/0.008ms. Different live world states/ports; requires investigation, not permission to blindly optimize. Next finish10min sample, test clean-login on sequential7777 engines, commit scoped work; remaining high CPU must be stated.

## Committed dispatch fix and clean-login timing
- Barn573c2c7 commits claim-at-dispatch and safe tick/gate telemetry; conformance4b6ac54 commits ready-sibling regression. Full suite/race and real1p+16p workload gates passed; subsequent telemetry lease-order focused race passed11.604s.
- Saved dispatch-fix-summary.json:595.45s sample, reset boundaries0/s, median0.722s, duty98.5%, max15.90s. Reset storm gone; high background CPU remains. No conditional protected-builtin optimization attempted on inconclusive different-state tick comparison.
- Sequential7777 clean-login script probes: Toast firstattempt20.512s fromlaunch; Barn firstattempt SQL confuncerror, secondattempt clean at137.041s (includes firstattempt receive-wait/retry policy). Toast process1194791 stopped after verifiedcwd. Final rebuilt BarnPID88896 remains7777.
- Scripts/docs and Fable reports still uncommitted; next append timing limits to follow-up report, stage only owned files, commit, final concise report. Notes and unrelated untracked files stay uncommitted.

## Final delivery 2026-09-18
- Tooling/docs/Fable reports committed6f4caae; dispatch573c2c7 and external conformance4b6ac54 remain on fix/mongoose-workload. Tracked working tree clean; notes, consultation prompt, and unrelated untracked work preserved.
- Fable recommendations1-4/6 implemented and measured. Conditional5 investigated with per-job script; inconclusive different-state tick counts mean no protected-builtin optimization. High background load, startup latency, same-state job comparison and full ten-minute Toast idle baseline remain open.
- Final BarnPID88896 running7777, clean-login retry completes at137.041s fromlaunch versusToast20.512s. Details and limitations in reports/mongoose-fable-followup-20260918.md.

## External-completion latency work
- User authorizes tracing helper completion to task resume, fixing verified ordering and making login prompt-driven. Same fix/mongoose-workload branch; initial tracked tree clean.
- New external conformance server/exec_resume_order.yaml: helper completes during busy fork, which then forks new task. Toast managed2passed order[busy,exec,new]; Barn red[busy,new,exec]. Root cause candidate: Ready appends catalog resumptions after heap forks regardless ready time. No scheduler fix yet.
- Added external command elapsed log, Task exec-ready timestamp and runtime queue_wait/ready_to_vm telemetry (uncommitted). Added promptBuffer/loginCommands helper/tests and CLI login-prompts/stop-after-login options; client race session45001 pending. Need wire script defaults and capture baseline before scheduler fix.
- Next unit red for merged readiness, fix ordering, managed oracle rerun, live before/after metrics, docs and commits. Final prior live Barn stillPID88896 until explicit replacement.

## External-ready ordering fix and measured baseline
- Expanded oracle to two exec cases: fork created after completion, and before completion while helper runs. Both Toast pass (3 incl admission). Barn baseline failed busy,new,exec; fixed binary managed4passed including ready-sibling. Unit red demonstrated merely merging by timestamp insufficient for second case.
- Scheduler now stable-sorts merged heap/catalog readiness; Task SchedulingSnapshot gives completed external tasks time-zero priority and arrival-order tie key, matching Toast resume_task. Existing selected batch remains unchanged. Engine/task/client normal tests passed; engine race84125 pending.
- Prompt client responds username/password/MOTD, immediate PROXY; main integration and task ready-timestamp tests race passed. UntilCleanLogin immediate failures initially caused tight retry201; stopped run, added1s RetryDelay and restarted single baseline. Do not use flooded run as evidence.
- Valid baseline .tmp/mongoose-resume-20260918 bootprefix151226, warmprobe session1459: tz17.4ms followed13.702s queue_wait, another141ms followed2.836s; login hook136ms followed10.788s. Baseline has telemetry but old scheduler. Next save log, restart fixed build, measure same probe, doc/commit. No new commits yet.

## Narrow completion ordering and retry confound
- Replaced broad ready sorting with stable promotion of completed external tasks only; ordinary resumptions retain prior order. New unit caught the broad-sort side effect.
- Final managed Barn selection5passed; WSL Toast4passed. Full Go suite and engine/task race suite passed (engine194.405s).
- Repeated cold login attempts failed to reach clean login within300s; first background s_run remains busy. This measurement differs from valid single-connection baseline and is not evidence of an improvement.
- Next restart with a single cold connection, compare warm helper/resume telemetry, investigate any persistent startup regression, then document and commit owned changes. No current-turn commits yet.

## Measured narrow fix and oracle commit
- Single startup then warm login succeeded: prefix153001, password-to-MOTD30541ms vs baseline34172ms; UTC/Denver smoke13.6764s vs16.6998s. Single observations, not a latency distribution.
- Fixed helper queue waits183.5933/3551.9166/10078.7095ms; substantial existing-batch delay remains. Saved final-runtime.jsonl; BarnPID26992 running7777.
- Oracle cases now require helper exit0. Final Toast4passed13.15s, Barn5passed8.86s; external conformance committed3ba427e with lint/contract hook passing.
- Client now labels failed hooks separately and retains receive timeout after failed hooks to avoid reconnect flooding; race client3.625s passed. Next report and Barn commit; notes stay uncommitted.

## Source-first scheduling audit and research
- Prior changes committed14dada9; source audit now reads WSL Mongoose Toast72e3c7f tasks.cc and server.cc directly. Toast admits by current programmer, FIFO within queue, chooses least whole-wall-second usage, runs one task then revisits IO/completions; input precedes background within a queue.
- Barn runtime.go drains all selected batches before recollecting; server input lanes already run separately, so new socket input differs from an exec-resumed welcome hook. Existing engine fairness test explicitly enshrines whole-pass drain.
- User additionally requests SOTA research/plugin. Read research skill; its fork workflow delegated only reports/research-barn-scheduling.md to /root/scheduler_research. No runtime changes this turn yet.
- Live owner probe session47897 pending in .tmp/mongoose-resume-20260918/scheduler-owners.txt. Next resolve scheduler identity/transaction constraints, document source findings and research-backed implementation path without claiming every measured delay proven.

## Fable consultation includes commit-gate scheduling
- Source/research reports committed ef5fcb0. User requests gaming out scheduler with commit gate via Claude Fable; read external-agents skill, launched exact model fable session e01c6c16-a9e2-4085-9279-9c5ac05c7673 (exec54242).
- Added uncommitted scripts/consult-fable.ps1, docs/fable-consultations.md, bounded prompt prompts/fable-scheduler-gate-20260918.md. Fable owns only reports/fable-scheduler-gate-20260918.md; read-only runtime review, no implementation/tests/server changes.
- Local source confirms exclusive gate before all nonretryable resumed slices, shared gate on ordinary writing commits, read-only Commit fast-return, exclusive checkpoint; first-run forks can retry internally but remain solo scheduler batches. Gate wait occurs after physical execution lease acquisition.
- Fable report now exists but process not yet terminal; next read completed recommendation, challenge concrete deadlock/parking/cancellation scenarios in same session, synthesize and commit review artifacts. No runtime policy edits.

## Fable first recommendation and second-round challenge
- First round completed success53turns728916ms; report reports/fable-scheduler-gate-20260918.md finalized (~730lines). Wrapper initially failed on verbose JSON event array; fixed to select terminal result object. CLI itself succeeded.
- Fable proposes exclusive arbiter + pre-dispatch grant for nonretryable slices + pre-effect try-or-abort/requeue gated retry. It flags solo production scheduler batches, direct lanes, synchronous curl under gate, checkpoint and GC coupling.
- Same-session followup exec84630 pending, prompt prompts/fable-scheduler-gate-followup-20260918.md; requested ~1500word concrete first slice, challenges shared commits, holder progress, GC roots, cancellation and continuation representation.
- Parent verified Toast curl uses background_thread and Barn client.Do synchronous. Added gate correction to source audit. Prior sampled exec queue_wait==ready_to_vm remains evidence of ready-queue delay; reject unqualified residual-hold/age hard bounds in first report. Next read final followup, synthesize honest design, commit only owned artifacts.
