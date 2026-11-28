# Use the scheduler heap key to scan waiting tasks

## Goal

Fix #1232 when the start and wake timestamps differ.

## State

The readiness scan stops at the same key the heap uses. It checks the full deadline before selecting tasks from the heap or pending queue.

## Decisions and why

A due ordering key only means a task must be considered. A later start may still hold that task even when a sibling behind it is ready. A future wake key keeps that task in the heap.

## Surprises

The deadline accessor can return a future time, which previously satisfied the scan's nonzero check.

## Next step

Publish for review and CI.
