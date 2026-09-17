# Protected builtin dispatch

Protected builtins used the runtime host callback to run `#0:bf_<name>`.
In direct eval this created another runtime task and VM for every call; in a
committing task it created a nested VM. The new path pushes an ordinary verb
frame on the existing VM, sharing its return, exception, and suspension logic.
Non-VM builtin callers retain the host callback.

The CPU profile attributed 32.09% cumulative time to `maybeProtectedRedirect`
and 28.00% to `engine.callVerbWithArgstr`. This is the path changed here.

## Measurement

Baseline production source: `52491410fe031c763d340638f37b10d05e2bd7e4`.
The baseline also includes the new benchmark and its `testing.TB` fixture
signature. After measurements use this PR's production changes, uncommitted
at measurement time. Generated corpus JSON records the baseline HEAD for
both builds; the binary hashes distinguish the binaries.

Windows Go 1.26.0, AMD Ryzen 9 5950X, `GOMAXPROCS=1`. Each benchmark operation
executes 1,000 calls, including eval compilation. The `Verb` case calls the
same wrapper directly and serves as a control. Setup is outside the timer.

Initial sequential samples:

```powershell
$env:GOMAXPROCS='1'
go test ./engine -run '^$' -bench '^BenchmarkProtectedBuiltinDispatch$' -benchmem -benchtime=500ms -count=10
benchstat before.txt after.txt
```

`benchstat.txt` reports protected dispatch 8.685 ms to 2.107 ms, allocated
bytes 2969.0 KiB to 342.3 KiB, and allocations 28,032 to 8,036. The direct
verb control also changed by -6.18%, so these timings are not an isolated
measurement of production throughput.

A subsequent sequential comparison pinned to logical CPU 2, with 1s samples,
had severe drift: direct-verb time doubled and the protected-call timing
difference was not significant (p=0.105). Its raw samples are retained as
`pinned-before.txt` and `pinned-after.txt`. The allocation reductions held.
Do not infer a stable latency multiplier from either sequential run.

The final comparison alternated before/after binaries for ten pairs, reversing
their order on odd pairs, pinned to logical CPU 31 with `GOMAXPROCS=1` and
500ms samples. `interleaved-benchstat.txt` reports:

```text
ProtectedBuiltinDispatch/Builtin  12.277 ms -> 2.608 ms  -78.76% (p=0.000 n=10)
ProtectedBuiltinDispatch/Verb      2.021 ms -> 2.092 ms        ~ (p=0.971 n=10)
Builtin bytes                    2968.9 KiB -> 342.3 KiB -88.47%
Builtin allocations                 28,032 -> 8,036     -71.33%
```

The unchanged direct-call control supports attributing this focused speedup
to the dispatch change. Samples still vary; the result is not a whole-server
latency or throughput claim. Run each precompiled test binary with
`-test.run '^$' -test.bench '^BenchmarkProtectedBuiltinDispatch$' -test.benchmem
-test.benchtime 500ms -test.count 1` to reproduce each sample.

## Representative corpus

`corpus.txt` is the 17-workload corpus from PR #307, run serially using
`scripts/bench_differ.py --corpus <corpus> --barn-linux <binary> --repeats 5`.
Both Barn binaries were built with WSL Debian Go 1.24.6. The baseline binary
was built in a separate clean worktree at the source revision above.
The oracle is `/root/src/toaststunt/build-release/moo`.

The protected-wrapper row (140,000 calls) changed from 748.46 ms to 238.84 ms.
All 17 rows returned matching values in both comparisons. Other workloads
and the oracle also drifted between runs; this supports the direction of
the focused change but does not establish whole-server throughput. Raw
transcripts and JSON are stored alongside this report.

No live multiplayer validation was performed, as requested.

## Correctness

`oracle.yaml` records managed conformance probes run first against Toast,
then Barn. Both sessions included capability admission and completed with
all four probes passing: same task and caller, suspend then return,
exception value, and non-executable-wrapper fallback. The caller probe
uses an explicit test-owned driver verb.

Go regressions cover these semantics, owned argument storage, direct eval,
and committing tasks. Before the change the identity probe returned `{0, 0}`
instead of `{1, 1}`, and a non-executable wrapper returned its body result
instead of falling through to the real builtin for a wizard.

Validation commands:

```text
go vet ./...
go test ./...
uv run --frozen moo-conformance <oracle.yaml> --server-command "<server command>" --moo-host=127.0.0.1 -v --strict-markers
```

For WSL testing of the Windows-created worktree, `GIT_DIR` and
`GIT_WORK_TREE` were set to their translated Linux paths so the repository
hygiene test could resolve Git metadata.
