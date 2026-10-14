# Barn gameplay diagnostics, October 14, 2026

The new tooling captured a real magic failure before cleanup and five Go
diagnostic artifacts around completed flight. These are private diagnostic
cohorts, not accepted performance baselines: magic failed and both Barn worlds
failed final checkpoint publication. No Go engine implementation changed, no
live Mongoose process was accessed, and no speedup or memory reduction is claimed.
Raw `.tmp` links below identify local preserved evidence, not shipped artifacts.

## Identity and collection

The tooling checkout was `aa8845c3b292b81e1b8e90ed5e84d45e78bafdb3` plus the
uncommitted sources copied into each run's `controller-sources` directory. Go
core remained `db01daf`. Inputs match the
[first Barn cohort report](mongoose-barn-gameplay-20261014.md): historical
checkpoint SHA-256 `fab6957acffa1db8bc1c4b911fb53a8e8d6e605f4de2a31b9f8e4fa874c3200d`,
sound catalog `ba15182e25c60e9795c10f8f9479095ff4d22ea9bf2a03d7d061446cfca241e4`,
Barn executable `2bb5a49b6da0736fe3424653f77b301eabef37e1eb65153906c11e9a12f04d9a`,
and compatible Toast bootstrap
`f96c79de3ac701557c7fe71e2a9be42816c188d6d085afba1a7b4b3c4e7c7a01`.
Registered gameplay options were `OUTBOUND_NETWORK=1`, `PROMOTE_NUMBERS=1`.
Network access remained inside a loopback-only namespace. Prepared per-run
bootstrap images are not established as equal across engines.

Frozen source SHA-256 identities:

| Module | SHA-256 |
| --- | --- |
| Error metadata projection | `cb3d8ea6f98359a04909260986cfcf779733c0471cade49e2a201c7bc9e41788` |
| Go diagnostic collector | `ad2f40227b6d630fd5a59778ee2166da4982157796f754d6dd738de440c113bb` |
| Gameplay runner | `79c0bcdb7b863958da7a0961282325c0f1d398f8177572db1fb51b591ee43a41` |
| Owned Barn lifecycle | `73178da970c37f81cdc4b3472448a890cd29e3af0e4ec1f24e7e59bd0a59110f` |

`BARN_RUNTIME_DIAGNOSTICS=1` selected debug logging. Go CPU collection requested
15 seconds after the before vars/natural heap snapshots; the after snapshots
followed attempted actions, failure capture if applicable, and CPU drain, before
cleanup. Request transmission is recorded, but profiler activation/action overlap
has no acknowledgement. CPU, action-clock and heap windows differ.

The collector bounds responses, deadlines and gzip expansion, pins exclusive
file descriptors and owned process identity, and retains malformed/unavailable
outcomes. Heap requests use `gc=0`; no forced GC was introduced. Raw profiles,
runtime logs and vars remain local. Transport completion alone never makes a
profile or gameplay cohort valid.

## Magic: the failing verb is now visible

[Barn magic result](../../.tmp/barn-magic-error-capture-02/magic/result.json)
retains `ScenarioFailure: expected ordinary refusal was absent` for the
`weave lumos with 0 focus` negative control. The actor-specific issues record
was available before cleanup and uniquely matched one bounded retained history
entry. Its parsed frames, innermost first, were:

```text
#55:make                 receiver #55       line 0
#59:_fix_preps           receiver #59       line 0
#59:full_prep            receiver #59       line 0
#6:my_huh                receiver #279303   line 0
#410:my_huh              receiver #279303   line 0
#56:do_huh               receiver #56       line 0
#2700:weave              receiver #277800   line 1
```

Most reported line positions are zero and do not identify an expression.
Formatted history does not provide effective programmer or per-frame player;
those fields remain unavailable. The issue's actor/time is event context, not
proof of exclusive command/task causality. Error code is unavailable in this
projection. `weave` is the command-facing frame name; immutable source has
`#2700:huh`, whose fallback invokes `do_huh`, rather than a literal saved
`#2700:weave` verb. Source `#55:make` is the real list-building helper called by
preposition normalization.

Projection happens inside MOO before FileIO or control recording. It exports
only bounded frame identifiers/positions, shape counters and correlation status;
messages, error values, source, arguments and runtime variables stay inside the
server. Limits are 15 history events, 32 frames/event, 256 verb characters,
1024 inspected header characters and 262144 exported bytes. Unsupported markers,
missing fields and truncation remain counted. Baseline capture follows setup;
failure capture precedes cleanup. Exclusive-file collisions/replacements are
neither followed nor deleted. Metadata failures cannot replace the primary
scenario failure.

The independent compatible-Toast native projection probe completed one actual
magic sequence with two attempts and no fizzles, owned shutdown exit 0 and all
five final checks true. Its baseline returned `ACK [1,8739,0]`. This establishes
real native projection execution, not stock-oracle conformance.

## Narrow semantic witness

The managed candidate
[repeated_list_halving.yaml](../../tests/mongoose-conformance/language/repeated_list_halving.yaml)
preserves the immutable 17-line `#55:make` body (fingerprint
`cc454a0bf08c08fac30bdad9d0bebb142bdaa5bf700cd1524fadaa4ff3594cfb`).
An explicit owned driver checks caller/receiver through ordinary programmer-owned
verbs. Separate assertions check integer division/assignment at the branch.
The initial candidate revision, before adding the caller-splice expressions,
produced the first two retained summaries:

```text
Stock WSL Toast, canonical capability admission plus both cases:
3 passed, 1 warning in 7.53s
Barn, same matched TestDB options and same cases:
3 passed, 1 warning in 6.73s
```

The warning concerns pytest `record_property` with JUnit xunit2. Evidence is
retained as `stock-list-halving-*` and `barn-list-halving-*` under
`.tmp/conformance`. Admission records confirm `OUTBOUND_NETWORK=true` and
`PROMOTE_NUMBERS=false`. The rebuilt managed Barn binary has the same pinned
SHA-256 as the gameplay binary. This rules out failure of the freshly installed
helper/primitive under those exact conditions; it does not explain the imported
Mongoose failure.

The final candidate additionally covers computed `length(all)-1` arguments and
direct splicing of the verb-call result for three input shapes, rather than only
splicing local lists inside the helper. Re-running admission and both cases gave:

```text
Stock WSL Toast: 3 passed, 1 warning in 6.89s
Matched Barn:   3 passed, 1 warning in 6.87s
```

These recordings are `stock-list-splice-*` and `barn-list-splice-*` under
`.tmp/conformance`; the final YAML SHA-256 is
`adb25e0648fbb9206a724b16941b5708fe90373262262d558759e1613edbc547`.
The saved magic logs do not recover the primary error code:
their coded `E_INVIND` traceback belongs to a different disconnect call chain,
so it is not joined to the preposition failure.

A separate Barn promotion-enabled probe did not execute either case:

```text
invalid-comparison: feature option.PROMOTE_NUMBERS differs:
oracle=False target=True
canonical capability admission did not pass;
packaged conformance was not executed
1 error in 6.01s
```

Its XML/config are retained separately. No gate was bypassed. Source inspection
shows pure int/int division precedes numeric promotion, but is not test coverage
for this unmatched configuration. Promotion-enabled reproduction remains unknown.

## Flight: measured CPU shares and memory

[Flight result](../../.tmp/barn-flight-go-diagnostics-02/flight/result.json)
completed one domain sequence. Its action measurement was 11.949701789s wall
and 14.76s whole-process user/system CPU. Ending RSS/high-water RSS was
3569476KiB. These include scheduled background and observer work. The outer
manifest is `invalidated` because the final checkpoint failed. Source and process
identity gates passed; all five Go artifacts were captured.

Offline Go 1.27.1 pprof commands parsed CPU and both heap profiles against the
pinned executable; all seven initial commands and the allocation difference
command exited 0. The Go command/tool hashes and complete output remain in
[the analysis directory](../../.tmp/barn-flight-go-analysis-02/tool-and-binary-sha256.txt).
The CPU profile has build ID `ad27135668346ddfcd0d83d6f0362cbd98aea1d5`, type
`cpu`, 15s duration and **19.94s nonzero sampled CPU** (132.93% across threads).
This independently validates samples; it does not rewrite the original capture
manifest, whose automated semantic validation remains pending.

| CPU path | Flat share | Cumulative share |
| --- | ---: | ---: |
| VM.executeLoop | 5.17% | 75.68% |
| Value.Equal | 2.91% | 4.61% |
| propTable.all.1 | 2.81% | 5.67% |
| startVerbCallResolved | 2.01% | 13.04% |
| pendingFinalizationValueInList | 1.91% | 5.12% |
| executeCallBuiltin | 1.10% | 27.93% |
| Session.dispatch | 1.00% | 23.72% |

Cumulative rows overlap and must not be added. Root `moo.verb` labels account for
17.37s/19.94s; `#1007:s_run` alone labels 13.32s (66.80%), `#9655:start_rearm`
2.67s (13.39%), and `#598:tick` 0.59s (2.96%). These identify root tasks, not the
currently executing nested verb. Foreground commands are a small part of this
captured window; the scheduled workload must remain visible in future comparisons.

Offline root-label filters retain the original 19.94s denominator. Under
`#1007:s_run`, `executeLoop` accounts for 11.64s cumulative and builtin dispatch
3.72s; under `#9655:start_rearm`, persistent WAIF root/finalization work accounts
for 0.40s cumulative. These paths overlap their enclosing execution and are
leads for task-specific investigation, not estimates of removable cost.

Natural sampled `inuse_space` was 709.73MB before and 737.23MB after. Ending
allocation sites include property-slot clones 138.54MB (18.79%), `NewMap`
106.43MB (14.44%), database value loading 93.16MB flat (12.64%),
`toastLookupInsert` 88.01MB (11.94%), and lists 43.50MB (5.90%). These sites
include loaded data and do not establish ownership, leaks or recoverable savings.

The sampled allocation difference between the two heap captures is 2.56GB.
Lists account for 15.86% flat, `maps.clone` 9.19%, `storeVerbMemoUse` 6.36%,
map keys 6.29%, strings 5.69%, and map pairs 5.49%. This is process-wide churn
between sampled snapshots, subject to heap accounting lag; it is not flight-only
allocation and is not the process-lifetime after profile's 8583.26MB total.

Ending vars report `HeapAlloc=808969336`, `HeapSys=3684630528`,
`HeapIdle=2378055680`, `HeapReleased=7372800`, and `NumGC=7` (before: 6).
Large idle reserved heap with little released is a concrete RSS lead, not proof
of a leak. Heap profiling reflects sampled allocation sites and the latest
completed GC; it need not equal MemStats. No forced-GC or idle-recovery comparison
was performed.

## Scheduling and next decisions

`barn_logs -runtime` processed the entire retained log, including startup and
cleanup, and returned 1 because the log contains the final-checkpoint error.
The summary contains 21323 phase records, zero malformed records, 158 threaded
starts/completions and zero rejected completions. Selected aggregate wall data:

```text
retry_waste:          359 attempts; 16.692979725s; 23,460,846 ticks
commit_gate:          1261 records; total 35.188438077s; max 845.020165ms
background_admission: 5736 records; total 64.070465408s; max 1.611930877s
resume_to_vm:         157 records; total 1.633006163s; max 671.845705ms
sqlite_handle_wait:   349 records; total 1.893075ms
sqlite_work:          349 records; total 265.646631ms
```

These intervals overlap across concurrent work, are wall rather than CPU time,
and are not attributable to flight by surrounding counter deltas. The SQL handle
data does not support prioritizing handle contention in this particular run.

First isolate the imported magic/preposition failure and queued-task checkpoint
failure with deterministic managed witnesses. Next rank background simulation,
discarded retries and list/map churn using task-filtered evidence and a valid
repeatable gameplay cohort. Reader/history/finalization retention census,
natural idle recovery, nested MOO CPU attribution, profiler overhead and
scheduled-arrival latency remain missing. Isolation-sensitive changes require
the pinned Elle workflow, not only Go tests. No engine optimization is accepted
from these diagnostic results.

The frozen tooling suite ran `195 tests in 0.650s / OK`; an independent read-only
review found no blocking ownership/privacy/failure-retention issue. Tests and
profile parsing do not override the retained failed gameplay/checkpoint outcomes.
