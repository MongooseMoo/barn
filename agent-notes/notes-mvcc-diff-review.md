# MVCC diff review — session state (2026-07-21)

## Task
Strict code review of `git diff 41b904c...HEAD` (branch work/mvcc-concurrent-moo, ~12.5k added lines).
Deliverable: concrete, triggerable bugs only, with file:line, trigger scenario, and why tests miss it.
Focus: MVCC correctness, scheduler retries/side effects, concurrency. No file modifications.

## Observed so far
- Diff scope: db/store (store_txn.go ~2034 new lines, store_cow.go, store_history_gc.go,
  store_core/lifecycle/properties/relationships/verbs changes), scheduler (scheduler.go,
  task_runtime.go, access_footprint.go, call_verb.go, eval.go, waif_lifecycle.go),
  vm (op_property.go, op_verb.go, anonymous_gc.go), builtins (many), server/input_processor.go,
  kernel/context.go. Plus large new test files (store_txn_test.go 1739 lines, worker_pool_test.go,
  concurrency_*_test.go).
- Read plans/mvcc-concurrent-moo-plan.md. Key invariants to review against:
  lazy runtime read-set capture; staged write sets validated at commit; effects (output, forks,
  state transitions) published only after commit; abort/retry only before irreversible effects;
  per-connection command/read ordering preserved; serialized fallback for unsafe ops.
- Relevant commit subjects on branch: "Batch only commuting ready tasks", "Avoid retry after live
  store mutations", "Defer server option cache publishes", "Flush yin side effects after commit",
  "Co-schedule conflict-retryable tasks in parallel", "Prune object history below the live-read
  floor (COW phase 4)", "Publish committed objects copy-on-write (COW phase 0)".

## Approach
Dispatched 4 parallel analyst subagents (background), one per slice:
1. txn-reviewer — store_txn.go + snapshot/core interaction (visibility, conflict detection, commit atomicity, lock discipline).
2. cow-reviewer — store_cow.go, store_history_gc.go, decentralized commit hunks (COW aliasing, publication ordering, history-GC floor).
3. sched-reviewer — scheduler/* + server/input_processor.go + task/kernel (retry side-effect duplication, commuting-footprint soundness, ordering, deferred-effect flush).
4. vm-reviewer — vm/* + builtins/* (mixed snapshot/live reads, missing txn routing, read-your-writes, anon GC races).

Each was told to verify by reading full code paths and report file:line + trigger + test-gap; no speculation.

## State
- txn-reviewer and vm-reviewer reports received. cow-reviewer and sched-reviewer still running.
- I have personally verified (read the cited code myself) these candidates so far:

### VERIFIED (txn slice)
1. commitDecentralized (store_cow.go:344-374) locks only WRITE slots; read-set validation via
   s.load() on unlocked slots; publication later. Two commits with crossing read/write sets can
   both validate then both publish → write skew / stale-read commit. Coarse path validates under
   exclusive Lock, decentralized does not.
2. COW image builders (buildImageWithScalar store_cow.go:84 etc.) share properties/verbs/propOrder
   with old image; rememberOldImageLocked stores old image w/o clone; coarse path then mutates
   live.properties in place (store_txn.go:1655, 1675, 1692-94) → corrupts history snapshot that
   shares the map → old-readTS reader sees new data. Verified map sharing + in-place writes.
3. adoptLiveReadsLocked (store_txn.go:1711-64) re-baselines ENTIRE read set to live on liveMutated
   commit — swallows OTHER tasks' concurrent commits → lost update. Code confirmed; concurrency
   reachability depends on scheduler co-scheduling (await sched-reviewer).
4. Coarse apply loops return mid-iteration (store_txn.go:1587-88, 1607-08, 1624-26, 1685-86) after
   earlier map-iteration writes already applied+stamped; no rollback → permanent partial commit.
   (Decentralized path pre-validates all targets at store_cow.go:380-410; coarse does not.)
5. BeginReadOnly (store_txn.go:124-132) loads clock then registerReadTS under RLock only; the
   decentralized pruner also runs under RLock with floor=clock when registry empty
   (store_history_gc.go:78-80). Window: reader loads clock, preempted; 2 commits prune the entry
   it needs; reader then gets nil for a live object (E_INVIND). historyFloor's comment claims
   exclusion that doesn't exist (RLock vs RLock).
6. Anon objects: no history, coarse in-place stamp (store_txn.go:1583-1603); objectLocked:204-207
   returns nil if anon version > readTS → concurrent commit makes live anon invisible (E_INVIND)
   to older-readTS txns. Also nil object reads record NO read-set entry → unvalidatable.
7. Store.Snapshot (store_snapshot.go:39-91) under RLock only; decentralized commits publish under
   RLock → torn checkpoint: slot loaded twice (lines 67, 77) can yield different images →
   Objects vs PropertyNames misalignment → dump propval shift; multi-object commit half-captured.
8. Delete-then-redefine same prop in one txn: DefineProperty (store_txn.go:1311-13) clears
   propertyDeletes but NOT propertyDefinitionDeletes; DeleteDefinedProperty earlier staged it
   (1450-57). Both maps at commit → decentralized pre-check E_INVARG (store_cow.go:397-402),
   coarse: definePropertyLocked E_INVARG mid-apply → whole-task failure or partial commit.
9. stageVerbCode keys verbWrites by verb.name (store_txn.go:1862-68); builder.AppendVerb indexes
   verbs[names[0]] last-wins with dup names in verbList (builder.go:68-74); SetVerbCodeByIndex on
   a dup-named verb programs the WRONG verb at commit / collapses two writes to one key.

### VERIFIED (vm slice)
10. delete_verb (builtins/verbs.go:533-537) does live DeleteVerb + markLiveStoreMutated but NO
    AdoptLiveVerbs — unlike add_verb (:485-490) and set_verb_info (:602-607). Task still sees and
    can call the deleted verb via snapshot for rest of task.
11. AdoptLiveVerbs re-apply loop (store_txn.go:279-289) skips (continue) staged verbWrites whose
    name is gone but LEAVES the stale entry; coarse commit then E_VERBNFs the whole txn
    (store_txn.go:1684-86) → set_verb_code+rename or set_verb_code+delete_verb in one task fails
    entire commit (legal on Toast), possibly partially applied (see #4).
12. pass() fallback (vm/op_verb.go:348-357) mixes snapshot pre-check with live-store Parent read →
    error class (E_INVIND vs E_VERBNF) depends on concurrent commit timing. Verified by agent;
    I read the excerpt, plausible; lower severity.
13. (agent-verified, I spot-checked claims 1-2 myself) set_verb_info/set_verb_args/chparent:
    perm check on snapshot verb, write applied to live by name; recycle/renumber adopt-set from
    snapshot but live rewires more; anon GC inline sweep (scheduler/eval.go:188-196) can recycle
    an anon created by a concurrently running task (create() is immediate live mutation now).
    These need my verification pass or corroboration from sched-reviewer.

### VERIFIED (cow slice — agent reproduced 4/5 with probes; I re-read the code paths)
14. (= #2, reproduced) decentralized publish + later coarse in-place write corrupts pinned
    snapshots through shared maps; probe: reader saw p=42 want 1; verb code variant too.
15. NEW, reproduced: coarse commit mixing scalar write + property define on one object —
    scalar loop stamps ts, then definePropertyLocked's internal rememberObjectLocked
    (store_properties.go:490) appends a MID-COMMIT clone at ts (scalar applied, define missing);
    ts-dedup in rememberObjectLocked (store_core.go:195-197) then blocks the correct at-ts image
    forever → committed write invisible at readTS >= its own commit ts. I verified the dedup line.
16. (= #8, reproduced) delete-then-redefine loses whole txn with E_INVARG.
17. NEW, reproduced: raw setters remember ANON objects into history (rememberObjectLocked has no
    !anonymous guard, store_core.go:189+); objectLocked walks history BEFORE anonObjects fallback
    (store_txn.go:189-206) → once an anon has any history entry every reader gets the stale clone
    forever (probe: fresh reader saw name="" want "after"). Recycled anons stay visible similarly.
18. (= #5, corroborated structurally by second agent) BeginReadOnly/pruner floor race.

## Next step
- Await sched-reviewer report; verify its claims; corroborate reachability of #3 (adoptLiveReads
  lost update) and vm #13 (anon GC inline sweep vs running task). Then dedupe, rank, deliver
  final vetted list. No file modifications.

## Blockers
None.
