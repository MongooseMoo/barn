# Flush read-set ownership and allocations

The seven object-version dependency maps share one inline `objectReadSet` in
`StoreTxn`. A successful `FlushStagedToLive` resets that value once. Reads after
the reset allocate only the maps they use. Validation and renewal algorithms
are unchanged; failed and no-op flushes still return before the reset.

## Measurement

Baseline is integrated master after PR #598. Candidate changes only the read-set
field ownership and successful-flush reset, plus regression/benchmark coverage.
The benchmark source is identical in both lanes: a Go overlay replaces the
baseline's `db/store/commitalloc_bench_test.go` with the candidate file.

Windows amd64, Go 1.27.1, AMD Ryzen 9 5950X, GOMAXPROCS=4, GOGC=100. Two frozen
test executables; ten sequential pairs alternating base/candidate and
candidate/base order, one second per benchmark case. Explicit transaction
Release is included. No CI or compiler was launched by this task during the
measurements. This is a shared workstation, not an isolated timing host.

Each operation begins a snapshot, stages a move alternating between #0 and
nothing, calls PrepareLiveMutation, FlushStagedToLive and MarkLiveMutated, then
commits and releases. This follows the lifecycle in
`builtins/store_reads.go:flushStagedBeforeCoarse`. The second case also reads the
location and a property after flush. It measures the store sequence, not a full
coarse builtin or an end-to-end Mongoose command.

| Case | Baseline B/op | Candidate B/op | Baseline allocs/op | Candidate allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Without later reads | 7280 | 6944 | 45 | 38 |
| With location/property reads | 8992 | 8752 | 53 | 48 |

All ten observations have these same allocation counts and byte counts. The
savings are 336 bytes / seven allocations and 240 bytes / five allocations,
respectively. Time differences are not statistically significant in benchstat:
6.693 to 6.742 microseconds (p=0.684) and 8.224 to 8.208 microseconds (p=1.000).
No elapsed-time or whole-server throughput improvement is claimed.

Raw observations: [baseline](base.txt), [candidate](candidate.txt),
[benchstat](benchstat.txt). Frozen binaries remain in the supervising checkout's
`.tmp/lazy-flush-base.exe` and `.tmp/lazy-flush-candidate.exe`. SHA256 respectively:
`9733A0723EA1C0A6C5AD4A67AD5F18AF0E86B9831661123C94294C3E4BD7D4F6` and
`9F3CE3373A45D59D31C7279FD2104BB44713DDAD7495ECF250E43BA61E6D1A50`.
Only a source comment was corrected after these binaries were built.

Build each lane with `go test -c -o <binary> ./db/store`; baseline also takes
`-overlay <overlay.json>`. Run each binary with the environment above and quoted
PowerShell arguments `'-test.run=^$' '-test.bench=^BenchmarkTxnTopologyFlush$'
'-test.benchmem' '-test.benchtime=1s' '-test.count=1'`. Compare with
`benchstat base.txt candidate.txt`.

## Regression coverage

`TestStoreTxnFlushPostReadsValidate` stages a real move, flushes, then reads all
seven dependency categories. It requires a successful coarse commit without
intervening mutations and all seven conflict kinds after real store mutations.
The successful case also starts with a superseded pre-flush scalar read,
demonstrating that old dependencies are discarded. Existing failed-flush
coverage remains unchanged. The authored focused test passed; full unit/race,
managed MOO conformance and pinned Elle checks run on the PR.
