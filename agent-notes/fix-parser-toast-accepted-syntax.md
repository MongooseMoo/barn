# Fix parser for Toast-accepted syntax (issue #984)

## Goal

Make Barn accept, with Toast's meaning, every source form Toast's lexer and
grammar accept that Barn's parser rejects, starting with `expr.:name` and
identifiers containing bytes above 127, each pinned first by a Toast-passing
conformance test.

## State

- Toast source read on the pinned oracle ref (`implementation_ref` in
  `profiles/toast/stock-wsl-testdb.json`): `src/parser.y`, `src/unparse.cc`,
  `src/keywords.gperf`, `src/waif.cc`, `src/network.cc`, `src/db_io.cc`,
  `src/tasks.cc`, `src/list.cc` (`bf_chr`).
- Conformance file `src/moo_conformance/_tests/language/parser_toast_accepted_syntax.yaml`
  written in the suite worktree. First managed WSL Toast run: 14 passed
  (13 tests + capability_admission), 2 failed (a scratch probe file whose
  expectations were placeholders, deleted afterwards). Load at start:
  `load average: 26.47, 22.20, 14.75` on 8 CPUs.
- Barn fix written: `parser/lexer.go` (high byte ends the program text,
  `\v`/`\f` whitespace, ASCII-only identifier letters), `parser/parser.go`
  (`.:name`). Go tests `parser/toast_accepted_syntax_test.go`
  (TestColonPropertyIsPrefixedDynamicProperty, TestHighByteEndsProgramText,
  TestFormFeedAndVerticalTabSeparateTokens) pass. Seeds added to
  `internal/fuzzseed/seeds.go`; seed-only runs of FuzzParseFormatRoundTrip,
  compiler Fuzz*, vm FuzzSourceExecution pass.
- Barn-red binary (unchanged origin/master source) SHA-256
  `04a230732c628b4b461bc9ffa990fca1964a100d6a52889d510a74e98a8888ce`.

## What the structured generator needs (internal/fuzzseed/structured.go, after #980)

The generator builds IR and the formatter spells it. `.:name` has no IR node of
its own: its tree is `PropertyExpr{PropertyExpr: LiteralExpr(":" + name)}` (and
`PropertyTarget{NameExpr: ...}` for assignment), which the formatter prints as
`x.(":name")`, the same text Toast's unparser prints.
1. Tree: in `expr` case 8 (structured.go:414-421) and `propertyTarget`
   (structured.go:595-603), sometimes set the name expression to a string
   literal `":" + g.pick(propertyNames)`; also add a class-prefixed
   `":name"` property to the fuzz world so reads are not all E_PROPNF.
2. Spelling: the source text must sometimes carry `.:name` rather than
   `.(":name")`. Either a source-level rewrite after formatting (replace
   `.(":NAME")` with `.:NAME`, optionally `. : NAME`, when NAME is an
   identifier) or a formatter option that spells colon-prefixed literal names
   that way. Both engines must get the same text.
3. Whitespace: a source-level mutation replacing a token-separating space with
   `\v` or `\f`.
4. High byte: a source-level mutation that appends `chr(0x80..0xFF)` plus any
   trailing text after a complete statement, or inside an expression (expect
   the same syntax error on both engines). The text then reaches the engines
   only through set_verb_code/eval strings, not the telnet input line, since
   Toast's network layer drops bytes above 127 (network.cc:408).

## Rule-by-rule comparison (Toast parser.y on the pinned ref vs Barn parser/)

Toast cites are `git show <ref>:src/parser.y` line numbers.

### Toast accepts, Barn rejects (fixed by this task)

1. `expr '.' ':' tID` (parser.y:405-416) builds EXPR_PROP with the string
   `":" + name`. Barn `parser/parser.go:423-458` (TOKEN_DOT case) requires an
   identifier or `(` after `.`, so `x.:foo` is a syntax error. The `.` and `:`
   are separate tokens in Toast (parser.y:1065 and 1066 `default: return c`),
   so `x . : foo` is accepted too (Toast-green test
   `colon_property_separate_tokens`).
   - Meaning: on an object, `o.:bar` is the property named `:bar` (Toast
     `[E_PROPNF, E_PROPNF, 4, 4]` for missing, then add_property(o, ":bar")).
     On a WAIF value the name gets a second prefix in `waif_get_prop`
     (waif.cc:686-690), so `w.:foo` reads class property `::foo`
     (Toast `[5, E_PROPNF, 9]`). On an int, E_TYPE for read and write.
   - Unparse: `unparse_name_expr` (unparse.cc:600-619) prints the name bare
     only when `ok_identifier` (unparse.cc:587-598) holds; `:foo` fails it, so
     Toast prints `x.(":foo")`, and `#0.(":foo")` (not `$`, unparse.cc:626-633).
2. Bytes above 127 outside a string literal or comment. Toast
   `parser.y:1304-1322` `my_getc` (the set_verb_code/eval list parser) stores
   the byte in a plain `char` and returns it, so on x86-64 Linux a byte >= 0x80
   is a negative int. No branch of `yylex` (parser.y:875-1068) matches it
   (`isspace`/`isdigit`/`isalpha` are false for negative values in the "C"
   locale; Toast never calls `setlocale`: `git grep setlocale <ref> -- src` is
   empty), so `default: return c;` returns a negative token, which the
   Bison parser treats as end of input. Everything from that byte on is
   dropped. There is NO identifier with a byte above 127 in Toast; `é = 1;`
   "compiles" because it is the empty program.
   Toast-green: `eval("x = 1; " + chr(195,169) + " = 2; return x;")` is
   `{1, 0}`; `eval("return 1 +" + chr(200) + " 2;")` is
   `{0, {"Line 1:  syntax error"}}` (line of the truncation point, not the
   phantom final line); verb_code after `{"x = 1;", "return x; é = 2;",
   "return 3;"}` is `{"x = 1;", "return x;"}`.
   Inside a string literal the byte is kept (parser.y:1032-1047), and inside a
   comment it is skipped (parser.y:889-903). Byte 0xFF is `-1 == EOF`: inside a
   string it gives "Missing quote", inside a comment "End of program while in a
   comment" (scratch probe on Toast, see "Barn accepts, Toast rejects").
   Barn `parser/lexer.go:356-361` and `:501-504`: `isLetter` uses
   `unicode.IsLetter(rune(byte))`, which treats Latin-1 letter bytes
   (0xAA, 0xB5, 0xBA, 0xC0-0xFF except 0xD7/0xF7) as identifier characters, so
   Barn rejects `é` (C3 A9: A9 is not a letter) but accepts `ê` (C3 AA) and
   `x = 1; <C3>y = 2;` as identifiers with a different meaning from Toast.
   Other paths: Toast DB load (`db_io.cc:253-269`) uses `fgetc` (unsigned), so
   the same byte is a syntax error there; `.program` input (tasks.cc:683-691)
   is signed like the list parser. The network layer drops bytes above 127
   before they reach any of this (`network.cc:408`, `isgraph` in the C locale),
   so only chr()/decode-built strings reach the parser with such bytes.
3. Form feed and vertical tab are whitespace (`isspace`, parser.y:884-887).
   Barn `parser/lexer.go:69-73` skips only space, tab, LF, CR, so chr(11)/chr(12)
   between tokens is a syntax error on Barn. Toast-green:
   `{eval("return" + chr(11) + "4;"), eval("return" + chr(12) + "5;")}` is
   `{{1, 4}, {1, 5}}`.

### Barn accepts, Toast rejects (listed, not changed by this task)

Verified on the managed WSL Toast oracle with a scratch probe (set_verb_code,
not committed), and on Barn with `bin/moo`:

4. `(length)("abc")`: Toast syntax error (only `tID '(' arglist ')'`,
   parser.y:496); Barn compiles it (parser.go:366-409 accepts any
   parenthesized identifier as callee).
5. `o:f(1,)` and `$f(1,)`: Toast syntax error (`arglist`, parser.y:700-740,
   has no trailing comma); Barn accepts (parser.go:196-208 and 496-508 loop
   exits on `)` after a comma).
6. `for x in {1, 2} endfor`: Toast syntax error (only `(expr)` and
   `[a..b]`, parser.y:147-187); Barn accepts (parser_stmt.go:268-274).
7. `return 1; /* a **/`: Toast "End of program while in a comment"
   (parser.y:892-903 consumes the char after `*` even when it is `*`); Barn
   accepts (lexer.go:99).
8. Byte 0xFF inside a string: Toast `{"Line 1:  Missing quote",
   "Line 1:  syntax error"}`; inside a comment: "End of program while in a
   comment". Barn accepts both.

### Same acceptance, different tree (listed, not changed)

9. Literal-name property/verb forms: Toast verb_code of
   `return x.("foo") + #0.("bar") + x:("baz")() + #0:("q")() + x.("if") + x.("a b");`
   is `return x.foo + $bar + x:baz() + $q() + x.("if") + x.("a b");`; Barn
   keeps `x.("foo")`, `#0.("bar")`, `x:("baz")()`, `#0:("q")()`.
10. Unary minus on a literal: Toast folds it into the literal
    (parser.y:597-617); Barn keeps a UnaryExpr but prints the same text
    (`x = -5; y = -5; z = -5; w = -x;` on both).

### Checked, no difference found

Statements (if/elseif/else, while with and without label, fork with and
without id, for list/pair/range, return, break/continue with and without
label, try/except/finally, empty statement), scatter (`{a, ?b = e, @c} = e`),
catch expressions and their default, map/list literals, `$name`,
`$name(args)`, `expr:name(args)`, `expr:(e)(args)`, `expr.(e)`, index/range
and `^`/`$` contexts, operator precedence/associativity table
(parser.y:103-113 vs parser/operator.go and parser.go:53-69), keywords and
error names (keywords.gperf), numeric/object/string lexing, `..`, `|.`, `&.`,
`^.`, `->`, `=>`. This is a source reading; only the items above were run.

## Decisions and why

- `.:name` is parsed into the existing dynamic-property form with a string
  literal `":name"` (PropertyExpr.PropertyExpr), not a static name: that is
  the tree Toast builds (`EXPR_PROP` with a string constant), and Barn's
  formatter then prints `x.(":name")`, which is exactly Toast's unparse.
- Bytes above 127 end the program in the lexer (Toast's list-parser meaning),
  because the spec is Toast's behaviour. The issue's premise that Toast
  accepts `é` as an identifier is false; see item 2.
- Unicode check (Barn strings are Unicode by design, a standing decision).
  Barn reads verb source as the UTF-8 bytes of its Unicode strings.
  - Before: inside a string literal (`readString`) and inside a comment every
    byte was kept/skipped. Outside them, `isLetter` applied
    `unicode.IsLetter(rune(b))` to each single byte, so Latin-1 letter bytes
    (0xAA, 0xB5, 0xBA, 0xC0-0xFF except 0xD7/0xF7) were identifier
    characters and any other byte above 127 was a syntax error: `é` (C3 A9)
    was rejected, `ê` (C3 AA) was an identifier.
  - Toast: inside a string literal the byte is kept; inside a comment it is
    skipped; outside both it ends the program text (item 2).
  - This change: only the outside-literal, outside-comment case changes. The
    check sits at token start in `NextToken`, after whitespace and comments
    are consumed, and `readString` is not touched, so source with non-ASCII
    inside string literals lexes exactly as before (Go test case
    `return "a\xc3\xa9b";`; conformance test
    `high_byte_kept_in_string_and_comment` is green on both red and fixed
    Barn). `isLetter` is now ASCII-only, which only affects identifier
    characters. No MOO string-value code is changed.
  - Not changed (inside a literal, out of scope by the Unicode rule): byte
    0xFF inside a string or comment, where Toast reports "Missing quote" /
    "End of program while in a comment" (item 8).
- Waif read of a colon-prefixed name: `getWaifProp` now always prepends `:`
  as Toast `waif_get_prop` does (run log below).

## Surprises

- Item 2: Toast silently discards source after a high byte.
- Barn accepts some Latin-1-looking UTF-8 identifiers (`ê`, `µ`) that Toast
  truncates.

## Next step

Suite test and Barn fix are committed locally and merged with their default
branches; the lead publishes the suite PR first, then Barn. Still owed by
issue #984 and not done here: the structured fuzz generator emitting these
forms (see "What the structured generator needs"); only seeds were added.

## Run log

- 2026-11-21: Resumed by a new agent after the previous one hit a usage limit; reading note and diff next.
- 2026-11-21 Toast-green rerun (unchanged yaml, load 11.69 on 8 CPUs, Test.db SHA-256 1a3f23eb...d408e, Toast 03af4fa2 build-release): from the suite worktree,
  `uv run moo-conformance -k "parser_toast_accepted_syntax or capability_admission" -m "admission or conformance" --server-command "wsl -d Debian -u root -e env TOAST_MOO=/root/src/toaststunt/build-release/moo bash /mnt/c/Users/Q/code/barn-parser-toast-syntax/scripts/run_toast_wsl.sh {db} {port}" --server-db=src/moo_conformance/_db/Test.db --server-db-dir=src/moo_conformance/_db/startup --moo-host 172.30.247.77 --oracle-profile-manifest=<barn>/profiles/toast/stock-wsl-testdb.json --target-profile-manifest=<same> --fail-on-unexpected-skip --strict-markers -q`
  Result: 14 passed (13 tests + capability_admission). Without `--target-profile-manifest` the harness refuses (Toast writes no `{manifest}`).
- 2026-11-21 Barn-red run (Linux Barn cross-built from the unchanged branch source, `bin/barn-red-linux` SHA-256 b57ccb764452f79b7c65b472037332ee4b508b0b97b1550844167461f0b840e6; load 8.32 on 8 CPUs). The Windows Barn cannot be compared: the profile gate refuses `runtime_os` windows vs the linux oracle manifest, so Barn runs inside WSL like CI. WSL root's uv 0.9.4 cannot read the suite's `uv.lock`; the runner's `/home/github-runner/.local/bin/uv` (0.12.7) can. Command, inside WSL from the Barn worktree:
  `UV_PROJECT_ENVIRONMENT=$HOME/.cache/barn-984-conformance-linux /home/github-runner/.local/bin/uv run --project <suite> --frozen moo-conformance -m "admission or conformance" --server-command "<barn>/bin/barn-red-linux serve --db {db} --listen tcp://127.0.0.1:{port} --config=<barn>/profiles/barn/outbound-on.conf --profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest}" --server-db=<suite>/src/moo_conformance/_db/Test.db --server-db-dir=<suite>/src/moo_conformance/_db/startup --moo-host=127.0.0.1 --oracle-profile-manifest=<barn>/profiles/toast/stock-wsl-testdb.json --fail-on-unexpected-skip --strict-markers -q -k "capability_admission or parser_toast_accepted_syntax"`
  Result: 10 failed, 4 passed (capability_admission, colon_property_keyword_name_is_syntax_error, high_byte_inside_expression_is_end_of_input, high_byte_kept_in_string_and_comment). Failing: all six other colon_property_* tests, vertical_tab_and_form_feed_separate_tokens, high_byte_ends_program_text, high_byte_ends_program_on_later_line.
- 2026-11-21 First Barn-green attempt with the parser/lexer fix (`bin/barn-fix-linux` b88c17460abec4feacaba05453d5a8169b2a0601d1ed4ceaf4b2fcc9da45c981, load 24.54 on 8 CPUs, same command): 13 passed, 1 failed: colon_property_on_waif_value_prepends_a_second_colon expected `[5, E_PROPNF, 9]`, got `[5, 5, 5]`.
  Cause: `vm/op_property.go` getWaifProp added the class-property colon only when the name did not already start with one, so `w.(":foo")` (and now `w.:foo`) read class property `:foo`. Toast `waif_get_prop` (waif.cc:686-690 on 03af4fa2) always prepends `:`, so it reads `::foo`. This was already a Barn divergence for the existing `w.(":foo")` form. The write path (`executeSetProp` waif branch) already prefixed unconditionally. Fix: always prefix. Go test `vm/waif_colon_property_test.go` TestWaifColonNamedReadAddsSecondColon (both `w.:foo` and `w.(":foo")`) fails without the change and passes with it.
- 2026-11-21 Barn-green run with the parser/lexer fix plus the waif read fix (`bin/barn-fix-linux` SHA-256 959a622befca63f868c0f739b28c1fba3a0a5b9796ffc01996482c2ba45dab0f, load 11.09 on 8 CPUs, same command): 14 passed.
- 2026-11-21 Go checks on the changed packages: `gofmt -l parser vm internal/fuzzseed` empty; `go vet ./parser/... ./internal/fuzzseed/... ./vm/` clean; `go test ./parser -run 'TestColonPropertyIsPrefixedDynamicProperty$|TestHighByteEndsProgramText$|TestFormFeedAndVerticalTabSeparateTokens$'` ok; `go test ./vm -run 'TestWaifColonNamedReadAddsSecondColon$'` ok. The full `go test ./...` was not run locally (CI runs it).
- 2026-11-21 Correction: the Windows `uv run` for the Toast run (no `--frozen`) rewrote the suite worktree's `uv.lock` (revision 3 to 5, added `[options]` exclude-newer entries; package set not compared). The WSL runs used `--frozen` against that rewritten lock, and WSL root's uv 0.9.4 failed on the rewritten lock, not necessarily on the committed one. The rewrite was reverted with `git checkout -- uv.lock` and is not committed. Toast runs from Windows should pass `--frozen`.
- 2026-11-21 PR CI red on golangci-lint: `parser/parser.go:429:4: QF1003: could use tagged switch on p.current.Type (staticcheck)`; the added `.:` branch made the `.` property if/else chain three-way. Rewritten as `switch p.current.Type` (LPAREN / COLON / default), same behaviour. Local checks on Windows (go1.27.1; golangci-lint 2.14.0 built with go1.27.1; staticcheck 2026.2.1 (0.8.1)): `golangci-lint run ./parser/... ./vm/... ./internal/fuzzseed/...` 0 issues; `staticcheck ./parser/...` clean, exit 0; `gofmt -l parser` empty; `go test ./parser -count=1` ok.
