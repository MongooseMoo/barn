# Verification Task: Semantic Verb IR Normalization Spec Integration

## Worker Identity

You are the independent integration verifier. Modify no file except the
requested report.

## Files to Inspect

- `prompts/spec-draft-semantic-normalization.md`
- `reports/spec-discovery-semantic-normalization.md`
- `reports/codex-spec-review-semantic-normalization.md`
- `reports/agy-spec-review-semantic-normalization.md`
- `plans/multilingual-verb-language-cleanup-plan.md`
- `spec/statements.md`
- `spec/operators.md`
- `spec/vm.md`
- `spec/grammar.md`
- `spec/types.md`
- `parser/AGENTS.md`

## Criteria

1. The corrected, reviewed draft is integrated completely.
2. Markdown headings, tables, and fences are valid and non-duplicated.
3. The top-level grammar reaches one concrete `try_statement`, which requires
   at least one handler or finalizer and preserves all existing exception-code
   forms.
4. The five normalized semantic families are sealed and old/new coexistence is
   forbidden.
5. Concrete MOO syntax and runtime behavior remain unchanged.
6. Cross-references remain consistent and no JavaScript, generic frontend,
   registry, adapter, alias, or compatibility path was introduced.

## Output

Write `reports/spec-verification-semantic-normalization.md`, beginning with
exactly `PASS` or `FAIL` and followed by source-cited findings. Modify no other
file.
