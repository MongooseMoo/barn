2026-09-17: User waived live multiplayer validation; optimizing protected builtin dispatch in barn-perf-protected on perf/protected-builtin-dispatch.
Managed Toast oracle confirmed same task/caller, suspend return, exception value, and non-executable wrapper fallback (5 tests including admission).
CPU profile identified nested runtime tasks; added native VM wrapper dispatch and regression tests, which reproduced task/caller and non-executable differences before changes.
Engine and VM suites pass; builtin characterization fixture lacks execute permission and needs correction. Ten-sample before/after benchmarks collected/in progress.
Next: fix fixture, test suspension and full suites, compare benchmarks, document bounded performance evidence. PR307 remains unqueued with its completion-boundary finding open.

2026-09-17 checkpoint: Native wrapper dispatch and regression tests are implemented; managed Barn matches all four oracle probes plus admission.
Full Linux go vet and go test completed; WSL tests require GIT_DIR/GIT_WORK_TREE because Windows worktree gitdir paths do not translate automatically.
Initial ten-sample benchmark: 8.685ms to2.107ms per1000 calls, 88.47% fewer bytes and71.33% fewer allocations. Corpus protected row748.46ms to238.84ms, all17 values match.
Pinned sequential timing was noisy (control doubled, optimized spread288%); retain this evidence and avoid claiming a stable timing factor. Allocation reduction held.
Next: interleave measurements to assess drift, finish experiment report, publish separate perf PR, run full CI; no live multiplayer validation.

2026-09-17 delivery: PR315 https://github.com/MongooseMoo/barn/pull/315, head ce7ed9bb902da8e339cdaf379d9b854ba4437c29; worktree clean.
Final alternating ten-pair comparison on CPU31: protected calls12.277ms to2.608ms (-78.76%); direct control unchanged statistically (p=.971); bytes-88.47%, allocations-71.33%.
All raw runs, including noisy sequential data, corpus results, oracle probes, and report committed under experiments/2026-09-17-protected-dispatch.
Full Windows and Linux Go suites passed. CI35192180679 build/unit job success and full conformance step success; conformance job still saving setup-go cache at this checkpoint.
No queue or merge action taken for315. PR307 remains separate and unqueued; live multiplayer validation skipped by user request.

Final CI35192180679 completed successfully at ce7ed9bb902da8e339cdaf379d9b854ba4437c29: build/unit and MOO conformance jobs both success. Artifact reports zero failures and errors; PR315 body updated with exact-head CI and report links.
