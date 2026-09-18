# Barn and Toast scheduling: source audit

Toast's protection against a busy background producer is per-queue service
accounting and reconsidering the next task after each execution. Barn instead
drains a global ready snapshot. The earlier external-completion ordering repair
did not add Toast's fairness mechanism.

This is a source diagnosis and implementation recommendation. No runtime
scheduling policy was changed during this audit. The companion research report
is `research-barn-scheduling.md`.

## Sources inspected

- WSL Debian `/root/src/toaststunt-mongoose-login-20260917`, commit
  `72e3c7f96ce7a41fdeba793aef8818dc4408072e`. Tracked source was clean;
  `build-release/` was untracked.
- Barn `14dada9`, `engine/runtime.go`,
  `engine/internal/scheduler/scheduler.go`, `server/input_processor.go`,
  `engine/task_factory.go`, and `engine/task_runtime.go`.

## What Toast actually does

1. `src/tasks.cc:1183` inserts waiting tasks by ready time. Forks are associated
   with their activation's programmer; suspended VMs use `progr_of_cur_verb`.
   `src/eval_vm.cc:66` defines that as the top activation's programmer.
2. `src/tasks.cc:1629` admits due tasks into each programmer's background FIFO.
   Admission of many tasks does **not** mean executing all those tasks at once.
3. `activate_tqueue`, line 395, maintains active queues in increasing usage order.
   Its `<=` comparison inserts a queue after other queues with equal usage.
   `ensure_usage`, line 408, gives a newly active queue the current minimum usage,
   rather than zero accumulated service or unlimited credit for time asleep.
4. `run_ready_tasks` selects the first active queue and tries input before
   background work within that queue, subject to `hold_input`/`read` rules.
   It stops after one task execution (`did_one`), charges `end - start` to usage,
   and reinserts the queue. Usage here is integer wall-clock seconds, not a
   high-resolution CPU-time measurement (`tasks.cc:1804`).
5. `src/server.cc:854` calls `network_process_io`, then `run_ready_tasks`, then
   `deal_with_child_exit`. It therefore revisits input, ready admission, and
   external completions between task executions.
6. `resume_task`, `tasks.cc:1324`, makes an external completion ready at time
   zero and activates its programmer queue. It still appends to that queue's
   already admitted background FIFO when admission runs. It does not jump ahead
   of all existing work globally.

Source excerpts:

```cpp
while (active_tqueues && !did_one) {
    tq = active_tqueues;
    // ...
    t = dequeue_input_task(tq, ...);
    if (!t) t = dequeue_bg_task(tq);
    // Execute one task; set did_one.
    // ...
    tq->usage += end - start;
    activate_tqueue(tq);
}
```

Toast does not arbitrarily preempt a running MOO activation to meet a latency
deadline. A long execution slice or long queue within the same programmer can
still cause delay. Its threaded builtins are not evidence that ordinary MOO
task execution has arbitrary instruction-level preemption.

## Where Barn diverges

`engine/runtime.go:465` obtains one global snapshot, then `runReadyTasks` loops
over **every** batch planned from that snapshot. `scheduler.Run` waits for all
members of each worker batch. Ready admission is not reconsidered between those
batches. The scheduler has no per-programmer service accounting.

```go
readyTasks := s.scheduler.Ready(time.Now(), s.taskManager.Snapshot())
s.runReadyTasks(readyTasks)
// runReadyTasks:
for _, batch := range s.scheduler.Plan(readyTasks) {
    // Claim the selected tasks.
    s.runTaskBatch(claimed)
}
```

`server/input_processor.go:197` allows only one such runtime pass in flight.
New socket input can still run on separate connection lanes, and
`engine/task_factory.go:394` resumes `read()` synchronously on its connection
lane. Consequently, this is not a claim that all foreground work is trapped
behind the runtime pass. But after a welcome hook suspends in `exec()`,
`Task.CompleteExec` makes it queued and it returns through the global scheduler.
It loses the connection lane's immediate execution path.

The subsequent gate review adds a second independent ordering boundary:
`engine/task_runtime.go:133` takes exclusive gate access before every
nonretryable resumed slice executes, including a slice that ultimately performs
no writes. Connection lanes bypass the ready scheduler but do not bypass this
gate. Ordinary writing commits acquire its shared side; checkpoints acquire its
exclusive side. Gate admission therefore belongs in the scheduling design.

Also distinguish snapshot length from batch width: current fork/saved-VM work
is put in solo batches by `taskIsConflictRetryable` and `Scheduler.Plan`.
Reconsidering between dispatches is useful; merely reducing worker batch size
would not change these already-single-task batches. See the Fable review and
follow-up for gate/worker/GC interactions. Neither source finding establishes a
hard latency bound or the dominant delay on every login stage.

The existing internal test `engine/scheduler_fairness_test.go` explicitly
requires all ready tasks to complete in one pass. That is an implementation
contract, not proof of MOO fairness or a Toast conformance requirement. Any
replacement must update that internal contract and add source-derived fairness
coverage; existing cross-server assertions must not be weakened to fit Barn.

## Mongoose evidence

A read-only `verb_info` query on the disposable running Barn returned:

```text
=> {{#2778, "rxd", "s_run"}, {#2259, "rxd", "confunc"}, {#2, "rxd", "time_offset"}}
Latency password_to_account_welcome_ms=17
Latency password_to_clean_login_ms=39650
```

Evidence: `.tmp/mongoose-resume-20260918/scheduler-owners.txt`, event prefix
`barn-20260918-153939-495`. These distinct verb owners make programmer fairness
directly relevant. Static verb ownership does not prove every dynamic suspension
has that programmer: nested calls and permission changes must be accounted for.
The earlier helper completion-to-dispatch measurements establish actual queue
delay; this audit identifies a concrete missing mechanism rather than proving
how many milliseconds each proposed policy would save.

## Replacement boundary and tuning

Recommended implementation sequence, informed by the companion research:

1. Separate admission, policy selection, and worker execution. Keep persistent
   admitted FIFOs, including unstarted tasks, rather than emptying the heap into
   a long-lived execution plan. Revisit arrivals after each bounded dispatch.
2. Identify the scheduling principal from the correct current activation at
   suspension/fork, never simply `Task.Owner` or the root verb's original owner.
   Preserve FIFO and kill/queued-task visibility within admitted queues.
3. Add service accounting across principals. A busy producer's additional forks
   must not buy additional CPU share. Define sleep/rejoin credit explicitly.
   Start with a simple weighted fair policy; do not label a partial imitation
   of EEVDF as an implementation of EEVDF.
4. Preserve the bounded MVCC worker pool, but cap admitted in-flight work and
   reconsider policy before filling more slots. Reevaluate whether joining all
   siblings unnecessarily delays dispatch to an idle worker. Maintain the
   lifecycle/GC barriers and retry/nonretryable isolation.
5. Treat share and latency preference as separate operator-controlled settings.
   Candidates: per-principal/class weight, latency target, and maximum in-flight
   work. Interactive preference must be bounded; background progress is required.
   Proposed targets are soft until maximum nonpreemptible execution is bounded.
6. Unify or explicitly account for direct foreground execution if claiming
   whole-server weighted fairness. Tuning only the background/resume scheduler
   cannot establish that claim while connection lanes bypass it.

Instruction-level preemption is a separate change. Barn holds an exclusive
commit gate after certain irreversible effects (`engine/task_runtime.go:185`).
Suspending the Go goroutine while retaining that gate can worsen priority
inversion; releasing it or committing at a new point changes observable MOO
interleaving. Existing MOO yield/transaction boundaries are the initial safe
dispatch boundaries. Neither a smarter priority queue nor a shorter Go runtime
quantum solves that semantic problem.

Validation should follow this source-derived design: cross-programmer fairness
under a sustained fork stream; an external completion arriving mid-pass;
same-programmer FIFO; dormant-queue reactivation; cancellation/visibility;
background progress under continuous input; and worker/GC/commit-gate behavior.
Then measure login and command p50/p95/p99, queue wait, active execution, gate
wait, retries, and background progress on equivalent fresh workload snapshots.
