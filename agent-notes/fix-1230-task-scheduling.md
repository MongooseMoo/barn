# Keep terminal tasks catalogued until execution ends

## Goal

Fix #1230 in manager and runtime cleanup.

## State

The common sweep checks settlement and removes a task under the same task lock. Runtime cleanup records diagnostics after releasing the catalog lock.

## Decisions and why

A killed or completed task can still be executing. Its unfinished index entry continues to hold its readier until the physical lease ends. Both cleanup entry points now use the existing settlement predicate.

## Surprises

The production input loop called the runtime sweep, which had a separate logical-state check with the same bug.

## Next step

Publish for review and CI.
