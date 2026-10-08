# Deferred finalization progress (#270)

## Contract and measurement plan (before production changes)

Baseline: `34e6622`, current origin/master when the worktree was created.
Branch: `fix/270-deferred-gc-progress` in `C:/Users/Q/code/barn-issue-270`.
Authorized: update #270, implement and verify locally. Publication and merge
are separate milestones and have not been authorized.

The baseline regressions fail because throttled pending anonymous collection
has no idle wake, and overdue collection never excludes fresh admissions while
other physical VM leases remain active. The dispatcher change from #398 stays.

The change is an independent deadline-driven finalization worker using the
existing admission pause and GC barriers. It must hold no execution lease or
admission reservation when requesting the pause. Task-boundary collection stays
prompt when cheap; the collector still fails closed on active VM roots.

Primary measurement: the number of collectible anonymous objects retained
after 500 ms of complete runtime idle. `TestDeferredGCProgressMeasurement`
creates exactly 1,000 through MOO execution under the startup hold, seeds a
preceding expensive sweep with its deadline 100 ms away, releases the hold,
and submits no more work. The candidate must retain zero in all five pairs;
the baseline must retain 1,000. This measures progress, not a production speedup.

Command:
`BARN_GC_PROGRESS_MEASURE=1 go test ./engine -run '^TestDeferredGCProgressMeasurement$' -count=1 -v`

Secondary guard: five alternating baseline/candidate runs of
`go test ./engine -run '^$' -bench '^BenchmarkDeferredGCCommands$' -benchmem -benchtime=1s -count=1`.
Both the plain and anonymous-allocation command must return the same result.
Record every sample, median throughput time, allocations, and measured p99.
Do not claim faster command service from these small synthetic workloads.
A median cost increase over 10% calls for investigation rather than dismissal.

The harness is `engine/deferred_gc_progress_test.go`, committed before the
production delta and copied unchanged onto the baseline measurement worktree.
Additional safety tests may be added separately, without changing these
measurement functions. Measurements use the same host, Go version and
GOMAXPROCS=4, sequentially; no Mongoose database or external sound files are used.

Verification: focused red/green tests, GC and shutdown/root lifecycle tests,
repository Go/static/bench-driver gates, race checks on store and engine, and
the managed MOO conformance gate. Any uncertain MOO behavior is verified on
the canonical WSL Toast oracle first.

## Initial evidence

`go test ./engine -run '^TestDeferredGCProgress' -count=1 -timeout=20s`:

```text
throttled anonymous collection never ran after the runtime became idle
overdue collection never excluded fresh admissions to drain overlapping VMs
red_test_exit=1
```

The initial managed Toast selection used the Makefile's `{db}.out` output and
failed eight `.db.new` file assertions. `scripts/run_toast_wsl.sh` documents
that managed restart adopts `{db}.new`. The Makefile output was corrected;
the same selected run is being repeated. No conformance assertions changed.
An attempted positional YAML-file narrowing was rejected by the CLI and ran
no tests; use the supported `-k` selection for this run.

## Implementation and review

Contracts/measurement plan: `274419e`. Production and safety tests: `5ae0bc6`.
Harness blob in both measurement trees:
`21c7e32da501a8a975d755f4ad654129553e4ebc`.

The runtime starts one maintenance goroutine and joins it on Stop. Enqueue
records the first pending time; cheap collections allow ordinary boundaries
first, while expensive collections retain the previous two-second cadence.
The worker requests admission Pause outside all task reservations and leases.
It then uses the existing root/sweep barriers. Opportunistic sweeps use TryLock
so a borrowed reservation cannot block a maintenance drain. Contended barriers
leave a retry deadline, release admission, and remain cancellable. Timed wakes
recheck the pending work before pausing, avoiding maintenance on a stale timer.

Safety checks cover cancellation during admission drain, checkpoint barrier
contention, startup/shutdown transfer, and suspended anonymous roots. Existing
nested sweep/reentrant run_gc, eval/server-hook exclusion, panic cleanup, and
lifecycle checks hold. Two old shutdown fixtures directly assigned pending
queues without locking or registration; they now use AdoptPendingFinalizations
with their assertions preserved. The new shutdown fixture was also corrected
to use that loaded-root path rather than a task-owned request without a VM.

The manual review checked acquisition order (admission drain before sweep and
VM-start barriers), lease publication, preempted-owner readmission, startup
holds, shutdown ownership, and cancellation. The implementation retains the
tracing collector and asynchronous dispatcher; it changes the opportunity to
run collection rather than ignoring live VM roots or redefining MOO behavior.
No subagents were requested or dispatched.

## Paired results

Go `go1.27.1 windows/amd64`, AMD Ryzen 9 5950X, GOMAXPROCS=4.
Five sequential alternating base/candidate pairs, with the source/harness above.
All workloads returned the required result. Raw samples are the twenty
`*-retention.txt` / `*-latency.txt` files in
`experiments/2026-10-07-deferred-gc-progress-evidence/`.

| Pair | Base retained | Candidate retained | Base plain ns/op | Candidate plain ns/op | Base anonymous ns/op | Candidate anonymous ns/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1000 | 0 | 16239 | 14589 | 287735 | 292559 |
| 2 | 1000 | 0 | 15328 | 14160 | 293105 | 301754 |
| 3 | 1000 | 0 | 15092 | 15871 | 319178 | 331992 |
| 4 | 1000 | 0 | 15639 | 15900 | 313285 | 326088 |
| 5 | 1000 | 0 | 16701 | 14054 | 283353 | 302707 |

The primary retention criterion holds in every pair: 1,000 allocations,
500–501 ms observation, 1,000 retained on the baseline and none on the candidate.
Median paired command-cost ratio: plain 0.92380, anonymous 1.04015. The anonymous
case incurs about 4% added service time in this synthetic comparison, within
the stated 10% investigation threshold. Allocations stay 82/op plain and 198/op
anonymous. Candidate anonymous p99 ranges 1.568–1.896 ms versus 1.565–1.595 ms
on the baseline; pair four's tail is worse and is retained in the evidence.
The plain-case lower timing is not a claimed speedup. These small workloads
establish collection progress and its observed cost, not Mongoose throughput.

## Verification so far

Canonical managed WSL Toast, after the output-name repair:
`make conformance-toast K="anonymous or waif or shutdown" CONFORMANCE_ENV=/root/.cache/barn-270-conformance-linux CONFORMANCE_ARGS=--tb=short`
returned `135 passed, 61 skipped, 12894 deselected`.

Conformance repository: clean tracked tree at `7f05b70`.
Linux Barn SHA-256:
`343bb689834f91489a3c35012e75d3cc1e6c6bc50cd9ba5bf78ff2b017722cc2`.

- `go test ./... -count=1 -timeout=300s`: exit 0.
- `go vet ./...`: exit 0.
- `go build ./...`: exit 0.
- Changed Go files: gofmt output empty.
- `python -m unittest discover -s scripts -p 'test_*.py'`: exit 0.
- Installed staticcheck v0.7.0 cannot decode Go 1.27 export data; the repository
  pin `go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...` returns exit 0.

Race and full managed Barn conformance runs are still active at this checkpoint.
Their generated logs/XML are local diagnostics, not part of the committed
measurement sample set. Final results will be appended after terminal output.

## Final verification

`go test -race ./db/store ./engine/... ./types -count=1 -timeout=600s`
returned exit 0 (engine 234.511 seconds). This includes every engine internal
package, the store, and the value types.

Full managed Barn command (Debian WSL, from the implementation worktree):

```text
make conformance CONFORMANCE_ENV=/root/.cache/barn-270-conformance-linux CONFORMANCE_ARGS="--tb=short --junitxml=/mnt/c/Users/Q/code/barn-issue-270/experiments/2026-10-07-deferred-gc-progress-evidence/conformance-results.xml"
```

Terminal output:

```text
12670 passed, 420 skipped, 1 warning in 933.18s (0:15:33)
managed_conformance_exit=0
```

Canonical capability admission ran in this session; strict marker and
unexpected-skip enforcement were enabled. The warning concerns pytest's
record_property with xunit2 JUnit output; the gate exited successfully.

The complete Go, static, race, and managed gates cover the production tree at
`5ae0bc6`. The later commits contain documentation/measurement outputs only;
`git diff --quiet 5ae0bc6 HEAD -- engine Makefile` returns exit 0. Source diff
whitespace checks pass; captured raw text files preserve native Windows CRLF
and Go's padding and are intentionally excluded from that whitespace check.

Outcome: implemented and verified locally, with the liveness fix and the
observed synthetic cost recorded. No CI was triggered and no branch/PR was
published or merged. #270 remains open. The original checkout and unrelated
worktrees were preserved; the baseline measurement worktree and generated gate
logs/XML remain available locally.
