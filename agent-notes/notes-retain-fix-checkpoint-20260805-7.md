# Retain/fix checkpoint

- Draft PR 112 corrects the single default-profile CI command. Its run completed without the old fixture/connection cascade: 11,158 passed, 410 declared skipped, 17 genuine failures.
- The 17 failures are eight bcrypt `$2x$`, three transactional anonymous-GC cases, and six anonymous startup-repair output cases.
- PR 109 exact head `f6bfdc4` fixes raw-dollar parsing and genuine `$2x$`; managed Toast and Barn focused runs each pass admission plus eight cases. Independent review is active.
- New issue 113 tracks the GC root cause. Local clean commit `7db7f0f` adds transaction-view anonymous reachability and passes managed Toast/Barn focused cases plus Go gates; it is not pushed yet.
- Managed WSL Toast passes admission plus all six startup-repair anonymous DB cases when the required `--server-db-dir` is supplied. Barn default-profile CI fails those six because the expected `.db.new` files are absent.
- Conformance managed-lifecycle repair is in progress in a separate worktree and remains unpushed.
- Next: independently review GC, create its narrow PR, assign the startup-repair cluster, finish/review lifecycle PR, then resolve the green merge dependency without weakening the gate.
