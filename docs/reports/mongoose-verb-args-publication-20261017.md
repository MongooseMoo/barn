# Barn Mongoose verb-argument publication experiment

Master c840f0eb was incorporated through the two-parent integration f214d4bf.
The diagnostic implementation is 02fe102e; the measured performance candidate is
2b787862244a90a544ce881ab2c31093f898eca2. Results apply to that candidate.

The warmed Store.SetVerbArgs plus transaction-adoption benchmark used 35.6% less
time and allocated 33.1% fewer bytes per operation in its large-object row.
The fresh offline Mongoose cohort still failed its original zero-focus refusal
control and produced no verified productive pair. This is a component improvement,
not a completed gameplay performance baseline or a measured RSS reduction.

## Change and experiment

Numbered-object argument publication now replaces the resolved target node while
sharing unchanged verb nodes only after their names/source arrays have been
defensively detached from constructor or loader inputs. Code installation clears
that proof. All non-verb defensive copies remain: retained builder input slices
also needed detachment. Alias/wildcard winners, history pruning, stamps, shape
invalidation, errors and anonymous behavior are preserved. Transaction adoption
continues to make its existing full copies.

Setup/preregistration f4e72c5e and calibration 98d92419 precede production edits.
The contracts, benchmark, runner and calibration plan are recorded under
`experiments/2026-10-17-verb-args-publication.md`. Ten fixed alternating A/B pairs
produced 160 comparison rows; baseline-only calibration produced eight rows.
Each operation alternates two fixed argument specs after one warm operation.
The adoption rows open and release a snapshot within each operation; this includes
first materialization plus adoption, rather than measuring an entire MOO builtin.
Setup, compilation and final-state validation are outside the timed region.

Linux amd64, Go 1.27.1, GOMAXPROCS=1, GOGC=100, GOMEMLIMIT unset. Primary gate:
large live-publication median paired time ratio <=0.80, upper 95% paired bootstrap
endpoint <1, tiny live control ratio <=1.05. All ten pairs are retained, including
one tiny adoption pair with ratio 1.003885. An independent parser/bootstrap matched
every raw name, count, metric, hash, order and analysis row.

| Path | Verbs / source lines | Median paired time ratio | Allocated bytes/op, baseline to candidate | Allocations/op |
| --- | --- | ---: | ---: | ---: |
| Live | 1 / 0 | 0.874469 | 1,048 to 1,000 | 10 to 9 |
| Live | 16 / 64 | 0.152868 | 24,784 to 1,848 | 61 to 11 |
| Live | 128 / 64 | 0.067775 | 193,424 to 8,504 | 397 to 11 |
| Live | 128 / 512 | 0.010025 | 1,258,384 to 8,504 | 397 to 11 |
| Setter + adoption | 1 / 0 | 0.952576 | 4,152 to 4,104 | 37 to 36 |
| Setter + adoption | 16 / 64 | 0.705159 | 75,360 to 52,424 | 190 to 140 |
| Setter + adoption | 128 / 64 | 0.554201 | 581,280 to 396,360 | 1,198 to 812 |
| Setter + adoption | 128 / 512 | 0.644307 | 3,776,161 to 2,526,281 | 1,198 to 812 |

The primary interval is [0.0084902, 0.0107181]; the large adoption interval is
[0.630801, 0.664854]. These are approximate percentile bootstrap intervals for
the median of paired ratios, not ratios of the separately calculated side medians.
Machine-readable results are in
`experiments/verb-args-publication-20261017-results.json`.

## Correctness and evidence

Raw terminal summaries:

```text
go test ./... -count=1: exit 0
race (8 packages): exit 0
Elle driver: exit 0
healthy verdicts: 6 true / exit 0
deliberate faults: 6 false / exit 1, each with a nonempty witness
coverage: 20 positive keys per healthy seed; native_exception 48 each
healthy raw EDN fail/info: 0/0
source-before/after: 1706 identical files; clean HEAD 2b787862
archive: 67 files byte-matched against preserved raw evidence
INDEPENDENT_TERMINAL_AUDIT PASS
INDEPENDENT_COHORT_ARTIFACT_AUDIT PASS
```

The initial private outcome auditor incorrectly required explicit zero entries
in Go's sparse frequency map. Its script and failure remain preserved. The second
audit counts anchored raw EDN outcomes and matches the source's sparse-map contract;
no test, checker or history was rerun to repair that audit.

The standard, untagged Barn executable is SHA-256
`dc52820cb2a7c0f35dab96a39903d868264a50260ccd75d91672754194d1815d`,
27,281,328 bytes, with the existing command PGO input. It has no embedded VCS fields;
the build command and complete before/after source inventory bind it explicitly.

Raw component and correctness evidence remains in sibling worktree
`C:/Users/Q/code/barn-verb-args-profile/.tmp/verb-args-experiment-20261017-01`
and `.tmp/verb-args-verification/run-01`. Independent reviews are under
`.tmp/verb-args-final-review` there and under the gameplay worktree's
`.tmp/master-refresh-20261017/verb-args-plan-review`.

## Actual offline Mongoose follow-up

Exactly one cohort used the original checkpoint/catalog/bootstrap, eight matching
controller sources, seed 1, one magic iteration, 30-second warmup, 120-second
action deadline and 15-second Go CPU profile in an isolated network namespace.

```text
domain: failed, expected ordinary refusal was absent
control: zero_focus_lumos
productive_pairs_verified: 0
source/ownership/checkpoint gates: 10/10 true
shutdown: exit 0, forced false, checkpoint verified
offline profile validation: 11 commands, all exit 0, CPU/heap types nonzero
terminal: task 1788854374, #277800:weave, E_MAXREC/seconds_budget, committed true
```

The action log does not expose a command-to-task identity join. The terminal row
identifies the weave task; timing alone is not an explicit command join. All 62
terminal rows are retained (60 E_INVARG, one E_INVIND, one E_MAXREC). Original
collector manifests keep `go_profiles_validated=false` and `source_commit=null`;
separate external validation and explicit source/binary bindings provide the
evidence without rewriting those records.

The failed action window is 12.5387 seconds with 19.50 seconds of whole-process
CPU across threads. The CPU profile spans 15 seconds and 23.01 sampled CPU seconds.
RSS was 3,470,876 to 3,809,776 KiB, observer peak 3,810,208 KiB. Natural sampled
heap in-use was 702,733,258 to 738,463,584 bytes. These windows and failed-domain
work mixes do not establish an end-to-end timing or memory improvement.

Exact stack grouping of the two existing weave profiles narrows the next target:

| cloneVerbForReadTxn sampled CPU seconds | Old failed cohort | New failed cohort |
| --- | ---: | ---: |
| Store.SetVerbArgs publication | 3.06 | No samples |
| AdoptLiveVerbs | 2.82 | 4.25 |
| Total | 5.88 | 4.25 |

All old publication samples specifically pass through Store.SetVerbArgs; SetInfo,
SetCode and commit writers are absent from these clone chains. The new adoption
source-array-copy line alone accounts for 2.36 sampled CPU seconds. Both tasks hit
the seconds budget (9.86 and 9.96 task-labelled CPU seconds), so different amounts
of work before termination prevent a throughput or call-count inference.

The follow-up artifacts are in the gameplay worktree at
`.tmp/barn-magic-verb-args-20261017-01/analysis/{safe-summary.json,final-bindings.json}`;
the exact caller comparison is
`.tmp/master-refresh-20261017/magic-candidate-plan/clone-caller-comparison.json`.
The next hypothesis is sharing proven-owned immutable source arrays while retaining
private verb nodes and copied names during adoption and full object cloning.
