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

## Results

Primary ratio 0.800868 (19.91% allocated-byte reduction), 95% interval [0.800547, 0.801100]. Runtime median ratios text64 0.9925, text4096 0.9486, binary4096 0.9551, text65536 0.9447; every runtime interval upper bound <=1.028. File-read buffer allocation disappears from top profile; Builder.WriteByte and WriteString now account for 94.73% of allocation. Size-class bound 64KiB per item; no global count bound. Strings are copied by conversion builders before deferred buffer return.
Ten alternating AB/BA pairs completed with no exclusions. Host timing outliers remain in raw evidence; median/bootstrap analysis was frozen before measurement. This proves isolated builtin allocation improvement, not application-wide throughput.
Fast contracts passed: `go test ./builtins -run '^TestFileRead' -count=1` => `ok github.com/MongooseMoo/barn/builtins 0.199s`. Evaluator diff from prereg to source is empty.
Committed raw stdout, top profiles, analysis JSON, and exact runner/analyzer scripts are in `2026-09-26-pool-fileio-evidence/`. Binary profiles and binaries remain outside tracked tree at the prereg path; all raw output was obtained on that same machine.
Reproduce analysis: `python experiments/2026-09-26-pool-fileio-evidence/analyze.py fileio`. Run measurement script with Name=fileio and Bench=BenchmarkPoolFileRead after compiling both named binaries beside script. Profiles used 300ms each, not gate timings.
Source commit: 0e5fdbaab0342b8117af0a669772e55709682b76; prereg d7b8c75. Recommendation: promote after independent retention/correctness review and sealed holdout. Worker has not run holdout or promoted source.
