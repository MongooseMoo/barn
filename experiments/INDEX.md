# Campaign: Barn VM execution speed

## Frame

Primary development metric: geomean `sec/op` across exactly these current
`BenchmarkVM` rows, measured with the command below at `-cpu=1`:

- `int_arith_1M`
- `float_arith_1M`
- `string_concat_10k`
- `list_append_10k`
- `list_index_1M`
- `builtin_abs_200k`
- `tostr_200k`

```powershell
go test ./vm -run='^$' -bench='^BenchmarkVM/(int_arith_1M|float_arith_1M|string_concat_10k|list_append_10k|list_index_1M|builtin_abs_200k|tostr_200k)$' -benchmem -count=10 -cpu=1
```

`B/op` and `allocs/op` are mandatory regression guards. The campaign goal is a
cumulative reduction of at least 25% in the development-set geomean `sec/op`
relative to the current-HEAD baseline recorded below.

Holdout: exactly `BenchmarkVM/nested_1k` and `BenchmarkVM/list_iter_1M`. Do not
run, profile, tune from, or otherwise open these rows during campaign frame,
triage, or full experiments. Historical numbers are prior art only. The holdout
may be run later only by an independent verifier after a new user message
authorizes the exact candidate commit and evaluation identity.

Budget: 8 triage probes and at most 3 full experiments. Kill the campaign when
the budget is exhausted or after two consecutive completed triage rounds
produce no survivor. A triage result is never promotion evidence.

Full-experiment keep threshold: paired `-count=10` A/B on the exact development
command; at least 3% better development-set geomean `sec/op`; a statistically
significant coherent improvement (`p < 0.05`) in the targeted row or cluster;
no statistically significant development-row regression greater than 3%; no
`B/op` or `allocs/op` regression; and all required correctness/conformance gates
passing. Added complexity with benchstat `~` is rejected.

No candidate may be integrated or run on holdout without the separate verifier
and explicit user authorization required by the Campaign protocol.

## Current development baseline

Baseline commit: `86f1580f360ec33a755f0c0ff58bc8d165794de7`.

Artifacts:

- [Raw `-count=10` samples](2026-07-15-vm-dev-baseline.txt)
- [`benchstat` summary](2026-07-15-vm-dev-baseline-summary.txt)
- [Prior-art gate](../reports/barn-performance-prior-art-2026-07-15.md)

Each row has 10 samples. Timing and spread below are the median and confidence
interval reported by `benchstat`:

| Development row | `sec/op` median and CI | `B/op` median and CI | `allocs/op` median and CI |
|---|---:|---:|---:|
| `int_arith_1M` | 31.91m ± 2% | 8.656Ki ± 0% | 11.00 ± 0% |
| `float_arith_1M` | 32.94m ± 0% | 8.656Ki ± 0% | 11.00 ± 0% |
| `string_concat_10k` | 840.1µ ± 3% | 519.5Ki ± 0% | 10.03k ± 0% |
| `list_append_10k` | 1.021m ± 16% | 1.398Mi ± 0% | 10.05k ± 0% |
| `list_index_1M` | 80.34m ± 1% | 113.7Ki ± 0% | 1.034k ± 0% |
| `builtin_abs_200k` | 13.56m ± 4% | 8.656Ki ± 0% | 11.00 ± 0% |
| `tostr_200k` | 36.12m ± 1% | 12.21Mi ± 0% | 599.9k ± 0% |
| **geomean** | **11.98m** | **131.7Ki** | **701.1** |

Noise concern: `list_append_10k` reports a ±16% timing interval, materially
wider than the other development rows (±0–4%). The fixed baseline was not
rerun, warmed up separately, or selectively sampled.

## Authority boundary

Reversible development baseline, profile, triage, and full-experiment work
within this metric and budget is authorized. Holdout consumption, budget
expansion, goal or scope change, and integration or promotion require their
stated gates. In particular, holdout consumption requires a separate verifier
and a new user message authorizing the exact candidate commit and evaluation
identity.

## Correctness and conformance gates

Current documented control surfaces substantiate these exact commands. They are
recorded here for candidate work. During Frame, only the Go correctness gate
and the exact development baseline command above were run; no conformance or
oracle server was run.

Go correctness gate:

```powershell
go test ./...
```

Frame validation result at baseline commit: exit 1. All listed packages except
`barn/scheduler` passed; `barn/scheduler` failed at the pre-existing tracked
`TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent` regression
recorded in the prior-art gate. This is not a passing campaign correctness gate
and must be green before a full experiment can satisfy its keep threshold.

Preferred managed Barn conformance entrypoint from `README.md`:

```powershell
.\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788
```

When uncertain or surprising behavior must first be verified against Toast, the
managed WSL oracle command substantiated by the prior-art gate is:

```powershell
wsl --cd /mnt/c/Users/Q/code/moo-conformance-tests --exec bash -lc 'uv run moo-conformance --server-command="/root/src/toaststunt/build-release/moo {db} {db}.out -p {port}" -v'
```

No manual server command is a correctness gate.

## Candidates

| ID | Hypothesis | Status | Evidence | Cause of death / result |
|---|---|---|---|---|

## Round log

### Frame — 2026-07-15

Established and committed the current development baseline and campaign frame.
No candidate work occurred, and neither holdout row was run, profiled, tuned
from, or otherwise opened.
