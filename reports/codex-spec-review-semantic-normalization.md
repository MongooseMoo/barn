APPROVE

1. The draft keeps concrete MOO syntax in the grammar while requiring direct
   lowering into language-neutral semantic IR. `elseif`, the two loop
   spellings, the combined try syntax, scatter syntax, and `^`/`$` remain
   concrete productions, while their semantic-lowering notes explicitly forbid
   corresponding syntax-shaped IR variants
   (`prompts/spec-draft-semantic-normalization.md:163-227`,
   `parser/AGENTS.md:3-24`).

2. The normalized statement shapes are sealed and exhaustive for the affected
   families: nested conditionals replace `ElseIfClause`; one try variant owns
   ordered handlers and an optional finalizer; and collection and range loops
   are distinct variants without nullable form selectors
   (`prompts/spec-draft-semantic-normalization.md:17-53`,
   `prompts/spec-draft-semantic-normalization.md:147-160`). The draft also
   expressly forbids old and normalized variants from coexisting
   (`prompts/spec-draft-semantic-normalization.md:155-157`), matching Phase 3's
   deletion-first requirements
   (`plans/multilingual-verb-language-cleanup-plan.md:189-225`).

3. The combined try grammar now requires either one-or-more handlers, optionally
   followed by a finalizer, or a finalizer alone; it cannot admit a bare try.
   It retains `any`, dynamic `@ expression`, and the existing identifier,
   `error`, and string exception-code forms, and Section 1 is explicitly updated
   to reference the combined nonterminal
   (`prompts/spec-draft-semantic-normalization.md:181-212`,
   `spec/grammar.md:26-42`, `spec/grammar.md:111-143`).

4. The assignment model is one sealed non-expression target family containing
   exactly variable, property, index, range, and destructuring variants. The
   destructuring member family is separately sealed to required, optional, and
   rest variable bindings; it excludes nested general targets and preserves the
   single assignment path
   (`prompts/spec-draft-semantic-normalization.md:73-113`). This covers the
   existing lvalue and scatter forms without allowing arbitrary expressions as
   targets (`spec/operators.md:53-127`, `spec/operators.md:225-263`).

5. The behavioral contracts are preserved by explicit reference: ordered
   conditional and handler selection, collection value/index/key behavior,
   upward inclusive ranges, and finalizer precedence remain unchanged
   (`prompts/spec-draft-semantic-normalization.md:43-65`). Assignment retains its
   evaluation order, errors, and value result
   (`prompts/spec-draft-semantic-normalization.md:86-104`), consistent with the
   existing operator contract (`spec/operators.md:53-127`).

6. First/last ownership is correctly separated from MOO spelling. The parser
   lowers `^` and `$` to semantic boundary expressions before bytecode
   compilation; first resolves to one and last to the collection length at
   evaluation time, while one-based indexing, inclusive ranges, strict bounds,
   and existing errors remain intact
   (`prompts/spec-draft-semantic-normalization.md:115-129`,
   `prompts/spec-draft-semantic-normalization.md:229-240`,
   `spec/types.md:366-425`).

7. The draft stays within the semantic-normalization gate. It adds no
   JavaScript, generic frontend interface, registry, adapter, alias,
   compatibility path, opcode redesign, or parallel runtime surface
   (`prompts/spec-draft-semantic-normalization.md:38-41`,
   `prompts/spec-draft-semantic-normalization.md:155-160`,
   `plans/multilingual-verb-language-cleanup-plan.md:5-11`,
   `plans/multilingual-verb-language-cleanup-plan.md:59-74`).
