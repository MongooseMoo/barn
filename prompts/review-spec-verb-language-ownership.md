# Review Task: Verb Language Ownership Specification Draft

## Context

This is Phase 1 of a deletion-first cleanup preparing Barn for a future second
verb source language. It does not add multilingual support. Specifications must
define the ownership boundary before implementation moves.

## Files to Read

- `prompts/spec-draft-verb-language-ownership.md`
- `reports/spec-discovery-verb-language-ownership.md`
- `plans/multilingual-verb-language-cleanup-plan.md`
- `spec/vm.md`
- `spec/statements.md`
- `spec/grammar.md`
- `spec/database.md`
- `parser/AGENTS.md`

## Review Criteria

1. Does the draft follow existing specification conventions?
2. Is the parser/verb/bytecode/compiler/runtime ownership graph internally
   consistent?
3. Does it preserve original MOO source as the database and `verb_code()`
   artifact while keeping semantic IR separate?
4. Does it correctly distinguish persisted queued-task source from live runtime
   state?
5. Does it eliminate tree-walker and parser-owned executable-AST language
   without introducing a compatibility bridge or generic language framework?
6. Are the proposed Go examples specification-level and consistent with the
   later normalization phases, without prematurely specifying JavaScript?
7. Are grammar and database cross-references preserved?
8. Identify contradictions, ambiguous ownership, missing invariants, or terms
   that would permit the old and new paths to coexist.

## Output

Write the requested review report. Begin with exactly one verdict:

- `APPROVE`
- `CONCERNS`
- `REJECT`

Then give source-cited findings. Do not modify specifications or production
code.
