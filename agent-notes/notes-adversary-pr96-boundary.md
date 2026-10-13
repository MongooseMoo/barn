# Barn PR #96 architectural adversary gate

- Target: `C:/Users/Q/code/.worktrees/barn-pr96-current-integration`, exact HEAD `3a7015848ccb0803dc14d33f861d24e26c3ecf0a`.
- Scope observed: current source, current tests, and `origin/master...HEAD` diff only; no historical project material or external actions.
- Current observations: store snapshot is the graph linearization point; task-owned queued/suspended roots are passed as serialization-only roots; shutdown publication accounts for admitted producers, deferred GC, and sink completion; lifecycle hooks run before checkpoint writing.
- Open architectural concern: verify whether shutdown caller-root capture plus terminal task handoff creates two owners for the same value, masked by store-level de-duplication.
- Blocker: none yet; review is still in progress.
- Next action: trace caller shutdown ownership and inspect exact production/test evidence for compatibility shims and prohibited scope expansion, then return APPROVE or BLOCK.
