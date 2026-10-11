Issue 272: defer runtime variable maps and traceback allocation on raises.
Authority: TDD implementation and PR in Barn; no merge requested.
Read issue and repository guidance; no matching open PR; initial tracked changes zero.
Toast oracle verification required before Barn diagnosis; locating managed command, Debian installed.
Next: inspect managed wrapper and conformance coverage, verify Toast option behavior.

Managed Toast wrapper inspected: scripts/run_toast_wsl.sh; canonical executable exists in Debian (test -x exit 0).
Conformance checkout exists at C:\Users\Q\code\moo-conformance-tests; read-only use only.
CLI selector normalizer inspected: requires _tests marker and retains relative YAML suffix.
Existing suites: language/error_traceback.yaml, builtins/task_stack_call_shapes.yaml, audit/task_scheduling_toast_oracle.yaml.
Next: finish managed CLI setup and admission selection, run Toast behavior verification before Barn diagnosis.

Managed WSL Toast regression baseline: 85 passed in 5.96s (exit 0), traceback and task_stack_call_shapes plus admission.
Log: C:\Users\Q\AppData\Local\Temp\tmpapsm03.tmp. Existing conformance .venv blocked uv; separate UV_PROJECT_ENVIRONMENT under TEMP/barn-272-conformance-windows works.
Toast src/include/server.h defines include_rt_vars flag default 0; execute.cc gates rt map creation on include_variables and uses option for error stacks.
No product edits yet. Current Barn branch master; fetched origin master successfully.
Next: resolve fetched base, create isolated issue worktree, inspect VM/task stack and 16p harness, write red regression.

Created isolated worktree C:/Users/Q/code/barn-issue-272 on issue-272-lazy-error-stacks, from origin/master 22a6e6199ce57fb207e8d5387153c7a82b20cf3d.
VM HandleError currently eagerly builds exception/traceback before handler search; Run snapshots activation frames before calling HandleError.
Read complete HandleError; finally and recycle continuations need careful preservation. syncTaskLineNumbers eagerly builds all local maps.
No implementation or regression edits yet; tracing task_stack lazy variable ownership next.
Known verified paths in worktree include vm/vm.go, vm/traceback.go, task/task.go, builtins/tasks.go, kernel/context.go; see discovery output for exact mixed-separator literals.

Task stack rejects current running task; other-task stack reads use copied ActivationFrames with RuntimeVariables maps.
Considering immutable raw local snapshots at suspension, map materialization only for requesting consumer; must account for value roots/persistence.
Runtime option cache owner is builtins/limits.go Session serverOptions using kernel.PendingServerOptions; include_rt_vars not currently present.
16p harness owner engine/mongoose_real_bench_test.go; input C:\Users\Q\code\barn\mongoose.db.new exists.
Still no code edits; next finish option/cache and snapshot ownership then implement TDD regression.

Red regression TestLineSyncDoesNotMaterializeRuntimeVariables failed with intended materialized-map diagnostic (go test ./vm focused).
Added vm/lazy_runtime_variables_test.go and types/runtime_variables.go, immediately inspected each.
Partial implementation: ActivationFrame now points to immutable RuntimeVariableSnapshot; Task setter renamed to install snapshots; syncTaskLineNumbers only lines.
VM now captures raw snapshots at yield; types RuntimeVariableMap materializes map on demand, fallback supports restored eager maps.
Next: wire task_stack consumer and GC roots, add preservation tests, option cache and lazy exception/snapshot handling; no green gate or commit yet.

Line-sync regression green. task_stack now calls RuntimeVariableMap; value_roots visits raw snapshot values.
Second red test initially malformed: VM.CurrentFrame returns cached vm.frame, manual Frames slice alone did not initialize it.
Rule 0 durable repair installed in worktree AGENTS.md and committed standalone e58a05c10c2f3d573e3da0807451589e045889b7; diff-tree inventory is only AGENTS.md.
Fixture repaired with inspected pushFrame helper and CurrentFrame assertion; rerun now fails only intended unobserved-value diagnostics for except and finally.
Next: complete raw scope/guard verification if needed, implement errorObservation prepass and defer HandleError traceback and Run uncaught snapshots. Option cache remains outstanding.

Added vm/error_observation.go; prepass recognizes unnamed except/finally and conservatively treats recycle continuations as observing.
HandleError constructs exception/trace only when observed; Run activation snapshot only when prepass says uncaught. Focused VM regressions green.
Added IncludeRTVars fields to cached serverOptionsState and PendingServerOptions; red builtins TestIncludeRTVarsCacheRefresh proved option not loaded.
Implemented limits.go getter, snapshot apply/reset and property collection for include_rt_vars. Not yet rerun this gate.
Next: wire traceback 7th map field, add option/locals preservation regressions, format focused checks, measure 16p baseline/candidate and conformance, then commit/PR. No PR yet.

Traceback option regression red (6 fields wanted 7), then green after buildTraceback materializes snapshots/maps only when IncludeRTVars(ctx) true.
Getter respects task-local PendingServerOptions before committed cache; focused VM/builtins/task-stack tests green.
Added snapshot-survives-resume regression; gofmt applied to all candidate Go files; scoped tracked diff --check exit 0.
16p baseline ran on original exact parent 22a6e6199ce57fb207e8d5387153c7a82b20cf3d, BARN_MONGOOSE_BENCH=1 DB original mongoose.db.new PLAYERS=16. Process exited 0.
Baseline log C:\Users\Q\AppData\Local\Temp\tmptfhrdu.tmp not yet read; next bounded summary, candidate benchmark, complete tests and managed conformance; commit/PR outstanding.

16p baseline bytes/op=54847400; candidate=40635770; measured reduction=25.91%, both process exits 0. Candidate log C:\Users\Q\AppData\Local\Temp\tmpo2vqj1.tmp.
Added vm/testdata/lazy_error_stacks.yaml with enable/disable option and bound-local checks; immediately inspected file.
Oracle invocation with --moo-suite-root failed setup exit4: requires independently supplied --candidate-root; log C:\Users\Q\AppData\Local\Temp\tmpxp2lqn.tmp.
Rule 0 active: classify as global CLI dependent-option setup; identified global C:\Users\Q\.codex\AGENTS.md exists, durable guard still must be installed before correcting invocation.
Next: install global guard, read dependent option validation/help, rerun Toast row and verify repairs; then Barn build/conformance/full Go gate and PR.

CLI setup Rule 0 repaired: appended two narrowly scoped global guards in C:\Users\Q\.codex\AGENTS.md (not Git tracked); raw count2 global, count0 repo.
Inspected harness root/default DB validation. Copied canned Test.db to isolated .tmp/Test-272.db, immediately inspected header.
Corrected managed Toast option row invocation uses --candidate-root worktree, --moo-suite-root vm/testdata, --server-db .tmp/Test-272.db plus admission.
Toast option row now exit0, 2 passed in1.29s; log C:\Users\Q\AppData\Local\Temp\tmpxp2lqn.tmp. Both durable and immediate repair verified.
Next resume delivery: build Barn canonical cmd/barn, run same option and existing trace/task-stack conformance, full Go suite, review scoped diff/commit, fetch base, publish PR. Performance gate already25.91% bytes/op reduction.

Built .tmp/barn-272.exe via verified ./cmd/barn; binary exists, size22990336, PE header77,90.
Barn managed option row exit0 2passed; existing traceback/task-stack suites exit0 85passed. Log C:\Users\Q\AppData\Local\Temp\tmpxkt0vq.tmp.
Final review found uncaught hook engine/task_runtime.go constructs stack frames via ToList(), so include_rt_vars still needs uncaught hook coverage/integration.
VM snapshotActivationFrames in vm/traceback.go builds ActivationFrame from raw frames and currently omits variable snapshots. Read full function, currently lines72-122.
Next: add uncaught runtime-variable regression and wire optional snapshot/hook serialization, rerun relevant checks (performance default path may need remeasurement), full Go gate and PR outstanding.

Uncaught snapshot red test added, then implementation captures optional raw locals and engine hook includes requested map field; focused VM/builtins/engine tests green.
Reviewed candidate diffs, staged exact15 paths, cached whitespace check exit0 and candidate inventory equality true.
Committed product a91e756 (15files), separate prior AGENTS commit e58a05c. No PR yet.
Full Go suite next on committed candidate. Created empty full-suite log C:\Users\Q\AppData\Local\Temp\tmp3uh3wr.tmp; must inspect it directly immediately after this mandatory checkpoint.
Remaining: full suite, rebuild final binary and rerun affected conformance/perf after uncaught change, refresh base, publish PR. Earlier performance25.91% applies before final uncaught integration.

Full go test ./... on a91e756 exited0; log C:\Users\Q\AppData\Local\Temp\tmp3uh3wr.tmp. Final binary built and final option2/trace-stack85 conformance passed.
Final16p candidate bytes/op40244295 vs parent54847400 =26.62% reduction, exit0; log C:\Users\Q\AppData\Local\Temp\tmpph2zsn.tmp.
Refetched origin/master remains22a6e619; tracked worktree clean. Verified push_repository=MongooseMoo/barn same_repository=True; branch pushed successfully.
Reporting Rule0 repaired by global multi-claim output guard; raw guardcount1 plus both final conformance summaries emitted.
Next immediate: author/read .tmp/pr-272.md (absence verified), gh pr create for issue-272-lazy-error-stacks targeting master, verify live ownership/body/head/URL, final response. No merge authorized.
