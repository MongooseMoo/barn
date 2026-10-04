# Independent source diagnostics (#291)

`ParseError` now retains `verb.Position` (line, column, byte offset), and compiler
diagnostics preserve it. `Diagnostic.Error` remains the existing line-only
MOO message. Unterminated comments retain their opening position; raw NUL
retains the actual illegal token position. Ordinary syntax errors retain the
offending token, including the existing unexpected-EOF line convention.

`ParseProgramWithDiagnostics` collects independent validation failures while
the normal grammar can still consume complete constructs. The convenience
`ParseProgram` delegates to it and returns joined causes. Both return no
program if any diagnostic exists. Compilation stops before lowering and never
caches failed source. Concurrent callers receive separate mutable diagnostic
slices; retries preserve the original positions, stages, and messages.

The parser owns MOO validation: loop exits and fork boundaries, index-boundary
contexts, floating-point literal overflow, assignment/scatter targets, rest
target counts, and unreachable EXCEPT clauses. Registry membership remains
compiler-owned through a callback at the closing token of a completed builtin
call. Incomplete calls emit the syntax error without a speculative unknown
builtin message. Nested completed calls report the inner call before the outer
one. Stable sorting by validation-token offset preserves that order and ties.

Validation positions are the tokens where the grammar checks a construct:
closing builtin parentheses, loop-exit semicolons, completed assignment RHS
tokens, and EXCEPT keywords. Literal and index-context diagnostics retain the
literal/boundary token. Semantic node positions remain available separately.

An invalid scatter expression prefix is discarded before count validation.
This avoids an extra rest-count error that would depend on an invalid prefix.
Optional scatter syntax converts the initial expression prefix before parsing
the remaining simple bindings. Ordinary syntax failures stop immediately;
there is no token skipping or semicolon/block recovery.

`set_verb_code` and interactive programming retain their existing integration:
they expose the complete ordered batch and keep the previous verb body on
failure. Interactive programming accepts a subsequent valid replacement.

## Oracle and regression evidence

The companion `parser_independent_diagnostics.yaml` scenarios were verified
first through managed Debian WSL Toast using
`/root/src/toaststunt/build-release/moo`. Every session included the canonical
`capability_admission` test and ran to its terminal summary.

The initial seven-scenario contract plus admission passed on Toast (8 passed,
14.97s) and failed on unchanged Barn (6 failed, 2 passed, 6.74s). Expanded cases
cover named loop exits, illegal assignment targets, unreachable EXCEPT clauses,
nested calls, incomplete arguments, and invalid scatter prefixes. Final oracle
selection: 14 passed in 23.56s. Final managed Barn selection: 14 passed in 7.45s.
The scatter regression also failed locally before the repair: it emitted an
extra count error on line 1 and stopped at a syntax error on line 2.

Focused Go tests check full offending positions, line-only formatting, ordered
independent batches, fail-fast syntax, no partial program/cache entries, final
token positions without a phantom EOF line, concurrent caller isolation,
`set_verb_code` body preservation, and interactive replacement/retry behavior.
The existing database parser/formatter census remains part of `go test ./...`.

The managed harness owns disposable working databases. The tracked Test.db
SHA-256 remains
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
No fresh full conformance baseline was run; PR and merge-queue CI provide the
full conformance gate.

Final checks:

```text
go test ./...                                      completed
go test -race ./parser ./compiler ./builtins ./server
  parser 96.570s; compiler 2.086s; builtins 60.852s; server 14.036s
go vet ./...                                      completed
staticcheck ./...                                 completed
DIAGNOSTICS_FULL_RACE_STATIC_OK
```
