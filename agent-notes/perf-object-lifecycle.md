# Object creation and destruction performance

## Goal

Optimize creation and destruction of regular and anonymous objects in this
isolated worktree. Q also authorized opening a PR and merging after green CI.

## State

Q said "Make it so!". Implementation and publication through normal merge are
authorized. The branch was fast-forwarded to current master before editing.
Benchmark-only baseline retained in a detached sibling worktree. Its fixture
check completed with expected counts, including automatic anonymous collection
and exactly-once inherited recycle hooks. Profiles identified inherited-slot
map materialization and property-table copies as allocation costs. Implemented
reuse of an existing matching inherited base through one shared ancestry walk,
and replaced recycle's temporary property-value slice with direct visitation.
New semantic regressions pass, including multiple-parent shape union. Repeated
paired store measurements with the corrected snapshot-release guard completed:
ten samples per arm, 32 objects per operation. For a single 150-slot parent,
regular/anonymous create time fell 83.02%/87.37% and pair time fell
74.32%/82.10%; allocated bytes for creation fell about 69%/74%. These are
synthetic results with some noisy rows, not production throughput claims.
The single-parent 150-slot store-only recycle time is statistically unchanged.
VM batches of 32 show 69.61%/56.70% lower creation bytes, 18.08%/18.96% lower
recycle bytes, and 39.33%/45.05% lower pair time for regular/anonymous objects.
Longer empty-store measurements did not reproduce initial short-run slowdowns.
Retained heap is unchanged at about 555/631 bytes per regular/anonymous object.
The isolated store recycle fixture now completes setup GC before timing, which
removed the repeatable two-parent slowdown. Short empty-store timing changes
vary in sign across campaigns; retain them but make no small-case speed claim.
Final CPU/allocation/in-use profiles and all raw samples are preserved under
`experiments/object-lifecycle/`, with methods, rejected samples and limitations
in its README. Source and performance work is complete. Full CI, PR publication
and normal merge are the remaining delivery steps. Branch:
`perf/object-lifecycle`.

Read `AGENTS.md`, `docs/workload-diagnostics.md`, `docs/elle-isolation.md`,
the Go performance and benchmarking-statistics skills, and the source below.
Processed Bonwick's 1994 allocator paper through paper-process's notes-only
workflow, inspecting all twelve page images. Paper relevance is medium:
separate construction cost from allocation cost and measure reclamation as well
as allocation; its kernel allocator results are not Barn results.

## Baseline source findings

- `builtins/objects.go:229`: regular creation stages through `StoreTxn.CreateObject`
  until a task has mutated live state. Anonymous creation crosses `beforeCoarse`,
  creates live state, then adopts the object and relationships.
- `db/store/store_txn_objects.go:177`: regular creation builds inherited slots,
  shares their base, clones a pristine image, and stages parent child deltas.
- `db/store/store_lifecycle.go:9`: anonymous creation holds the store lock,
  builds and shares inherited slots, retains an out-of-band identity and
  publishes parent relationship updates. It must not become a numbered object.
- `db/store/store_txn_property_inheritance.go:261` and
  `db/store/store_properties.go:9`: inheritance construction walks ancestry and
  materializes a table. `db/store/prop_table.go:501` then sorts keys and constructs
  owner/permission arrays before looking up the existing shared base. This is a
  candidate allocation cost, not a measured bottleneck.
- `builtins/objects.go:451` and `db/store/store_txn_properties.go:275`: recycle
  materializes property values and scans them for anonymous references before
  choosing staged regular recycling or coarse recycling.
- `db/store/store_txn_objects.go:250`: simple regular recycling retains
  relationship dependencies, stages removals, and builds a tombstone. Complex
  regular objects and anonymous objects use `Store.Recycle`.
- `db/store/store_lifecycle.go:66`: coarse destruction updates children,
  contents, parents, flags and direct references under the store lock.
- `vm/recycle_lifecycle.go`: the production VM carries recycle hooks and exit
  hooks in continuations. A benchmark bypassing these calls cannot establish an
  end-to-end lifecycle improvement.
- `db/store/store_reachability.go:276` and `vm/anonymous_gc.go`: automatic
  anonymous collection has candidate selection, persistent and VM roots, and
  callback routing distinct from explicit recycle.

Existing evidence: `BenchmarkCreateChild` in
`db/store/created_slot_sharing_test.go` covers committed creation under a
150-slot class. `scripts/bench_mongoose_matrix.py` contains a regular
create/recycle workload. `experiments/2026-10-08-deferred-gc-router.md` measures
an older candidate-routing change; it does not measure the present creation or
destruction paths. No task-specific exception frames, conflict census, current
CPU/heap profiles or four-way lifecycle measurements have been collected.

## Plan

1. Add focused lifecycle benchmarks in `db/store/object_lifecycle_bench_test.go`
   and `engine/object_lifecycle_bench_test.go`, using existing store/engine fixture
   builders. Measure regular and anonymous creation, explicit recycle, and
   complete create/recycle pairs separately. Use empty parents and a realistic
   150-slot class, plus a bounded multiple-parent fixture. Include both repeated
   tasks and a bounded batch within one task. Time setup separately from work;
   destruction fixtures must already exist before its timed interval. Verify
   returned counts and invalidated references so an error/empty path cannot win.
   Add one bounded drop-reference/collection case for anonymous lifetime, with
   observable recycle-hook completion. Keep growing object directories and
   retained tombstones visible; use identical bounded cohorts in both arms.
2. Run only the benchmark files being written locally, with unit tests disabled,
   allocation reporting and fixed Go/runtime settings. Retain a benchmark-only
   baseline commit; compare it to the implementation with the same benchmark
   source and machine. Collect separate CPU and allocation profiles; inspect
   flat/cumulative CPU, alloc_space and retained heap. Record environment and
   failures. Existing full-suite tests run in PR CI, as Q requires.
3. Select the smallest change indicated by those profiles. First candidates are
   avoidable inheritance-table materialization and recycle's temporary property
   slices/full facet copies. Reuse the existing property representation and
   iteration owners. Do not assume any candidate is hot or commit to a cache,
   pool, allocator, locking redesign, or transaction redesign before evidence.
   Preserve permission checks, parent precedence, dependency reads, callback
   ordering, exactly-once hooks, stable identities, and native errors.
4. Repeat at least ten observations per arm, alternating arm order in matched
   batches. Use benchstat for ns/op, B/op and allocs/op, reporting every workload
   independently. Reject added complexity without a repeatable benefit; inspect
   regressions in the other object type and in destruction/retained memory.
   Profiles diagnose costs and are separate from timing comparisons. Do not
   infer a production speedup from microbenchmarks.
5. Add behavioral regressions for the actual change. Resolve any uncertain MOO
   behavior first through the documented managed WSL Toast oracle, with capability
   admission. Preserve existing regular/anonymous lifecycle, suspension, roots,
   property ownership, and concurrent-recycle regressions. If definition or
   relationship reads/publication change, require the existing pinned Elle CI
   workflow with raw histories, manifests, nonzero paths, and explicit verdicts.
6. Commit the substantive work and this note, open the authorized independent
   PR against master, watch all current-head CI, repair red checks within scope,
   and merge normally when green and required gates clear. Verify the remote
   merge and local integration, then remove only this task's worktrees/branches.

## Decisions and why

The baseline VM fixture now shares its loaded class slots before measurement,
matching the store fixture and production loader. Baseline and candidate use
the same benchmark source. The inherited-base path retains every ancestor scan
and falls back to materializing the union if another parent adds slots. Changed
slot ownership/permissions use the existing full construction. Property
visitation retains both shape and per-value dependencies. No lifecycle order,
identity allocation, transaction publication or locking policy changes.

Measurement precedes the production edit. Both regular and anonymous lifetimes
are in scope, including explicit recycle and an automatic-anonymous collection
guard. Existing isolation and lifecycle boundaries remain the correctness
contract; shortening a benchmark by bypassing them is not an optimization.

## Surprises

The initial store benchmark committed snapshots without calling Release, which
retained history until finalizers ran. Those store samples and exploratory
profile magnitudes are rejected as performance proof. The benchmark now uses
explicit Release and mechanically rejects any nonzero ActiveReadTransactions
count at a cohort boundary. Both worktrees have the corrected benchmark.

The first relative profile-path invocation produced no profile files at the
requested root location. Go's command docs were checked; absolute profile paths
and an explicit test-binary output produced readable CPU and heap profiles.
The initial VM profile includes untimed fixture setup, so its totals are not
timed-region percentages. The PropertyValues allocation line is visible, but
its benefit must be judged by the setup-excluded paired benchmarks.

The paper fetch helper's Semantic Scholar request returned HTTP 429 and could
not resolve metadata. The retriever's open-repository fallback downloaded the
exact intended PDF; the title was checked on its first page and all pages were
rendered and inspected. Raw retrieval failure remains in the session output.

A search mistakenly named nonexistent `engine/gc_finalization.go` and returned
an IO error. That search supplies no evidence. Subsequent discovery returned
the actual `engine/deferred_gc_maintenance.go` and `engine/gc_handshake.go` files.

## Next step

Publish the authorized PR and clear its current-head CI, including managed
conformance and pinned Elle. Normal-merge when green, verify remote and local
integration, preserve canonical paper assets, and remove only this task's
worktrees and branches. No further implementation is planned. Publication and
normal merge are already authorized; do not ask Q again.
