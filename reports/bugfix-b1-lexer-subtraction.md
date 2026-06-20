# Bugfix B1 — Barn lexer mis-lexes `1-2` (subtraction without spaces)

Branch: `fix/b1-lexer-subtraction` (worktree `C:/Users/Q/code/barn-fix-b1`, off master `467d7ea`).
NOT merged. Verifier next.

## Step 1 — RULE ZERO: Toast confirmed (WSL oracle, toastcore.db, port 9451)

```
>> ; return 3-4;
=> -1
>> ; return 1-2;
=> -1
>> ; return 1+2*3-4;
=> 3
>> ; return -5;
=> -5
```

Toast parses subtraction-without-spaces fine and treats `-5` as unary minus on 5.
Barn must match.

## Step 2 — Root cause + fix

Root cause: `parser/lexer.go`, the `case '-':` in `NextToken`. It had a branch
`else if isDigit(l.peekChar()) { tok = l.readNumber() }` that folded a `-`
immediately following a digit into a *negative number literal*. So `1-2` lexed as
`INT(1)` then `INT(-2)` with NO operator between them → parse error. `1 - 2` (spaces)
worked because the `-` then stood alone. MOO has no negative numeric literals; `-5`
is unary minus applied to `5`, and the parser already handles that.

Fix (file:line):
- `parser/lexer.go` ~148-163 (`case '-':`): removed the negative-number branch.
  A `-` that is not `->` now always emits `TOKEN_MINUS`; the parser builds
  unary/binary minus (`parser.go:196` UnaryExpr; binary minus via precedence).
- `parser/lexer.go` `readNumber` ~343: removed the now-dead leading-`-` consumption
  (`readNumber` is only entered on a leading digit).
- Object-literal handling (`#-1`, `#-3`) is UNTOUCHED — those are legitimate MOO
  object-id literals, not arithmetic.

Tests that had encoded the BUG (Barn unit tests, not conformance — corrected to the
spec/Toast behavior):
- `parser/parser_expr_test.go` `TestParseUnaryMinus`: `-5`/`-42` expected `LiteralExpr`
  (negative literal) → now expect `UnaryExpr`. Stale comment at the precedence test fixed.
- `parser/parser_test.go` `TestParseIntegerLiterals`: `{"-5",-5}` → `{"5",5}`.
- `parser/parser_test.go` `TestParseFloatLiterals`: `{"-0.5",-0.5}` → `{"0.5",0.5}`.
- `parser/lexer_test.go` `TestLexerIntegerTokens`: `-5` and `42 -17 0` asserted
  `TOKEN_INT` with negative values → now `TOKEN_MINUS` + positive `TOKEN_INT`.

## Step 3 — New Go tests

New file `parser/lexer_subtraction_b1_test.go`:
- `TestB1SubtractionLexesAsMinusOperator` — `1-2`, `3-4`, `1+2*3-4` lex with `TOKEN_MINUS`
  between the operands.
- `TestB1SubtractionParsesAsBinaryMinus` — `1-2`/`3-4` parse as `BinaryExpr(MINUS)` over
  two positive int literals.
- `TestB1Precedence` — `1+2*3-4` parses as `(1 + (2*3)) - 4` (the structure that = 3).
- `TestB1UnaryMinusStillParses` — `-5` → `UnaryExpr(MINUS, lit 5)`; `-y` → `UnaryExpr(MINUS, id y)`.

`go test ./parser/` → `ok  barn/parser`.

Runtime end-to-end check (Barn on port 9460, Test.db), matching Toast:
```
; return 3-4;     => -1
; return 1-2;     => -1
; return 1+2*3-4; => 3
; return -5;      => -5
```

## Proposed conformance test (PROPOSED — NOT added; conformance is sacred)

`basic/arithmetic.yaml` already has subtraction tests but every one uses spaces
(`4 - 2`, `8 - 11`); a grep for `[0-9]-[0-9]` across the whole `_tests/` tree returns
nothing. The spaceless case was genuinely untested — which is why this fix repairs the
bug without changing the conformance count. Proposed additions to
`~/code/moo-conformance-tests/src/moo_conformance/_tests/basic/arithmetic.yaml`
(Toast-captured expected values):

```yaml
  - name: subtraction_no_spaces
    code: "3-4"
    expect:
      value: -1

  - name: subtraction_no_spaces_one_two
    code: "1-2"
    expect:
      value: -1

  - name: mixed_precedence_no_spaces
    code: "1+2*3-4"
    expect:
      value: 3
```

## Gates

- `go build ./...` → exit 0. PASS
- `go vet ./...` → exactly the 2 known findings (`cmd/moo_client/main.go:53` IPv6,
  `vm/stack.go:49` ReadByte signature). PASS
- `go list -deps ./db/store | grep parser` → EMPTY (unchanged). PASS
- `go test ./...` → `barn/parser` PASS plus all my new tests. The only failures are
  pre-existing FIXTURE/path failures, NOT from this change:
    * `barn/conformance` — missing `cow_py/tests/conformance` dir; fails IDENTICALLY on
      pristine master `467d7ea` (verified in the main tree).
    * `barn/db/format` `TestLoadMongooseSnapshot` — needs untracked `mongoose7_snapshot.db`,
      absent from a fresh worktree; copying that file in, `barn/db/format` PASSES.
- Conformance suite (synchronous, managed server, foreground):
  `3871 passed, 0 failed, 131 skipped` — EXACTLY the expected count. The fix repaired an
  untested case, so the count is unchanged. No conformance test encoded the buggy behavior.

## Commit
See branch `fix/b1-lexer-subtraction`. NOT merged.
