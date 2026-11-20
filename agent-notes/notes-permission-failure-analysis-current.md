# Current permission-failure analysis

## Current state

- Read-only analysis target: six Barn permission failures in paired conformance run `30990243152`.
- Exact candidate pair: Barn `54382063457e94cb991f08f0de4307b21d7a11cb`; conformance `13e7bf598ce693070d05c1754e8c99c4158601c0`.
- Toast admission, harness gates, and all three Toast profiles were green; only the paired expected-success verdict failed.
- No Barn or Toast server has been launched manually.

## Findings and observations

- Live issue-title searches show no existing issue explicitly covering these six permission failures.
- Barn issue #13 concerns built-in property assignment bypassing permission checks and may be adjacent, but coverage has not yet been proven from its current body/code path.
- The exact conformance failures are semantic, not harness errors: five expected `E_PERM` calls returned success (`0` or `[]`), while `finished_tasks()` returned a task-ID list.
- The property/verb cases create an object as wizard, clear its object `w` or `r` flag, then invoke the builtin as programmer. The Toast matrix proves the expected result is `E_PERM`.
- Current Barn candidate symbols show `builtinDeleteProperty` and `builtinDeleteVerb` have explicit permission TODOs, while `builtinProperties`/`builtinVerbs` need inspection for missing object-read checks.
- Current `builtinFinishedTasks` is in `builtins/signatures.go`; its surrounding permission gates need exact inspection.
- Candidate worktree `C:\Users\Q\code\.worktrees\barn-conformance-admission-integration` is clean at the paired Barn SHA `54382063457e94cb991f08f0de4307b21d7a11cb`.
- Five failures share a literal omission: `builtinProperties`, `builtinAddProperty`, `builtinDeleteProperty`, `builtinVerbs`, and `builtinDeleteVerb` do not call Barn's object read/write authority logic. The delete functions retain explicit permission TODOs.
- `builtinAddVerb` already implements the needed Toast-shaped object-write rule inline: wizard, object owner, or object `w` flag. The missing read analogue is wizard, object owner, or object `r` flag.
- `builtins/store_reads.go` exposes transaction-aware `objectOwnerForRead` and `hasObjectFlagForRead` but no common object-allows helper, which explains duplicated and omitted permission logic.
- `builtinFinishedTasks` validates arity and immediately enumerates all global tasks; unlike adjacent `builtinThreads` and `builtinThreadPool`, it has no `ctx.IsWizard` gate.
- Current `_test.go` files contain no direct coverage for the six denials. Existing `newReviewCtx`, `mustCreate`, `mustAddVerb`, and store flag helpers support focused table tests.
- Exact GitHub search found no issue or PR mentioning these functions/case names. Issue #13 is adjacent but distinct: it covers VM built-in/user-defined property assignment, not schema/introspection builtins.

## Blocker

- None. The paired run is in `MongooseMoo/moo-conformance-tests`, not the Barn Actions repository; the exact run was located there.

## Next action

- Audit error-ordering and transaction/coarse-operation placement, then provide the parent a facts-first root-cause report and two narrow TDD/PR clusters.
