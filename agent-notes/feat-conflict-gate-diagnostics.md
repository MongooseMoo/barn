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

Built and committed on the branch; not pushed, no PR. Master merged in after
#684 (harness home repair), #685 (the boundary validates and carries keyed map
reads per entry) and #686 (exec crosses the boundary itself, after its checks):

- The boundary now degrades only a keyed assignment's pending base
  (`boundaryMapKeys` -> `degradeMapKey(rkey, st, DegradeBoundary)`); live
  mutation, flush and rename degrade through `degradeMapKeysVia(path)`, which
  loops over `degradeMapKey`. `Degraded.Boundary` therefore counts only
  pending-base slots; a plain keyed read lost at the boundary is a `map-key`
  path with `LostAt.Boundary` and no degrade.
- `builtinExec` passes `"exec"` to `beginIrreversible`, so exec gate holds and
  boundary sites keep their builtin (curl already passed `"curl"`).
- Tests: the boundary path test expects a map-key loss with no degrade; new
  `TestConflictPathCountsPendingBaseDegradedAtBoundary` and
  `TestConflictPathCarriedKeyedReadLosesAtFinalCommit` (a carried keyed read
  keeps its pre-boundary read site and loses at the final point); the exec
  boundary test asserts the builtin name.

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
- G10: `barn -grep-verbs REGEX` (internal/dbtool GrepVerbs) prints
  `#obj:verb:line: text`. Inherited property values in full and several
  queries per load already exist through `-eval` and `-eval-file`.
- Measured (16 players, fixture copy of the 2026-11-10T0143 dump, gated under
  25% CPU): goodput tracking off base 133, 99 (both runs had unsettled `home`
  commands) vs new 151, 142; tracking on base 87, 197 vs new 155, 154. The
  spread between runs of one binary is larger than any difference. Go
  benchmarks: per command no new allocation, -30 B; per commit unchanged;
  +2 allocations per exclusive gate acquire.

## Run evidence

A tracked 16-player run named the whole #2585.handle story in one table:
866 losses, all at the boundary opened by `sqlite_execute()` at
`#6128:lookup_cache_version@7` (mean tick 8211), read at `#2585:handle@1`,
read-to-loss wall mean 12.04 ms of which 11.61 ms was exclusive gate wait,
200 of them after a hot-read renewal; the `exec()` hold of `#9655:arm` was
ahead of the say holders in most acquires.

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
- `TestCensusCountsRefusedRenewals` was red on the branch before the merge
  (only focused tests had been run): the always-on per-key `HotRefused` gives
  the refused hot slot its own row, so the census has two keys. The test now
  expects that row.
- The merge's only textual conflict was `degradeMapKeys`; the exec call site
  merged cleanly but did not compile (`beginIrreversible` takes the builtin
  name on this branch), and master's exec tests used the old
  `func() bool` hook.

## Next step

Apply the skill doc draft in the main checkout. Open a PR when the
coordinator asks (CI has not run on this branch). `do_command` tasks in the
harness may not carry `FromCommand`, so their slow timelines are kept for the
endpoint but possibly not logged; check before relying on the log record.
