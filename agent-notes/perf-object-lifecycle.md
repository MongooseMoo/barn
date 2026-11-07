# Object creation and destruction performance

## Goal

Optimize creation and destruction of regular and anonymous objects in this
isolated worktree. Q also authorized opening a PR and merging after green CI.

## State

Q said "Make it so!". Implementation and publication through normal merge are
authorized. The branch was fast-forwarded to current master before editing.
Benchmark-only baseline added in the two planned packages; its one-iteration
fixture check completed with the expected counts, including automatic anonymous
collection and exactly-once inherited recycle hooks. Production code is still
unchanged. Repeated measurements and profiles are next. Branch:
`perf/object-lifecycle`.

Read `AGENTS.md`, `docs/workload-diagnostics.md`, `docs/elle-isolation.md`,
the Go performance and benchmarking-statistics skills, and the source below.
Processed Bonwick's 1994 allocator paper through paper-process's notes-only
workflow, inspecting all twelve page images. Paper relevance is medium:
separate construction cost from allocation cost and measure reclamation as well
as allocation; its kernel allocator results are not Barn results.

## Source findings

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

Measurement precedes the production edit. Both regular and anonymous lifetimes
are in scope, including explicit recycle and an automatic-anonymous collection
guard. Existing isolation and lifecycle boundaries remain the correctness
contract; shortening a benchmark by bypassing them is not an optimization.

## Surprises

The paper fetch helper's Semantic Scholar request returned HTTP 429 and could
not resolve metadata. The retriever's open-repository fallback downloaded the
exact intended PDF; the title was checked on its first page and all pages were
rendered and inspected. Raw retrieval failure remains in the session output.

A search mistakenly named nonexistent `engine/gc_finalization.go` and returned
an IO error. That search supplies no evidence. Subsequent discovery returned
the actual `engine/deferred_gc_maintenance.go` and `engine/gc_handshake.go` files.

## Next step

Capture repeated baseline measurements and profiles, then choose the smallest
production optimization they support. Publication and normal merge are already
authorized; do not ask Q to authorize those again.
