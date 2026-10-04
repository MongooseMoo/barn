# Semantic program validation (#290)

`verb.Validate` is the authoritative shape and depth check for a complete
program; `ValidateNode` applies it to a standalone semantic node. Compiler
lowering and both checked MOO formatter entrypoints require full validation.
`ValidateNesting` remains the explicit depth-only API used by the parser.

The validator covers every sealed expression, statement, target, collection
target, and binding family. It rejects unsupported enums, inactive literal
payloads, missing mandatory children and names, typed nil children, competing
static/dynamic names, conflicting or absent catch selectors, empty error-code
names, try statements without clauses, and multiple rest bindings. Optional
return values and optional binding/catch defaults remain legal when absent.
Empty lists, maps, blocks, and programs remain legal.

`EmptyStmt` replaces the nil-expression sentinel. The parser emits it for `;`;
the lowerer emits no operation and the formatter emits `;`. An `ExprStmt`
without an expression is now malformed rather than an alternative statement.
The old lowerer and formatter sentinel branches are deleted.

`ValidationError` carries a semantic position, field, and reason, and unwraps to
`ErrInvalidProgram`. Compiler diagnostics preserve that position. The iterative
graph walk checks a node before following its fields, retains the existing
depth budget and sibling accounting, and rejects embedded foreign families
without calling their potentially invalid `Position` method. Semantic cycles
reach the depth limit; foreign nodes are rejected before any non-semantic cycle
inside them can be followed.

MOO token spellings, keywords, error names, and representability stay in
`parser`; neither `parser` nor `verb` imports runtime value types. A formatter
receiver records the first typed `FormattingError` from nested expressions,
targets, bindings, or statements. It rejects invalid identifier spellings,
unknown error-name spellings, nonfinite floats without literal spellings, and
NUL source bytes. Specific causes remain available through `errors.Is` and
`errors.As`. Recursive placeholder output and the old unparse helper surface
are deleted.

`FormatMOOChecked` and `FormatMOOFullyParenthesizedChecked` return complete
source or nil source plus an error. Convenience formatters return nil on any
error, including one in a later statement; callers needing diagnostics use the
checked entrypoints. Formatting never changes stored original source.

## Regression evidence

The initial malformed direct-IR table demonstrated ignored contradictory
payloads, crashes, and diagnostic text emitted as source:

```text
nil_program/compiler: panic: invalid memory address or nil pointer dereference
typed_nil_expression/formatter: panic: invalid memory address or nil pointer dereference
index_value: got source ["1[<unknown expr: <nil>>];"], error <nil>
try_no_clause: got source ["try" "endtry"], error <nil>
FAIL github.com/MongooseMoo/barn/compiler
```

Review also reproduced a hang in the initial separate depth prepass when an
unknown embedded family contained a non-semantic self-cycle:

```text
TestUnknownSemanticFamilyDoesNotTraverseForeignCycles (1.00s)
validation followed a foreign non-semantic cycle
FAIL github.com/MongooseMoo/barn/compiler
```

Full validation now checks shape and depth in one walk. The final regression
table exercises the validator, lowerer, both checked formatters, and both
convenience formatters, requiring typed errors and no program/source/panic.
Additional tests cover valid members of every family and every operator,
4096 sibling statements, cycles, explicit empty statements, diagnostic
positions, and whole-output rejection of nested frontend spelling failures.
The existing complete database census continues to pin parser/formatter
acceptance and semantic preservation for the tracked fixture.

## Verification

Go commands run inside Debian WSL, with `GIT_DIR` and `GIT_WORK_TREE` pointing
at this linked worktree for the root repository-hygiene test:

```bash
go test ./...
go test -race ./verb ./compiler ./parser
go vet ./...
/root/go/bin/staticcheck ./...
```

Final output after combining the graph walk:

```text
ok github.com/MongooseMoo/barn/parser 18.477s
ok github.com/MongooseMoo/barn/verb 1.056s
ok github.com/MongooseMoo/barn/compiler 2.078s
ok github.com/MongooseMoo/barn/parser 95.742s
SEMANTIC_FINAL_FULL_RACE_STATIC_OK
```

Managed conformance uses the documented Linux command, the ordinary
`bin/barn` build, outbound-on profile, and canonical capability admission in
the same session. The focused selector is:

```text
capability_admission or parser_comment_contract or string_escape_authority or unparse
```

The conformance checkout is at `4c6d705a48ba868eda7f478a634f6f9c32cd1ecc`.
Its Test.db SHA256 is
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
The run includes admission, the existing precedence/unparse assertion, all
comment-contract scenarios, and the string-escape authority assertion:

```text
13 passed, 1567 deselected in 33.68s
```

This issue defines rejection of malformed Go IR, rather than introducing a
new server language expectation. Existing managed oracle-backed scenarios are
reused. No fresh full-suite baseline, manual server, tracked-database execution,
or conformance assertion change was needed. PR CI remains the full-suite gate.
