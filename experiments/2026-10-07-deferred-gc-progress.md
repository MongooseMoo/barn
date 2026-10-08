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
