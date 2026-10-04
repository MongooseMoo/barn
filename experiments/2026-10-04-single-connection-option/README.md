# Single connection-option lookup (#360)

## Preregistered comparison

Baseline production is `builtins/network.go` and `builtins/connection.go` at
published merge `7221a6cb690ab2f61c6a922381bd3fbeb70377cd`. Warm boolean option
reads allocate nine times with defaults and twice with stored overrides. The
regressions preserve default/override values, missing-name errors, player/session
isolation, snapshot independence, and concurrent reads/writes before the fix.

Add a private single-value getter that copies one immutable value under RLock,
then resolves only that option's default when the player has no stored map.
Keep the existing full-map snapshot implementation. Route boolean and named-value
reads through the getter without changing names, defaults, truthiness or authority.

Compare `BenchmarkSingleConnectionOption` on truthiness, held-input checks, and
full snapshots, each with default and stored options. Sessions/registries and
overrides are prepared outside timing; every workload checks its output. Use the
same WSL Go 1.24.6 toolchain and test source, compiled frozen binaries,
GOMAXPROCS=4, affinity 8-11, 200ms duration, twelve paired samples randomized from
seed 360, then two holdouts. Record time/bytes/allocations, raw order, hashes,
benchstat and paired intervals. Profile default truthiness allocations separately.
The target is zero allocations for warmed scalar reads; full snapshots remain
independent maps. These are getter measurements, not input/server throughput.

## Results

The private getter now serves boolean, held-input/flush-command and both named
builtins. Full snapshots still copy the map. Independent expected-value tests
cover all seven defaults and overrides, missing names, snapshot mutations,
player/session isolation and concurrent readers/writers. The scalar allocation
gate failed on the baseline (nine default allocations, two stored allocations),
and passes for default/stored scalar options, held-input and both named builtins.
Full builtins tests, race tests, vet and staticcheck passed; the final expanded
named-builtin allocation gate also passed under the race detector.

| Workload | Baseline B/op, allocs/op | Candidate B/op, allocs/op | Paired time change (95% interval) | Holdout |
| --- | --- | --- | --- | --- |
| Truthy default | 896, 9 | 0, 0 | -98.147% [-98.178%, -98.121%] | -98.070% |
| Held default | 896, 9 | 0, 0 | -98.133% [-98.146%, -98.123%] | -98.126% |
| Truthy stored | 400, 2 | 0, 0 | -93.364% [-93.413%, -93.319%] | -93.348% |
| Held stored | 400, 2 | 0, 0 | -93.800% [-93.836%, -93.767%] | -93.780% |
| Snapshot default | 896, 9 | 896, 9 | -0.152% [-1.307%, +0.999%] | +1.315% |
| Snapshot stored | 400, 2 | 400, 2 | +0.012% [-1.567%, +1.380%] | +1.480% |

All fourteen samples agree on the allocation counts. The unchanged snapshot
controls show no significant main-sample timing difference; their small positive
holdouts do not establish a regression. Scalar results are microbenchmark
measurements, with no server/input throughput or RSS claim. Frequency/governor
and other machine workloads were not controlled. Benchstat uses twelve main
samples; paired intervals bootstrap log ratios 10,000 times and holdouts are
kept separate. Raw samples, order, hashes and summaries are in `results/`.

Allocation profiles used the same frozen binaries, `TruthyDefault`, 10,000
iterations and memprofilerate=1, outside the paired timing run. Including the
single calibration call, baseline lookup stacks allocate 90,009 objects and
8,750.88 KiB through `getConnectionOptions`, `defaultConnectionOptions` and
`defaultIntrinsicCommands`. The candidate lookup focus has zero samples. Both
profiles also include untimed registry/compiler setup; those allocations are
not lookup costs. Full space/object profiles and focused lookup-object profiles
are retained as text; binary profiles remain outside the repository.

Test-source SHA-256:
`7431050bdd1bcc9f211ff8e09873d241e65fd6c408b78e51256e4b75e1da7542`.
Frozen baseline binary:
`9ad613a458ed9ef3087a27cdf517afcdac29e6e90bdd60dc48e756deaffd0529`.
Frozen candidate binary:
`568dcdb637b2efba5286f52e22104cfacdc75d631076c62d3d4e7aa96638c826`.
Both were built with WSL Go 1.24.6 using the same test source; baseline production
was supplied through a Go overlay from the published merge above.
Benchstat was built from golang/perf commit
`406019bb8b6893dd1245d31bf511c719619bb5c9`.

Reproduce paired measurements with the two frozen benchmark binaries:

```sh
python3 experiments/2026-10-04-single-connection-option/run_pairs.py \
  --baseline /tmp/barn-360-tools/baseline.test \
  --candidate /tmp/barn-360-tools/candidate.test \
  --benchstat /root/go/bin/benchstat \
  --output experiments/2026-10-04-single-connection-option/results
```

Profile either binary separately with `-test.run=^$`,
`-test.bench=^BenchmarkSingleConnectionOption$/^TruthyDefault$`,
`-test.benchtime=10000x`, `-test.memprofile=<temporary path>` and
`-test.memprofilerate=1`, using GOMAXPROCS=4 and taskset affinity 8-11.
Use `go tool pprof -top -alloc_space` or `-alloc_objects`; focused attribution
adds `-nodefraction=0 -focus=ConnectionOptionTruthy`.
