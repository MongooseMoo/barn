# Jepsen checkpoint durability workload

Concurrent Jepsen list-append clients and process faults against one
disposable Barn server on the public Test.db profile
(`barn-linux-testdb-outbound-on`). CI runs it in `.github/workflows/jepsen.yml`.

- This project (Jepsen 0.3.15, JVM `-Xmx2g`) schedules the transactions and
  the pause/resume/crash/checkpoint nemesis ops and records the history.
- `scripts/jepsen_checkpoint.py` owns every Barn process generation through
  `scripts/mongoose_barn_process.py` and reaches MOO through
  moo-conformance-tests' `SocketTransport`. It splits the history per
  generation, checks each with the pinned Elle CLI
  (`.github/actions/elle-cli`, serializable), and checks the recovered ledger
  against each explicit checkpoint baseline.

A checkpoint is a graceful owned SIGTERM whose database/WAIF pair the owner
verified; the baseline is the ledger read just before it. Every later
generation starts from a copy of the latest verified pair. Disconnects and
timeouts are `:info` unknowns.

Each client request carries a run-unique operation id. The adapter journals
the request, the send (generation, MOO, fault phase) before it reaches Barn,
and the completion before it replies; analysis reconciles that journal with
the Jepsen history and rejects any disagreement. An operation Jepsen stopped
waiting for stays `:info` in the generation it was sent to. Before every
crash and checkpoint the campaign reads every row its clients touched in the
generation (an extra read-only transaction in that generation's Elle
history); after every resume and launch it requires a fresh-connection write
and read within 30 s.

Analysis checks each scenario obligation (pause observed as a stopped owned
PID, SIGKILL observed, verified checkpoint, successful reads and writes before
and after each crash and checkpoint, the next generation admitted from the
expected pair, final observations, progress probes) and each generation's
runtime log, and exits with one result class:

| Exit | Verdict | Examples |
|---|---|---|
| 0 | healthy | every obligation met, no finding |
| 1 | correctness-violation | Elle rejection, durability violation, panic record |
| 3 | recovery-failure | failed progress probe or observation, checkpoint or task-restore failure record, failed fault operation |
| 4 | incomplete-experiment | missing obligation, unexamined ERROR record, native error, Elle undecided, no history |
| 5 | invalid-harness-evidence | journal contradiction, adapter timeout, missing journal or provenance |

`results.json` lists every finding with its class.

Run (Linux, Java 21, from the repository root):

```bash
(cd tests/jepsen-checkpoint && lein uberjar)
uv run --project ../moo-conformance-tests --frozen python scripts/jepsen_checkpoint.py \
  --barn bin/barn --db ../moo-conformance-tests/src/moo_conformance/_db/Test.db \
  --jepsen-jar tests/jepsen-checkpoint/target/barn-checkpoint-0.1.0-standalone.jar \
  --elle-jar "$ELLE_JAR" --output jepsen-evidence
```

Re-check a finished evidence directory without launching anything:

```bash
python3 scripts/jepsen_checkpoint.py --analyze jepsen-evidence \
  --output jepsen-reanalysis --elle-jar "$ELLE_JAR"
```

Evidence: `provenance.json` (binary, configuration, source tree, checker and
Jepsen jars, fixture and transport hashes, conformance revision, runner, CPU,
filesystem, parameters), `journal.jsonl` (the adapter journal),
`lifecycle.jsonl` (each generation's source pair hashes, baseline, recovered
ledger, server identity, stop/crash results, probes, observations),
`history.jsonl` (the Jepsen history with raw native replies), `gen-*/` (server
logs and profile manifests), `gen-NNN.edn` with Elle logs and witnesses,
`store/` (Jepsen's own store), and `results.json`. CI checks out
moo-conformance-tests at the fixed `CONFORMANCE_REF` in
`.github/workflows/jepsen.yml`.
