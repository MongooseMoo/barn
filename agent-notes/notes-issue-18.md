# Issue 18 working ledger

## Scope

- Fix Barn issue #18: nested `try/finally` unwinding skips the outermost finalizer.
- Preserve a Toast-backed regression in `moo-conformance-tests` and a focused Go regression in Barn.
- Open separate repository PRs; the Barn PR must contain `Closes #18`, pass required CI, and use a normal merge commit.

## Current findings and observations

- Clean worktrees/branches:
  - Barn: `.tmp/issue-18-barn`, `agent/issue-18-nested-finally`, based on `origin/master` at `b750f8a`.
  - Conformance: `.tmp/issue-18-conformance`, `agent/issue-18-nested-finally`, based on `origin/main` at `21add5a`.
- Durable conformance coverage adds four cases to `language/try_except.yaml`: two and three nested finalizers, finally inside except, and except inside finally.
- Managed WSL Toast command used the canonical wrapper and selected canonical admission plus the four focused cases. Result: 5 passed, 11,586 deselected.
- The unchanged pre-fix Barn binary passed admission and the two mixed except/finally cases, but failed the two affected cases:
  - two levels: got `{ "inner" }`, expected `{ "inner", "outer" }`;
  - three levels: got `{ "inner", "middle" }`, expected `{ "inner", "middle", "outer" }`.
- Conformance regression commit: `d136821 Cover nested finally unwinding` (one file, 95 insertions, no unrelated line-ending churn).
- Barn Go regressions currently modify only:
  - `vm/bytecode_execution_test.go`;
  - `bytecode/disassemble_test.go`;
  - `bytecode/program_test.go`.
- Focused Go red proof:
  - VM nested-finally test fails at the same two assertions as managed conformance.
  - disassembly test shows `END_FINALLY` has no operands.
  - fork extraction test shows an `END_FINALLY` handler IP is not rebased.

## Implementation direction

- Encode the matching finally handler IP as a two-byte absolute operand on both `OP_END_FINALLY` instructions emitted for one `OP_TRY_FINALLY`.
- At runtime, consume the operand and pop only a top `HandlerFinally` whose `HandlerIP` matches it; if its own handler was already removed during error unwinding, preserve/re-raise `PendingError` instead of consuming an enclosing finalizer.
- Update disassembly operand decoding and fork-body absolute-IP rebasing for the bytecode-format change.

## Blocker

- None.

## Next action

- Commit the failing Barn tests, implement the narrow compiler/VM/disassembler/rebase change in a separate green commit, then rerun focused Go and managed conformance gates.

## Checkpoint after implementation and publication

- Barn TDD commits are complete:
  - `940ce54 Add nested finally regression tests` is the red test commit.
  - `ee85437 Preserve nested finally handlers` is the green implementation commit.
- The implementation adds the matching two-byte absolute finalizer IP to both `OP_END_FINALLY` instructions, matches it in the VM, decodes it in disassembly, and rebases it in extracted fork bodies.
- Focused Go tests pass for `bytecode` and `vm`; `go test ./... -count=1` passes repository-wide.
- Fixed Barn passes the unchanged managed focused slice: canonical admission plus four nested-finally cases, 5 passed.
- A local Windows full conformance run is not a valid paired authority because Barn marks Windows Test.db profiles diagnostic until a matching Windows Toast oracle exists. Without an oracle profile, capability metadata is empty and `ONLY_32_BITS` probes fail; the startup-repair rows also do not produce Toast-named output files on Barn. These failures are unrelated to issue #18 and no changes were made for them.
- Conformance commit `d136821 Cover nested finally unwinding` was pushed and opened as `MongooseMoo/moo-conformance-tests#77`.
- PR #77 scope audit: one commit, one file, 95 additions, ready for review, body says `Supports MongooseMoo/barn#18` rather than closing it.
- PR #77 CI passed trust-boundary classification, trusted-controller validation, schema/lint, and the exact-head status probe.
- Three required Toast matrix jobs are currently in progress. Each has passed checkout, dependency/build steps, pinned oracle profile generation, and canonical capability admission, and is running every packaged conformance case:
  - 32-bit outbound-on;
  - 64-bit outbound-on;
  - 64-bit outbound-off.

## Current blocker

- Waiting for the three required Toast matrix jobs on conformance PR #77. No implementation blocker.

## Updated next action

- Continue watching PR #77. If every required check passes, re-audit files, commit list, scope, check state, and the exact normal-merge command, then merge with `gh pr merge 77 --merge` and verify the merge commit has two parents. After conformance `main` contains the durable regression, push/open the Barn PR with literal `Closes #18`, watch its required Linux/WSL CI, normal-merge, verify two parents, and confirm issue closure.

## Final state

- Conformance PR `MongooseMoo/moo-conformance-tests#77` passed every required check, including complete 32-bit and 64-bit Toast matrices, and was normal-merged.
- Conformance merge commit: `ac0819f8909faa179bd5f70ca221ea942c3da300`, verified with two parents (`21add5a`, `d136821`).
- Barn PR `MongooseMoo/barn#126` contained exactly two commits and eight issue-scoped files, with literal `Closes #18` in the body.
- Barn required CI passed:
  - Build and unit tests: 56 seconds.
  - Managed MOO conformance suite: 4 minutes 25 seconds, consuming the merged conformance regression from `main`.
- Barn PR #126 was normal-merged with `gh pr merge 126 --merge`.
- Barn merge commit: `598df037b511384ac468e520de0e3e600d3e1cdc`, verified with two parents (`b750f8a`, `ee85437`).
- Barn issue #18 closed at `2026-08-09T01:43:00Z`.
- No blocker remains. The requested task is complete.

This ledger is intentionally uncommitted and must not be staged or pushed.
