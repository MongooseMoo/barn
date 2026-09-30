# Independent review of selective finalization pooling

Decision: PROMOTE the narrowed source to the final application confirmation.
This is not an application nonregression claim; the initial application evidence
remains inconclusive and the fixed longer confirmation has not run yet.

Source reviewed: ea251f7, which adds only the original unpooled collection path
for non-WAIF Values. Direct WAIF Values remain pooled. The pooled Frame collector,
retention bound, reference clearing and VM-owned pending collections are unchanged.
No source defect found: both dispatch paths invoke the same existing collector
and pending-root append operation. The branch restores allocation-free small
anonymous Value collection without changing root traversal or batching order.

The independent alternating-boundary test (b541dad, integrated as 86b16bd)
checks direct WAIF aliases, anonymous values, nested MAP/LIST values, another
direct WAIF, earlier retained roots and another VM. It does not assert pool
identity or require that the runtime retains an item. All earlier reviewer
concurrency, output ownership, bound, and reset tests remain unchanged.

Focused execution at the narrowed source, raw output preserved in adjacent
2026-09-26-pool-refinement-review-evidence/:
```
go test ./builtins ./vm -run '^Test(PoolReview|FinalizationScratch)' -count=1 -timeout=120s
ok github.com/MongooseMoo/barn/builtins 0.315s
ok github.com/MongooseMoo/barn/vm 0.166s
normal exit=0
go test -race ./builtins ./vm -run '^Test(PoolReview|FinalizationScratch)' -count=1 -timeout=120s
ok github.com/MongooseMoo/barn/builtins 2.269s
ok github.com/MongooseMoo/barn/vm 1.163s
race exit=0
go test -race ./vm -run '^TestPendingFinalizationValueSelectionBoundary$' -count=1 -timeout=120s
ok github.com/MongooseMoo/barn/vm 1.187s
selection race exit=0
```

Independently parsed all fourteen development/diagnostic rows, ten complete
pairs per row and ten PASS markers per side. Recomputed the frozen paired median
percentage changes and 20,000-resample percentile intervals with seed 20260926
reset per metric. Results reproduce the worker's declared gate:
```
primary byte reduction median 12.94195465% CI95 [12.93678148,12.94884633]
primary runtime change median -7.50994% CI95 [-12.55894,-6.68807]
Anonymous/1/Value runtime -3.84892% CI95 [-5.55144,-0.63277], B/op 0 -> 0
Anonymous/8/Value runtime -1.47665% CI95 [-3.27364,0.87469], B/op 0 -> 0
independent gate True
seal diff exit=0
```
All thirteen diagnostic rows meet the preregistered no-credible->5%-regression
guard. This guard does not prove every row is noninferior: Anonymous/256/Value,
Anonymous/257/Value and Anonymous/257/Frame have upper runtime-change interval
endpoints above 5%. The oversized Frame still allocates approximately 64.5 more
bytes per operation (19048 -> 19112.5 median), reflecting discarded scratch;
this known penalty remains visible and was not removed from the gate. Direct
waif and pooled-frame gains remain. Non-WAIF Value collections deliberately
forgo the original pool's large-collection savings to preserve their prior path.

The six sealed evaluator paths are unchanged from preregistration 5a05b1e to
ea251f7. Full worker raw evidence/analysis was integrated separately at 76ea087.
No new performance runs were made by this reviewer: the reserved Frame code is
unchanged from its previously passing independent holdouts. Parent owns full
repository gates and the longer application confirmation.

## Final application analyzer frozen before confirmation

experiments/pool-application-confirmation-analyze.py is copied from the independent
reviewer implementation after normalizing p50/p99 duration units (ns/us/µs/μs/ms/s)
to milliseconds. This fix preceded all confirmation data; raw numeric goodput
analysis is unchanged. SHA256:
0CDCE11CB09586FB8681D980D9D7427853DBB0E4D3DA77CF2F2E9154FC14346D.

Requires twenty terminal exit-zero logs, ten pairs, one players=16 row each.
Primary paired goodput median, 20,000 bootstrap resamples, seed 20260929.
Upper interval below 0.95 -> CLEAR_LOSS (exit 1); lower interval >=0.95 ->
NONINFERIOR (exit 0); otherwise INCONCLUSIVE (exit 2). Correctness failures
cannot pass. Every paired ratio, range, failure count and diagnostic metric is
preserved. No partial input, zero-goodput comparison, or output overwrite is
accepted. Analyzer has not been run on confirmation data.
