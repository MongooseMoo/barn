# ci/conformance-red-green-gate

## Goal

Let an automatic Barn repair of a Toast-comparison finding merge unread when a
mechanical red/green proof holds, as Q approved on 2026-11-24: a conformance
test the managed harness passes on Toast, fails on Barn at the merge base, and
passes on Barn at the repair's head counts as the repair's proof, alongside the
existing fuzz-input proof.

## State

Built and pushed on ci/conformance-red-green-gate; no PR opened (the
orchestrator opens it).

- scripts/automatic_fix_scope.py: Q's 2026-11-24 rule text verbatim in the
  docstring; `--conformance-evidence PATH`; `conformance_proves()` accepts only
  Toast green, base red (admission passed, a selected test failed), head green
  over one identical non-empty selection. Otherwise the reason text is
  unchanged.
- scripts/test_automatic_fix_scope.py: ConformanceEvidenceTests (fuzz only in;
  red/green in; base passing, Toast failing, head failing, no evidence out;
  base that never admitted out; mismatched or empty selection out; evidence
  does not excuse touching a profile). 17 tests pass on Python 3.12 (Windows)
  and 3.11 (WSL Debian).
- .github/workflows/claude-fix.yml: scope step exports `base`; new steps
  "Run the conformance test on Toast, the merge base and the head" (proof),
  "Check the repair's scope with the conformance evidence" (proof_scope),
  "Let the proven repair merge" (short Claude run that arms); the decision
  step reads proof_scope's result first and names a proof run that did not
  finish.
- actionlint v1.7.12 with shellcheck (WSL Debian): only the pre-existing
  `$/.github/actions/...` "ref is missing" errors, identical on master.
- The evidence program was run on the saved JUnit fixtures in
  scripts/fixtures/fuzz/cross-engine and parsed them; the CLI was run with a
  red/green evidence file (exit 0, in scope) and with none (exit 1, today's
  reason).
- Not exercised on a runner: none of the new workflow steps has run.

## Decisions and why

- Engines and suite come from what CI already does on these runners:
  - Toast: `bash scripts/build-toast-oracle.sh` (ci.yml cross-engine job "Build
    Toast oracle", fuzz.yml comparison lanes), run by the harness as
    `$toast {db} {db}.new -p {port}` with `--target-profile-manifest` set to
    profiles/toast/stock-wsl-testdb.json (scripts/cross-engine-session.sh:30 and
    :42-44; Makefile conformance-toast).
  - Barn: `go build -o ... ./cmd/barn` and the server command of ci.yml's
    "MOO conformance suite" job (`serve --db {db} --listen ... --config=
    profiles/barn/outbound-on.conf --profile-id=barn-linux-testdb-outbound-on
    --profile-manifest={manifest}`, with `--oracle-profile-manifest`).
  - Suite: the barn-fix/pr-N branch that the conformance-test action already
    committed and pushed in ./moo-conformance-tests; its added `_tests` files
    against origin/main (the same range the action checks at
    action.yml:125) are the selection, passed as package paths the way ci.yml's
    issue-423 step passes `_tests/vm/tick_accounting.yaml`.
- Base and head Barn are each built from a fresh `git worktree` at the exact
  commit, so files Claude left in the workspace cannot leak into either build.
  Profiles, the oracle manifest and build-toast-oracle.sh come from the base
  worktree (master history); a repair that changes them is out of scope anyway.
- Red/green is read from each run's JUnit file, not only its exit code: base
  counts as red only when capability admission passed and a selected test
  failed, so a harness or startup failure cannot pass for "base failed".
- Arming: claude-code-action revokes its app token when its step ends
  (anthropics/claude-code-action action.yml "Revoke app token"), and a queue
  entry made with GITHUB_TOKEN stalls (claude-fix.yml header). Letting the
  conformance Claude arm before the mechanical proof exists would let the
  merge queue race the proof. So the Toast-check Claude still arms only when
  the fuzz rule already holds (unchanged may_queue), and when the conformance
  proof makes a repair in scope a short second Claude run arms it after the
  proof. The final decision step still disarms when anything fails.

## Surprises

- The scope check ran before the Toast check and fed `may_queue`; the
  conformance proof can only exist after the Toast check, so scope is checked
  again after the proof.

- The local Python 3.7 (`python3` on Git Bash) cannot import
  fuzz_red_green.py (walrus operator); use `uv run --no-project --python 3.12`.
- git inside WSL cannot read this Windows worktree's gitdir, so the CLI was
  exercised from Windows.

## Next step

Open the PR against master, then exercise the path on a real repair: re-run
the conformance stage for barn PR #1130 (claude/issue-1124) and check the
evidence JSON, the proof_scope verdict and the arming step in the run log.
