# Current checkpoint 26

- Subtask: harden moo-conformance-tests draft PR #45 for shutdown/GC edge evidence; no Barn edits and no PR #22 overlap.
- Branch/worktree: `agent/toast-shutdown-gc-edges` at `C:\Users\Q\code\.worktrees\moo-conformance-shutdown-gc-edges`.
- Implemented but not committed: canonical `wait_for_server_exit` action with timeout/exact code and no stop/kill; byte-exact `assert_file.equals_file`; schema/runner/server tests for success, timeout, wrong code, cleanup masking, and mismatch false positive.
- Managed WSL Toast generated deterministic replacements for all three startup fixtures after recipes explicitly added fixed `source` and `time` properties. New generated sizes are 2534, 2597, and 2637 bytes.
- Next: turn temporary probes into checked-in managed provenance suites and recipe DB inputs, replace fixed waits in behavioral suites, inspect the actual panic dump for exact suspended-task WAIF/anonymous serialization, add false-positive regressions, update checksums/workflow, run focused/full gates and the four-cell Toast matrix, then update draft PR #45 and issue #44.
- Managed oracle remains `/root/src/toaststunt/build-release/moo`; no manual server launch was used.
