# Spec Draft: Semantic Verb IR Normalization

## Target Files

- `spec/statements.md`
- `spec/operators.md`
- `spec/vm.md`
- `spec/grammar.md`
- `spec/types.md`
- `parser/AGENTS.md`

## Section to Modify: `spec/statements.md` Section 13

Replace Sections 13.1 and 13.2 with:

```markdown
### 13.1 Sealed Semantic Statements

Language-level statement meaning is represented by the sealed `verb.Stmt`
family. The MOO parser constructs only the normalized variants in this family:

- A **semantic conditional** contains a condition, a then-body, and an optional
  else-body. Each MOO `elseif` clause is another semantic conditional nested as
  the sole statement in the preceding else-body. There is no semantic
  `ElseIfClause`.
- A **semantic try** contains its body, ordered zero-or-more handlers, and an
  optional finalizer. It must contain at least one handler or a finalizer. The
  concrete try/except, try/finally, and try/except/finally spellings do not
  create separate semantic statement types.
- A **collection loop** contains its label, value variable, optional index/key
  variable, collection expression, and body.
- A **range loop** contains its label, value variable, start expression,
  inclusive end expression, and body.

Range and collection loops are distinct variants. No semantic loop uses nil
fields or a discriminator to select between both forms.

Semantic statement nodes are compiler input: they do not contain VM methods and
do not execute themselves. The MOO parser constructs them directly, with no
parser-owned executable AST, parser-to-IR adapter, old/new alias, or parallel
statement path.

### 13.2 Compiler Control Flow

The bytecode compiler exhaustively lowers the sealed statement family. Loop
labels, break and continue targets, exception handlers, and finalizer regions
are compiler bookkeeping rather than parser syntax or runtime semantic nodes.

Lowering preserves the source-language behavior specified above: conditional
branches remain ordered, handlers use first-match order, collection iteration
preserves value/index/key behavior, range iteration is upward and inclusive,
and finalizers retain their control-flow precedence. Normalization does not
require one semantic node to map to one opcode.
```

Replace Section 13.3 with:

```markdown
### 13.3 Finalizer Guarantee

The bytecode emitted for a semantic try with a finalizer must run the finalizer
on normal completion, return, loop control transfer, or exception propagation,
as specified in Section 10. The VM enforces this through compiled handler
metadata and control flow; semantic statement nodes do not recursively execute
try bodies, handlers, or finalizers.
```

## Section to Modify: `spec/operators.md` Sections 2 and 6

Add after the source-language assignment behavior in Section 2:

```markdown
### 2.4 Semantic Assignment Targets

Every semantic assignment contains a right-hand-side expression and one target
from the sealed `verb.Target` family:

| Target variant | Meaning |
|----------------|---------|
| Variable | Assign a local variable |
| Property | Assign a static or dynamic property of an object expression |
| Index | Assign one element of a collection expression |
| Range | Assign an inclusive range of a collection expression |
| Destructuring | Assign an ordered sealed family of variable bindings |

Targets are not expressions. Every constructed target is valid by shape, and
bytecode compilation exhaustively lowers target variants rather than accepting
an arbitrary expression and rejecting invalid lvalue types later.

The target family does not change source-language evaluation order, errors, or
the rule that assignment evaluates to the assigned value.

A destructuring target contains only members from the sealed `verb.Binding`
family:

| Binding variant | Payload |
|-----------------|---------|
| Required | Variable name |
| Optional | Variable name and optional default expression; no default means the MOO zero value |
| Rest | Variable name |

Bindings cannot contain property, index, range, destructuring, or other general
assignment targets. At most one rest binding is permitted, and binding order is
the source order.
```

Add to Section 6:

```markdown
Scatter syntax lowers to the destructuring member of the same sealed semantic
assignment-target family. It does not create a second statement-only target
model or a parallel assignment path.
```

## Section to Modify: `spec/operators.md` Sections 14.3 and 15.2

Add to Section 14.3 and replace the implementation wording in Section 15.2:

```markdown
MOO `^` and `$` are frontend spellings. In index or range position, the parser
lowers them directly to language-neutral **first** and **last** semantic
boundary expressions. Parser tokens and source marker spelling do not cross
into verb IR or bytecode compilation.

First resolves to one. Last resolves to the current collection length at the
point the index or range is evaluated. Single indexing remains one-based,
ranges remain inclusive, and all bounds and error behavior remain as specified
here and in `types.md`.
```

## Section to Modify: `spec/vm.md` Sections 12.3 and 12.4

Replace the final paragraph of 12.3 with:

```markdown
The sealed expression family includes semantic literals and operators,
collection/property/call expressions, semantic first/last boundary
expressions, and assignments whose target is a sealed non-expression family.
The compiler exhaustively handles every expression and target variant. An
arbitrary expression cannot stand in for an assignment target, and MOO token
types or `^`/`$` spelling cannot stand in for semantic operators or boundaries.
```

Replace Section 12.4 with:

```markdown
### 12.4 Statement Compilation

Statement compilation exhaustively dispatches on the sealed semantic statement
family and emits bytecode control flow. The normalized family contains nested
semantic conditionals, one semantic try variant with ordered handlers and an
optional finalizer, distinct range and collection loop variants, and the other
language-neutral statement variants defined by `verb`.

The old and normalized variants must not coexist: there is no semantic
`ElseIfClause`, no separate try/except/finally statement types, and no nullable
multi-form `ForStmt`. Statement nodes do not execute themselves. Normalization
does not prescribe new opcodes or require a one-to-one semantic-node-to-opcode
mapping; VM looping, exception, finalizer, and collection behavior remains as
specified in Sections 3 and 7.
```

## Section to Modify: `spec/grammar.md`

Add this semantic-lowering note after Section 2.1:

```markdown
**Semantic lowering:** `elseif` is concrete MOO syntax. Each clause lowers to a
semantic conditional nested in the else-body of the preceding conditional; it
does not create an `elseif` semantic type.
```

Add this note after Section 2.2:

```markdown
**Semantic lowering:** The collection and range productions lower to distinct
semantic loop variants. No semantic loop selects its form through nullable
fields.
```

In Section 1, replace the `try_except_statement` and `try_finally_statement`
alternatives with one `try_statement` alternative.

Replace the production summary in Section 3 with this concrete combined grammar
while retaining every currently accepted exception-code form:

```ebnf
try_statement   ::= "try" { statement }
                    ( except_clause { except_clause } [ finally_clause ]
                    | finally_clause )
                    "endtry"

except_clause   ::= "except" [ identifier ] "(" exception_codes ")"
                    { statement }
finally_clause  ::= "finally" { statement }

exception_codes ::= "any"
                  | "@" expression
                  | exception_code { "," exception_code }

exception_code  ::= identifier
                  | "error"
                  | string_literal
```

Add after that grammar:

```markdown
**Semantic lowering:** All three concrete combinations lower to one semantic
try statement containing an ordered handler list and an optional finalizer. At
least one handler or finalizer is required.
```

Add to the assignment/scatter grammar notes:

```markdown
**Semantic lowering:** Every accepted assignment form constructs one sealed
semantic assignment target. Scatter syntax constructs the destructuring target
variant; it does not create a separate semantic statement path.
```

Add to Section 7:

```markdown
**Semantic lowering:** `^` and `$` lower to language-neutral first and last
boundary expressions. The concrete token spelling remains parser-owned.
```

## Section to Modify: `spec/types.md` Section 5.3

Replace the substitution paragraph with:

```markdown
**Evaluation order:** MOO `^` and `$` syntax is lowered before bytecode
compilation to semantic first and last boundary expressions. During collection
evaluation, first resolves to index 1 and last resolves to the collection length
at that moment. The resolved values then participate in the normal one-based,
inclusive range and strict bounds rules. If a resolved range has `start > end`,
an empty list or string is returned as specified above.
```

## Section to Modify: `parser/AGENTS.md`

Add to `Hard boundaries`:

```markdown
- Lower MOO `elseif` clauses, concrete try clauses, collection/range loop
  spellings, assignment syntax, and `^`/`$` directly into their normalized
  sealed semantic families. Do not emit `ElseIfClause`, multiple semantic try
  statement types, nullable multi-form loop nodes, arbitrary-expression
  assignment targets, parser-token boundary values, or any old/new bridge.
```

## Cross-References

- Concrete MOO syntax remains in `spec/grammar.md` and the language-behavior
  sections of `spec/statements.md` and `spec/operators.md`.
- Collection indexing remains one-based and ranges inclusive per
  `spec/types.md`.
- VM handler/finalizer and loop behavior remains unchanged in `spec/vm.md`.
- Original source and canonical formatting remain separate per `spec/vm.md`
  Section 1.
- Phase 3 of `plans/multilingual-verb-language-cleanup-plan.md` remains the
  implementation control surface.

## Open Questions

None.
