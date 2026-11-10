# fix/keyed-map-reads-at-boundary

## Goal

Stop the irreversible-effect boundary (`CommitAndRenewCarryingReads`) from
turning keyed map dependencies (`obj.prop[k]`, `obj.prop[k] = v`) into whole
reads at the version first seen. In Mongoose every `say` does a keyed read and
write of `#68987.state[who]` ($away:stored) and then reaches an inline
`sqlite_execute`; the degrade made it lose whenever any other player's entry
was committed in between (1,026 whole-property losses on `#68987.state` in one
64-player run, read site `stored@6`).

## State

Done on the branch, not pushed:

- `db/store/store_txn_mapkeys.go`: `degradeMapKey` (one slot; `degradeMapKeys`
  now loops over it, diag call unchanged), `boundaryMapKeys` (before
  validation), `carryBoundaryMapKeys` (on the renewed transaction).
- `db/store/store_txn_commit.go`: the boundary calls `boundaryMapKeys` instead
  of `degradeMapKeys`, then `next.carryBoundaryMapKeys` after the renew.
- Tests in `db/store/store_txn_mapkeys_test.go` (`TestMapKeyBoundary*`), and
  `TestBoundaryRenewDegradesKeyedReadsOnRepublishedMap` renamed to
  `TestBoundaryRenewCarriesKeyedReadsOnRepublishedMap`: its internal-state
  assertion pinned the old whole-read carry; its behaviour checks (own
  publication commits, later change to the read entry loses) are unchanged.

## Decisions and why

- At the boundary the keyed state stays on the transaction, so
  `validateReads` checks it per entry (`validateMapKeysLocked`) and `Commit`
  rebases keyed writes onto the live map (`rebaseMapKeyWritesLocked`), exactly
  as an ordinary commit does.
- The renewed transaction carries each keyed slot keyed: the same entries,
  with values re-based to what the boundary published (an entry this
  transaction wrote takes its written value; any other entry keeps the value
  the boundary just validated as live), and owner, perms, version and baseline
  taken from the renewed snapshot, which is the map the boundary published.
  Serializable because: under the exclusive gate the boundary is a commit at
  timestamp ts whose reads were validated per entry against the live store,
  so the slice's first half is equivalent to a serial execution at ts. The
  carried entries are exactly what a serial execution at ts reads at those
  keys, so the final commit failing iff one of those entries (or the slot's
  owner/perms/map-ness) changed after ts is the same per-entry check an
  ordinary keyed commit makes. Carrying the old whole read was stricter than
  needed; carrying nothing would stop watching the entries if a live mutation
  lands after the boundary, which the existing carry exists to catch.
- A slot with a keyed assignment in flight (`hasPendingBase`: base fetched
  before the boundary, writeback after) degrades to a whole read at the
  boundary. Its writeback on the renewed transaction finds no pending base and
  becomes a whole write built on the pre-boundary map; kept keyed, that write
  would overwrite entries other commits changed before the boundary. A whole
  read makes the boundary lose in that case, as on master.
- A carried slot that is no longer a map on the renewed snapshot is read whole
  at its current version; one that is gone drops out (as the footprint carry
  does for removed properties).

## Surprises

- The pending-base hazard above (not in the diagnosis).
- `TestElleIsolationHistories` skips even with `-tags elle` (pre-existing).

## Next step

Lead: push, PR, CI; then measure the 64p census for `#68987.state` losses.
Another agent is editing diag code near `degradeMapKeys`; its diag call moved
into `degradeMapKey` verbatim, so a merge may need that hunk re-placed.
