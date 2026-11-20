# Issue 50 checkpoint

## Findings and observations

- Issue #50 identifies eager recursive `fmt.Errorf` wrapping in `parseListExpr` and `parseMapExpr` as the source of quadratic error-message construction.
- The focused regression was red before the implementation: at nesting depth 1,000, list detail was 30,027 bytes and map detail was 25,027 bytes, versus 267 and 227 bytes at depth 8.
- The implementation returns the underlying child parse error directly at the six issue-identified list/map element, key, and value sites.
- The regression asserts that list and map error details are identical at shallow and deep nesting. Its final deep case is 32,000 opening tokens, matching the issue's reported workload.
- Focused parser tests and the full local CI-equivalent formatting, vet, build, and unit-test gates passed before increasing the regression depth from 1,000 to 32,000.
- The worktree contains extensive unrelated untracked files. Only `parser/parser.go` and `parser/parser_collection_error_test.go` belong to this task.

## State

- Branch: `agent/issue-50-bounded-parse-errors`
- Tracked implementation file modified: `parser/parser.go`
- Task test file untracked: `parser/parser_collection_error_test.go`
- TDD red evidence captured; green evidence captured at depth 1,000.

## Blocker

- None.

## Next action

- Run the focused regression at depth 32,000, rerun the final gates, audit only the two intended parser files, commit and push them, open a ready PR with literal `Closes #50`, watch required CI, normal-merge with `gh pr merge --merge`, then verify issue closure and a two-parent merge commit.
