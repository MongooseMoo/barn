# PR #345: retryable continuations and completed-work measurements

The root VM now restores a failed resumed slice from an in-memory checkpoint.
Earlier committed slices stay committed: input and threaded results are reused,
and SQL statements or HTTP requests issued by the prior slice are not repeated.
Supported continuations can share optimistic scheduler batches instead of taking
the exclusive commit gate merely because they have a saved VM.

## Execution ownership

- Capture runs under the task's physical execution lease, before consuming its
  wake value. The checkpoint copies mutable stack/frame storage, loops, handlers,
  pending errors/returns, native continuations and finalization bookkeeping.
  Immutable program/value payloads are shared. Restore installs the active frame
  through the production helper and rebinds services; persistence is not involved.
- A failed attempt discards its deferred effects and new forks, restores task
  context/local state and wake/error mode, and uses a fresh store snapshot.
  Suspension generations remain monotonic. Checkpoint values remain GC roots
  until the slice settles. Existing completion observers remain on the task.
- Replay starts with the slice-entry instruction count. The seconds deadline
  remains anchored to the first attempt, with existing contention-wait exclusions.
  The existing 63-conflict escalation policy protects the final attempt.
- Irreversible/coarse effects still cross their publishing boundary under the
  exclusive gate. Recycle reservation acquisition/release crosses that boundary
  too; an aborted validation cannot lose an earlier reservation. Admission waits
  are cancellable, including shared commit admission.

`sort` uses stable generic sorting of zero-based indices and direct string
comparisons. Numeric ordering retains the previous NaN/signed-zero behavior.
`all_members` reads the immutable element slice directly and returns one-based
indices. Thread-mode behavior, error delivery, natural ordering, key validation,
stable ties and whole-result reversal remain covered by differential tests.

## Measurement method

Control `aefe85c` contains original PR head `b33a487`, merged master `ab78a08`,
and the same completion harness/full-cohort benchmark used by candidate
`a3fcc7b`. Callback control functions preserve the original implementations in
the differential benchmark. Callback and checkpoint code is unchanged from
`4a4d12d`, where their test binaries were built.

Go 1.26.0, Windows/amd64, Ryzen 9 5950X, `GOMAXPROCS=4`. The driver serializes
its runs with `Global\BarnBenchmark`; host load and CPU frequency are not pinned.
Ten continuation control/candidate pairs alternate order. Callback implementations
run paired in one binary, with control first. Benchstat reports distributions;
these short local measurements do not establish general production gains.

The continuation workload completes four threaded sorts plus an arithmetic loop
per command. It waits for every task's terminal notification and checks its final
result. The shared-write variant updates the same property from all players.

| Completed continuation workload | Control commands/s | Candidate commands/s | Change |
| --- | ---: | ---: | ---: |
| 1 player, read-only | 4,366 | 4,537 | inconclusive, p=0.393 |
| 1 player, shared-write | 3,647 | 3,677 | inconclusive, p=0.739 |
| 16 players, read-only | 6,053 | 10,716 | +77.06%, p<0.001 |
| 16 players, shared-write | 4,974 | 4,038 | -18.81%, p<0.001 |

All rows have ten samples. Read-only concurrency benefits from removing the
exclusive gate. Shared-property contention performs more discarded work and
allocates more: the 16-player write cohort increases from 932.1 KiB to 1,617 KiB
and from 7,489 to 12,899 allocations. This is a material workload tradeoff.
An experimental escalation after two conflicts did not improve that workload
and was rejected. No worker pool or whole-input membership preallocation was added.

Callback time decreases 11-23% for the tested string/keyed sorts and 5-7% for
numeric sorts; sort allocations fall from five to three. Membership time decreases
9-10% for no/sparse matches. Dense matches are inconclusive (p=0.123).
Checkpoint capture at depth 1/eight locals costs 1.270 microseconds, 1,616 bytes
and eight allocations; restore costs 0.758 microseconds, 688 bytes and five
allocations. At depth 32/256 locals, capture is 136 microseconds and restore is
162 microseconds, with wide timing distributions. Raw samples and benchstat
output are in [the evidence directory](../experiments/2026-09-30-pr345/).

## Application completion inventory

The Mongoose harness retains the exact command task through its start hook,
waits through every suspension and final output flush, and allows one command
per connection at a time. Goodput uses the cohort's full elapsed time, including
bounded drain. Uncaught command errors, kills and timeouts cannot count as
completed successful work. Detached fork children are outside the command barrier.
The scheduler driver uses bounded ready batches and wake notifications.

The fixture is `mongoose.db.new`, 106,067,660 bytes, SHA-256
`489FF8D14884392DCFBA6CD88D407E53A2140C03D4F0CC4B1B19CF2AFD8A031E`.
Each run loads it read-only into memory and uses a fresh disposable file/SQLite
workspace. The mix is look/say/inventory/who/home at 1, 4 and 16 players, with a
one-second warm-up, three-second submission window and ten-second per-command
completion deadline. All five alternating pairs are retained, including failures.

All five control processes fail the completion criterion; four of five candidate
processes fail it. There is no clean full pair at all three player counts, so
there is no valid whole-application throughput ratio. Aggregated counts below
describe the inventories, not a performance comparison: submission windows
realize different command counts and all unsettled commands are failed work.

| Revision / players | Submitted | Completed | Failed / unsettled |
| --- | ---: | ---: | ---: |
| Control / 1 | 15 | 10 | 5 |
| Candidate / 1 | 42 | 38 | 4 |
| Control / 4 | 121 | 117 | 4 |
| Candidate / 4 | 311 | 310 | 1 |
| Control / 16 | 693 | 673 | 20 |
| Candidate / 16 | 600 | 595 | 5 |

Every failed measured command in these inventories was missing terminal flush
acknowledgement at its deadline. Timeout diagnostics include queued tasks with
`reading=-1`, `lease=0` and a saved VM. Most are `home`; a control run also has a
`who` timeout. Increasing a control probe's deadline to 60 seconds did not settle
all commands. These observations do not establish the underlying cause. All raw
process logs and a structured [application inventory](../experiments/2026-09-30-pr345/application-inventory.json)
are retained, without selecting successful runs to compute a ratio.

## Validation context

Final source is `d5ae6a1`; engine measurements use `a3fcc7b`, with identical
production code. The later source commit adds cancellation coverage and makes
the measurement driver reject invalid comparisons after collecting every pair.

Final local commands: `go test ./... -count=1 -timeout=240s`, `go vet ./...`,
`staticcheck ./...`, focused race tests across engine/builtins/VM/task/store, and
`go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn` followed by a command build.
Their terminal evidence is retained in
[validation.txt](../experiments/2026-09-30-pr345/validation.txt):

```text
go_all_exit=0
vet_exit=0
staticcheck_exit=0
focused_race_exit=0
github.com/MongooseMoo/barn/cmd/barn main
build_exit=0
```

The cancellation regression was also run with shared commit cancellation
disabled, then with the original source restored:

```text
FAIL ordinary: cancelled shared commit wait did not return
FAIL renewed: cancelled shared commit wait did not return
mutation_exit=1
ok github.com/MongooseMoo/barn/db/store 0.589s
restored_test_exit=0
```

Earlier full race coverage passed across VM/task/engine/builtins/store before the
final instruction-budget regression. The final focused race run includes that
64-attempt replay/escalation case and the new shared-commit cancellation cases.
The driver's negative probe collects all ten failed process inventories, rejects
the application comparison, and releases the benchmark mutex.

Managed conformance uses moo-conformance-tests `378ef52bf8f174e88ca7884a9c59c71332606104`.
Canonical WSL Toast selectors cover capability admission, threaded builtins,
sort/member call shapes and activation thread mode. Barn additionally covers the
relevant lifecycle paths. Before the final budget regression, full Windows Barn
runs on both candidate and control share these terminal results:

```text
Toast focused:    47 passed, 13027 deselected
Barn focused:    223 passed, 29 skipped, 12822 deselected
Barn candidate:  3 failed, 12631 passed, 440 skipped, 1 warning
Barn control:    3 failed, 12631 passed, 440 skipped, 1 warning
```

The three shared failures are `memory_usage_returns_floats`,
`audit_memory_usage_windows_behavior`, and `memory_usage_returns_five_elements`;
`memory_usage()` returns `E_FILE` in those Windows runs. An extra Toast lifecycle
probe also reproduces `next_recycled_object_returns_zero_when_none` against the
oracle itself; it is not treated as a candidate regression. Linux full managed
conformance is supplied by the exact-head PR checks through `CI_RUNNER_LABELS`.
