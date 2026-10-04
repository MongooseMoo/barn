# Canonical source rejects NUL

Issue #289. Canonical MOO source cannot represent an embedded NUL. Preserve all
other literal bytes, and reject invalid input rather than returning a valid prefix.

The lexer distinguishes its synthetic EOF sentinel from a zero byte inside the
input using the current offset. A raw NUL returns an illegal token at its actual
line/column/byte offset and advances the scanner. The same helper covers string
and escaped-string scanning and block comments. ParseProgram retains the first
lexical error, wraps the shared ErrNULInSource sentinel, and returns no program.
The private comment-error slot becomes a lexical-error slot; the #287 comment
diagnostic and opening position remain intact.

Each formatter statement is checked for NUL before its lines can be returned.
If any nested literal or later statement contains NUL, the complete result is nil
with ErrNULInSource. Both legacy formatter entrypoints discard that error and
return nil, never the already-formatted prefix. Checked formatting exposes the
deterministic message `NUL byte is not representable in MOO source`.
No generic semantic-shape validation or runtime-value conversion was added.

## Oracle evidence and scope

The canonical Debian WSL Toast source was inspected at parser.y:1298-1326:
the list-backed parser reader advances to the next line at C NUL. ast.cc:90-105
alloc_string uses strlen/strcpy. This cannot preserve an embedded zero byte in
a source string. The canonical executable was then verified through the managed
conformance harness, including admission, with a temporary programmer test:

```text
return {length(chr(0)), encode_binary(chr(0))};
expected/actual value: [0, ""]
test_capability_admission PASSED
zero_character_representation PASSED
2 passed in 6.51s
```

Executable: `/root/src/toaststunt/build-release/moo`.
Test.db SHA256:
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
The temporary test was removed after observation and did not alter the already
published conformance PR. This work defines a fail-closed Barn frontend API for
invalid canonical source; it does not claim Toast returns Barn's new NUL error
or change chr()/runtime binary-string behavior.

## Regression and validation evidence

Before implementation, checked formatting emitted source containing NUL with no
error. Parsing a valid statement followed by NUL returned a partial program;
NUL in a comment was misreported as an unterminated comment:

```text
FormatMOO: lines=["return 1;" "if (1)" "  return \"\x00\";" "endif"] error=<nil>
ParseProgram("return 1;\x00return 2;"): non-nil partial program, error=<nil>
ParseProgram("/*\n \x00 */ return 1;"): End of program while in a comment
```

The replacement tests assert nil output for checked, legacy and fully-parenthesized
formatters with NUL in nested lists/maps/if bodies and later statements. They
check all 255 non-NUL bytes by semantic parse-format-parse in both formatter modes,
including quote, backslash, CR/LF/tab, high bytes and invalid UTF-8 byte sequences.
Raw-source tests cover prefix/interior/final NUL, strings, escaped NUL, comments,
multiple NUL followed by another lexical error, exact token positions, first-error
retention, errors.Is(ErrNULInSource), no partial program, and eventual lexer EOF.

```text
Focused NUL and block-comment regressions: ok 0.551s
go test ./...: successful (parser 18.418s)
go test -race ./parser -count=1: ok 97.049s
go vet ./...: successful
staticcheck ./...: successful
NUL_FULL_RACE_STATIC_OK
```

The documented managed Barn command selected capability_admission,
parser_comment_contract and string_escape_authority from the companion
conformance worktree. It used bin/barn built from verified ./cmd/barn, disposable
Test.db, the barn-linux-testdb-outbound-on profile/config, and the stock WSL
oracle manifest. It includes the complete #287 contract and existing escape
authority rather than inventing a server diagnostic for raw NUL:

```text
12 passed, 1568 deselected in 35.66s
```

Go checks in the Windows-created linked worktree set matching Linux GIT_DIR and
GIT_WORK_TREE paths. Managed Python uses the WSL-only environment outside the
checkout at `/root/.cache/barn-287-conformance-venv`. No manual servers, tracked
database mutations, or direct pytest collection probes were used.
