# Bugfix B2c — syntax-error message format to match Toast

Branch: `fix/b2c-error-format` (worktree `C:/Users/Q/code/barn-fix-b2c`, off master `571b443`). NOT merged.

## Status: IN PROGRESS — RULE ZERO Toast capture DONE; implementing.

## RULE ZERO — Toast transcripts (ToastStunt build-release, toastcore.db, port 9451, via WSL)

Scratch verb on player (#3 wizard? player) via add_verb; tested via `set_verb_code`.

```
; set_verb_code(player,"scratch",{"x = 1;"})            => {}              (valid)
; ...{"x = 1"}                                          => {"Line 2:  syntax error"}   (missing ;, EOF=line+1)
; ...{"if (1)"}                                         => {"Line 2:  syntax error"}
; ...{"if (1) x=1;"}                                    => {"Line 2:  syntax error"}   (missing endif, EOF)
; ...{"for x in"}                                       => {"Line 2:  syntax error"}
; ...{"x = \"abc"}                                      => {"Line 1:  Missing quote", "Line 1:  syntax error"}  (lexer + parse)
; ...{"x = (1 + ;"}                                     => {"Line 1:  syntax error"}
; ...{"x = }"}                                          => {"Line 1:  syntax error"}
; ...{"endif"}                                          => {"Line 1:  syntax error"}
; ...{"x = 1;","y = ;"}                                 => {"Line 2:  syntax error"}   (multi-line: actual line)
; ...{"x = 1;","y = 2;","z = ;"}                        => {"Line 3:  syntax error"}
; ...{"x = 1;","y = 2;","z = 3"}                        => {"Line 4:  syntax error"}   (missing ;, EOF=lines+1)
; ...{"@#$"}                                            => {"Line 1:  syntax error"}
; ...{"x = `foo;"}                                      => {"Line 1:  syntax error"}
; ...{"x = 1 $ 2;"}                                     => {"Line 1:  syntax error"}
; ...{"return return;"}                                 => {"Line 1:  syntax error"}
; ...{"while x;"}                                       => {"Line 1:  syntax error"}
; ...{"x = {1,2;"}                                      => {"Line 2:  syntax error"}   (EOF, list not closed)
; ...{"fork (5)"}                                       => {"Line 1:  syntax error"}
; ...{"x = 0123;"}                                      => {}                          (accepted)
; ...{"break;"}                                         => {"Line 1:  No enclosing loop for `break' statement"}
; ...{"continue;"}                                      => {"Line 1:  No enclosing loop for `continue' statement"}
; ...{"x = 1; break;"}                                  => {"Line 1:  No enclosing loop for `break' statement"}
; ...{"return 1","return 2"}                            => {"Line 2:  syntax error"}
; ...{"",""}                                            => {}                          (blank lines accepted)
; ...{"","","x = ;"}                                    => {"Line 3:  syntax error"}   (blank lines counted)
; ...{"x = 1;","","y = ;"}                              => {"Line 3:  syntax error"}
```

## DECISION
Toast collapses essentially ALL parse errors to the GENERIC `Line N:  syntax error`
(word "Line", space, line number, colon, TWO spaces, "syntax error"). It does NOT expose Barn's
specific inner text ("expected ';'", "expected '|' in ternary", etc.) for parse errors.

Therefore Barn should emit `Line N:  syntax error` for parse/syntax errors — matching Toast's
ACTUAL output (the spec). Barn's specific inner messages are dropped for the user-facing list.

Notable extras (NOT plain parse errors — out of B2c scope unless cheap):
- Lexer "Missing quote" emits an EXTRA `Line 1:  Missing quote` entry BEFORE the syntax error.
- `break`/`continue` outside loop → compile-time semantic error `Line N:  No enclosing loop for
  `break' statement` (NOT "syntax error"). This is a semantic, not parse, error.

## Line-number semantics
- Multi-line: error reports the ACTUAL line of the offending token.
- Unexpected-EOF (missing `;`, missing `endif`, unclosed list): Toast reports `lines + 1`
  (the phantom line after the last source line).
- Blank lines ARE counted toward line numbers.

## Barn current behavior
Barn emits `parse error: <inner>` with NO line number, via wrap sites
`bytecode/verbcache.go:30` + `bytecode/compiler.go:206`; conformance runner wraps at
`conformance/runner.go:139,155`. ~74 `fmt.Errorf` sites in parser; Position.Line exists (B2a uses it).

## Plan
1. Thread a Line into parser errors (typed parser error carrying the offending token's line;
   default to current token line). EOF → lines+1 falls out naturally if EOF token line is set right.
2. Format syntax errors as `Line N:  syntax error` at the wrap sites (set_verb_code path +
   compile path + conformance runner). Keep B2a's `Line N:  Unknown built-in function: NAME` intact.
3. Update Barn Go tests asserting old `parse error:` text.
4. Gates incl. conformance EXACT 3871/0/131 synchronous.

## Architecture findings (verified)
- `parser.Token` has `Position{Line,Column,Offset}`. Lexer tracks `l.line` (1-based, ++ on `\n`).
- `ParseProgram()` (parser/parser_stmt.go:6) returns `(stmts, error)` — first error aborts.
- `parseStatement` and ~74 `fmt.Errorf` sites build messages from `p.current`.
- Wrap sites discard line: `bytecode/verbcache.go:30` `"parse error: %v"`, `bytecode/compiler.go:206`.
  Conformance runner: `conformance/runner.go:139,155`.
- B2a `UnknownBuiltinError{Name,Line}` lives in bytecode/compiler.go; set_verb_code in
  builtins/verbs.go formats `Line N:  Unknown built-in function: NAME` via errors.As. MUST stay.

## EOF line semantics (VERIFIED via lexer probe vs Toast)
Source is `strings.Join(code,"\n")` (NO trailing newline) so at EOF `l.line == numLines`.
Toast reports the offending TOKEN's line; when the offending token is EOF, Toast uses `numLines+1`.
Barn lexer currently sets EOF token line = `l.line` (= numLines). FIX: set EOF token line to
`l.line + 1` so EOF errors match Toast's phantom-final-line. Non-EOF errors already match
(token line is correct). Verified cases: "x = 1" -> Toast L2 (EOF); "x=1;\ny=;" -> L2 (real `;`);
3-line missing-`;` -> L4 (EOF); "if (1) x=1;" -> L2 (EOF); "x = {1,2;" -> L2 (EOF).

## Plan (refined)
1. Lexer: EOF token Position.Line = l.line + 1 (phantom final line, matches Toast).
2. Parser: add typed `ParseError{Line int; Msg string}` with Error() = Msg. Add helper
   `p.errorf(format, args...)` that captures `p.current.Position.Line`. Convert error returns to
   carry the line. Minimal: wrap at ParseProgram return OR thread a line. Since Toast collapses to
   generic "syntax error", I can capture the line at the failing token: store the line on the
   Parser error via a typed wrapper at the ParseProgram boundary using p.current's line at failure.
   SIMPLEST: ParseProgram, on err, wrap as ParseError{Line: p.current.Position.Line, Msg:"syntax error"}.
   (p.current is the failing token when parseStatement returns error.)
3. Wrap sites: format `Line N:  syntax error` from ParseError.Line. Keep B2a path.
4. Update Barn Go tests asserting old `parse error:` text.
5. Gates incl conformance EXACT 3871/0/131 synchronous.

Caveat to verify: is p.current reliably the failing token when an inner fmt.Errorf fires? Many
errors fire on p.current (e.g. "expected ';'" checks p.current.Type). Need to confirm p.current
isn't advanced past the error point. Will verify empirically against the Toast line numbers.

## IMPLEMENTED SO FAR
- `parser/lexer.go`: EOF token Position.Line = l.line + 1 (phantom final line).
- `parser/parser_stmt.go`: new `ParseError{Line,Msg,Detail}` type; ParseProgram wraps any
  statement error as `&ParseError{Line: p.current.Position.Line, Msg:"syntax error", Detail: err}`.
- `bytecode/verbcache.go`: new `formatParseError(err)` -> `"Line N:  <msg>"` via errors.As on
  *parser.ParseError; used at CompileVerb error return. (added "errors" import)
- `bytecode/compiler.go:206`: dropped "parse error: " prefix (errs[0] already Toast-formatted).
- builtins/verbs.go set_verb_code parse path uses CompileVerb errors directly -> now Toast format.
  B2a UnknownBuiltin path untouched.

## conformance/runner.go (Barn's INTERNAL Go runner, NOT the sacred external suite)
runner.go:139,155 wrap ParseExpression/ParseProgram errors into TestResult.Error. VERIFIED this
text is only DIAGNOSTIC (conformance_test.go:54 prints it); expectations.go compares MOO error
CODES, not parse-error strings. Safe to reformat. The REAL conformance gate is the external
moo-conformance-tests over the network, which does not use this code path.
NOTE: ParseExpression path (line 150) does NOT go through ParseProgram, so its error is a raw
parser error (no ParseError wrap, no line). formatParseError fallback handles it (Line 1: <msg>).

## BUILD: `go build ./...` OK.

## Line-number verification (Barn CompileVerb vs Toast) — 17/19 EXACT MATCH
All match Toast except TWO edge cases:
- `x = {1,2;`     Toast Line 2 / Barn Line 1   (Toast treats unclosed `{` as EOF-line error; Barn
  fails at the `;` token on line 1)
- `fork (5)`      Toast Line 1 / Barn Line 2   (Toast errors in fork header on line 1; Barn reaches
  EOF/line 2)
These are line-number-only divergences on the GENERIC "syntax error" message; the message text and
format match. NOT conformance-asserted (only language/looping.yaml asserts error text, lenient
`(syntax error|Parse error)` regex which both satisfy). Documented as known minor deltas; matching
them exactly would require token-position tweaks in list/fork parsing beyond B2c's format scope.

## Barn Go tests asserting old text: NONE found
grep for "parse error:" in *_test.go -> none. grep for parse/syntax error text assertions in
bytecode/cache_test.go, parser/parser_stmt_test.go, vm/bytecode_execution_test.go -> none assert
on the wrapped error string. So ZERO Barn Go tests needed updating for the format change.

## GATES (all run, quoted)
- `go build ./...` => exit 0 ("BUILD OK").
- `go vet ./...` => exactly the 2 KNOWN issues (cmd/moo_client IPv6 main.go:53; vm/stack.go:49
  ReadByte). No new vet issues. PASS.
- `go list -deps ./db/store | grep parser` => EMPTY. db/store still parser-free. PASS.
- `go test ./parser/ ./bytecode/ ./builtins/ ./vm/` => all `ok` GREEN.
  `go test ./conformance/` => KNOWN fixture failures only (cow_py/tests/conformance dir not found),
  unrelated to this change.
- **EXTERNAL conformance (synchronous foreground, managed barn.exe -db {db} -port {port}):
  `3871 passed, 131 skipped in 143.17s` => 3871/0/131 EXACT. No count shift, no broken assertion.**

## RULE ZERO reconfirm (live Barn 9500 vs live Toast 9451, side by side)
set_verb_code parse-error cases, Barn list values == Toast EXACTLY:
- `{"x = (1 + ;"}` => `Line 1:  syntax error`   (both)
- `{"if (1)"}`     => `Line 2:  syntax error`   (both)
- `{"x = 1;","y = ;"}` => `Line 2:  syntax error` (both)
- `{"endif"}`      => `Line 1:  syntax error`   (both)
- B2a intact: `{"x = foo(1,2,3);"}` => `Line 1:  Unknown built-in function: foo` (Barn, unchanged).

## Known minor deltas (NOT B2c scope; documented)
1. Line-number-only divergence on 2/19 probe cases: `x = {1,2;` (Toast L2 / Barn L1) and `fork (5)`
   (Toast L1 / Barn L2). Format + message identical; only the line index differs because Barn's
   p.current at failure sits on a different token than Toast's yacc position. Not conformance-
   asserted. Exact match would need token-position tweaks in list/fork parsing.
2. PRE-EXISTING B2 divergence (NOT introduced or touched by B2c): live set_verb_code ACCEPTS
   `{"x = 1"}` (missing `;`) via `normalizeVerbSourceLines` while Toast REJECTS it
   (`Line 2:  syntax error`). `builtins/verbs.go` has an EMPTY diff vs 571b443 — this is the B2
   normalization path, out of B2c's format-only scope. Barn's raw `bytecode.CompileVerb("x = 1")`
   DOES return `Line 2:  syntax error` (matches Toast); the divergence is the normalization layer
   above it. Flagged for a future B2 follow-up.

## Files changed (source + tests + report)
- parser/lexer.go         (EOF token line = l.line+1)
- parser/parser_stmt.go   (ParseError type; ParseProgram wraps with line + generic "syntax error")
- bytecode/verbcache.go   (formatParseError -> "Line N:  <msg>"; +errors import)
- bytecode/compiler.go    (drop "parse error: " prefix; errs[0] already Toast-formatted)
- conformance/runner.go   (formatParseError helper; both parse wrap sites; +errors import)
- reports/bugfix-b2c-error-format.md (this file)
- Barn Go tests updated: NONE (no test asserted on old "parse error:" text).

## COMMIT: 144cc81 on branch fix/b2c-error-format (off master 571b443).
NOT merged — verifier next.
