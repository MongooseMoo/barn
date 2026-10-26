# Elle isolation checks

Run this check when changing transaction validation, publication, hot renewal,
relationship or definition reads, dispatch memoization, or task retry/suspension.
It drives the real VM and runtime with four admitted workers, then checks the
external MOO observations with [Elle](https://github.com/jepsen-io/elle).
Ordinary `go test ./...` runs the recorder regressions and skips history generation.
Java is needed only by the separate checker job, never by Barn.

## Models and evidence

The append history contains property values, contents, children, property
definitions, verb definitions, verb code, names, inherited property values,
a property of one WAIF that a persistent property holds, and two entries of
one map-valued property, read and written as `#0.elle_map[key]` so the store
tracks them per key.
Fresh objects, definition names and literal lines carry unique integer tokens.
MOO normalizes each row into a list. A single-threaded preflight appends twice
to every row and asserts exact order before concurrent work starts.

The register history contains blind writes to a property, name and verb code,
with unique values. Reads span keys in each transaction. Both models request
`serializable` explicitly; the CLI's default is stronger and inappropriate here.
Each publication uses a separate Elle process, so the checker does not impose
session ordering on old read-only snapshots. Wall-clock ordering, store versions,
read timestamps, task return values and generated object IDs are not correctness
witnesses. The witness contains values MOO actually read or wrote.

An observation is buffered in the current attempt and published through the
runtime's existing commit-deferred effect mechanism. This divides histories at
successful slice commits and at publications inside irreversible-effect
boundaries. Hot renewal carries the same pending observations. Discarded retry
attempts are omitted. A native exception can still publish writes and is `ok`.
An opt-in runtime outcome hook identifies failed commits as `fail`; other unconfirmed
attempts are `info`. The generator fails if either is present in a completed run.
Recorder regressions exercise these boundaries independently of Elle.

Concurrent mutators also check location/contents and parent/children reciprocity,
fixed-token conservation, and absence of recycled references. They move and
reparent objects, create/recycle temporary objects, delete properties/verbs,
clear inherited slots, and change flags, owners and permission metadata.
Scratch definitions and overrides exist in committed state before deletion and
clearing; flags and owners alternate across rounds. Read-only
snapshot tasks run the same invariants during the workload and at the end.
Inherited dispatch runs alongside ancestor verb additions and reparenting.

The manifest requires nonzero actual committed paths: parallel and coarse
exclusive commits, exclusive gate commits, hot renewal, verb memo hits, suspends,
forks, native exceptions and read-only observations. It also reports every
invariant operation and every committed append/register key. Probe counters are
coverage evidence only. They do not participate in Elle's correctness graph.
The synchronized hot reader and append waves make those paths reproducible.

Before the random rounds, the append workload runs eight synchronized waves and
three held pairs. In a wave, four tasks read the same row, wait at a barrier until
all four have read, then append: property value, name, verb code, contents,
WAIF property and one map entry, one wave for each validation stage. In the
seventh wave two tasks append to map entry `a` and two to entry `b`; a commit
to one entry keeps the other's appends only because the map it publishes is
rebuilt on the live one. In the eighth wave the map is captured before its index
expression writes the same entry. Each task observes the captured list, then
appends to that list; only the final list is published. The read must keep its
snapshot dependency even though index evaluation has now written the entry.
After all waves drain, one ordinary task appends fresh tokens to both map
entries. This extends both observed prefixes, making a lost suffix conflict
with append order instead of relying on the timing of a final read-only
observation. The concurrent waves retain their independent keyed accesses.
In the write-skew pair, each task reads the
row the other appends to, and both are held after validation, before
publication, until both have validated or two seconds pass. A healthy commit
excludes the second task while the first is held, so the first times out and
publishes and the second loses validation and retries.

In the torn-snapshot pair, a writer appends to two rows in one commit and is
held once the commit has its timestamp, before it publishes anything. A reader
begins a snapshot inside that hold, reads the first row, lets the commit
publish, then reads the second row. It must observe both appends or neither.
No fault is needed for this one: it failed on the unmodified store until a
commit's timestamp became the clock only after its last image was published.

In the valid/recycle pair, the `valid` row reads `{}` while `#50` is valid and
`{50}` once it is recycled. A checker reads that row through `valid(#50)`, then
waits until a recycler has published before appending to the property row. The
recycler reads the property row after the checker's read, recycles `#50` and
appends 50 to `valid`. A healthy checker loses validation on `#50` and retries;
without the read `valid()` records, both commit and each misses the other's write.

For focused experiments, direct generation accepts `BARN_ELLE_PATHS` as a
comma-separated selection of `parallel,exclusive,exclusive_gate,hot_renewal,
verb_memo,suspend,fork`. The manifest records the selection and requires every
selected path to occur. These settings select workload features; actual internal
paths can also occur incidentally. The CI driver always enables all features.

## Run and replay

Use Linux, including WSL Debian on Windows. Run from the Barn checkout. Install
Go, Java 21, Leiningen and Graphviz. Build the same pinned CLI used in CI:

```sh
git clone https://github.com/ligurio/elle-cli.git /tmp/barn-elle-cli
git -C /tmp/barn-elle-cli checkout --detach 9c2a18b3719f573dab06ae05c955c9c9d5225673
cd /tmp/barn-elle-cli
JAVA_CMD="$JAVA_HOME/bin/java" lein uberjar
```

Then return to Barn and run:

```sh
python3 scripts/check_elle.py \
  --jar /tmp/barn-elle-cli/target/elle-cli-0.1.11-standalone.jar \
  --java "$JAVA_HOME/bin/java" --output /tmp/barn-elle-evidence
```

The output directory must be new. Defaults are seeds 412, 395 and 396, with 40
rounds each, plus one run of the first seed under each deliberate fault.
Replay one workload with `--seeds 412 --rounds 40`; a seed reproduces generated
operations, while worker scheduling can change. Preserve the original EDN to
replay the exact observed history. `results.json` records the checker commands.
Direct generation needs `BARN_ELLE_OUTPUT`, `GOMAXPROCS=4` and `-tags elle`:

```sh
BARN_ELLE_OUTPUT=/tmp/barn-history BARN_ELLE_SEED=412 BARN_ELLE_ROUNDS=40 \
  GOMAXPROCS=4 go test -tags elle ./engine \
  -run '^TestElleIsolationHistories$' -count=1 -v -timeout=5m
```

The dedicated `elle` build tag enables coverage hooks, pause points and the
`BARN_ELLE_FAULT` switch. Each fault removes one protection, and Elle must
reject the resulting append history with an anomaly witness:

| `BARN_ELLE_FAULT` | Removes | Made certain by |
|---|---|---|
| `property-reads` | property-value read validation | property wave |
| `scalar-reads` | name/owner/flags read validation | name wave |
| `verb-reads` | verb read, scan and dispatch-memo validation | verb-code wave |
| `relationship-reads` | location/contents/children read validation | contents wave |
| `waif-reads` | WAIF image read validation | WAIF wave |
| `map-key-reads` | validation of map entries read through keyed access | map-entry wave |
| `map-key-rebase` | rebuilding a keyed write's map on the live one at commit | two-entry wave |
| `map-key-capture` | dependencies for maps captured before an index expression writes the entry | captured-map wave |
| `commit-exclusion` | the commit mutex: one commit validating and publishing at a time, only during the held pair | write-skew pair |
| `valid-reads` | the read a true `valid()` records on the object | valid/recycle pair |

A fault run is the preflight, the waves, the held pairs and the final read, with
no random rounds and no coverage requirement. The witness is then the anomaly
that fault's interleaving made, on the row it targets. With the rounds left in,
a fault corrupts so much that Graphviz spends minutes plotting the cycles.
The commit-exclusion fault is active only around the held write-skew pair;
the other waves retain publication serialization, including the WAIF wave whose
image timestamps require it. The fault still must produce an Elle anomaly,
and every generated workload must finish without unknown publication outcomes.

A checker that has never been shown to reject a class of defect is not evidence
that the class is absent. When you add a protection to validation or
publication, add the fault that removes it, the workload that makes its loss
certain, and its name to `elleFaults` and to `FAULTS` in `scripts/check_elle.py`.
`isolationprobe.Pause` names a point inside the store where a workload can hold
a goroutine; use it when the interleaving you need is otherwise microseconds
wide. Normal server builds compile away the hooks, the pause points and the
faults, and cannot disable validation through this environment variable. Never
ship a server built with this checker tag.

The driver requires literal `true` and exit zero for every healthy history,
and `false`, exit one and an anomaly witness for every fault append history.
`unknown`, missing verdicts, unexpected subprocess exits, incomplete Go runs
and missing coverage all fail. An inconclusive verdict need not have a nonzero
exit status, so checking exit status alone is wrong.
CI uploads histories, seed manifests, raw Go/Elle logs, cycle text/graphs and
results even on failure.
The artifact contains `elle-evidence.tar.gz`; extract it with
`tar -xzf elle-evidence.tar.gz` on Linux/WSL. This preserves Elle's original
HTML filenames, which can contain quotes rejected by GitHub's artifact uploader.
To investigate, start with the Go log and manifest,
then read the anomaly's transaction edges in the `.txt` witness and inspect
its SVG. Preserve native errors; do not catch them or weaken the model to
obtain a green result.

This is a bounded isolation corpus. It provides no performance or liveness
claim and does not close the Mongoose home-command timeout investigation.
