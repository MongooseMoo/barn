# Collection iteration without temporary pair lists

Indexed list and map loops previously allocated a two-element list for every
input entry before evaluating the loop body. This change retains list snapshots
directly and prepares maps as separate value/key lists in the existing tree
order. String indexes are derived from the cursor rather than pair lists.
Map keys stay rooted even in value-only loops. New opcodes leave the meaning of
saved pair-based bytecode intact; compiler temporaries remain ordinary Values
that the existing checkpoint and finalization machinery can visit.

## Measurement

Base: `501b83bb2c80a9f1fb777688551b26a258884076`. Candidate: this change.
Go 1.26.0, Windows/amd64, AMD Ryzen 9 5950X, `-test.cpu=1`.
Ten alternating baseline/candidate rounds, reversing order every round, with
`-test.benchtime=100ms -test.count=1`. Baseline was compiled before production
edits. Both binaries run the same benchmark body; the baseline's investigation
name `BenchmarkFilterSurvey` was normalized to `BenchmarkCollectionIteration`
in base.txt solely for benchstat. base-raw.txt retains the original benchmark
names and samples. Archived text normalizes line endings and trailing whitespace.
CPU frequency was not pinned. No concurrent test workloads were run during
measurement. These are synthetic VM filters, not live Mongoose throughput.

Fixtures and compilation are outside the timer. VM/context construction, loop
execution, and return-value checking are included. `rejectfalse` selects half
the elements; `rejecttrue` selects none. `count` is a diagnostic that counts
matches instead of building a list, not an equivalent optimized implementation.

Selected benchstat time rows (10,000 inputs, ten samples each):

```text
indexed/rejectfalse   3.243 ms -> 1.785 ms   -44.96% (p=0.000 n=10)
map/rejectfalse       3.602 ms -> 2.162 ms   -39.99% (p=0.000 n=10)
indexed/rejecttrue    2.109 ms -> 0.692 ms   -67.18% (p=0.000 n=10)
map/rejecttrue        2.546 ms -> 1.123 ms   -55.87% (p=0.000 n=10)
```

Reject-all indexed allocations fall from 20,021 to 19 per operation, and map
allocations from 20,022 to 23. Half-selected output still allocates intermediate
list headers: indexed 25,053 to 5,051; map 25,054 to 5,055. Header elimination
and SIMD were not implemented. Map snapshots still require O(n) bytes, but only
a constant number of allocations. Plain-list and count controls did not regress
in this sample set; consult all rows rather than treating the selected cases as
an application-wide speedup.

Raw samples, full benchstat output, and unit-test output are under
`experiments/2026-09-25-collection-iteration/`.

## Verification

The allocation regression was added first. On the baseline, increasing inputs
from 10 to 1,000 produced list allocations 33 -> 2,013 and map allocations
34 -> 2,014, failing the fixed-allocation requirement. Candidate results are
11 -> 11 and 15 -> 15.

Commands:

```text
go test ./... -count=1
go test -race ./vm ./types -run 'Test(IndexedIterationAllocationGrowth|ColumnLoad|ValueOnlyMapIteration|PairIterationBytecode|SuspendedLoop|DispatchFastPathParity|MapColumns)' -count=1
go test ./bytecode -count=1
git diff --check
```

Full Go suite exit: 0. Focused race check: vm and types passed. Tests cover
map traversal order and slice ownership, map-key roots in value-only loops,
overwritten finalizable bindings, duplicate loop variables, fast/generic
dispatch parity, old pair bytecode, malformed operand validation, and checkpoint
round trips for indexed lists, both map loop forms, and indexed strings.

Managed conformance ran in the verified Debian WSL distribution, using
`scripts/check-conformance.sh`, the stock Test.db fixture, canonical capability
admission, and a separate WSL uv environment:

```text
UV_PROJECT_ENVIRONMENT=/root/.cache/barn-direct-iteration-conformance
toast /root/src/toaststunt/build-release/moo /tmp/barn-direct-iteration-oracle looping _tests/language/looping.yaml
30 passed, 1 warning in 51.94s
toast /root/src/toaststunt/build-release/moo /tmp/barn-direct-iteration-oracle-snapshot 'iteration_snapshot or tick_accounting' _tests/language/iteration_snapshot.yaml _tests/vm/tick_accounting.yaml
71 passed, 1 warning in 114.53s
barn .tmp/iteration-perf/barn-linux /tmp/barn-direct-iteration-candidate 'looping or iteration_snapshot or tick_accounting' _tests/language/looping.yaml _tests/language/iteration_snapshot.yaml _tests/vm/tick_accounting.yaml
100 passed, 1 warning in 167.19s
```

Each engine command above is an argument list for the managed script. Barn was
cross-compiled with GOOS=linux GOARCH=amd64 CGO_ENABLED=0 after resolving
`./cmd/barn` with go list. The warning concerns pytest record_property with
xunit2 output. This is focused conformance, not the full conformance suite.
The new snapshot cases reside in moo-conformance-tests at
`src/moo_conformance/_tests/language/iteration_snapshot.yaml`, commit `691715d`.
