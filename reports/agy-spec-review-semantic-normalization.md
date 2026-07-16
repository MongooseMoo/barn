APPROVE

# Specification Review: Semantic Verb IR Normalization Specification Draft

This review evaluates the proposed specification modifications for Phase 3 of the Barn multilingual verb language cleanup.

## 1. Concrete MOO Grammar vs. Semantic IR
The proposed specification preserves the concrete MOO grammar structure in [grammar.md](file:///C:/Users/Q/code/barn/spec/grammar.md) while enforcing that the frontend parsers lower them directly into language-neutral, normalized semantic IR representations:
- **`elseif` Clauses**: The EBNF production for `elseif` remains unchanged in [grammar.md](file:///C:/Users/Q/code/barn/spec/grammar.md#L48-L56), but is supplemented with a semantic lowering rule: it must compile into a nested [verb.Stmt](file:///C:/Users/Q/code/barn/verb/ir.go#L23-L26) conditional nested inside the preceding else-body, eliminating the need for a separate [ElseIfClause](file:///C:/Users/Q/code/barn/verb/ir.go#L330-L334) semantic type ([spec-draft-semantic-normalization.md:L167-L171](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L167-L171)).
- **Loop Spellings**: Collection and range loops are preserved as concrete EBNF productions in [grammar.md](file:///C:/Users/Q/code/barn/spec/grammar.md#L58-L72), but lower directly to distinct range and collection loop statement variants, avoiding nullable selector fields in the semantic loop node ([spec-draft-semantic-normalization.md:L173-L179](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L173-L179)).
- **Try Statements**: Concrete grammar for try statements combines the separate `try_except` and `try_finally` non-terminals into a unified `try_statement` production ([spec-draft-semantic-normalization.md:L181-L205](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L181-L205)) while ensuring that they lower directly to a single try statement variant containing an ordered handler list and optional finalizer ([spec-draft-semantic-normalization.md:L209-L212](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L209-L212)).
- **Scatter and Assignment**: Destructuring/scatter syntax continues to exist in EBNF ([grammar.md:L176-L201](file:///C:/Users/Q/code/barn/spec/grammar.md#L176-L201)) but lowers directly to the destructuring member of the sealed assignment-target family ([spec-draft-semantic-normalization.md:L214-L220](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L214-L220)).
- **Index Markers**: `^` and `$` are treated purely as frontend syntax spellings that the parser translates directly into language-neutral first and last boundary expressions ([spec-draft-semantic-normalization.md:L222-L227](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L222-L227)).

## 2. Sealed, Exhaustive, Language-Neutral Families
The specification defines five strictly normalized semantic families as part of the [verb](file:///C:/Users/Q/code/barn/verb/ir.go) contract:
- **Conditionals**: Nested conditional nodes represent `elseif` clauses in the else-body, eliminating [ElseIfClause](file:///C:/Users/Q/code/barn/verb/ir.go#L330-L334) entirely ([spec-draft-semantic-normalization.md:L22-L25](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L22-L25)).
- **Exceptions**: A unified try statement variant handles try-except, try-finally, and try-except-finally spellings, containing only its body, ordered zero-or-more handlers, and an optional finalizer, requiring at least one handler or finalizer ([spec-draft-semantic-normalization.md:L26-L29](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L26-L29)).
- **Loops**: Range loops and collection loops are defined as distinct variants without discriminator or nullable fields to choose their type ([spec-draft-semantic-normalization.md:L30-L36](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L30-L36)).
- **Assignment Targets**: Every assignment carries a target from the sealed [verb.Target](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L76) family (Variable, Property, Index, Range, Destructuring) ([spec-draft-semantic-normalization.md:L75-L84](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L75-L84)). Destructuring targets are restricted to contain only elements of the sealed [verb.Binding](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L93) family (Required, Optional, Rest) ([spec-draft-semantic-normalization.md:L93-L100](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L93-L100)).
- **Index Boundaries**: Syntactic markers `^` and `$` are lowered to language-neutral first and last boundary expressions ([spec-draft-semantic-normalization.md:L120-L123](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L120-L123)).

## 3. Coexistence Exclusion
To prevent dual compatibility paths, the draft explicitly rules out the concurrent existence of old and new variants:
- "The old and normalized variants must not coexist: there is no semantic `ElseIfClause`, no separate try/except/finally statement types, and no nullable multi-form `ForStmt`." ([spec-draft-semantic-normalization.md:L155-L157](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L155-L157)).
- The compiler structure is specified to exhaustively compile all expressions and statements without fallback aliases, shims, or wrappers ([spec-draft-semantic-normalization.md:L38-L41, L86-L88, L149-L154](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L38-L41)).
- [AGENTS.md](file:///C:/Users/Q/code/barn/parser/AGENTS.md) is updated to forbid emission of syntax-shaped IR remnants or old/new bridges ([spec-draft-semantic-normalization.md:L247-L252](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L247-L252)).

## 4. Behavior and Error Preservation
Language semantics are fully preserved by the normalization rules:
- **Execution semantics**: ordered conditional branches, first-match exception handling, collection key/value/index iteration, upward range loops, and finalizer precedence guarantees remain as specified ([spec-draft-semantic-normalization.md:L49-L53, L61-L65](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L49-L53)).
- **Index and range bounds**: Collection indexing remains one-based, range bounds are inclusive, and strict error checking (rather than clamping) is enforced ([spec-draft-semantic-normalization.md:L125-L129, L234-L240](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L125-L129)).
- **Assignment semantics**: Right-hand-side expression evaluation order, runtime errors, and assignment evaluation values are preserved ([spec-draft-semantic-normalization.md:L89-L91](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L89-L91)).

## 5. Complete Assignment Target Model
The sealed [verb.Target](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L76) family is complete:
- It eliminates the pattern of allowing arbitrary expressions as assignment targets ([spec-draft-semantic-normalization.md:L86-L88](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L86-L88)).
- Scatter assignment lowers to the destructuring member of [verb.Target](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L76), ensuring there is no parallel scatter-specific statement model or path ([spec-draft-semantic-normalization.md:L110-L113](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L110-L113)).
- Destructuring members only contain [verb.Binding](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L93) elements, which explicitly cannot contain nested property, index, range, or other general targets ([spec-draft-semantic-normalization.md:L102-L103](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L102-L103)), matching concrete MOO constraints and preventing invalid target nesting at compile-time.

## 6. Token/Spelling Isolation
The draft prevents syntactic details from leaking beyond the compiler boundary:
- MOO index markers `^` and `$` are lowered in the parser directly to language-neutral first and last boundary expressions ([spec-draft-semantic-normalization.md:L120-L123](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L120-L123)).
- The draft specifies: "MOO token types or `^`/`$` spelling cannot stand in for semantic operators or boundaries." ([spec-draft-semantic-normalization.md:L140-L141](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L140-L141)).

## 7. Cross-Reference Hygiene
Cross-references are preserved and correct:
- Modifies [statements.md](file:///C:/Users/Q/code/barn/spec/statements.md) Sections 13.1, 13.2, 13.3 ([spec-draft-semantic-normalization.md:L12-L66](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L12-L66)).
- Modifies [operators.md](file:///C:/Users/Q/code/barn/spec/operators.md) Sections 2, 6, 14.3, 15.2 ([spec-draft-semantic-normalization.md:L68-L129](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L68-L129)).
- Modifies [vm.md](file:///C:/Users/Q/code/barn/spec/vm.md) Sections 12.3, 12.4 ([spec-draft-semantic-normalization.md:L131-L161](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L131-L161)).
- Modifies [grammar.md](file:///C:/Users/Q/code/barn/spec/grammar.md) Sections 1, 2.1, 2.2, 3, 7 ([spec-draft-semantic-normalization.md:L163-L228](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L163-L228)).
- Modifies [types.md](file:///C:/Users/Q/code/barn/spec/types.md) Section 5.3 ([spec-draft-semantic-normalization.md:L229-L240](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L229-L240)).
- Modifies [AGENTS.md](file:///C:/Users/Q/code/barn/parser/AGENTS.md) ([spec-draft-semantic-normalization.md:L242-L252](file:///C:/Users/Q/code/barn/prompts/spec-draft-semantic-normalization.md#L242-L252)).

## 8. Avoidance of Unrelated Design/Redesign
The draft does not mention JavaScript or any other execution engine, plugin architecture, registry, or unrelated runtime optimizations. The modifications focus strictly on the normalization of existing MOO IR structures within the verb package and compilation phase boundaries, exactly as required by [multilingual-verb-language-cleanup-plan.md](file:///C:/Users/Q/code/barn/plans/multilingual-verb-language-cleanup-plan.md).
