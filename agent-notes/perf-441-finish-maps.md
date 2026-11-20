# Issue 441: finish MOO map memory work

## Goal

Fully execute the groomed issue 441 plan following Q's explicit make-it-so instruction: implement, measure, publish a PR, validate through CI, normal merge, verify closure and remove task worktrees/branches. Preserve unrelated work. Ownership: Barn types map implementation, existing benchmark driver, focused tests/corpus and this note. Exclude store validation/publication redesign and a new map representation.

## State

Issue body, both comments, merged PRs 449 and 455, current map implementation, persistent-map tests, loader, snapshot rewriting, VM assignment, memory-report implementation and CI read. Local master and fetched origin/master identify the same tip. Existing unrelated untracked work preserved.

Implementation and measurements complete; PR CI and merge pending. The 13 workloads were reconstructed from the issue's primary definitions and expansions are labeled in scripts/testdata/map_workloads.txt. The benchmark result check rejects mismatches, failed evals and failed processes. Promotion builds its missing 17th pair privately; finalization uses the existing list atomic pattern, shrinking the measured header from 112 to 96 bytes. Small updates coallocate the header and exact-sized typed entry storage. The bulk cache, insertion history and rootOnce remain.

## Decisions and why

- Keep the types map API and value immutability. The actual loader calls NewMap; VM assignment calls MapSet; snapshot anonymous-ID rewriting consumes PairsInInsertionOrder before NewMap.
- Do not equate hash-trie traversal with insertion order. The trie has hash paths, entries and child links, with no insertion ordinal. The snapshot consumer explicitly needs insertion order.
- Separate the outstanding no-slower-workload acceptance condition from speculative additional savings. Do not silently accept the recorded map17 regression.

## Surprises

- types/list.go already uses an atomic finalization cache. Map finalization still uses sync.Once; this is an existing local pattern to evaluate, not a reason to introduce a new cache layer.
- PR 455 reports another unresolved cost: one extra 160-byte allocation on converted-map overwrite and retention of both forms when the bulk source remains alive.
- A probe named nonexistent vm/collection.go. Actual owner discovered as vm/collection_helpers.go. Existing external read-path hook was extended to inspect tool_input.input wrappers as well as code/raw-string wrappers. Direct probes returned missing-path exit 2 and existing-path exit 0. Hook transport enforcement in this running session is unverified. This external hook edit is separate from the Barn grooming artifact.

## Feasibility verdict

There is a concrete, bounded implementation route to finishing the original issue. Its small-map and large-load compaction already exist. The remaining mandatory performance condition is unproved: the documented 17-entry workload was slower after small-map compaction. A promising source-level way to reduce that work exists, but neither its speedup nor the complete gate can honestly be guaranteed before measuring it.

The instruction to CHECK whether order can be derived can be settled now: insertion order cannot be recovered from the current hash index alone. That does not prevent completing the original issue; it means the result of that investigation is to retain order information. Eliminating the separate order chain would require storing equivalent information somewhere else and measuring the tradeoff. It is not a free deletion.

## Requirement inventory

| Item | Evidence and present state | Remaining work |
| --- | --- | --- |
| Map-size census | Issue comment reports 240,570 maps / 1,117,420 pairs; 97.2% of maps at most 16 pairs. Original scratch census test was deleted according to the contemporaneous task record. | These are historical counts. Preserve the historical results; any refreshed census must state whether it counts occurrences or unique payloads and how nested/shared values are treated. Exact retained bytes per class are not available from the issue table alone. |
| Small compact maps | PR 449 merged; types/map.go:34,669 uses the 16-entry flat form. | Preserve its gains and recover the outstanding boundary workload. |
| Compact large loaded maps | PR 455 merged; types/map.go:696-703 retains a flat slice plus bulk hash index. db/format/reader_value.go:152 is a real producer. Empty values reuse loadedEmptyMap at reader_value.go:134. | Preserve cold-load savings and first-write behavior; assess retained dual forms separately. |
| Derive order | map_index.go:15-18,74-77 has hash-trie fields without insertion ordinals. map.go:498 replays insertion order into its lazy ordered tree. | Keep order information. A different representation remains a separate measured design, not an assumption. |
| No map workload slower | Issue comments record map17_grow_and_shrink_100k at 780 ms before small maps, 925-939 ms after. | Refresh current timing, repair, and compare all workloads. Historical timings alone do not prove today's regression magnitude. |
| Unit/property tests and managed conformance | Existing tests and CI entrypoints inspected; not executed as part of grooming. | Run through the implementation PR as required. |
| Two sync.Once fields | map.go:55,60; root cache has an explicit concurrent-shared-root contract in map_cache_test.go. | Evaluate finalization cache using existing list pattern; keep rootOnce unless a separately measured and equivalently safe replacement is justified. |
| Runtime-grown indexed maps | map.go:529,553-559; map_index.go:228-248. | Still use trie and order chain. Their retained cost after real mutation needs evidence before selecting a new representation. This is additional optimization mentioned in comments, beyond the original compact-small-map deliverable. |

## Current evidence

Source was read on master after fetching origin/master; both tips matched, with latest subject `Merge pull request #667 from MongooseMoo/claude/issue-666`. Tracked diff was empty before and after the investigation. Source identities belong in execution manifests when implementing, rather than embedded commit IDs in this note.

Original checkpoint found at `C:/Users/Q/projects/mongoose/db/mongoose.db.new.2026-10-10T0548`, length 165,578,514. Its SHA-256 begins `12286FDA9B50BFFA`, matching the contemporaneous record; full hash was returned by Get-FileHash in this turn. No source checkpoint was modified.

Commands run after verifying `go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn` returned `github.com/MongooseMoo/barn/cmd/barn main`:

```text
go version
go version go1.27.1 windows/amd64

go run ./cmd/barn -db C:/Users/Q/projects/mongoose/db/mongoose.db.new.2026-10-10T0548 -memory-report -memory-profile .tmp/issue-441-current.heap
objects:          32667
property_slots:   4956720
clear_slots:      4191470
private_slots:    765250
heap_alloc:       402672728
heap_objects:     3486567
heap_inuse:       517103616
sys:              905956600
total_alloc:      978227920
gc_cycles:        6
```

This is one Windows run, not an A/B experiment, RSS measurement or timing result. internal/app/app.go:251 dispatches directly to the offline report. internal/dbtool/memory.go:33-34 performs two GCs; line 63 keeps the store alive through measurement. No server was started.

Raw relevant rows from `go tool pprof -list='types.NewMap' -sample_index=inuse_space .tmp/issue-441-current.heap`:

```text
Total: 389.34MB
89.58MB 91.61MB (flat, cum) types.NewMap
17MB       668: m := &goMap{byteSize: listVarOverhead}
47.52MB    671: m.flat = make([]flatEntry, 0, len(pairs))
24.06MB    696: flat := make([]flatEntry, len(pairs))
2.03MB     702: if bulk, ok := newBulkIndex(flat); ok {
1MB        703: return mapValue(&goMap{...})
```

These are sampled allocation-site estimates as labeled by pprof. They are not a map-size-class census. The profile shows the current cold-load map cost mainly at flat constructors, not the historical large-map trie allocation sites. Header savings cannot eliminate the entry-array cost.

## Contracts that constrain the design

- Real assignment: vm/collection_helpers.go:61 invokes MapSet, then quota checks the result. Preserve ValueBytes and error behavior.
- Real loading: db/format/reader_value.go:123-152 builds whole maps; empty loaded maps are shared.
- Real snapshot rebuilding: db/store/store_snapshot.go:347-363 uses PairsInInsertionOrder then NewMap when anonymous IDs change. Replacing it with Pairs changes the input ordering to the constructor.
- types/value.go:314-348 makes Identical insertion-order-sensitive for flat, mixed-representation and indexed maps. Store callers use Identical to detect no-op writes and validate keyed map observations (store_txn_mapkeys.go:144,200,293; store_txn_properties.go:111,390). Preserve this contract as well as visible traversal.
- builtins/maps.go:17 uses Keys, line 32 case-sensitive GetWithCase, line 42 Pairs. map.go:746 MapColumns is another traversal API. Preserve all paths, including lazily built versus warm ordered trees.
- types/boolean_map_test.go and map_columns_test.go contain topology/isolation expectations. Before changing any uncertain MOO behavior, obtain exact managed WSL Toast evidence. No new oracle verdict is claimed here.
- Indexed deletion currently copies the newer prefix of the insertion chain (map.go:584-598). Keeping a raw hash index while deleting order information would also remove what this path and insertionEntries consume.

## Complete implementation plan, after make it so

1. Freeze the measurement inputs: original checkpoint, current default-branch control, pre-small-map historical control, toolchain, architecture, GC settings and exact corpus. Retain full manifest and raw results outside this note. Recover the historical map_corpus.txt and bench_premaps/bench_now reports if possible. The contemporaneous record says they survived in scratch barnmem, but this turn did not recover them from the bounded candidate locations checked. Otherwise reconstruct all 13 complete statement lists from primary records and label the reconstructed corpus. The issue comment abbreviates several rows, so copying it literally is insufficient.
2. Refresh the baseline timings with scripts/bench_differ.py using --corpus, --repeats, --barn-linux and separate --out directories for each control/candidate. Run both engines on the same verified installed WSL distribution and use the canonical WSL Toast executable. Do not use the script's default corpus: scripts/bench_differ.py:56-76 contains no maps. Preserve complete outputs, successful eval results, sample counts and process statuses.
3. Repair the smallest shown construction cost in types/map.go and its existing builder: the 16-to-17 path at map.go:529 first builds a private 16-entry indexed map, then immediately invokes persistent set, which clones the update path and creates another header. Build the final promoted result while that builder is private. Keep a single implementation of construction, existing public methods, immutable aliases, metadata accounting and insertion history. Do not add a promotion cache, change the threshold or remove order as a substitute for this repair. Benefit is a hypothesis until measured. Bulk-source conversion caching must remain correct for callers repeatedly deriving from the same source.
4. Evaluate the map finalization header against the existing types/list.go:93-120 atomic cache pattern. Preserve immutable initial proofs and resolved-state propagation, nested anon/WAIF scanning and concurrent readers. Update the existing first-scan allocation test rather than pretending a reset sync.Once still tests the new cache. Measure allocated size class, retained bytes and cold/warm scanning before retaining this change. Leave the ordered-root cache's shared-root contract intact.
5. Recheck the comment's unresolved bulk-write cost with separate cold conversion and warm overwrite measurements; PR 455 reported an extra 160-byte allocation without a diagnosed cause. Capture an allocation profile before assigning a cause. Measure source-retained and source-dropped lifetimes separately because bulkIndex.indexed intentionally keeps the converted form reachable. Remove redundant work only when the current measurements demonstrate it. Do not claim the extra allocation still exists from the old report alone.
6. Settle order explicitly in the deliverable: current index cannot derive insertion history. Preserve order and explain why. If runtime-grown-map retained profiles later justify compressing that information, evaluate an integrated insertion ordinal or another concrete representation separately, including sort/traversal and persistent-update costs. No new representation is selected by this grooming task.
7. Verify the complete acceptance set below on the final candidate. Fix actual failures within scope. Publish only when requested; implementation and publication have not been authorized in this grooming turn. Independent PRs must use the default branch, never stack.

## Acceptance and evidence

- Same original file: before/after memory-report with full inputs and raw stdout; candidate heap_alloc must improve on its current control. Preserve the sampled profiles, and distinguish retained memory from allocation churn and RSS.
- Existing persistent snapshot property, mixed-key flat/indexed comparisons, full-hash collision test, duplicate overwrite, delete/reinsert, ValueBytes, finalization and concurrent alias/root tests. Add focused boundary histories retaining all prior versions and both cold/warm reads where existing tests do not cover the new path.
- All 13 map workloads, including 16-to-17 build/delete, must have complete successful samples and oracle-equivalent results. Compare both against current control and the pre-small-map control that exposed the original failure. No silently accepted regression, threshold bump or waived workload. Use interleaved repeated comparisons to separate a repeatable slowdown from noise; no speedup magnitude is promised here.
- Include cold bulk first writes, warm derived writes, duplicate-heavy bulk construction and source-retained memory so the historical corpus's omissions cannot conceal transferred costs. Retain the original corpus unchanged as the original gate, with additional workloads clearly identified.
- scripts/bench_differ.py:164 prints lane exit codes but does not reject them, and lines 282-286 return failure only for missing samples; values_match false alone does not fail its exit status. Its green exit is insufficient evidence. Correct the existing result check during authorized implementation so process failures, failed evals, mismatches and incomplete samples cannot pass. Preserve raw failing output as a regression fixture before that repair; no replacement harness or extra review gate is needed.
- CI on the implementation PR: existing .github/workflows/ci.yml runs formatting, vet, staticcheck, lint, build, full Go tests, types/store race tests, driver tests and the full managed conformance suite with admission evidence. Keep its CI_RUNNER_LABELS routing. No local broad test or conformance run was done during grooming.
- For any uncertain map ordering/lookup/persistence behavior, use the documented managed oracle workflow in docs/mongoose-conformance.md first, with capability_admission in the same session. scripts/check-map-conformance.sh identifies the focused packaged map selections; a focused run does not replace full conformance.
- Keep store validation/publication and other isolation-sensitive logic unchanged. If implementation reaches those owners, docs/elle-isolation.md applies, including raw histories, manifests, explicit checker verdicts and nonzero committed paths. Existing unit tests are not an Elle verdict.

## Literature

Applied paper-reader to the existing complete local Blade paper directory, whose required text artifacts already exist, and reread all 14 canonical page images. Terei and Levy, Blade: A Data Center Garbage Collector (2015), especially pp. 2-3,7-10: heap/collector costs and end-to-end latency/throughput require separate observations; its experiments use historical Go 1.4.2 and redundant services. Usefulness here is limited to measurement discipline. It supplies no compact-map layout or proof of a Barn speedup, and does not justify adding GC coordination to this single-server task.

The web search found the CHAMP paper as a possible source for a later trie redesign. It was not read through paper-reader and no design or claim here relies on it.

## Next step

Publish the measured implementation and its evidence, then watch the PR's build/unit/race, full managed conformance, and Elle required checks. Fix failures, normal merge, verify issue closure and two-parent topology, and clean up only this task's worktrees and branches. The living note is staged with the substantive implementation in the candidate worktree.

## Execution results

- Current control: both engines exit 0, 91/91 samples each, all matching; map17 median 930.10 ms. Promotion/header-only candidate 807.61 ms versus pre-flat 734.04 ms failed the boundary requirement and was not accepted.
- Profiling attributed 69.69% of allocated objects to goMap.set. Coallocating the same flat representation removed one allocation per small insertion/replacement; the authored regression fails on the control (2 allocations) and passes on the candidate (1). Pointer-survival, alias, finalization propagation and cold-reader tests passed locally; broad checks remain for PR CI.
- Final complete corpus: candidate map17 719.53 ms, pre-flat 734.04 ms; candidate medians are lower on all 13 workloads than both controls, with complete successful matching values. Additional paired independent process medians: pre-flat 747.94/721.38/767.95 ms; final candidate 741.91/748.88/776.59 ms. The median of those process medians is 747.94 versus 748.88 ms, indistinguishable at this observed variation; no consistent historical slowdown is established. This is not a universal speed guarantee.
- Original checkpoint offline memory-report, same Windows Go toolchain: heap_alloc control 402665608; candidate 399468272. Same object/slot counts. Raw heap profiles remain outside the task worktrees under the main checkout's .tmp directory.
- Same-process warm writes: bulk-converted=6 allocations, directly-indexed=6 in both control and candidate. The historical extra allocation is not reproduced; no guessed fix applied. Two-thousand-pair source-retained versus source-dropped retained bytes: control about 670273/546658; candidate about 665905/542321. Different process hash seeds vary trie shape, so these are diagnostic samples, not a precise cross-process savings claim.
- Versioned evidence prepared in experiments/issue-441: current-results.json, preflat-results.json and candidate-results.json. Their git_head is the driver checkout, not proof of a supplied binary's source; the report wording was corrected accordingly. Candidate source and binary identities will be documented in the experiment report, not this note.
