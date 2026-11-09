# Barn Mongoose immutable verb-source sharing

Candidate `5a1790b3cdfa9fe97fdba57776780b93c6408668` follows the integrated
verb-argument publication optimization at `e66500c9`. The large warmed
publication-plus-adoption benchmark uses 94.4% less paired time and allocates
96.0% fewer bytes per operation. **NO-MERGE:** the actual offline Mongoose run
failed its original magic control and failed two required preservation gates.
The candidate remains on its isolated local experiment branch. Component gains
do not override the failed operational check or establish gameplay/RSS gains.

## Change and measurement

`cloneVerbForReadTxn` still allocates a fresh node and copies names. It shares
source arrays only when the incoming node already proves detachment from borrowed
constructor/loader inputs. Otherwise it defensively copies source. Source writes
still replace the array, refresh CodeKey and clear the proof. No permission,
quota, retry, name-sharing or published-node mutation changes were made.

The prior offline cohort attributed 4.25 sampled task CPU seconds to this clone
through AdoptLiveVerbs, including 2.36 seconds on its source-array copy line.
That task hit its seconds budget and the ordinary-refusal control failed;
these samples motivated the component study without establishing gameplay speed.

Baseline-passing contracts were committed at `057fa090`; calibration was sealed
at `7a337728`, both before production edits. The unchanged eight benchmark rows
cover Live and Adopt, with 1/16/128 verbs, 0/64/512 source lines and three aliases.
Adopt includes a fresh snapshot's materialization plus adoption within each
operation. One complete warm operation precedes measurement; no reader survives
between operations. It is not a complete MOO builtin or gameplay measurement.

Ten alternating A/B pairs, reverse row order on even pairs, Linux Go 1.27.1,
GOMAXPROCS=1, GOGC=100, GOMEMLIMIT unset. Baseline-only calibration fixed row
counts at Live 50000/50000/50000/45000 and Adopt 50000/20000/3000/1000. No builds,
tests or worlds overlapped timing. Every sample is retained.

| Path | Verbs / lines | Median paired time ratio | Baseline to candidate B/op | Allocations/op |
| --- | --- | ---: | ---: | ---: |
| Live | 1 / 0 | 1.010314 | 1,000 to 1,000 | 9 to 9 |
| Live | 16 / 64 | 1.025832 | 1,848 to 1,848 | 11 to 11 |
| Live | 128 / 64 | 1.025603 | 8,504 to 8,504 | 11 to 11 |
| Live | 128 / 512 | 0.986960 | 8,504 to 8,504 | 11 to 11 |
| Setter + adoption | 1 / 0 | 0.975719 | 4,104 to 4,104 | 36 to 36 |
| Setter + adoption | 16 / 64 | 0.433581 | 52,424 to 15,560 | 140 to 108 |
| Setter + adoption | 128 / 64 | 0.320472 | 396,360 to 101,448 | 812 to 556 |
| Setter + adoption | 128 / 512 | 0.055804 | 2,526,281 to 101,449 | 812 to 556 |

Primary Adopt/V128/L512 paired ratio is 0.055803750451150005, with approximate
95% paired bootstrap interval [0.05355929923149275, 0.05691167195088399]. The
preregistered <=0.80/upper<1 gate passes. Tiny Adopt median 0.9757192038780216
passes <=1.05; its interval [0.9214422158548233, 1.0073158425832491] crosses one,
and four pairs regress, worst 1.1345724907063197. Live medians regress up to
2.5832%; this is not an all-shapes-faster result. Bootstrap uses 10000 resamples,
seed20261017 and fixed row order; the gate uses medians of paired ratios, not
ratios of separate side medians. Two independent parsers reproduced all168raw
hashes/counts/names/order and all eight medians/intervals.

## Correctness and build provenance

```text
go test ./... -count=1: exit 0
race (8 packages): exit 0
pinned Elle driver: exit 0
healthy checker verdicts: 6 true / exit 0
deliberate faults: 6 false / exit 1, each with nonempty witness
healthy coverage: 20 positive keys per seed, native_exception 48 each
healthy raw fail/info outcomes: 0 / 0
strict audit: valid=true, errors=[]
source files hashed before/after: 1709, unchanged
archived evidence: 67 files, all byte-matched to raw
standard executable build: exit 0
```

Elle uses pinned CLI9c2a18b and JAR e53120b9, seeds412/395/396, both healthy
models and all six faults, including WAIF reads and commit exclusion. Archive
SHA-256 is63bb705d055c35c6091b307e2dca6f5efd0611617486b0c8fc3a5b0761430d3c.
This is explicit isolation evidence, separate from ordinary Go/race tests.

Standard Barn has no Elle tag, uses default PGO and verified `./cmd/barn` main,
Go1.27.1 linux/amd64 CGO1. Binary SHA-256:
`3df7dae7687bae43201d8704ee64291186ddcd018e7fb5f21333da39498da5f0`.
Full1709input hashes were unchanged around build; source inventory SHA-256:
`1d6331757f7545615148a5be2fa74f373755e6608c7e979a16bac3a6bae90d84`.
Source attribution uses explicit manifests rather than invented embedded VCS.

Benchmark binaries are baselinea215ea3a and candidate212ab3e4. Exact original
CommandExecution records bind the057fa090 and5a1790b3 commits immediately to
successful `go test -c` commands, explicit Linux Git working tree, buildinfo and
binary hashes. The copied raw records hash to
`8d7da1de929c2dc3672fb6f2ce5a011b66974e8874afd7914d368a5c4c6e9210`.
The two original JSON records match, with LF/CRLF separator normalization.
There was no contemporaneous full-source inventory around these two benchmark
builds. Empty saved build stdout adds no provenance; no historical inventory was
retrofitted. This limitation is distinct from the stronger standard-build chain.

## Offline Mongoose follow-up

Exactly one cohort used the prior checkpoint, sound catalog, compatible
bootstrap, eight controllers, configuration and registry. Input DB SHA-256 is
`fab6957acffa1db8bc1c4b911fb53a8e8d6e605f4de2a31b9f8e4fa874c3200d`;
catalog SHA-256 is
`ba15182e25c60e9795c10f8f9479095ff4d22ea9bf2a03d7d061446cfca241e4`.
Magic1, seed1, warmup30s, deadline120s, GoCPU15s, isolated loopback,
diagnostics/debug enabled. No retry, control/quota change or timeout extension.

```text
controller exit: 1
domain: expected ordinary refusal was absent (zero_focus_lumos)
verified productive pairs: 0
preservation gates: 8 / 10
owned_shutdown: false
owned_final_checkpoint: false
shutdown: forced after 90s, exit -9
final checkpoint: unknown, verified=false
source after cohort: all1709files unchanged, owned PID absent
```

The previous publication-optimization cohort had the same refusal-control
failure, but all ten preservation gates passed and shutdown/checkpoint were
verified. This new preservation failure blocks promotion independently of that
preexisting domain limitation. `finalization_errors=[]` does not override the
explicit failed gates. The original profile-validation flag remains false;
independent profile reads are separate evidence and never rewrite manifests.

The controller's unchanged stop path sent verified-pidfd SIGTERM, waited90s,
then sent SIGKILL after TimeoutExpired. Thus -9 is the controller's forced kill,
not evidence of OOM. Raw fixed phases show shutdown initiation and entry at
01:16:33 on October18; no shutdown-complete marker was retained. Temporary
checkpoint218,147,345B and sidecar1,420,244B remain unpublished; no `.new` pair
exists. Their retained bytes are not a verified checkpoint and were not parsed
or repaired. Why the in-process shutdown failed to finish remains unknown.

The native typed terminal census independently reparsed135records with no
malformed/invalid rows:132E_INVARG/unknown,2E_INVIND/unknown and one
E_MAXREC/seconds_budget. The latter is task1788854219, root#277800:weave,
committed=true, intrinsic=false. Prior run had62records:60/1/1 in those groups.
The formatted saved exception includes #59:_fix_preps through full_prep/my_huh/
do_huh/weave; that saved leaf is distinct from a native CPU sample's root label.
No explicit action-to-task causal join is available.

Independent Go profile readers validated all eleven retained base views and
three task-focused views with exit0, expected types and nonzero samples. The
whole-process profile contains2,418samples/24.18sampled CPU seconds in its15s
HTTP window. Task1788854219 has9.96sampled seconds; another weave-root task
contributes0.02s. Root labels identify the task entry, not a sampled MOO leaf.

For task1788854219, cloneVerbForReadTxn has2.54s cumulative/0.56s flat CPU;
its source-copy line has no attributed samples. The fresh-node line has1.65s
cumulative and the names-copy line0.86s. AdoptLiveVerbs is4.34s cumulative and
builtinSetVerbArgs6.33s cumulative; these nested values overlap and must not be
added. The previous budget-terminated weave task had4.25s clone cumulative,
including2.36s on source copying. These different, budget-clamped task samples
show the targeted line disappearing from observed samples; they do not measure
completed-work speedup or imply the remaining cost is exclusively one command.
Borrowed source still requires defensive copying even when no sample lands there.

Descriptive, failed-cohort process observations: action window10.128406084s,
all-Barn-thread CPU15.65s, RSS2,803,620 to3,883,608KiB, peak3,884,728KiB across
103samples. Natural HeapAlloc2,662,658,512 to1,891,596,472B; TotalAlloc
5,578,506,824 to13,005,862,336B is cumulative including loading. These capture
different background work and GC state; they are not command-exclusive CPU,
completed gameplay throughput or a comparative memory result. Actor, room, tutor
and wand cleanup each returned1; the private account remains by documented policy.

Preservation DTO SHA-256:
`1f8f815dcd2762137d85b165d04fbfbb6b3eecb057ad05ed608e813850ef63ae`.
Original final manifest SHA-256:
`1643dc19e20fc20d86c5f968135677ea1a4dca997bb83b7fb5478585196de960`.
Original manifests, partial checkpoint and failed profile cohort are preserved.

Next work is to explain the incomplete shutdown and seconds-budget failure
using retained phase/task/profile evidence before a separately documented new
experiment. No code-causality finding, integration, push or deployment is claimed.

The sidecar's existence also narrows the recorded checkpoint progress: source
creates it only after database serialization, Sync, Close and database hashing
succeed. The process therefore reached identity-sidecar handling. Exact stack
at termination, sidecar completion and the cause of the deadline expiry remain
unknown; no temporary checkpoint parser or repair was run.

Machine-readable results: `experiments/verb-source-sharing-20261018-results.json`.

Raw component evidence: `.tmp/verb-source-experiment-20261018-01`.
Independent audits: `.tmp/verb-source-review` and `.tmp/verb-source-final-review`.
Correctness/build evidence: `.tmp/verb-source-verification`.
Actual cohort: gameplay worktree `.tmp/barn-magic-verb-source-20261018-01`.
