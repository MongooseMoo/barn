# PR 45 adversarial review checkpoint

- Scope: adversarial review of moo-conformance-tests PR #45 at exact head `5c5132bf8f6f563ee236a1249aa42677c3bda1e5`.
- Current findings: exact head and CI run `31012325954` are green, including 3/3 managed pinned-Toast fixture regeneration on every profile. The recipe inputs begin with zero pending roots/tasks and use embedded startup code to produce byte-exact checked-in outputs; this provenance gate is substantively sound.
- State: exact-head verdict is `NO-MERGE`.
- Blockers: POSIX status `134` is accepted as abort without proving signal provenance, so ordinary `exit(134)` is a false positive; a successful `wait_for_server_exit` disconnects and then unconditional cleanup/test teardown attempts to reconnect to the dead server. The current tests cover timeout cleanup only.
- Additional gaps: `assert_file` performs realpath validation before pathname reopen, leaving a symlink-swap gap; it has no output freshness/stability boundary, so persistent managed temp directories permit stale identical files and partial-file races. Panic payload checks use exact fragments but do not structurally bind them to the suspended-task section. PR #22 overlaps runner/schema dispatch and must be reconciled and reverified after #45.
- Next action: remove ambiguous positive POSIX wrapper normalization or add trustworthy provenance; define successful-exit cleanup/teardown semantics and test them; harden file assertions against symlink, stale, and partial output; then rerun the exact managed Toast matrix and independent review.
