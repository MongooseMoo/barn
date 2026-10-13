# PR 81 current-state audit

- Scope: Read-only audit of open PR #81 and issue #4 against `origin/master` at `26ffaa21`.
- Guardrails read: `C:/Users/Q/code/barn/AGENTS.md`; no project planning or historical Markdown will be read.
- Current finding: the required analyst gauntlet file was not present at `C:/Users/Q/.Codex/AGENTS.md.d/gauntlet.md`; a search under `C:/Users/Q/.codex` returned no match.
- Blocker: none yet; the role protocol path is unavailable, so continue with the explicit analyst checks in the task instructions.
- Next action: inspect repository/worktree state and current GitHub PR/issue metadata without making source or GitHub changes.
- This note must remain uncommitted and unstaged.

## Current findings

- GitHub `master` and local `origin/master` are both `26ffaa21fcbbabd8c52f7c44b40c9cbed99defce`; PR #81 head is the single commit `e2fc3653322a8188506208d8a08055ad5267906e` based on `aa82abf05a6a23783ba69105d0c393f926304a13`.
- PR metadata: open, `MERGEABLE`, `UNSTABLE`; `Build & unit tests` passed, `MOO conformance suite` failed on the old base at `index_third_arg_str`; body contains literal `Closes #4`.
- `git merge-tree --write-tree origin/master e2fc365...` succeeded without conflicts and produced exact integration tree `639f06f81975fc4d460d3fab942b010da68183bf`.
- Exact integration proof in a disposable archive: focused random tests repeated 20 times pass; `go test -race ./builtins -count=1` passes; `go vet ./...` passes; `go test ./... -count=1` passes after initializing only the disposable archive as a local Git repository for the root hygiene test.
- Canonical managed WSL oracle command used: `wsl -d Debian -u root -e env MOO=/root/src/toaststunt/build-release/moo bash /mnt/c/Users/Q/code/barn/scripts/wsl_oracle.sh ";reseed_random()" ";return reseed_random();" ";{typeof(reseed_random()), reseed_random() == 0}"`. It exited 0 and returned `=> 0` and `=> {0, 1}`; Toast observably returns integer zero.
- A profile-less managed conformance run was invalid evidence: it used the wrong conformance checkout/profile and ended with capability-probe/server-lifecycle cascades (190 failed, 11010 passed, 384 skipped). Exclude it.
- Current conformance dependency `main` is `21add5a254843de871d1daf8ec929b26f5e3e55f`; current `master` CI run 31109852913 at `26ffaa21` passed with 11177 passed, 410 skipped.

## Current blocker

- Exact-profile local managed command was run from `C:/Users/Q/AppData/Local/Temp/barn-pr81-audit-twqwokcy.zph` against merge tree `639f06f` and conformance `21add5a`; expected capability admission followed by the packaged suite.
- Actual result: harness startup error before admission. Barn exited 1 because the managed command parser stripped backslashes from `--config=C:\Users\Q\AppData\Local\Temp\barn-pr81-audit-twqwokcy.zph\profiles\barn\outbound-on.conf`, producing nonexistent `C:UsersQAppDataLocalTempbarn-pr81-audit-twqwokcy.zphprofilesbarnoutbound-on.conf`. Server log: `C:\Users\Q\AppData\Local\Temp\moo_conformance_v279ols0\server.log`.
- Per `AGENTS.md`, stop rather than improvise another launch. Current PR conformance on the present profile remains unverified locally; GitHub cannot recompute it until the branch is updated.

## Next action

- Finish source/test/scope review from current trees, identify exact repair, and report retain/fix/close recommendation. Do not merge, edit source, push, or change GitHub.

## Final source finding

- The current canonical Toast source at WSL commit `aecc51e9449c6e7c95272f0f044b5ba38948459e` disproves issue #4 and PR #81's shared-family claim. `src/numbers.cc` has distinct streams: `random()` uses static `std::mt19937 rng`; `reseed_random()` only calls `reseed_rng()`; `frandom()` calls C `rand()`; `random_bytes()` calls `sosemanuk_prng(&run_context, ...)`. `src/server.cc` separately schedules/initializes SOSEMANUK and uses it once to seed C `rand()`.
- Therefore the PR correctly fixes the integer `random()` reseed path but incorrectly reroutes `frandom()` and `random_bytes()` through the new ChaCha8 stream. Its deterministic shared-family test locks in the wrong architecture.
- Current base advanced to `d2d8ccf9a3cba21a5348e96ff2d84105dd2c6884`; PR is 61 commits behind. Fresh synthetic merge tree `ffb685ed1491818b8cd8f8e657e361952c598ee0` is conflict-free and passes focused tests x20, race, vet, formatting, and full Go tests.
- Recommendation: retain issue #4 and PR #81 for repair; do not merge or close as fixed. Normally merge current `origin/master` into the PR branch, then revert the `frandom()` and `random_bytes()` reroutes, narrow the generator and tests to `random()`/`reseed_random()`, add independence/failure-path coverage, correct the PR/issue shared-family claims, and obtain fresh required CI.
