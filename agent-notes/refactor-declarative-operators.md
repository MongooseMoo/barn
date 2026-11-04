# Declarative binary operator parsing

## Goal

Execute Q's approved bounded parser cleanup in an isolated worktree, measure
speed, and evaluate the larger authoritative-grammar direction. Q authorized
execution with "Fully execute (worktree)" after the staged recommendation.

## State

Branch: refactor/declarative-operators, based on integrated origin/master.
The binary parser now consumes one operator record. The old mapping helper and
binary-token case list are deleted. The round-trip test now covers every pair
of binary operators on either side, and the direct parser benchmark is added.
Baseline parser/compiler binaries were built before the production edit.
PR #599 is open. Initial hosted checks passed: Build & unit tests (2m11s),
MOO conformance suite (10m41s), Serializable histories and negative control
(2m43s). CI run 41917659777 and Elle run 41917659902 record those results.
Ten alternating before/after pairs are saved in experiments/declarative-operators
with benchstat and methodology. No significant timing regression was observed;
nested parsing was 4.97% lower time and uncached compilation 3.98% lower time.
Corpus/malformed timing differences were not significant; all allocation
counts match. These shared-workstation results are not a server throughput
claim. Final CI after including the evidence is pending. Preserve unrelated
work in the main checkout. No language behavior change is intended.

## Decisions and why

Smallest change: let binaryOperatorSpecs decide binary membership, binding
power, associativity and IR operator through one lookup per infix iteration.
Delete semanticBinaryOperator and the separately enumerated binary-token case.
Keep explicit assignment, ternary, postfix and contextual parsing. Start with
the existing lookup implementation; do not add indexed storage without a
measured reason. Estimate roughly 100-200 changed lines including measurement
and focused coverage.

Owner: parser/operator.go holds concrete MOO binary operator metadata.
parser/parser.go consumes it; parser/format.go already uses it for rendering.
No new parser framework or second grammar is part of this change.

## Cleanup iteration 1

- Read parser/AGENTS.md, parser.go, operator.go, operator_roundtrip_test.go,
  relevant arithmetic/bitwise/ternary tests, diagnostics.go, format.go,
  ir/ir.go, database_census_test.go and .github/workflows/ci.yml.
- Keep binaryOperatorSpec, associativity constants, binaryOperatorSpecs and
  token/semantic lookup functions: these describe or access the actual owner.
- Delete semanticBinaryOperator: the selected record already carries meaning.
- Consolidate binary recognition into the record lookup; delete the token list.
- Reduce precedence to handling the remaining non-binary infix forms.
- Preserve all other parser code and formatter behavior in this bounded slice.
- Search for the deleted helper and separate binary-token list after editing.
- Existing same-precedence tests cover all binary operators; extend their
  round-trip combinations across precedence groups using the existing helper.
- Benchmark direct parsing of all tracked Test.db verbs and deep/malformed
  expressions. Retain failures instead of selecting only accepted verbs.
- Ordinary tests, lint and managed MOO conformance run in PR CI, not locally.

## Surprises

The first benchmark invocation supplied unquoted dotted flags to PowerShell,
which passed an unintended -test argument. The binary rejected the command
before measurements. Quoting every native flag corrected the invocation.
That failed invocation is excluded from benchmark samples. Both variants
emit the same tracked database startup-repair warnings during untimed setup.

## Full grammar evaluation

The operator cleanup reaches the chosen boundary without a new representation.
A complete grammar export is a separate, larger change: the authored grammar
must generate or drive the production parser, and the exporter must consume
that same owner. Exporting an independently maintained EBNF file would retain
the current drift problem. The existing operator table can already be shared
without converting the whole parser.

A generated Go parser is a viable direction to investigate if a full grammar
artifact is required. Goyacc documents generation of Go parsers and semantic
actions (https://pkg.go.dev/golang.org/x/tools/cmd/goyacc). This evaluation does
not select it or claim its performance for Barn. Replacing this parser with a
custom runtime grammar interpreter would introduce a new mechanism without
evidence that it is needed.

The local ToastStunt source provides a concrete example to study:
C:/Users/Q/src/toaststunt/CMakeLists.txt:128 runs bison_target on src/parser.y;
that grammar declares operator precedence, statement/expression productions,
and embedded actions for assignment validation, loop scope and index context.
This source inspection establishes a reference representation, not a new
oracle run or a performance comparison with Barn. A Go adaptation would need
Barn's existing ir.Program construction and diagnostic contracts, rather than
copying the C AST or its allocation/registry assumptions.

The conversion would have to preserve these current concrete contracts:

- parser/parser_stmt.go:53-139 lowers elseif clauses directly to nested ir.IfStmt.
- parser/parser_stmt.go:234-319 distinguishes range/collection loops and
  constructs the corresponding existing IR type.
- parser/parser_stmt.go:351-355 clears sourceLoops while parsing a fork body.
- parser/diagnostics.go:149-164 validates enclosing loop names.
- parser/parser.go's parseIndexOperands scopes first/last index markers;
  lowerAssignmentTarget produces the existing sealed target family.
- parser/parser_stmt.go:623-648 scans an independent lexer cursor to distinguish
  list syntax from optional scatter syntax. A generator must resolve the
  corresponding ambiguity rather than treating all brace forms identically.
- parser/diagnostics.go:40-89 distinguishes stored-source builtin lowering,
  independent validation diagnostics, syntax failures and final nesting checks.
- parser/format.go remains the concrete syntax printer over ir.Program.

A plain context-free production dump cannot on its own express legal loop
targets, builtin admission or executable type/value choices. A future fuzz
consumer needs context-aware generation in addition to syntax productions.
Generated input from the production grammar also shares its mistakes; raw
source mutation, known regressions and the independent managed MOO oracle
remain necessary evidence.

Recommendation: retain the existing recursive parser after this cleanup. If
the complete grammar becomes the next selected deliverable, evaluate a
generated-parser candidate against the full stored corpus, malformed inputs,
diagnostic positions, nesting limits and managed conformance before selecting
it. Compare direct parsing and uncached compilation as well as allocations.
No full rewrite is warranted by the fuzzing hypothesis alone, and no generated
parser performance result has been measured here.

## Code review

Reviewed the published PR #599 diff against master. The production diff only
changes binary dispatch: for binary tokens, the same table supplies precedence,
associativity and semantic enum; for other tokens, the previous switch supplies
the same binding power. The former loop condition is now an explicit break.
Right-associative versus left-associative recursion is unchanged. No issue
found in this source review; CI and measurements remain separate requirements.
No review was posted to GitHub.

## Next step

Include the grammar assessment and measurement evidence with the substantive
task commit and update the PR description. Required CI must pass on that final
tree; reuse the completed code review because code is unchanged. Then
normal-merge through the queue, integrate master, and remove this task's
worktree/branch. No full grammar migration was selected or implemented.
