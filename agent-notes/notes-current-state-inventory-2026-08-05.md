# Current-state Barn issue inventory checkpoint

- Scope: read-only inventory from live GitHub issues and PRs plus current non-Markdown Go/source/test/config files. Repository Markdown and historical notes/plans are excluded.
- Current checkout: `master` at `aa82abf05a6a23783ba69105d0c393f926304a13`; pre-existing mixed/untracked worktree must be preserved.
- Live inventory: 77 open issues: #4-#19 (except #20), #21, #25-#32, #39, #40, #42-#45, #47-#77, #82-#93, #95, #97, #100.
- Open PR coverage: #81 closes #4; #94 closes #93; #96 closes #95; #98 closes #97; #99 closes #91; #101 closes #100. #94/#96 have green current checks; #98/#99/#101 are blocked by conformance sequencing; #81 is older and red on current conformance.
- Confirmed current-master examples: #4 still has `rand.Seed`; #15 still has overflowing `maxV-minV+1`; #54 still has slice `fmt.Printf`; #52 allocates requested file-read size; #39 still permits constant index 255 despite 0xFF sentinel; #7 still performs repeated `SetLocalByName`; #83/#84/#85 remain visible through `task.GetManager`, untyped `TaskContext` services, and global builtin state.
- Likely invalid/stale: #74 explicitly describes a readability trap, not a behavioral defect; current `CutPrefix` branch is behaviorally correct. Treat as cleanup, not bug. #31 is measured but root mechanism remains intentionally unestablished; keep open and do not merge it into transactional fixes without the A/B regression.
- Emerging clusters: task/scheduler concurrency and ownership (#30, #59, #62-#64, #76, #83/#84/#89); storage atomicity/history/durability (#5, #28/#29/#31/#45/#77, #90); parser/bytecode denial and encoding bounds (#25/#26/#39/#50/#51/#69/#70); stateful builtin ownership and async behavior (#4/#47/#53/#65/#66/#85); architecture dependency cleanup (#82-#92).
- Next action: complete subsystem-by-subsystem current code/test verification, identify duplicates and dependency order, then return a compact every-issue mapping with prioritized root-cause clusters.

