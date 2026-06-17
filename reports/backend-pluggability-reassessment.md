# Backend Pluggability Reassessment

Date: 2026-06-17

## Entry Criteria Check

Property mutation:
- Store-owned for production runtime paths.
- Remaining direct property writes are format readers, startup/load repair, `db.Store`, and tests.
- Server/builtins/VM remaining property access is read/traversal.

Verb mutation:
- Store-owned for production runtime paths.
- Remaining direct verb writes are format readers, `db.Store`, and test setup.

Object lifecycle and relationship mutation:
- Store-owned for create, recreate, recycle, move, parent changes, object assignment, and loader insertion/high-water bookkeeping.
- Remaining direct lifecycle/relationship writes are format readers, startup repair, `db.Store`, and tests.

Snapshot boundary:
- `server.checkpoint` is the checkpoint owner.
- `db.CheckpointManager` was deleted.
- Writer serialization now starts from a package-private `store.snapshot()` export instead of walking live store state throughout the dump.
- Reader/writer arg/prep conversions and dump-order property traversal no longer have duplicate helper surfaces.

## Backend Decision

Do not add a live backend interface now.

Reason:
- Runtime mutation ownership has converged on `db.Store`, but read/query access is still broad: builtins, VM, server matching, scheduler code, and command tooling still directly read object fields returned by `Store.Get`.
- A real transactional backend would need a complete read contract, not only a mutation contract.
- Adding an interface now would either be too wide to be useful or would become a shim around pointer-based reads.
- The current snapshot boundary solves the immediate persistence problem without introducing an adapter or compatibility layer.

## Recommended Architecture

Keep:
- In-memory `db.Store` as the live runtime owner.
- LambdaMOO/ToastStunt text snapshots as reader/writer persistence.
- `server.checkpoint` as the checkpoint orchestration owner.
- Package-private store snapshot export for writer consistency.

Next optional backend-shaped work:
- Add narrow store read/query methods only when replacing specific direct field reads.
- Add a derived index/query layer for introspection if a real caller needs it.
- Add alternate snapshot persistence only as reader/writer implementations after the text-format boundary stays clean.
- Consider a live backend interface only after direct field reads no longer define the runtime contract.

## Search Gates

`rg -n "obj\\.Properties\\[|delete\\(obj\\.Properties|prop\\.Value =|prop\\.Clear =|prop\\.Owner =|prop\\.Perms =" builtins vm server db`

Result:
- Runtime mutation hits are in `db.Store`.
- Non-store hits are format readers, startup/load-time code, tests, or read-only server matching/writer serialization.

`rg -n "obj\\.Verbs\\[|delete\\(obj\\.Verbs|obj\\.VerbList\\s*=|append\\(obj\\.VerbList" builtins server vm db`

Result:
- Runtime mutation hits are in `db.Store`.
- Non-store hits are format readers or tests.

`rg -n "obj\\.(Parents|Children|Location|Contents|AnonymousChildren|Anonymous|Recycled|Flags)\\s*=|append\\(.*\\.(Parents|Children|Contents|AnonymousChildren)" builtins server vm db`

Result:
- Runtime mutation hits are in `db.Store`.
- Non-store hits are format readers, startup repair, tests, or read-only traversal.

`rg -n "store\\.Get\\([^\\n]*\\)|GetUnsafe\\(|\\.snapshot\\(|NewWriter\\(|WriteDatabase\\(|CheckpointManager" builtins vm server db cmd`

Result:
- No `CheckpointManager` remains.
- Writer uses one package-private store snapshot at `WriteDatabase` start.
- Remaining direct store reads are broad enough that a live backend interface would be premature.

## Runtime Gates

Passed during Phase 4/5 execution:
- `go test ./db ./vm`
- `git diff --check`

Known unrelated baseline reproduced:
- `go test ./db ./server` fails only in `barn/server` `TestTLSListenerLoginAndEval`.

Commit:
- `d9a42a1 Record backend pluggability decision`
