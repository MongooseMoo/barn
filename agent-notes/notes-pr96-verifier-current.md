# PR 96 verifier checkpoint

## Current state

- Review target: draft PR 96 at exact head `0450cf22c84d0c3ebdb96a9172b6072db1a7f56a`, base `aa82abf05a6a23783ba69105d0c393f926304a13`.
- Exact-head branch worktree: `C:/Users/Q/code/.worktrees/barn-baseline`.
- The worktree has concurrent uncommitted changes in `db/store/store_pending.go`, `db/store/store_test.go`, `vm/anonymous_gc.go`, and `vm/anonymous_gc_test.go`; four overlap PR-owned paths. They must not be modified or used as exact-head runtime evidence.
- Exact-head inspection is therefore using immutable Git object reads (`git diff base..head` and `git show head:path`).
- GitHub `Build & unit tests` passed. `MOO conformance suite` failed. The PR is open, draft, mergeable, and `UNSTABLE`.

## Inspected scope

- PR files: `vm/anonymous_gc.go`, `vm/anonymous_gc_test.go`, `db/store/store_snapshot.go`, `db/store/store_test.go`, and `db/format/dump_persistence_test.go`.
- Issue 95 requires cyclic pending-finalization survival through shutdown snapshot, focused pinned conformance, full Go test/build, and full managed conformance.
- The implementation now captures bare anonymous VM locals/stack values, derives deterministic component roots, seeds serialization reachability from pending values, rewrites pending references, and emits reachable anonymous objects.

## Current observations

- Cycle roots are ordered by descending reachable-closure size, then ascending anonymous identity; already covered candidates are skipped.
- Snapshot serialization clones pending values, includes them as anonymous reachability roots, rewrites their references through the same plan, and emits anonymous objects in sorted identity order with above-max serialization IDs.
- The new persistence test constructs a two-object cycle, queues one root, writes/reloads, and checks the pending root, both anonymous objects, and both cycle edges.

## Blocker and next action

- No code verdict yet. Exact-head local tests cannot safely run from the dirty shared worktree without disturbing concurrent edits.
- Next: inspect exact-head helper implementations and writer/reader pending-finalization paths, test deterministic-root correctness on overlapping closures/disconnected components, assess snapshot ID collision/reload invariants and transaction/state safety, then inspect the exact CI failure and decide MERGE/NO-MERGE.
