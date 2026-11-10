# feat/conflict-gate-diagnostics

## Goal

Q: "the agents should actively build tooling to make this easier to diagnose —
into the actual binary itself / better diagnostics / better profiling output",
for the Mongoose goodput campaign (agent-notes/mongoose-goodput.md). Close the
gaps that blocked this week's diagnoses, in the real binary (debug endpoint,
structured log, shutdown summary), reused by the in-process harness through one
implementation:

- (a) per lost commit: loser verb, key, kind, read site, writer, ticks wasted,
  attempt, aggregated by (key, loser verb, read site);
- (b) exclusive commit gate per holder (verb + builtin): count, hold time, ticks
  under the gate, and shared waits queued behind an exclusive;
- (c) per verb: retries, wasted vs committed ticks;
- (d) per-task timeline for slow/unsettled tasks, printed by the harness with
  the MOO error and traceback of every failed command;
- the coordinator's later list: loss point (boundary / suspend / final) and the
  boundary's builtin, degraded keyed reads, gate wait between read and loss,
  and G1-G9 of the 2585-handle diagnosis (loss point split, read-to-loss wall
  split, gate queue depth and labels ahead with per-holder wait histogram,
  writer publication point, retries per command, renewals per key, other
  writers by root, builtin calls by site, first-boundary tick).

## State

Built and committed on the branch; not pushed, no PR.

- db/store/store_conflict_paths.go (new): census `Paths`, `Verbs`,
  `BuiltinCalls`; `SetConflictOrigin`, `NoteBoundary`, `NoteCommitGateWait`,
  `SetConflictCommitPoint`, `NoteTaskRetries`, `NoteBuiltinCall`; `WallHist`.
- db/store: `CommitPoint` and `DegradePath` vocabularies, writer
  `PublishedBy`, `OtherWriterRoots`, per-key `HotRenewed/HotRefused/HotDeclined`,
  `RenewedThenLost`, `Degraded`; `SharedGateBlocked`; `degradeMapKeysVia`.
- internal/commitgate: `Grant.Blocked`, `AcquireLabeled`, `Grant.Ahead`.
- engine: gate holds gain builtin, ticks under the gate, acquire wait
  histogram, queue depth and labels ahead (`sliceHold`); task timelines
  (task_timeline.go) with slow-task INFO log and `Runtime.TaskTimelines`;
  diag_report.go formats the census and gate report (harness and shutdown
  summary share it). `BeforeIrreversibleEffect(builtin)`; every caller of
  `beginIrreversible`/`beforeCoarse` names its builtin.
- internal/app: `/debug/commit-gate`, `/debug/task-timelines`, and the
  `diagnostics summary` record at server stop.
- Harness: prints the shared report; every failed/unsettled command gets its
  MOO error, traceback and timeline; the stale "say will raise" sound line is
  reworded (the fixture repair block was not touched).
- docs/workload-diagnostics.md: "Contention diagnostics in the binary".

## Decisions and why

- One switch: everything per-transaction rides the existing census tracking
  (`CONFLICT_CENSUS = 1`, harness `BARN_MONGOOSE_STALE_READS`). Gate holds,
  shared-behind-exclusive counts and per-key hot-read outcomes are rare events
  and stay always on.
- Paths key the loser by the verb's defining object (`t.VerbLoc`), so every
  player's `say` is one row; the existing per-key `Labels` keep `this`.
- The builtin name is passed explicitly through `beginIrreversible(ctx, name)`
  rather than decoded from the VM: explicit, zero cost.
- commitgate labels are `any` and only rendered when reported, so a queued
  request costs no formatting; exclusive-only grant data sits behind a pointer
  and the bools are packed so the per-commit Grant allocation did not grow.
- Timeline state (admission wait, retries) lives in the timeline, so a task
  without tracking carries one extra pointer.

## Surprises

- The barn-run-logs skill exists only as an untracked file in the main
  checkout; worktree isolation refuses edits there. The drafted text is in the
  coordinator's progress file (scratchpad plan/build-diagnostics.md, "Skill doc
  draft").
- The first cut cost one allocation and ~210 B per command and 96 B per commit
  with tracking off; fixed before measuring (see State/Decisions).

## Next step

Apply the skill doc draft in the main checkout; read a tracked harness run's
report against a fresh diagnosis. G10 (inspection tooling: inherited
property view, batch inspection, verb-code grep) was not started.
