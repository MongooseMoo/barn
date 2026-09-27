# Exact file conversion capacity experiment

## Preregistration (frozen)

Origin: original file_read experiment profile found conversion builders responsible for 94.73% of remaining allocation. Follow-on separately authorized by parent. Prior experiments search found no conversion capacity record.
Single variable/hypothesis: compute exact text/binary conversion output length and Grow strings.Builder once; remove geometric backing-array growth without retaining an input-sized allocation for all-filtered text. Binary capacity arithmetic must skip reservation when len(data)>MaxInt/3.
Baseline: fa42d69, source includes file_read pool 0e5fdba. Sealed evaluator: builtins/file_conversion_bench_test.go blob 4f098c7c0a1798ed480796f0a17d432299628f6c and inherited builtins/fileio_pool_bench_test.go blob b1c77740663e040526171633cb1c17c60a3e2422.
Profile before: memprofilerate=1, 100ms each of ten direct conversion rows; Builder.WriteByte 77.54%, Builder.WriteString 22.36% of alloc_space; all-filtered rows already allocate zero.
Primary metric: median per-pair geometric mean candidate/baseline B/op across eight nonzero-baseline rows, 95% bootstrap upper bound <=0.90. Filtered rows must remain zero allocation.
Runtime guard: every one of ten rows must have paired median ratio <=1.05 with 95% upper bound <=1.05; any ambiguous interval is no-go. This explicitly tests extra count-pass cost at 64 and 65536 bytes for text, filtered, mixed, binary, and escaped shapes.
Plan: ten alternating AB/BA pairs; GOMAXPROCS=4; `<binary> -test.run=^$ -test.bench=^BenchmarkFileConversion$ -test.benchtime=300ms -test.count=1 -test.benchmem`; timeout ten minutes. Build baseline/candidate via `go test -c -o C:/Users/Q/code/barn-pool-evidence/conversion-{base,candidate}.exe ./builtins`.
Analysis: median paired ratios and min/max spread, deterministic bootstrap seed260926, 10000 resamples, percentile95% median interval. No timing exclusions or peeking-based stopping. Raw files at C:/Users/Q/code/barn-pool-evidence/conversion-{base,candidate}-{0..9}.txt.
Falsification/kill: missing output, failed semantic contracts, altered evaluator, less than10% allocation improvement, any runtime guard miss, arithmetic overflow or oversized retention. No retuning within this experiment.
Contracts: TestFileConversionAllBytes plus file_read tests; ownership already covered. Holdout BenchmarkFileConversionHoldout is actual 32768-byte binary file_read and only independent reviewer runs it.
Other callers: file_readline, file_readlines, EncodeRawToBinary, and system command stdout/stderr conversion. Byte encoding/filtering behavior remains identical. This is internal allocation work, not a change to MOO behavior.
Environment: go1.26.0 windows/amd64, AMD Ryzen9 5950X. Performance runs serialized with parent. Instrumentation after uses same allocation profile command as before and cannot substitute for gate timing.
