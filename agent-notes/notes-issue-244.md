2026-09-17: Issue 244 delivery uses C:/Users/Q/code/barn-issue-244, branch fix/issue-244-builtin-descriptors from 4eacf59.
Prerequisite #238 is closed; no existing PR for #244. Main checkout has unrelated changes; preserve them.
Clean conformance dependency at C:/Users/Q/code/barn-244-conformance, commit 9bdfd2e. Windows harness with canonical WSL Toast: 312 passed, 11 skipped in 14.20s.
WSL-hosted harness was interrupted during collection (0 tests) after filesystem wait; Windows harness uses external platform-specific UV_PROJECT_ENVIRONMENT.
Next: finish exact oracle contracts and capability policy, then descriptor construction tests and migration; no implementation changes yet, only a new oracle YAML fixture. PR authorized, merge not requested.

2026-09-17 checkpoint: Independent custom managed Toast cases plus admission passed (5 cases); JSON embedded object output includes #. Optional background_test/malloc_stats/read_stdin/finished_tasks absent in stock oracle.
Capability policy chosen: preserve Barn default callable extensions, distinguish availability from runtime permissions, allow explicit family filtering.
Added typed descriptors/capabilities and 250 consolidated base rows; removed generated signature file/generator and name-based policies. VM contributes eval/pass descriptors.
Construction tests failed before implementation, then focused descriptor/completeness/duplicate tests passed. Current worktree is mid-migration and not ready to publish.
Next: migrate test registration to immutable construction fixtures, wire runtime/profile capabilities and compiler layout ownership, run Go and managed conformance gates. Main checkpoint remains uncommitted.

2026-09-17 checkpoint: Runtime now accepts a constructed registry; config has explicit capability selection with nil preserving defaults.
Engine test fixture helper constructs complete descriptors before runtime creation and uses callback slots for tests capturing their runtime.
Pending fixture migration includes 11 engine test files plus VM anonymous GC and server eval tests; two engine helpers register callbacks on existing runtimes and need separate adaptation.
No PR/commit yet; only focused descriptor tests have run after production edits. Full build is expected to fail until fixture migration finishes.
Next: replace Register call sites, derive profile/server feature reporting from registry, preserve pass lookup and compiled-layout ownership, then validation.

2026-09-17 checkpoint: All Register call sites migrated; base and VM descriptors construct immutable registries, and test callbacks use construction-time slots/substitution.
Runtime/profile capability parsing and registry-derived presence are wired. Compiled programs carry layout fingerprints checked before builtin/pass dispatch.
First package gate: builtins, vm, engine, server, internal/app passed; compiler failed its old implicit-pass fixture, config/profile failed old presence/registry expectations.
Config/profile fixtures updated after those failures; pass fixture still needs explicit registration. No commits or publication.
Next: expand admission/capability/layout tests, fix pass characterization, format and run repository/managed conformance gates; documentation and PR remain.

2026-09-17 checkpoint: Capability combinations, dispatch parity, VM descriptors, config parsing, runtime presence, and profile presence focused tests pass.
Asked optional preference on old checkpoint IDs; no answer yet, stated default is preserve IDs. Registry now keeps descriptor order and leaves disabled ID holes.
Added fingerprint propagation through fork extraction, VM snapshots, checkpoint encoding/decoding, and restore checks. Older raw snapshots retain stable default IDs.
Historical 252-name layout digest independently computed from HEAD registration order is 73cd9d376515c5d1fb6682bb2c1aa213fd3d6e54a450a5d322dc3980b9cc272d; test expected digest corrected after fixing probe sorting.
Documentation added. Next: finish tests/review, format literal Go paths, full Go/static gate, managed Barn conformance and oracle JSON cases, then commit/PR/CI. No publication yet.

2026-09-17 checkpoint: Full Go tests and static checks passed before latest fixes; Windows managed JSON/admission run passed after startup fix.
Regression fixed connection_option semantic error order; server now constructs and shares its immutable registry before profile generation and database load.
Full managed Windows run first exposed startup nil registry (fixed), then missing admission evidence context (invocation corrected by omitting optional evidence output).
Full conformance rerun active in session 1872; log barn-issue-244/.tmp/conformance.log. No commit or PR yet.
Next: collect terminal conformance results, rerun final Go/static gates, review diff, commit and publish PR with Closes #244, then exact-head CI.

2026-09-17 checkpoint: Final Windows go test ./..., go vet ./..., and staticcheck ./... completed successfully (session 11193 exit 0).
Full managed conformance remains running in session 1872, beyond 80%, with failure markers; collecting terminal inventory before diagnosis.
Reviewed descriptor effects, capability reporting, compiler layout ownership, persistence and injected fixtures against issue; removed one stale effect-helper comment.
Formatting and git diff --check clean; CI_RUNNER_LABELS remains self-hosted Linux X64 barn. No commit or PR yet.
Next: inspect full conformance failures, verify disputed behavior through managed Toast first, establish any baseline/platform failures, resolve acceptance and publish.

2026-09-17 checkpoint: Windows full run finished 12469 passed, 394 skipped, 74 failures all from missing ONLY_32_BITS profile metadata; zero distinct builtin assertion failures.
Adding canonical oracle manifest rejects Linux/Windows runtime_os pairing. Reported concrete Windows comparison limitation; Linux managed run now active session 95639.
Linux binary built natively in Debian; clean conformance 9bdfd2e archive extracted to /root/.cache/barn-244-conformance-source to avoid slow p9 collection, separate uv environment retained.
Audit confirms all 250 implementation/order entries and 237 preexisting signatures unchanged, no effect mismatch; 13 missing signatures completed. 56 issue-owned files staged; no commit or PR.
Next: collect Linux full suite from .tmp/conformance-linux.log, then commit/push/create PR using .tmp/pr-body.md and monitor exact-head CI. Windows profile-mismatch run was session 98587.

2026-09-17 checkpoint: Linux managed full suite completed exit 0: 12517 passed, 420 skipped, 1 warning in 314.58s; admission and profile matching included.
Committed 28a131971ce0c760275645d53065783f3d119188 and opened https://github.com/MongooseMoo/barn/pull/318 with Closes #244; no merge authorized.
CI run 35202034241 on that exact head remains active. Go formatting/vet/staticcheck/build/test steps succeeded; setup-go post-cache step still running.
Conformance CI is running its suite; local worktree clean. Original checkout notes remain uncommitted and unrelated changes preserved.
Next: wait for terminal exact-head CI, diagnose only concrete failures, then report PR and verified checks. Final memory citation MEMORY.md:3103-3120, rollout 01a05987-cb0b-7433-8abc-fa41efb5af8f.

2026-09-17 final checkpoint: PR #318 remains open and mergeable at 28a131971ce0c760275645d53065783f3d119188; not merged.
CI run 35202034241 completed success on that exact head: Build & unit tests pass 6m24s; MOO conformance suite pass 10m34s.
Local isolated worktree is clean; no additional source changes after the reviewed commit.
Windows Go/static gates and focused managed cases passed; complete comparison gate passed locally in Linux and in CI.
Authorized implementation and PR delivery complete. Original checkout changes and this uncommitted note preserved.
