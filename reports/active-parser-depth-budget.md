# Active parser depth budget (#25)

The parser now shares one active syntax budget across recursive expression
calls and enclosing block statements. Unary/splice/catch prefixes,
parentheses, list/map operands, arguments, and right-associative expressions
can no longer descend through an attacker-controlled tail before the
post-parse depth check runs. Block statements consume the same budget;
terminal statements, terminal operands, and siblings do not accumulate it.
The separate parenthesis and statement counters are deleted.

The immutable ceiling remains `verb.MaxNestingDepth = 256`. A terminal
operand may occupy the final expression call below construct 256. Another
prefix construct is rejected before its token is consumed; additional
recursive calls reject before descending. All decrements are deferred so
errors and successful sibling parses release their active budget.

The iterative semantic validator remains required after parsing for flat
additive, postfix, and normalized elseif trees. The bytecode entrypoint and
both checked formatters continue to validate direct semantic IR before
recursive traversal. Their shared limit and node ownership are unchanged.

The initial regression failed on the unchanged parser: unary, list, map,
power, and assignment source at depth 2560 reached EOF before rejection.
Mixed statement/parenthesis depth 257 was incorrectly accepted. The new
tests check rejection while the cursor is still near the prefix, rather than
merely observing a late depth error. Tests also cover depths 255, 256, 257,
and 2560 for unary, parentheses, lists, maps, power, assignment, additive,
index/property postfix, if/while/try, elseif, mixed syntax, and direct IR.
Wide source includes 4096 sequential statements and 4096 list elements.

Over-limit compilation returns one SyntaxStage `syntax error`, deterministic
`maximum nesting depth exceeded (max 256)` detail, no program, and no cache
entry. Direct over-limit IR returns no bytecode or partial formatted output.
The bounded collection-error regression now compares two depth rejections;
shallow malformed syntax has a different bounded detail because its terminal
token is reachable, whereas deep malformed syntax stops at the limit first.

## Managed oracle evidence

Before implementation, managed Debian WSL Toast using
`/root/src/toaststunt/build-release/moo` verified the existing server-liveness
resource-limit scenario (2 passed in 6.29s including capability admission).
New test-owned generated-source scenarios verify acceptance at depth 256:
unary, parentheses, list, map, power, assignment, postfix, if, while, try, and
mixed if/parentheses/unary syntax. Toast: 5 passed in 10.89s including admission.
Barn's focused managed run of those scenarios and the existing resource-limit
case: 6 passed, 1578 deselected in 42.21s. No fresh full baseline was run.

These tests assert the verified portable acceptance boundary rather than
claiming identical server limits above it. Barn's exact 257 rejection is tested
through its Go API. Managed sessions own disposable databases and run to their
terminal summaries; no manual server or debug executable was used.

Final verification:

```text
go test ./...                             completed (tracked census included)
go test -race ./parser ./compiler ./verb  parser 8.712s; compiler 2.072s; verb 1.043s
go vet ./...                             completed
staticcheck ./...                        completed
go build ./...                           completed
ACTIVE_DEPTH_FULL_RACE_STATIC_BUILD_OK
```
