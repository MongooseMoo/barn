# Compiler bookkeeping cleanup

## Goal
Remove unused compiler scope bookkeeping and redundant identifier lookup without changing functionality. Work only in `refactor/compiler-bookkeeping`; parent owns publication.

## State
Implemented against base `cd004bcc`. Smallest change: deleted the unused scope type, stack, lifecycle calls and map writes; use `declareVariable` once and emit `OP_GET_VAR` once in `compileIdentifier`; deleted `resolveVariable` and the obsolete test setup call. Production reduction: 61 net lines, plus one obsolete test setup line. No argument compilation refactor. Full implementation diff inspected.

Implementation commit: `4ab0415c`. Published as https://github.com/MongooseMoo/barn/pull/501 from isolated worktree `C:/Users/Q/code/barn-compiler-bookkeeping`, branch `refactor/compiler-bookkeeping`. Initial GitHub response: `{"number":501,"headRefOid":"4ab0415c48a4560b9305620d990ab383a90f69d8","statusCheckRollup":[]}`. Check the PR for current CI results; no merge is authorized by this task.

## Decisions and why
- At base `cd004bcc`, `compiler/lower_locals.go:43-47` already resolves existing names through the canonical-key `variables` map before allocation. The scope map is only written (`:73-74`), and its parent link only assigned (`:27`); neither supplies name lookup.
- At base, all three branches of `compiler/lower_expressions.go:46-73` emit the same variable opcode and index. The resulting function (`:46-51`) delegates lookup/allocation to `declareVariable` (`compiler/lower_locals.go:18-49`), retaining its overflow handling, display spelling and builtin-slot assignment. Internal allocation (`:56-74`) and compaction (`compiler/lower.go:104`) are unchanged.
- Keep internal variable allocation and compaction unchanged. Do not add tests or machinery. PR CI supplies verification; no local CI reruns.
- `.github/workflows/ci.yml:6` triggers PR CI; existing formatting, vet, build, unit tests and managed conformance steps are at lines 39, 42, 57, 60 and 111. Results belong to the exact PR head; no passing result is recorded here.
- Raw inspection evidence: `git diff --stat` returned `4 files changed, 1 insertion(+), 63 deletions(-)`; compiler search for `beginScope|endScope|resolveVariable|\.scopes|type scope struct` returned exit 1 with no matches.

## Surprises
None identified. Runtime and performance parity remain unverified; source removal alone is not a measurement.

## Next step
Read PR CI and diagnose any in-scope failure. No tests or performance measurements were run locally. The PR remains open for Q; retain its worktree while it is open.
