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

- Pending for this iteration.

Next slice:

- Complete independent spec integration verification, commit Phase 1, reread
  the plan, then begin Phase 2 verb IR extraction.
