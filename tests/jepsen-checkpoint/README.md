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

`--workloads` selects the row families the clients mix (default `smoke`; CI
runs `smoke,props,keyed`). Each family has its own Elle list-append generator
over its own key range (`FAMILIES` in `scripts/jepsen_checkpoint.py`, passed to
this project as `--families`):

- `smoke`: entries of the map `#0.jepsen_rows`, mirrored in the durable
  `#0.jepsen_ledger`; every transaction copies and rewrites both properties.
- `props`: one list property per key on four row objects, read and appended
  directly, with repeated reads, read-your-writes and intermediate writes.
- `keyed`: entries of the map property `jm` on a child holding its own map and
  on a child whose slot is inherited and clear. By operation id, transactions
  use keyed `o.jm[K]`/`maphaskey(o.jm, K)` access, a captured map whose index
  expression writes the same entry, or a whole-map read and replacement.
  `vm/jepsen_keyed_rows_test.go` runs the generated keyed MOO
  (`scripts/testdata/jepsen_keyed_txns.json`) and shows two transactions on
  different entries both commit, which a whole-map pair cannot.

Each generation's admission creates fresh row objects, so every family's
rows start empty in each generation. Only smoke rows are compared with the
checkpoint ledger.

A checkpoint is a graceful owned SIGTERM whose database/WAIF pair the owner
verified; the baseline is the ledger read just before it. Every later
generation starts from a copy of the latest verified pair. Disconnects and
timeouts are `:info` unknowns.

The live phase (`--phase-live`, default 60 s; 0 disables it) checkpoints
while clients continue. A `live-checkpoint` sends SIGALRM, which calls Barn's
`requestCheckpoint` directly, and is acknowledged only by the `checkpoint
complete` log record plus the stable digest of the published
`.new`/`.new.waifids` pair; the next request waits for that (Barn queues one
request and drops further ones). A `checkpoint-crash` reads the generation's
rows, sends SIGALRM, waits a seeded random delay (up to 1.5x the last
checkpoint's duration), stops the owned process (state T), records the frozen
publication stage from the files (`not-staged`, `staging`,
`sidecar-prepared`, `journal-published`, `database-renamed`, `both-renamed`,
`cleanup`), kills it, copies every artifact to `gen-NNN-forensic/`, and
restarts from the interrupted checkpoint's actual output path in place, with
its journal, saved generations and sidecar together (or from a copy of the
unchanged input when no output or journal exists). Before admission, one task
reads the epoch marker, the queued task count and every recovered row. The
snapshot oracle then requires: the recovered pair is the one the frozen stage
permits and equals the unchanged input, the completed checkpoint A or the
pending checkpoint F; for that cut, every write acknowledged before its
request is present and nothing first sent after A's acknowledgment is; each
transaction's writes are recovered all or none; an included transaction's
reads are included; every read is prefix-comparable with the recovered rows;
no repeated or never-sent token. The recovered rows are also appended to the
crashed generation's Elle history as a read-only observation. SIGKILL is
process-crash evidence, not power-loss evidence.

The final checkpoint follows a 6 s client-only closing phase, so the
generation a crash or checkpoint-crash restarted as the last time limit
expired still gets client transactions before it is checkpointed.

The stage schedule (`--stages all` or a list of `STAGE_SCHEDULE` steps; needs
Barn built with `-tags elle`) replaces the fixed and live phases with one
client phase per step (`--stage-seconds`, default 10). Each generation is
launched with `BARN_ELLE_CHECKPOINT_STOP` set to its step's point and
occurrence (occurrence 2 means after one completed live checkpoint, so a
previous pair exists). Its `stage-crash` requests the checkpoint, waits for
the process's own `checkpoint stop` record and /proc state T, records the
reached point from that record, then classifies, kills and restarts exactly as
a `checkpoint-crash`, so the snapshot oracle applies unchanged. The
`double-crash` step also starts the restarted Barn with a stop between its
recovery's restore renames, kills it there, keeps that second artifact set in
`gen-NNN-recovery-forensic/`, and recovers the same files again. `results.json`
`coverage` has one row per step: requested and reached point, stopped state,
frozen stage, kill exit, recovered cut and verdict; a selected step that was
not reached, stopped, killed and recovered is incomplete, a record from another
point or process is invalid, recovery to a cut other than the plan table's or
a double crash that does not converge is a recovery failure, and unselected
steps are listed as `not-selected`.

`--control NAME` runs a negative control: its elle fault
(`BARN_ELLE_FAULT`, every process) and its one stage step. It exits 0 only
when that run's recovery reports the designated witness (`mixed-pair`,
`lost-checkpointed-update`, `duplicate-token`), and 6 otherwise.

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
PID, SIGKILL observed, verified checkpoint, successful reads and writes of each
selected family before and after each crash and checkpoint, the next
generation admitted from the
expected pair, final observations, progress probes) and each generation's
runtime log, and exits with one result class:

| Exit | Verdict | Examples |
|---|---|---|
| 0 | healthy | every obligation met, no finding |
| 1 | correctness-violation | Elle rejection, durability violation, panic record |
| 3 | recovery-failure | failed progress probe or observation, checkpoint or task-restore failure record, failed fault operation |
| 4 | incomplete-experiment | missing obligation, unexamined ERROR record, native error, Elle undecided, no history |
| 5 | invalid-harness-evidence | journal contradiction, adapter timeout, missing journal or provenance |

`results.json` lists every finding with its class, and `families` gives each
selected family's successful transactions with a read and with a write.

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
