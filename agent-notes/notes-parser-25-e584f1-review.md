# Parser depth implementation review checkpoint

- Audited exact clean head `e584f1ee0f0250d3a2d301729c06c419758ff842`, atop red commit `5ebb5ed5052bd7f01d416dbaadf2cde624beb34f` and live master `b3d730e8bd3579517124fd011126f0d514547ce2`.
- Live issue #25 contract remains the review source; no historical project Markdown was read.
- Current implementation uses one immutable `verb.MaxNestingDepth = 256`, combined statement/expression parser depth, an iterative semantic validator, compiler-entry validation, and validation-before-formatting.
- Current semantic traversal covers all existing expression, target, binding, statement, handler/finalizer, and fork edges. The node-type census test compares the validator list to all `Position` receiver types declared in current `verb/ir.go`.
- Postfix-assignment lowering was made iterative, and flat additive/postfix/elseif shapes are rejected by post-parse semantic validation before recursive compiler/formatter entry.
- Compilation failure returns no program, one SyntaxStage `syntax error` diagnostic with deterministic `NestingDepthError` detail, and does not populate the compiler cache.
- Formatter API is now `FormatMOO(*verb.Program) ([]string, error)` and validates before allocating output. All current repository call sites were updated; `verb_code` falls back to raw stored source if parsing or formatting fails.
- Focused packages and their race variants are green: `./verb ./parser ./bytecode ./compiler ./builtins`.
- The database corpus formatter test opens the tracked fixture read-only, builds an in-memory store, reads at most 50 parseable verbs deterministically, and left the audited worktree clean.
- No actionable finding yet. Remaining review: test-diff integrity, full-scope audit, performance/future-edge risks, and final verdict.
