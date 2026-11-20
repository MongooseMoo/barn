# Fuzz test temp files on tmpfs (#801)

## Goal
#801: FuzzCheckpointResumeEquivalence workers died "exit status 2" five
times (2026-11-12..14). Q 2026-11-15: "The hang that keeps coming back seems
rather important".

## State
- Cause, caught by #856's crash capture in a local WSL campaign (CI's
  retained corpus, -parallel=8): Go's worker wraps each input in
  `time.AfterFunc(60s, panic("deadlocked!"))` (internal/fuzz/worker.go:492).
  The stuck input was in fsync in db/format/checkpoint_publication.go:226,
  via checkpointSuspendedVM (vm/checkpoint_resume_test.go:391). The property
  writes a checkpoint at every suspend(); each checkpoint copies its file and
  fsyncs data, journal and five directories.
- The saved input (6c0ec4f2339829b1) passes alone in 1.01 s. 20 runs on an
  idle machine: 4.86 s with TMPDIR on disk, 1.14 s on /dev/shm.
- Fix: fuzz.yml sets TMPDIR=/dev/shm/barn-fuzz-<run>-<target> (fsync is free
  on tmpfs), GOTMPDIR on disk for builds, and the job's last always-step
  removes the tmpfs directory (a killed worker leaves its files there).

## Decisions and why
- Workflow env, not a no-sync option in db/format: no production surface,
  and it covers FuzzWorldCheckpointEffects's checkpoints too.
- tmpfs pages count against the listener's 12 GiB cgroup; each case's
  t.TempDir is removed when the case ends, and the property's checkpoint is
  bounded by checkpointValueBudget (64 MiB).

## Not covered here
- List `+` has no value limit on Toast or Barn (execute.cc:1492,
  vm/op_arith.go:85); `while n (l + E_NACC + n + n)` allocates 11.5 GB in
  one case of this lane, the guest's ~11.2 GB OOM kills (#799). Bounding it
  needs a Barn change or a VM hook: Q's decision.
