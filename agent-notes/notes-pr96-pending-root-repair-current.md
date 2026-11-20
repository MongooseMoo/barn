# PR 96 pending-root repair checkpoint

## State

- Clean repair worktree: `C:\Users\Q\code\.worktrees\barn-pr96-pending-roots-repair`
- Temporary branch: `agent/pr96-pending-roots-repair`
- Current local repair commit: `ff95ff75b7da18ee5cb077a4c26c42f73e42f2b8`
- No push and no Barn or Toast process launch.

## Findings

- VM collection must retain a lossless deterministic candidate set and must not decide persistence from the mutable Store graph.
- Snapshot's held Store read lock is the correct linearization point for filtering candidates already reachable from persistent properties and for canonicalizing graph-redundant roots.
- The frozen normalized pending set must drive both snapshot `PendingFinalizations` and anonymous serialization seeds without mutating the live candidate queue.
- Current VM collection covers frame locals and active operand stack only. It misses frame `ThisValue`, `Args`, `SavedThisValue`, structured `VMException.Value`, TaskContext value fields, pending value storage, and yielded/fork payload values.

## Red evidence

Focused `go test ./vm -run 'TestCollectPendingFinalizationValues' -count=1` currently fails as intended:

- currently persistent candidate remains pending;
- cycle produces two pending roots instead of one canonical root;
- staged persistent edge addition remains pending after snapshot;
- live-field test captures only local and stack roots (`4` and `9`) instead of roots `4` through `17`.

The existing staged edge-removal regression remains in place and expects both roots to survive when the edge is removed before snapshot.

## Next action

Implement complete VM candidate collection, then normalize/filter pending candidates and build the anonymous serialization plan together under the existing `Store.Snapshot` lock. Re-run focused repeat/race, cycle round-trip, full `go test ./... -count=1`, `go build ./...`, and diff checks before committing locally.

## Implementation update

- VM candidate collection now includes frame locals, `ThisValue`, `Args`, `SavedThisValue`, structured pending exception values, active stack values, TaskContext value fields, pending value storage, and yielded/fork payload values.
- Snapshot planning now computes persistent reachability and normalizes the lossless pending candidate queue while holding the existing Store read lock.
- Persistent candidates are excluded from pending output; unreachable graph-redundant candidates are canonicalized deterministically; the normalized roots seed serialization and are returned in the snapshot without changing the live queue.
- Focused `go test ./vm -run 'TestCollectPendingFinalizationValues' -count=1` is green.

Next: run focused high-count and race tests, existing store and dump/reload coverage, then the full Go test/build and final diff audit.

## Final verification update

- Snapshot now takes the Store's exclusive lock. This is required because decentralized commits publish under `s.mu.RLock`; retaining the old read lock would not freeze the graph at the persistence linearization point.
- Added composite live-queue normalization coverage and an acyclic root/leaf frozen-graph canonicalization assertion.
- Focused VM tests passed 50 repetitions and 10 race-detector repetitions.
- Focused Store tests passed 50 repetitions and 10 race-detector repetitions.
- Existing pending anonymous cycle dump/reload passed 20 repetitions and 5 race-detector repetitions.
- `go test ./... -count=1` passed after the final changes.
- `go build ./...` and `git diff --check` passed.

No blocker remains. Next action: audit the four intended paths, stage them explicitly, commit on the temporary branch, and report the exact commit without pushing.
