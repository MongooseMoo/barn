# Issue 279: memory evidence and runtime string sharing

## Goal

Execute the groomed [Barn issue 279](https://github.com/MongooseMoo/barn/issues/279) work in an isolated worktree and publish a PR. Preserve the completed load-time sharing and compact property storage. Collect the remaining evidence, fix verified diagnostic defects and resolve the optional runtime short-string interning consideration from measurements. Leave unproved original acceptance requirements explicit.

## State

Execution authorized, 2026-11-09, in C:/Users/Q/code/barn-279 on perf/279-memory-evidence. Draft PR 671 is open. Preserve unrelated work in the main checkout. Initial CI build/unit, full conformance, Elle and checkpoint checks passed. The temporary measurement job failed and its raw evidence is preserved privately under C:/Users/Q/code/barn/.tmp/issue-279-ci/42594936494-1. Do not treat it as a passing performance comparison. Final changes fix Linux resident units in the workload diagnostic, strengthen loader representation-sharing checks, retain canonical-name/large-backing retention regressions and an allocation benchmark, and record all experiment outcomes. The temporary workflow is removed. No production intern or respond_to implementation change is retained. Focused authored tests passed; final-head PR checks are next.

The issue is open, but its three principal implementation slices exist. Its body is stale about the property representation and baseline. The last comment reports that Windows resident memory met the target; it explicitly leaves Linux resident memory, final goodput and under-load GC evidence unmeasured. That comment's runtime-created slot gap was split into issue 456, now closed as completed; the implementation is present in current source.

## Requirement inventory

| Requirement | Current primary evidence | Remaining work |
| --- | --- | --- |
| Share loaded property names | `db/format/reader_helpers.go:145` creates a shared layout pool; `layoutFor` reuses a single parent's layout when definition/slot counts permit. | Preserve behavior; no new layout implementation needed. |
| Intern loaded string values | `db/format/reader.go:61` returns one Value per exact string content; `reader_value.go:54` calls the buffered-byte path. The pool is discarded at `reader.go:235`. | Preserve exact bytes and load-only lifetime. |
| Compact property storage | `db/store/prop_table.go:34` stores shared bases, sorted private `setSlot` entries, overflow names and removed names. `base.slot` supplies a clear, value-less, version-zero slot. | Preserve owner/perms, definition ordering, clear behavior and publication versions. The body's flat Value slice proposal has already been superseded. |
| Runtime-created objects | `store_metrics.go:66` shares newly created tables through the store pool; actual producers are `store_lifecycle.go:28,205` and `store_txn_objects.go:198`. `created_slot_sharing_test.go:64,107` covers sharing and definitions. Live issue 456 is closed. | Do not reopen this implementation under 279. |
| Never-written slot versions | `prop_table.go:87` requires version zero for omitted placeholders; `store_txn_validation.go:107` compares an observed slot version with the live slot on that object. | No reason to invent a private entry for every clear slot. Any future representation edit must preserve reparent/write invalidation and run the pinned Elle workflow. No such edit is proposed here. |
| Heap and RSS | Fresh heap output below; final issue comment historically reports 560,930,816 bytes resident on Windows. | HeapAlloc, HeapInuse, Sys and resident memory are distinct. Refresh actual resident memory on the named platform; Linux result remains unknown. |
| GC and 16-player goodput | Issue's middle comment says historical variation was 20–25%, and the final comment says final-build goodput/per-cycle mark time were not measured. | Finish valid under-load evidence. Do not infer speed from a cold-load heap reduction or a forced collection. |
| Dump and persistence | Issue reports equal dump bytes after earlier slices. Current CI owns the managed conformance lane. | Preserve historical evidence; run appropriate persistence conformance and dump comparison for any new candidate. No new green claim from this grooming turn. |
| Consider bounded runtime short-string interning | `types/str.go:32` already shares empty strings. Nonempty NewStr allocates a rep. `vm/op_verb.go:517` and `vm/vm.go:335` are real repeated verb-name producers; `vm/op_index.go:35,354` can pass short substrings backed by larger strings. | Establish duplicate content, lifetime, bytes and producer cost before selecting an intern design. “Consider” does not require shipping a cache with no measured benefit. |

## Fresh baseline evidence

Toolchain output:

```text
go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn
github.com/MongooseMoo/barn/cmd/barn main
go version
go version go1.27.1 windows/amd64
```

Original issue-comment checkpoint, verified in this turn:

```text
C:/Users/Q/projects/mongoose/db/mongoose.db.new.2026-10-10T0548
Length: 165578514
SHA256: 12286FDA9B50BFFA826186E518F1BA14781BEDBBFAE7EAE35197E9D8EBFC0D56
```

Command and raw stdout, exit 0:

```text
go run ./cmd/barn -db C:/Users/Q/projects/mongoose/db/mongoose.db.new.2026-10-10T0548 -memory-report -memory-profile .tmp/issue-279-current.heap
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

`internal/app/app.go:251` dispatches directly to the offline report. `internal/dbtool/memory.go:33-34` forces two collections; the helper drops the loader and keeps the store alive through measurement. This does not start a server or modify the checkpoint. It is one current Windows observation, not an A/B timing or RSS result.

Selected raw rows from `go tool pprof -top -sample_index=inuse_space .tmp/issue-279-current.heap`, exit 0:

```text
Type: inuse_space
Showing nodes accounting for 386.55MB, 98.72% of 391.55MB total
flat       cum       owner
91.36MB    93.37MB   github.com/MongooseMoo/barn/types.NewMap
65.34MB   250.79MB   github.com/MongooseMoo/barn/db/format.(*Database).readValueAfterType
39.65MB    39.65MB   slices.Clone[...store.setSlot...]
34.09MB    54.59MB   github.com/MongooseMoo/barn/db/format.(*Database).loadedStrBytes
31.50MB    31.50MB   github.com/MongooseMoo/barn/types.newSliceList
24MB       24MB     github.com/MongooseMoo/barn/types.NewStr
```

These are sampled allocation-site estimates. Load already deduplicates string Values, so the NewStr row is not evidence that a runtime intern would reclaim those 24 MB. No current runtime string content/lifetime census, runtime CPU profile, valid 16-player cohort, Linux RSS or per-cycle GC comparison was collected here.

## Decisions and why

- Finish the original issue rather than repeat its completed slices. Keep map work in its existing owner; issue 441's changes are already in current master.
- Treat runtime interning as a measured hypothesis. NewStr is a hot shared constructor, and changing it globally could add hashing/synchronization to unique, transient strings. A cold-load profile does not justify that change.
- Preserve byte-exact content, case and invalid UTF-8. `Value.Identical` compares exact string content (`types/value.go:301`); MOO equality has a separate case-insensitive contract. Neither is pointer identity.
- Preserve append semantics. `types/str.go:177` has watermark-backed append storage, while NewStr reps have no watermark. Do not canonicalize mutable builder state or route all append results through a new intern.
- A bounded number of short keys is not by itself a backing-byte bound: short substrings can retain large allocations. Any retained candidate keys must own only the intended bytes, and accounting must include keys, reps and table storage.
- Do not select a capacity, length threshold, eviction policy, sharding or weak-reference machinery without the workload evidence. No new flag, dependency or public API is justified now.

## Literature and standard-library research

Applied the paper-reader skill to the existing complete local Blade paper directory and reread all 14 canonical page images. Terei and Levy, *Blade: A Data Center Garbage Collector* (2015), pp. 2–3 and 7–10 distinguish collector work, application latency and throughput. Its evaluation used Go 1.4.2 and redundant distributed services; it provides measurement context, not an interning algorithm or evidence of a Barn improvement. No GC coordination is proposed.

Found Appel and Gonçalves, *Hash-consing Garbage Collection* (1993), through the author's [publication page](https://www.cs.princeton.edu/~appel/papers/hashgc.pdf), but did not read its page images; no design conclusion here relies on it.

Read the official [Go interning article](https://go.dev/blog/unique) and [Go 1.27.1 unique API](https://pkg.go.dev/unique@go1.27.1). `unique.Make` is concurrent and returns a Handle; handle lifetime matters to canonicalization. It is not a ready-made fixed-budget cache of Barn `strRep` values. Simply interning Go string bytes would still leave NewStr's rep allocation. Changing Value/strRep ownership to keep handles needs separate justification and is not selected.

## Prepared execution plan

1. Start an isolated branch/worktree from current origin/master after implementation authority. Carry this living note into that branch. Preserve the dirty main checkout. Record actual source and executable identities in an experiment manifest, not commit IDs in this note.
2. Finish the baseline with existing tools: original checkpoint above, actual Windows resident memory, Linux resident memory if the execution platform is available, and under-load CPU/retained/allocation profiles. Use the current diagnostics collector and immutable fixture plus a real sound catalog. Resolve the catalog from the existing workload setup; do not accept the harness's empty-catalog error path. Record players, repair settings, command mix, GC configuration and machine load.
3. Run the existing 16-player workload via PR CI, with terminal inventories, native errors and per-cycle GC trace preserved. Use repeated paired observations for any candidate. Distinguish CPU mark cost, concurrent mark wall duration and stop-the-world pauses; retain raw gctrace. Reject failed or unsettled cohorts as performance comparisons. Do not manufacture a performance fix from an invalid workload.
4. Assess runtime string duplication at the producer identified by the runtime profiles. If the existing profiles cannot resolve retained duplicate short strings, add the smallest focused diagnostic/benchmark to the existing owner, rather than permanent production instrumentation. Separate repeated values from unique values, retained from transient lifetimes, and sequential from concurrent construction. Check substring-backed inputs and append aliases. Run authored tests only locally; broad tests and workload runs belong to PR CI.
5. If runtime duplication is material, write the concrete budget and ownership decision from that evidence before implementing it. Prefer the narrow producer or existing owner that removes the shown duplication. Candidate must preserve exact bytes, append aliases, concurrency and the existing Value API. If it does not improve retained memory or allocation cost without a measurable goodput regression, leave it out and record that the optional consideration was evaluated. This is a decision condition, not a promise of a cache.
6. For any candidate, compare the same checkpoint's heap profile and actual RSS with control; preserve dump output and compare bytes. Through the PR, run existing build/unit/types-race/store-race and full managed conformance with capability admission, including persistence. Keep runner selection behind CI_RUNNER_LABELS. If storage validation/publication is touched despite the current scope, first read docs/elle-isolation.md and run its pinned workflow with nonzero-path verdicts and raw histories. No local broad CI rerun.
7. Draft the final issue update around completed slices, current evidence, platform limits and the runtime-intern decision. Publishing the update, opening a PR or closing the issue requires Q's explicit request for those posts/actions. This grooming request does not authorize them.

## Surprises

The largest correction is scope: the issue title/body describe an implementation that is largely finished. Its last comment is also behind issue 456's completion. Starting from the body alone would duplicate current code and confuse heap figures with resident memory.

- Current offline resident memory: Windows 557,805,568 bytes; Linux 132,473 pages at 4,096 bytes/page = 542,609,408 bytes. Linux heap_alloc 399,455,992 bytes. These are cold/offline observations.
- The existing workload readMemSample treated Linux page counts as bytes, logging resident=0 MiB after load. Factored the existing parser, reproduced the wrong result using the captured 132473.0 reading, then converted Linux counts using os.Getpagesize. Focused authored regression and strengthened loader test pass locally. Broad checks belong to the PR.
- The first uninstrumented process failed copying the sound file with native read ENOMEM. Later processes copied it successfully. Memory/pressure/mount/kernel data were inspected; no same-day kernel OOM match was found. Primary Go source and WSL incident reports read; the underlying native ENOMEM cause is still unknown, so no speculative copy retry or recovery was added.
- Full-mix samples: 7,386 completed / 6 failed / 1 unsettled, then 8,024 completed / 5 failed / 0 unsettled. Native E_NACC traces include player 71's home path. The diagnostic process also has an unsettled command. Preserve all failed cohorts; no speedup or complete goodput gate is claimed.
- Runtime profile identifies a narrow duplicate producer: builtinRespondTo accounts for 5,366,086 of the sampled 10,117,708 NewStr allocations. It builds NewStr(strings.Join(verb.Names, " ")) for each readable result even when a single canonical alias exactly equals the supplied string Value. Ordinary VM calls already reuse their input name Value (op_verb.go:64,269), so a global intern would also hash strings already shared.

## Current implementation decision

Rejected direct reuse of respond_to's input Value after measuring it. The exact-alias microbenchmark removed one allocation and 64 B/call (160 B/3 allocations to 96 B/2), but a four-byte input sliced from a 16 MiB source retained 16,736,328 bytes through the response alone; control retained no corresponding large allocation (measured delta -48,744 bytes). The initial probe retained the lookup memo under both builds; corrected the fixture to release/drop the finished transaction before making this comparison. The VM is a real substring producer at op_index.go:354. The provisional statement that this had no lifetime extension was wrong; the corrected retention experiment disproved it. Removed the candidate implementation. Preserve canonical-output and large-backing-retention tests plus the benchmark for future work. No global intern pool or per-verb cache is justified by a valid application comparison here. Keep the existing implementation and record that the optional runtime intern was considered. Invalid full-mix measurements remain a limitation rather than justification for changing permissions or task scheduling.

## Next step

Commit the final changes and run final-head PR CI; inspect results and the published diff. Leave PR 671 open; merging was not requested. Preserve the failed experiment and explicitly leave issue 279's original full-mix throughput/per-cycle GC improvement requirement unproved. That unresolved requirement is not a code fix or memory improvement this PR can claim.
