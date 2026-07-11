# Review Task: Semantic Verb IR Normalization Specification

## Context

This is the specification gate for Phase 3 of a deletion-first parser cleanup.
It normalizes existing MOO semantic IR families but adds no JavaScript or
multilingual execution.

## Files to Read

- `prompts/spec-draft-semantic-normalization.md`
- `reports/spec-discovery-semantic-normalization.md`
- `plans/multilingual-verb-language-cleanup-plan.md`
- `spec/statements.md`
- `spec/operators.md`
- `spec/vm.md`
- `spec/grammar.md`
- `spec/types.md`
- `parser/AGENTS.md`
- `verb/ir.go`

## Review Criteria

1. Does concrete MOO grammar remain distinct from normalized semantic IR?
2. Are conditionals, exceptions, loops, assignment targets, and index
   boundaries specified as sealed, exhaustive, language-neutral families?
3. Does the draft forbid old and normalized semantic variants from coexisting?
4. Are ordered branch/handler behavior, finalizer precedence, collection and
   range loop behavior, assignment evaluation, one-based indexing, inclusive
   ranges, and errors preserved?
5. Is the sealed assignment-target family complete without treating invalid
   expressions as targets or creating a second scatter path?
6. Does semantic first/last ownership avoid leaking parser tokens or MOO
   spelling into bytecode and VM contracts?
7. Are cross-references and existing specification conventions correct?
8. Does the draft avoid JavaScript, generic language interfaces, registries,
   adapters, aliases, compatibility paths, and unrelated runtime redesign?

## Output

Begin with exactly `APPROVE`, `CONCERNS`, or `REJECT`, then provide
source-cited findings. Modify no specification, production code, test, plan,
prompt, or existing report. Write only the report path named by the invoking
command.
