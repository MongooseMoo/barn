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

Pending PR-run measurement using the existing `TestMongooseRealWorkload` harness,
three independent uninstrumented 16-player processes plus a separate diagnostic
process. All use GOGC=400, GOMAXPROCS=32, five seconds warmup, thirty seconds measure,
the original checkpoint and the real sound catalog (481,214,464 bytes, SHA-256
`250b9b69fae2dad6788061a1abb029332f564b92e630514c93181030f7277ca9`). Preserve process exits,
terminal inventories, failures, GC traces and profiles. Failed/unsettled cohorts
do not establish valid application throughput. Instrumented timing is separate.

The issue-specific workflow is temporary and must be removed from the final PR.
Raw world diagnostics remain private on the workstation. Commit only bounded
metadata and summaries that do not expose database values or source.
