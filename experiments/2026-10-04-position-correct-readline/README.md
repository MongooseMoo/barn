# Position-correct chunked file_readline

Issue #361. Baseline: 5c68c8b748128bd7a1748a79556bd15583348e67.

Preregistered comparison before timing: freeze the same test/benchmark source
against baseline and candidate fileio.go. Twelve randomized paired samples and
two holdouts, seed 361, 200ms per workload, GOMAXPROCS=4, affinity CPUs 8-11,
WSL Debian Go 1.24.6. Report paired geometric timing ratios and bootstrap 95%
intervals, benchstat, B/op and allocs/op. Six workloads, each text/binary:
empty line; 64 one-character lines; one and 64 32-character lines; one 1024-
character line; one 65536-character line. Fixtures/opening are outside timing;
timing includes resetting position, builtin dispatch, conversion and value checks.

Adoption requires substantially fewer Go Read calls for 1KiB/64KiB lines,
unchanged values/positions, and timing benefits on 32B/1KiB/64KiB workloads
confirmed in holdouts. Empty/one-character controls must have no significant
regression exceeding 5%. Separate instrumented binaries count Go Read calls;
they are not timing binaries or an OS syscall trace. No server throughput/RSS claim.

Regular-file reads use two one-byte probes (preserving tiny-line costs), then
borrow an existing 4096B file-read pool buffer. Unused chunk bytes are rewound
before returning. No read-ahead survives a call; seek/write/read/tell/EOF need no
buffer invalidation and external writes cannot be hidden by cached unread bytes.
The handle's position mutex covers all ten position-sensitive builtins, including
readlines' temporary seek/restore and existing scanner behavior in countlines/grep.
Close and Session.Close intentionally bypass this mutex to interrupt blocked
I/O. Nonregular files keep byte reads. Stat failure at open selects that fallback
rather than introducing a new open failure. Existing mode/filter/error behavior
and file-size policy remain unchanged.

## Results

The candidate meets the preregistered adoption gates. The paired geometric
timing changes below use candidate/baseline (negative is faster). Intervals are
bootstrap 95%; holdouts are two additional randomized pairs.

| Characters / lines / mode | Change | 95% interval | Holdout |
| --- | ---: | ---: | ---: |
| 0 / 1 / text | +3.47% | +2.94 to +4.10% | +2.39% |
| 0 / 1 / binary | +4.30% | +3.35 to +5.52% | +3.57% |
| 1 / 64 / text | +5.40% | +4.41 to +6.75% | +4.25% |
| 1 / 64 / binary | +4.24% | +3.73 to +4.78% | +3.51% |
| 32 / 1 / text | -86.03% | -86.12 to -85.93% | -86.03% |
| 32 / 1 / binary | -85.81% | -85.94 to -85.63% | -85.90% |
| 32 / 64 / text | -86.22% | -86.34 to -86.03% | -86.24% |
| 32 / 64 / binary | -85.99% | -86.05 to -85.92% | -86.03% |
| 1024 / 1 / text | -99.28% | -99.28 to -99.27% | -99.26% |
| 1024 / 1 / binary | -99.07% | -99.07 to -99.06% | -99.04% |
| 65536 / 1 / text | -99.64% | -99.64 to -99.63% | -99.61% |
| 65536 / 1 / binary | -99.43% | -99.44 to -99.42% | -99.40% |

Tiny controls have statistically detectable overhead; they are not unchanged.
The text one-character point estimate is +5.40%, but its interval includes 5%
and its holdout is +4.25%, so a regression exceeding the preregistered 5%
threshold is not established. This is a narrow margin, not evidence of no
regression. Benchstat's unpaired median estimate is +4.99% for that case.

Tiny controls retain their allocation counts/bytes. Single 32B text lines change
240B/8 allocations to 176B/6; binary 304B/9 to 240B/7. Single 1KiB lines change
6704B/19 to about 4547B/12 in both modes. Single 64KiB lines use about
595KB/34 instead of 571KB/45: chunked slice growth removes allocation calls
but increases allocated bytes by about 4.2%. This is not an across-the-board
allocation reduction. Multi-line 32B results are in the raw artifacts.

Separate counter overlays change only the existing `h.file.Read(tmp)` call to
a delegate incrementing a counter and invoking the same os.File.Read. The
budget test fails on baseline and passes on candidate; values are asserted.

| Characters plus newline | Baseline Go Read calls | Candidate Go Read calls |
| --- | ---: | ---: |
| 0 | 1 | 1 |
| 1 | 2 | 2 |
| 32 | 33 | 3 |
| 1024 | 1025 | 3 |
| 65536 | 65537 | 18 |

This independently demonstrates the intended mechanism without instrumenting
the timed binaries. It does not count OS syscalls. Counter tests execute alone;
the counter is deliberately not concurrency instrumentation.

## Frozen inputs and reproduction

Test source SHA256:
`2896e4414b8c223d9a815f8701145eed4dfbf2ca1eca541a5a51aed4de864c7b`.
Go 1.24.6 Linux/amd64 binaries:

- baseline timed: `38140bc557d2b2b18400cc018890e66d6b60af9d8ba8301ace2e69f0b289e18d`
- candidate timed: `2732cd9abb41162b2c26fc3f63e88f48cf4c9ec6638e382da3fb715eaa931da0`
- baseline counted: `b455afec2bee1882899b70182a1c923b6bca6baf55038db5bdec1ca0a7af23e7`
- candidate counted: `28dac59f4bdf442f0a8e58b40558999f81a11b254f5143b98ce1e30e3b530962`

From the worktree in WSL, with GIT_DIR/GIT_WORK_TREE set to the corresponding
Linux paths for this Windows-created linked worktree:

```sh
python3 experiments/2026-10-04-position-correct-readline/prepare_binaries.py \
  --baseline 5c68c8b748128bd7a1748a79556bd15583348e67 --output /tmp/barn-361-tools
python3 experiments/2026-10-04-position-correct-readline/run_pairs.py \
  --baseline /tmp/barn-361-tools/baseline.test \
  --candidate /tmp/barn-361-tools/candidate.test \
  --benchstat /root/go/bin/benchstat \
  --output experiments/2026-10-04-position-correct-readline/results
/tmp/barn-361-tools/baseline-count.test -test.run=^TestReadlineReadCallBudget$ -test.v
/tmp/barn-361-tools/candidate-count.test -test.run=^TestReadlineReadCallBudget$ -test.v
```

The baseline counter command intentionally fails its read-count budget.
Benchstat is golang/perf commit 406019bb8b6893dd1245d31bf511c719619bb5c9,
built with Go 1.26.8; it does not change the measured Go 1.24.6 toolchain.
Files live on the WSL-mounted Windows filesystem with warm caches on an AMD
Ryzen 9 5950X. Governor/frequency/background machine load are uncontrolled.
The measurements include seeking once per workload and conversions/assertions;
they are not isolated disk latency, cold-cache results, or server throughput.
Timing and allocation results should not be extrapolated to other filesystems,
special files, arbitrary line distributions, or process RSS.

## Validation and ownership

- Baseline sequential text/binary values, byte positions, chunk boundaries,
  mixed read/readline/seek/tell/write/writeline/readlines, external writes,
  long EOF without newline, and pipe/close tests pass.
- Baseline concurrent shared-handle regression fails with duplicate empty
  strings; candidate eight-reader consumption returns every line exactly once.
  This is an internal Go concurrency invariant, not a new MOO semantic claim.
- `go test ./builtins -count=1`: 5.952s.
- `go test -race ./builtins -count=1`: 59.986s.
- `go vet ./...` and `staticcheck ./...`: successful.
- Expanded final test-only boundary/long-EOF cases:
  `go test -race ./builtins -run ^TestReadline -count=3`: 4.827s.

No other production owner accesses these file positions outside fileio.go.
Session.Close detaches handles under the registry lock, releases that lock,
then closes descriptors directly. The new position lock is never held while
acquiring the registry lock; descriptor Close remains able to interrupt reads.
The pool holds only bounded temporary buffers; output strings own their storage.
No read-ahead or chunk buffer is stored on a handle.
