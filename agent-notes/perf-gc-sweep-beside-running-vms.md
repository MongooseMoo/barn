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
- "widen" (team-lead): the parked-root walk (StoreTxn.VisitPrivatePropertyValues)
  also visits every anonymous object cached in the parked txn's tx.objects.
  A txn deep-clones an anonymous object on read (store_txn_snapshot.go:33-34,
  140-141), and anon objects are mutated in place with no history, so a value
  the task can still read may live only in that clone. Guard (e).
- "parked readTS only" (team-lead): history seeding
  (StoreTxn.VisitSnapshotHistoryValues via addParkedHistoryRoots) visits, per
  object, only the newest history entry with ts <= a PARKED txn's readTS, and
  only when the live image is newer than that readTS. A waif judged live is
  dropped from the batch for good (waif_lifecycle.go `continue` after the
  batch is cleared), so seeding versions kept only for other readers would
  lose its :recycle. A sweep with no parked VMs seeds no history. Guard (f).

## Surprises
- My stated reason for "parked readTS only" was wrong: a suspended task does
  NOT hold a registered snapshot. runTaskSliceAdmitted's deferred cleanup
  releases the context txn when the slice returns (task_runtime.go:192-199);
  HistoryStats showed Readers:1 (only the spinner) with a task suspended. The
  rule still matters for any other non-parked reader that retains history
  (e.g. a checkpoint or store-level snapshot), so guard (f) uses a plain idle
  store snapshot instead of a suspended task.
- Guard (f) as first written (idle suspended task) passed under the mutation
  that seeds every retained version, which is how the wrong premise showed up.

## Next step
- Push/PR when told to.
