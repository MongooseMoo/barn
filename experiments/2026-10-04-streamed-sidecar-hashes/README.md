# Streamed checkpoint hashing (#359)

## Preregistered comparison

Baseline production is `db/format/waif_identity_sidecar.go` at published merge
`f6e526dedb0a22a410b8b97430432be2b9668356`. Both sidecar operations currently
read the complete database into memory solely to hash it. The regression reports
8,408,500 allocated bytes per 8 MiB read on Windows; this establishes allocation
scaling, without a timing or peak-RSS claim.

Compare the identical `BenchmarkWaifIdentitySidecarHash` read/write workloads on
1, 8, and 32 MiB files. Payload byte i is byte(i*31+7); each sidecar uses fixed
identity `00112233445566778899aabbccddeeff`. Preparation and exact header/identity
checks occur outside timing. Write samples include the unchanged sidecar Sync.
Use WSL Go 1.24.6, compiled baseline/candidate binaries with identical test source,
GOMAXPROCS=4, affinity 8-11, 200ms duration, twelve randomized paired samples from
seed 359 and two frozen-binary holdouts. Save outputs, order, hashes, and allocation
metrics; compare with benchstat and paired log-ratio intervals. Record input and
output digests so the memory reduction cannot come from dropping work.

The target is bounded hash allocation as database size grows. A shared streamed
SHA-256 helper using an opened file and io.Copy should preserve all bytes and
close the file on every path. Checkpoint publication/recovery (#356), serialization
changes, buffer pooling, server throughput, and peak RSS are outside this slice.
Capture allocation profiles for the 32 MiB read workload to attribute the removed
database-sized allocation. Profiles are diagnostic, outside timing samples.

## Results

Go 1.24.6 linux/amd64, Ryzen 9 5950X, the settings above. Files are prepared in
WSL temporary storage. Twelve pairs and both holdouts completed with unchanged
binaries and exact sidecar checks. Candidate allocations remain approximately
37,800 bytes per read and 33,500 bytes per write as database size grows; precise
byte ranges are recorded below.

| Workload | Baseline B/op range | Candidate B/op range | Baseline allocations | Candidate allocations |
| --- | ---: | ---: | ---: | ---: |
| Read 1 MiB | 1,062,019-1,062,088 | 37,805-37,825 | 18 | 21 |
| Read 8 MiB | 8,402,490-8,402,585 | 37,800-37,808 | 21-22 | 21 |
| Read 32 MiB | 33,568,264-33,568,476 | 37,800-37,862 | 21-23 | 21 |
| Write 1 MiB | 1,057,815-1,057,915 | 33,532-33,544 | 15-16 | 18 |
| Write 8 MiB | 8,398,191-8,398,309 | 33,528-33,548 | 18-19 | 18 |
| Write 32 MiB | 33,563,992-33,564,169 | 33,528-33,594 | 18-19 | 18 |

The streamed path uses a few more small allocations for 1 MiB files while
removing the database-sized allocation. Larger files do not increase its buffer
allocation. These are allocated bytes per operation, not retained heap or peak
RSS measurements. Sidecar serialization and Sync remain part of write workloads.

The timing comparison is secondary evidence for these prepared-file workloads.
Positive changes mean candidate time increased. The paired interval resamples
twelve log ratios 10,000 times with seed 359 plus the sorted workload index.

| Workload | Paired time change | 95% interval | Holdout change |
| --- | ---: | ---: | ---: |
| Read 1 MiB | -17.978% | -20.176% to -15.877% | -19.811% |
| Read 8 MiB | -18.490% | -21.828% to -14.230% | -22.013% |
| Read 32 MiB | -26.857% | -27.989% to -25.332% | -26.870% |
| Write 1 MiB | -6.190% | -15.484% to +1.478% | -3.471% |
| Write 8 MiB | -21.877% | -24.573% to -19.125% | -21.170% |
| Write 32 MiB | -27.230% | -29.249% to -25.748% | -26.945% |

`results/benchstat.txt` compares the twelve main samples; holdouts are saved
separately. Benchstat detects no significant 1 MiB write timing difference.
Its version is `golang.org/x/perf@v0.0.0-20260929162123-406019bb8b68`, built
with Go 1.26.8; the measured binaries both use Go 1.24.6. Frequency/governor and
shared disk activity were not controlled. No server throughput or cold-disk
speedup is inferred from the timing data. Allocation reduction is the target.

## Allocation attribution and correctness

Diagnostic profiles use the frozen binaries, `Read/32MiB`, ten iterations,
GOMAXPROCS=4, affinity 8-11, and `-test.memprofilerate=1`, outside paired runs.
Go also runs one calibration iteration. Baseline `os.readFileContents` allocates
360,537 KiB across eleven database reads. Candidate `io.copyBuffer` allocates
352 KiB across the same eleven reads. The profiles also include two 32 MiB
fixture allocations outside benchmark timing; those stay unchanged. The focused
candidate ReadFile profile contains only 1 KiB of tiny sidecar-output reads used
for untimed validation, rather than database reads. Top and focused profile text
is saved under `results/`; diagnostic binary profiles remain outside the repo.

Both read and write memory budgets fail on the baseline and pass with streaming,
including Windows and WSL race checks. Complete format tests, complete format
race tests, vet, and staticcheck pass. Tests cover empty, small, and 8 MiB data,
exact headers/identities, missing/unreadable database paths, and mismatches. Existing
checkpoint identity roundtrips and panic-checkpoint separation remain covered.
The shared hash helper closes its opened file on success and read-error paths;
both callers preserve their existing contextual hash errors.

Raw main/holdout logs, order, summary, toolchain, binary hashes, and workload
digests are in `results/`. Text artifacts trim trailing output padding and use
LF, preserving every measurement. The final test/benchmark source SHA-256 is
`33592924c0c83e5384ae3dd83b84857950af4d350cbb6000afe609aa61938ea9`.
Baseline binary SHA-256 is
`124fcd98a49ee70ca6ace6b51b017151fe1ae68666b38716e084613d2f4fe1a4`;
candidate is `aa01d89546f7ceefe2609cce600231947b96de5b46f7b409d62d82138d2fca43`.
The baseline binary uses an exact-source Go overlay for the published production
file and the same new test source. Checkpoint publication and portable dump
serialization are unchanged. Full repository and merge-group CI gate merge.
