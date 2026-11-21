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

## Implementation (C2)

Owner decisions (2026-11-21, via the lead): D1 build C2; D2 unknown values
keep today's batched sweep with no hold (build so a hold could be added
later, do not add one); D3 other tasks stay unordered against a task's
recycle verbs (known difference from Toast); D4 recycle verbs of a completing
command run before its completion is delivered.

### State

All six steps done; not pushed (the lead's instruction). The PR body is at the
end of this note.

- Step 1 done: the four #1040 tests (`engine/suspend_finalization_order_test.go`,
  unchanged) fail on master (`k.hits ... = 0, want 1`, all four).
- Step 2 done: the mechanism; the four tests pass (30 runs each), and
  `go test ./...` passes.
- Step 3 done: `engine/finalization_escape_channels_test.go`, one test per
  reachable channel, each for a task-created WAIF and anonymous object, with
  the sweep throttled so only the boundary decision could recycle in the
  window: numbered property (plain, under the slice's grant, and permanent
  after the holder lets go), tentative from an earlier slice, anonymous
  property, WAIF property, fork, resume, task_stack, queued_tasks (Barn
  extensions off so the finished-tasks record does not also hold it), the
  finished-tasks record, set_connection_option, hook arguments (tag level:
  `RunServerVerbTaskWithArgstr` with a stamped WAIF); positive controls: the
  C2 case (published under the grant, holder recycled in the slice) recycled
  before resume, a plain confined value recycled at suspend and before
  completion is delivered. Each channel test was checked by removing its
  mark (`mutcheck.py`, outside the repo): every one then fails.
- Step 4 done: the oracle, `-tags finalizationoracle`
  (`engine/finalization_oracle_on.go`; the default build compiles the no-op
  `finalization_oracle_off.go`). Every fast "dead" verdict (whose recycle verb
  did not suspend) is recorded and keeps a sweep due; the next sweep, after
  capturing its roots, checks each against them (WAIFs: `liveWaifs` over the
  captured roots; anonymous objects: `Store.ReferencedAnonymousIDs`, which
  keeps ids of recycled objects) and panics on a disagreement. Tests:
  `engine/finalization_oracle_test.go` (catches a held WAIF; agrees on
  confined values). `TestMongooseRealWorkload` logs the counts per window.
  Build the server with it: `go build -tags finalizationoracle -o bin/ ./cmd/barn`.
  `go test -tags finalizationoracle ./engine ./builtins ./vm` passes.

Where the mechanism lives:
- `types/finalizer_tag.go`: `FinalizerTag` (creator task id; state confined /
  tentative under grant N / escaped; numbered-holder count while tentative).
  A WAIF carries it in its rep (`types/waif.go`); anonymous objects in
  `Store.anonTags`, dropped when the object is recycled
  (`db/store/store_lifecycle.go` Recycle).
- `db/store/store_reachability.go` `refreshDirectRefsPublishing` /
  `noteHolderChangeLocked`: every reference a numbered holder's index entry
  gains is a publication (tentative when the commit holds an exclusive
  grant, otherwise escape), every one it loses decrements the tentative
  holder count. The commit path passes the grant
  (`db/store/store_cow.go` publishStagedLocked); every other caller of
  `refreshDirectRefs` passes none.
- `Store.exclusiveGrantSeq` numbers exclusive grants (`noteExclusiveGrant`);
  a bound transaction records it (`StoreTxn.grantSeq`, `BindExclusiveGrant`);
  `sliceGate.seq` holds it for the slice, and `commitGrantSeq` is read just
  before the gate is released after the slice's commit
  (`engine/task_runtime.go`).
- `engine/finalization_fast.go`: `decideConfined` (after the slice commit:
  released WAIFs, anonymous objects in the ledger and those the task created
  (`task.State.TakeCreatedAnonymous`); confined and not held by the task's
  own VM, call-stack snapshot or result = dead) and `recycleConfinedDead`
  (the sweep's own runners: `callVerbWithArgstr(..., vmOwnershipSweep)` for a
  WAIF after `BeginWaifRecycle`, `recycle()` under `gcRecycleContext` for an
  anonymous object).
- Wired into `handOffSuspended` (commit, decide, queue the rest, recycle, prune
  the ledger, save the VM, requeue) and the completion path of
  `runTaskSliceAdmitted` (decide in `settleCompletedTaskFinalizations`,
  recycle after the completion's own commit, before `SetVM(nil)` and before
  the lease is released and the completion callback runs).

### Decisions and why

- Everything the fast path does not find dead goes to the sweep exactly as
  before, including values a numbered object holds and confined values the
  task still holds. The design allowed dropping numbered-held values from the
  queue; I kept them because a holder that later lets go at a release site
  that records nothing (OP_POP and the others, out of scope) would then never
  be swept, which today's sweep does catch. Cost: the sweep runs as often as
  on master; the fast path only adds work.
- The numbered-holder count is kept only for tentative values of task-created
  values (the only use C2 has for it); loaded values carry no creator and
  cost one atomic load per reference in `refreshDirectRefs`.
- Escape into an anonymous object or a WAIF is marked at staging time
  (conservative: an aborted attempt still escapes the value), and again at
  commit for every property of an anonymous object the commit wrote or the
  inheritance walk joined. A commit whose inheritance walk joined descendants
  publishes nothing tentatively.
- A tentative mark is valid only for the grant it was made under: any later
  decision or publication under another grant (or none) makes it an escape.
  So a tentative value still held by a numbered object when its slice ended
  can never be called confined later.
- The creation floor request for anonymous objects (`deferAnonGC`) is still
  queued whenever it was before; the fast path recycles its dead ones first
  and the sweep finds them gone. A standalone hook's anonymous objects (no
  creator) still need that request.
- Anonymous recycle verbs and WAIF recycle verbs run on the releasing task's
  goroutine under its execution lease and admission scope, so no sweep can
  start meanwhile (`collectAllGCRefs` sees a running lease) and nothing waits
  on another VM.
- Hold for unknowns (D2) not built: `confinedVerdict.undecided` is the count a
  hold would test, at the one place each boundary queues its rest.

### Surprises

- Channels with no test because MOO code cannot reach them with a
  finalizable value: `move()` raises E_TYPE for a TYPE_ANON object (and a
  WAIF) before anything holds it; `listen()` rejects a non-OBJ object by its
  signature; `background_test()` takes a string; `sort()` of a list holding a
  WAIF raises E_TYPE. Their marks stay (cheap, conservative). all_members()
  and the sqlite builtins accept any value; not tested because of the next
  item.
- A threaded builtin's resumed task is sometimes never run by the engine
  test harness (`ProcessReadyTasks` loop): `r = sort({3, 1}); return r;`
  stays queued with its wake value set in 3 of 4 runs on origin/master too,
  so it is not this change. Not investigated further.
- A compiler temporary keeps the base of an indexed or property assignment
  (`w2.ref = v`) in a hidden local slot past the statement, so the task's own
  VM still "holds" `w2` (and, through it, `v`) at the next boundary. That
  only makes the fast path say alive (safe), but it hides garbage from it;
  the WAIF-property test makes its write inside a verb for that reason.

### Escape channels (file:line)

Each marks every WAIF and anonymous object the value holds directly (through
lists and maps; a value inside a WAIF or anonymous object's property was
marked when written there):
- Property of a numbered object: `db/store/store_reachability.go:159`
  `refreshDirectRefsPublishing` (publication time, tentative under a grant).
- Property of an anonymous object: `db/store/store_txn_properties.go:413`
  (SetPropertyValue), `:510` (DefineProperty),
  `db/store/store_txn_mapkeys.go:208` (SetPropertyMapKey), at commit
  `db/store/store_cow.go:559,566`; inherited slots of a new anonymous object
  `db/store/store_lifecycle.go:33`.
- Property of a WAIF: `db/store/store_txn_waif.go:97` (setWaifProperty, both
  SetWaifProperty and ReplaceWaifProperty).
- fork: `vm/control.go:183-185` (the child's `this` and every variable).
- resume: `builtins/tasks.go:192`.
- set_connection_option: `builtins/network.go:1380`.
- task_stack, queued_tasks, finished_tasks results: `builtins/tasks.go:725-730`
  (`escapingResult`); the finished-tasks record itself:
  `engine/lag_callback.go:79`.
- Hook arguments: `engine/task_factory.go:101` (runServerVerbTask: uncaught
  error, timeout, lagging task and other server hooks), `:213`
  (CreateLoginHookTask), `engine/call_verb.go:204-206` (every standalone VM:
  receiver and arguments).
- move: `builtins/objects_movement.go:31` (object and destination: location
  and contents name each other, and both reach the lifecycle verbs).
- Threaded builtins: `builtins/threaded.go:83` with the values each passes:
  sort `builtins/lists.go:327`, all_members `builtins/math.go:500`, sqlite
  parameters `builtins/sqlite.go:203`; background_test `builtins/system.go:880`.
- listen: `builtins/connection.go:253`.
- A task's result at completion is treated as held by the task (not dead):
  the Go caller of a hook keeps it.
- Not channels (read): `CallVerbInContext` (same task, synchronous VM);
  the WAIF registry used by property renames (rewrites images, hands no value
  out); anonymous children lists of a parent (not readable by MOO code).
- Out of scope, same blind spot as the sweep: an anonymous object reached by
  number (`toobj`, or an owner/location field read back as an object number).

### Gates

Load is the WSL 1-minute/5-minute/15-minute load average (`/proc/loadavg`) on
the 8-CPU workstation, which also runs the CI runners.

- `go test -race ./db/store ./engine`: pass (store 10.4 s, engine 461 s);
  load 5.05/10.19/13.02 before, 5.84/7.76/10.81 after. With
  `-tags finalizationoracle`: pass (store 11.8 s, engine 498 s); load
  6.33/7.80/10.79 before, 5.84/11.99/12.77 after.
- Elle, as `.github/workflows/elle.yml` runs it (test_check_elle.py,
  `TestElleRecorder`, `check_elle.py --seeds 412 395 396 --rounds 40`), on a
  copy of the branch head in WSL, checker built from the pinned elle-cli
  commit, Java 21: every healthy history (list-append and rw-register for
  each seed) valid, every fault case rejected as expected, lifecycle cases as
  expected; load 8.46 at start, 28.32 at end. Again with the oracle tag
  (`check_elle.py`'s `-tags elle` changed to `elle,finalizationoracle` in the
  copy only): same verdicts; load 28.32 at start, 11.13 at end. (Java 17,
  the WSL default, cannot run the pinned checker.)
- Managed conformance, suite main at its head (fresh clone in WSL, toastcore
  submodule initialised), Barn built with `-tags finalizationoracle`, the
  CLAUDE.md canonical invocation narrowed with `-k "capability_admission or
  (anonymous_held_only_by_recycled_object_is_destroyed or
  waif_recycle_verb_called_on_release)"`:
  - Toast first (`make conformance-toast`, same selection): 3 passed.
  - Barn, 20 runs at the workstation's own load (7.2-10.6): 3 passed each.
  - Barn, 20 runs with 12 extra CPU-bound shell loops (load 16.1-40.1):
    3 passed each.
- Full managed suite, oracle build, after merging origin/master (an earlier
  run on the pre-merge head failed
  `server/task_timeout_traceback_lines.yaml::forked_tick_abort_reports_ticks_and_current_lines`,
  which passes on Toast and on master: the suite's newest test needs master's
  #1039, which the branch had not merged yet): 13854 passed, 327 skipped,
  0 failed; load 16.55 at start, 13.52 at end.
- Lint: `gofmt -l` clean, `go vet ./...` (and `-tags finalizationoracle
  ./engine`) clean, `staticcheck ./...` clean after one tagged-switch fix in
  `types/finalizer_tag.go`, `golangci-lint run ./...` 0 issues.
- Real workload (`TestMongooseRealWorkload`, Windows, go1.27.1,
  `BARN_MONGOOSE_DB=C:/Users/Q/code/barn/mongoose.db.new`, sound catalog
  `C:/Users/Q/code/files/sqlite/sound.sqlite`, players 1 and 16, warmup 5 s,
  measure 30 s, `BARN_MONGOOSE_GOGC=400`), precompiled test binaries from
  detached worktrees of origin/master and of the branch head (origin/master
  plus this work), five rounds each running master, branch, and branch with
  `-tags finalizationoracle` in turn. Load is Windows CPU % and the WSL
  1-minute load average before each run; raw logs in the session scratchpad
  `perf/` folder. 30 s windows, so the spread is wide.

| round | load before (cpu, wsl 1m) | arm | 1p goodput | 1p p50 / p99 (ms) | 16p goodput | 16p p50 / p99 (ms) |
|---|---|---|---|---|---|---|
| 1 | 52 %, 18.1 | master | 56 | 8.74 / 225.5 | 383 | 13.81 / 381.1 |
| 1 | 46 %, 17.7 | branch | 67 | 7.64 / 157.0 | 306 | 15.44 / 512.2 |
| 1 | 58 %, 16.4 | oracle | 71 | 7.17 / 147.6 | 308 | 15.04 / 588.2 |
| 2 | 46 %, 17.5 | master | 85 | 6.43 / 125.8 | 366 | 13.32 / 413.4 |
| 2 | 48 %, 12.0 | branch | 78 | 7.08 / 131.3 | 389 | 14.19 / 423.3 |
| 2 | 49 %, 16.8 | oracle | 77 | 7.03 / 128.1 | 309 | 15.66 / 524.5 |
| 3 | 47 %, 14.3 | master | 89 | 6.30 / 54.5 | 376 | 13.31 / 463.0 |
| 3 | 51 %, 11.9 | branch | 85 | 6.33 / 109.1 | 364 | 12.88 / 376.2 |
| 3 | 34 %, 17.4 | oracle | 96 | 5.79 / 99.4 | 316 | 14.30 / 537.1 |
| 4 | 41 %, 9.2 | master | 76 | 7.29 / 119.2 | 329 | 14.83 / 495.5 |
| 4 | 73 %, 17.2 | branch | 74 | 6.96 / 135.4 | 337 | 13.67 / 542.3 |
| 4 | 74 %, 24.4 | oracle | 62 | 8.12 / 173.6 | 293 | 16.62 / 548.5 |
| 5 | 45 %, 32.4 | master | 70 | 7.24 / 142.0 | 293 | 15.58 / 662.1 |
| 5 | 48 %, 21.2 | branch | 68 | 7.64 / 152.7 | 308 | 15.58 / 496.7 |
| 5 | 44 %, 20.3 | oracle | 75 | 7.07 / 140.2 | 313 | 14.40 / 559.4 |

  Medians, master vs branch: 1p goodput 76 vs 74/s (pairwise branch minus
  master +11, -7, -4, -2, -2; master's own range 56-89), p50 7.24 vs
  7.08 ms, p99 125.8 vs 135.4 ms; 16p goodput 366 vs 337/s (pairwise -77,
  +23, -12, +8, +15, median +8; master's range 293-383), p50 13.81 vs
  14.19 ms, p99 463.0 vs 496.7 ms (pairwise +131, +10, -87, +47, -165,
  median +10). Every difference is inside the spread of master's own runs;
  no failed or uncaught command in any run. The oracle build is slower at
  16p (it keeps a sweep due after every verdict); it is a test build only.
- Oracle on the real workload: 0 disagreements in all five runs. Its
  counters are per process and cumulative (the 16p line includes the 1p
  window and both warmups): recorded / checked at the end of each run
  9376 / 9361, 9546 / 9492, 9967 / 9822, 8773 / 8748, 9627 / 9627 (47,289
  fast "dead" verdicts, 47,050 re-checked by a later sweep; the rest came
  after the run's last sweep). At the end of the 1p window alone: 2120 /
  2014, 2169 / 2169, 2576 / 2563, 1836 / 1786, 2120 / 2088.
- Not run: the Jepsen checkpoint workflow (design section 8 item 5 lists it;
  the lead's gate list does not).

### Next step

The lead pushes the branch and replaces PR 1092's body with the one below (or
opens the PR), and decides D2's hold for unknown values. Separate issues named
in this note, untouched here: the release sites that record nothing, the
sweep's missing roots, compiler temporaries that keep an assignment's base
alive, anonymous objects reachable by number, and the engine test harness not
running a resumed threaded-builtin task.

### PR body

Fixes #1083

A WAIF or anonymous object whose last reference a task slice drops now has its
`:recycle` verb run before that task resumes, and before a completing
command's completion is delivered, when only that task could have held it.
ToastStunt runs every such recycle before the next task slice (`server.cc`
main loop: `recycle_anonymous_objects(); recycle_waifs();` before
`run_ready_tasks()`). Barn left them to the batched sweep, which runs later and
skips while sweeps are expensive or other VMs are running, so
`recycle_keeps_shared_anonymous::anonymous_held_only_by_recycled_object_is_destroyed`
and `waif_recycle_verb_called_on_release` failed intermittently.

How (design C2 in `agent-notes/fix-finalization-order-released-cost.md`):
- Each WAIF and anonymous object records its creating task and whether it may
  be held elsewhere (`types.FinalizerTag`). Every channel that hands a value to
  another holder marks it: publication to a numbered object's property
  (`refreshDirectRefs`), writes into an anonymous object or a WAIF, fork,
  `resume`, `set_connection_option`, `task_stack` / `queued_tasks` /
  `finished_tasks` and the finished-tasks record, server hook arguments and
  standalone verb calls, `move`, threaded builtins, `listen`.
- A publication made under the slice's own exclusive commit grant is
  tentative: if no numbered object holds the value after the slice commits, no
  other task can have kept it (other commits were excluded, and readers of the
  publication must validate after it).
- After a slice commits, released values confined to the task and not held by
  its own VM are recycled on the task's goroutine, each in its own transaction
  as the sweep does it. Everything else goes to the sweep unchanged. No other
  VM has to park and nothing waits in the scheduler.

Known difference from Toast: other tasks that are already running are not
ordered against a task's recycle verbs; they can run while those verbs run.
Toast's single-threaded loop runs the verbs before any other slice. Values the
boundary cannot decide (held inside another WAIF or anonymous object, or
shared through a property and dropped by another task) still go to the sweep,
unordered.

Tests: the four tests of #1040 (fail on master, pass here); one Go test per
reachable escape channel for a WAIF and an anonymous object, each shown to fail
when its channel's mark is removed; an oracle (`-tags finalizationoracle`) that
re-checks every boundary recycle against the next full sweep and fails the run
on a disagreement.

Gates (load = WSL load average on the 8-CPU workstation):
- `go test -race ./db/store ./engine`: pass, also with the oracle tag.
- Elle as `elle.yml` runs it, seeds 412 395 396: every healthy history valid,
  every fault case rejected; the same with the oracle tag.
- Managed conformance: the two tests on Toast first (pass), then on Barn with
  the oracle 20 times at load 7-11 and 20 times at load 16-40: all pass. Full
  suite with the oracle: 13854 passed, 327 skipped, 0 failed.
- `TestMongooseRealWorkload`, five interleaved rounds (30 s windows, load
  9-32):

| | master median | branch median | pairwise branch - master |
|---|---|---|---|
| 1p goodput | 76/s | 74/s | +11, -7, -4, -2, -2 |
| 1p p50 / p99 | 7.24 / 125.8 ms | 7.08 / 135.4 ms | |
| 16p goodput | 366/s | 337/s | -77, +23, -12, +8, +15 |
| 16p p50 / p99 | 13.81 / 463.0 ms | 14.19 / 496.7 ms | p99: +131, +10, -87, +47, -165 |

  All within the spread of master's own runs (1p 56-89/s, 16p 293-383/s).
  With the oracle on: 0 disagreements over 47,050 re-checked verdicts.
- gofmt, go vet, staticcheck, golangci-lint clean.
