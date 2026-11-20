# bench_players.py settle default

## Goal
Q 2026-11-03 ("Make it so: A and B", item A): make scripts/bench_players.py
measure a settled server by default (--settle 2 -> 60 s).

## Why
Toast's 1-player level read 2.4-4.0/s on today's mongoose.db.new: its
server.log showed the level logged in 2 s after Toast began listening, so
the window was 4-14 s after boot, during the database's catch-up work on
Toast's single thread. With --settle 60: Toast 1p 64.2, 69.6, 71.4/s (p99
40-44 ms); Barn 1p 52.2, 51.5/s. 16p: Barn 287-289/s, Toast 61-64/s.
Details in agent-notes/mongoose-workload-gap.md.

## State
Default and module docstring changed. PR open; merge when CI is green.

## Next step
None after merge. scripts/bench_mongoose_matrix.py also defaults to
--settle 2; not changed (not asked).
