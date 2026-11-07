# test/generated-recovery-equivalence

## Goal

Add a generated property comparing an uninterrupted execution with one that is
checkpointed, written, reloaded and restored at each suspend boundary,
checking terminal value or error, persistent effects, and exception state.
Exposed as one property callable from a deterministic Go test over seeds and
as a native Go fuzz target. Unsupported states are explicit skips. A
perturbation test proves the property can fail with a precise witness.
Source: plan section 5 of `C:/Users/Q/code/barn/agent-notes/compute-bug-discovery.md`.

## State

Implemented and committed locally on branch test/generated-recovery-equivalence
(based on origin/master at the #656 merge). Not pushed; no PR. PR body draft:
`.tmp/pr-body.md`.

- `vm/checkpoint_resume_test.go`: `FuzzCheckpointResumeEquivalence` (package
  `./vm`), property core `checkpointResumeDifference`, shared helper
  `checkpointSuspendedVM`, MOO observer of persistent state, four pinned
  checked seeds with `TestCheckpointResumeSeedsReachCheckpoint`, and the
  falsifiability test `TestCheckpointResumeEquivalenceDetectsDroppedWrite`.
- `vm/fuzz_test.go`: the existing driver became `sourceFuzzTasks.runAll` with
  an optional `checkpoint` hook at suspend yields; `executeFuzzSource` uses it.
- `vm/persistence_temporaries_test.go`: `resumeThroughCheckpoint` now calls
  `checkpointSuspendedVM` instead of its own write/load/restore copy.

Survey (before building): no generated checkpoint-equivalence property
existed. Fixed-source round trips only: `vm/persistence_temporaries_test.go`,
`vm/persistence_thread_mode_test.go`, `vm/exception_reason_test.go`,
`vm/move_lifecycle_persistence_test.go`, `db/format/*checkpoint*_test.go`.

Measured: a scratch scan (not committed) of 15000 pseudo-random 96-byte choice
lists: 20 checked, 29 skipped on the seconds budget, 14951 never suspended;
no difference and no failure. None of `fuzzseed.AddChoices`'s 16 seeds
suspends.

## Decisions and why

- Persistent effects are observed by a MOO program run as a wizard (each valid
  object's name, owner, location, contents, parent, flags, defined properties)
  rather than a Go store walk: master's `CompareRoundTripStores` compares no
  property values, PR 655 (unmerged) deepens it, and a second Go comparator
  would duplicate that owner.
- The seeded database is #0 and #2 with an rw property `p`, because generated
  programs run unprivileged (Programmer #-1) and could otherwise change nothing.
- The checkpointed run resumes on the RELOADED store, so persisted writes pass
  through the writer and reader. The restored VM keeps the task state and tick
  count, since a snapshot carries no ticks and the fuzz budget spans the task.
  The restored context copies Player/Programmer/This/Verb/IsWizard, as
  `engine/task_load.go:72-80` does from the saved call stack.
- Checkpoints happen only at suspend() yields. Fork yields are not checkpoint
  states (the runtime queues the child before the parent continues). Fork
  children queued before a checkpoint stay in memory and run on the reloaded
  store; queued-task retention through a checkpoint is a separate plan track.
- Skips: no task suspended; any task in either run hit
  `ExceptionReasonSecondsBudget` (wall-clock dependent).
- `executeFuzzSource` now checks the literal round trip after forked children
  run, not before; toliteral/equal of a value do not read the store.
- Crash-recovery idempotence through the checkpoint subprocess helpers is NOT
  covered by this change.

## Surprises

- Generated programs rarely suspend (about 1 in 750 random lists), so the
  pinned seeds and coverage-guided fuzzing carry the property.
- Native fuzz mode on the Windows workstation stalls (execs stop advancing
  after a few seconds) for this target AND for unmodified master's
  `FuzzStructuredSourceExecution`. Cause not investigated.

## Next step

Root: review, push, open the PR, CI. Afterwards add `./vm`
`FuzzCheckpointResumeEquivalence` to `.github/workflows/fuzz.yml`'s matrix.
