Tool and model: Claude Code (Claude Agent SDK), model claude-fable-5-2 (Claude Fable 5.2).

# Claude fable 5.2 Jepsen collaboration

Tool and model: Claude Code (Claude Agent SDK), model claude-fable-5-2 (Claude Fable 5.2).

Planning only. This file is the collaborator's sole writable output.

## Progress log

- Read: AGENTS.md, tests/jepsen-checkpoint/README.md, project.clj, src/barn_checkpoint/core.clj. Reading the Python owners next.
- Read: scripts/jepsen_checkpoint.py, scripts/test_jepsen_checkpoint.py, scripts/mongoose_barn_process.py, .github/workflows/jepsen.yml, docs/elle-isolation.md, scripts/check_elle.py. Reading Barn's signal, checkpoint publication, startup recovery, and task persistence source next.
- (A hook asked for an update to agent-notes/jepsen-expansion-plan.md; the parent's brief assigns that note to the parent, so progress is recorded here only.)

## Interim findings (verified in the harness source, before reading Barn internals)

- The server runs with `--checkpoint-interval 0` (scripts/mongoose_barn_process.py:151), and the only checkpoint is the final one written on SIGTERM (scripts/jepsen_checkpoint.py:265-284). No checkpoint is ever taken while clients run.
- `gen/phases` waits for outstanding client operations before the checkpoint phase (tests/jepsen-checkpoint/src/barn_checkpoint/core.clj:110-123), so every checkpoint is of a quiescent server.
- A crash is SIGKILL followed by a launch from a fresh copy of the last verified pair in a new directory (scripts/jepsen_checkpoint.py:213-227, 258-263). The crashed process's directory is never reopened, so Barn's own crash recovery is never executed.
- Every transaction reads and rewrites two whole map properties on `#0` (scripts/jepsen_checkpoint.py:53-72), as the wizard (lines 205, 332).
- Also read: server/signals_unix.go, server/signals.go, server/server.go:119-120 and 330-646, db/format/checkpoint_publication.go, checkpoint_stage.go, checkpoint_lock.go, reader.go:141-195, engine/admission.go:59-73, engine/task_factory.go:458-480, engine/task_load.go:16-176, builtins/system.go:818-846, docs/checkpoint-recovery.md, docs/deployment-mongoose.md:50-160, moo-conformance-tests transport.py:227-247 and 390-560. Nothing was run.

# Final report

Everything below comes from reading source. I ran no server, test or checker, and read no paper.

## Bottom line

Today's campaign checks one thing that can fail: a quiescent SIGTERM dump reloads with the same ledger. Its crash half is vacuous, and it never touches the hourly live checkpoint, Barn's own publication recovery, saved tasks, keyed updates, nonwizard clients or retried effects. The highest-value change is one lane: live checkpoints under load, SIGKILL inside the checkpoint, restart on the exact directory, and a checker for atomic and bounded recovery.

## 1. What the campaign tests, and where it passes without exercising anything

What it really tests:
- **Quiescent signal dump round trip.** The ledger read before SIGTERM must equal the ledger a new process loads from the published pair (scripts/jepsen_checkpoint.py:265-284, 447-470). This goes through `dumpCheckpoint(..., checkpointTerminal)`, the hook-free shutdown path (server/server.go:584-590).
- **Whole-property serializability over a real socket**, per process generation (jepsen_checkpoint.py:522-539).
- **Client timeouts, by accident.** `SocketTransport` sets a 3-second socket timeout (transport.py:230) and the pause lasts 4 seconds (core.clj:115), so operations spanning a pause should become `:info` and drop their connection. Inferred from source, not observed.

Where it passes vacuously:
- **Crash recovery.** After SIGKILL the next generation starts in a new directory from a byte-identical copy of the last verified pair (jepsen_checkpoint.py:213-227, 258-263). The crashed directory is never reopened, so `recoverCheckpointPair` (db/format/checkpoint_publication.go:299-353) never runs. Each crash still counts as a "checkpoint comparison" (jepsen_checkpoint.py:473-487), and the "attempted after checkpoint" branch (lines 463-466) cannot fire.
- **Live checkpoints.** The server runs with `--checkpoint-interval 0` (scripts/mongoose_barn_process.py:151). The running-mode checkpoint, which serializes and publishes outside the barrier while tasks commit (server.go:528-533), never executes. Neither do the `checkpoint_started`/`checkpoint_finished` hooks (server.go:472-487), `dump_database()` (builtins/system.go:825-846) or the SIGALRM/SIGUSR2 requests (server/signals_unix.go:37-42).
- **Crash during checkpoint.** `gen/phases` drains clients before each checkpoint (core.clj:110-123), and crashes only occur with no checkpoint in flight.
- **Snapshot atomicity.** All durable state is one map property (jepsen_checkpoint.py:64, 70), so a checkpoint torn between properties or objects could not show.
- **Concurrency paths.** Every write transaction rewrites the same two whole properties, so keyed map tracking and rebase (docs/elle-isolation.md:14-16, 58-60) and parallel commits never occur.
- **Tasks, permissions, definitions.** No fork or suspend, and every client is the wizard (jepsen_checkpoint.py:332).
- **Progress.** A generation with no `:ok` operation is "skipped" and still passes (jepsen_checkpoint.py:523-525, 540).

## 2. Highest-yield paths, and how to cover them

**A. Live checkpoint under load, kill inside it, restart in place.** This is the hourly production path (default interval 3600 s, docs/deployment-mongoose.md:118-119).
- Workload: clients keep running. The harness requests a checkpoint with SIGALRM (signals_unix.go:41-42) or a wizard `dump_database()`.
- Durability acknowledgment: the `checkpoint complete` log record, written only after `Publish` returns (server.go:479-493, 528-533). Never a MOO reply.
- Fault timing without new Barn hooks: tail the log for `checkpoint started` (server.go:470), SIGSTOP at once, classify the frozen directory, then SIGKILL. The classes are: staging only; journal present with old output; new database with old sidecar; both new with journal; journal removed. Pad the ledger with large values so serialization takes long enough to hit.
- Restart: on the same directory, loading the output name, which is the documented recovery entry (docs/checkpoint-recovery.md:30-32; reader.go:157-166).
- Checker, for recovered ledger R, last acknowledged checkpoint c (requested at `t_req`, acknowledged at `t_done`):
  - Lower bound: every transaction `:ok`-completed before `t_req(c)` is wholly in R.
  - Upper bound: nothing invoked after `t_done(c)` is in R, unless a later checkpoint was in flight at the kill.
  - Atomicity: each transaction's appends are all in R or all absent. This needs the ledger spread over several properties and objects.
  - Order: R[k] is prefix-compatible with the longest list read for k before the crash.
  - Monotonic: R contains the previous verified recovery.
  - Coverage: a nonzero count of kills per publication stage.

**B. Wire-level isolation on real paths.** Add transaction shapes: keyed `#0.prop[k] = ...` assignment, one property per key on separate objects, and a slow single-slice read-modify-write that forces overlap and retries. Keep Elle list-append. Add a second verdict at `strict-serializable`: sockets give real-time order for free, and a serial server like Toast cannot return a stale read after an acknowledged write. Whether Barn promises this is an unverified contract (docs/elle-isolation.md:22-26 chose `serializable` for the in-process recorder).

**C. Saved tasks.** Each transaction appends a marker m and forks or suspends a task that later appends token t, with delays spanning checkpoints and kills. After a restart: m in R means t appears exactly once within a bound; m absent means t never appears; no token appears twice. This tests that the store and task snapshots are one cut (server.go:510-515). It also catches the silent path where a queued task that fails to restore is logged at WARN and skipped (engine/task_load.go:22-27), and the dropped-task ERROR (task_load.go:40-51).

**D. External effects under retry and connection loss.** Each transaction emits `notify(player, token)`. Each `:ok` reply must carry each token exactly once, since retried attempts must not leak effects (docs/elle-isolation.md:29-32). Add a client-side socket close right after send; a final read per generation resolves those unknowns.

**E. Nonwizard clients, permissions, definitions.** Clients log in as `Programmer` (transport.py:237). A wizard client revokes and restores property permissions and reprograms the verb the appends go through; each change also appends to an epoch row in the same transaction. Programmer transactions read the epoch row and return typed outcomes (`ok`/`denied`) through try/except, so a denial is a definite `:fail`. Violation: an `:ok` append whose epoch read shows the revoked state.

**F. Progress after healing.** After every resume and restart, require an `:ok` within a bound and record time to first `:ok`.

## 3. What the harness conceals or discards

- **Barn's logs are archived but never read.** `analyze` reads only `lifecycle.jsonl` and `history.jsonl` (jepsen_checkpoint.py:500-542). Uncaught exceptions, recovered panics, `checkpoint failed`, task-restore warnings and heap-growth records cannot fail a run.
- **The crashed directory's state is never recorded** (jepsen_checkpoint.py:258-263), so nothing says where a kill landed.
- **Crash comparisons inflate the evidence count** (see section 1).
- **No final read before a crash**, so an `:info` append in a crashed generation is checked only by Elle within that generation.
- **Zero-`:ok` generations pass** (lines 523-525).
- **Any native error fails the run** (lines 510-511), which blocks workloads with expected errors such as E_PERM.
- **The seed binds only stagger timing** (core.clj:172-174), and the nemesis schedule is fixed sleeps (core.clj:115-116), so seeds explore nothing.
- **No end-to-end negative control.** CI runs one healthy campaign (jepsen.yml:88-96). The unit tests cover pure functions only (scripts/test_jepsen_checkpoint.py:1).
- **Latency is discarded.** The history carries times; nothing reports unavailability windows.

## 4. Dependency-ordered changes to existing owners

1. **Read the evidence already collected** (`jepsen_checkpoint.py` analyze).
   - Change: fail on ERROR records, uncaught exceptions, panic stacks, and the task-restore and checkpoint-failure messages in each generation's logs. Fail a generation with no `:ok` after admission. Report crash copies separately from durability comparisons.
   - Acceptance: unit fixtures in `test_jepsen_checkpoint.py`, one per record type, each making analyze fail.
   - Control: `--analyze` over an archived CI evidence tarball either stays green or surfaces a real record.
2. **Strict-serializable second verdict** per generation, recorded beside the existing one.
   - Acceptance: a hand-written EDN history with a stale read after a completed write is accepted at `serializable` and rejected at `strict-serializable` by the pinned jar. Not verified that the pinned elle-cli accepts that model name.
3. **Live checkpoint and in-place restart lane** (`mongoose_barn_process.py`, `core.clj` nemesis, analyze).
   - Change: add a checkpoint signal, a log wait, freeze-and-classify, and launch on an existing database path; add `checkpoint-live` and `crash-in-checkpoint` ops; spread the ledger; add the section 2A checker.
   - Acceptance: a healthy run with nonzero acknowledged live checkpoints under load, in-flight kills, in-place restarts and recovered journals.
   - Controls: restart from the previous pair after an acknowledged checkpoint must be reported as lost acknowledged tokens; deleting the journal and recovery directory after a kill in the "new database, old sidecar" stage must be reported as a recovery failure; pure fixtures for each checker rule.
4. **Transaction shapes** (`txn_code`, generator): keyed, per-object and slow variants, with the variant recorded in the history.
   - Acceptance: nonzero retries or conflicts per generation, read from the debug endpoint the owner already discovers (mongoose_barn_process.py:252-262).
   - Control: a server built with `-tags elle` and `BARN_ELLE_FAULT=property-reads` (internal/isolationprobe/enabled.go:16-19) must be rejected with a witness through the same wire campaign. I did not verify that `cmd/barn` builds under that tag or that the slow variant makes the loss certain. If it is not certain, it is not a control.
5. **Effects and connection faults**: notify tokens, client-side close, final read per generation.
   - Acceptance: exactly-once holds in a healthy run with nonzero retries and nonzero dropped connections.
   - Control: fixture replies with a duplicated token line.
6. **Task lane**: marker and task-token transactions with the section 2C checker.
   - Control: a wizard `kill_task()` of a pending task after the acknowledged checkpoint, hidden from the checker, must be reported as "marker durable, task token never appeared".
   - Needs Toast admission first (section 6).
7. **Nonwizard lane**: typed outcomes, epoch row, programmer clients.
   - Control: fixtures only, unless a fault build proves certain.
8. **Seeded schedules**: draw nemesis timing from the seed and record the full schedule in `results.json`.
   - Acceptance: two runs with one seed produce the same invocation sequence per process. If Elle's generator is not seedable, that test shows it.
9. **Workflow split** (`jepsen.yml`): PRs keep a bounded deterministic run with the deterministic controls. `workflow_dispatch` takes seeds, durations, lanes and padding, as `elle.yml` does (docs/elle-isolation.md:119-131), and records no tested-tree pass. No schedule.

## 5. What to avoid adding

- Partitions, clock skew, multiple nodes or any replication claim: there is one process and a dummy SSH node (core.clj:128-129).
- Power-loss or filesystem fault injection: docs/checkpoint-recovery.md:71-74 disclaims it. It is a separate decision.
- Longer or faster runs of the current workload: one serial conflict on one property finds nothing new.
- Copying the in-process Elle held pairs into Jepsen: the wire lane is for unscheduled interleavings and process faults.
- Catching native errors wholesale to get green (docs/elle-isolation.md:270-271).
- Mongoose's database, file I/O, SQLite or outbound exactness across crashes: no stated contract.
- A schedule replayer or shrinker before a failure fails to recur under the same seed.
- New test hooks in Barn's checkpoint path: SIGSTOP and classification need none.
- Scheduled or continuous runs, or discovery on the self-hosted runners, which are on the workstation.

## 6. Unverified contracts, possible bugs, oracle questions

Decisions for Q (they change the lane design):
- **Restart contract.** Barn loads only `--db` (server.go:119-120) and writes checkpoints to `<db>.new` (checkpoint_stage.go:33-35). Nothing in the server promotes one to the other. The deployment restart snippet starts on the same `--db` (docs/deployment-mongoose.md:147-157), which by this code loads the pre-checkpoint input. I have not verified what the production host does. The Jepsen harness currently plays the operator by copying the pair. The in-place lane needs the supported procedure stated.
- **Real-time order.** Is a stale read after an acknowledged write ever acceptable over the wire?
- **Discovery compute.** I did not verify what non-workstation runners exist.

Possible bugs, not established:
- A kill during publication leaves the journal beside `<db>.new`. A restart on `<db>` does not recover it until the next checkpoint begins (checkpoint_stage.go:49). An operator copying `<db>.new` in that window may take a mismatched pair. I did not read `readWaifIdentitySidecar` to confirm load rejects it.
- A queued task that fails to restore is skipped with a WARN (task_load.go:22-27), while a suspended one aborts boot (server.go:278-281).
- The 3 s transport timeout against the 4 s pause (section 1).

Needs managed Toast oracle admission before any Barn change:
- when overdue forked and suspended tasks run after a restart;
- what happens to a task whose connection closed;
- whether a restore failure of a saved task should stop boot.

Not read:
- `vm.RestoreVMSnapshot`, `store.SnapshotWithRoots`, the MVCC commit path;
- `scripts/test_mongoose_barn_process.py`, `.github/actions/elle-cli`, `elle.yml`;
- line 1 of `internal/isolationprobe/enabled.go` (the build constraint).

# Addendum: reconciliation with the parent's draft (2026-11-10)

Read for this addendum: the draft in `agent-notes/jepsen-expansion-plan.md`, `internal/isolationprobe/enabled.go`, the `runs-on` lines of `.github/workflows/fuzz.yml` and `fuzz-controller.yml`, and a count of `isolationprobe.Pause`/`Fault` call sites. Nothing was run. The first report above stays as written; the corrections below supersede it where they conflict.

## 1. Corrections to the first report

- **"The crash half is vacuous" is withdrawn.** SIGKILL still tests real things: in-flight socket transactions end as unknowns, the per-generation Elle check runs across the interruption, and the next generation must serve from the verified pair. What is true is narrower: the crash never loads the interrupted directory, so journal recovery (`db/format/reader.go:157-166`) does not run, and the crash-kind ledger comparison compares a file copy with its own baseline.
- **"Parallel commits never occur" is withdrawn.** Shared whole-property dependencies make writers conflict; they do not tell me which runtime commit paths execute. Supported claim: keyed map access is not generated (`scripts/jepsen_checkpoint.py:53-72`), and no path counts are collected.
- **"Self-hosted runners are on the workstation" is withdrawn for discovery.** `fuzz.yml:36-38` and `fuzz-controller.yml:36-38` target runner group `barn-bug-discovery` with label `barn-fuzz`. I did not verify where those runners are or whether they are ready. Recommendation: reuse that route and `scripts/fuzz_compute.cjs`; build no new scheduler.
- **The restart-contract question to Q is withdrawn.** Loading the `.new` output is documented (`docs/checkpoint-recovery.md:30-32`).
- **"Record a strict-serializable verdict" (change 2) is withdrawn.** No contract supports it. Gate on `serializable` only.
- **"Fail on any ERROR record" (change 1) is withdrawn.** See section 4.
- **"Monotonic: R contains the previous verified recovery" and "exactly once" for tasks and effects are withdrawn.** See section 3.

## 2. Stage guarantees

The bounded pause wiring is justified, as a second step after the ordinary-binary lane.
- **Why random timing is not enough.** The database and sidecar renames are consecutive calls (`db/format/checkpoint_publication.go:370-372`), as are the restore renames (lines 341-345). A randomized SIGSTOP cannot be required to land between them, so it cannot back a required coverage count or the "mixed pair is refused" control.
- **Why this extends a seam and does not copy the suite.** `isolationprobe.Pause` is already a compiled-out callback (`internal/isolationprobe/enabled.go:1`, `32-49`) with call sites in `db/store`. Adding about five named points in the publication code and one startup callback moves no held-pair workload into Jepsen.
- **Smaller control than the draft's pipe.** At the point named by an environment variable, the callback sends SIGSTOP to its own process. The harness already owns the pidfd and pause/resume (`scripts/mongoose_barn_process.py:390-404`); it sees the stopped state, copies the directory, then kills or continues. No pipe, no endpoint. Unverified: that `cmd/barn` builds and serves under `-tags elle`.
- **What it adds over `checkpoint_recovery_test.go:188-250`.** Those tests interrupt the format layer with synthetic files. This lane interrupts a serving process with live tasks and restarts through the real loader.
- **Signal.** Use SIGALRM: it calls `requestCheckpoint` directly (`server/signals_unix.go:41-42`). SIGUSR2 first runs `#0:handle_signal`, which can suppress the request (`server/signals.go:13-25`). Keep `dump_database()` as a separate scenario.

## 3. Live snapshot oracle

Scope every rule to one epoch (one process lifetime). Never compare across epochs.

Definitions:
- **S0**: the durable rows observed at this epoch's admission.
- **R**: the rows observed at the next admission; **D** = R minus S0 per row.
- **T**: a transaction with invoke time `i(T)`, completion time `c(T)` when `:ok`, and durable tokens `W(T)`.
- **A**: the last checkpoint in the epoch whose `checkpoint complete` record the harness observed, with `t_req(A)`, `t_ack(A)` and the pair hash taken at `t_ack(A)`.
- **F**: the first request after A with no observed acknowledgment, if any.
- **Restart input**: this epoch's output path if the output or its journal exists, otherwise this epoch's input.

Rules:
1. **Base.** S0[row] is an exact prefix of R[row] for every row.
2. **No checkpoint.** With neither A nor F, D is empty.
3. **Which cut.** If the loaded database's hash equals A's recorded hash, the cut is A. Otherwise it must be F; with no F this is a violation.
4. **Lower bound.** Cut A: every `:ok` T with `c(T) < t_req(A)` has all of `W(T)` in D. Cut F: the same with `t_req(F)`.
5. **Upper bound.** Cut A: every T with `i(T) > t_ack(A)` has none of `W(T)` in D. Cut F: only unsent operations are excluded.
6. **Whole transaction.** For every sent T, `:ok` or `:info`, `W(T) ∩ D` is empty or all of `W(T)`. Unknowns may fall either way, whole.
7. **Read closure.** If an `:ok` T is in D and T observed token x of this epoch in a row, the transaction that wrote x is in D.
8. **Order.** Every `:ok` read of a row in this epoch is prefix-comparable with R[row].
9. **Tokens.** No duplicate in a row; every token in D was sent in this epoch.

Notes:
- Elle stays per generation. If the durable rows are also the Elle rows, the adapter strips the S0 prefix from each read and reports a mismatch as a violation.
- Tasks and effects are judged per epoch against R. A marker in R obliges its task token to appear at most once per row and within a bound; a marker absent from R forbids the token in that epoch. A task or effect after the cut may legitimately repeat after rollback. Within a live epoch, a committed transaction's effect token appears once in its reply.

## 4. Errors and logs

- **Caught errors.** A caught error does not prove rollback. Classify a denial as `:fail` only when the generated program has no write statement before the guarded operation and the reply says the guard raised. Every other error reply is `:info`.
- **Logs.** Each scenario declares its expected structured records (message plus attributes), for example the task-drop ERROR at `engine/task_load.go:44-50` in a deliberately invalid-snapshot scenario. Analysis reports records outside that list as unexamined, by scenario. Raw logs are always kept.

## 5. Order

First tranche, each piece useful alone, no broad framework first:
1. Incremental adapter journal and a final observation of every touched row per epoch.
2. Durable rows spread over several objects and properties, plus a keyed-map variant.
3. SIGALRM live checkpoints under load, randomized stop/classify/kill on the ordinary binary, restart on the actual output path, and the section 3 oracle.

Then: instrumented stage points with their real-binary controls; tasks; authority and definitions; transport faults; discovery on the existing `barn-bug-discovery` route.

From the draft's section 1, take only the journal and final observations first. The obligation taxonomy, result classes and pinning can follow the first real-recovery lane.

Reduction starts with the Elle witness, `--analyze`, and artifact replay (restart Barn on a preserved copy of the crash directory, no workload). Add an execution reducer only after a real failure resists those.
