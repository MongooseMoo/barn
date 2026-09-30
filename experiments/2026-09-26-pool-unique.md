# unique scratch pooling experiment

## Preregistration (frozen)

Origin: user requested investigation of all four identified pool candidates. Repo experiments search for pool/unique/file_read found no relevant prior experiment.
Hypothesis and single variable: Reuse unique bucket map and predecessor slice in bounded size classes, retaining independent result storage. Profile attributes 427.58MB to bucket map, 27.81MB to predecessor slice, 84.64MB to result.
Baseline source: 3c0f510; harness baseline commit: bb6a74e. Branch: perf/pool-builtins.
Evaluator seals: builtins/lists_pool_bench_test.go=59c5889c44ab5bcafe45da716822f516452b252a; builtins/lists_unique_test.go=5369c7e2a50ec952251aebadbd2d98758a7654ad. No evaluator edits after this preregistration.
Primary metric: geometric mean candidate/baseline B/op ratio across the four development rows; require <=0.90 and paired 95% bootstrap interval upper bound <=0.90.
Runtime guardrail: no row with median paired runtime ratio >1.05 and bootstrap 95% interval lower bound >1.05. Ambiguous runtime evidence is not a promotion pass.
Plan: ten paired processes, alternating AB/BA order, identical binaries and workload inputs; GOMAXPROCS=4; each invocation `-test.run=^$ -test.bench=^BenchmarkPoolUnique$ -test.benchtime=300ms -test.count=1 -test.benchmem`. Deadline ten minutes. Raw outputs outside checkout under C:/Users/Q/code/barn-pool-evidence/.
Analysis: per-row paired ratios, median and min/max; aggregate geometric mean per pair; deterministic percentile bootstrap (seed 260926, 10000 samples) medians, 95% interval. All ten pairs completed without peeking-based stopping.
Kill criteria: semantic contract failure, evaluator change, missing/crashed measurement, primary miss, demonstrated runtime regression or unsafe retention. Do not tune a missed candidate within this experiment.
Falsification: pooled scratch does not remove >=10% of aggregate allocated bytes, or safe cleanup removes its runtime benefit.
Retention: size classes bounded at 16384 elements; larger inputs bypass pooling; clear map keys before reuse.
Holdout: BenchmarkPoolUniqueHoldout, excluded from worker measurements and reserved for independent verifier.
Fast contracts: existing unique/file_read tests plus new result-ownership tests. Source semantics remain unchanged; no oracle behavior changes.
Instrumentation: Go alloc_space profile before and after, memprofilerate=1; binaries compiled using `go test -c -o <evidence>/<name>-{base,candidate}.exe ./builtins`. Profile command adds `-test.memprofile=<evidence>/<name>-{before,after}.pprof -test.memprofilerate=1` to benchmark command.
Environment: go1.26.0 windows/amd64; AMD Ryzen 9 5950X; baseline profile outputs retained as unique-profile-before.txt. Profiles are diagnostic and excluded from timing gate.

## Results

Primary ratio 0.231535 (76.85% allocated-byte reduction), 95% interval [0.231191, 0.232757]. Runtime median ratios tiny8 1.0174, mixed128 0.7666, distinct10000 0.6374, duplicate4096 0.6969; every runtime interval upper bound <=1.028. Profile scratch allocation moved from dominant map allocation to result storage; borrowUniqueScratch accounts for 2.33% cumulative allocation after. Largest pooled item approximately 2-3MiB map storage plus 128KiB predecessor slice; size-class bounds are per item, not global bounds. Cleared string keys retain no caller data; returned lists are not pooled.
Ten alternating AB/BA pairs completed with no exclusions. Host timing outliers remain in raw evidence; median/bootstrap analysis was frozen before measurement. This proves isolated builtin allocation improvement, not application-wide throughput.
Fast contracts passed: `go test ./builtins -run 'Unique' -count=1` => `ok github.com/MongooseMoo/barn/builtins 0.292s`. Evaluator diff from prereg to source is empty.
Committed raw stdout, top profiles, analysis JSON, and exact runner/analyzer scripts are in `2026-09-26-pool-unique-evidence/`. Binary profiles and binaries remain outside tracked tree at the prereg path; all raw output was obtained on that same machine.
Reproduce analysis: `python experiments/2026-09-26-pool-unique-evidence/analyze.py unique`. Run measurement script with Name=unique and Bench=BenchmarkPoolUnique after compiling both named binaries beside script. Profiles used 300ms each, not gate timings.
Source commit: 7e0363d1c1d5db68eda7e25091e7d23c458217f4; prereg 7304918. Recommendation: promote after independent retention/correctness review and sealed holdout. Worker has not run holdout or promoted source.

## Retained-heap diagnostic

Saved end-of-benchmark inuse_space: baseline total24.46KiB; candidate690.52KiB, of which borrowUniqueScratch672.39KiB. This snapshot demonstrates the allocation-versus-retention tradeoff; it is neither peak live heap nor a global pool bound. Largest class still permits roughly2-3MiB map storage plus128KiB predecessor backing per item. Scheduler concurrency and GC affect retained counts; no lifetime guarantee is asserted.
Command: `go tool pprof -top -inuse_space <binary> <before-or-after.pprof>` on original saved profiles. Full raw tables committed alongside allocation profiles; no new timing sample used.
