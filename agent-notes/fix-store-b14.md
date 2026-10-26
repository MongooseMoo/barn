# fix/store-b14: history keeps the committed image at a commit timestamp

## Goal
B14 (db-store review): a coarse commit can supersede one object's published
image more than once at its commit ts T. Every history append skipped an entry
whose ts equalled the newest entry's ts, so history kept the FIRST (intermediate)
image at T, and a snapshot with readTS T read it once a later commit superseded
the final image. Q approved the smallest fix: a same-ts entry replaces the newest
entry. Store tests only, no conformance PR.

## State
Done on branch `fix/store-b14` (worktree `C:/Users/Q/code/barn-store-b14`), not
pushed. PR body drafted in the session scratchpad (`pr-b14-barn.md`).

## Decisions and why
- One helper `withHistoryEntry` (db/store/store_core.go) holds the rule; every
  history append calls it: rememberObjectLocked, republishForMutation
  (store_core.go), rememberOldImageLocked (store_cow.go, decentralized, under
  historyMu), SetVerbArgs (store_verbs.go). The prune sites in
  store_history_gc.go only shrink history and are unchanged.
- The replacement allocates a new slice instead of writing the last element in
  place: loadSnapshotObject (store_txn_snapshot.go) and the conflict census
  (store_conflict_census.go) capture the slice header under historyMu and walk it
  after unlocking.
- Replacing is right only if an image is stamped with T before it is superseded
  again. Every coarse site read (applyStagedToLiveLocked, define/delete property,
  Renumber's parent/children loop) republishes, mutates, then stamps before the
  next republish of the same object.

## Surprises
- Same-ts also reaches the decentralized path: a coarse commit leaves the newest
  entry at T, and a later decentralized commit superseding the final T image hit
  the same skip. Also Renumber (store_lifecycle.go) republishes an object that has
  oldID in both parents and children twice at one ts.

## Next step
Lead pushes/opens the PR. Wave 3b replaces the coarse applier later.
