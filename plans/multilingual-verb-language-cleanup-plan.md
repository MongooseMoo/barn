# Multilingual Verb Language Cleanup Plan

## Purpose

Cleanly separate MOO syntax from executable verb meaning before adding any
second source language. This work must improve the existing MOO implementation,
remove syntax and semantic IR from the runtime, and establish one source-to-
bytecode path without changing database persistence or VM behavior.

This plan does not add JavaScript, a JavaScript engine, a generic language
plugin system, or a CST.

## Current State

Planning was performed on `master` at `c099532a13a81585cfeb346bfb9f6e69615c5426`.

The current pipeline is nominally:

```text
MOO source -> parser AST -> bytecode compiler -> bytecode program -> VM
```

In practice, compilation has several competing paths:

- `bytecode.CompileVerbBytecode` parses, compiles, and caches source.
- `bytecode.CompileVerb` returns `VerbProgram` containing `[]parser.Stmt`.
- Scheduler tasks carry parsed MOO AST through `task.Task.Code interface{}` and
  compile it on first execution.
- Eval, queued-task restoration, interactive programming, `set_verb_code`, VM
  verb calls, command verbs, server hooks, and command-line evaluation perform
  different subsets of parsing, validation, compilation, source attachment,
  and caching.
- `disassemble()` walks a small subset of the parser AST and emits pseudo-
  opcodes rather than decoding the compiled bytecode program.
- The parser AST mixes MOO concrete syntax with executable meaning. Examples
  include `parser.TokenType` operators, `ParenExpr`, `ElseIfClause`, three
  separate try-statement types, and nullable fields selecting different loop
  forms.

The database/store boundary is already correct: verbs persist their original
source lines and do not own parser AST or bytecode caches.

## Target Architecture

```text
Stored MOO source
       |
compiler.CompileMOO
       |
       +-- parser: MOO text -> verb.Program
       |
       +-- bytecode: verb.Program -> bytecode.Program
       |
       +-- content-addressed cache
                         |
                  task / scheduler / VM
```

Ownership after cleanup:

- `parser` owns the MOO grammar, tokens, parsing, diagnostics, and canonical MOO
  formatting.
- `verb` owns language-neutral executable verb meaning and source locations.
- `bytecode` owns IR-to-bytecode compilation, bytecode programs, opcodes, and
  bytecode decoding.
- `compiler` owns the complete MOO-source-to-bytecode operation, structured
  diagnostics, source attachment, and content-addressed caching.
- `db/store` continues to own original persisted verb source.
- `task`, `scheduler`, and `vm` consume compiled bytecode programs only. They do
  not carry parser syntax or verb IR.

The `verb` package is a real semantic owner, not an adapter. No generic
frontend interface, language registry, compatibility bridge, or parallel old
and new AST path is permitted.

## Invariants

The cleanup must preserve:

- Toast-compatible MOO parsing and runtime semantics.
- Original verb source returned by `verb_code()`.
- Original verb source in database checkpoints and round trips.
- Empty-but-programmed verb representation.
- Source-line attribution in errors, tracebacks, forks, queued tasks, and eval.
- Builtin-name validation and its error format.
- Verb compilation cache correctness.
- Command, server-hook, login-hook, eval, fork, suspend, resume, and queued-task
  behavior.
- Existing bytecode and VM semantics except where exact Toast oracle evidence
  requires a correction.

Canonical formatting is semantic rather than textual. Exact whitespace,
comments, spelling, and redundant parentheses are not part of the IR.

## Forbidden Final Surfaces

The following production surfaces must not remain when the cleanup is complete:

- `parser.TokenType` used as executable operator meaning.
- `parser.Node`, `parser.Expr`, or `parser.Stmt` outside the MOO frontend.
- `bytecode` importing `barn/parser`.
- `bytecode.VerbProgram`.
- `bytecode.CompileVerb`.
- `task.Task.Code interface{}` carrying `[]parser.Stmt`.
- Scheduler first-run AST compilation.
- Independent parse/compile paths in scheduler, VM, builtins, server, and
  commands.
- AST-based pseudo-disassembly in `builtins`.
- Aliases, wrappers, shims, fallback paths, or compatibility branches that
  preserve any of these surfaces under another name.

## Phase 0: Baseline and Contract Record

Before editing production code:

1. Verify the branch, revision, and tracked-file state.
2. Create the cleanup fixed-point record required by the deletion-first
   workflow.
3. Build the current Barn executable.
4. Run the documented managed conformance baseline:

   ```powershell
   uv run --project ..\moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}" --tb=no -q --junitxml <baseline-path>
   ```

5. Record current focused Go test results. The planning pass observed one
   unrelated existing failure:
   `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`.
   Do not absorb that defect into this cleanup. Record it as baseline debt and
   require zero additional failures.
6. Add characterization coverage for:
   - Parse and compile diagnostics, including source lines.
   - Every expression and statement family.
   - Empty programmed verbs.
   - Eval and command compilation.
   - Builtin-name validation.
   - Fork source extraction.
   - Queued-task restoration.
   - Database source preservation.
   - Cold and warm compilation-cache identity.

Commit the baseline tests and record before beginning structural changes.

## Phase 1: Specify the Ownership Boundary

Update the repository specifications before moving implementation ownership:

- Change `spec/vm.md` to specify:

  ```text
  MOO source -> MOO parser -> verb IR -> bytecode compiler -> VM
  ```

- Specify that original source and semantic IR are separate artifacts.
- Specify that tasks and scheduler state never own syntax or IR.
- Specify that canonical formatting does not preserve exact source text.
- Remove obsolete tree-walker descriptions such as `Stmt.Execute()` from
  `spec/statements.md`.
- Update the parser package charter to state that it may produce `verb.Program`
  but does not own executable semantic types or runtime values.

Commit the specification slice independently.

## Phase 2: Extract the Language-Neutral Verb IR

Delete the parser-owned semantic AST first, then repair callers onto the real
semantic owner.

1. Add package `verb` containing:
   - `Program`.
   - Sealed statement and expression node families.
   - Language-neutral source locations.
   - Semantic unary and binary operator enums.
   - Literal kinds and payloads without importing Barn runtime values.
   - Semantic first/last index-boundary operations.
2. Make the MOO parser construct `verb.Program` directly. Do not introduce a
   parser-AST-to-IR adapter or retain two ASTs.
3. Change the bytecode compiler to consume `verb` nodes.
4. Change tests to assert the semantic representation.
5. Delete `parser/ast.go`.
6. Delete `ParenExpr`; parsing parentheses returns the contained semantic
   expression with the appropriate source location.
7. Prove that `verb` contains no parser token types and that `bytecode` no
   longer switches on MOO token spellings.

This is one atomic ownership-move commit. Do not leave parser and verb semantic
node families coexisting across commits.

## Phase 3: Normalize Syntax-Shaped IR Families

Perform one family per committed slice. Each slice ends in a committed kept
reduction or a full Git revert before the next begins.

### 3.1 Conditional normalization

- Lower MOO `elseif` clauses to nested semantic conditionals.
- Delete `ElseIfClause` and compiler handling dedicated to MOO spelling.

### 3.2 Exception normalization

- Replace `TryExceptStmt`, `TryFinallyStmt`, and `TryExceptFinallyStmt` with one
  semantic try node containing handlers and an optional finalizer.
- Consolidate the three bytecode compiler paths.

### 3.3 Loop normalization

- Replace nullable-field selection in `ForStmt` with distinct valid semantic
  range-loop and collection-loop forms.
- Preserve labels, value variables, index/key variables, break values, and
  continue behavior.

### 3.4 Assignment-target normalization

- Replace arbitrary expression assignment targets with a sealed semantic target
  family.
- Represent variable, property, index, range, and destructuring targets
  explicitly.
- Remove compiler-time type switches whose only purpose is rejecting impossible
  assignment shapes.

### 3.5 Index-boundary normalization

- Replace MOO `^` and `$` token markers with semantic first/last boundary nodes.
- Keep MOO's one-based and inclusive range behavior in the bytecode/VM semantic
  owner, not in frontend token types.

After each slice, run focused parser, verb, bytecode, and VM tests and update
the fixed-point record.

## Phase 4: Establish One Source-Compilation Owner

Add package `compiler` as the owner of complete source compilation.

`compiler.CompileMOO` must own:

- Joining source lines.
- MOO parsing.
- Structured syntax and compile diagnostics.
- IR-to-bytecode compilation.
- Attaching original source lines to the bytecode program.
- Content-addressed caching.

Move raw-source caching out of `bytecode`; bytecode is not the owner of source
language identity or parsing.

Keep diagnostics structured through the pipeline with source location and
message. Do not format diagnostics to strings and later reconstruct meaning
from those strings.

`bytecode` should expose compilation from `verb.Program` only. It must not
import the MOO parser.

## Phase 5: Remove IR and Syntax from Runtime

Delete `VerbProgram` and `CompileVerb` first. Use resulting compiler failures as
the caller inventory and migrate all callers to `compiler.CompileMOO`.

Required convergence surfaces:

- Command verbs.
- Server-initiated hooks.
- Login hooks.
- Eval.
- `set_verb_code`.
- Interactive programming.
- Queued-task restoration.
- VM verb calls and `pass()`.
- Command-line expression evaluation.
- Diagnostic and dump commands.

Then:

1. Replace `Task.Code interface{}` with a typed compiled-program field.
2. Make `NewTaskFull` and scheduler task creation accept a compiled program.
3. Delete scheduler first-run AST compilation.
4. Compile before task creation at each source boundary.
5. Ensure queued tasks compile their persisted source once during restoration.
6. Keep forked tasks on extracted bytecode programs without reparsing source.

This phase is complete only when scheduler, task, VM, builtins, server, and
commands have no parser or verb-IR imports.

`Task.BytecodeVM interface{}` and `ForkInfo.Body interface{}` are explicitly out
of this plan's language-boundary scope. They are runtime package-cycle problems
and must be recorded as deferred rather than silently treated as complete.

## Phase 6: Replace AST Pseudo-Disassembly

`disassemble()` behavior is a conformance surface. Before changing it, verify
exact behavior using the documented WSL Toast oracle in
`reports/toast-oracle-wsl.md`.

If Toast verification is blocked, stop this phase and report the blocker.

After verification:

1. Compile the target verb through `compiler.CompileMOO`.
2. Decode the actual `bytecode.Program`.
3. Match Toast's verified output shape.
4. Delete `disassembleStmt`, `disassembleExpr`, `disassembleLiteral`,
   `opToOpcode`, and `unaryOpToOpcode` from `builtins`.
5. Remove the parser/verb-IR dependency from `builtins`.

Commit this behavior slice independently.

## Phase 7: Rebuild Canonical MOO Formatting

Replace `UnparseProgram` with a canonical MOO formatter over `verb.Program`.

Required property:

```text
Parse(Format(Parse(source))) == Parse(source)
```

Equality here is semantic IR equality excluding source locations. It does not
promise exact whitespace, comments, spelling, or redundant-parenthesis
preservation.

Requirements:

- Cover every IR node.
- Produce stable output under repeated parse/format cycles.
- Preserve precedence and associativity.
- Test the existing parser corpus plus representative database verbs.
- Keep `verb_code()` returning original stored source.
- Keep database writing original stored source.
- Do not switch production persistence to formatted output.

## Phase 8: Fixed-Point Verification

### Search gates

The following searches must produce zero production hits:

```powershell
rg '"barn/parser"' bytecode vm task scheduler builtins server cmd
rg 'parser\.(Node|Expr|Stmt|TokenType)' --glob '*.go'
rg 'VerbProgram|CompileVerb\(' --glob '*.go'
rg 'Code\s+interface\{\}|Code\.\(\[\]parser\.Stmt\)' --glob '*.go'
rg 'disassembleStmt|disassembleExpr|opToOpcode' builtins
```

Any production hit starts another cleanup iteration. Do not rename a forbidden
surface merely to satisfy a search.

### Runtime gates

Run after each relevant slice:

```powershell
go test ./parser ./verb ./compiler ./bytecode
go test ./vm ./task ./scheduler
go test ./builtins ./server ./cmd/...
git diff --check
```

Also run focused regression gates for:

- Database verb-program round trips.
- Empty programmed verbs.
- Eval and builtin eval.
- Command, server, and login verb execution.
- Fork creation and fork-source persistence.
- Queued-task persistence and restoration.
- Runtime source-line and traceback attribution.
- Compilation cache cold misses, warm hits, and source changes.

Before final completion, rebuild Barn and run the complete documented managed
conformance suite. Record exact counts and artifacts from the new run.

The pre-existing scheduler ID-collision failure does not authorize unrelated
repair. The cleanup must introduce no additional failures; final handling of
that existing failure requires separate scope or an explicit user decision.

## Git and Record Discipline

- Verify branch and tracked state before every source slice.
- Work on one bounded family at a time.
- Delete the old surface before repairing callers.
- Do not introduce wrappers, adapters, aliases, fallback paths, or dual APIs.
- A rejected slice is fully restored before another begins.
- A kept slice is tested, recorded, and committed before another begins.
- Required experiment and fixed-point records are committed repo-local files,
  not chat-only notes.
- Do not claim completion while any forbidden production surface or unchecked
  phase remains.

## Completion Criteria

The cleanup is complete only when:

- MOO parsing produces the language-neutral verb IR.
- Bytecode compilation consumes only that IR.
- One source-compilation owner performs parsing, diagnostics, source attachment,
  bytecode generation, and caching.
- Tasks and scheduler state carry compiled programs only.
- All old AST-bearing and duplicate compile paths are deleted.
- `disassemble()` is backed by actual bytecode and verified against Toast.
- Canonical MOO formatting round-trips semantically.
- Original verb source persistence remains unchanged.
- All search gates are zero-hit.
- All cleanup-specific runtime gates pass.
- The final managed conformance run is recorded with exact results.
- Every phase is completed or explicitly deferred by the user.

Only after these conditions hold should design or implementation of JavaScript
verb support begin.
