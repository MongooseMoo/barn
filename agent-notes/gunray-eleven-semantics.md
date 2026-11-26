# Gunray builtin semantics

## Goal

Fix the eleven new builtin discrepancies from the Gunray replay.

## Decisions

- ANSI conversion selects a color for each random tag using Toast's palette. Removal operates on recognized tags, so an already expanded escape remains in the output.
- Numeric conversion accepts signed hexadecimal significands with explicit or implicit binary exponents and whitespace after the object marker.
- Embedded-types JSON protects annotated-looking strings in both map keys and values, annotates boolean keys, reads tagged booleans, and uses the existing decimal-prefix conversion for tagged integers.
- MOO pattern compilation validates the start anchor at branch boundaries and matches the start and end of lines. Directional assertions and numbered backreferences use the existing bounded regexp2 dependency. Ordinary patterns use RE2. The bounded matcher maps aborts to E_QUOTA, the result from Toast's do_match. The rightmost wrapper keeps captures numbered as in the source pattern.

## Surprises

Toast's random tag can reach five distinct codes because two entries in its six-entry palette are aliases. The existing reverse-anchor behavior reported in Barn PR #123 remains outside this task. Toast's legacy regexp engine and Go also differ in some other boundary and byte-indexing cases. The bounded path preserves Barn's ASCII boundary behavior for %b and %B and converts engine offsets to Barn character positions.

## State

Baseline Go regressions reproduced the eleven discrepancies. The initial patched managed replay passed 22 cases, including capability admission and all eleven in scope. The only failure was gunray_ten_replay_reverse_anchor. The selected packaged compatibility run passed 248 cases. The full replay outputs are in WSL at /tmp/barn-gunray-eleven-worker.9yCMJL7T.

## Next step

Run the paired companion conformance suite at the published heads.
