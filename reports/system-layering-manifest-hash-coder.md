# Manifest checksum resource ownership

## Goal

Remove database-sized temporary checksum buffers from profile generation in
the existing checksumFile owner, preserving exact SHA-256 digests and native
errors. No cache, pool, new hash layer or startup-order change belongs here.

## State

Completed the bounded two-source-file implementation. checksumFile now reads
through one fixed 32 KiB local buffer in its existing owner, hashes every
returned byte before checking EOF, closes on success/read error and preserves
read/close errors with errors.Join. Other agents' source changes remain outside
this job. Exact tests pass; repeated measurements support the retained variant
for both measured database and config sizes. PR CI remains the coordinator's
next step; no publication or broad test run was performed here.

## Decisions and why

Keep one checksum implementation. The same new benchmark stays in the existing
manifest test file across baseline and candidate measurements. Use ten samples
per side and benchstat; no application-wide bottleneck ranking follows from it.

## Baseline measurement

The unchanged production helper passed exact empty/abc/multichunk digests and
native missing-file error assertions. The 32 MiB baseline established a
file-sized allocation, 33,563,128 to 33,563,177 B/op in ten samples, seven
allocations/op. Raw output is preserved at
C:/Users/Q/code/barn-system-layering/.tmp/system-layering/manifest-hash-before.txt.

Exact scoped command inside verified Debian WSL:

```text
go test ./profile -run '^TestChecksumFile$' -bench '^BenchmarkChecksumFile$' -benchmem -count=10 -benchtime=200ms
PASS
ok github.com/MongooseMoo/barn/profile 2.987s
```

This demonstrated allocation selected a streaming candidate for comparison.
No comparison verdict was made from the baseline alone.

## Streaming candidate measurement

The first candidate opened the file, streamed into sha256.New with io.Copy,
closed it on both successful/error copy results and joined read/close errors.
No new cache, pool, wrapper or hash implementation was introduced. The same
focused command passed after the correction:

```text
gofmt_exit=0
diff_check_exit=0
PASS
ok github.com/MongooseMoo/barn/profile 2.677s
```

Raw samples are C:/Users/Q/code/barn-system-layering/.tmp/system-layering/manifest-hash-after.txt. Existing
/root/go/bin/benchstat compared both ten-sample captures:

```text
32 MiB checksum sec/op: 29.47ms -> 18.66ms, -36.67% (p=0.000 n=10)
B/op: 32776.52KiB -> 32.45KiB, -99.90% (p=0.000 n=10)
allocs/op: 7 -> 8, +14.29% (p=0.000 n=10)
```

This is a focused warm-file Linux/WSL measurement, not an application startup
profile or broad throughput claim. The same helper also hashes small config
files. io.Copy's buffer adds allocation there relative to ReadFile, so small
input evidence was still necessary at this point before retaining it. The
following measurement resolves this limitation.

## Rejected io.Copy small-input cost

The coordinator authorized temporarily restoring only this uncommitted helper
to its original implementation for a two-input comparison, then restoring the
streaming candidate. The benchmark now has config and database subcases with
one shared body; no duplicate checksum code or second test file was introduced.
Exact tests passed in both variants. Raw samples are
[system-layering-manifest-hash-original.txt](system-layering-manifest-hash-original.txt)
and C:/Users/Q/code/barn-system-layering/.tmp/system-layering/manifest-hash-copy-both.txt.

```text
config: 12.59us -> 15.38us, +22.22% (p=0.000 n=10)
config B/op: 1016 -> 33224, +3170.08% (p=0.000 n=10)
database: 25.48ms -> 18.78ms, -26.31% (p=0.000 n=10)
database B/op: 32776.49KiB -> 32.45KiB, -99.90% (p=0.000 n=10)
```

The io.Copy candidate does not satisfy the nonnegative requirement for the
real small-config producer and must not be retained on an aggregate claim.
The proposed alternative is a fixed local stack buffer/read loop in the same
helper, preserving read/close errors and exact digests without thresholds,
fallbacks, wrappers or pools. The coordinator approved that minimal variant
after inspecting this measured small-input regression.

## Retained fixed-buffer variant

The direct read loop uses a fixed local 32 KiB buffer. It processes n>0 before
EOF, preserves other native read errors, always closes after reading and returns
no success digest when read or close fails. The exact tests cover empty/abc and
a multichunk binary fixture, missing-file errors and directory-read PathError
preservation. Only the new focused test/benchmark selectors ran locally.

```text
gofmt_exit=0
diff_check_exit=0
PASS
ok github.com/MongooseMoo/barn/profile 5.184s
```

The same two-input benchmark command and existing benchstat compare against
the original helper, ten samples each:

```text
config sec/op: 12.59us -> 11.72us, -6.87% (p=0.002 n=10)
database sec/op: 25.48ms -> 19.88ms, -21.97% (p=0.000 n=10)
config B/op: 1016 -> 328; allocs/op: 7 -> 6
database B/op: 33563128.5 -> 328; allocs/op: 7 -> 6
```

Raw retained samples are
[system-layering-manifest-hash-loop.txt](system-layering-manifest-hash-loop.txt).
No file-sized or 32 KiB heap allocation appears in either retained sample set.
These are Linux/WSL warm-file microbenchmarks for 21-byte and 32 MiB inputs;
they do not establish all-input or application-wide non-regression. Samples
were collected in sequential batches with other host work still possible.
No cleanup-error injection or race/broad-suite run was performed.

## Next step

Coordinator reviews the two-file source/test diff and complete evidence, then
delivers it through ordinary PR CI. No commit, push, PR, merge, dependency or
configuration mutation was performed by this worker.
