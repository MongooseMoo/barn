# String source round-trip audit (#26)

The formatter defect described in #26 was repaired by
`7ccd1e18857d7f6ca68fb352bc3de3d21b3bbf4e` ("Fix MOO string literal formatting").
The production formatter uses `quoteMOOString`: it iterates bytes, escapes
only quote/backslash, and preserves all other non-NUL bytes verbatim. It no
longer uses Go string escaping for MOO literal output. This delivery adds
integration and property coverage; production formatting is unchanged.

The Go integration test performs three `set_verb_code` -> `verb_code` ->
recompile cycles, checking exact canonical source and decoded literal bytes.
It includes tab, carriage return, high byte 0x80, quote, and backslash. The
existing all-255-non-NUL-byte formatter property now also checks a second
format pass for idempotence in both formatter modes. Existing byte-equal
semantic round-trip cases remain covered.

The companion managed scenario uses a test-owned object/driver verb and
constructs accepted string bytes using `chr`. It compares execution with the
original value, canonical source with the explicitly escaped input, and a
second `verb_code` result after recompilation. Toast verified this contract
before Barn verification. Every managed selection includes capability
admission and owns disposable working databases.

NUL handling is the explicit checked-formatter/source rejection delivered in
PR384 (#289); NUL is not a representable canonical source byte. Tests assert
deterministic rejection without output rather than silently dropping a suffix.

## Separate literal-linefeed finding

The issue's original inference that literal LF must be accepted because
Toast's emitter writes other bytes verbatim is incorrect. Managed Toast
rejects a raw LF inside a quoted source literal. Its lexer checks for LF/EOF
after stripping a backslash (`src/parser.y`, string scanner); the emitter and
lexer have different constraints. The Go all-byte property describes Barn's
internal structural round-trip, not portable source acceptance for LF.

Exact probe, using a new object `o` and its own `probe` verb initially returning
99:

```moo
source = {"return \"a" + chr(10) + "b\";"};
errors = set_verb_code(o, "probe", source);
return {errors, o:probe()};
```

Managed Toast: `{{"Line 1:  Missing quote", "Line 1:  syntax error"}, 99}`.
Managed Barn at `5cae0cd4e236794bec6555f299422ce1be7aa70b`:
`{{}, "a<LF>b"}`. The combined discovery session completed with Toast 3 passed
in 7.84s and Barn 1 failed, 2 passed in 6.61s. This identifies a separate lexer
acceptance/diagnostic gap, rather than a remaining Go-escaping formatter bug.
The shared formatter regression contains only the accepted-byte contract;
literal-LF rejection needs its own regression and lexer repair before that
assertion can enter the shared suite.

An initial fixture used `decode_binary` as though it returned a string and
produced E_TYPE; it was corrected to construct bytes through `chr` before the
behavior comparison. No result from that invalid fixture is conformance
evidence. No fresh full conformance baseline or manual server was run.

Final accepted-byte selection: managed Toast 2 passed in 6.96s; managed Barn
2 passed in 8.36s. `go test ./...`, focused builtin/parser string race tests
(1.181s/1.051s), `go vet ./...`, and `staticcheck ./...` completed with
`STRING_ROUNDTRIP_FULL_FOCUSED_RACE_STATIC_OK`. Existing full Go tests include
the tracked parser/formatter database census.
