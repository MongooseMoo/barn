# Startup resource lifetimes

## Goal

Validate startup-script input before server resources are constructed, release
builtin-session handles when listener startup fails, and hash profile inputs
with bounded memory in the existing checksum owner.

## State

This independent branch starts from origin/master. The completed candidate's
six Go files, three reports and two raw benchmark captures have moved from the
system-layering campaign without redesign. The source diff is 187 additions
and 15 deletions. All six transferred source diffs matched exactly, including
after formatting, and the owned campaign source files have no remaining diff.
The reports link the retained original/loop samples in this branch; rejected
candidate samples remain in the campaign's temporary evidence directory.
The campaign overview remains at
[refactor-system-layering.md](/C:/Users/Q/code/barn-system-layering/agent-notes/refactor-system-layering.md).
Publication and full PR CI remain with the coordinator.

## Decisions and why

Keep startup execution order and nil-versus-empty script behavior. Complete
the existing listener-failure cleanup after input/runtime/background work stops;
preserve panic's separate abort contract. Retain the measured fixed 32 KiB
checksum read loop and exact native error propagation. The io.Copy candidate
was rejected after its small-file regression.

## Surprises

The late script read could fail after profile/runtime creation. Listener failure
left real file and SQLite handles open. Hashing the whole database allocated a
file-sized temporary buffer; a generic streaming copy regressed the config path.

## Next step

The scoped move and direct source/report inspection are complete. Formatting
and diff checks passed. No test, benchmark or build was rerun for the identical
move; the reports retain the prior focused red/green and ten-sample evidence.
The coordinator must review, commit, publish and collect full PR CI.
