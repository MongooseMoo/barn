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
git -C /tmp/barn-elle-cli checkout --detach 3c2504815945cd4ab1b5cc0142d2fe206e1ee285
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
The driver also replays the saved checkpoint drop-append history from CI run
42981299875. Elle 0.2.7 deadlocked on this input; the pinned Elle 0.2.8 must
reject it with an explicit `false`, exit 1 and an anomaly witness. Its input,
checker log and witness are retained with the generated histories.
Replay one workload with `--seeds 412 --rounds 40`; a seed reproduces generated
operations, while worker scheduling can change. Preserve the original EDN to
replay the exact observed history. `results.json` records the checker commands.
The existing `Elle isolation` workflow also accepts `seeds` and `rounds` on a
manual dispatch. For example, run a bounded larger campaign with:

```sh
gh workflow run elle.yml --ref master \
  -f 'seeds=412 395 396 397 398 399 400 401' -f rounds=200
```

PR and merge-queue runs retain the three-seed, 40-round defaults. Manual runs
use the same pinned checker, negative controls and evidence archive; no recurring
campaign is scheduled. Choose a bounded campaign size within the workflow's
25-minute limit and preserve a failed run's evidence before changing that size.
Manual runs do not record a tested-tree pass for the standard PR configuration.

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
| `ring-reads` | property-value validation only for the four ring properties | held four-transaction ring |
| `map-key-metadata` | owner and permission validation for keyed accesses | nonwizard map permission and ownership revocation pairs |
| `map-key-presence` | validation of an absent map entry | missing-key read/insertion pair |
| `map-key-whole-read` | promotion to a whole-property read after a keyed write | whole-map observation after writing another entry |
| `hot-map-carry` | cloning keyed reads into the transaction renewed at a stale hot read | held map/hot-property pair |
| `hot-memo-guard` | checking dispatch shape changes since the pre-renewal snapshot | held missing-dispatch/hot-property pair |
| `property-scans` | validation of enumerated property definitions | definition enumeration/add pair |
| `property-shapes` | validation of absent property lookup's fall-through | absent property/create pair |
| `verb-scans` | validation of verb-list generations | verb enumeration/add pair |
| `verb-memo` | validation of the dispatch shape clock after a memo hit | warmed dispatch/shadow pair |

The append workload also runs sixteen held definition pairs. Each row has exactly
one transition from `{}` to its unique token: creation, deletion or rename of a
definition; inherited override, clear or reparent; dispatch code replacement,
shadow, deletion or reparent; and numeric verb selection after an earlier verb is
deleted. Presence predicates normalize the actual lookup result, as the valid row
does; property and dispatch reads return the actual stored list. The mutator reads
a separate append ledger before changing the definition, while the observer reads
the definition and stages an append to that ledger in one transaction. Each successful observer records
`definition/NAME` in the manifest. The memo pairs check that the observer really
hit the warmed dispatch memo before releasing their mutator. Other dispatch pairs
stage a write first to exercise precise ancestry scan validation. The ledger is
not the hot-renewal property, so renewal cannot mask a missing definition check.

The hot-renewal pairs retain one reader publication. A reader observes a map entry
or a warmed missing verb, waits for a writer to publish that dependency together
with an append to the hot property, and then reads the hot property. Healthy
renewal refuses the changed carried dependency and keeps the old snapshot. Each
fault instead permits renewal: the reader observes the old dependency and the new
hot value. The map pair uses the existing `map-a` row. The dispatch pair adds a
`hot-memo` row, initially empty while the verb is missing and then the literal list
returned by its newly created verb. Committed pair coverage is reported as
`hot_map_pair` and `hot_memo_pair`; the specific fault also requires that renewal
actually happened. Tokens 210 through 213 are reserved for these pairs.

Irreversible publication is different: the runtime holds the exclusive commit
gate through the remainder of the slice. An ordinary competing commit cannot be
inserted inside that interval. Recorder regressions check separate effect,
suspend, fork and exception publications, retry of a resumed slice, and a failed
publication after an earlier successful one. A store regression checks that a
keyed dependency is converted to a whole-property dependency at renewal, accepts
its own republished map, and still detects a later change. These are semantic
regressions, not additional Elle anomaly witnesses or MOO scheduling-order proof.

Eight additional held pairs exercise keyed reads against whole-map replacement,
whole-map reads against keyed writes, whole reads after keyed writes, missing
entries, inherited map entries, clear overrides, and nonwizard reads across
property permission or ownership changes. Each pair owns a separate map and
marker. Its observed row changes once from `{}` to a unique-token list; the
writer reads the marker before publishing and the held reader then appends to
the marker. Removing the reader's dependency exposes a cycle through values both
tasks actually observed. Missing-key `E_RANGE` and denied-read `E_PERM` are
recorded as the selected scenario's state, while other errors remain native
failures. Healthy runs execute all eight pairs and require committed reads and
appends for every pair row. The new fault runs execute only their precise
scenarios: metadata runs both nonwizard revocations, presence runs the missing
entry pair, and whole-read runs the whole-after-keyed pair. Existing faults skip
these additional map pairs and retain their original bounded witness schedules.
Every fault still requires a completed Go run and an Elle rejection with a witness;
each selected pair retains its final-state assertion and committed row counts.

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

The checkpoint path has its own pause points, listed with the call each one
follows in `internal/isolationprobe/checkpoint.go` (`checkpoint/capture` through
`checkpoint/recovery-database-restored`). A `barn` command built with this tag
and started with `BARN_ELLE_CHECKPOINT_STOP=<point>:<occurrence>` (occurrence
from 1, counted per point) writes one JSON line to stderr on reaching that
occurrence (`msg` `checkpoint stop`, `point`, `occurrence`, `pid`, `artifacts`)
and then sends `SIGSTOP` to itself. The process that owns it resumes it with
`SIGCONT` or kills it. The stop is sent to the calling thread (`tgkill`), so
the process stops before the code after the point runs; a process-directed
`kill` can be delivered to another thread and let the checkpoint continue
(Jepsen CI run 42964137644 froze every rename stop with both renames done).
An unknown point or a non-Linux platform is a startup error.

Three checkpoint faults serve the Jepsen stage schedule's negative controls
(`scripts/jepsen_checkpoint.py` `CONTROLS`): `checkpoint-mixed-pair` makes
recovery keep the next database without its WAIF identity sidecar instead of
restoring a pair (`db/format/checkpoint_publication.go` `recoverCheckpointPair`);
`checkpoint-drop-append` and `checkpoint-duplicate-append` make the writer omit
or repeat the last token of each nonempty list in a property named `jp*`, the
Jepsen property rows (`db/format/checkpoint_fault.go`). Their tests in
`db/format/checkpoint_fault_test.go` skip unless `BARN_ELLE_FAULT` names them.

The Jepsen effects lane has one live-process control, `pending-effect-retry`:
the commit-conflict retry in `engine/task_runtime.go` publishes the lost
attempt's deferred effects instead of discarding them, so a notification from a
retried transaction reaches its receiver twice. The Jepsen run
`--control pending-effect-retry` passes only when the effect checker reports
`duplicated-live-retry-effect`.

## Task lifecycle ordering

The same CI driver runs `TestUncaughtHandlerBeforeYieldedParent` with healthy
behavior and with the `finished-task-index` fault. The test holds a real fork's
uncaught-error handler before its write, after the fork committed 194. A
concurrent scheduler pass must leave the yielding parent blocked. After release,
the handler must commit `{194, 1}` and the parent must read that value.
The fault drops killed tasks from the unfinished index before their physical
execution ends, reproducing issue 423's premature `{194, 0}` read. It exists only
in the `elle` build. The driver requires a completed healthy pass and a completed
fault failure with that specific premature-read witness; other failures do not
count as rejection. Both raw Go logs and commands/verdicts are retained.

This is an explicit ordering assertion alongside Elle, not an Elle isolation
anomaly. Serializable histories alone permit a read between the fork's commit
and its handler's commit; the model remains unchanged.

The ordinary conformance CI job also repeats the unchanged issue 423 scenario
in 20 managed sessions, each with canonical capability admission, while two
CPU load workers run. It retains each session's JUnit and admission evidence.

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

Ordinary-user authority also has focused transaction regressions in
`builtins/verbs_txn_test.go`. The existing
`TestDeleteVerbAuthorityRevocationConflictsBeforeMutation` checks object owner and
write-flag revocation during a staged deletion. The distinct
`TestVerbCodeReadPermissionRevocationConflictsBeforePublication` admits a code read
as a nonwizard/nonowner, removes only the verb's read bit, and checks a dependent
marker cannot publish from that stale permission view. It requires a conflict on
the selected verb, not merely its list generation, and a serial retry returns
E_PERM. These are semantic regressions, not additional Elle graph witnesses.

Map byte quotas are a separate semantic invariant: a serializable merge of
disjoint keys can still exceed the task's value limit. The
`TestKeyedMapRebaseRespectsTaskValueQuota` regression in `vm/map_key_test.go`
stages real VM property assignments from the same snapshot. It checks preservation
of both updates below the limit, validation loss without publication at or above
the limit, and E_QUOTA when the rejected assignment is rerun against the committed
map. These outcomes are not Elle dependency-graph witnesses.

This is a bounded isolation corpus. It provides no performance or liveness
claim and does not close the Mongoose home-command timeout investigation.

The corpus also runs a held four-transaction ring. Each task reads the row its
predecessor will append to before any task can write, then appends to its own row.
The `ring-reads` fault removes validation only for these properties, leaving the
other waves healthy so its witness must come from the ring.

Healthy random rounds vary one to four slices per task and one to four appends
per generated append body, including repeated appends to a key and sparse reads.
Register bodies alternate blind writes, read-only/repeated reads, cross-key
read/write dependencies, intermediate writes followed by own reads, and full reads
around a write. Every shape must appear in committed coverage. Random writes use
unique increasing tokens starting at 2^40, separate from held schedules; forked
operations use the same allocator. Seeds reproduce generation on the same tree,
not scheduling or histories produced by an older generator.
