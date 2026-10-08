# Deferred GC exhausted-candidate routing (#270)

## Contract and frozen comparison

User authorized the bounded router optimization and repeated measurements.
Owning files: `vm/anonymous_gc.go`, routing tests, and experiment records.
Retain the independent progress worker, root barriers, immutable candidate
snapshot, callback order, first eligible context, and exactly-once recycling.
Publication and merge remain separate milestones.

Optimization: finish routing when every member of the frozen candidate list
has already been handled. No later request can invoke another callback. Existing
tests protect objects created by recycle hooks; additional cases cover nil
contexts, uncovered candidates, unsorted candidate order and later request floors.
Timing assertions do not belong in correctness tests.

Before the production delta, commit the new contract tests and a microbenchmark
with 1,024 frozen candidates and 1/128/4,096 requests. Each callback digest must
match. Preserve a detached worktree of that unoptimized progress implementation.

Repeat five triples using the unchanged concurrent harness blob
`121d034527dfa4aeef15fc330d35e731dfe64b1e`: original baseline `34e6622`,
unoptimized progress implementation, and optimized implementation. Odd triples
run base/pre/optimized; even triples run optimized/pre/base. Windows Go 1.27.1,
GOMAXPROCS=4, sequential commands, no concurrent verification jobs. Run the
router microbenchmark with each arm (`-benchtime=250ms -count=1`). Record all
samples and digests; no workload or cadence tuning after seeing the results.

Compare optimized against both controls. Retain the previous responsiveness
guard: investigate repeated maxima over 50 ms with more than 20 ms added versus
original baseline. Three or more of five paired crossings count as repeated.
The synthetic guard uses raised background quotas, not a production SLA. The
microbenchmark should remove request-count growth once the list is exhausted;
unchanged allocation count and callback digest are required. Neither result
proves a production throughput gain. A separate profile may check the identified
redundant-map-check hotspot; it is excluded from the paired measurements.

If the concurrent guard remains unsatisfied, attribute residual delays before
expanding scheduler scope. Keep merge on hold rather than weaken collection
progress. Verification includes focused routing/root/lifecycle tests, full Go
and static gates, relevant race checks, and the documented managed conformance
gate on the final production tree.
