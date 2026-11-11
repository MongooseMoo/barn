# Jepsen evidence journal (PR A)

## Goal

Implement PR A of `plans/jepsen-expansion.md` (section 1, "Make the existing
campaign's evidence and verdict precise", plus pinning from Delivery item 1):
incremental adapter journal, final observations, per-scenario obligations,
distinct result classes, per-generation log analysis, pinned
moo-conformance-tests revision and provenance. Implementation authorized by Q
2026-11-10.

## State

Read: plan, collaborator report, core.clj, jepsen_checkpoint.py,
test_jepsen_checkpoint.py, mongoose_barn_process.py, check_elle.py,
jepsen.yml, tested-tree and elle-cli actions, ci.yml suite pinning.

Findings from source:
- `analyze` (`scripts/jepsen_checkpoint.py:500-542`) reads only
  lifecycle.jsonl and history.jsonl; per-generation `gen-NNN/logs/latest.jsonl`
  is never read.
- Jepsen history is written only after the run (`core.clj:174-175`); an
  interrupted Jepsen JVM leaves no client evidence.
- `analyze:523-525` skips zero-:ok generations, and `:540` passes the run if
  any one generation was Elle-checked; the post-crash generation can be empty.
- jepsen.yml checks out moo-conformance-tests at its floating default branch;
  ci.yml resolves `refs/heads/main` per run and feeds it to tested-tree
  `extra`.
- The SIGTERM checkpoint ledger read (`checkpoint`, line 270) is already a
  fresh-connection read of every key; crash has no pre-kill observation.

## Decisions and why

- Journal: `journal.jsonl` written by the adapter, one line per event
  (`request` on receipt, `send` before `execute` with generation/port/MOO/
  phase, `complete` before the reply is written; `fault-begin`/`fault-end`).
  Each line is opened, written, flushed and fsynced. Jepsen sends an op id
  (shared AtomicLong) and the process; the adapter echoes the id; the
  completion carries `op-id`. Reconciliation fills an adapter-timeout op's
  generation from its `send` record (it stays `info`), marks ops with no
  `send` as definitely unsent, and reports every disagreement as invalid
  harness evidence.
- Final observation: before every crash and graceful checkpoint the campaign
  reads every row key any client touched in that generation (retired keys
  included) and the analyzer adds it to that generation's Elle history as a
  read-only transaction. After every resume and every launch a bounded
  fresh-connection write probe and read probe run (30 s ceiling, an
  unmeasured bound, recorded with latency). The admit ledger read remains the
  post-restart observation of every durable key.
- Pause is observed: `OwnedBarnServer.pause` waits (bounded) for the owned
  PID's /proc state `T`; crash is observed by exit code -9.
- Result classes and exit codes: correctness-violation 1, recovery-failure 3,
  incomplete-experiment 4, invalid-harness-evidence 5, healthy 0 (2 is
  argparse usage). All findings are listed; exit code follows precedence
  correctness > recovery > invalid harness > incomplete.
- Lifecycle `failure` rows: `jepsen`/`campaign` operations are invalid
  harness evidence; fault operations (pause/resume/crash/checkpoint/
  final-stop) are recovery failures. The raw message is kept either way.
- Logs: each generation's `logs/latest.jsonl`; panic messages are correctness
  violations, checkpoint/restore failure messages recovery failures (WARN
  restore failures included), any other ERROR an unexamined record
  (incomplete). `server.log` lines starting `panic: `/`fatal error: ` (an
  unrecovered Go crash) are correctness violations. This scenario declares no
  expected diagnostics.
- Pin: moo-conformance-tests at a fixed revision in jepsen.yml, passed to
  tested-tree `extra` in both check and record jobs; setup-uv pinned to the
  same SHA ci.yml uses. Provenance written to `provenance.json` at campaign
  start and copied into results.json by analysis.

Implemented (uncommitted at time of writing, then committed with this note):
`scripts/jepsen_checkpoint.py` (journal, reconcile, probes, observations,
obligations, log analysis, result classes, provenance),
`scripts/mongoose_barn_process.py` (`run_state`, observed pause),
`tests/jepsen-checkpoint/src/barn_checkpoint/core.clj` (op id, process,
reply id check), `.github/workflows/jepsen.yml` (pinned CONFORMANCE_REF,
tested-tree extra, pinned setup-uv), README, tests. Local:
`python -m unittest discover -s scripts -p 'test_jepsen_checkpoint.py'` 48
tests OK on Windows. `test_mongoose_barn_process.py` is Linux-only (33
errors on Windows before this change too); CI runs it.

Acceptance item -> test (scripts/test_jepsen_checkpoint.py):
- missing reply: `test_barn_missing_reply_stays_unknown_in_its_generation`,
  `test_adapter_reply_missing_stays_pending_in_the_send_generation`
- late reply: `test_late_reply_after_jepsen_gave_up_stays_unknown`
- unsent operation: `test_unsent_operation_is_excluded_and_a_sent_failure_is_a_contradiction`
- mixed generation: `test_mixed_generation_is_invalid_harness_evidence`
- no post-heal progress: `test_no_post_heal_progress_is_a_recovery_failure`
- zero actual fault: `test_zero_actual_fault_is_incomplete`
- missing final observation: `test_missing_final_observation_is_incomplete`
- exact `--analyze` replay: `test_analyze_replay_of_an_archived_result_is_exact`
- post-crash case: `test_post_crash_generation_without_success_no_longer_passes`
- result classes/exit codes: `test_exit_status_names_the_most_severe_class`
- logs: `test_runtime_log_records_are_classified_with_their_raw_record`
- provenance: `ProvenanceTests`

Not verified locally: the Clojure change compiles (no lein locally; CI's
`lein uberjar` step checks it); real campaign behavior (CI campaign).

## Surprises

- Jepsen 0.3 reopens a client after an `:info` completion (new process), so
  a timed-out stream is not reused; the reply-id check in core.clj is
  defensive.
- The old analysis refused to split any history containing an
  adapter-timeout op (no generation); the journal now places it.

## Next step

Commit, push, open PR, queue auto-merge, watch the CI campaign; if it
reports findings, read the raw records in the evidence artifact before
changing anything.
