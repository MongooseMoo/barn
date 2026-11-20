# SetVerbArgs publication experiment

Status: evaluator preparation. No candidate production edits or final comparison yet.

The failed offline Mongoose magic cohort recorded 5.88 sampled CPU seconds in
cloneVerbForReadTxn for task 1788854378: 3.06 under live publication and 2.82
under transaction adoption. These overlapping stack samples identify a component
target, not a successful gameplay baseline or a current live-server diagnosis.

Baseline implementation: 02fe102ecac0025ef40ad1c8e21767eb7f4ce1c4.
Experiment branch: perf/mongoose-verb-args-publication.

Hypothesis: specializing numbered-object SetVerbArgs publication can reduce its
steady-state time by avoiding repeated copies of proven-owned unchanged verbs.
Keep all non-verb defensive copies and full AdoptLiveVerbs behavior unchanged.
Borrowed verb inputs retain their existing detach boundary. Resolve alias and
wildcard winners before copying; always replace the selected node rather than
mutating a historical node. Preserve anonymous behavior, errors, history pruning,
versions, shape invalidation and locking.

The evaluator consists of baseline-passing ownership, old-image, lookup,
stamp/refusal and adoption/conflict contracts plus a fixed benchmark matrix:

| Verbs | Source lines per verb | Aliases per verb |
| --- | --- | --- |
| 1 | 0 | 3 |
| 16 | 64 | 3 |
| 128 | 64 | 3 |
| 128 | 512 | 3 |

Each row measures live SetVerbArgs separately from the complete setter plus
per-operation transaction adoption. Setup and one complete warm operation occur
before timing. Alternate two fixed valid argument specifications. Validate final
state after timing. No reader persists across operations; use normal history
pruning. The full row opens and releases its transaction within each operation.
Source strings and aliases are identical on both sides. Report ns/op, B/op and
allocs/op for every row. This evaluates steady state; correctness contracts also
cover newly borrowed inputs before and after warm publication.

Primary acceptance metric is the median paired candidate/baseline ns/op ratio
for live V128/L512. Require ratio <= 0.80 and a deterministic 95% paired bootstrap
interval with upper endpoint < 1.00. The live V1/L0 median ratio must be <= 1.05.
Allocation results and other rows are reported without substituting them for the
primary metric. Any semantic failure, race, missing isolation evidence or changed
evaluator disqualifies promotion regardless of timing.

Use ten fixed A/B pairs. Odd pairs run baseline then candidate, even pairs reverse
that order. Reverse row order on alternate pairs. GOMAXPROCS=1, default GOGC,
GOMEMLIMIT unset; identical Go version, PGO input, compiler flags and machine.
Do not overlap benchmarks with tests, builds or gameplay captures. Preserve raw
output, platform/tool versions, binary/source hashes and per-row iteration counts.
Bootstrap 10000 paired resamples with Python Random seed 20261017; compute the
median ratio per resample, and linear-interpolated 2.5th/97.5th percentiles.

Calibration is baseline-only, before candidate edits: start at 1000 operations per
row. For each row choose max(1000, ceil(0.75 / elapsed_seconds) * 1000), capped at
50000, using the reported ns/op times 1000 as elapsed time. Freeze those counts,
the evaluator hashes and this record in a local setup commit before production
changes. There is no retuning, row deletion or extra pair selection after results.
Failed runs remain evidence and invalidate their pair; do not silently replace
them. A material evaluator defect starts a new recorded experiment.

After the frozen comparison, run full Go tests, appropriate race tests and the
pinned complete Elle driver with all six faults on final candidate source.
Preserve histories, manifests, all twelve verdicts, positive path coverage and
anomaly witnesses. An independent reviewer audits correctness and root recomputes
the timing gate before any local integration. No end-to-end speed, live CPU-share
or lower-RSS claim follows from a component benchmark.

Evaluator/preregistration setup commit: f4e72c5e151354b40fe601bdda013b4579c722bc.
All six baseline contracts and both adoption subcases passed before calibration.
The independent synthetic runner audit accepted fifteen checks, including raw
numeric/count tampering, missing/duplicate rows, pair order and timeout retention.

Frozen evaluator SHA-256:

- Contracts: dd4656753774fec89e8ff12b848a127f88da0e649320bcaab50ea88dcc2d6450.
- Benchmarks: 99e47733291e96731ca1983fdbfde7baae1f788ddcd26ab8a590a05d555e41d3.
- Runner: a7944432017edad2026e8edd5333e2ed394d6ea0253ff1a11ce3b14a40d007a1.
- Baseline test executable: 9e4e1c04e4f2251d979c5137ff08e6ac5a594eb55bc465d2156b1c6c7928e3ac.
- Calibration plan: 8a88d963a2c27611ade27af297a0e57efb1cd20d8ca8f4191fc428d1b4dfd383.

Go 1.27.1, Linux amd64, CGO enabled, GOAMD64=v1. Baseline production source
remained 02fe102e throughout setup/calibration. Fixed counts in the committed
verb-args-publication-20261017-plan.json are Live: 50000/40000/5000/1000 and
Adopt: 50000/14000/2000/1000 in table order. Calibration and baseline-build raw
output live under .tmp/verb-args-experiment-20261017-01; semantic output is under
.tmp/verb-args-evaluator. No candidate edits occurred before this freeze.

Candidate comparison and correctness/isolation results: pending.
