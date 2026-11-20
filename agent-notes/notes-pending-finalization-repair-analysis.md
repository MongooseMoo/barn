# Pending-finalization repair analyst notes

- Review target: `C:\Users\Q\code\.worktrees\barn-pr96-pending-roots-repair`, exact HEAD `6f1022f1c6131a7e5ef6197499f211c3c67e0723`.
- Comparison base: rejected `25d689e`; 21 changed paths, 633 insertions and 62 deletions.
- Worktree state at start: only `?? reports/runs/`; preserve it and the external junction. No edits, commits, staging, pushes, or manual Barn/Toast launches.
- Protocol state: requested `C:\Users\Q\.Codex\AGENTS.md.d\gauntlet.md` does not exist. Read the installed `protocols:gauntlet` skill; analyst contract is defect-finding only.
- Current observations: repair moves task-local storage behind `Task.GetTaskLocal`/`SetTaskLocal`, introduces shutdown-phase pending-finalization handoff, recursively traverses WAIF graphs with identity guards, and rewrites pending anonymous IDs during snapshots.
- Confirmed blocker: every popped-frame WAIF is copied into both `PendingWaifs` and `PendingFinalizations`; ordinary completion clears only the first, then its own-VM liveness scan sees the second and suppresses `:recycle`.
- Confirmed blocker: `ProcessReadyTasks` flushes deferred GC after a joined pass even after `BeginShutdown`; GC liveness excludes the store pending-finalization queue, so an older deferred request can recycle a just-handed-off shutdown root before checkpoint.
- Confirmed correctness defect: a pending WAIF containing an anonymous object becomes both a WAIF root and a separate bare anonymous root because VM collection traverses WAIF properties while store normalization deliberately stops direct-anon collection at WAIF boundaries.
- Coverage gap: new shutdown tests cover a bare anonymous root/cycle and a bare WAIF, but not WAIF-to-ANON nesting, ordinary WAIF recycle, an older deferred batch crossing shutdown, or real runtime-created anonymous identities. Many VM tests use numbered anonymous objects installed through `Store.Add`.
- Writer identity audit: `map[unsafe.Pointer]int` correctly distinguishes two live WAIF instances during a writer's lifetime; the `types.Value` snapshot roots keep payloads live and current Go pointer identity remains stable. No writer-identity blocker found.
- Verification: focused packages green; focused repeat run green 25 times; `go test -race ./vm ./db/store ./db/format ./scheduler` green; `go test ./...` green. These tests do not cover the confirmed lifecycle sequences.
- Blocker: repair is not mergeable with the two lifecycle blockers above.
- Next action: report exact line evidence and test results to the parent; managed conformance was not run because source-level proof is sufficient and the requested immediate verdict takes priority.
