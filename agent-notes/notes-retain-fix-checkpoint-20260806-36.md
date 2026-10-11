# Retain/fix checkpoint 36

## Current findings and state

- PR #81 repaired head `69deb94474710cd12ffef6f43f822f4c76c8575e` received independent CODE APPROVE. The exact final diff against master is three files: `builtins/math.go`, `builtins/random.go`, and `builtins/random_test.go`.
- Exact-head local focused x20, focused race, full tests, full race, build, full vet, gofmt, and diff checks are green.
- The local managed conformance result is excluded: profile admission metadata was empty, both mutually exclusive 32/64-bit tests failed before execution, and later connection failures cascaded. It is not attributed to the code.
- PR #81 head was pushed. The title/body now state that `reseed_random()` affects only `random()`, preserve independent `frandom()` and secure `random_bytes()`, record the canonical WSL Toast contract, remove the stale four-file/shared-stream claims, retain literal `Closes #4`, and require fresh admitted GitHub CI.
- PR #81 fresh exact-head GitHub build/unit and default-profile conformance jobs are both currently running.
- A worker is active on the #29/#45/#77 transaction cluster in isolated worktree `C:\Users\Q\code\.worktrees\barn-fix-29-45-77-coarse-atomic`, branch `agent/fix-29-45-77-coarse-atomic`, from exact master `d2d8ccf9a3cba21a5348e96ff2d84105dd2c6884`.
- The transaction worker is adding the audited red tests before implementation. No Toast uncertainty has arisen yet; no GitHub changes are authorized for that worker.

## Blockers

- PR #81 cannot merge until both exact-head GitHub checks complete green and the uploaded conformance artifact proves the admitted default-profile baseline.
- The transaction cluster cannot be declared fixed until red proofs, implementation, independent review, exact-head CI, and literal closure of only #29/#45/#77.

## Next action

1. Verify PR #81 CI and artifact counts/admission; audit files/commits/issue/checks/closing phrase; normal-merge and verify two parents plus issue #4 closure.
2. Monitor the transaction worker's red/green proof and review the implementation before any push.
3. Keep parser #25 separate until its numeric nesting ceiling is verified against Toast and the current verb corpus.

This note is intentionally uncommitted and must not be staged, committed, or pushed.
