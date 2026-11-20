### Checkpoint 2026-09-25: SIMD and Mongoose collection filters
- Fetched origin/master 501b83b; current perf/mongoose-live-20260922 HEAD 70106bc is 0 ahead/19 behind; inspected master without switching or changing implementation.
- September 22 local mongoose.db.new lexical census: 12,841 verb blocks, 3,589 anchored self-append sites in 1,497 verbs; 1,420 sites lexically nested under loop/if in 813 verbs. These are heuristic source counts, not runtime frequency or AST eligibility.
- Master already specializes self-append and loop instructions; append still creates list headers, and map/indexed-list iteration materializes pair lists. Expiration filter #1902:6 is a promising scalar-specialization candidate; SIMD benefit remains unknown.
- No implementation or conformance changes; earlier integer CPU profile was on the older branch with Go 1.26.0. Next: benchmark current-master iteration pairs and append headers, then restricted scalar/SIMD filters; verify uncertain semantics through managed WSL Toast before implementation. No blocker identified.

### Checkpoint 2026-09-25: updated master and measured filter costs
- Switched this checkout to master and fast-forwarded to origin/master 501b83bb2c80a9f1fb777688551b26a258884076; added uncommitted vm/filter_survey_test.go investigation probes, with no production edits.
- AST census supersedes lexical estimates: 12,841 extracted bodies, 12,827 parsed, 14 parse failures excluded; 3,629 self-append assignments in 1,506 verbs, 1,444 nested under loop/if, 249 indexed collection loops, 145 strict filter shapes (52 builtin, 34 verb call, 31 property, 28 no calls/properties). Shapes are not proof of optimization eligibility or runtime hotness.
- Five baseline samples per case, Go 1.26.0 Windows amd64, cpu=1: 10k reject-all representative rows list 570641 ns/15816 B/19 allocs, indexed 2080118 ns/1221624 B/20021 allocs, map 2521114 ns/1704952 B/20022 allocs. Half-selected list median 1694883 ns and 5051 allocs; count-only median 721220 ns and 11 allocs (different work, not a drop-in optimization).
- Indexed reject-all alloc_objects profile attributes 99.57% cumulatively to executeIterPrep; half-selected list CPU profile attributes 32.43% cumulatively to sliceList.append. Profiles and census/disassembly output: C:/Users/Q/AppData/Local/Temp/barn-filter-master-ec1c331bbacc4ff19871a707c93be51c. Next: direct-iteration prototype and measured header reduction after managed Toast semantic checks; SIMD and whole-Mongoose speedup remain unmeasured.

### Checkpoint 2026-09-25: direct iteration implementation underway
- User authorized implementation; isolated branch perf/direct-collection-iteration at C:/Users/Q/code/barn-direct-iteration, based on 501b83b. Main checkout remains master with investigation probes untouched.
- Managed Debian WSL Toast: looping 30 passed; new iteration_snapshot.yaml plus tick_accounting 71 passed (admission included). New conformance file is uncommitted in moo-conformance-tests.
- Baseline allocation regression reproduced: list 33->2013 and map 34->2014 allocations for 10->1000 elements. Baseline test binary retained in worktree .tmp/iteration-perf/base.test.exe.
- First column implementation passes focused VM tests. Review found value-only map iteration must retain keys as GC roots; next revise column loading to preserve keys, expand checkpoint/GC coverage, run managed Barn conformance and paired benchmarks. No implementation published or committed.

### Checkpoint 2026-09-25: column implementation validated locally
- Column preparation now always retains map keys, including value-only loops. New prep/value/indexed-load opcodes preserve old pair bytecode for suspended programs; maps use two snapshots and lists retain their original immutable value.
- Allocation regression now passes: list 11/11 and map 15/15 allocations for 10/1000 entries. Added map order/isolation, old-bytecode, key-root, dispatch, and indexed/map/string checkpoint tests; go test ./... -count=1 exited 0.
- Managed Barn looping/snapshot/tick run is ongoing (72% at last poll), binary .tmp/iteration-perf/barn-linux; no failure markers observed yet, final result pending. Test binaries base.test.exe and candidate.test.exe built for paired timing.
- Investigation-only copied census probe replaced with focused collection_iteration_bench_test.go in worktree; original main-checkout probe untouched. Next: finish conformance, interleave ten baseline/candidate samples, run focused race checks and final review. Changes remain uncommitted.

### Checkpoint 2026-09-25: first optimization complete locally
- Barn commit 391b1f3 on perf/direct-collection-iteration in C:/Users/Q/code/barn-direct-iteration; conformance commit 691715d in moo-conformance-tests. No push, PR, or merge; main Barn checkout remains master.
- Managed Barn looping/snapshot/ticks: 100 passed; full Go suite exit 0; focused VM/types race tests passed. Checkpoint tests additionally verify keys/indexes after reload and passed normally and with race instrumentation.
- Ten interleaved benchmark pairs: 10k half-selected indexed filter 3.243->1.785 ms (-44.96%), map 3.602->2.162 ms (-39.99%); reject-all allocations indexed 20021->19, map 20022->23. These are synthetic filter measurements, not whole-Mongoose throughput.
- Report and raw samples committed under reports/collection-iteration-2026-09-25.md and experiments/2026-09-25-collection-iteration/. Archived text only normalizes whitespace; committed diff check exited 0. Remaining opportunities: append-header reduction and predicate/SIMD specialization, not implemented in this slice.
