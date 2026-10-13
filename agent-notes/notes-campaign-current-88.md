# Current campaign state

- PR 106 exact head `cc2407b7f38df988a6ece89abcb2e5b15adedd10` is running managed pair `31066742002` against conformance `acc2718cbc3093c7527448faec0f6c162dac9fe6`.
- All three Toast profiles passed and the run is executing every packaged case against Barn. The strict expected set is 12 identities: eight genuine 2x bcrypt cases, three GC cases, and one pending-finalization case.
- PR 106 build/unit CI, focused race, full Go tests, build, formatting, and diff gates are green. Legacy conformance remains red and is separately tracked by conformance issue 18. Existing unrelated `go vet` warning remains at `vm/stack.go:49`.
- PR 96 implementation has two commits, `34aae2b` and `3a70158`, spanning a coherent 22-file lifecycle boundary. It is unpushed and under two independent exact-head gates: concurrency/edge-case analysis and architecture adversary review.
- PR 96 worker reports lossless VM capture, exclusive snapshot normalization, atomic lifecycle admission/reservation, deferred-GC ownership, task-owned serialization roots, asynchronous reentrant shutdown, and no forbidden startup/suspension/task-manager/builtin scope.
- Conformance issue 18 reusable paired-workflow implementation is active in a new isolated worktree from exact conformance main. It must preserve repository_dispatch red/green mode while adding strict expected-success workflow_call mode.
- Next actions: finish PR 106 verdict and normal-merge if exact; retarget/update PR 108; resolve any PR 96 analyst/adversary blockers before push; review issue 18 workflow commits before opening a PR.

