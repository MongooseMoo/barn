# Live campaign checkpoint 34

## PR #45 exact head accepted by analyst

- PR #45 exact head `5c5132bf8f6f563ee236a1249aa42677c3bda1e5` has analyst MERGE approval.
- Portable abort mapping is intentionally narrow: direct POSIX SIGABRT, wrapper 134, Windows CRT abort code 3. Windows fast-fail `0xC0000409` remains rejected as a different crash semantic.
- Per-launch log boundary, natural exit, payload ownership, byte provenance, traversal checks, local 308 units, and managed CI `31012325954` are accepted.
- PR remains draft/open/unmerged; adversarial exact-head review is active.

## PR #46 exact head under review

- PR #46 confinement head `055b993e206bedf887780e64d84dfa7fcadb2f75` is fully green in managed run `31012613429`.
- Ubuntu harness: 329 passed and one Windows-only junction skip; all Toast profiles and exact union green.
- Fresh hostile review is reproducing former suite/DB symlink and junction escapes plus all prior controller/runtime trust boundaries.

## Barn repair active

- Suspension transition is being redesigned around a task-locked requested/publishing state, buffered completion, and kill dominance.
- Shutdown producer admission must close at request and reject post-boundary execution.
- Startup finalizer order is pinned to Toast source: queue roots, disconnect hooks, server_started, anonymous LIFO, then WAIF.

## Next action

- Await PR #45 adversary and PR #46 hostile verdicts.
- Merge neither until both review and CI gates pass at stable exact heads.
- Continue Barn deterministic state-machine tests; no push until fresh analyst/adversary acceptance.

This file is a local untracked checkpoint only. Do not stage, commit, or push it.
