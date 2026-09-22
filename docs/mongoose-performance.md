# Reproducible Mongoose workload measurements

Run `scripts/bench_mongoose_matrix.py` **inside Linux**, with both native
executables. It rotates engine order across rounds and starts every world from
a fresh copy of the same database and sound database. Servers run in separate
Linux directories; the source checkpoint is never opened by a server.

Use the Mongoose branch's PROMOTE_NUMBERS Toast build for this real-world
workload. The canonical stock Toast conformance oracle is a different build;
it is not interchangeable for the mixed-numeric PBT suite.

From the Barn checkout in the verified Debian WSL distribution:

```bash
set -euo pipefail
run_root=$(mktemp -d /tmp/barn-matrix.XXXXXX)
go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn
go build -o "$run_root/barn" ./cmd/barn
go build -o "$run_root/tz" ./cmd/mongoose_tz
export MONGOOSE_USERNAME=q
read -r -s -p 'Mongoose password: ' MONGOOSE_PASSWORD
export MONGOOSE_PASSWORD
python3 scripts/bench_mongoose_matrix.py \
  --db mongoose.db.new --sound files/sqlite/sound.sqlite \
  --barn "$run_root/barn" \
  --toast /root/src/toaststunt-mongoose-login-20260917/build-release/moo \
  --tz "$run_root/tz" --out "$run_root/results" \
  --lanes toast,barn-g4,barn-g32 --rounds 3 --samples 10 \
  --pbt-samples 3 --levels 1,4,8 --operations 20 --iterations 2000
python3 scripts/report_mongoose_matrix.py "$run_root/results/matrix.json" \
  --output "$run_root/report.md"
unset MONGOOSE_PASSWORD
```

The output directory must not exist. Use `--lanes toast,barn-g4,barn-g1` for a
one-worker comparison instead of the 32-worker configuration above.
Credentials are neither written to the
manifest nor passed to server children. Preserve raw results even when the
driver exits nonzero: failures are evidence, and the reporter excludes errored
worlds from timing comparisons. Reports can also be generated after an interrupted run
from the saved per-world `result.json` files. Raw command transcripts can contain
world/player information; keep them local rather than committing them.

The driver covers the literal empty `$list_utils:range(10000)` loop, counted
range loops, integer and float arithmetic, list building/indexing, string
concatenation, map updates, property reads, verb calls, and create/recycle work.
Each interpreter result must exactly match its expected value. Server elapsed
time wraps `eval()`, so it includes compilation and any suspension inside that
call. TCP wall time additionally includes command dispatch, queueing, output,
and client parsing. Neither clock is a CPU measurement.

PBT runs first, before lifecycle churn can leave inherited Mongoose hooks
running. Its own terminal results table is the completion signal; success
requires exactly 65 passed out of 65. Both the suite's reported time and client
wall time are saved. No additional polling task is inserted into the world to
wait for PBT. Randomized PBT data and existing background activity remain live.

Concurrency uses distinct logged-in accounts and connections, synchronized
client starts, and fixed operation counts. Read-only transactions share an
object; disjoint writers use separate objects; contending writers share one
counter. Each write request performs repeated increments without `suspend()`.
Every successful request must return correctly, and final counters must equal
the total requested increments. This exercises commit/conflict handling and
catches lost updates; it is not a full serializability proof. The measurements
include parsing and network costs, not just the store transaction implementation.
Process CPU and debug-counter snapshots cover the complete case, including
counter initialization and validation, and include concurrent background work.

Fixture preparation is outside timed samples: temporary accounts, wizard and
programmer flags on their players, `cloaked` when absent, private
counter objects, and foreground limits of 10,000,000 ticks / 60 seconds.
Original limits and selected players are recorded. `--login-trace` optionally
exposes the exception swallowed by `#10:welcome`, only in the disposable world.
Concurrent clients use a temporary `matrix_eval` command verb with a tagged
reply. This avoids existing players' different eval features and formatting
preferences; it still executes through normal input dispatch and `eval()`.

The default port is 17920, so `$prod()` is false; this matches the earlier
non-production account probes, not a production-port startup. SQLite login-hook
problems, task restore differences, and background jobs are not silently fixed
or removed. `--port 7777` selects the production startup path when that port is
free, but it defines a different experiment and must be reported separately.

`barn-g1` sets `GOMAXPROCS=1`; `barn-g4` sets it to 4. Barn's default admission
capacity also follows worker count, so this is a combined runtime/admission
configuration comparison, not an isolated CPU scaling experiment. Toast uses
its normal execution model. Other host workloads can still interfere.

Use per-world medians as the independent repeated observations; ten samples
within one world are not ten independent fresh-world experiments. The report
shows median and range across world medians rather than claiming statistical
significance from a small trial. Compare only identical corpus/settings and
successful cases. Keep baseline binaries frozen when testing an optimization.

For a paired concurrency-only follow-up, supply a second frozen executable and
skip the preceding interpreter/PBT phases. This avoids changing world age merely
because one executable finishes the earlier workloads faster:

```bash
python3 scripts/bench_mongoose_matrix.py \
  --db mongoose.db.new --sound files/sqlite/sound.sqlite \
  --barn "$run_root/barn-before" --barn-candidate "$run_root/barn-after" \
  --toast /root/src/toaststunt-mongoose-login-20260917/build-release/moo \
  --tz .tmp/mongoose-pbt-fixed-20260919/tz --out "$run_root/paired" \
  --lanes barn-g32,barn-candidate-g32 --rounds 4 \
  --cases '^$' --pbt-samples 0 --settle 10 \
  --levels 1,4,8 --operations 20 --iterations 2000
```

Both binary hashes are recorded. The first executable alternates each round;
every world receives the same source database and setup. Startup/background
activity and host interference can still differ, so retain every world's results.

Map microbenchmarks and retained-heap diagnostics can be run separately from
the live measurements:

```bash
go test ./types -run '^$' -bench 'BenchmarkMap' -benchmem -count 5
BARN_MAP_FOOTPRINT=1 go test ./types -run '^TestMapRetainedFootprint$' -v -count 1
```

The footprint probe forces garbage collection with many maps kept live; its
bytes are diagnostic observations rather than a portable test threshold. To
compare an older revision, copy only `types/map_update_bench_test.go` into an
isolated checkout of that revision. It uses the public map API. Do not run these
CPU-heavy probes alongside the live workload matrix.

Harness acceptance checks:

```bash
python3 scripts/test_bench_mongoose_matrix.py
```

Unchanged managed map conformance checks, including capability admission:

```bash
bash scripts/check-map-conformance.sh toast \
  /root/src/toaststunt/build-release/moo "$run_root/conformance"
bash scripts/check-map-conformance.sh barn \
  "$run_root/barn" "$run_root/conformance"
# Optional focused follow-up; capability admission remains selected.
MAP_TEST_SELECTOR=first_last_index bash scripts/check-map-conformance.sh barn \
  "$run_root/barn-before" "$run_root/baseline-conformance"
```

Run conformance separately from timed measurements. A failure on the oracle
is a recorded expectation disagreement, not permission to weaken an assertion
or count a faster failing run as a performance improvement.
