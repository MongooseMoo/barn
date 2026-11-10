# Transactional recycle scalability

> RESUMED: Q directed "superb, make it so! worktree!" after the corrected handoff.
> Implement the corrected direction: remove numbered-object reservations, keep
> anonymous guards, and reuse transactional movement and deletion. The earlier
> reservation-migration proposal below remains rejected historical context.

## Goal

Investigate the worker's `barn-recycle-commit-gate.md` report from
`mongoose@mongoose.world:~/agent-workspace/tasks/issue-795-cpu-for-bugs/` and
prepare a measured scalability improvement in an isolated worktree and PR.

## State

- Branch: `perf/transactional-recycle`, created from fetched `origin/master`.
- Worktree: `C:/Users/Q/code/barn-transactional-recycle`.
- The worker report was read over SSH. It explicitly contains no measurements.
- Q authorized the corrected design after the off-design handoff.
- Production code remains unchanged at the baseline-measurement stage. The
  scaling benchmark is in `engine/object_lifecycle_bench_test.go`.
- The rejected oracle scenario was replaced by a controlled two-fork scenario
  for numbered and anonymous objects. Managed Toast returned exit zero:
  `4 passed, 1 warning in 8.24s`. Unchanged Barn returned exit one:
  `1 failed, 3 passed, 1 warning in 8.56s`; the numbered case returned E_INVARG
  instead of the expected successful result. The anonymous case passed.
- Evidence: `.tmp/recycle-oracle/results-corrected.xml`,
  `admission-corrected.json`, `results-barn-baseline.xml`, and
  `admission-barn-baseline.json` in the same directory.
- Read `docs/workload-diagnostics.md`, `docs/elle-isolation.md`,
  `docs/mongoose-conformance.md`, and the convergence workstream's oracle rules.

## Findings from current source

- `builtins/objects.go:434-443` crosses the irreversible boundary before taking
  the session-wide recycle reservation. Placement is not required for this gate
  acquisition.
- `engine/task_runtime.go:355-430` acquires the exclusive commit gate at that
  boundary, validates/publishes/renews, and retains the gate for the attempt.
- `builtins/objects.go:591-630` also crosses that boundary, flushes staged
  topology, and moves through `DirectTxn()` during VM recycle cleanup.
- `builtins/objects.go:533-580` has a transactional tombstone path, but prior
  direct movement or a staged-topology flush selects the coarse path instead.
- `db/store/store_txn_objects.go:250-291` already stages deletion of a numbered
  object with no children or contents, including removal from its location.
- `db/store/store_txn_relationships.go:121-174` already stages movements with
  commutative inverse contents deltas.
- `db/store/store_core.go:282` allocates numbered IDs with atomic
  `s.ids.highWater.Add(1)`. `MaxObject` is not the allocation mechanism.
- `vm/recycle_lifecycle.go` executes recycle/exit callbacks through VM
  continuations. `vm/persistence.go:124-180` restores their session reservations.
- `engine/recycle_retry_boundary_test.go` and `builtins/recycle_retry_test.go`
  protect a reservation surviving a committed suspension and an abandoned
  continuation attempt. Merely removing `beginIrreversible` is not a design.
- Existing `engine/object_lifecycle_bench_test.go` deliberately installs a recycle
  hook and measures operations separately; it is not a concurrent placement
  scalability measurement.

## Literature

Applied the research-papers plugin's paper-reader entrypoint to the existing
Kung/Robinson paper directory. Its notes, abstract, citations, canonical PDF and
page images already exist (the skill's existing-complete case). Directly inspected
page images 000-003, printed pages 213-216; the paper title page is dated 1981,
despite the existing directory name containing 1979. Only these pages were read
in this task. They describe private writes followed by validation and publication,
including transaction-local create/read/write sets. This supports using Barn's
existing transaction owner rather than adding a second publication mechanism;
it does not establish Barn-specific lifecycle semantics or a speedup.

## Decisions and why

- Preserve unrelated work in the main checkout by using a sibling worktree.
- Treat the report as a source-backed candidate, not a measured production
  bottleneck. No profiles, settled cohorts, gate census or scaling results have
  been collected for this task.
- Resolve observable reservations and suspension semantics before choosing how
  much of recycle can remain speculative. A move-only edit leaves the first gate
  acquisition intact.

## Surprises

- The initial reservation, not just placed-object movement, takes the gate.
- The report's unknown ID allocator is already an atomic counter.

## Superseded implementation plan - do not implement

1. Establish baseline evidence using the existing runtime and diagnostic owners.
   Extend the existing object-lifecycle benchmark with concurrent tasks doing
   create, place, recycle, and create again. Cover both previously committed and
   same-slice-created objects; independent rooms and a shared room; no callback
   and a pure MOO callback. Use 1, 2, 4 and 8 workers, equal instrumentation and
   settled cohorts. Retain raw timings, throughput, allocations, gate holds and
   waits, retries and wasted ticks. Run measurements on a Linux compute runner,
   not the workstation or the live Mongoose service. Existing diagnostics are
   sufficient; do not add another metrics subsystem.
2. REJECTED: Move the reservation's authoritative state into the store-owned object
   lifecycle, carried by its existing scalar MVCC version. Stage acquisition and
   release in `StoreTxn`; check acquisition through the transaction so a stale
   observation conflicts. A plain bool in the object image and its existing
   scalar write record is sufficient for ordinary reserve/release state; do not
   expose a new MOO property or invent a separate lock service. Read-only validity
   remains true while the recycle hook runs. An initial reservation discarded
   with a failed attempt never becomes globally visible. A reservation published
   by a suspended slice remains visible until its owning continuation finishes.
   Remove the session map and its independent mutex after all callers use this
   store owner. Preserve separate-world isolation using distinct stores.
3. Reconstruct transient reservations from the existing persisted recycle
   continuations at cold restore, with duplicate detection and all-or-nothing
   restoration. Preserve the current snapshot format unless source inspection
   during implementation proves a missing contract. Do not confuse restoring a
   cold checkpoint with retrying a continuation whose reservation is already
   committed. Release on exitfunc failure through the same transaction owner;
   abandoned retries must retain the prior committed reservation.
4. In `builtins/objects.go`, replace unconditional irreversible entry/finish
   work with transaction-owned reservation operations. Keep cleanup moves in
   `StoreTxn.MoveObject`, preserve their last-move bookkeeping and callback
   order, and finish an ordinary non-player numbered object with the existing
   `StoreTxn.RecycleObject`. Do not flush its staged topology merely to advance
   the lifecycle. Keep its later creates on the existing transactional path.
   Anonymous deletion, player disconnection, child reparenting and a hook's
   genuinely irreversible builtin continue through their existing coarse
   boundary. These are explicit limits of this PR, not a claim that every recycle
   becomes parallel.
5. Preserve the current early-abort regression when a resumed hook has already
   invalidated its read dependencies: abandon that attempt before handing control
   back to its caller. Reuse the existing read-validation implementation at the
   replacement lifecycle boundary; do not weaken the test or rely on a final
   commit to hide an already-observed stale continuation.
6. Verify reservation acquisition races, recursive recycle, concurrent recycle
   of one object, concurrent move/create under the target, inherited callback
   changes, both hook error paths, suspend/resume, failed continuation attempts,
   checkpoint restore and duplicate restore. Assert committed relationship
   reciprocity and exactly-once durable callback effects. Add an Elle held pair
   for reservation acquisition/publication and a fault removing its dependency
   validation; require an explicit anomaly witness from the fault. Exercise
   normal relationship validation with the existing pinned Elle corpus as well.
7. Compare baseline and candidate with the identical workload. The ordinary
   lifecycle must no longer produce a recycle-caused exclusive gate hold;
   contention should appear as ordinary MVCC conflicts when objects overlap.
   Report measured limits and remaining coarse paths. A speedup is unknown until
   these results exist.
8. Publish one independent PR against `master` with this note and raw evidence
   references. Use the repository's `CI_RUNNER_LABELS` policy. Run required CI,
   managed conformance and the pinned Elle workflow on the PR; preserve failure
   artifacts and repair within scope. Follow the standing normal-merge and
   worktree-cleanup instructions after required checks and review clearance.

## Oracle evidence and remaining evidence

- The focused managed Toast run completed with exit zero. Raw terminal summary:
  `7 passed, 13813 deselected, 1 warning in 260.43s (0:04:20)`.
  The warning is the existing `record_property` / `junit_family=xunit2`
  incompatibility. This is evidence for the selected scenarios only.
- Admission and JUnit artifacts are
  `/tmp/barn-recycle-plan-oracle-admission.json` and
  `/tmp/barn-recycle-plan-oracle.xml` inside WSL Debian. The exact command,
  run from this worktree through `wsl -d Debian --exec bash -lc`, was:

```bash
set -euo pipefail
cd /mnt/c/Users/Q/code/barn-transactional-recycle
export UV_PROJECT_ENVIRONMENT=/root/.cache/barn-conformance-linux
uv run --project /mnt/c/Users/Q/code/moo-conformance-tests --frozen moo-conformance \
  -m 'admission or conformance' \
  --server-command='/root/src/toaststunt/build-release/moo {db} {db}.new {port}' \
  --server-db=/mnt/c/Users/Q/code/moo-conformance-tests/src/moo_conformance/_db/Test.db \
  --server-db-dir=/mnt/c/Users/Q/code/moo-conformance-tests/src/moo_conformance/_db/startup \
  --moo-host=127.0.0.1 \
  --oracle-profile-manifest=profiles/toast/stock-wsl-testdb.json \
  --target-profile-manifest=profiles/toast/stock-wsl-testdb.json \
  --fail-on-unexpected-skip --strict-markers \
  --admission-evidence-output=/tmp/barn-recycle-plan-oracle-admission.json \
  --admission-evidence-context=barn-recycle-plan-oracle \
  --junitxml=/tmp/barn-recycle-plan-oracle.xml \
  -k 'capability_admission or recycle_exit_callback_context or recycle_hook_error_propagates_after_recycling_permanent_object or calling_recycle_in_recycle_verb_on_anonymous_fails' -q
```

- Cross-task recycle while a hook is suspended needs a specific oracle scenario
  before changing that behavior. Existing source/tests alone are not an oracle
  observation. Implement this scenario after code authorization, verify it on
  Toast first, and preserve its raw results before Barn changes.
- No performance baseline or candidate results exist. No claim about production
  impact, scalability gain, isolation coverage or PR readiness is made.

## Historical off-design handoff - corrected design now authorized

### Transcript and departure

The agent's proposal to Q was: "Put recycle reservations under existing object
MVCC." The agent also wrote: "Keep ordinary movement and deletion transactional,
preserving suspended callbacks." Q's authorization was: "Go".

The false premise was treating Barn's session-wide numbered-object reservation
as required MOO behavior and proposing to move it into MVCC. That premise came
from Barn's existing source and tests, before verifying the corresponding Toast
case. Implementing that plan would preserve an existing divergence and add
unneeded state. No implementation of that proposal was made.

### Correct source and corrected direction

The installed documented Toast oracle source is
`/root/src/toaststunt/src/objects.cc`. At lines 803-805 it rejects a flagged
recycle only when the value's type is `TYPE_ANON`. At lines 811-812 it sets
`FLAG_RECYCLED` only for `TYPE_ANON`. A valid numbered object proceeds to its
recycle callback without that guard. The running oracle build's source revision
was checked against the pinned convergence-plan revision.

The public primary source also contains this distinction:
https://github.com/lisdude/toaststunt/blob/master/src/objects.cc

Corrected direction: remove the numbered-object session reservation rather than
introduce an MVCC replacement for it. Keep the required anonymous guard, its
coarse publication boundary, and anonymous continuation restore protection.
Use existing transactional moves and tombstones for ordinary non-player numbered
objects. Preserve the genuine coarse paths for anonymous destruction, player
connections, child reparenting and actual irreversible callback effects.
Numbered-object continuations must not restore a guard that Toast does not have.
Do not build new object fields, locks or reservation-validation faults for the
rejected numbered-object guard.

### Everything built on the false premise

- This note's former proposed-plan steps 2, 3, 5 and 6: superseded. Remaining
  measurement and publication goals still describe intended work, not results.
- Uncommitted addition in
  `tests/mongoose-conformance/builtins/recycle_unrelated_tasks.yaml`:
  `recycle_suspended_hook_rejects_competing_recycle`. It expects a competing
  numbered-object recycle to return E_INVARG immediately. This expectation is
  rejected. The same edit extends the file's builtin requirements for the probe.
- Its managed Toast run completed with exit one. Raw summary:
  `1 failed, 2 passed, 1 warning in 11.01s`. The new scenario timed out in
  `SocketTransport._receive`; its server-log excerpt ends with installing the
  target's recycle verb. The timeout alone does not prove an exact MOO outcome.
  The source distinction above is independently checked evidence.
- Raw JUnit and admission evidence are retained under
  `.tmp/recycle-oracle/results.xml` and `.tmp/recycle-oracle/admission.json` in
  this worktree. The input is the uncommitted YAML itself. The documented managed
  command used candidate mode with this worktree, that YAML path, copied packaged
  databases under `.tmp/recycle-oracle/db`, canonical capability admission, and
  the documented WSL Toast executable. A Linux-only task environment was used:
  `/root/.cache/barn-recycle-conformance-linux`.
- At the stopped handoff, no production source, benchmark or Elle code was changed. No commit, push,
  PR, deployment, production DB mutation or production service restart occurred.
- At the stopped handoff, no subagents were dispatched and no task commands remained running.
- Only this task note carries the mistaken design framing on disk; it is marked
  superseded above. The rejected test is retained as investigation input, not a
  publishable conformance contract.

### Undo and redo when Q directs the correction

1. Remove the rejected added scenario and its unnecessary requirement-list
   expansion, preserving the pre-existing unrelated-queued-task regression.
2. Write an oracle scenario that can observe two numbered-object recycle hooks
   without waiting for an assumed rejection: use a separate controller task to
   release the hooks, record entry/return outcomes explicitly, and bound waits.
   Add a separate anonymous case for the type-specific guard. Verify both with
   managed Toast and capability admission before changing Barn behavior.
3. Correct numbered-object guard acquisition, release and persistence callers;
   retain anonymous semantics. Make normal cleanup transactional through the
   existing store APIs. Preserve early detection of stale resumed attempts using
   existing dependency validation as justified by the corrected semantic tests.
4. Add the planned scaling benchmark; measure identical baseline and candidate
   fixtures, then run PR CI, managed conformance and pinned Elle. Use existing
   relationship/lifecycle protection and appropriate real dependency witnesses;
   do not retain a fault for a nonexistent numbered-object reservation.
5. Publish and merge only after the corrected implementation and required
   evidence exist, following the original worktree/PR scope and standing rules.

### Open decisions and unverified evidence

- Exact return/error ordering when two numbered-object hooks overlap and one
  destroys the object first is not yet observed through a completed managed test.
- Anonymous guard release on hook and exitfunc errors needs the corresponding
  oracle observation before altering those branches.
- No scaling benchmark or baseline/candidate measurement exists. No isolation
  verdict exists for a candidate change because no candidate exists yet.
- The guest `barn-compute` is reachable through the documented management SSH
  route. It reports eight CPUs; its load average was 11.76, 13.13, 13.85 during
  inspection. Four fuzz runners were busy. Existing work must not be interrupted
  to obtain measurements. Go is installed in the existing Actions tool caches.
- Repository CI is configured for the ordinary `barn` pool, which GitHub lists
  as `Q-barn-wsl-x64` runners; do not call those off-workstation measurement
  resources. The separate `barn-compute` guest is the planned measurement host.

### Historical handoff next step

Give Q this note path and stop, as required by the off-design handoff rule.
If Q directs the fix, resume from the corrected design above.

## Current next step

Capture scaling baseline on `barn-compute`, implement the approved correction,
and publish the independent PR. Verify through PR CI and pinned Elle, then
normal-merge and clean up the task worktree when required checks are clear.
