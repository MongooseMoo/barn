# #525 loaded WAIF registration

## Goal

Retain decoded WAIF definitions until Store registration, releasing only resolved property-index scratch maps. Smallest change: approximately 5–10 production lines in reader_helpers.go and reader.go comments; extend existing checkpoint WAIF coverage for registry counts, aliases and repeated Store construction. No new identity/cache machinery.

## State

Worktree starts at verified origin/master c537d6c952791b2fe49bcb9d7cba90102767a36f. resolveWaifProperties now retains definitions and clears only propsByIndex scratch maps. RegisterWaif's existing identity deduplication and cleanup remain unchanged. Extended two existing tests for alias deduplication and repeated Store construction, covering valid and absent classes.

Focused command: `go test ./db/format -run '^(TestWriteCheckpointPreservesWaifIdentityWithoutChangingPortableDump|TestCheckpointKeepsSuspendedTaskWaifAndAnonymousRootsTaskOwned)$' -count=1`.
Before repair: `store 0 WAIF count = 0, want 2 distinct definitions`; `store 0 class #9 WAIF count = 0, want 1`.
After repair: `ok github.com/MongooseMoo/barn/db/format 1.232s`. No full CI or server/oracle run. Diff inspected: 28 insertions, 6 deletions across four Go files; only two production files changed.

## Decisions and why

Keep definitions on Database across repeated Store construction; clear each propsByIndex map during resolution, including missing-class records. #522 owns a separate anonymous-index field/cleanup and does not overlap these lines. #526 will use a separate branch for header matching.

## Surprises

Existing checkpoint identity coverage uses absent classes, so it also exercises retention when property resolution cannot find a class. Existing task-owned WAIF coverage supplies a valid class and resolved property.

## Next step

Parent reviews/publishes and reads PR CI. This worker proceeds independently to #526 in another worktree. Preserve #522's separate anonymous-index cleanup when merging branches.

## PR draft

Title: fix: retain decoded WAIF definitions for Store registration

Body: Loaded WAIFs lost their registry entries because property resolution discarded the definitions before Store construction. Retain those definitions and release only indexed-property scratch maps, so every Store constructed from the loaded Database registers each distinct WAIF. Extend existing checkpoint tests for aliases, repeated construction, and valid/missing classes.

Fixes #525. Validation: focused checkpoint WAIF tests failed with zero registry counts before the repair and passed after it. Full checks belong to PR CI.
