# Issue #87 working notes

## Findings

- `compiler` used one package-global 8,192-entry LRU keyed only by `sourcekey.Key`.
- `bytecode/compiler.go` owned semantic `verb` IR lowering and `bytecode/literal_values.go` also imported `verb`, contrary to issue ownership.
- Builtin IDs are embedded in `OP_CALL_BUILTIN`, while `builtins.Registry.Register` can mutate the name-to-ID mapping.
- The focused red tests prove both defects: registry B received registry A's ID (17 instead of 23), and a mutated registry received its prior ID (31 instead of 47).
- The main checkout has extensive unrelated untracked files but no tracked edits. Work is isolated in `C:\Users\Q\code\.worktrees\barn-issue-87-compiler` on `agent/issue-87-compiler-cache`.

## State

- Red test commit: `c0ae151 test: expose compiler cache registry leakage`.
- `bytecode/compiler.go` and its internal builtin-slot test have been moved into `compiler` in the working tree.
- Lowering references are being qualified against the bytecode representation package; the tree is intentionally between red and green and does not compile yet.

## Current blocker

- No external blocker. The mechanical ownership move exposed additional semantic lowering helpers in `bytecode/literal_values.go` plus unqualified representation names that must move or be qualified.

## Next action

- Finish moving literal lowering into `compiler`, add the compiler-instance and immutable builtin-ID snapshot API, invalidate the registry-owned compiler instance on `Register`, migrate all callers, then rerun focused tests, full Go gates, and managed conformance.

## Progress update

- Semantic lowering and literal conversion now live under `compiler`; `bytecode` no longer contains the lowering implementation.
- `compiler.Compiler` now owns a copied `BuiltinIDs` snapshot and a bounded per-instance LRU.
- `builtins.Registry.Compiler()` owns the active compiler instance, and `Register` invalidates it when the registry layout changes.
- The compiler, builtins, bytecode, and VM packages now compile after migrating their production and test callers.
- Remaining work is caller migration in engine/server/app packages, then focused/full verification and API-boundary inspection.

## Green boundary update

- All production and test callers have now migrated to registry-owned compiler instances.
- Compiler-specific source and loop-label contract tests moved from `bytecode` to `compiler`; bytecode disassembly tests still compile inputs through the public compiler API.
- `go test ./compiler ./bytecode -count=1` is green.
- A source scan finds no `verb` import or reference under `bytecode`.
- Focused registry isolation, mutation invalidation, immutable snapshot, and zero-allocation cache-hit tests are green.
- No blocker is active. Next is full test/vet/race-sensitive verification, diff audit, then managed conformance.

## PR and CI update

- PR #192 opened ready for review with `Closes #87` and exact head `2d45514bb0aa1463c4fdba76bd79da74ab988fe4`.
- Required `Build & unit tests` and `MOO conformance suite` checks both completed successfully on that head.
- Downloaded conformance artifacts report 11,611 tests, 0 failures, 0 errors, and 420 declared skips; all admission probes passed.
- Final PR audit confirmed the two intended commits, clean merge state, exact file scope, closing reference, and unchanged base `fa5b338`.
- Executed the authorized normal-merge command `gh pr merge 192 --merge`; it returned successfully.
- Next action is post-merge verification of PR state, the merge commit's two parents, master reachability, and issue #87 closure.

## Completion

- PR #192 is merged at `60d24bfa7dc47c32ef3fe76adcd24d0a3830b9dc`.
- The merge commit has exactly two parents: base `fa5b338d4651e1b98acc550ed10a2bdac9bf20b6` and feature head `2d45514bb0aa1463c4fdba76bd79da74ab988fe4`.
- `origin/master` points to the merge commit and contains the feature head.
- Issue #87 is closed with reason `COMPLETED`.
- No blocker or remaining requested action exists.

This file is a local checkpoint only. Do not stage, commit, or push it.
