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
timeouts are `:info` unknowns. A native MOO error, an unverified checkpoint,
an Elle rejection, a durability violation, zero checkpoint comparisons or
zero Elle-checked generations fails the campaign.

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

Evidence: `lifecycle.jsonl` (each generation's source pair hashes, baseline,
recovered ledger, server identity, stop/crash results), `history.jsonl`
(the Jepsen history with raw native replies), `gen-*/` (server logs and
profile manifests), `gen-NNN.edn` with Elle logs and witnesses, `store/`
(Jepsen's own store), and `results.json`.
