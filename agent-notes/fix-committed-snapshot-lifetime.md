# Committed snapshot lifetime

## Goal

Release the final attempt's old transaction before committed-effect replay,
without changing effect order, gate ownership, physical execution or admission.

## State

The real-VM blocking-notification regression captured RED with one registered
reader pinning floor 2 behind clock 3, after write_value=42 is published. Moving
the existing Release before final replay produces GREEN: zero readers and floor
equal to clock while output remains blocked; completion still waits for replay.
Raw comparison is in reports/system-layering-committed-snapshot-coder.md.

## Decisions and why

Move the existing release once; create the fresh cleanup snapshot after replay
as before. Do not change irreversible-boundary or finalization replay sites.
Use a real MOO task and channel-blocked connection to observe retention directly.

## Surprises

The initial compile probe caught an unused test import, corrected before the
behavioral RED run. It is not counted as a regression witness.
Release deregisters the timestamp; it does not clear cached object references.
The source comment and claims describe history retention only.

## Next step

Inspect the final scoped diff and hand it to the parent for independent review
and exact-head PR CI. No broad local tests, publication or merge. Pinned Elle
evidence remains the PR workflow's job.
