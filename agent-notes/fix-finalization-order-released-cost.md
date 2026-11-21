# Finalization at released cost (issue #1083)

## Goal

Design (no production code) how Barn decides whether the anonymous objects and
WAIFs a slice released are dead at a cost proportional to what was released,
not to the database size, and runs their `recycle` verbs before the releasing
task resumes, as ToastStunt does. Q chose this direction over the size-dependent
hold of closed PR #1040.

## State

Design complete; no production code. Recommendation: design C2 (section 7),
with the census evidence in section 5 and four open decisions for the owner
in section 9. The measurement patch is uncommitted in the detached worktree
`C:/Users/Q/code/barn-finalization-measure` and saved as
`C:/Users/Q/AppData/Local/Temp/claude/655669a3d1b9c796/ea6e86d2-9d20-42c3-abeb-b77523e31b37/scratchpad/fincensus.patch`.

## Decisions and why

- Measure before choosing: no earlier note had measured what Mongoose
  releases. The census showed over 90 % of released WAIFs are alive and held
  directly by a numbered object, and almost all the rest are confined to the
  task that made them, so the common case needs neither exact reference
  counts (A) nor task registration (B).
- Counts are used only to prove a value alive (numbered holder), never to
  prove it dead, so the 2026-10-28 objection to plain counts (transitive
  holders) does not apply to C2.
- Classify after the slice commit: run 1 classified suspends before the
  commit and counted holders the slice was about to overwrite.
- The census marks escape at write time (conservative); the design marks
  it at publication with the exclusive-gate refinement, because the #1083
  anonymous scenario publishes and then destroys its only holder inside one
  gated slice.

## Surprises

- No anonymous object was created in any measured window on this fixture;
  every released anonymous object was held by a numbered object. The
  anonymous fast path has no real-data evidence.
- The #1083 anonymous scenario is not plainly confined: `add_verb` makes the
  slice coarse, and `recycle(h)` flushes `h.ref` to the live store before
  destroying `h`.
- Anonymous objects are reachable by number from any task
  (`toobj("#4611686018427387904")`, `db/store/store_core.go:56,699-711`;
  `valid`, property reads, verb calls, `move`, `chparent` work). Toast
  anonymous objects have no number. Not filed; reported to the lead.
- The current sweep has holders it does not treat as roots (section 2: wake
  values, fork variables, call-stack snapshots read by `task_stack`, task
  rows, connection options, the finished-tasks ring, hook args before their
  VM, threaded-builtin closures). Not probed for a wrong recycle.
- Several VM release sites record nothing (`OP_POP`, discarded verb return,
  indexed assignment, except variable); read from the code, not probed.
- WAIF property writes go through two store entry points
  (`SetWaifProperty`, `ReplaceWaifProperty` from `vm/op_property.go:448`);
  the first census runs instrumented only the first.
- Toast never recycles a WAIF cycle (WAIFs are not GC collections); Barn's
  sweep does (both read from source, not probed).
- The "cheap" pre-check `HasAnonymousAtOrAbove` and the candidate scan range
  over the whole numbered directory (`store_reachability.go:32-45`).

## Next step

The owner answers D1-D4 (section 9). Then a fresh implementation agent
builds C2 from this note, test plan section 8 first (the #1040 tests and the
per-channel tests failing on master), and runs the real-workload gate
interleaved against master. The measurement worktree can be removed once
the lead no longer needs it.

## Findings

### 1. Toast (lisdude origin/master, read with `git show origin/master:src/...`)

- `utils.cc:217-277` `complex_free_var` (ENABLE_GC build): TYPE_WAIF at
  refcount 0 is inserted in `destroyed_waifs[w] = false`; TYPE_ANON at
  refcount 0 is `queue_anonymous_object(v)` unless already RECYCLED/INVALID;
  a nonzero decrement of an anon (or list/map) makes it a possible cycle root
  (`gc_possible_root`). Cost per release: O(1). Nested values are released
  when their container is freed (`release_container`), so a chain dies in one
  cascade.
- `server.cc:607-631` `queue_anonymous_object` (takes a new ref),
  `server.cc:633-673` `recycle_anonymous_objects`: swaps out the whole queue,
  sets FLAG_RECYCLED, `run_server_task(-1, v, "recycle", ...)` synchronously,
  destroys, frees. Objects queued while it runs (props of a destroyed anon,
  values the recycle verbs drop) wait for the next main-loop iteration.
- `server.cc:675-702` `recycle_waifs`: runs `:recycle` once for every
  `destroyed_waifs` entry still `false`, marks it `true`, frees entries whose
  refcount is back to 0 (a recycle verb can resurrect a WAIF; it is not
  recycled twice).
- `server.cc:770-883` `main_loop`: each iteration `gc_collect()` only if
  `run_gc()` was called, more than `GC_ROOTS_LIMIT` (2000, `options.h:54`)
  possible roots are buffered, or a checkpoint is due; then checkpoint
  handling; then `recycle_anonymous_objects(); recycle_waifs();` (878-879);
  then `network_process_io`; then `run_ready_tasks()` (883).
- `tasks.cc:1629-1823` `run_ready_tasks`: moves due waiting (forked and
  suspended) tasks into their tqueues, then runs at most one task (`did_one`)
  and returns.
- Order Toast guarantees: every recycle verb queued by a task slice (values
  whose refcount reached 0 during that slice) runs before ANY later task
  slice starts, the releasing task's own resume and every other ready task
  alike, because one main-loop iteration runs exactly one slice and every
  iteration drains the queues first. A second-generation release (an anon
  held only by an anon being recycled) runs one iteration later, so one other
  task slice can run between the parent's and the child's recycle; Toast does
  not promise the cascade before the releaser resumes.
- Cycles: anonymous objects in a garbage cycle are found by Bacon-Rajan trial
  deletion (`garbage.cc:120-405`; `collect_white` queues anons at 356-366),
  only when `gc_collect` runs (run_gc, >2000 roots, checkpoint). Its cost is
  proportional to the buffered roots' subgraph, not the database. WAIFs are
  not collections (`structures.h:191-193` `is_collection` = LIST, MAP, ANON),
  so a WAIF cycle is never recycled by Toast.

### 2. Barn today (worktree base: origin/master at the time of writing)

Problem, file:line:
- `engine/task_runtime.go:878-891` `handOffSuspended`: at a suspend the slice's
  released WAIFs go to `deferPendingWaifs` and, when anything anonymous may
  be garbage, `deferAnonGC(ctx, floor, nil)` queues a request; the VM is then
  saved and the task requeued (`:920-930`). Nothing waits for the queued work.
- `engine/waif_lifecycle.go:64-140`: both only append to
  `lifecycle.PendingWaifs` / `PendingAnonGC`. The verdict "dead" is made only
  by `flushDeferredGCBarrier` (`:236-392`), which needs the sweep barrier
  (`SweepMu`, `VMStartMu`, `:251-273`), skips when the last sweep cost
  >= 50 ms and was < 2 s ago (`:281-285`), and gives up unless every other
  running VM is parked (`collectAllGCRefs`, `:291-294`). Callers:
  `task_runtime.go:225` (slice boundary), `runtime.go:574,614,623,633`,
  `call_verb.go:251`; the maintenance worker
  (`deferred_gc_maintenance.go:41-81`) sweeps at `PendingSince + 2s` (or
  `LastGCSweep + 2s` once sweeps are expensive) and backs off 2 s after a
  failed rendezvous.
- The sweep's verdict is global: WAIFs are live if in
  `PersistentWaifRootSet` (closure over every holder in the direct-ref index,
  `db/store/store_reachability.go:365-402`) or any live VM; anonymous
  objects are recycled if `id >= floor`, not `FlagUser`, and not reachable
  from the persistent anonymous closure plus every live VM
  (`vm/anonymous_gc.go:401-444`, `store_reachability.go:47-81,305-324`).
  `HasAnonymousAtOrAbove` and `AnonymousRecycleCandidates` range over every
  anonymous object AND the whole numbered directory (`rangeAnonymousLocked`,
  `store_reachability.go:32-45`), so even the "cheap" pre-check is
  proportional to the database size.

Why Barn needs a sweep at all: at release time Barn knows only that one
holder (a local, a frame, a property slot) let go of the value. It does not
know whether any other holder exists, because Go values are copied without
any hook (no copy constructor, no destructor), so there is no count to
decrement. The only way to learn "no holder remains" is to enumerate every
holder: a trace over all roots. The anonymous half does not even track
releases: it recycles whatever is unreachable above the slice's creation
floor (`task_runtime.go:487-491`, `anonymous_gc.go:299-311`).

Where a reference to an anonymous object or WAIF can live (inventory with
file:line in `C:/Users/Q/AppData/Local/Temp/claude/655669a3d1b9c796/ea6e86d2-9d20-42c3-abeb-b77523e31b37/scratchpad/escape-channels.md` (session scratchpad, may be cleared), summarized; "root" = the current
sweep treats it as a root):
- The releasing task's VM: frames (locals, args, `this`, loop iterators,
  pending finally values, move/recycle continuations), operand stack,
  `yieldResult` (val, call stack, fork info), task-local, its txn's staged
  WAIF images (`vm/value_roots.go:17-182`). Root.
- Other tasks' VMs, running (root only once parked) or suspended/queued.
  Root.
- Committed properties of numbered objects (direct-ref index), of anonymous
  objects (in place, no history), WAIF property images (per-WAIF history).
  Root, transitively.
- Uncommitted transaction views: staged property writes and private anon
  copies; walked only for parked VMs (`engine/gc_handshake.go:82-93`).
- Retained history images served to old snapshots: root only for parked
  readers (`gc_handshake.go:100-114`).
- Admission roots (`engine/admission.go:92-128`): root.
- NOT roots today (each can hold a value the sweep would call dead):
  `task.State.WakeValue` (resume value before the VM reads it),
  `task.State.ForkInfo.Variables`, `task.State.CallStack` (frame `this`,
  args, the locals snapshot taken at every yield, read by another task's
  `task_stack()`), published task rows (`s.tasks.rows[].Value`), the
  `set_connection_option` store (`builtins/network.go:252-270`), the
  `finished_tasks()` ring (`this` of finished slices), args of
  `handle_uncaught_error` / `handle_task_timeout` / `handle_lagging_task`
  before their VM exists (`engine/task_factory.go:98-151`, not pinned when
  a scope is passed), goroutine closures of threaded builtins (`sort`,
  `background_test`).
- TYPE_OBJ forgeries: an anonymous object is reachable by number from any
  task (`toobj("#4611686018427387904")`, `db/store/store_core.go:56,699-711`;
  probe with `bin/moo`: `valid()`, property read, verb call, `move()`,
  `chparent()` all work). Toast anonymous objects have no number. Neither the
  sweep nor any design here can count such references; it is a separate
  divergence (see Surprises).

Release sites that feed the ledger (`PendingWaifs`, `PendingFinalizations`):
frame pop (`vm/vm.go:83-97`), local overwrite (`OP_SET_VAR`, loop variables,
scatter, callee return into a local), property overwrite of objects, map
keys and WAIF properties (`vm/op_property.go:175,398,417,453`), dropped
finally values, `recycle()` of an object (`builtins/objects.go:450-463`).
Sites that drop a value WITHOUT recording it: `OP_POP` (a discarded
expression result, e.g. a bare `new_waif();`), a discarded verb return
(`vm/stack.go:213-215`), indexed assignment `x[i] = v` (the replaced
element), the except-clause variable (`vm/vm.go:1632`), operand-stack
truncation on a caught error, consumed operands, `yieldResult` replacement.
For anonymous objects the creation floor hides most of this; for WAIFs an
unrecorded drop should mean no `:recycle` until something else releases the
same WAIF (read from the code, not probed against Toast or Barn). A
release-driven design must record these sites too.

### 3. Prior work (digest in `C:/Users/Q/AppData/Local/Temp/claude/655669a3d1b9c796/ea6e86d2-9d20-42c3-abeb-b77523e31b37/scratchpad/prior-gc-notes-digest.md` (session scratchpad, may be cleared))

- July 2026 (`notes/mongoose-differential-2026-07-01.md:259-305`): per-task
  full-database sweeps were the wall; options were (a) sweep only when
  pending, (b) WAIF reference counting, (c) batch/throttle. (c) was chosen;
  no reason is recorded for not doing (b).
- 2026-10-28 (`agent-notes/mongoose-faceoff-2026-10-28.md:179-232`, main
  checkout only): one sweep cost 682-988 ms on a Mongoose world; commit-time
  persistent reference counts for anon objects and WAIFs were proposed and
  rejected because "plain counts fail because both walks are transitive"
  (a holder that is itself an anonymous object or WAIF may be dead). The
  direct-ref index (PR 566) was built instead; sweeps fell to 53-124 ms. The
  designs below keep that objection in view: a count is only used to prove a
  value ALIVE when its holder is a numbered object (always a root), or, in
  design B, together with Toast's cascade (a recycled container releases its
  property values), which makes a count of direct holders exact for acyclic
  graphs.
- `notes/perf-execution-log.md:348-350` (lists): reference counting Go
  value-typed `Value`s was judged very high risk;
  `agent-notes/store-immutable-roots-research.md:859-860,1021-1023` rejects a
  retain/release call on every property assignment and a general graph heap.
  Design A below is that, and is not recommended for the same reasons.
- No earlier note measured what fraction of released values are dead or
  confined, and no note proposes an escape/confinement test.

### 4. Candidate designs

All three run the recycle verbs on the releasing task's goroutine after its
slice commit and before the task is handed back (`handOffSuspended` before
`SetVM`/requeue; on completion before the terminal state and `CloseDone`),
each in its own transaction exactly as the sweep's verb runner does today
(`callVerbWithArgstr(..., vmOwnershipSweep)` for WAIFs, `recycleFn` with
`gcRecycleContext` for anonymous objects). What the fast path cannot decide
goes to the existing batched sweep unchanged; the sweep stays as the cycle
collector and backstop, as Toast's `gc_collect` is.

#### A. Exact reference counts on anonymous objects and WAIFs (Toast's model)

Every holder increments, every drop decrements; zero queues the recycle.
- Holders that must count: every VM slot (locals, operand stack, frame
  `this`/args, iterators, pending values, yield results, retry checkpoints),
  every container (a list or map holding a WAIF must itself be counted, or
  the WAIF's count cannot drop when the last copy of the list goes), store
  properties of numbered and anonymous objects, WAIF images including
  retained history, task rows, connection options, the finished-tasks ring,
  hook arguments, threaded-builtin closures.
- Code paths: every opcode that copies or drops a value, every builtin that
  builds or returns a container, every store write, every place Go code
  copies a `types.Value`. Go has no copy constructor or destructor, so a
  container's death cannot be observed except by an explicit release at
  every owner; `runtime.AddCleanup` runs at Go GC time, not at the release.
- Cost: an atomic increment/decrement per copy of a finalizable-bearing
  value, plus container counts; MVCC retries must restore counts with the
  retry checkpoint.
- Guarantees Toast's order for acyclic garbage. Cycles never reach zero; the
  sweep stays.
- How it fails: one missed increment recycles a live object (a valid WAIF or
  anon becomes recycled under a task holding it); one missed decrement leaks
  (no `:recycle`). The surface is the whole VM and builtin library.
- Not recommended: it is the change earlier work judged too risky, and the
  measurements in section 5 show the common case does not need it.

#### B. Holder counts at slice boundaries (store + tasks), MVCC for in-flight readers

Counts, per anonymous object and WAIF, of holders that are not running VMs:
- store holders: committed direct references from properties of numbered
  objects, of anonymous objects, and current WAIF images, maintained at
  commit from the old and new direct references of each written slot (the
  numbered half is already computed by `refreshDirectRefs`,
  `db/store/store_reachability.go:154-175`);
- task holders: at every slice boundary a task registers the identities its
  VM and task state hold (frames, stack, task-local, `CallStack` and its
  locals snapshot, `ForkInfo`, `WakeValue`); the diff against its previous
  registration adjusts the counts; registration is published before the
  slice's commit becomes visible and undone if the commit fails;
- channel holders: task rows (`resume` values), connection options, the
  finished-tasks ring, hook arguments, threaded-builtin closures, counted at
  put and remove; a task that obtains a value through a channel that is not
  a store read (`task_stack`, `queued_tasks`, `finished_tasks`, hook args)
  registers it at once.
- In-flight safety: a running slice is not registered. Anything it obtained
  from the store at a snapshot older than the commit that dropped the last
  holder is in its read set, so it fails validation and re-runs (hot renewal
  refuses when a carried read is stale, `db/store/store_hot_read.go:203-240`;
  Elle checks serializability). Values it obtained from another task's
  registered state stay counted by that task until that task's next
  boundary.
- Decision at the releasing task's boundary, after its commit: every
  identity whose count reached zero in this slice (its commit's store
  decrements, its registration diff, its creations) and that its own VM does
  not hold is dead. Recycling an anonymous object then releases its property
  values (store decrements) and the loop repeats: Toast's cascade, finished
  before the task resumes (Toast runs the second generation one main-loop
  iteration later).
- Cost: per commit O(direct references of written slots); per boundary
  O(finalizable values in the task's own VM and state); per channel
  operation O(references in the value). Nothing proportional to the
  database.
- Checkpoints: counts are runtime state rebuilt after load from the loaded
  store and loaded suspended/queued tasks (one walk at startup, as the
  loader already does for the direct-ref index); pending finalizations are
  written exactly as today.
- Cycles never reach zero; the batched sweep collects them (Toast: only at
  `gc_collect`).
- How it fails: a missed holder kind (any of the "NOT roots today" list in
  section 2) recycles a live value; a store read of an anonymous object or
  WAIF that is not in the validated read set breaks the in-flight argument
  (Elle covers property registers, not anonymous-object or WAIF reads);
  direct transactions (`bin/moo`, `barn eval`) publish without validation
  and must keep using the sweep; registration order against commit order is
  a new invariant on the commit path.

#### C. Confinement plus numbered-holder proof (recommended)

Two cheap facts decide nearly every released value (section 5):
1. Confined: the value was created by this task (`new_waif()` and
   `create(..., 1)` stamp the creating task id on the WAIF rep / anonymous
   object) and has never escaped. Escape is a monotone bit, set when a value
   containing it is written to any property (at commit, from the same
   direct-reference visit `refreshDirectRefs` and the WAIF/anonymous
   publication already do), copied into a fork (`vm/control.go:131-180`),
   staged as a `resume` value, stored by `set_connection_option`, returned
   by `task_stack` / `queued_tasks` / `finished_tasks`, passed to a server
   hook, handed to a move-lifecycle verb, or captured by a threaded builtin.
   A confined value can only be held by its own task, so it is dead exactly
   when that task's own VM and task state no longer hold it: one walk of the
   task's own state, no other VM, no store.
2. Numbered-held: some numbered object's committed property holds it
   directly. Numbered objects are roots, so it is alive and nothing is to be
   done. The count is maintained in `refreshDirectRefs`, which every commit
   that changes such a reference already calls.
Everything else ("unknown") goes to the batched sweep, unordered, as today.
- Guarantees: both #1083 conformance scenarios are confined (an anonymous
  object created and dropped in one slice; a WAIF from `c:new()` dropped
  before `suspend(0)`), so they get Toast's order deterministically,
  independent of sweep cost and other VMs. No new invariant on the commit
  path; the fast path never needs another VM to park.
- Does not guarantee: order for "unknown" values (escaped and not held by a
  numbered object: held inside another WAIF or anonymous object, or by
  another task), or for cycles. These keep today's deferral (open decision
  D2).
- Code paths: two creation stamps; the escape sites above; the
  numbered-holder count in `refreshDirectRefs`; the boundary decision in
  `handOffSuspended`, `settleCompletedTaskFinalizations` and
  `call_verb.go:464` (verbs the sweep itself runs); the release sites that
  do not record today (section 2); the inline recycle runner.
- MVCC: property-write escape is set at commit, so an aborted attempt does
  not escape; fork/resume/channel marks are set at execution time
  (conservative). The decision runs after the commit, so a retry cannot
  resurrect a value the decision saw dead. Anonymous creation is not undone
  on retry (`db/store/store_lifecycle.go:11-56`); an orphan of a discarded
  attempt is confined and not held by the restored VM, so the next boundary
  recycles it, as the floor sweep does today.
- Checkpoints: stamps and bits are runtime state; values loaded from a
  database have no creating task, so they always take the numbered-held or
  sweep path. Pending finalizations are written as today.
- Interaction with the batched sweep: the sweep still runs for unknowns and
  cycles. A value the fast path recycled is marked (`BeginWaifRecycle`,
  anonymous RECYCLED/invalid) and the sweep skips it, as it skips any
  recycled value today.
- How it fails: a missed escape channel lets a confined value be held by a
  second task and recycled under it. Mitigation: the census oracle
  (section 5: every fast "dead" verdict re-checked by the full sweep) kept as
  a test-build assertion in the conformance, Elle and Jepsen runs.

Required refinement (C2), found by running the census on the #1083
anonymous scenario (`TestFincensusScenarios` in the measurement worktree,
statement copied from #1040's `recycleOrphanThenSuspend`): the WAIF scenario
classifies `confined_dead`, but the anonymous one classifies
`unknown_own_escaped`, because the anonymous object was written into `h.ref`
before `recycle(h)`. In that statement `add_verb` is a coarse builtin
(`builtins/store_reads.go:58-66` `beforeCoarse` -> `beginIrreversible`, then
`flushStagedBeforeCoarse`), so the task holds the exclusive commit gate for
the rest of the slice and `recycle(h)` flushes `h.ref` to the live store
before destroying `h`: the value is published for an instant and then its
only holder is gone (numbered-holder count 0 at the boundary). Plain C sends
it to the sweep, so the conformance test would stay as flaky as today. C2:
a publication made while this slice holds the exclusive gate is a tentative
escape; at the boundary, after the slice's commit, a tentatively escaped
value whose numbered-holder count is 0 and which no anonymous object or WAIF
image holds is still confined. This is sound because no other task can
commit while the gate is held, and a transaction that read anything the
holder published must validate after the holder commits
(`db/store/store_txn.go:356-367` `mustValidateReads`/`overlapsHold`), so a
reader of `h.ref` fails validation and re-runs. Publications without the
gate (direct transactions) remain permanent escapes. `readPublishedAfter`
(`store_txn.go:371-400`) checks scalar, relationship, property-scan,
shape-scan and verb-scan reads, property-value reads, map-key reads, verb
reads and WAIF image bases, so a read of `h.ref` is covered. Not covered by
this argument: a private copy of an anonymous object taken by another
transaction (anonymous objects are mutated in place, no history); C2 must
therefore treat a publication into an anonymous object's property as a
permanent escape, as plain C does.

### 5. Evidence: how Mongoose's released values classify (census)

Throwaway patch, uncommitted, in detached worktree
`C:/Users/Q/code/barn-finalization-measure` (origin/master base), saved as
`C:/Users/Q/AppData/Local/Temp/claude/655669a3d1b9c796/ea6e86d2-9d20-42c3-abeb-b77523e31b37/scratchpad/fincensus.patch` (session scratchpad, may be cleared). It adds `internal/fincensus`: creation stamps in
`new_waif()` and anonymous `create()`; escape marks at property writes
(numbered, anonymous, WAIF, map keys), fork, `resume`, `set_connection_option`,
`task_stack`/`queued_tasks`/`finished_tasks` results and server-hook args;
a numbered-holder count kept in `refreshDirectRefs`; a classification of every
slice's released values (the slice's `PendingWaifs`, new anonymous ledger
entries, anonymous objects the task created) after the slice's commit in
`handOffSuspended` and in `settleCompletedTaskFinalizations`; and an oracle
that compares every fast "dead" verdict with the deferred sweep's verdict
when the sweep later processes that value (`flushDeferredGCBarrier` WAIF
loop, `RecycleOrphanAnonymousBatch` reachable set).

Run: `go test ./engine -run 'TestMongooseRealWorkload$'` on Windows
(go1.27.1), `BARN_MONGOOSE_BENCH=1 BARN_FINCENSUS=1
BARN_MONGOOSE_DB=C:/Users/Q/code/barn/mongoose.db.new` (260,031,588 bytes,
sha256 prefix abf324f0a74123fb; the 2026-11-12 fixture used for #1040 was
not found on this machine), `BARN_MONGOOSE_SOUND_DB=
C:/Users/Q/code/files/sqlite/sound.sqlite`, players 1 and 16, warmup 5 s,
measure 30 s, `BARN_MONGOOSE_GOGC=400`. Counters cover the measure window
only. Load is the WSL 1-minute load average on the same 8 CPUs.

Runs (all on the same fixture and command):
- run 1: suspends classified before their commit (wrong point: a property
  overwrite staged in the slice still counted its old holder). Load 15.9.
- run 2: suspends classified after the commit. Load 18.1 before, 23.9
  after.
- run 3 (16p only) and run 5 (16p only): labels on "unknown" samples.
- run 4: run 2 plus the WAIF property write through `ReplaceWaifProperty`
  (`vm/op_property.go:448`) marked as an escape; runs 1-3 missed that path.
  Load 28.7 before, 17.6 after. Run 4 is the result below; runs 1-3 gave
  the same shape (decided suspend slices 99.80-99.84 % at 1p, 100 % at 16p;
  0 oracle disagreements in each).

Run 4, 1 player (3,110 commands; goodput of this instrumented build 104/s,
not a performance measurement):

| | count | share |
|---|---|---|
| released WAIFs | 130,743 | |
| held directly by a numbered object (alive, no work) | 126,585 | 96.8 % |
| confined, dead (fast recycle) | 2,269 | 1.7 % |
| confined, still held by own VM (alive) | 1,843 | 1.4 % |
| unknown (sweep) | 46 | 0.035 % |
| released anonymous objects | 3,110 | all numbered-held |
| anonymous objects created | 0 | |
| suspend slices with a release | 11,126 of 11,659 | |
| ... all released values decided | 11,110 | 99.86 % |
| completed slices with a release | 8,564 of 14,914 | |
| ... all released values decided | 8,540 | 99.72 % |
| oracle: fast dead, sweep agrees | 2,314 | |
| oracle: fast dead, sweep says live | 0 | |

Run 4, 16 players (11,768 commands; instrumented goodput 379/s):

| | count | share |
|---|---|---|
| released WAIFs | 179,992 | |
| numbered-held | 162,760 | 90.4 % |
| confined, dead | 7,472 | 4.2 % |
| confined, own VM holds | 9,731 | 5.4 % |
| unknown | 29 | 0.016 % |
| released anonymous objects | 1,276 | all numbered-held |
| suspend slices with a release, all decided | 18,688 of 18,688 | 100 % |
| completed slices with a release, all decided | 32,774 of 32,799 | 99.92 % |
| oracle agree / disagree | 7,489 / 0 | |

Escapes seen in the window: only property writes to numbered objects (545
at 1p, 208 at 16p; 196 and 208 of them WAIFs a task had created). No fork,
`resume`, connection-option, introspection, hook or WAIF-property escape of
a task-created WAIF or anonymous object occurred.

What the unknowns are (run 5 samples, 30 of 30 labelled): 29 are WAIFs made
by `#851:new` in command tasks, stored in a numbered object's property
(an escape), then taken out and dropped by a forked `#2478:schedule_flush`
task when it completes; 1 is a WAIF made by `#3565:new` during
`server_started` and dropped by `#606:_handlers`. These are genuinely shared
values (the property was readable by every task), so no confinement test can
decide them; design B could. They are dropped by a completing forked task,
not by a command that suspends.

The census also counted, per sweep, WAIFs the sweep found dead whose last
fast verdict was something else: 1,903 (1p) and 9,652 (16p) were
`confined_dead` (the same WAIF appears in several pending entries; the first
entry consumed the oracle check), 73 and 61 were `unknown`.

Census cost: 100 us (1p) and 54 us (16p) per classified slice in run 4
(70-74 us in run 2), measured around the whole census call (global mutex, Go
maps, own-VM walk on every slice). This is an upper bound for the
instrument, not the design: the design keeps the creator and escape bits on
the WAIF rep / anonymous object, decides numbered-held values with one
counter read, and walks the own VM only on slices that released a confined
value (4,112 of 130,743 releases at 1p).

Not covered by this evidence: anonymous objects. This fixture and harness
created none in the window, so the anonymous fast path has no real-data
measurement; only the conformance and Go tests exercise it. The July note's
"the world creates anon objects every task" (`#502:new`) does not hold for
this fixture's command mix.

The two #1083 scenarios (`TestFincensusScenarios` in the measurement
worktree, statements copied from #1040's tests, `go test ./engine -run
'TestFincensusScenarios$'`): WAIF scenario `confined_dead_waif:1`,
`oracle_agree_waif:1`, result 1; anonymous scenario
`unknown_own_escaped_anon:1` with `escapes=prop_of_numbered_anon:1` and
`numbered_held_keys=0` at the boundary, result 1 (the unthrottled sweep ran
in time). The anonymous scenario is the case C2 exists for (section 4).

### 6. Toast's order against other ready tasks, and Barn's

Toast (section 1): the recycle verbs queued by one task slice run before any
later task slice, the releasing task's resume and every other task alike.
Barn today: neither; the releasing task and every other task can run first.
With design C2 (or B) Barn runs the recycle verbs on the releasing task's
goroutine before that task is handed back, so the releasing task's own
resume (and, on completion, its terminal state, which the next command of
the same player waits on in the harness and server) is ordered after them.
Other tasks that are already running, or that start on other goroutines
between the slice's commit and the recycle verbs' commits, are not ordered;
they can observe the state before the recycle. Matching Toast for them
needs every new slice start to wait while any recycle verb runs (a global
pause per release); open decision D3.

### 7. Recommendation

Build C2 (section 4): creator stamp, escape bit set at publication with the
exclusive-gate refinement, numbered-holder count in `refreshDirectRefs`,
decision after the slice commit, recycle verbs run inline before the task is
handed back, everything else to the existing batched sweep. On the measured
Mongoose workload it decides every released value without a sweep in 99.86 %
(1p) and 100 % (16p) of suspending slices, its "dead" verdicts agreed with
the full sweep 9,803 times out of 9,803 (run 4). The #1083 WAIF scenario is
confined (census verdict); the anonymous scenario is confined under C2's
rule by the census facts (its only escape was into `h.ref`, numbered-holder
count 0 at the boundary), which is reasoning, not a run of C2. It needs no other VM to park and adds no wait to the
scheduler, so the #1040 failure modes (15-20x goodput loss, Elle's
"unsettled task ... queued") do not arise. Do not build A. Keep B as the
follow-up if the owner wants the residual (shared values dropped by other
tasks, about 1 per 70-400 commands here) ordered too.

Also needed, independent of the design (each a Barn bug against Toast, to
be confirmed Toast-first before any change): the release sites that do not
record today (section 2: `OP_POP`, discarded verb return, indexed
assignment, except variable, stack truncation); the "NOT roots today"
holders of the current sweep (section 2), which the sweep can recycle under
a live holder.

### 8. Test plan

1. The four tests of #1040 (`engine/suspend_finalization_order_test.go` on
   `origin/fix/anonymous-recycle-after-suspend-flake`): unchanged, must fail
   on master and pass with C2 (they throttle the sweep or keep a sibling VM
   running, so they pass only if the fast path decides without the sweep).
2. Go tests per escape channel: a task-created WAIF and anonymous object
   handed through each channel in section 4 (fork, `resume`,
   `set_connection_option`, `task_stack`, `queued_tasks`,
   `finished_tasks`, hook args, move lifecycle, threaded builtin, property of
   a numbered object, anonymous object, WAIF) must not be recycled while the
   other holder has it; and the C2 case (published under the gate, holder
   gone at commit) must be recycled before resume.
3. The census oracle as a build-tagged assertion (every fast "dead" verdict
   re-checked by the next full sweep; a disagreement fails the run), on in
   items 4-6.
4. Managed conformance (Toast-first, `capability_admission` in the
   selection): a loop of
   `recycle_keeps_shared_anonymous::anonymous_held_only_by_recycled_object_is_destroyed`
   and `audit/divergence_waif_statement_semantics.yaml::waif_recycle_verb_called_on_release`
   under load (record load per run), then the full suite.
5. Elle isolation (`scripts/check_elle.py`, seeds 412/395/396 and the CI
   set) and the Jepsen checkpoint workflow, with raw histories kept.
6. Real-workload gate: TestMongooseRealWorkload at 1 and 16 players, five
   interleaved master/branch pairs, load recorded per run, goodput and p50/p99
   not worse than master; plus `go test -race ./db/store ./engine`.
7. Toast-first conformance tests for the unrecorded release sites before
   changing them (e.g. a bare `new_waif();` statement, `x[1] = 0` over a
   WAIF element, then `suspend(0)` and read the recycle log).

### 9. Open decisions (the owner's)

- D1. C2 now, or B directly? For C2: it decides 99.86-100 % of suspending
  slices measured, touches about a dozen escape sites and one counter in
  `refreshDirectRefs`, and adds no invariant to the commit path. For B: it
  also decides shared values whose last holder another task dropped (the
  residual: 46 per 3,110 commands at 1p, 29 per 11,768 at 16p; 29 of 30
  sampled at 16p were dropped by a completing forked task), at the cost of per-boundary registration of
  every task's held values and an ordering rule between registration and
  commit; it relies on read validation for in-flight readers, which Elle
  checks for property registers but not for anonymous-object or WAIF reads.
- D2. What happens to an "unknown" value released by a suspending slice?
  (a) today's deferral, unordered: 16 of 11,126 suspending slices at 1p and
  0 of 18,688 at 16p had one; (b) hold only those slices for a sweep, the
  #1040 mechanism restricted to them: each hold waits for a full sweep and
  for every running VM to park, with no fixed bound (#1040 note); (c) B.
- D3. Order against other tasks (section 6): leave other running tasks
  unordered (Barn's concurrency, no pause), or pause new slice starts while
  recycle verbs run to match Toast exactly? Evidence for leaving it: no
  conformance test observes it, and on Mongoose 2,269 (1p) to 7,472 (16p)
  fast recycles per 30 s would each pause every player. Evidence for
  matching: Toast guarantees it (`server.cc:878-883`, `tasks.cc:1629-1823`).
- D4. Recycle verbs of a completing command run before its completion is
  delivered (needed for the same player's next command to see them). That
  moves their run time into the command's latency (today it is spent in the
  sweep later). Accept, or deliver completion first and order only resumes?
  No measurement yet of the recycle verbs' own cost on Mongoose; the
  implementation's real-workload gate measures it.
