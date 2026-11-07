# Mongoose player lag

## Goal

Complete Q's approved diagnostics and lag investigation using the existing
Mongoose player harness. Preserve the original checkpoint's semantics, repair
verified defects, compare the runtime mutex repair, and inspect remaining waits.
No live deployment. PR #614 was already merged normally.

## State

The shared implementation is local and has not been published. The input
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
No full-change CI/Elle or merge verdict exists yet. New PR publication still
awaits Q's explicit posting authority; do not push/post while that hold remains.

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
   this note. Integrate newly fetched origin/master changes before publication.
   Upstream now includes newer Elle coverage, parser, checkpoint and output fixes.
2. Complete final scoped verification without claiming CI/Elle. Native inputs
   were rechecked after the reason-stack cutover (engine9.038s); startup controls
   passed (server1.136s). Final worker/analyst focused commands are in reports.
3. Control worktree C:/Users/Q/code/barn-mongoose-lag-control exists on
   bench/mongoose-lag-control. It starts historically before the mutex repair
   and has the subsequent shared harness change applied forward. Apply identical
   completed common fixes and upstream code there, then verify that implementation
   differences are only the mutex repair. Do not revert the active branch.
4. Run quiescent paired1/4/16 cohorts with identical original fixtures/settings,
   retain failures, and inspect slow task phases, profiles and scheduling before
   any further performance change. Current diagnostic smoke still has real tails.
5. Obtain explicit posting authority only when the result is concrete/reviewable,
   then run required PR CI and pinned Elle workflow before any merge. A green
   local focused test is not a remote exact-head, isolation or deployment verdict.

PR #614 evidence from its removed old worktree remains archived in
C:/Users/Q/code/barn/.tmp/player-command-latency-evidence.tar. Root master was not
fast-forwarded because unrelated untracked paper files would be overwritten.
The task worktree preserves those unrelated files and other agents' work.
