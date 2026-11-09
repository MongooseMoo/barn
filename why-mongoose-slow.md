# Why Mongoose feels slow

Historical raw-artifact links in this record refer to the recording workspace's
ignored `.tmp` directories and sibling worktrees. They are local evidence,
not files supplied by a fresh checkout. The
[tracked acceptance summary](docs/reports/mongoose-gameplay-acceptance-20261013.md)
and [runner guide](docs/mongoose-gameplay-runner.md) provide portable provenance
and reproduction commands. Database snapshots and private raw logs stay outside
the source commit.

Evidence collected from `mongoose@mongoose.world` on October 12, 2026 Mountain time (October 13 UTC).

**Yes: the live MOO's main execution thread was using essentially an entire CPU core during both measured windows, and subsequent passive observations caught ship-cycle electronics/cache invalidation and test cleanup executing.** Player commands and background MOO tasks share that thread. The strongest current explanation is that work on this serial execution path leaves too little capacity for prompt player responses. These are concrete candidate paths; **their CPU shares and contribution to reported player sluggishness have not yet been measured.**

## What we actually measured

The target was the running production process, not an offline reproduction:

```text
PID: 1996378
executable: /home/mongoose/toaststunt-d80a17c/build/moo
executable SHA-256:
1d9864c792b9ce3ba92f23f24b53c8c6985cd36ad1bb928f4aa8be121f322c2c
logical CPUs: 48
```

Two saved process captures independently showed main-thread saturation. Percentages use one logical CPU as 100%:

```text
capture 02: mean_main_cpu_percent = 100.01088228468899
            main_cpu_intervals   = 8
capture 03: mean_main_cpu_percent = 99.87273458114642
            main_cpu_intervals   = 3
            running_moo_verb     = "unknown"
```

The earlier eight-second `pidstat` observation also showed approximately 100% on the main `moo` thread, with its two native worker threads mostly asleep. Earlier live `vmstat` interval samples showed 83–84% host CPU idle, zero I/O wait, and zero swap-in/swap-out. These are observations from short windows, not an all-day utilization history.

This matters because idle cores do not automatically increase the capacity of ToastStunt's main MOO execution thread. A costly verb, builtin, interpreter operation, housekeeping routine, or busy loop on that thread can delay commands even while the machine has spare CPU capacity. The CPU readings alone cannot distinguish those possibilities.

## What was competing for execution

One bounded, read-only MOO metadata query returned:

```text
queued_count                    = 109
scheduled_count                 = 1386
scheduled_examined              = 500
scheduled_inspection_limited    = 1
scheduled_overdue               = 2
oldest_scheduled_lag_s          = 1.37873077392578

queued groups:
  #1007:s_run                   = 53
  #3842::create_temp_player     = 20
  #6038:_acquire_suite          = 19
  #1:loc_enter loc_exit         = 3
```

There is scheduler work alongside temporary-player and suite-acquisition activity. The displayed scheduled records also include recurring `pulse` and `dispatch_event` calls. That makes background scheduling and test/operations activity reasonable candidates to investigate alongside gameplay.

**These counts are not CPU attribution.** Queued tasks can be suspended, reading, or waiting for a future time; 109 queued tasks does not mean 109 runnable tasks. The overdue count and oldest lag cover only the first 500 of 1,386 scheduled entries. They are a partial snapshot, not a complete backlog measurement or player-command latency distribution. We cannot yet say that `s_run`, tests, starship simulation, or magic is the culprit.

## Is memory or the whole server the problem?

The MOO process itself had approximately 675 MiB resident memory and no swapped pages in the saved capture:

```text
peak_main_rss_bytes = 707837952
VmRSS              = 691248 kB
VmSwap             = 0 kB
Threads            = 3
```

The much larger systemd `MemoryCurrent` value belongs to the entire service cgroup. That cgroup also contained `moodb`, multiple `codex`/`claude` processes, and Python/Node/uv processes. It must not be reported as MOO heap usage or evidence of a MOO memory leak.

Taken together, the sampled host idle capacity, lack of swap activity, and busy main thread favor a serial CPU bottleneck over machine-wide CPU exhaustion or swapping during those windows. They do not rule out intermittent host pressure, allocation/collection costs, or a separate network/client contribution to perceived lag.

## Active verbs subsequently observed

A source-derived, binary-pinned passive reader can now observe executing activations without a debugger attachment. Its first successful two-second capture accepted 3 of 40 observations and rejected 37 whose metadata changed or failed validation. One accepted call chain was:

```text
#1007:s_run -> #1007:s_call -> #4143:cycle
-> #4129:cycle (ship receiver #28747)
-> #4129:check_system_power -> #4129:refresh_electronics
-> #2200:refresh_electronics -> #13232:refresh
-> #13232:release -> #9655:invalidate
```

A later five-second capture limited to the top frame accepted 46 of 100 observations and rejected 54. Its accepted observations included:

```text
#9655:invalidate       28
#1584:bf_recycle       10
#6038:_acquire_suite    3
#6038:drain             2
#131166:pulse           1
#4160:recycle_test_ship  1
#2585:pick_value        1
```

This makes ship electronics/cache invalidation and test-fixture recycling concrete candidates rather than guesses based on queued verbs. **These counts are not CPU percentages.** Reads are non-atomic, rechecks cannot exclude every race, and accepted reads favor activations stable enough to observe. Missing observations could change the ranking. No current executing source line is claimed: the interpreter keeps its current PC in local variables.

## What is still missing

We now have best-effort executing MOO stack observations, but no live CPU-weighted semantic profile or native CPU stack profile. The queued-task view alone cannot reveal the executing verb. We also did not measure a player's command-to-response time, separate network RTT from server delay, or correlate a slow command with the busy thread.

The live host did not have `perf` installed at inspection time. No profiler installation, debugger attachment, core dump, restart, or live code change was performed. GDB attachment would initially stop the process, so it does not satisfy a requirement for uninterrupted observation. See the [GDB attachment documentation](https://sourceware.org/gdb/current/onlinedocs/gdb.html/Attach.html).

The isolated native CPU-clock profiler is now implemented at the deployed source revision, with coherent atomic stack/PC publication, retained Program generations, separate native-builtin self CPU, worker/fork exclusion, and complete lost-period accounting. The [profiler guide](../../src/toaststunt-cpu-profiler/docs/Features/cpu-profiler.md) describes capture and export. Passive accepted-read counts still cannot provide CPU shares. **No instrumented binary has been deployed, so live per-verb CPU shares remain unknown.**

Offline checks measured a known four-to-one MOO loop workload and passed:

```text
heavy=185 light=46 denominator=232
heavy_share=0.7974137931034483 light_share=0.19827586206896552
9 tests, 219 assertions, 0 failures, 0 errors
native CTest: 1/1 passed
actual-object worker/fork exclusion: PASS
```

Five paired offline measurements on the same instrumented binary found median active/disabled main-thread CPU ratios of 1.254x for interpreter loops (range 1.249–1.259x), 1.568x for tiny verb calls (1.545–1.614x), and 1.000x for synchronous native work (0.996–1.005x). The [SHA-bound raw summary](../../src/toaststunt-cpu-profiler/.tmp/vm-profiles-05/vm-overhead-95855-87ea78853d5d-summary.json) preserves all 30 phases and 15 complete profiles. These remain substantial observer costs, so the tool is an offline diagnostic prototype. They do not establish Mongoose overhead, stock-versus-disabled instrumentation cost, or maximum export memory/pause. Selective losses can hide whole verbs and make rankings incomplete. Native profiling, foreground command timing and scheduler lateness should accompany an eventual live semantic capture. Only after that evidence should we select a performance fix.

For Barn, the corresponding workload must exercise real flight and magic while recurring world tasks run, and measure foreground response latency, scheduler deadlines, CPU, and memory. A lightweight login/look/home loop cannot establish that Barn will remain responsive under this mix. Nothing collected here is a Barn speedup result.

## Actual copied Mongoose diagnostic captures

The new offline controller copies the SHA-256-pinned October 12 23:49 checkpoint,
sets up a private wizard account and runs a nonproduction MOO in a loopback-only network
namespace. Natural scheduled universe/electronics work runs without gameplay
fixture repairs. These captures describe instrumented, restored background work;
they do not establish the live server's verb ranking or ordinary gameplay latency.

The first detailed capture exhausted context metadata. An explicit definitions
mode retains Program generations, names, aliases, caller PCs and builtin identity,
while marking receiver/player/programmer/task attribution unavailable. It exposed
an existing decompiler short-circuit bug at a legal catch-expression opcode PC.
That failed cohort is preserved. A deterministic actual-compiled-Program test
reproduced the panic and verified a minimal pointer-consumption fix.

The subsequent copied-world export completed:

```text
cohort03 main CPU seconds       = 10.498786641
accounted periods              = 1050
drops / overruns / unobserved   = 0 / 0 / 0
late deliveries                = 0
unknown periods                = 479 (45.62%)
published profiler periods     = 455 (43.33%)
semantic coverage              = 4.48%
state slots                    = 8192 / 8192
frames                         = 1212
structural metadata budgeted   = 6716382 bytes
independent request-window CPU = 9.991234 seconds
RSS sampled                    = 596292..601240 KiB
universe cycle                 = 41115234 -> 41115239
owned shutdown exit            = 0
```

The native timer window extends through execution of the stop request; the
independent external window ends before sending that request. They have different
boundaries. Before/after universe snapshots also bracket control/export work.
Neither duration is a per-verb CPU measurement, and RSS is not a heap allocation
profile or evidence of a memory improvement.

That cohort still could not identify the dominant workload verb: the state cap fills with
unsampled execution paths, and observer metadata work dominates much of the
remaining capture. Its zero dictionary-rejection counter is misleading because
the state-cap branch failed to increment it. This led to the retention change
below: preserve every sampled identity, recycle only unsampled states under
coherent publication, and account for exhausted capacity. Unchanged-stack refresh work also needs to
fall before rankings approximate ordinary execution well.

The sampled-state retention change produces a substantially more complete
offline observation in cohort04. Sampled IDs remain immutable; unsampled states
can be reused within the same bounded dictionary. CPU shares retain the full
denominator, including metadata work and missing observations:

```text
cohort04 main CPU seconds     = 9.996166
accounted periods            = 1000
unknown periods              = 3 (0.30%)
published profiler periods   = 337 (33.70%)
transition periods           = 49 (4.90%)
#2585:pick_value self         = 162 (16.20%)
#9777::validate self          = 35 (3.50%)
recycle builtin self         = 57 (5.70%)
state retirements            = 92536
state capacity rejections    = 0
dictionary rejections        = 3375
structural metadata budgeted = 6974276 bytes
buffer drops / overruns      = 0 / 1
late deliveries              = 1
universe cycle               = 41115234 -> 41115238
owned shutdown exit          = 0
```

`pick_value` is the largest attributed self-CPU verb in this instrumented
restored-world interval. Its callers and the `recycle` builtin are useful next
investigation targets. They are not established live bottlenecks: this is one
ten-second offline cohort, and published profiler states account for roughly a
third of sampled CPU, excluding handler and export cost. At that point, repeated
unchanged-stack metadata work was the next tool target; later refinement and
catalog-backed captures are documented below. Inclusive verb
shares overlap; builtin self CPU remains separate from MOO self CPU.

The root-path view adds a material qualification: 160 of the 162 `pick_value`
self samples came through `#0:handle_uncaught_error` and message/sound delivery.
The full error-handler root accounts for 223 attributed periods (22.30% of the
full denominator). Of the picker self samples, 147 resolve to canonical line18:
`this.last_picks[key] = ref;`. That identifies the map update as a measured
operation to investigate, without establishing why that operation is expensive.

Read-only `moodb` inspection of the owned shutdown dump also found a retained
`#249.notify_mail_snd: cannot resolve 44028` error. The initial controller gives
the restored world empty owned resource directories. Its sound resolver opens
`sound.sqlite`, so missing catalog data can redirect work into error handling.
The retained history can include preexisting errors; it does not establish an
arrival count or prove which errors caused these sampled calls. Future captures
need the pinned real catalog and a bounded before/after error census. Optimizing
an offline error storm would not demonstrate ordinary gameplay responsiveness.

The raw profile SHA-256 is
`69521bead2825256cbb981c11b71cd0881177820266eef5cdb52f4bfaf32c89e`.
The source checkpoint hash remained unchanged. Start and stop/export control
round trips were 4.100376ms and 25.009330ms; those wall durations include queueing,
wrappers, serialization and FileIO, and are not verb CPU measurements.

Cohort05 supplied the pinned real sound catalog through its normal file path:
229142 Sounds rows, SQLite quick_check `ok`, source and copy hashes equal.
It used the same frozen retention executable and added bounded error-history
and sound-map-size observations. Its full-denominator results are:

```text
cohort05 main CPU seconds        = 10.070083
accounted / unknown periods     = 1007 / 0
drops / overruns / unobserved    = 0 / 0 / 0
late deliveries                 = 0
metadata/control                = 450 (44.69%)
#0:handle_uncaught_error root    = 2 (0.20%)
#1007:s_run root generations     = 212 + 56 (21.05% + 5.56%)
#9655:start_rearm root           = 117 (11.62%)
#9777::validate self             = 52 (5.16%)
#313:atmosphere_owner self       = 27 (2.68%)
recycle builtin self            = 58 (5.76%)
#2585:pick_value self            = 11 (1.09%)
sound history keys before/after = 18023 / 18023
state retirements               = 113309
capacity / dictionary rejects   = 0 / 0
owned shutdown exit             = 0
```

These are separate instrumented restored-world intervals, not a paired speedup
comparison. Matching the catalog makes the ordinary scheduler/physics paths
much more useful to investigate than the earlier error-heavy ranking. The
retained error history remains bounded at 15, with one cannot-resolve record
before and after; that record can predate the run. The source checkpoint and
catalog hashes remained unchanged. The raw profile SHA-256 is
`ab8654be18264174ec9d7919a1b79ae2734369aab0f3c4a9e6843c6c23a09038`.

This interval still includes startup physics rearming, inherited test tasks and
scheduled catch-up: overdue entries among the inspected 500 changed 301 to 0.
An explicit uncaptured warmup and real flight/magic actions are needed for a
representative gameplay cohort. Published profiler states account for 44.69% of
sampled CPU, excluding handler and export. Reducing total cost requires independent equal-work measurements because
a fast publication guard can move observer work into the MOO sampling category.

- [Detailed cohort01 raw profile](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-01/files/natural-background-cpu-profile.json).
- [Failed definitions cohort02 manifest](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-02/manifest-final.json).
- [Definitions cohort03 CPU report](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-03/natural-background-cpu-report.md).
- [Cohort03 source/process/shutdown manifest](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-03/manifest-final.json).
- [Retention cohort04 CPU report](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-04/natural-background-cpu-report.md).
- [Cohort04 root-path and source-line report](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-04/root-line-report-v2.md).
- [Cohort04 raw profile](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-04/files/natural-background-cpu-profile.json).
- [Cohort04 source/process/shutdown manifest](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-04/manifest-final.json).
- [Catalog-backed cohort05 CPU report](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-05/natural-background-cpu-report.md).
- [Cohort05 source/catalog/process/shutdown manifest](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-05/manifest-final.json).
- [Offline controller and scope guide](../barn-gameplay-workloads/docs/offline-mongoose-cpu-profiling.md).

## Warmed, catalog-backed cohort06 and next measurements

The accepted sampler candidate adds sampled-state retention and a guarded fast
path for unchanged execution identity. Independent fixed-work measurements
support local tooling integration; they do not establish a Mongoose speedup.
Cohort06 uses that binary and the real catalog, with 30s uncaptured warmup:

```text
native main CPU seconds         = 11.313072
accounted / unknown periods     = 1132 / 0
drops / overruns / unobserved    = 0 / 0 / 0
metadata/control                = 506 (44.70%)
#9655:start_rearm root           = 211 (18.64%)
#10566:_fork root                = 187 (16.52%)
#1007:s_run root                 = 134 (11.84%)
#9777::validate self             = 123 (10.87%)
#1584:bf_respond_to self          = 47 (4.15%)
#313:atmosphere_owner self       = 42 (3.71%)
state retirements               = 100171
capacity / dictionary rejects   = 0 / 0
start / stop-export RPC ms       = 2.265 / 1329.275
native export ms                = 9.934
owned shutdown exit             = 0
```

The timer continues until the stop request executes. External request-window
CPU is 10.002905s over 10.002764s wall, so these control boundaries are different
from the native timer interval. The 1.329s stop/export round trip is a concrete
latency target, without assigning it to one verb. During warmup, inspected
overdue entries fall 301 to 21; after capture they are 2, while queued work rises
190 to 292. Rearming remains prominent, so 30s does not establish steady state.

Capture/control RSS observations range 594680..600768 KiB, process high-water
604412 KiB. Sound history remains 18023 keys; bounded retained errors remain 15
with one cannot-resolve record. These are neither allocation attribution nor
error-arrival counts. Source checkpoint/catalog hashes remain unchanged.
Raw SHA-256 `ba75fad8092b10b10541c02f22b9c66ed4250041319b85053471ebfdc73be6fc`.

The metadata/control fraction counts published profiler states; it excludes
sampler-handler and export cost. Total observer overhead needs a paired
benchmark. Fast-guard work can also be sampled in the MOO category, so a lower
metadata fraction alone would not establish a cheaper observer.

The tool would improve most from lower metadata publication cost, less intrusive
stop/export, and repeated foreground flight/magic cohorts. Those cohorts should
count completed actions and measure response latency, unprofiled main-thread
CPU and process/heap memory separately. Prioritize validation, atmosphere/ship
physics and rearming as source-guided hypotheses; prove each against equal work
before changing either Mongoose or Barn. The catalog correction also shows why
matching real dependencies must precede interpretation of an offline ranking.

- [Warmed cohort06 report](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-06/natural-background-cpu-report.md).
- [Cohort06 manifest](../barn-gameplay-workloads/.tmp/offline-mongoose-cpu-06/manifest-final.json).
- [Accepted sampler fixed-work evidence](../../src/toaststunt-profiler-retention-refresh-order-20261013/experiments/2026-10-13-retention-refresh-order.md).

## Saved live evidence and collection limits

- [Successful process and MOO metadata capture](../barn-gameplay-workloads/.tmp/live-mongoose-20261013-03.json): process samples around 00:34:35–00:34:38 UTC; bounded queue/scheduler snapshot afterward.
- [Earlier process capture](../barn-gameplay-workloads/.tmp/live-mongoose-20261013-02.json): valid CPU samples; its MOO metadata query failed compilation and is excluded from queue conclusions.
- [Initial failed capture](../barn-gameplay-workloads/.tmp/live-mongoose-20261013-01.json): local SSH executable discovery failed; contains no live measurements.
- [Collector source](../barn-gameplay-workloads/scripts/capture_live_mongoose.py): procfs/service observations and an optional authenticated read-only metadata query. The query adds a small amount of normal request work; its overhead has not been quantified.
- [First accepted executing stack](../barn-gameplay-workloads/.tmp/live-moo-verbs-20261013-02.json): 3 accepted / 40 attempted; full ship-cycle/electronics path.
- [Top-frame capture](../barn-gameplay-workloads/.tmp/live-moo-verbs-20261013-03.json): 46 accepted / 100 attempted; includes invalidation and recycling.
- [Passive reader source](../barn-gameplay-workloads/scripts/sample_live_moo.py): read-only process memory, compiled layout helper in a temporary directory, explicit binary/source pins, and retained rejection counts. Temporary helper compilation adds host work; reader overhead is separate from target CPU and has not been quantified as a latency effect.

After the successful capture, the service still reported:

```text
MainPID=1996378
NRestarts=0
ActiveState=active
SubState=running
```

**Current answer:** Mongoose's main execution thread was saturated while much of the host was idle. We caught real ship-cycle electronics/cache invalidation and test cleanup on that thread. These are now concrete investigation targets, but their CPU shares and causal effect on player-visible delay remain unknown.

## Real gameplay and CPU evidence, October 13

The [one-command offline runner](../barn-gameplay-workloads/docs/mongoose-gameplay-runner.md)
now completes actual connected ordinary-player flight and catalogue Lumos/Nox.
[Both01](../barn-gameplay-workloads/.tmp/gameplay-both-01/manifest-final.json)
completed one flight sequence and one light/extinguish pair, including one
natural fizzle, with both diagnostic profilers off. Natural turn, displacement,
speed, fuel, braking and cutoff are checked; casting checks native formation,
single payment, refill/recovery, wand emission and actual room illumination.
The owned world stopped with exit 0 and all five input/executable/controller/
catalog/shutdown checks true. Failed cohorts remain available.

[Diagnostic02](../barn-gameplay-workloads/.tmp/gameplay-diagnostic-02/manifest-final.json)
also completed both sequences with valid MOO CPU sampling and required native
main-thread sampling. Its final status was complete, shutdown exit 0, five final
checks true, and finalization_errors empty. These are instrumented offline
observations, including background tasks and controller observations; live verb
CPU shares and causal player-visible latency remain unknown.

| Diagnostic | Accounted MOO periods | Unknown or missing | Metadata/control | Native user CPU samples |
|---|---:|---:|---:|---:|
| Flight | 747 | 0/747 | 21.55% | 588 |
| Magic | 664 | 1/664 | 19.43% | 510 |

In the [flight MOO report](../barn-gameplay-workloads/.tmp/gameplay-diagnostic-02/flight-cpu-report.md),
`#9777::validate` had 25 self periods/3.35% and `#9655:invalidate` had 18/2.41%.
In the [magic MOO report](../barn-gameplay-workloads/.tmp/gameplay-diagnostic-02/magic-cpu-report.md),
`#9655:invalidate` had 21/3.16%; its inclusive share was 23.64%, while
`#9655:rearm` was 29.97% inclusive and `#2200:settle_damage` 29.82% inclusive.
The biggest builtin self shares were `recycle` (18.21% flight, 15.36% magic)
and `mapkeys` (12.85% flight, 20.03% magic).
Inclusive paths overlap and cannot be summed. The metadata/control percentages
describe observed categories; they do not measure added profiler overhead
against an uninstrumented baseline.

Both01's action windows were flight 3.180644918s wall/3.182204075s main CPU and
magic 3.082636184s wall/3.077598100s main CPU. Its whole-process high-water RSS
was 604636KiB. Diagnostic02 had 605408KiB high-water RSS after magic. These are
single cohorts with different natural attempt counts and instrumentation;
neither engine speedup nor memory reduction follows from their differences.
RSS does not identify retained objects or allocation causes.

The native captures retain perf.data, sample periods, DSOs and raw reports,
with 0 reported lost samples. This perf build lacks ELF/unwind support, so its
raw reports show addresses and flat samples. Matching saved mappings/build IDs
against the SHA-pinned binary permits separate offline symbol resolution;
shared-library and unresolved weights must remain visible. No native call
graph or exact source-line coverage is established.

The [offline symbolizer](../barn-gameplay-workloads/scripts/symbolize_mongoose_native.py)
now verifies both captured build ID and executable SHA, converts saved mapping
offsets through ELF load segments, and requires finite symbol bounds. It
resolved 476/588 flight and 413/510 magic samples; all unresolved weights remain
in the denominator, including shared-library samples. Exact source lines remain
unavailable. Saved [flight](../barn-gameplay-workloads/.tmp/native-symbol-flight-20261013-02/symbols.json)
and [magic](../barn-gameplay-workloads/.tmp/native-symbol-magic-20261013-02/symbols.json)
reports identify these native self-IP estimates:

| Native function or identified subset | Flight | Magic |
|---|---:|---:|
| `db_fixup_owners(long)` | 105/588, 17.86% | 102/510, 20.00% |
| `moo_profile::refresh_stack(...)` | 70/588, 11.90% | 60/510, 11.76% |
| All identified profiler-specific symbols | 115/588, 19.56% | 105/510, 20.59% |

[Ownership repair source](../../src/toaststunt-cpu-profiler/src/db_objects.cc:1425)
visits all persistent object slots and anonymous objects, and
[safe recycling calls it](../../src/toaststunt-cpu-profiler/src/objects.cc:926).
This gives a source-backed native investigation target consistent with the
MOO `recycle` observations. Foreground and background work remain mixed;
owned fixture cleanup runs after collection. The profiler-specific subset
identifies real instrumented-run work, not total overhead or an off/on penalty.
Native and MOO capture boundaries differ, so their percentages are not combined.

Next performance work should use equal completed outcomes and repeated
sampler-off cohorts, separately account for natural fizzles and scheduled work,
validate remaining offline service dependencies, and compare Barn on the same
domain checks. Definition validation, invalidation/rearming and atmosphere/
electronics remain measured investigation targets. They are not yet proven
causes of live command delay or accepted optimization changes.
