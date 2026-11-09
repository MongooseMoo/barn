# Issue 279 memory evidence

## Scope

Complete the remaining evidence for the existing property and load-string sharing,
and assess whether runtime short-string interning is justified. The three original
implementation slices are present in current source. No new intern pool is assumed.

## Original checkpoint

165,578,514 bytes; SHA-256
`12286fda9b50bffa826186e518f1ba14781bedbbfae7eae35197e9d8ebfc0d56`.
The source is read only. All measurements below use Go 1.27.1.

## Current Windows load, 2026-11-09

`barn -memory-report` on the original checkpoint, after the report's two forced GCs:

```text
objects:          32667
property_slots:   4956720
clear_slots:      4191470
private_slots:    765250
heap_alloc:       399474800
heap_objects:     3486565
heap_inuse:       511688704
sys:              897322232
total_alloc:      974329784
gc_cycles:        6
```

A separate `barn -eval 'memory_usage()'`, exit 0:

```text
=> {0.0, 557805568.0, 0.0, 0.0, 0.0}
```

On Windows the second element is working-set bytes
(`builtins/resource_stats_windows.go`). This observation is below 600,000,000 bytes.
It is an offline eval process, not the steady-state resident size of a busy server.

## Current Linux/WSL load, 2026-11-09

Same checkpoint and source in Debian WSL, linux/amd64. Offline report, exit 0:

```text
objects:          32667
property_slots:   4956720
clear_slots:      4191470
private_slots:    765250
heap_alloc:       399455992
heap_objects:     3486364
heap_inuse:       513114112
sys:              905891288
total_alloc:      974256200
gc_cycles:        6
```

Separate offline eval, exit 0:

```text
=> {1217311.0, 132473.0, 3758.0, 2292.0, 351724.0}
```

Linux returns `/proc/self/statm` page counts (`builtins/resource_stats_linux.go`),
not bytes. `getconf PAGESIZE` returned `4096`; the second element is therefore
542,609,408 resident bytes, below 600,000,000 bytes in this offline observation.

## Workload experiment

PR experiment [42594936494](https://github.com/MongooseMoo/barn/actions/runs/42594936494)
used the existing `TestMongooseRealWorkload` harness, three independent
uninstrumented 16-player processes plus a separate diagnostic process. All used
GOGC=400, GOMAXPROCS=32, five seconds warmup, thirty seconds measure,
the original checkpoint and the real sound catalog (481,214,464 bytes, SHA-256
`250b9b69fae2dad6788061a1abb029332f564b92e630514c93181030f7277ca9`). Preserve process exits,
terminal inventories, failures, GC traces and profiles. Failed/unsettled cohorts
do not establish valid application throughput. Instrumented timing is separate.

The experiment failed. The temporary issue-specific workflow was removed after
collection as planned; its failure is retained here, not reported as a passing
performance check. Raw evidence remains private beneath
`C:/Users/Q/code/barn/.tmp/issue-279-ci/42594936494-1/`: manifest.txt, engine.test,
sample-1.txt, sample-2.txt, sample-3.txt, sample-diagnostic.txt, runtime.cpu,
runtime.heap and logs/. The manifest records the PR merge source, executable and
input hashes, toolchain and environment. The test executable SHA-256 was
`e4f261c9347448fdfce43c1d59b2edd001e6e20691873837b68170ca83803a25`.

| Process | Submitted | Completed | Failed | Unsettled | Process result |
| --- | ---: | ---: | ---: | ---: | --- |
| 1 | No measurement | No measurement | No measurement | No measurement | FAIL: native read ENOMEM while copying sound catalog |
| 2 | 7392 | 7386 | 6 | 1 | FAIL |
| 3 | 8029 | 8024 | 5 | 0 | PASS, with native command failures |
| Diagnostic | 768 | 762 | 6 | 1 | FAIL |

Raw summaries:

```text
sample 2: cohort submitted=7392 completed=7386 failed=6 unsettled_at_return=1 elapsed=30.104831984s drain=104.831984ms
sample 2: players=16 goodput=245/s failed=6 uncaught=5 abort=21.64%
sample 3: cohort submitted=8029 completed=8024 failed=5 unsettled_at_return=0 elapsed=30.156641015s drain=156.641015ms
sample 3: players=16 goodput=266/s failed=5 uncaught=5 abort=22.51%
diagnostic: cohort submitted=768 completed=762 failed=6 unsettled_at_return=1 elapsed=32.090198076s drain=2.090198076s
diagnostic: players=16 goodput=24/s failed=6 uncaught=7 abort=14.16%
diagnostic: gc: cycles=2 pause_total=1.520834ms gc_cpu=2.25s busy_cpu=73.21s gc_share=3.1% alloc_rate=167MiB/s
```

Native errors include player 71's `home` reaching E_NACC through
`#9501:move_as_owner`. Unsettled samples retain task/readiness/admission/stack
evidence. No permission or scheduling change was inferred from these outputs.
The first file-read failure is a native setup failure of unknown underlying
cause; later processes copied the same catalog. Memory/pressure/mount/kernel
probes and primary Go/WSL documentation were inspected. No same-day kernel OOM
match was found, and no retry or system restart was introduced.

These samples do not establish the original no-regression goodput requirement.
There is no valid before/after application comparison. The heavily instrumented
diagnostic process is not comparable to the uninstrumented timings.

## GC evidence and resident-unit correction

One intact raw diagnostic trace row, retained as a diagnostic observation only:

```text
gc 10 @63.816s 0%: 0.37+42+0.071 ms clock, 12+0/323/791+2.3 ms cpu, 2710->2711->618 MB, 2811 MB goal, 0 MB stacks, 0 MB globals, 32 P
```

Concurrent mark wall time and CPU work are different quantities; the two-cycle
aggregate is not a per-cycle CPU saving. Debug output interleaved some GC lines,
so the trace is not a complete clean series for comparison.

The diagnostic logged `resident=0` MiB after load because `readMemSample` treated
Linux page counts as bytes. This PR converts with the actual page size while
preserving Windows byte units. The captured 132473.0 reading produced this
regression failure before the fix:

```text
resident bytes = 132473, want 542609408
```

The test includes 4 KiB and 16 KiB Linux pages and the raw Windows byte reading.
It passes after the correction. This changes test-harness reporting, not the
MOO `memory_usage()` result. No corrected under-load RSS sample is claimed.

## Runtime string decision

The diagnostic heap profile includes startup and loading in allocation totals;
it is useful for identifying producers, not for attributing the whole total to
the measured window. Selected sampled `alloc_objects` rows:

```text
types.NewStr:                        10117708
NewStr through builtinRespondTo:      5366086
NewStr through Database.loadedStr:     434196
```

The retained profile attributes 29.50 MB to NewStr overall; it does not identify
all those bytes as duplicate short strings. Normal VM calls already reuse a
name Value supplied by the program (`vm/op_verb.go:64,269`).

Tested a producer-local alternative: return the input name Value when a single
canonical alias exactly matches it. On Windows, three 300 ms samples of the
authored benchmark showed:

| Exact-match call | ns/op samples | B/op | allocs/op |
| --- | --- | ---: | ---: |
| Existing implementation | 514.7, 539.3, 531.4 | 160 | 3 |
| Direct-input-sharing prototype | 383.8, 383.1, 400.2 | 96 | 2 |

Those are small local microbenchmarks, not workload speed measurements.
Other paths preserved canonical case, wildcard spelling and all aliases.

The prototype was rejected: a four-byte name sliced from a 16 MiB string can
keep that backing allocation alive through a long-lived response. VM string
slicing is a real producer (`vm/op_index.go:354`). The retention regression drops
the finished lookup transaction and its memo before measuring the response:

```text
existing implementation: retained bytes with only the response alive: -48744
prototype:               retained bytes with only the response alive: 16736328
prototype:               FAIL: short canonical response retained the large input backing
```

Negative small deltas reflect unrelated fixture garbage reclaimed by GC, not
negative object sizes. An initial probe mistakenly retained the lookup memo and
therefore pinned the input under both builds; that probe was corrected before
the comparison above. The retained regression permits 1 MiB of measurement
noise and distinguishes the 16 MiB retention. The prototype is absent from the
final implementation.

A global bounded intern would additionally need owned key bytes, a demonstrated
working set, capacity/eviction and concurrency measurements. No such pool is
selected. The optional runtime-intern consideration is resolved by retaining
the existing code and recording the demonstrated tradeoff, without claiming
that every possible intern design is unhelpful.

## Acceptance limits

The original file's offline resident target is met in the recorded Windows and
Linux observations. Existing load-time sharing and compact slots remain in place;
the loader test now checks actual representation reuse, rather than content
equality alone. The full-mix before/after goodput and per-cycle GC improvement
requirements remain unproved. This PR does not close issue 279 or claim that
those requirements passed.
