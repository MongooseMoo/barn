APPROVE

# Specification Review: Verb Language Ownership Specification Draft

This review evaluates the proposed specification modifications for Phase 1 of the Barn multilingual verb language cleanup.

## 1. Specification Conventions
The draft proposed in [spec-draft-verb-language-ownership.md](file:///C:/Users/Q/code/barn/prompts/spec-draft-verb-language-ownership.md) follows existing repository conventions:
- Modifications to [vm.md](file:///C:/Users/Q/code/barn/spec/vm.md) preserve the markdown tabular and ASCII flowchart style for the compilation pipeline (Lines 14-55) and match the Go signature formats for compiler structs and compilation cases (Lines 57-113).
- Updates to [statements.md](file:///C:/Users/Q/code/barn/spec/statements.md) (Lines 130-163) maintain standard Go interface conventions and markdown subsections.
- The rewrite of [AGENTS.md](file:///C:/Users/Q/code/barn/parser/AGENTS.md) (Lines 165-205) is structured as a clear, list-based package rule set.

## 2. Ownership Graph Consistency
The draft defines a clean, acyclic ownership graph with strict boundaries:
- **`parser`**: Owns MOO grammar, tokens, parsing, diagnostics, and canonical formatting. It constructs `verb.Program` but does not own its semantic types ([AGENTS.md:Lines 172-179](file:///C:/Users/Q/code/barn/parser/AGENTS.md)). It has no dependencies on `barn/types` or Barn runtime packages.
- **`verb`**: Language-neutral semantic owner of `verb.Program`, expressions, statements, operators, and source locations ([AGENTS.md:Lines 177-179](file:///C:/Users/Q/code/barn/parser/AGENTS.md), [statements.md:Lines 139-143](file:///C:/Users/Q/code/barn/spec/statements.md)).
- **`bytecode`**: Owns bytecode compilation, opcodes, instruction sets, and bytecode program representation ([vm.md:Lines 76-85](file:///C:/Users/Q/code/barn/spec/vm.md)).
- **`compiler`**: Owns the complete source-to-bytecode operations, caching, and diagnostics ([vm.md:Lines 64-74](file:///C:/Users/Q/code/barn/spec/vm.md)).
- **`task`/`scheduler`/`vm`**: Consume compiled bytecode only, carrying no parser syntax trees or semantic verb IR ([vm.md:Lines 47-50, 122-124](file:///C:/Users/Q/code/barn/spec/vm.md)).

## 3. Original MOO Source vs. Semantic IR
The specification maintains a clear distinction between raw source and semantic representations:
- **Original source** is defined as the exact persisted sequence of MOO lines, returned by `verb_code()` and written to the database ([vm.md:Lines 32-33](file:///C:/Users/Q/code/barn/spec/vm.md)).
- **Verb IR** is the language-neutral semantic meaning, which does not contain formatting, whitespace, comments, or Barn runtime values ([vm.md:Lines 34-37](file:///C:/Users/Q/code/barn/spec/vm.md)).
- **Canonical formatting** is defined as generating MOO source from semantic IR. Crucially, the draft guarantees that canonical formatting never replaces the original stored source in the database or for `verb_code()` ([vm.md:Lines 42-45](file:///C:/Users/Q/code/barn/spec/vm.md), [AGENTS.md:Lines 192-194](file:///C:/Users/Q/code/barn/parser/AGENTS.md)).

## 4. Queued-Task Source vs. Runtime State
The draft correctly isolates persisted queued tasks from live runtime structures:
- In-memory execution state (tasks, frames, and variables) refers strictly to compiled bytecode programs ([vm.md:Lines 47-48, 122-124](file:///C:/Users/Q/code/barn/spec/vm.md)).
- Persisted queued tasks in database records retain the original source code text, which is parsed and compiled exactly once upon database restoration ([vm.md:Lines 48-50, 124-127](file:///C:/Users/Q/code/barn/spec/vm.md)).

## 5. Tree-Walker and Executable-AST Elimination
The draft successfully eliminates tree-walking runtime logic without introducing any wrappers or fallback paths:
- The Go `Execute(vm *VM) error` method signature on statements is deleted from [statements.md](file:///C:/Users/Q/code/barn/spec/statements.md).
- Recursive statement-execution rules (such as `TryFinallyStmt.Execute`) are removed, deferring control flow entirely to compiled bytecode handler metadata and the VM interpreter ([statements.md:Lines 156-162](file:///C:/Users/Q/code/barn/spec/statements.md)).
- Compatibility bridges, adapters, or dual-AST paths are explicitly forbidden ([statements.md:Lines 145-147](file:///C:/Users/Q/code/barn/spec/statements.md), [AGENTS.md:Lines 195-196](file:///C:/Users/Q/code/barn/parser/AGENTS.md)).

## 6. Go Examples & Normalization Readiness
The Go structs and compiler dispatch descriptions are specification-level and lay the groundwork for later normalization phases:
- The introduction of `LoopContext` and scopes in `Compiler` ([vm.md:Lines 78-85](file:///C:/Users/Q/code/barn/spec/vm.md)) aligns with compiler-managed control flow.
- Statement and expression compilers dispatch strictly on semantic variants ([vm.md:Lines 93-112](file:///C:/Users/Q/code/barn/spec/vm.md)).
- The draft correctly avoids any reference to JavaScript or runtime engine details, ensuring that Phase 1 stays strictly focused on language-boundary hygiene.

## 7. Cross-Reference Hygiene
Cross-references are preserved and correct:
- [grammar.md](file:///C:/Users/Q/code/barn/spec/grammar.md) remains the concrete MOO grammar specification.
- [database.md](file:///C:/Users/Q/code/barn/spec/database.md) Section 6 (verb source lines) and Section 7 (queued tasks and suspended VM state) are respected.
- The [multilingual-verb-language-cleanup-plan.md](file:///C:/Users/Q/code/barn/plans/multilingual-verb-language-cleanup-plan.md) continues to govern the project milestones.

## 8. Contradictions, Ambiguities, and Coexistence Analysis
There are no identified contradictions or loopholes that would allow the old and new compilation/execution paths to coexist:
- Strict convergence criteria demand a single parser path constructing `verb.Program` directly, with no parallel old/new code paths ([AGENTS.md:Lines 198-204](file:///C:/Users/Q/code/barn/parser/AGENTS.md)).
- **Note on `ConstantKey`**: The use of `ConstantKey` instead of the runtime `Value` in `Compiler.constants` ([vm.md:Lines 78-82](file:///C:/Users/Q/code/barn/spec/vm.md)) is consistent with the rule that literal payloads are converted to Barn runtime values only at the bytecode compiler boundary ([vm.md:Lines 96-97](file:///C:/Users/Q/code/barn/spec/vm.md)). This maintains package isolation between `verb` (semantic model) and Barn runtime packages.
