# Binary-only exact conversion capacity confirmation

## Preregistration (frozen)

Origin: combined text/binary exact capacity experiment rejected: text mixed65536 runtime ratio1.25422 CI[1.22862,1.30169]. Binary development rows showed improvement; parent authorized one separate final refinement. This confirmation does not salvage or replace the negative combined experiment.
Single variable: exact output count plus Builder.Grow ONLY in encodeBinaryBytes. Text source remains baseline. Skip reservation above MaxInt/3 input length to avoid overflow.
Baseline: fa42d69 (file_read pool0e5fdba plus conversion harness). Evaluator seals: builtins/file_conversion_bench_test.go=4f098c7c0a1798ed480796f0a17d432299628f6c; builtins/fileio_pool_bench_test.go=b1c77740663e040526171633cb1c17c60a3e2422. Full ten-row harness unchanged; original conversion baseline binary used.
Primary metric: paired geometric mean B/op ratio across four binary/escaped rows, median and bootstrap95% upper bound<=0.85. Stronger15% threshold reflects evidence-driven selection from previous experiment. Every other nonzero-byte row must have ratio<=1.01; filtered rows remain zero allocation.
Runtime guard: each binary/escaped row paired median and bootstrap95% upper bound<=1.05. Unchanged text rows diagnostic only: significant median>1.05 with lower95%>1.05 invalidates run as environment instability; otherwise report intervals without claiming text improvement.
Plan: ten new alternating AB/BA pairs; GOMAXPROCS=4; `<binary> -test.run=^$ -test.bench=^BenchmarkFileConversion$ -test.benchtime=300ms -test.count=1 -test.benchmem`; ten-minute deadline. Baseline copied from original conversion-base.exe; new candidate compiled via go test -c. Raw files C:/Users/Q/code/barn-pool-evidence/binary-{base,candidate}-{0..9}.txt.
Analysis: paired ratios, median/min/max; bootstrap seed260926 and10000 median resamples, percentile95% intervals; retain all host outliers. No peeking-based stopping, no further refinement if no-go.
Kill/falsification: any gate miss or ambiguity, semantic failure, altered evaluator, overflow, missing output or no meaningful allocation gain. Byte encoding unchanged. Relevant callers: read/readline/readlines, EncodeRawToBinary, system stdout/stderr.
Contracts: TestFileConversionAllBytes and file_read ownership/error tests. Holdout BenchmarkFileConversionHoldout (32768-byte binary actual file_read) remains reserved for independent verifier. Worker never ran this holdout in either experiment.
Instrumentation: before alloc profile from original unchanged binary baseline; after allocation profile ten rows100ms, memprofilerate1. Environment go1.26.0 windows/amd64 Ryzen9 5950X. Raw profile data clearly showed geometric builder growth; binary maximum output length is3x input.
