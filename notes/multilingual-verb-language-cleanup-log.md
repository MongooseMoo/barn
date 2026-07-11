# Multilingual Verb Language Cleanup Fixed-Point Log

Started: 2026-07-11

Plan: `plans/multilingual-verb-language-cleanup-plan.md`

## Target architecture

- `parser` owns MOO syntax, parsing, diagnostics, and canonical MOO formatting.
- `verb` owns language-neutral executable verb meaning.
- `bytecode` owns IR-to-bytecode compilation and bytecode decoding.
- `compiler` owns MOO-source compilation, structured diagnostics, source
  attachment, and content-addressed caching.
- `db/store` persists original source.
- Runtime tasks consume compiled bytecode programs, not parser syntax or verb
  IR.

## Forbidden surfaces

- Executable operators represented by `parser.TokenType`.
- Parser AST types outside the MOO frontend.
- `bytecode` importing `barn/parser`.
- `bytecode.VerbProgram` and `bytecode.CompileVerb`.
- `task.Task.Code interface{}` carrying `[]parser.Stmt`.
- Scheduler first-run AST compilation.
- Duplicate source parse/compile paths.
- AST pseudo-disassembly in `builtins`.
- Compatibility wrappers, aliases, or fallback paths preserving those
  responsibilities.

## Search gates

- `rg '"barn/parser"' bytecode vm task scheduler builtins server cmd`
- `rg 'parser\.(Node|Expr|Stmt|TokenType)' --glob '*.go'`
- `rg 'VerbProgram|CompileVerb\(' --glob '*.go'`
- `rg 'Code\s+interface\{\}|Code\.\(\[\]parser\.Stmt\)' --glob '*.go'`
- `rg 'disassembleStmt|disassembleExpr|opToOpcode' builtins`

## Runtime gates

- `go test ./parser ./verb ./compiler ./bytecode`
- `go test ./vm ./task ./scheduler`
- `go test ./builtins ./server ./cmd/...`
- Database verb-program round trips.
- Eval, command, server-hook, login-hook, fork, and queued-task regression tests.
- `git diff --check`.
- The documented managed `moo-conformance-tests` command.

## Iteration 0 - Plan record

Slice read:

- `plans/multilingual-verb-language-cleanup-plan.md`

Surfaces:

- Cleanup plan
  - Disposition: keep
  - Owner after cleanup: `plans/`
  - Action: committed the user-approved plan before implementation.
  - Evidence: commit `c1c381e`.

Gate results:

- Pass: plan exists in Git history.

Commit:

- `c1c381e docs: plan multilingual verb language cleanup`

Next slice:

- Phase 0 baseline and characterization contracts.

## Iteration 1 - Phase 0 baseline and characterization

Slice read:

- `parser/*.go`
- `bytecode/*.go`
- `vm/bytecode_execution_test.go`
- `builtins/verbs_set_code_b2a_test.go`
- `scheduler/task_runtime_test.go`
- `db/format/verb_program_roundtrip_test.go`
- `db/format/writer_task_test.go`

Surfaces:

- Managed conformance baseline
  - Disposition: keep as evidence
  - Owner after cleanup: fixed-point record plus disposable JUnit artifact
  - Action: built Barn at `c1c381e` and ran the documented managed harness.
  - Evidence: `11335 passed, 126 skipped in 182.14s`; JUnit artifact
    `.tmp/multilingual-cleanup-baseline.xml`.
- Full Go test baseline
  - Disposition: keep as evidence
  - Owner after cleanup: fixed-point record
  - Action: ran `go test ./...`.
  - Evidence: all packages passed except the pre-existing unrelated
    `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent` failure in
    `barn/scheduler`. This failure is not cleanup scope and does not authorize
    an adjacent repair.
- Stale Barn test processes
  - Disposition: delete runtime state
  - Owner after cleanup: none
  - Action: stopped PIDs 151256 and 104336 only after verifying both were
    two-day-old `C:\Users\Q\code\barn\barn.exe` processes using Claude
    temporary conformance databases. They had prevented the required build from
    replacing `barn.exe`.
  - Evidence: the unchanged build and managed command succeeded afterward.
- Existing characterization coverage
  - Disposition: keep
  - Owner after cleanup: current focused test packages, migrated with their
    production owners in later phases
  - Action: verified coverage for empty programmed verbs, original database
    source, cache identity, eval/login/server execution, fork/suspend execution,
    queued-task serialization, and unknown-builtin diagnostic lines.
  - Evidence: `db/format/verb_program_roundtrip_test.go`,
    `bytecode/cache_test.go`, `vm/bytecode_execution_test.go`,
    `server/tls_listener_test.go`, `server/scheduler_login_test.go`,
    `scheduler/server_verb_task_test.go`, `db/format/writer_task_test.go`, and
    `builtins/verbs_set_code_b2a_test.go`.
- Complete source-compilation node-family contract
  - Disposition: keep, then move to the single compilation owner in Phase 4
  - Owner after cleanup: `compiler`
  - Action: added one source program spanning every current expression and
    statement family, original-source retention, line mapping, empty-program
    bytecode, and positional parse diagnostics.
  - Evidence: `bytecode/source_compilation_contract_test.go`.

Gate results:

- Pass: `go build -o barn.exe ./cmd/barn/`.
- Pass: managed baseline, `11335 passed, 126 skipped`.
- Expected baseline failure: `go test ./...` has only the recorded scheduler
  ID-collision failure.
- Pass: `go test ./bytecode`.

Commit:

- `377366b test: lock MOO compilation cleanup baseline`

Next slice:

- Commit Phase 0, reread the plan, then execute Phase 1 specification ownership
  changes.

## Iteration 2 - Phase 1 ownership specification

Slice read:

- `spec/README.md`
- `spec/vm.md`
- `spec/statements.md`
- `spec/grammar.md`
- `spec/database.md`
- `parser/AGENTS.md`
- `plans/multilingual-verb-language-cleanup-plan.md`

Surfaces:

- Related specification discovery
  - Disposition: keep as review evidence
  - Owner after cleanup: `reports/`
  - Action: inventoried compilation, statement, grammar, persistence, and parser
    charter contracts before drafting.
  - Evidence: `reports/spec-discovery-verb-language-ownership.md`.
- Ownership specification draft
  - Disposition: keep as review evidence
  - Owner after cleanup: `prompts/`
  - Action: drafted the parser, verb IR, bytecode, compiler, persistence, and
    compiled-only runtime ownership graph outside the live specifications.
  - Evidence: `prompts/spec-draft-verb-language-ownership.md`.
- External review gates
  - Disposition: keep as review evidence
  - Owner after cleanup: `reports/`
  - Action: addressed four Codex concerns about the enclosing source compiler,
    queued-task source, illustrative semantic-family examples, and semantic
    constant keys. Codex then approved. Gemini CLI was unavailable because its
    configured individual tier returned `UNSUPPORTED_CLIENT`; the user
    explicitly directed use of `agy` instead, and `agy` approved the corrected
    draft without concerns.
  - Evidence: `reports/codex-spec-review-verb-language-ownership.md` and
    `reports/agy-spec-review-verb-language-ownership.md`.
- Ownership specifications and parser charter
  - Disposition: keep
  - Owner after cleanup: `spec/` and `parser/AGENTS.md`
  - Action: integrated only the reviewed sections. The specification now makes
    `compiler` the only source-to-bytecode owner, `verb` the semantic owner,
    `bytecode` the IR lowering owner, and task/scheduler/VM compiled-only runtime
    consumers. Original verb source, queued-task `code`, canonical formatter
    output, IR, and compiled state remain distinct.
  - Evidence: `spec/vm.md`, `spec/statements.md`, and `parser/AGENTS.md`.

Gate results:

- Pass: Codex review verdict `APPROVE`.
- Pass: user-approved `agy` second-review verdict `APPROVE`.
- Pass: `git diff --check`.
- Pass: independent integration verification report.
  - Evidence: `reports/spec-verification-verb-language-ownership.md`.

Commit:

- `83c7099 docs: define verb language ownership boundary`

Next slice:

- Begin Phase 2 verb IR extraction.

## Iteration 3 - Phase 2 language-neutral verb IR

Slice read:

- `parser/ast.go`
- `parser/parser.go`
- `parser/parser_stmt.go`
- `parser/unparse.go`
- parser semantic assertion tests
- `bytecode/compiler.go`
- `bytecode/parser_literals.go`
- `bytecode/verbcache.go`
- direct parser and bytecode callers in `builtins`, `cmd`, `scheduler`, `task`,
  and `vm`

Surfaces:

- Parser-owned semantic AST
  - Disposition: move
  - Owner after cleanup: `verb`
  - Action: deleted `parser/ast.go` first and created `verb.Program` plus sealed
    semantic statement/expression families, language-neutral positions,
    semantic literal payloads, semantic operators, and index boundaries.
  - Evidence: `verb/ir.go`; `parser/ast.go` absent.
- MOO parser construction
  - Disposition: rewrite
  - Owner after cleanup: `parser`
  - Action: `ParseProgram` now constructs `*verb.Program` directly and
    `ParseExpression` constructs `verb.Expr` variants directly. Token-to-
    semantic operator mapping occurs only while parsing; there is no parser AST
    or AST-to-IR adapter.
  - Evidence: `parser/parser.go`, `parser/parser_stmt.go`.
- Parenthesized expressions
  - Disposition: delete
  - Owner after cleanup: none
  - Action: deleted `ParenExpr`; parentheses now control parsing precedence and
    return the contained semantic expression with its own source position.
  - Evidence: `parser/parser_expr_test.go`, `parser/parser_ternary_test.go`.
- Parser semantic tests
  - Disposition: move and rewrite
  - Owner after cleanup: `verb` for family contracts, `parser` for frontend
    output assertions
  - Action: moved family sealing/position tests to `verb/ir_test.go` and changed
    parser tests to assert explicit `verb` variants and semantic operators.
  - Evidence: `verb/ir_test.go` and parser test files.
- Bytecode semantic input
  - Disposition: rewrite
  - Owner after cleanup: `bytecode`
  - Action: the compiler now accepts only `verb` nodes and programs, switches on
    semantic operators/boundaries, and contains no MOO token cases. Literal-to-
    runtime-value conversion moved from `parser_literals.go` to
    `literal_values.go`.
  - Evidence: `bytecode/compiler.go`, `bytecode/literal_values.go`.
- Direct runtime and diagnostic callers
  - Disposition: rewrite pending later deletion phases
  - Owner after cleanup: `verb` for semantic code carried temporarily;
    compiled-only runtime ownership remains Phase 5
  - Action: changed remaining parser-node type references in builtins,
    scheduler, task, VM, and command code to the semantic owner. This does not
    preserve a parser alias or second node family.
  - Evidence: zero-hit parser-node search below.

Gate results:

- Pass: `go test ./parser ./verb ./bytecode`.
- Pass: `go test ./vm ./task ./builtins ./server ./db/format ./cmd/...`.
- Pass: focused scheduler execution, suspend/resume, and concurrent VM tests.
- Expected baseline failure: broad `go test ./scheduler` still fails only
  `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`.
- Pass, zero hits: `rg 'parser\.(Node|Expr|Stmt|TokenType)' --glob '*.go'`.
- Pass, zero hits: `rg 'ParenExpr|IndexMarkerExpr' --glob '*.go'`.
- Pass, zero hits: `rg 'TOKEN_' bytecode --glob '*.go'`.
- Pass, zero hits: runtime/parser imports from `verb`.

Commit:

- `de8072a refactor: move verb semantics out of parser`

Next slice:

- Commit the atomic Phase 2 ownership move, reread the plan, then normalize the
  conditional semantic family in Phase 3.

## Iteration 4 - Phase 3 normalized-family specifications

Slice read:

- `spec/statements.md`
- `spec/operators.md`
- `spec/vm.md`
- `spec/grammar.md`
- `spec/types.md`
- `parser/AGENTS.md`
- Phase 3 of the cleanup plan

Surfaces:

- Semantic normalization discovery and draft
  - Disposition: keep as review evidence
  - Owner after cleanup: `reports/` and `prompts/`
  - Action: identified and drafted the exact normalized conditional,
    exception, loop, assignment-target, and index-boundary contracts before
    implementation.
  - Evidence: `reports/spec-discovery-semantic-normalization.md` and
    `prompts/spec-draft-semantic-normalization.md`.
- Review corrections
  - Disposition: keep as review evidence
  - Owner after cleanup: `reports/`
  - Action: Codex initially rejected an exception grammar that admitted bare
    try, narrowed existing exception-code forms, was disconnected from the
    program grammar, and underspecified destructuring bindings. The draft was
    corrected to preserve all source forms and seal binding payloads; Codex and
    the user-approved `agy` reviewer then approved it.
  - Evidence: `reports/codex-spec-review-semantic-normalization.md` and
    `reports/agy-spec-review-semantic-normalization.md`.
- Integrated normalization contract
  - Disposition: keep
  - Owner after cleanup: `spec/` and `parser/AGENTS.md`
  - Action: specified nested semantic conditionals, one semantic try, distinct
    loop variants, sealed assignment targets/bindings, and semantic first/last
    boundaries while retaining concrete MOO grammar and runtime behavior.
  - Evidence: modified specifications plus
    `reports/spec-verification-semantic-normalization.md`.

Gate results:

- Pass: Codex review verdict `APPROVE` after corrections.
- Pass: user-approved `agy` second-review verdict `APPROVE`.
- Pass: independent integration verification verdict `PASS`.
- Pass: `git diff --check`.

Commit:

- `6ecf9a6 docs: specify normalized verb semantics`

Next slice:

- Commit the Phase 3 specification slice, then execute conditional
  normalization as the first bounded implementation family.

## Iteration 5 - Phase 3.1 conditional normalization

Slice read:

- `verb/ir.go` conditional types
- `parser/parser_stmt.go` conditional parsing
- `bytecode/compiler.go` conditional lowering
- `parser/unparse.go` conditional formatting

Surfaces:

- `ElseIfClause` and `IfStmt.ElseIfs`
  - Disposition: delete
  - Owner after cleanup: none
  - Action: removed the MOO-spelling-shaped semantic clause type and field.
- MOO `elseif` parsing
  - Disposition: rewrite
  - Owner after cleanup: `parser`
  - Action: concrete clauses are parsed and lowered directly into nested
    semantic `IfStmt` values in the preceding else-body, preserving clause
    positions and evaluation order.
- Conditional bytecode lowering
  - Disposition: consolidate
  - Owner after cleanup: `bytecode`
  - Action: deleted dedicated elseif compiler logic; recursive lowering of the
    sealed conditional variant handles nested else conditionals.
- Conditional semantic assertions
  - Disposition: keep
  - Owner after cleanup: parser frontend tests
  - Action: added a structural test proving two elseif clauses become nested
    conditionals with source lines 1, 3, and 5.

Gate results:

- Pass: zero Go hits for `ElseIfClause|ElseIfs`.
- Pass: `go test ./parser ./verb ./bytecode ./vm`.
- Pass: `go test ./builtins ./server ./cmd/...`.
- Pass: `git diff --check`.

Commit:

- `37743ec refactor: normalize semantic conditionals`

Next slice:

- Normalize the exception family.

## Iteration 6 - Phase 3.2 exception normalization

Slice read:

- `verb/ir.go` exception statement types
- `parser/parser_stmt.go` try parsing
- `bytecode/compiler.go` try lowering
- `parser/unparse.go` try formatting
- existing parser, bytecode, and VM exception tests

Surfaces:

- `TryExceptStmt`, `TryFinallyStmt`, and `TryExceptFinallyStmt`
  - Disposition: delete
  - Owner after cleanup: none
  - Action: replaced the three concrete-spelling-shaped semantic statements
    with one `TryStmt` containing ordered handlers and an optional finalizer.
- `ExceptClause`
  - Disposition: rewrite
  - Owner after cleanup: `verb.ExceptionHandler`
  - Action: retained semantic error matching, optional binding, body, and
    source position without retaining the MOO `except` spelling in the type.
- Try bytecode lowering
  - Disposition: consolidate
  - Owner after cleanup: `bytecode.compileTry`
  - Action: deleted the three duplicated lowering paths and emit the same
    nested handler/finalizer bytecode shape from the one semantic node.
- Try semantic assertions
  - Disposition: keep
  - Owner after cleanup: parser frontend tests
  - Action: added structural coverage proving handler-only, finalizer-only,
    and combined MOO forms all become `verb.TryStmt`.

Gate results:

- Pass: zero Go hits for the deleted statement, clause, and compiler names.
- Pass: `go test ./parser ./verb ./bytecode ./vm`.
- Pass: `go test ./builtins ./server ./cmd/...`.
- Pass: `git diff --check`.

Commit:

- `4413472 refactor: unify semantic try statements`

Next slice:

- Normalize the loop family.

## Iteration 7 - Phase 3.3 loop normalization

Slice read:

- `verb/ir.go` loop statement types
- `parser/parser_stmt.go` for-loop parsing
- `parser/unparse.go` loop formatting
- `bytecode/compiler.go` loop dispatch and lowering
- `spec/grammar.md` and the approved semantic loop contracts

Surfaces:

- Nullable multi-form `ForStmt`
  - Disposition: delete
  - Owner after cleanup: none
  - Action: replaced it with sealed `CollectionLoopStmt` and `RangeLoopStmt`
    variants whose required fields are valid by construction.
- MOO for-loop parsing
  - Disposition: rewrite
  - Owner after cleanup: `parser`
  - Action: directly constructs the matching semantic variant and preserves
    labels, value bindings, collection index/key bindings, expressions, and
    bodies without an adapter or compatibility type.
- Range index/key binding
  - Disposition: reject
  - Owner after cleanup: MOO grammar and parser
  - Action: corrected the grammar and parser to reject
    `for value, index in [start..end]`. The documented ToastStunt 2.7.3_5 WSL
    oracle returned `{0, {"Line 1:  syntax error"}}`, and Toast's `parser.y`
    has no two-binding range production.
- Loop bytecode lowering
  - Disposition: split by valid semantic variant
  - Owner after cleanup: `bytecode`
  - Action: deleted nullable-field dispatch, renamed the lowering functions
    with `gopls rename`, and retained the shared loop bookkeeping used for
    labels, break values, and continue jumps.
- Specification correction and review artifacts
  - Disposition: keep
  - Owner after cleanup: `spec/`, `prompts/`, and `reports/`
  - Action: restricted the optional second grammar identifier to collection
    iteration; Codex and `agy` approved, and independent verification passed.

Gate results:

- Pass: zero Go hits for `ForStmt`, its nullable range fields, and retired
  compiler names.
- Pass: `go test ./parser ./verb ./bytecode ./vm`.
- Pass: `go test ./builtins ./server ./cmd/...`.
- Pass: managed conformance in the `agy` review, 11,335 passed and 126 skipped.
- Pass: Codex spec review verdict `APPROVE`.
- Pass: user-approved `agy` spec review verdict `APPROVE`.
- Pass: independent spec integration verdict `PASS`.
- Pass: `git diff --check`.

Commit:

- `9bb5e4d refactor: split semantic loop variants`

Next slice:

- Normalize assignment targets.

## Iteration 8 - Phase 3.4 assignment-target normalization

Slice read:

- `verb/ir.go` assignment, statement, and expression families
- `parser/parser.go` assignment parsing
- `parser/parser_stmt.go` destructuring parsing
- `parser/unparse.go` assignment formatting
- `bytecode/compiler.go` assignment and scatter lowering

Surfaces:

- `AssignExpr.Target Expr`
  - Disposition: delete
  - Owner after cleanup: none
  - Action: replaced arbitrary expression targets with the sealed `verb.Target`
    family: variable, property, index, range, and destructuring.
- Collection assignment bases
  - Disposition: seal
  - Owner after cleanup: `verb.CollectionTarget`
  - Action: restricted index and range targets to variable, property, or nested
    index bases instead of accepting every target or expression shape.
- `ScatterStmt` and `ScatterTarget`
  - Disposition: delete
  - Owner after cleanup: none
  - Action: lowered both simple and extended scattering syntax into the same
    `AssignExpr` plus `DestructuringTarget` path using required, optional, and
    rest binding variants.
- Assignment validation
  - Disposition: move to frontend construction
  - Owner after cleanup: `parser`
  - Action: invalid lvalue expressions now fail while constructing a semantic
    target, before bytecode compilation.
- Multiple rest bindings
  - Disposition: reject
  - Owner after cleanup: `parser`
  - Action: the ToastStunt 2.7.3_5 WSL oracle returned
    `More than one '@' target in scattering assignment.`; the MOO parser now
    enforces the same invariant before constructing the destructuring target.
- Assignment bytecode lowering
  - Disposition: consolidate
  - Owner after cleanup: `bytecode.compileAssign`
  - Action: removed the statement-only scatter dispatch and retained assignment
    result, property/index/range writeback, optional/default binding, rest
    binding, and expression-statement behavior through one compiler entry.

Gate results:

- Pass: zero Go hits for `ScatterStmt`, `ScatterTarget`, `compileScatter`, and
  `Target Expr`.
- Pass: `go test ./parser ./verb ./bytecode ./vm`.
- Pass: `go test ./builtins ./server ./cmd/...`.
- Pass: `git diff --check`.

Commit:

- Pending for this iteration.

Next slice:

- Commit assignment-target normalization, then normalize index boundaries.
