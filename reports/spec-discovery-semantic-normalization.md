# Specification Discovery: Semantic Verb IR Normalization

## Scope and governing decision

Phase 3 requires five semantic normalizations, one committed family at a time:
MOO `elseif` lowers to nested conditionals; one try node owns ordered handlers
and an optional finalizer; range and collection loops are distinct variants;
assignments use a sealed target family; and `^`/`$` lower to semantic first/last
boundaries (`plans/multilingual-verb-language-cleanup-plan.md:190-226`). This is
normalization of `verb` IR and bytecode lowering only. It does not add
JavaScript, language registries, frontend interfaces, adapters, compatibility
paths, or runtime redesign (`plans/multilingual-verb-language-cleanup-plan.md:5-13`,
`plans/multilingual-verb-language-cleanup-plan.md:55-76`).

The current ownership contract is already suitable: `parser` owns concrete MOO
grammar and spelling, constructs `verb.Program` directly, and must not own the
semantic node types (`parser/AGENTS.md:3-8`). MOO tokens and keyword spellings
remain in `parser`, while their language-neutral representation belongs in
`verb.Program` (`parser/AGENTS.md:10-19`). `bytecode` consumes `verb.Program`,
and runtime state contains bytecode rather than parser syntax or verb IR
(`spec/vm.md:17-49`). The Phase 3 draft must make the five normalized families
part of that same boundary, not introduce another layer.

## Current evidence and normalization gaps

The `verb` family is sealed at its expression and statement interfaces
(`verb/ir.go:11-30`), but four of the five current representations still encode
concrete MOO shapes or invalid combinations:

- `IfStmt` owns an `ElseIfs` slice and the syntax-specific `ElseIfClause` type
  (`verb/ir.go:322-337`); the compiler has a dedicated `elseif` loop
  (`bytecode/compiler.go:1863-1889`).
- The IR has `TryExceptStmt`, `TryFinallyStmt`, and
  `TryExceptFinallyStmt` (`verb/ir.go:387-421`), and bytecode has three separate
  lowering paths, including an inline duplication of try/except lowering for
  the combined form (`bytecode/compiler.go:2282-2513`).
- `ForStmt` selects collection versus range form by nullable `Container`,
  `RangeStart`, and `RangeEnd` fields (`verb/ir.go:349-361`); the compiler tests
  `RangeStart != nil` to choose its path (`bytecode/compiler.go:1945-1949`).
- `AssignExpr.Target` is an arbitrary `Expr` (`verb/ir.go:275-282`). The
  compiler switches on expression types, rejects invalid shapes, and separately
  recognizes a `ListExpr` as destructuring (`bytecode/compiler.go:770-823`).
  `ScatterStmt` and `ScatterTarget` form a second syntax-shaped destructuring
  surface (`verb/ir.go:423-438`).

Index boundaries are partly normalized already: `IndexBoundaryExpr` contains
the semantic `IndexFirst`/`IndexLast` enum (`verb/ir.go:140-146`,
`verb/ir.go:199-205`), and the parser constructs those nodes from MOO spelling
(`parser/parser.go:191-194`, `parser/parser.go:252-255`). The missing
specification work is to make this the required and exclusive representation
and to state where one-based/inclusive behavior is owned. Current VM-facing
names and descriptions still call the operation a `^/$` “marker”
(`bytecode/opcodes.go:114`, `vm/op_index.go:367-369`), which could allow token
spelling to survive below the frontend.

## Exact specification and charter targets

### `spec/statements.md`

Modify **Section 13, Implementation Boundary**, especially **13.1 Semantic
Statements** and **13.2 Compiler Control Flow** (`spec/statements.md:682-699`).
Add the exact semantic statement-family contract:

- conditionals have only condition, then-body, and optional else-body; an
  `elseif` is represented as another conditional in the else-body;
- one try statement owns its body, ordered handlers, and optional finalizer;
- range loops and collection loops are distinct statement variants, with no
  nullable discriminator fields;
- all variants are exhaustive members of the sealed `verb.Stmt` family.

Keep Sections 3, 4, 9, and 10 as the MOO language behavior cross-reference.
They correctly preserve source-order conditional evaluation
(`spec/statements.md:45-69`), collection and range behavior
(`spec/statements.md:91-280`), ordered first-match exception handling
(`spec/statements.md:485-540`), and finalizer precedence/guarantees
(`spec/statements.md:558-608`). Section 13.3 must continue to require finalizers
on normal completion, return, loop transfer, and exception propagation
(`spec/statements.md:701-707`), but should refer to the unified semantic try
node rather than a distinct semantic “try/finally statement.”

### `spec/operators.md`

Modify **Section 2, Assignment Operators**, particularly **2.1 Valid targets**
through **2.3 Range Assignment** (`spec/operators.md:53-127`), and **Section 6,
Scatter Assignment** (`spec/operators.md:225-263`). Add an implementation
boundary stating that every assignment carries one sealed semantic target, with
explicit variable, property, index, range, and destructuring variants. The
right-hand side remains an expression and assignment still returns its assigned
value. The grammar's accepted lvalues and the runtime errors described here do
not authorize arbitrary `verb.Expr` targets or a parallel scatter-only target
family.

Modify **14.3 Indexing** and **15.2 Index Markers**
(`spec/operators.md:808-825`, `spec/operators.md:845-864`) to distinguish MOO
notation from semantic IR: `^` and `$` are frontend spellings that lower to
semantic first and last boundary operations. Preserve single-index, inclusive
range, and error behavior; do not describe a lexer token or source marker as
bytecode input.

### `spec/vm.md`

Modify **12.3 Expression Compilation** and **12.4 Statement Compilation**
(`spec/vm.md:601-620`). Replace the open-ended “representative cases” wording
with the exact normalized families. In particular, the statement section must
require nested semantic conditionals, one semantic try variant, and distinct
range/collection loops. The expression section must require a sealed assignment
target family and semantic first/last boundary nodes. Exhaustiveness means no
old and normalized variant may coexist.

Keep the source/IR/bytecode ownership and artifact distinctions in Section 1
unchanged (`spec/vm.md:9-49`). Cross-reference **3.8 Looping**, **3.9 Exception
Handling**, and **3.11 Collection Operations** (`spec/vm.md:154-193`) only to
state that normalization does not change VM behavior or require a one-to-one
mapping between semantic nodes and opcodes. Cross-reference **Section 7** for
handler/finalizer runtime behavior (`spec/vm.md:383-438`). Phase 3 consolidates
compiler paths; it does not prescribe new opcodes or VM control structures.

### `spec/grammar.md`

Keep this document concrete-MOO-facing. Its `elseif` production
(`spec/grammar.md:48-56`), two `for` spellings (`spec/grammar.md:58-72`),
exception spellings (`spec/grammar.md:111-143`), assignment/scatter productions
(`spec/grammar.md:176-201`), and `^`/`$` productions
(`spec/grammar.md:281-299`) remain syntax authority.

Add semantic-lowering cross-references at those sections so readers cannot
infer a one-to-one semantic type from each production. Section 3 should also
present the concrete try grammar as one `try` construct that may contain
handlers and/or a finalizer, consistent with the already documented combined
form in `spec/statements.md:592-608`; its current pair of separately named
productions does not show that combined syntax.

### `spec/types.md`

Modify **5.3 Special Index Markers** (`spec/types.md:412-425`) only to separate
syntax from execution ownership. Preserve the established rules: collections
are one-based (`spec/types.md:366-386`), ranges are inclusive with strict bounds
(`spec/types.md:388-410`), semantic first resolves to index 1, and semantic last
resolves to the collection length at evaluation time. Rephrase “markers are
substituted” so it cannot mean frontend token substitution in bytecode or VM.

### `parser/AGENTS.md`

Extend the existing hard boundary at lines 10-24. State that the parser must
lower `elseif`, concrete try clauses, the two `for` spellings, assignment
syntax, and `^`/`$` directly into the normalized sealed families. It must not
emit `ElseIfClause`, multiple try statement types, nullable-form loop nodes,
arbitrary-expression targets, or token-shaped boundary values. The existing
no-adapter/no-parallel-path deletion rule remains controlling
(`parser/AGENTS.md:23-32`).

No other package charter was found for these owners. `verb/ir.go` has only the
package declaration comment establishing language-neutral semantic ownership
(`verb/ir.go:1-2`); `spec/vm.md` and `parser/AGENTS.md` are the substantive
ownership records for this draft.

## Cross-references that must remain consistent

1. **Concrete MOO grammar versus semantic IR.** `spec/grammar.md` and the
   language-behavior sections of `spec/statements.md`/`spec/operators.md` retain
   the user-visible spellings. Their lowering cross-references must point to one
   normalized semantic representation, never duplicate the concrete variants.
2. **Semantic IR versus bytecode compiler.** `spec/vm.md:596-620` must require
   exhaustive lowering of the sealed families. Dedicated old paths such as the
   current `elseif` loop, three try compilers, nullable `ForStmt` switch, and
   invalid-target type switch are forbidden end states.
3. **Bytecode versus VM behavior.** Preserve ordered handler matching and
   finalizer control-flow guarantees (`spec/statements.md:520-522`,
   `spec/statements.md:568-580`), range-loop inclusivity and upward iteration
   (`spec/statements.md:234-243`), collection snapshot/index/key behavior
   (`spec/statements.md:101-127`, `spec/statements.md:152-201`), and one-based,
   inclusive index/range behavior (`spec/types.md:366-425`). Normalizing IR does
   not change these semantics.
4. **Assignment syntax versus valid semantic targets.** The accepted lvalue
   list in `spec/operators.md:66-70` and destructuring forms in
   `spec/operators.md:225-263` must map to the sealed target family. Invalid
   assignment shapes must be absent from valid IR rather than rejected solely
   by compiler-time expression type switches.
5. **Formatting.** Canonical MOO formatting may render normalized conditionals
   back with `elseif` and boundaries back as `^`/`$`; formatting is semantic,
   not evidence that those concrete forms exist in IR (`spec/vm.md:37-40`,
   `parser/AGENTS.md:20-22`).

## Required terminology and invariants

- **Concrete MOO syntax:** tokens, keywords, clause spellings, and productions
  owned only by `parser`.
- **Semantic conditional:** condition, then-body, optional else-body. Every MOO
  `elseif` lowers to a semantic conditional nested in the preceding else-body,
  preserving source order and locations. `ElseIfClause` is not a semantic type.
- **Semantic try:** one statement with body, ordered zero-or-more handlers, and
  an optional finalizer. It must have at least one handler or a finalizer.
  Handler order, binding, matching, propagation, and finalizer precedence remain
  as specified. The three syntax-combination statement types must not coexist.
- **Collection loop:** label, value variable, optional index/key variable,
  collection expression, and body. It preserves collection snapshot and
  value/index/key semantics.
- **Range loop:** label, value variable, start expression, inclusive end
  expression, and body. It preserves upward-only inclusive iteration. A loop
  node cannot simultaneously be a collection and range loop.
- **Assignment target:** a sealed non-expression family. Its variants are
  variable, property, index, range, and destructuring targets. Destructuring
  preserves ordered required, optional/defaulted, and rest targets. Every target
  is valid by construction; bytecode lowering is exhaustive rather than a
  general-expression validity check.
- **Semantic index boundary:** a language-neutral first or last operation,
  independent of `parser.TokenType`, `^`, or `$`. First resolves to 1; last
  resolves against the current collection length. One-based indexing, inclusive
  ranges, strict bounds, and error behavior belong to bytecode/VM collection
  semantics, not frontend token types.
- **Exact convergence:** after each family, the old semantic type and its
  dedicated compiler path are deleted. Aliases, wrappers, shims, dual APIs, and
  compatibility branches are not acceptable (`plans/multilingual-verb-language-cleanup-plan.md:95-110`,
  `plans/multilingual-verb-language-cleanup-plan.md:359-370`).

## Obsolete descriptions that must not survive the draft

- Treating `If/ElseIf/Else`, `Try/Except`, and `Try/Finally` in the language
  statement table as a list of semantic Go variants (`spec/statements.md:9-20`).
  They are concrete language forms; Section 13 must define the different,
  normalized semantic family.
- The “representative cases” language that explicitly leaves omitted statement
  and expression variants open-ended (`spec/vm.md:607-620`). It would permit
  old and normalized nodes to coexist despite sealed-family convergence.
- Separate try grammar productions without the combined concrete form
  (`spec/grammar.md:113-143`) if interpreted as separate semantic nodes.
- “Valid targets (lvalues)” without a semantic target ownership rule
  (`spec/operators.md:66-70`), which leaves arbitrary expression targets and a
  second scatter target system possible.
- “Special markers are substituted” (`spec/types.md:412-419`) and VM/code
  descriptions using `^/$ marker` below the frontend. These must describe
  frontend spelling lowering to semantic first/last operations.

## Open questions

No open questions remain. The Phase 3 plan fixes the normalized families and
forbids parallel compatibility paths; the existing grammar, statement,
operator, type, VM, and parser-charter evidence fixes the behavior and ownership
that the draft must preserve.
