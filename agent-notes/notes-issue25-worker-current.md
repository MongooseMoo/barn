# Issue 25 parser-depth worker checkpoint

- Isolated worktree: `C:\Users\Q\code\.worktrees\barn-fix-25-parser-depth`
- Branch: `agent/fix-25-parser-depth`
- Exact base: `b3d730e8bd3579517124fd011126f0d514547ce2`
- Current test-only commit: `6311a4e test: define parser nesting depth contract`
- Baseline focused command passed: `go test ./parser ./verb ./bytecode ./compiler -count=1`
- Minimal RED command failed as expected because `verb.NestingDepthError` and the checked `parser.FormatMOO` signature do not exist yet.
- Current phase: expand the red commit to the complete 255/256/257/2560 source and direct-IR matrix before production edits.
- Contract clarification: flat additive, postfix, and elseif chains count as semantic depth and reject above 256; wide siblings above 256 total nodes remain accepted.
- Blockers: none.
- Next action: finish compiler diagnostic/cache and semantic validator boundary tests, amend the red commit, rerun/report RED, then implement.
