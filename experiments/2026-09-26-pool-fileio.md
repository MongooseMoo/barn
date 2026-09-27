# fileio scratch pooling experiment

## Preregistration (frozen)

Origin: user requested investigation of all four identified pool candidates. Repo experiments search for pool/unique/file_read found no relevant prior experiment.
Hypothesis and single variable: Reuse file_read byte buffers in bounded size classes after text/binary conversion. Profile attributes 20.79% of allocation space directly to builtinFileRead; conversion builders dominate remaining allocations.
Baseline source: 3c0f510; harness baseline commit: e1ed597. Branch: perf/pool-fileio.
Evaluator seals: builtins/fileio_pool_bench_test.go=b1c77740663e040526171633cb1c17c60a3e2422; builtins/fileio_read_test.go=c706715b91047561a68fb02ceea3c31886096aab. No evaluator edits after this preregistration.
Primary metric: geometric mean candidate/baseline B/op ratio across the four development rows; require <=0.90 and paired 95% bootstrap interval upper bound <=0.90.
Runtime guardrail: no row with median paired runtime ratio >1.05 and bootstrap 95% interval lower bound >1.05. Ambiguous runtime evidence is not a promotion pass.
Plan: ten paired processes, alternating AB/BA order, identical binaries and workload inputs; GOMAXPROCS=4; each invocation `-test.run=^$ -test.bench=^BenchmarkPoolFileRead$ -test.benchtime=300ms -test.count=1 -test.benchmem`. Deadline ten minutes. Raw outputs outside checkout under C:/Users/Q/code/barn-pool-evidence/.
Analysis: per-row paired ratios, median and min/max; aggregate geometric mean per pair; deterministic percentile bootstrap (seed 260926, 10000 samples) medians, 95% interval. All ten pairs completed without peeking-based stopping.
Kill criteria: semantic contract failure, evaluator change, missing/crashed measurement, primary miss, demonstrated runtime regression or unsafe retention. Do not tune a missed candidate within this experiment.
Falsification: pooled scratch does not remove >=10% of aggregate allocated bytes, or safe cleanup removes its runtime benefit.
Retention: size classes bounded at 65536 bytes; larger reads bypass pooling; conversion finishes before returning buffers.
Holdout: BenchmarkPoolFileReadHoldout, excluded from worker measurements and reserved for independent verifier.
Fast contracts: existing unique/file_read tests plus new result-ownership tests. Source semantics remain unchanged; no oracle behavior changes.
Instrumentation: Go alloc_space profile before and after, memprofilerate=1; binaries compiled using `go test -c -o <evidence>/<name>-{base,candidate}.exe ./builtins`. Profile command adds `-test.memprofile=<evidence>/<name>-{before,after}.pprof -test.memprofilerate=1` to benchmark command.
Environment: go1.26.0 windows/amd64; AMD Ryzen 9 5950X; baseline profile outputs retained as fileio-profile-before.txt. Profiles are diagnostic and excluded from timing gate.
