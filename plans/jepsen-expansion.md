# Jepsen expansion plan

## Goal
Plan a substantial improvement to Barn's Jepsen testing with Claude fable 5.2 through the external agents protocol, aimed at finding real correctness defects. Q authorized implementation of the full plan on 2026-11-10 ("build this out fully and completely"); publication rules are unchanged.

## State
Implementation authorized 2026-11-10. PR A (`jepsen/evidence-journal`, task note `agent-notes/jepsen-evidence-journal.md`) covers tranche 1 evidence: section 1 (adapter journal, final observations, scenario obligations, result classes, per-generation log analysis) and the pinning/provenance part of Delivery item 1. Property/keyed rows (PR B) and live checkpoints/recovery (PR C) follow as separate PRs.

Planning complete, 2026-11-10. Claude fable 5.2's independent audit and reconciliation addendum are saved in `reports/jepsen-expansion-claude.md`; its completed CLI result identifies `claude-fable-5-2`, `subtype: success`, `is_error: false`. The reconciled plan below incorporates source-checked findings and explicitly bounds unverified behavior. Existing unrelated work is preserved. External collaborator prompt: `prompts/jepsen-expansion-claude.md`. No campaign, test, implementation, commit, or publication was performed for this planning task.

## Decisions and why
Use the existing Jepsen workload and owned Barn lifecycle as the starting point. Distinguish live transaction isolation, checkpoint recovery, task semantics, and liveness so each claim has a suitable oracle. Read actual source before choosing expansion targets.

## Surprises
The present campaign deliberately restores a previously verified pair into a fresh directory after a crash. It never loads the interrupted publication artifacts. Checkpoints occur only at drained phase boundaries through graceful SIGTERM. The existing lower-level checkpoint suite already covers process interruption at journal/database/sidecar publication, so the expansion should connect those boundaries to real server execution rather than duplicate the format tests.

## Next step
Merge PR A, then PR B (property rows across objects and direct keyed-map variants) against then-current master, then PR C (Delivery item 2). External posts still need Q's explicit request.

## Source basis and literature

Read current local master owners, not old task notes: `tests/jepsen-checkpoint/{README.md,project.clj,src/barn_checkpoint/core.clj}`, `scripts/{jepsen_checkpoint.py,test_jepsen_checkpoint.py,mongoose_barn_process.py,check_elle.py}`, `.github/workflows/{jepsen.yml,fuzz.yml,fuzz-controller.yml}`, `.github/actions/tested-tree/action.yml`, `scripts/fuzz_compute.cjs`, `docs/elle-isolation.md`, `server/{server.go,signals.go}`, `db/format/{checkpoint_stage.go,checkpoint_publication.go,reader.go}`, `engine/{task_factory.go,task_load.go,task_runtime.go}`, and `internal/isolationprobe/{enabled.go,disabled.go}`. Line references below refer to this inspected checkout; implementation must refresh them.

Used the research-papers **paper-reader** skill and read every local page image `page-000.png` through `page-013.png` of Kingsbury and Alvaro, *Elle: Inferring Isolation Anomalies from Experimental Observations* (2020), https://arxiv.org/abs/2003.10554. Existing paper artifacts were already complete and were preserved. Relevant reasoning: recoverable unique writes and traceable lists expose dependency edges (pp. 3–8); unknown transactions must remain indeterminate (pp. 5–8); varied transaction shapes and contention discover defects beyond hand-written schedules (pp. 9–11); the paper's analysis does not cover general predicates (p. 12). This supports combining generated histories, precise regression schedules, and separate semantic invariants. It does not establish Barn's guarantees.

Also read Jepsen's primary [nemesis tutorial](https://github.com/jepsen-io/jepsen/blob/main/doc/tutorial/05-nemesis.md): record injected faults, distinguish definitely unexecuted operations from unknown outcomes, vary contention, retain witnesses. Applying it to Barn means process, connection, and checkpoint faults against one server. No replica partition model is justified by this source audit.

## What the current campaign covers and misses

| Current evidence | Consequence for expansion |
|---|---|
| `core.clj:94-123`: five active keys, transaction lengths 1–4, at most 64 writes/key; two timed phases separated by drained checkpoints. | Keep this smoke workload, add independent shapes and live checkpoint overlap. More elapsed time alone does not reach new paths. |
| `jepsen_checkpoint.py:53-72`: copy both whole maps into locals and rewrite them; `:53` always reads both properties. | All writers share whole-property dependencies; logical key diversity does not establish keyed-map coverage. A global audit ledger would reproduce this bottleneck. |
| `jepsen_checkpoint.py:205,332`: wizard connections. | No ordinary-user authority path is exercised. |
| `mongoose_barn_process.py:151,195-207`: interval zero; launch assumes fresh output files and records input checksum before startup. | Recovery support needs explicit provenance in the existing lifecycle owner; silently relaxing fresh-run checks is wrong. |
| `jepsen_checkpoint.py:213-227,258-284`: verified pair copied into a new directory; only graceful stop advances the source. | SIGKILL tests transport interruption and rollback to that pair; it does not exercise journal recovery at `reader.go:159-173`. |
| `jepsen_checkpoint.py:441-485`: per-key prefixes, duplicate/foreign tokens, acknowledged-token survival. | These checks are useful but do not express a whole recovered transaction or a coherent world/task snapshot. |
| `jepsen_checkpoint.py:500-539`: no-success generations skipped; at least one comparison and one checked generation required. | A campaign can finish without checking successful work after a particular crash or proving each configured fault was exercised. |
| `core.clj:172-175`: seed controls stagger timing, not Elle generation; JSON history is written after Jepsen completes. | Seed alone is not a replay artifact. A harness interruption needs incremental durable evidence. |
| `.github/workflows/jepsen.yml`: fixed campaign, floating moo-conformance-tests checkout, manual runs bypass lookup but record the same tested-tree pass. | Pin external inputs and keep fixed regression evidence distinct from exploration; preserve existing PR/merge queue check ownership. |

These are coverage or harness-design findings. This task has not demonstrated a new Barn runtime defect or measured bug yield.

## Design and priorities

Retain Jepsen as scheduler/history owner, Python as transport/lifecycle owner, pinned Elle as isolation checker, and Barn as the system under test. Extend these existing owners. Keep ordinary binaries and test-instrumented binaries as separate recorded configurations. A positive result requires both a valid correctness verdict and evidence that the selected scenario actually ran. Fault activation counters are never correctness evidence.

The numbered sections specify responsibilities, not six prerequisites before useful work. **First deliver the incremental journal, final observations, property/keyed rows, and a live-checkpoint/recovery lane together across small independent changes.** Broader parameter exploration and result reporting follow that working lane. Retain the present completeness checks and add the concrete obligations each new scenario needs as it lands.

### 1. Make the existing campaign's evidence and verdict precise

Owners: `core.clj`, `jepsen_checkpoint.py`, `test_jepsen_checkpoint.py`, and the existing workflow.

1. Record a stable operation ID, client, generation selected **before send**, generated MOO, request/response framing, send/completion outcome, and checkpoint/fault phase. Append adapter events before sending and after receiving; keep Jepsen's original history as well. Preserve pending invocations on timeout/cancellation; do not turn them into failed transactions or drop them to make analysis succeed. Reconcile adapter and Jepsen records by operation ID, rejecting contradictions.
2. After every heal/restart, issue bounded fresh-connection read/write probes and an observation of every key touched in the epoch, including keys retired by the generator. This exposes acknowledged terminal writes with no later observer. Distinguish a legal unknown crash outcome from inability to resume useful work.
3. Replace the present aggregate minimum with scenario obligations: selected fault observed on the owned PID; successful client transactions before and after it; checkpoint begun/completed or deliberately interrupted; expected recovered generation observed; nonzero reads and writes for each selected row family; no unexamined live interval. Emit exact missing obligations. A skipped generation is visible and cannot satisfy a post-recovery obligation.
4. Preserve distinct results for a demonstrated correctness violation, bounded recovery/progress failure, incomplete experiment, and invalid harness evidence. None counts as a healthy pass. Intentional negative controls must produce their designated witness, not merely any failure.
5. Pin the conformance fixture/transport revision and record binary, config, source tree, checker JAR, Jepsen dependencies, fixture/sidecar hashes, runner/OS/filesystem, CPU allocation and all parameters. Feed external pinned inputs through the existing tested-tree `extra` input; do not create another result cache.

Analyze the existing per-generation logs with scenario-specific expectations for structured message and attributes. A checkpoint-error injection or deliberately invalid task has an expected diagnostic; an unexpected panic, restore failure or checkpoint failure must be surfaced with its raw record. Unexamined error records prevent a healthy verdict. Do not turn every ERROR into a runtime defect or ignore a whole class of errors. In permission scenarios a caught error is not proof of rollback: use `fail` only for a known no-effect invocation (including a guard before all writes/effects); otherwise preserve `info` and the native result. Expected native errors are a separate declared scenario outcome, never silently normalized away.

Acceptance: raw fixture regressions for missing reply, late reply, unsent operation, mixed generation, no post-heal progress, zero actual fault, and missing final observation; exact `--analyze` replay of an archived result. Existing healthy/unknown classifications remain sound. Use the current tests to prove the incomplete post-crash case can no longer pass simply because generation zero had one successful transaction. Existing unit tests mock Elle: the PR campaign must also use the pinned real checker.

### 2. Exercise live checkpoints and the actual recovery path

Owners: lifecycle methods in `mongoose_barn_process.py`; campaign checkpoint/restart methods; `core.clj` nemesis; precise test-only pause sites at `server/server.go:499-540`, `db/format/checkpoint_stage.go:112-143`, and `checkpoint_publication.go:171-238,286-398`.

Keep the graceful drained checkpoint as a reference case. Add a checkpoint while clients continue, using SIGALRM, which calls `requestCheckpoint` directly (`server/signals_unix.go:41-42`). SIGUSR2 goes through a potentially suppressing MOO hook and belongs in its own admitted scenario. Keep the automatic interval disabled while the harness drives identified checkpoint requests, with at most one unacknowledged checkpoint at a time. Record request, capture, serialize, publication and callback separately: accepting a request or observing a started callback is not a durability acknowledgment. `checkpoint complete` follows successful publication (`server.go:479-493`); record the matching stable database/sidecar digest before issuing another request. `server.go` releases the running checkpoint barrier before Serialize/Publish, so mutate objects and tasks during those stages deliberately.

Begin with ordinary-binary randomized stop/classify/kill during live checkpoints. Record the actual frozen artifact stage and never credit a stage merely because the generator requested it. For precise narrow boundaries, extend `isolationprobe.Pause`/its build-tag owner and install a test-only command startup callback. On the selected point and occurrence it emits a synchronous boundary record and sends SIGSTOP to its own process; the adapter verifies that owned process is stopped and uses the existing pidfd signal ownership to resume or kill it. No control pipe or production endpoint is needed. This bounded wiring is justified because the two renames are consecutive calls and randomized timing cannot guarantee hitting their interval. The parent owns deadlines/cleanup, including a stopped child. It must compile out of normal builds; first verify that the instrumented command builds/serves and does not activate unrelated faults. Run both the instrumented schedules and ordinary-binary timing experiments; record which produced each result.

The restart input must be the **actual output path** of the interrupted checkpoint, with its journal, saved generations and sidecar left together. Barn's loader recovers the supplied path; launching the original `input.db` would not recover `input.db.new`. `docs/checkpoint-recovery.md:30-32` explicitly documents this. If a first checkpoint has produced neither output nor journal, use the unchanged input and record that no new checkpoint existed. Preserve a forensic artifact set before recovery changes it, but never choose a hand-picked known-good pair instead of an existing output/journal. Use unique per-process logs/manifests and an explicit recovered-input provenance path in the existing owner. Record before/after file hashes and verify the runtime manifest against the database selected by recovery, not blindly against the pre-recovery mixed file. Retain fresh-run checks for ordinary launches.

| Injected interruption | Required observation and oracle |
|---|---|
| During capture; during Serialize; after temporary DB sync or sidecar preparation, before journal publication | With a previous checkpoint, that completed pair survives. For a first checkpoint with no published pair/journal, absence of a new durable checkpoint is allowed; bootstrap input remains available. Never claim interrupted staging was acknowledged. |
| After publication journal, before DB replacement | With previous pair: restore previous. First publication: recover the complete saved next pair. |
| After DB rename, before sidecar rename | Same decision as above; never accept a mixed DB/WAIF identity pair. |
| After both renames, before cleanup | Keep valid complete next pair, finish journal cleanup. |
| During recovery, including between restore renames; then crash again | Repeated recovery converges to the same valid pair. Preserve the initial and second-crash artifacts. |
| Rename/sync/storage errors | Keep raw native error and exact stage; recover according to the same publication contract. Start with the existing `checkpointIO` seams for selected faults, then bounded disk-full/read-only scenarios on disposable remote storage. |

Those stage outcomes follow the current recovery code; implementation must bind each stage to the exact before/after call and confirm them against existing recovery regressions. `SIGKILL` is process-crash evidence, **not power-loss evidence**. Real power-loss durability would need a separately scoped storage/VM fault experiment; do not claim it from this work.

Recovery checks: completed checkpoint obligations survive; uncheckpointed replies may disappear; atomic list transactions recover all-or-none; all rows belong to a permitted snapshot cut; no duplicated or unattempted append; matching WAIF identity/alias relationships; restored task obligations consistent with the same capture. Do not accept arbitrary attempted tokens as a sufficient oracle. For example, a two-key unknown atomic transaction recovered on only one key is invalid despite both tokens being individually allowable. Keep clean first-generation, prior-generation, identical-portable-dump/different-WAIF-identity, and double-crash cases separate.

The executable snapshot oracle is scoped to one process lifetime, with a separately recorded durable starting state S0, and recovery observation R **before admission resets rows or new workload tasks run**. Initial fixture setup must be checkpointed before its rows are assumed durable. In the task lane, continuations remain behind a test-owned driver until recovery observation is complete; do not let overdue tasks change R before the checker reads it.

1. Use unique `(epoch, client, operation, micro-operation)` tokens and maintain the touched-key list in the harness. S0 must be an exact prefix of R per append row. Strip that verified prefix to obtain D, the recovered additions; never silently strip a mismatching prefix.
2. Track completed checkpoint A and, at most, one later pending checkpoint F. A includes the database **and sidecar** digest. A frozen publication manifest plus recovered pair identifies A, F, or the unchanged input. Equal pair digests can make two cuts observationally identical: accept if either permitted cut satisfies the constraints; do not guess from a database hash alone. An unrecognized pair is a recovery finding. A completed log without a safely captured pair digest is incomplete evidence, not proof of failure.
3. For selected cut C, every successful single-slice append transaction completed before its request must be wholly present in D. For completed A, exclude transactions first invoked after its completion acknowledgment; for pending F, use its externally observed capture boundary if available, otherwise the weaker kill-time upper bound. With no checkpoint request/publication, D must be empty. A signal request alone never counts as completion.
4. For each admitted atomic transaction T, intersect its append tokens W(T) with D: the result is empty or W(T), never a subset. Preserve `info` transactions as either outcome. Exclude definitely unsent writes. Apply this only to bounded, single-slice, non-error workloads with no yield or irreversible boundary; native-error and multi-publication scenarios require their own publication model. An unseen exception cannot be assumed to roll back.
5. If an included successful T read a current-epoch token, require the producing transaction's included writes too. All successful append-row observations must be prefix-comparable with R; repeated tokens and invented tokens fail. Extend the per-epoch Elle history with the recovery snapshot as a read-only observation after verified S0 normalization, so cross-row dependency cycles are checked as well. A recovered snapshot is an observation of the selected historical cut, not a fresh wall-clock read; impose no new strict-real-time ordering.
6. Never require R to contain acknowledged writes lost in an earlier epoch's legal rollback. Start the next epoch's bounds from its actually selected checkpoint. Report exact violated token/transaction/prefix constraints and preserve the histories needed to assess them.

These rules are a specification to test, not a completeness proof. Analyzer fixtures must include partial unknown transactions, mixed snapshot cuts, an unacknowledged but valid next checkpoint, equal portable dumps with different WAIF identities, lawful rollback, and a post-capture mutation. Keep task/effect oracles separate from this append-only checker.

Acceptance: every named boundary reached in a bounded remote run, observed process death and recovery, all semantic checks pass on healthy code. Designated test-only mutations for mixed-pair acceptance, lost checkpointed update and duplicate recovered token must each be rejected with the intended witness. Saved-artifact fixtures alone test the analyzer; at least one corresponding real binary mutation must test the complete adapter/checker route.

### 3. Generate transactions that reach distinct storage paths

Owners: extend existing Jepsen workload functions and Python MOO generation; factor any genuinely shared MOO source from existing Elle tests into a single fixture owner only when both consumers need it. Do not copy the full Go corpus into Clojure.

| Workload | Concrete generated operations | Oracle / targeted failure |
|---|---|---|
| Property lists | Independent object/property rows; 1–4 reads/appends including repeated reads, read-your-writes and intermediate writes; sparse and cross-key dependencies. | Elle serializable histories; lost update, write skew, torn reads, intermediate/duplicate values. |
| Keyed maps | Direct `object.map[key]` reads/writes, disjoint and shared keys; mix keyed writes with whole-map reads/replacement, absent entries, inherited slots, captured-map/index-expression cases. | Elle plus exact map/value invariants; exercise keyed validation, live-map rebase, metadata, presence and capture paths. No whole-map audit ledger on every operation. |
| Definitions and authority | Known nonwizard reader/writer identities; concurrently revoke read/write/owner permissions, add/delete definitions, clear inherited values, replace/shadow verbs and reparent. | Observed values/explicit expected MOO errors; marker dependency cycles where sound; separate authority invariants. Wizard is setup/control only in this workload. |
| Relationships and lifetime | Move/reparent/create/recycle while snapshot observers enumerate contents/children and use inherited dispatch. | Reciprocal topology, no invalid surviving reference, conserved unique token set, and selected append witnesses. General predicates require explicit invariant checks, not a claim that Elle covers them. |

Generate topology, operation shape, client identity and fault schedule from recorded seeds/operation streams. Start with transaction lengths 1, 2, 4, 8; client counts 1, 8, 32; hot sets 2, 5, 32; same-key versus disjoint versus skewed access; GOMAXPROCS 1, 2, 4. These are proposed exploration values, not measured optimal settings. Use a covering schedule rather than the full Cartesian product. Include no-fault runs and moderate contention: saturating one shared property can suppress useful concurrency. Stop/rotate an epoch before history/memory limits erase its observability.

Acceptance: every selected shape has observed successful read/write coverage; map scenarios prove actual keyed access and absence of the old common-ledger dependency. Adapt the existing Elle faults (`map-key-rebase`, `map-key-presence`, `map-key-metadata`, `verb-memo`, etc.) only for the corresponding test-binary workload, require a matching witness, and retain the existing in-process Elle tests. Check serializable; do not silently strengthen to strict serializable or assume a multi-slice MOO command is one transaction.

### 4. Cover task persistence and externally visible effects

Owners: scenario verbs in the existing workload and adapter; production paths `TaskSnapshots`, `LoadQueuedTasks`, `LoadSuspendedTasks`, and the current VM persistence code. Read current conformance fixtures and verify uncertain MOO semantics against the documented managed WSL Toast oracle before writing expectations.

Create test-owned tasks with unique logical tokens: delayed fork; suspended continuation with locals/stack and resume value; task-local data; anonymous objects and WAIFs reachable only through continuation state. Hold them in a known persisted state, checkpoint, restart, release through an explicit driver, and compare logical results, ownership/permissions, object identity/aliasing, and task counts. Mutate a verb after suspension, cancel/resume selected tasks, and recycle references only in independently admitted scenarios. Check the current deliberate program/source-mismatch policy separately: `task_load.go:41-52` drops that task and continues startup. A deliberately invalid task is not a lost valid task.

Observe output/effects with a test-owned receiver and attempt token while forcing validation conflicts. Within a live process, retries before the irreversible boundary must not duplicate an effect, and committed publication boundaries must be respected. Across checkpoint rollback, do **not** require universal exactly-once delivery: an effect after the selected checkpoint can legitimately repeat when its continuation is restored. First test a controlled checkpoint-stable pending task that has emitted no effect before the crash, then add explicit ambiguous intervals. Never put the whole fork/suspend/effect sequence into one Elle transaction; use actual slices/publications or a separate lifecycle checker.

Use two concrete existing paths: deferred `notify` (`builtins/network.go:883-932`) and `set_connection_option` with its irreversible descriptor (`:1304`, `:1486`). Keep the notification receiver connected in the exactly-once live-retry case: notify to a disconnected target is explicitly a successful no-op. Received tokens, not the builtin's success value, establish delivery. Use a separate test connection for option changes so response framing for the workload remains observable. Assert external delivery only under the scenario's connected-receiver conditions; connection-loss runs retain ambiguity rather than turning a successful command reply into a delivery acknowledgment.

Acceptance: each valid captured obligation either resumes once in the controlled scenario or is explicitly killed according to verified semantics; wrong wake value, lost task, duplicated continuation, wrong alias, and duplicated live-retry effect have distinct reproducible witnesses and deliberate controls. Keep raw errors and snapshot artifacts. A booting server alone is not success.

### 5. Exercise transport failures and bounded recovery

Use the existing adapter/SocketTransport boundary for before-send failure, disconnect after send, reply truncation/loss, slow reader, pause/resume, and repeated crash/restart. A fault after sending remains unknown even if the client gets an exception. Correlate late responses with the original ID; close a timed-out stream before reusing it. Avoid automatic client retries that hide duplicate execution. Faults affect only owned test connections/processes.

After healing, reconnect and demand useful read/write completion plus task/checkpoint progress within a declared bound. Record first-success latency and individual failure kinds; retain logs and existing diagnostics for stalled tasks. Establish bounds from healthy runs on the target runner, with an explicit bounded ceiling; do not encode an unmeasured throughput promise. No all-unknown interval counts as passing recovery. No network partitions between imaginary Barn replicas, global clock changes, or workstation stress.

Acceptance: known pre-send failures remain `fail`; in-flight failures remain `info`; a late reply cannot answer a newer request; an injected permanently stalled recovery is rejected by the progress check, with raw evidence independent of Elle.

### 6. Turn runs into sustained remote bug discovery

Keep one bounded PR/merge-queue smoke campaign in `jepsen.yml`, using `CI_RUNNER_LABELS` and its established fallback. Preserve the current check while expanding its contents; do not make this a new review ceremony. Initial proposed budget: the existing 20-minute job, reserving time for analysis and artifact upload. Measure before assigning more scenarios to this lane.

Put 30-minute exploratory batches on the existing dedicated `barn-bug-discovery` / `barn-fuzz` compute path, never Q's workstation. Use existing `fuzz_compute.cjs` routing and replenishment ownership, extending shared handling only as needed; do not build another scheduler/service or copy its runner-policy code. Verify live runner policy before activation. Existing runner availability, variables and throughput have not been checked by this planning task. Discovery runs never populate the standard tested-tree pass.

Rotate workload/fault/config combinations by **uncovered behavior and reproducible findings**, including healthy baselines. A saved defect is useful discovery and remains red; it does not stop unrelated bounded experiments. Setup failure, missing evidence or zero-work is incomplete, never an accepted healthy run. Continue only once the original artifacts are safely retained. Keep total concurrency within existing dedicated limits.

Evidence bundle: original Jepsen history; incremental adapter journal; generated operation stream and MOO; immutable original crash artifact set; pre/post recovery hashes; all profile manifests/commands; fault reach/release/kill events; outcome/coverage census; checker stdout/stderr/verdicts and cycle witnesses. Do not overwrite raw evidence when reducing or reanalyzing.

Replay has two honest levels: deterministic offline checking of the same recorded history/artifacts, and a fresh execution of the same generated operations/fault choices whose OS schedule may differ. Start reduction with the existing Elle witness, `--analyze`, and a replay of the preserved full crash-artifact set through Barn's loader. Only after an actual failure resists those tools, add an execution reducer that removes irrelevant clients/operations/objects/faults while retaining the same failure predicate. Preserve initialization and IDs needed for traceability. Re-execution must report reproduction frequency; a fixed seed does not promise the same interleaving. Promote minimized cases to the existing Elle/checkpoint/conformance owner appropriate to the defect, then keep a small end-to-end case in Jepsen.

Report yield as distinct reproducible Barn defects and fault classes, time to first witness, time to a minimal reproduction, exercised phase/shape combinations, unknown-outcome fraction and incomplete-run fraction. Do not count negative controls as bugs or CPU hours as success. Initially archive draft finding records; external issue/comment/repair dispatch under Q's name requires the applicable explicit posting authorization.

## Delivery order and done criteria

Implement in sequential independent PRs against then-current master, after authorization; merge each before starting a dependent PR. No stacked PRs.

1. Incremental operation evidence and final observations; property rows across objects and direct keyed-map variants; pin the actual transport/fixture inputs. These can be two small sequential PRs. Preserve current verdict requirements and add only each scenario's necessary checks.
2. SIGALRM live checkpoints plus ordinary-binary stop/classify/kill and actual-artifact recovery, with the explicit snapshot oracle and post-recovery probe. This is the first full bug-discovery tranche; it does not wait for a reporting framework.
3. Deterministic stage pauses, targeted real-binary negative controls, double-crash recovery and scenario coverage reporting; complete the bounded checker result taxonomy as needed here.
4. Controlled task persistence/effects; authority/definition/topology scenarios; transport faults. Deliver independently and give each its own semantic controls rather than one oversized PR.
5. Bounded remote exploration/retention through existing compute owners. Begin with witness and artifact replay; defer an execution reducer until a finding demonstrates the need.

Every implementation PR carries its actual workload, observation-based checker, deliberate rejection evidence, raw artifacts and bounded CI execution together. CI runs remotely under the repository rules; changes to isolation boundaries also require the pinned Elle workflow and nonzero path counts. No local campaigns are needed for this plan. A phase is complete only when its healthy scenarios and intentional negative controls have the expected **specific** results, its evidence survives failure, and the authorized change is merged.

## Collaboration decisions

Claude's addendum supersedes its initial unsupported "vacuous crash half", "parallel commits never occur", workstation-runner, strict-serializable, generic ERROR, and cross-epoch exactly-once claims. The initial report remains intact as collaboration evidence; use this final plan and its addendum for decisions. We adopted the direct SIGALRM request, smaller self-stop pause mechanism, early spread/keyed workloads and real recovery, and deferred a bespoke reducer. The parent tightened the proposed oracle to use pair digests, allow indistinguishable cuts, prevent startup tasks from changing the observation, and exclude native-error/multi-publication commands from atomic all-or-none assumptions. No new user design decision is required to implement the bounded first tranche.

## Unverified before implementation

- Exact managed Toast behavior for newly introduced task/permission/signal scenarios; this is an implementation research prerequisite, not an assumed contract.
- Healthy timing distribution, useful rates, history size and memory ceiling on the dedicated runner; the numerical grid/budgets above are starting proposals.
- Live runner-group admission/variables, current GitHub checks and any newer overlapping work. No claim of current CI health or runner readiness is made here.
- No real new Barn failure has been reproduced by this plan. Existing format/Elle regressions provide targets for proving that broader end-to-end tests can reject real defect classes.
