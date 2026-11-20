# fix/gc-pause-holds-input-last

## Goal
Stop player commands from waiting out long background slices when maintenance pauses
admission. Q approved on 2026-10-12 after the investigation recorded in
`agent-notes/mvcc-simplify.md` (master checkout, "CAUSE of the 4/s").

## The defect
`engine/deferred_gc_maintenance.go` `rendezvousDeferredGC` (from #429, issue #270) and
`WithCheckpointBarrier` call `admission.Controller.Pause`, which held ALL fresh grants
until every reservation drained. The 10-11 Mongoose fixture has a world simulation
whose background slices run 3-8 s (bg limit is 20M ticks / 30 s). Measured on master
db01daf4's predecessor 1f1c0a17, 1 player: `input_admission` max 6245 ms; goodput 4/s,
p99 6.8 s; with the rendezvous skipped by a scratch switch 22-24/s, p99 209-248 ms.

## Design
Pause drains in two steps. Step 1 holds only fresh Background work and waits until no
Background segment is active or preempted; Input is still admitted. Step 2 holds
Input too and waits for full idle. The sweep and the checkpoint still run only at
full idle, so nothing about root capture changes, and #270's progress guarantee holds:
step 1 is bounded by the longest background slice, step 2 by the longest input slice.
What changes: an input command no longer waits for a background slice, only for other
input and for the sweep itself.

Rejected: bounded patience with retry (what I first proposed to Q). With slices
continuously in flight it never finds idle, so it either starves the GC (regressing
#270) or needs a forced blocking pause every N seconds, which keeps the 6 s stall.
Not attempted: making a quantum yield a GC safepoint (inspect parked VMs). It would
bound the pause to milliseconds but needs the sweep to treat live snapshots and
staged transaction values as roots.

## State (uncommitted)
- `internal/admission/controller.go`: fields `background`, `inputHeld`,
  `backgroundIdle`; `Key.Command`; `Key.drainsFirst()`; Pause; dispatchLocked;
  preempt; settleLocked.
- `engine/admission.go`: `backgroundAdmissionKey` sets `Command: t.FromCommand`
  (the command parser's own marker, task_runtime.go:1186); system entry through
  `enterInput(system)` sets `Command: true`.
- Tests: new `TestPauseAdmitsInputWhileBackgroundDrainsThenHoldsIt` (shown red with
  the old hold-everything rule: "input waited behind a background slice during a
  pause"); `TestPauseWaitsForPreemptedSegmentAndReadmitsIt` now enqueues Background
  fresh work; `TestDeferredGCProgressDrainsOverlappingExecution` probes with a
  Background key and adds a continuous input probe that must keep flowing.
- `go test ./internal/admission ./engine ./server`: ok. `-race ./internal/admission` ok.

## Measurements (real workload, today's 197 MB fixture, alternating runs, one each)
Step 1 only (Input not held during the background drain):
  1p  base 5/s p99 4733 | fix 5/s p99 6769 ; base 5/s p99 4439 | fix 5/s p99 6547
      look 375/341 ms -> 62/55 ms; say 206/202 -> 506/558 ms (still stalls)
  16p base 39/s p99 8199 | fix 62/s p99 5604 ; base 68/s p99 6976 | fix 100/s p99 3742
      look 300/226 -> 70/62 ms; home 459/288 -> 2.0/1.9 ms; say 583/239 -> 740/221 ms
Why `say` still stalled: it suspends for its SQLite call and every scheduled task,
including that continuation, re-enters through `enterBackground` as Background.
With `Key.Command` (this tree), CPU 49% at start:
  1p  base 4/s p99 7322 | fix 20/s p99 250 ; base 3/s p99 7277 | fix 20/s p99 232
  16p base 87/s p99 5626 | fix 191/s p99 3043 ; base 252/s p99 1567 | fix 347/s p99 491
16p varies too much run to run (base 39..252 across the evening) for one number;
the fix was higher in every pair. `home` fails 2 in both base and fix at 16p.
Gates on this tree: gofmt, vet, staticcheck, golangci-lint clean; `go test ./...`
exit 0; `go test -race ./internal/admission ./engine` ok (229 s). Elle driver in WSL:
3 seeds true/true, 5 faults false, exit 0. `make conformance` in WSL:
`12726 passed, 443 skipped in 910.69s`, exit 0.

## Surprises
- `task.Kind` is not a usable "is a command" signal: NewTask defaults it to TaskInput,
  so `CreateBackgroundTask` and `Runtime.Fork` tasks carry TaskInput.
  `task.FromCommand` is the reliable one.
- `Scope.ResumeBackground` has no callers outside tests.

## Known limit
Tasks a command forks are plain Background and still wait for the drain.

## Next step
PR is open; watch CI. After merge, remove this worktree and branch.
