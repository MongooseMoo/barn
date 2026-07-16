# Verification Task: Verb Language Ownership Specification Integration

## Worker Identity

You are the independent specification integration verifier. Do not modify any
file except the requested report.

## Files to Inspect

- `prompts/spec-draft-verb-language-ownership.md`
- `reports/spec-discovery-verb-language-ownership.md`
- `reports/codex-spec-review-verb-language-ownership.md`
- `reports/agy-spec-review-verb-language-ownership.md`
- `plans/multilingual-verb-language-cleanup-plan.md`
- `spec/vm.md`
- `spec/statements.md`
- `spec/grammar.md`
- `spec/database.md`
- `spec/README.md`
- `parser/AGENTS.md`

## Verification Criteria

1. The reviewed draft is integrated completely and accurately.
2. Markdown fences, tables, headings, and section numbering are valid, with no
   duplicate headings introduced.
3. The parser, verb IR, bytecode, compiler, persistence, and runtime ownership
   graph is consistent and admits no parallel AST or source-compilation path.
4. Original database verb source, canonical formatter output, queued-task
   `code`, verb IR, and compiled runtime state remain distinct.
5. Grammar and database cross-references still resolve without requiring a
   format change.
6. The integrated specifications add no JavaScript implementation, generic
   language interface, registry, adapter, compatibility bridge, or dual path.
7. The integration does not conflict with repository principles or the cleanup
   plan.

## Output

Write `reports/spec-verification-verb-language-ownership.md` beginning with
exactly `PASS` or `FAIL`, followed by source-cited findings. Do not modify
specifications, code, prompts, plans, or any other report.
