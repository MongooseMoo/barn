# Parser resource-exhaustion audit

## State

- Audit target: exact `origin/master` commit `d2d8ccf9a3cba21a5348e96ff2d84105dd2c6884`.
- Scope: read-only review of current source/tests and current GitHub issues #25, #50, and #51.
- No implementation, staging, commits, pushes, or GitHub mutations performed.
- The main checkout is dirty and behind `origin/master`; inspection uses `git show` and `git grep` against the exact ref.

## Findings

- #25 is the security-critical process-termination class. `Parser.ParseExpression` and nested statement parsing recurse without a budget, and player eval compiles before scheduler task/tick limits exist.
- A `ParseExpression`-only guard is incomplete: statements recurse independently, while flat left-associative and postfix expressions produce deep ASTs iteratively and are later traversed recursively by bytecode compilation and formatting.
- #50 shares collection-nesting input shapes with #25, but its root cause is independent eager `%w` wrapping in `parseListExpr` and `parseMapExpr`; `ParseProgram` already collapses the surfaced message to `syntax error`.
- #51 is independent of nesting. `wrapInt64Literal` uses `big.Int.SetString` for one token. Existing source comments and object-literal tests require modulo-2^64 wrapping, so a digit rejection cap would change the current contract. A streaming `uint64` modulo conversion can preserve semantics with O(n) time and O(1) auxiliary memory.
- Central entrypoints include intrinsic eval, programming mode, `set_verb_code`, stored-verb compilation, task reload, CLI eval, and direct parse/format in `verb_code`. Ingress-only limits would leave reachable paths unprotected.
- TCP programming/input accumulation is unbounded; WebSocket reads have a 1 MiB cap. Those transport facts do not replace central parser/compiler resource contracts.

## Proposed decomposition

1. Security PR closing #25: add a hard parser recursion budget covering expressions and statements; test a provisional cap (suggest 512, reject 513) against Toast and the current verb corpus before freezing it. Add compiler integration tests and probes for flat operator/postfix ASTs; guard downstream traversal too if those probes expose an independent stack path.
2. Performance PR for #50: propagate nested collection errors without repeated eager wrapping; assert bounded `ParseError.Detail`, and benchmark multiple depths with allocations rather than wall-clock assertions.
3. Performance PR for #51: replace production `big.Int` conversion with streaming modulo-2^64 conversion for both integer and object literals; keep `big.Int` only as a test oracle and add size-scaled allocation/throughput benchmarks.

## Blocker / next action

- The user restricted this audit to current source/tests and issue text, so the required Toast oracle and current database compatibility check were not run. They are implementation gates for the exact numeric depth threshold and diagnostic contract.
- Next: finish the safe red-test/benchmark matrix and report the precise plan to the parent agent.

## 2026-09-01 Toast parser contract checkpoint
- Key finding: Toast uses repeatable `/* ... */` comments, not `//`; Barn currently recognizes the wrong comment syntax.
- Current state: Toast source resolves NUL, recovery, and line-diagnostic contracts; whole-corpus Barn acceptance remains unmeasured.
- Blocker: no existing read-only Barn command reports acceptance across all 1,950 stored programs.
- Next: synthesize issue-ready fixes and explicitly defer full spans and exported-API deletion pending product need.

## 2026-09-02 parser issue-publication checkpoint
- Key finding: live duplicate audit found closed #25 and #35 plus open #26 overlapping recursion, comments, and string formatting.
- Current state: no new issues published; four other parser findings have no title-search duplicate.
- Blocker: #35 records Q's prior `//` correction, contradicting current Toast `parser.y`; managed behavioral proof is required before refiling.
- Next: finish duplicate adjudication, resolve the comment contract, then publish only non-duplicate scoped issues.

## 2026-09-02 parser issue-publication completion checkpoint
- Key finding: managed Toast admission plus four comment rows passed, superseding #35's retraction with behavioral evidence.
- Current state: focused issues #287 through #292 are open and individually read back.
- Blocker: original recursion issue #25 still needs reopening because its pre-parse budget acceptance criterion remains unmet.
- Next: reopen and annotate #25, verify all seven open records, and stop without implementation.
