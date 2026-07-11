# Parser Package Rules

The `parser` package owns the MOO language frontend: grammar, tokens, parsing,
source diagnostics, and canonical MOO formatting.

The parser constructs `verb.Program` directly but does not own the semantic
node types. The `barn/verb` package is the language-neutral semantic owner; it
is not a Barn runtime package.

Hard boundaries:

- Do not import `barn/types` or any other Barn runtime package from `parser`.
- Do not construct runtime values in `parser`.
- Do not encode truthiness, map-key validity, VM behavior, database behavior, server behavior, or builtin behavior here.
- Keep MOO token spelling, keyword recognition, `ANY`, and error-name syntax in
  `parser`; construct their language-neutral semantic representation in
  `verb.Program`.
- Parse MOO literal spelling directly into semantic literal kinds and payloads
  owned by `verb`. Runtime conversion remains outside both `parser` and `verb`.
- Lower MOO `elseif` clauses, concrete try clauses, collection/range loop
  spellings, assignment syntax, and `^`/`$` directly into their normalized
  sealed semantic families. Do not emit `ElseIfClause`, multiple semantic try
  statement types, nullable multi-form loop nodes, arbitrary-expression
  assignment targets, parser-token boundary values, or any old/new bridge.
- Canonical formatting consumes `verb.Program` and emits deterministic MOO
  source. It does not promise exact whitespace, comments, spelling, or redundant
  parenthesis preservation and never replaces stored original source.
- Do not add compatibility adapters, senders, helpers, or wrapper APIs to
  preserve deleted parser-owned semantic APIs.

Deletion-first rule:

- When an old parser API exposes executable semantic or runtime concepts, move
  callers to the real owner and delete the old parser surface.
- A parser cleanup is not complete until one parser path constructs
  `verb.Program` directly, with no parser-owned semantic AST, AST-to-IR adapter,
  or parallel old/new path.
