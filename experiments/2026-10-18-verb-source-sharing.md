# Immutable verb-source sharing experiment

Preregistration before candidate implementation. Baseline implementation and
integration: e66500c936d0dcbcae43196d64b2521dddffc848. Worktree:
`barn-verb-source-profile`, branch `perf/mongoose-verb-source-sharing`.

The previous actual offline Mongoose cohort still failed zero_focus_lumos's
ordinary-refusal control and its weave task hit seconds_budget. All remaining
cloneVerbForReadTxn task samples (4.25 CPU seconds) passed through AdoptLiveVerbs;
the source-array copy line accounted for 2.36 seconds. These are samples from a
budget-terminated workload, not a completed gameplay performance baseline.

Hypothesis: source arrays already detached from borrowed inputs can be shared
immutably when cloning a Verb, cutting adoption and full-object cloning costs.
Always allocate a fresh node and copy names as before. Only incoming
backingOwned=true permits code sharing; false still requires a defensive source
copy. The clone establishes transitive ownership proof. Every supported source
write continues to replace the slice through setCodeOwned/setCodeCopy, refresh
codeKey and clear the proof. No name/non-verb sharing expansion, node reuse,
guard/permission change or MOO-visible behavior change is proposed.

Seal baseline-passing source-ownership/replacement/adoption contracts before the
delta. Reuse the unchanged eight BenchmarkVerbArgsPublication rows and their
operation bodies. Source rows: V1/L0, V16/L64, V128/L64, V128/L512, three aliases,
both Live and Adopt. One complete warm operation precedes timing. The Adopt row
creates/releases one snapshot within each operation and includes materialization
plus adoption. No reader persists across timed operations. Fixed alternating args
and final-state validation remain unchanged.

Primary is now explicitly the Adopt/V128/L512 median paired ns/op ratio against
this new baseline. Require <=0.80 and deterministic 95% bootstrap upper endpoint
<1.00. Tiny Adopt/V1/L0 median ratio must be <=1.05. Report all rows, allocated
bytes and allocation counts; do not substitute them for the primary metric.
Semantic failure, race, altered evaluator or missing isolation evidence blocks
promotion. The earlier study's Live primary is retained in its original committed
plan and runner version; it is not being reinterpreted.

The runner now records metric_mode at baseline calibration; this study selects
Adopt. Default remains Live for old plans. All thresholds, rows, statistics,
pair count, order and calibration rule are unchanged. Ten A/B pairs, alternating
side order and reversing row order on even pairs. Linux Go 1.27.1,
GOMAXPROCS=1, GOGC=100, GOMEMLIMIT unset, identical compiler settings. No builds,
tests or worlds overlap timing. Preserve every raw sample and any failed run.

Baseline-only calibration runs 1000 operations per row; freeze
min(50000,max(1000,ceil(0.75/elapsed_seconds)*1000)) operations, where elapsed is
reported ns/op times1000. Commit evaluator and calibration hashes/counts before
production changes. Bootstrap10000 paired resamples using Random(20261017), one
stream in fixed row order, median ratio per resample, linear-interpolated
2.5th/97.5th percentiles. No retuning or selecting extra pairs after results.

After comparison, an independent reviewer recomputes the gate from raw files.
Run full Go tests, appropriate eight-package race tests and the complete pinned
Elle driver (three seeds, both healthy models, all six faults). Preserve all
histories/manifests/witnesses and require12expectedverdicts,20positivecoveragekeys
per healthy seed and zero raw fail/info outcomes. Build a standard untagged Barn
with verified main package and explicit source/binary bindings. Then run exactly
one new isolated magic cohort with the prior checkpoint, catalog, bootstrap,
eight controllers, seed1, magic1, warmup30s, deadline120s and GoCPU15s. Preserve
failure/terminal metadata, original manifests, profiles, cleanup and checkpoint
pair. No quota/control edits or hidden reruns. Keep component, gameplay, RSS and
natural-heap conclusions separate.

Setup commit057fa09050427322960abe8957e719d5ac58d65c precedes calibration.
Baseline contracts passed three tests/nine subcases plus the existing concurrent
reader test (`db/store 0.028s`, exit0). Independent new-mode probes passed9checks.
No candidate production edits occurred before this seal.

Frozen SHA-256:

- New contracts: 6477f65b7d86398dd4ba73e98fefa7bbdb1bbbe63020ff2dca00987fb572f163.
- Unchanged benchmark: 99e47733291e96731ca1983fdbfde7baae1f788ddcd26ab8a590a05d555e41d3.
- Runner: 6be97e36fb7f2b7f82a22a46b239991ef0069f4cf7d7af85cbad78a01ce1bf06.
- Baseline executable: a215ea3af7296cb6c8fd1248c85c5e2d77f2c1c8c21b29253c6b89cdd792c760.
- Plan: ed58289b609478d103cd69392e582ed78aad22a3c65d6c49227ee22cc16ecbbf.

Committed `verb-source-sharing-20261018-plan.json` records metric_mode=Adopt.
Fixed iterations in table order: Live50000/50000/50000/45000;
Adopt50000/20000/3000/1000. Calibration exit0. Raw calibration/build artifacts are
under `.tmp/verb-source-experiment-20261018-01`; baseline contracts under
`.tmp/verb-source-evaluator`. Candidate comparison/correctness/cohort pending.
