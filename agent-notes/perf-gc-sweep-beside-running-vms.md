# perf/gc-sweep-beside-running-vms

## Goal
Deferred GC rendezvous no longer pauses admission (which froze every player for
the length of the longest running slice, 3-5 s on Mongoose `say`). Instead the
sweep asks running VMs to park at their existing tick-checkpoint `preempt` hook,
walks the parked VMs (plus their txn's staged property writes) as roots, seeds
reachability from retained object history, sweeps, then releases them.
Design from team-lead brief (approved by Q), steps 1-5.

## State
- Implemented and committed locally on this branch (not pushed, no PR):
  - vm.Preempt takes the VM (`func(*VM)`), so the hook can publish it.
  - engine/gc_handshake.go: parkForGC, requestGCHandshake, awaitGCParked,
    leasesParkedLocked, collectParkedRoots, addRetainedHistoryRoots.
  - preempt closure (engine/task_runtime.go) parks unless the slice holds the
    exclusive commit gate, before the admission-yield check.
  - collectAllGCRefs fails closed unless every lease is parked; walks parked
    VMs and their staged writes, skips them in the task snapshot loop.
  - flushDeferredGCBarrier(patience, handshake): SweepMu, handshake + wait,
    VMStartMu, wait again for any VM that started in between, then as before;
    history roots added after the root walk.
  - rendezvousDeferredGC: no admission.Pause; same RetryAfter on failure.
  - store: StoreTxn.VisitStagedPropertyValues, Store.VisitHistoryPropertyValues.
- Guards in engine/gc_sweep_beside_test.go: (a) failed on unchanged master
  (sweep never ran beside the spinner); (b), (c), (d) each failed with only
  its own piece disabled; all four pass. Store unit tests in
  db/store/gc_root_visit_test.go.
- Updated pinning tests: TestDeferredGCMaintenanceCancellationReleasesAdmission
  (now: a never-parking lease, maintenance stops on cancel, handshake not left
  requested, admission open) and TestDeferredGCProgressDrainsOverlappingExecution
  (now: input and background admission keep flowing; no sweep while builtin-
  blocked VMs hold leases; settles after they finish).
- engine, db/store, internal/admission, vm tests pass; -race engine and
  db/store pass; vet and gofmt clean on changed packages.

## Decisions and why
- Parking happens for borrowed-scope slices too (design: every running VM
  parks except gate holders); only the admission yield stays owner-only.
- A lease parks only as a root slice VM; ExecutingTasks count must be exactly 1
  for that task, otherwise the round fails closed (nested inherited leases mean
  an outer VM is mid-builtin).
- Releaser clears the parked map before closing the release channel, so a
  later handshake never sees a stale parked entry.
- History seeding applies to every deferred sweep (opportunistic too), as the
  design says "the deferred sweep".

## Surprises / open questions (sent to team-lead, unanswered at commit time)
1. A parked txn caches deep clones of anonymous objects it read
   (db/store/store_txn_snapshot.go:140-141); anon objects are mutated in place
   with no history, so values only in that clone are invisible to the
   staged-write walk and history seeding. Proposed: also walk anonymous
   objects in tx.objects.
2. History seeding counts versions retained only for suspended tasks' idle
   snapshot txns (task_runtime.go:497-498, 748-749). A waif judged live is
   dropped from the batch (waif_lifecycle.go `continue`) and never requeued,
   so its :recycle can be lost. Proposed: seed only versions a parked txn's
   readTS can be served.

## Next step
- Apply team-lead's answers to the two questions (each needs a guard test).
- Then push/PR when told to.
