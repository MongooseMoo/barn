# Issue #4: real shared RNG reseed

## 2026-07-30 deterministic Go TDD checkpoint

- Removed both temporary `reseed_random_semantics.yaml` candidates after they proved only already-correct Toast-compatible return and permission behavior.
  - The isolated conformance worktree is detached and tracked-clean again.
  - The mirrored candidate is absent from the existing local conformance checkout; its unrelated untracked state was not touched.
- Created Barn branch `issue-4-reseed-random` at exact `origin/master` commit `aa82abf05a6a23783ba69105d0c393f926304a13`.
- Added a deterministic package test that replaces the standard `crypto/rand.Reader` with two identical 32-byte seeds.
  - The first assertion requires each `reseed_random()` call to consume exactly 32 entropy bytes, so the pre-fix failure does not depend on random output.
  - After that deterministic gate, the test compares interleaved `random()`, `frandom()`, `random_bytes()`, and another `random()` draw with one fixed `math/rand/v2.ChaCha8` reference stream, then repeats after an identical reseed.
  - A separate focused assertion preserves non-wizard `E_PERM`.
- Exact pre-fix focused result:
  - `go test ./builtins -run 'Test(RandomFamilySharesReseededGenerator|ReseedRandomRejectsNonWizard)$' -count=1`
  - failed with `reseed_random() consumed 0 entropy bytes, want a 32-byte seed`.
- Implemented one package-owned `ChaCha8` generator protected by one mutex.
  - Package initialization and each `reseed_random()` draw a 32-byte seed from `crypto/rand`.
  - `random()`, `frandom()`, and `random_bytes()` now consume that same locked state.
  - The existing zero-argument check, wizard gate, observable integer-zero result, and crypto-read error mapping remain unchanged.
  - The dead `math/rand.Seed` and `time.Now()` path is removed.
- Exact post-fix focused result: pass (`ok barn/builtins 1.006s`).
- Full one-slice diff review and `git diff --check`: clean.
- Relevant builtins package gate: `go test ./builtins -count=1` passed (`ok barn/builtins 1.029s`).
- Required full repository gate `go test ./... -count=1`: passed across all packages.
- Exact full managed conformance command passed: 11,451 passed, 128 skipped, 0 failed in 293.45 seconds.
  - Receipt preserved at `C:\Users\Q\code\barn-issue-4-reseed-random-20260730\reports\runs\20260730_105521`.
- Conformance reconciliation reverified after the interrupted turn:
  - isolated worktree `C:\Users\Q\code\moo-conformance-issue-4-reseed-20260730` is detached and clean;
  - existing checkout remains `main`, 16 commits ahead of `origin/main`, with its unrelated untracked corpus preserved;
  - `reseed_random_semantics.yaml` is absent from both worktrees;
  - no conformance candidate will be published.
- Added a narrow concurrent-use test that overlaps all three random consumers with `reseed_random()` through separate task contexts.
- Final focused issue tests passed (`ok barn/builtins 0.967s`).
- Relevant builtins package with actual concurrent access under `-race` passed (`ok barn/builtins 1.445s`).
- Final `go test ./... -count=1` rerun against the complete source and test set passed across all packages.
- Final gofmt, dead-call, cached-diff, and exact-path staging checks passed.
- Single kept source commit: `e2fc365` (`fix: reseed shared random generator`), containing exactly:
  - `builtins/crypto.go`;
  - `builtins/math.go`;
  - `builtins/random.go`;
  - `builtins/random_test.go`.
- Pushed `issue-4-reseed-random` and opened ready Barn PR #81:
  - `https://github.com/MongooseMoo/barn/pull/81`;
  - base `master` at `aa82abf05a6a23783ba69105d0c393f926304a13`;
  - head `issue-4-reseed-random` at `e2fc3653322a8188506208d8a08055ad5267906e`;
  - one commit, four intended changed files, zero commits behind;
  - open and non-draft with exact standalone `Closes #4`.
- CI run `30565274611` reached terminal state:
  - `Build & unit tests`: passed in 56 seconds;
  - `MOO conformance suite`: failed in 4 minutes 30 seconds;
  - exact unrelated failure: `index_call_shapes::index_third_arg_str` expected type `int` but received a string containing an `E_INVARG` traceback and `{1, 2}`;
  - CI summary: 1 failed, 11,437 passed, 128 skipped in 243.62 seconds.
- The failing `index()` conformance surface is unrelated to the four issue #4 random-generator paths, and the exact local full managed run passed with 0 failures.
- No CI rerun or issue-branch patch was made.
- Current blocker: unrelated master-baseline conformance failure `index_call_shapes::index_third_arg_str`.
- Next action: stop this issue branch for independent master-baseline repair; do not merge.

## 2026-07-30 Toast and managed candidate result

- Verified exact stock Toast identity before the oracle run:
  - source commit `aecc51e9449c6e7c95272f0f044b5ba38948459e`;
  - executable `/root/src/toaststunt/build-release/moo`;
  - Debian WSL NAT host `172.17.144.45`.
- Ran the temporary two-row candidate only through the documented managed wrapper:
  - `uv run moo-conformance -k reseed_random_semantics --server-command "wsl -d Debian -u root -e env TOAST_MOO=/root/src/toaststunt/build-release/moo bash /mnt/c/Users/Q/code/barn/scripts/run_toast_wsl.sh {db} {port}" --moo-host 172.17.144.45 -v`
  - Toast result: 2 passed, 11,566 deselected in 7.77 seconds.
- Toast observable authority:
  - a wizard's zero-argument `reseed_random()` call is observed as integer value `0`;
  - a non-wizard call raises `E_PERM`.
- Mirrored the exact candidate into the expected editable sibling path only after confirming it was absent, then ran unchanged pre-fix Barn through the required managed script:
  - `.\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788 -K reseed_random_semantics`
  - Barn result: 2 passed, 11,579 deselected in 4.15 seconds; failed tests 0.
  - Receipt preserved at `C:\Users\Q\code\barn-issue-4-reseed-random-20260730\reports\runs\20260730_104637`.
- Return value and permission are already Toast-compatible and authorize no observable-surface change.
- No managed MOO row can deterministically prove the actual defect:
  - cryptographic reseeding has no deterministic cross-engine value sequence;
  - comparing pre/post random outputs can pass even when reseeding is a no-op;
  - MOO cannot inject or inspect generator state to prove that `random()`, `frandom()`, and `random_bytes()` share one reseedable source.
- Therefore no broken conformance regression exists to make durable. The temporary candidate files will be fully removed, not left as untracked substitutes.
- Current blocker: none.
- Next action: remove only the two exact candidate files created for the oracle proof, verify both conformance worktrees retain their prior state, then use deterministic Go TDD for the actual shared-generator/no-op defect.

## 2026-07-30 conformance authority and candidate checkpoint

- The earlier conformance-base blocker was invalid; the user's original instruction authorizes resolving the established workflow from live evidence.
- Read `C:\Users\Q\code\moo-conformance-tests\CLAUDE.md`, `README.md`, and YAML schema documentation.
- Established conformance workflow:
  - Toast is the authority.
  - Durable YAML tests live under `src/moo_conformance/_tests`.
  - Tests must run against Toast first.
  - Contributions use focused branches and pull requests against `origin/main`.
- Local conformance `main` is tracked-clean at `aa312b9`, 16 commits ahead of `origin/main` `c34ebf6`, with extensive untracked state.
- GitHub has no copy of `aa312b9` (`gh api` reports no such remote commit), and recent remote history shows focused PR branches against `origin/main`.
- Created isolated detached conformance worktree `C:\Users\Q\code\moo-conformance-issue-4-reseed-20260730` at exact `origin/main` `c34ebf6`; local conformance `main` and all its untracked state remain untouched.
- Existing generated `reseed_random` coverage asserts zero arguments and an integer runtime result but does not verify the exact value or direct non-wizard denial.
- Added a temporary managed candidate file in the isolated worktree with two deterministic rows:
  - wizard zero-argument call expects observable value `0`;
  - programmer call expects `E_PERM`.
- These rows can establish secondary return/permission observables only. They cannot deterministically prove the actual defect: cryptographic reseeding and shared state across three builtins cannot be established by comparing random sequences without a false-positive probability.
- The candidate becomes durable only if it exposes a required Barn divergence. If both engines pass, it will be fully restored and deterministic Go TDD will own the actual defect.
- Verified `/root/src/toaststunt/build-release/moo` exists and is executable in Debian WSL.
- No Toast process has been invoked yet.
- Current blocker: none.
- Next action: verify the WSL Toast source commit and NAT host, run only the documented managed wrapper path against the focused candidate on stock Toast, and stop before Barn if the oracle is blocked.

## 2026-07-30 premise and oracle-workflow checkpoint

- Read live issue #4 and its existing maintainer comment. Required outcome is a real cryptographic reseed of one generator shared by `random()`, `frandom()`, and `random_bytes()`, with Toast-compatible zero-argument return and non-wizard permission behavior.
- Verified new temporary path `C:\Users\Q\code\barn-issue-4-reseed-random-20260730` was absent before creation.
- Fetched and verified `origin/master` at green merge commit `aa82abf05a6a23783ba69105d0c393f926304a13`.
- Created a clean detached worktree at that exact commit. Main checkout was not switched, stashed, reset, cleaned, or edited.
- Current code validates the issue premise:
  - `random()` and `frandom()` use the package-level `math/rand` generator.
  - `reseed_random()` calls `rand.Seed(time.Now().UnixNano())`, which is a no-op on the current Go toolchain, then returns integer `0`.
  - `random_bytes()` separately uses `crypto/rand.Read`, so the three builtins do not share generator state.
- No existing focused Go tests for these random builtins were found.
- Barn's root `pyproject.toml` loads the editable sibling conformance repository at `C:\Users\Q\code\moo-conformance-tests`.
- Read the documented stock-Toast workflow before any Toast invocation:
  - binary `/root/src/toaststunt/build-release/moo`;
  - managed wrapper `scripts/run_toast_wsl.sh`;
  - Windows harness supplies disposable database and port placeholders;
  - wrapper performs path translation, NAT reverse-DNS protection, managed restart output, and foreground lifecycle.
- No Toast command has been run yet.
- Current blocker: none.
- Next action: inspect the sibling conformance repository's branch/tracked state and existing builtin-test patterns, create the smallest zero-arg return/permission regression there, run it through the documented managed WSL Toast path, and stop before Barn analysis if the oracle is blocked.
