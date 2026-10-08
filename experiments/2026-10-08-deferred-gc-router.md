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

## Implementation and scope review

Contracts/plan: `90a6183`. Production: `0aa7805`. A five-line guard exits the
request loop when the number of handled unique IDs equals the frozen list size.
No callback remains possible at that point. Routing still visits requests and
candidates in their original order and records an ID before calling recycle.
Nil contexts, later lower floors, callback errors and hook-created objects retain
their earlier behavior. No locks, tracing roots, cadence, or scheduler paths change.
The store supplies unique candidates; duplicate input would merely prevent the
early exit, retaining the previous exactly-once behavior.

The contract tests pass both before and after this performance-only change;
there is no invented red timing assertion. The fixed microbenchmark captures
the original request-count scaling and acknowledges every callback. Existing
allocation and hook-created-object tests and focused engine GC/shutdown tests
also passed. The review was local; no independent agents were dispatched.

## Five-triple results

Concurrent harness blob in all three worktrees:
`121d034527dfa4aeef15fc330d35e731dfe64b1e`.
Routing contract/benchmark blob: `bb5e2216aebb0707ddcd080505b494982a6a0f31`.
Pre-router worktree is detached at `90a6183`; optimized production is `0aa7805`.
All fifteen concurrent samples and fifteen microbenchmark samples exited 0.
Both complete CSV inventories and all thirty raw text samples are preserved in
`2026-10-08-deferred-gc-router-evidence/`.

| Triple | Short base max ms | Short pre max ms | Short optimized max ms | Long base max ms | Long pre max ms | Long optimized max ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 3223.780 | 2633.997 | 632.580 | 0.536 | 13.664 | 8.844 |
| 2 | 846.970 | 2794.308 | 698.861 | 247.211 | 6.450 | 5.332 |
| 3 | 1204.937 | 1056.111 | 688.721 | 19.533 | 34.609 | 4.748 |
| 4 | 2193.087 | 2509.686 | 499.742 | 0.000 | 8.061 | 16.518 |
| 5 | 2708.527 | 2495.206 | 561.711 | 0.301 | 10.172 | 8.664 |

The declared regression guard has zero optimized crossings against original
baseline in both scenarios. Longer-slice optimized admission maxima are
3.704/5.128/0.532/6.192/4.918 ms. Rapid-allocation command stalls are shorter
than both matched controls in every triple, but remain 500–699 ms; this change
does not eliminate bulk recycle cost. Some cheap readings round to zero with
the observed Windows clock resolution.

Median paired short-case maximum-latency ratio versus pre-router is 0.24016;
long-case ratio is 0.82667, with one optimized maximum higher than its pre-router
control. These are descriptive synthetic results, not a production speedup or
a product responsiveness guarantee. Background completion counts and candidate
snapshots are retained in the CSV rather than assumed equal between policies.

| Triple | Pre 4096-request ns/op | Optimized 4096-request ns/op |
| --- | ---: | ---: |
| 1 | 53300260 | 36441 |
| 2 | 60627720 | 33298 |
| 3 | 67834660 | 35182 |
| 4 | 54383800 | 35695 |
| 5 | 54023660 | 36220 |

With the same 1,024 candidates, optimized 1/128/4,096-request runs remain around
33–39 microseconds/op. All variants use five allocations/op. Median paired
optimized/pre time ratios for 1/128/4,096 requests are
1.00617/0.02024/0.00065635. The claim is removal of redundant scans after list
exhaustion; worst-case routing before exhaustion can still scan multiple times.

A live one-second CPU snapshot during sampling showed unrelated work (including
Docker 1.41 CPU seconds and several Node processes near one CPU second). No
user process was changed. This shared-host interference and the elevated long
task quota limit any inference about production latency, particularly the large
original-baseline outlier in triple two. All samples remain included.

## Profile holdout

A separate optimized rapid-allocation profile, excluded from the triples,
processed 27,030 background invocations in the observation window. The routing
profile shows 10 ms cumulative at the already-handled check, versus 1.09 seconds
in the earlier separate unoptimized profile. Recycle callbacks now account for
900 ms of the router's 910 ms cumulative CPU. Work counts/host conditions differ,
so this is attribution supporting the fixed microbenchmark, not a paired CPU
speedup estimate. Maximum command delay was 660.957 ms; sweep service 985.434 ms.
The remaining bulk-recycling cost is a distinct optimization question.

The binary/profile remain local; readable profile output is committed with the
sample evidence. No scheduler safepoint change is warranted by this bounded result.

## Final verification

The full Go suite, vet, build, pinned staticcheck v0.8.1, and Python benchmark
driver gate exited 0 on the optimized production tree. Relevant race checks
(`go test -race ./db/store ./engine/... ./vm ./types -count=1 -timeout=600s`)
also exited 0 (engine 126.591s; VM 56.303s). Canonical managed WSL Toast with
`K="anonymous or waif or shutdown"`, retaining capability admission, returned
`135 passed, 61 skipped, 12894 deselected in 386.54s`; managed oracle exit 0.
The full managed Barn conformance command (Debian WSL, from this worktree) was:

```text
make conformance CONFORMANCE_ENV=/root/.cache/barn-270-conformance-linux CONFORMANCE_ARGS="--tb=short --junitxml=/mnt/c/Users/Q/code/barn-issue-270/experiments/2026-10-08-deferred-gc-router-evidence/conformance-results.xml"
```

Terminal output:

```text
12670 passed, 420 skipped, 1 warning in 1325.40s (0:22:05)
managed_conformance_exit=0
```

JUnit contains 13,090 cases, zero failures/errors, and 420 skips. The warning is
pytest's existing `record_property`/xunit2 format warning. Canonical capability
admission, strict markers, and unexpected-skip enforcement were enabled. The
tracked conformance suite is `7f05b7070f2954a200f7e8c2a06c814ef7981c4e`;
its existing untracked material was preserved. Linux Barn SHA-256:
`c7dbae3be4627f332aba652afd0af51db17562231d308a21fa40db82b5ac93a0`.

All gates cover optimized production `0aa7805`; subsequent changes are records
and measurement outputs. Source/configuration comparison against that tree exits
0, and the changed Go files have empty gofmt output. Complete commands also
included `go test ./... -count=1 -timeout=300s`, `go vet ./...`, `go build ./...`,
`go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...`, and
`python -m unittest discover -s scripts -p 'test_*.py'`.

Decision: accept this bounded optimization and proceed toward a PR. The local
correctness gates and defined synthetic regression guard pass; the shared-host
and bulk-recycle limits above remain relevant. Exact-head CI is still required
before an authorized merge. No broader scheduler change, PR publication, or merge
was performed as part of this scoped implementation.

Raw benchmark output retains Windows CRLF and Go's padded CPU description.
Strict whitespace checks apply to source/records; raw-output checks account for
those captured formatting characters rather than rewriting measurement evidence.
