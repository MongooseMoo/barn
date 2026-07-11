# Discovery Task: Semantic Verb IR Normalization

## Worker Identity

You are the specification discovery worker. Do not modify specifications,
production code, tests, plans, or existing reports. Write only the requested
report.

## Scope

Phase 3 of `plans/multilingual-verb-language-cleanup-plan.md` normalizes five
semantic families without adding JavaScript or multilingual execution:

1. Conditionals: MOO `elseif` syntax lowers to nested semantic conditionals.
2. Exceptions: one semantic try node owns handlers and an optional finalizer.
3. Loops: distinct semantic range and collection loop variants.
4. Assignments: a sealed semantic assignment-target family.
5. Index boundaries: semantic first/last nodes independent of MOO tokens.

## Required Discovery

- Read the complete Phase 3 plan.
- Search all `spec/` documents and package charters for the five families and
  related ownership statements.
- Identify exact files and sections that require modification.
- List cross-references that must remain consistent, especially concrete MOO
  grammar versus semantic IR and bytecode/VM behavior.
- Identify obsolete syntax-shaped implementation descriptions that would
  permit the old and normalized variants to coexist.
- Define the terms and invariants the specification draft must establish.
- Do not invent JavaScript, language registries, frontend interfaces, adapters,
  compatibility paths, or implementation outside Phase 3.

## Output

Write `reports/spec-discovery-semantic-normalization.md` with source-cited
findings, target sections, cross-references, required terminology, and any open
questions. If repository evidence resolves the questions, state that no open
questions remain.
