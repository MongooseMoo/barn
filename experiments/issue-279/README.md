# Issue 279 memory evidence

## Scope

Complete the remaining evidence for the existing property and load-string sharing,
and assess whether runtime short-string interning is justified. The three original
implementation slices are present in current source. The retained runtime change
is a producer-specific bounded owning intern. The historical failed experiment
below is preserved separately from the completed valid comparison.

## Repaired control and owning-intern candidate

The captured checkpoint has `#71.home == #62`, whose inherited `acceptable` verb
returns literal zero. Managed stock WSL Toast confirms that even an object's
nonwizard owner gets E_NACC when the destination's accept returns zero. The
existing explicit workload repair now assigns the designated accepting
`$player_start` only for this captured bad-home condition. Repair readiness is
fatal on failure. Command mix, 16 players, 5 s warmup, 30 s measure, GOGC=400,
GOMAXPROCS=32 and the existing 10 s command deadline remain unchanged.

Input copies are staged on native ext4 and checksum-verified before the workload
process starts. Staging failures remain fatal; no retry was added. Workload tests
now fail on failed commands as well as unsettled commands. This closes the old
case where a test process passed despite an invalid cohort.

[Repaired-control CI](https://github.com/MongooseMoo/barn/actions/runs/42661518475)
completed successfully. Private evidence is under
`C:/Users/Q/code/barn/.tmp/issue-279-ci/42661518475-1/`.

| Control process | Submitted | Completed | Failed | Unsettled | Goodput/s |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1 | 9395 | 9395 | 0 | 0 | 312 |
| 2 | 10060 | 10060 | 0 | 0 | 334 |
| 3 | 6636 | 6636 | 0 | 0 | 220 |
| Diagnostic | 8499 | 8499 | 0 | 0 | 277 |

The diagnostic process is separate from the uninstrumented controls. It retains
clean gctrace, CPU and heap profiles. Three control observations alone do not
establish a candidate speedup. The paired comparison alternates control/candidate
order across six independent pairs and collects separate diagnostic processes.
The saved control executable SHA-256 is
`4d550c64ad13f4cb0c710723bcbc0e898362c88fe9b66fe6d750177b52130c59`.

The input census in [verb-name-census.json](verb-name-census.json) finds 18,451
definitions / 13,202 distinct canonical names; 18,381 definitions / 13,142
distinct names are at most 64 bytes. The candidate calls `types.InternStr` only
for `respond_to`'s canonical result. Its 16,384 fixed slots are partitioned among
64 mutex-protected shards. Misses replace one slot with a representation owning
cloned bytes; collisions never mutate returned Values. Empty/long strings use
NewStr. The maximum owned representations/text occupy approximately 2 MiB plus
128 KiB of slots/mutexes, independent of the unique-input count. This is a cache
budget, not a total-process memory prediction.

Focused tests cover byte identity, case, invalid UTF-8, Unicode character counts,
append aliases, large backing ownership, eviction, concurrent access, bounded
retention and zero-allocation warm hits. The race test passed. The canonical-name
regression failed before implementation on all four cases. The owning candidate's
four-byte response retention delta was -49,304 bytes, with no 16 MiB backing
retained. Negative deltas mean unrelated fixture garbage was reclaimed.

Fresh offline control/candidate observations (Go 1.27.1, original input):

| Metric | Control | Candidate |
| --- | ---: | ---: |
| Windows heap_alloc, bytes | 399468400 | 399458800 |
| Windows resident, bytes | 559501312 | 560046080 |
| Linux heap_alloc, bytes | 399449536 | 399450400 |
| Linux resident, 4096-byte pages | 133641 | 137994 |
| Linux resident, bytes | 547393536 | 565223424 |

These separate offline eval processes are below the 600,000,000-byte resident
target; they do not measure busy-server residency. Linux control and candidate
`-dump` commands both completed their built-in reload/store comparison. `cmp`
returned zero, and both output dumps have SHA-256
`544a3bcd3af50ca68fb02248cff19e5281aafc0ea27c94de9dc1e437e48400b3`.
Private cold profiles/dumps remain under `/root/.cache/barn279-cold` in Debian WSL.

## Completed paired comparison

[Comparison CI](https://github.com/MongooseMoo/barn/actions/runs/42665898830)
completed successfully. Private raw evidence is under
`C:/Users/Q/code/barn/.tmp/issue-279-ci/42665898830-1/`: manifest, both exact
executables, original inputs, twelve uninstrumented logs, two diagnostic logs and
both CPU/heap profiles. All fourteen processes passed with zero failed commands
and zero unsettled commands. Native background exception output remains in the
raw logs; this statement concerns the measured command cohorts.

Control source is `478ab9bfea1e9bb4970290995527693ede9411bd` (tree identical to
the repaired pre-intern branch). Candidate source is
`fe3db676e7673b2e4e42bf9d94d8080f0ebb7b4f`; its executable SHA-256 is
`ecdaa1146216824cdae282d21655dea76c3f16b22bd4d513afbe7db4800620f6`.
The candidate was built from the branch rather than the moving synthetic merge
to exclude unrelated master changes from this comparison. Ordinary correctness
CI tested the synthetic merge separately. The temporary experiment job and its
checkout override are removed from the final workflow.

Odd pairs ran control then candidate; even pairs reversed that order. Each table
entry is `completed / goodput per second / allocations per completed command`.
Every submitted command completed successfully; there are no excluded samples.

| Pair | Control | Candidate |
| --- | --- | --- |
| 1 | 7413 / 246 / 27681 | 8800 / 293 / 26310 |
| 2 | 9599 / 319 / 25742 | 8974 / 298 / 25971 |
| 3 | 10776 / 357 / 25203 | 10565 / 350 / 25858 |
| 4 | 7728 / 256 / 25593 | 8230 / 273 / 24867 |
| 5 | 5634 / 184 / 30448 | 7272 / 224 / 26359 |
| 6 | 8754 / 291 / 26169 | 7964 / 264 / 26649 |

Goodput includes the actual measured interval and drain, not just the requested
30 seconds. Mean of the six exact rates is 275.59/s control and 283.50/s candidate
(+2.87%). Candidate wins three pairs and loses three; paired ratios range from
0.908 to 1.218. This does not establish a general throughput speedup. There is no
observed aggregate goodput regression in this set.

Using completed-command counts as weights, the rounded harness allocation
figures give approximately 26,497 versus 25,989 allocations/command (-1.92%) and
2,778,016 versus 2,733,553 bytes/command (-1.60%). Allocation medians are 25,955.5
versus 26,140.5, so the whole-program allocation results are also noisy. These
aggregate figures include background work during the window.

The separate allocation profiles directly verify the targeted producer:

```text
control builtinRespondTo:   cum alloc_objects 17406878
  child types.NewStr:                        5636439
candidate builtinRespondTo: cum alloc_objects 10938760
  child types.NewStr:                        not reported
```

Profiles include startup/loading and are sampled; their raw totals are not
window-only counts or a normalized application-speed comparison. The focused
warm canonical-name benchmark independently changes 160 B/3 allocations to
96 B/2 allocations (aliases: 176 B/4 to 112 B/3). Retain the owning intern for
this verified allocation saving with a fixed retention budget and no observed
aggregate goodput regression. Do not claim an overall GC or latency improvement.

## Separate GC and busy-memory diagnostics

Both diagnostic cohorts completed: control 9,943 and candidate 9,004 commands,
zero failed/unsettled. These instrumented timings are not pooled with the twelve
uninstrumented observations.

| Measured-window diagnostic | Control | Candidate |
| --- | ---: | ---: |
| GC cycles | 7 | 7 |
| GC CPU seconds | 13.16 | 13.56 |
| GC share of busy CPU | 5.0% | 5.1% |
| Total stop-the-world pause | 7.308389 ms | 6.240650 ms |
| Post-window forced-GC heap_alloc | 611 MiB | 933 MiB |
| Post-window forced-GC resident | 5393 MiB | 4125 MiB |

Unforced concurrent-mark wall times after server startup through warmup and
measurement, in trace order (milliseconds):

```text
control:   38 87 171 65 82 85 119 146 84 55   median 84.5
candidate: 48 103 203 78 103 138 97 60 94 93 median 95.5
```

Forced cycles remain in the raw traces and are excluded from that series.
Concurrent mark wall time, GC CPU and stop-the-world pause are distinct metrics.
The candidate's observed mark median and GC CPU are higher; no per-cycle GC
improvement is demonstrated. The busy heaps are different end states after
different command cohorts; the cold-load target is not a busy-server RSS limit.
The pool's owning text/reps are bounded independently of those multi-GiB runtime
observations. These results complete the missing measurement; they are not a
claim that all process memory or GC cost has been solved.

## Correctness and acceptance

The original load-time name/string sharing and compact property storage remain
in their existing owners. The loader regression now asserts representation reuse.
Cold resident memory is below 600,000,000 bytes on both named platforms, with the
new runtime pool linked in. Dump round-trip checks complete and dump bytes match.
The formerly invalid workload now has complete, valid paired observations.
Bounded runtime interning has been evaluated and retained only at the measured
producer; the unsafe direct-input alternative remains rejected.

The comparison commit's PR checks passed: build/unit/types-race/store-race,
managed conformance (zero failures/errors; 316 declared skips), all 18
persistence-related rows, capability admission, Elle and checkpoint campaigns.
Healthy Elle seeds 412/395/396 have both models true, no zero-coverage entries and
parallel counts 805/800/806; hot-renewal counts are 9/7/4. All 20 injected-fault
verdicts are false. Checkpoint generations have empty violation lists and true
Elle verdicts with nonzero operations. Raw correctness archives remain under
`C:/Users/Q/code/barn/.tmp/671-ci-candidate/`.

The final workflow selects the authored false-accept witness in its existing
local-conformance command. Final publication CI is tracked on PR 671; no
experiment-only job or checkout override remains.

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

## Historical failed workload experiment

PR experiment [42594936494](https://github.com/MongooseMoo/barn/actions/runs/42594936494)
used the existing `TestMongooseRealWorkload` harness, three independent
uninstrumented 16-player processes plus a separate diagnostic process. All used
GOGC=400, GOMAXPROCS=32, five seconds warmup, thirty seconds measure,
the original checkpoint and the real sound catalog (481,214,464 bytes, SHA-256
`250b9b69fae2dad6788061a1abb029332f564b92e630514c93181030f7277ca9`). Preserve process exits,
terminal inventories, failures, GC traces and profiles. Failed/unsettled cohorts
do not establish valid application throughput. Instrumented timing is separate.

The experiment failed. Its failure is retained here, not reported as a passing
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

## Historical rejected direct-input-sharing prototype

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

A bounded intern additionally needs owned key bytes, a demonstrated working set,
capacity/eviction and concurrency measurements. The earlier statement that the
optional runtime-intern consideration was resolved was premature: this rejected
one direct-sharing prototype. The resumed owning candidate and its evidence are
described above.

## Historical acceptance correction

The original file's offline resident target is met in the recorded Windows and
Linux observations. Existing load-time sharing and compact slots remain in place;
the loader test now checks actual representation reuse, rather than content
equality alone. The first experiment did not prove full-mix before/after goodput
or per-cycle GC behavior. Publishing its report was not completion of the
authorized task. The resumed valid comparison must finish those measurements.
