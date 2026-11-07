# Mongoose player lag

Shared-wait review is complete. The accounting helper, task-row and object
commit routes preserve gate order/exemption/publication and count each
successful acquisition once. Standalone admitted and sweep-owned hook coverage
is corrected and has focused race regressions. The analyst report preserves
the original finding and its addressed disposition. Full PR CI and Elle
acceptance remain outstanding.

## Goal

Complete Q's approved diagnostics and lag investigation using the existing
Mongoose player harness. Preserve the original checkpoint's semantics, repair
verified defects, compare the runtime mutex repair, and inspect remaining waits.
No live deployment. PR #614 was already merged normally.

## State

The shared implementation is published in PR #635. The input
reader wakeup repair, login-bracket synchronization and runtime mutex repair
precede the current native-import work. The latter is now implemented:

- Native layout over the same parsed IR matches exact saved PCs, vector sizes,
  widths and operand heights. No source effects are replayed.
- Original input coverage is1080 queued tasks and49 suspended/interrupted tasks,
  comprising70 native frames. All restore through the unchanged VM verifier;
  every queued absolute deadline remains unchanged.
- Implicit command environment survives suspension and reaches later callees.
  A second native checkpoint proves nonempty values in all six fields.
- The same existing iteration normalizer is shared by VM and converter.
- Invalid or unsupported suspended restoration returns aggregated errors.
  Server loading stops/closes the runtime session; the harness fails before
  dispatch. A valid/bad-native-PC control was RED then GREEN.
- Range bounds run left to right before binding; private iteration survives
  user-variable mutation and maximum integer. Dead range-result storage was
  deleted to preserve local capacity. Named while stores every test value.
- Break/continue use an explicit unwind jump. One owner-scoped pending-reason
  stack covers return/error/jump, survives checkpoint/retry and retains roots.
  Older ambiguous ownership is rejected rather than guessed. Ordinary jumps
  retain their existing interpretation.

Independent analyses and fixes are recorded in reports/*-analyst.md and
reports/*-coder.md. The final scoped cleanup review found the original pending
reason loss addressed: exact probes now return73,73 and an empty list.
No full-change CI/Elle or merge verdict exists yet. Q explicitly authorized
publishing and PR work in the current turn. Publish this branch and run its
required checks; no live deployment is authorized.

Shared native restoration is now committed locally. Both candidate and private
historical control have merged the same fetched upstream code without source
conflicts. Their executable source differs only in runtime.go, task_factory.go
and task_runtime.go (the mutex repair); notes and its regression test also differ.
The scoped final verifier found no new concrete blocker, with NO-MERGE pending
remote exact-head CI and pinned Elle evidence. Its report is retained under
reports/native-restoration-verifier.md and awaits the next substantive commit.

The first sequential 30-second paired capture is retained in
.tmp/player-lag-paired-r1/{control,candidate}. Binary hashes and source revisions
are in its manifests. No agent builds/tests run during capture; editor Go
metadata probes observed beforehand remain uncontrolled background activity.
Both sequential captures completed; all cohorts settled. Control vs candidate:
one-player p99=119.11/124.32ms, max243.09/343.69ms, goodput43/43;
four-player p99=622.44/667.83ms, max985.21/1205.13ms, goodput88/80;
16-player p99=1158.98/834.42ms, max2061.52/1913.32ms, goodput183/275.
Control application failures0/10/81; candidate0/9/119. Console and profiles
preserved. This instrumented single pair has mixed results and does not prove a
universal speedup or resolution of remaining lag. The mutex repair independently
has its deterministic unrelated-start regression. Whole-capture CPU reports
PendingReadier8.14% cumulative; VM execution68.25% cumulative. These overlap.
Input admission max2.718ms and completion-to-VM max45.297ms on the candidate do
not individually explain successful command tails near2s. Original errors and
shared #2585.handle conflicts remain visible; no MOO permission/content repair.

Diagnostic capture .tmp/player-lag-warmed-trace used the same candidate
binary, original fixtures, 16 players, 60s warmup and 30s measure, plus existing
Go trace, block/mutex profiling and runtime diagnostics. It stalled after77.3s
and did not produce a terminal cohort. Log/trace stopped at00:46:18. The
five-minute timeout did not produce stacks. Delve was mistakenly attached with
no persistent stdin; it exited at EOF and the target then exited before stacks
were captured. Partial trace/log/console are retained. No application-deadlock
conclusion is justified from this failed capture.

Primary upstream issue golang/go#80982 documents Windows mutex-profiling/GC
deadlock; its backport #81145 is in Go1.27.2. This binary uses Go1.27.1.
The workload now rejects enabled mutex profiling on Windows Go1.27.0/1 before
heavy loading, and diagnostics docs specify block/trace captures on this
toolchain. The controlled retry removes mutex profiling only. A trace parser
read the partial trace but its huge empty-region output was truncated; it is
not a complete deadlock verdict. Raw binary trace remains available.

The mutex-disabled retry .tmp/player-lag-warmed-block completed104.60s after
60s warmup: submitted9494/completed9358/failed136/unsettled0, goodput307/s,
p99=756.77ms/max1450.09ms. Its terminal tool output was truncated; retained
console.partial.txt identifies that limitation. Full JSON logs, Go trace, CPU
and block profiles and measurement-only MOO profile remain intact. The guard
was exercised with enabled mutex profiling and failed before loading at0.00s.

Concrete slow task2146093051 (#261:say) spans1442.878ms across174 slices, with
175 VM attempts (one retry), VM-attempt total690.231ms/max12.652ms,1883122ticks,
slice total1242.311ms and two SQLite operations. Task-filtered trace profile
reports523.52ms synchronization blocking, of which327.53ms is StoreTxn.Commit
(320.93ms task-rows-only); scheduling latency60.59ms. These are scoped to the
actual task's regions, not process-wide deltas. CPU sampling for that task is
0.57s cumulative through executeLoop; sampling is approximate. MOO profiles do
not have a moo.task label; attempted filter returned no samples and was rejected.

Source trace: original #1:forbids line21 calls yin() per handler; original
#1:can_be_seen_by line9 calls forbids for viewer and target. #35:say_action
line27 calls players_seeing_differently, which computes per-player titles.
builtins/tasks.go deliberately makes nonforked yin() commit after10000ticks.
This explains the observed slice scale but is not yet a changed policy or proof
that removing yields improves latency. No yin/performance policy was changed.

The Go trace viewer parsed the308665975-byte trace and took minutes/~43GB to
prepare. It is currently listening at127.0.0.1:20205 in owned session17902.
Task-scoped blocking profiles downloaded by pprof are in C:/Users/Q/pprof.
Stop that owned viewer before new performance measurements. No workload runs
remain active. Distinguish terminal command completion from first socket output;
the original-checkpoint TCP bootstrap remains blocked by Toast schedule changes.

## Fixtures

Original checkpoint:
C:/Users/Q/projects/mongoose/db/mongoose.db.new.2026-11-05T0034
(278104681 bytes), SHA256
28E8179503B246F50FFD5147AC10E43B47CCB297EF0846B171A67A921B8CF37F.

Sound catalog:
C:/Users/Q/code/barn/files/sqlite/sound.sqlite, SHA256
0FCDC8D7F909D3E57A801EC1089B69C86FB390B4DC1D85331E25E68E00639318.
The harness copies it into its worktree sandbox; preserve unrelated root dirt.

Native producer fixtures and provenance are in engine/testdata. The first
nested-call checkpoint proves before=1,after=1,result=173,finished=1 after resume.
The environment producer proves inherited values reach a callee after resume.
The incompatible byte-string fixture is ASCII base64 retaining exact binary
bytes and a checked decoded hash. Native source/oracle cases are also retained
under compiler/testdata and tests/mongoose-conformance.

## Decisions and why

Toast's loader reads queued start time into C int. Offline account bootstrap
changed415 of1129 scheduling rows, including task1787475465 from109290153959993
to416145977. Both earlier TCP engine runs therefore used altered scheduling.
Their throughput/tail comparisons are NOT original-world speedup evidence.
Canonical managed Toast also ran a newly created far-future fork immediately
after reload; do not emulate that truncation in Barn or silently patch an oracle.

The shared bootstrap owner now compares task identity/deadline/function-ID
censuses through current lambdamoo-db-py's existing reader before replacement.
Mismatch retains input, rejected .boot output, census and normal failure record.
Interruptions also retain input. Three callers share this owner. Current-reader
setup is documented; CI supplies a verified immutable first-party revision via
uv in its existing benchmark test step.

The native VM header's third field is function ID, NOT readiness. Earlier
interim wording was wrong. Frame values, control tags and builtin payloads must
be retained before conversion. Native PCs cannot be reused as Barn PCs.

Native builtin continuations, nonzero native VM function IDs, pending native
finally reasons (including FIN_EXIT), and active foreign byte-string iteration
whose normalized columns differ from native bytes remain explicit unsupported
errors. Normal Barn Unicode behavior was not changed. Do not claim generic
all-state cross-engine checkpoint compatibility.

The earlier empty-bytecode rejection is fixed for this exact original input.
The originally recorded E_MAXREC was tick_budget, not call depth. It has not
been established on an equivalent original Toast workload: the saved target's
deadline was altered by bootstrap. Keep errors and uncertainty visible.

## Latest diagnostic capture

.tmp/player-lag-full-native-smoke contains retained engine.test.exe, manifest,
console, JSON logs and Go/MOO profiles. Original hashes were rechecked. Settings:
1/4/16 players;2s warmup/8s measurement;GOMAXPROCS32;GOGC400;repairs off;
runtime debug diagnostics and profiles on; no Toast bootstrap.

The retained binary identifies the uncommitted source actually run. This is a
diagnostic smoke, not a clean paired speedup comparison; an analyst's short Go
probe builds overlapped it. It measures in-process terminal completion, NOT
socket first output.

Raw cohort results:

    players=1 completed=359 failed=0 unsettled=0 p99=126.76ms max=165.58ms
    players=4 completed=815 failed=4 unsettled=0 p99=566.59ms max=854.99ms
    players=16 completed=1340 failed=18 unsettled=0 p99=1415.43ms max=3097.09ms

Native full-capture terminal census: E_NACC10,E_PROPNF19, no E_MAXREC record.
Counts include warmup/background; do not attribute process-wide deltas to a
command. Source failure frames include #71 home/bf_move and @who/bf_idle_seconds;
do not change permissions from labels alone.

The largest whole-capture commit-gate waits (~1.426s) belong to restored startup
tasks around00:21:33. A6.384s slice has root label #0:server_started around
00:22:17; labels identify root tasks, not the currently running nested verb.
The16-player MOO wall profile has do_say42.52% and forbids36.67% cumulative.
Nested wall intervals overlap and are not CPU. Go whole-run samples include
startup/warmup/drain and show VM/property work plus scheduler scans. Neither
profile alone explains off-CPU waits or proves a new optimization.

## Surprises and corrected findings

Lossless native conversion required preserving CATCH/FINALLY type tags, selected
fork vector, error PC, temporary and builtin state. A one-line recompile would
lose continuation semantics. Exact native disassembly corpus and all70 original
frame continuations were checked.

The first control repair's passing focused tests missed nested pending reasons.
Independent probes exposed overwritten outer errors/returns/jumps; managed Toast
confirmed them. The corrective shared reason stack replaces the separate slots,
rather than layering backups. Nested cancellation is trimmed at the next actual
cleanup destination, not prematurely at an eventual jump target.

The elseif-layout objection and dynamic-except-count objection were retracted
after source checks; reports preserve those corrections. An initial loop probe
used expression mode and returned early; corrected statement-mode results are
the evidence. A literal non-ASCII string probe produced an empty loop; it is not
encoding evidence. The valid byte producer uses chr(195,169).

## Next step

1. Consolidate shared implementation in a substantive local commit, including
   this note (completed). Both branches now share newer upstream Elle coverage,
   parser, checkpoint and output fixes. Preserve generated files/ from staging.
2. Complete final scoped verification without claiming CI/Elle. Native inputs
   were rechecked after the reason-stack cutover (engine9.038s); startup controls
   passed (server1.136s). Final worker/analyst focused commands are in reports.
3. Control worktree C:/Users/Q/code/barn-mongoose-lag-control exists on
   bench/mongoose-lag-control. It starts historically before the mutex repair
   and has all common fixes and upstream code applied forward. Source differences
   were verified as the three mutex-repair files plus notes/test. Do not revert
   the active branch.
4. Paired1/4/16 captures and warmed trace are complete, with failures retained.
   The measured task-row wait omission is repaired and scoped tests/capture
   expose it. Preserve the remaining terminal/first-output distinction before
   making further performance-policy changes.
5. Posting authority is now explicit. Run required PR CI and pinned Elle
   workflow before any merge. A green
   local focused test is not a remote exact-head, isolation or deployment verdict.

PR #614 evidence from its removed old worktree remains archived in
C:/Users/Q/code/barn/.tmp/player-command-latency-evidence.tar. Root master was not
fast-forwarded because unrelated untracked paper files would be overwritten.
The task worktree preserves those unrelated files and other agents' work.

## Shared commit wait correction checkpoint

The real task-row-only regression reproduced missing shared-gate counters and
observer calls while queued behind a held exclusive grant. The object-writing
path and gate-exempt controls establish the existing intended accounting.
Both paths now use one acquireSharedCommitGate owner method; gate policy,
validation/publication and exemption checks remain unchanged. Focused selector
TestTaskRowSharedGateWaitAccounting and
TestTaskRowExclusiveGrantDoesNotAccountSharedWait passed (store1.529s).
Report reports/task-row-wait-coder.md retains RED/GREEN and a clock-resolution
fixture correction. Parent owns per-task engine/metrics phase integration.
The same focused race selector passed (store2.722s). Next: parent integration
and scoped review. No full
CI, Elle, publication or merge acceptance is established by this correction.

Parent runtime held-gate regression now passes with-race (engine1.306s), and
Store focused-race result is2.722s. The .tmp/player-lag-accounted-smoke run
finished all1/4/16 cohorts with restored1080/49 and zero unsettled tasks.
One-player shared phase7193/447.424ms matches store counters7193/447.42ms;
four-player14436/2905.361ms;16-player27330/31304.313ms. These are process-wide
window totals and are not attributed to an individual command. Per-task records
are saved in JSON. 16-player p99=961.41ms/max1444.69ms,2325 completed/33 failed;
this diagnostic validation is not a controlled speedup comparison.

Standalone CallVerb and sweep-owned hooks in engine/call_verb.go now use the
same shared observer with the available hook/owner identity and nil-scope
handling. Their missing-observation finding is addressed: admitted and sweep
hook endpoints both reproduced missing observations, then passed with
the held-gate root test under-race (engine1.469s). The analyst's source followup
found its coverage issue addressed. Canceled acquisitions are explicitly outside
the successful-acquisition accounting. No gate policy changes.
The owned Go trace viewer has been stopped; task-specific downloaded profiles
were copied beside the raw trace. Q has now authorized publication and PR work.

The existing barn_logs CLI read actual task2145966056 from the corrected capture:
shared_commit_gate172/306.3509ms versus exclusivecommit_gate2/3.0939ms,
174slices totaling1156.4991ms, invalidrecords0, terminalexceptions0 and
commandrecords0 (in-process harness). Raw CLI JSON is saved with the capture.
The final hook correction postdates this smoke; it has focused-race coverage.
Final scoped local review found no remaining concrete source blocker; its
verdict remains NO-MERGE pending exact-head PR CI and pinned Elle evidence.
Both requested prose corrections (current hook coverage and admission scope)
are applied. Source, reports and this note belong to the substantive correction.
PR #635 is open after integrating fetched upstream isolation/map changes.
Its first Go job failed Staticcheck ST1005 at compiler/native_import.go259
because an error began with a capitalized product name. The message is
corrected in source; no check was weakened. Raw first-job log is retained in
.tmp/pr635-first-go-job.log. CI and Elle were running against the published
head; no full acceptance is established. The next run passed formatting, vet,
Staticcheck, golangci-lint and build, then found the old malformed-bytecode
fixture still expected0xff to be unknown. UNWIND_JUMP now occupies that final
byte value; all opcode bytes are assigned. The same raw one-byte malformed
input now requires the specific truncated-UNWIND_JUMP rejection. The verifier's
unknown/dead/truncated checks remain intact. Raw second-job log is retained in
.tmp/pr635-second-go-job.log; the first interrupted Elle archive is retained
under .tmp/pr635-first-elle. Next: push the fixture correction and finish the
new exact-head PR CI and pinned Elle before any merge.
Remaining first-output work needs a schedule-preserving credential path; no
login script is available and the unsafe Toast bootstrap remains rejected.
