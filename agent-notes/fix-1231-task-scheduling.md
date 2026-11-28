# Deliver the result captured by the completing slice

## Goal

Fix #1231 in the completion callback shared by runtime execution paths.

## State

The callback and caller now consume the same result captured before the slice releases the execution lease.

## Decisions and why

The runtime already captures the result at the physical handoff. Deliver that value through the callback it already owns.

## Surprises

The return path already used the capture. Only callback delivery bypassed it.

## Next step

Publish for review and CI.
