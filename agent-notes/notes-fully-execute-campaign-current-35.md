# Campaign checkpoint 35

- Draft PR `MongooseMoo/moo-conformance-tests#45` remains open, draft, and unmerged; working head is still `5c5132b` with uncommitted adversary repairs in the isolated worktree.
- Abort normalization now rejects ambiguous positive 134 and accepts only direct POSIX negative SIGABRT or Windows CRT status 3.
- Natural exit is terminal at schema and runtime: only log/file evidence may follow; cleanup and test/suite teardown do not reconnect; non-evidence runtime steps fail before delay; the next suite requires a fresh managed launch. Server-dependent requirements now force a listening launch before admission.
- `assert_file` now uses a single validated descriptor, rejects traversal and link/reparse components/swaps, compares descriptor/path identity after open, requires stable fstat/read, and rejects files unchanged from the exact launch/test snapshot.
- Panic evidence now parses the one suspended-task section, the one activation `state` variable, WAIF class/owner, anonymous reference, and anonymous object owner; relocated state serialization fails.
- Current proof: focused units 123 passed before latest two regressions; full units 328 passed; collection 11,593; duplicate baseline 493 YAML / 4,525 tests exact; Ruff/mypy/checksums/diff green; managed WSL Toast provenance plus three shutdown/GC behaviors 6 passed with the structural parser.
