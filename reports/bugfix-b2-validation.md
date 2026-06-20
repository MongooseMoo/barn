# Bugfix B2 — set_verb_code / compile validation gaps vs Toast

Branch: `fix/b2-set-verb-code-validation` (worktree `C:/Users/Q/code/barn-fix-b2`, off master `c2714e5`). NOT merged.

## Status: IN PROGRESS — Toast confirmed for B2a & B2b; implementing fixes.

## Environment
- WSL Toast oracle: `~/src/toaststunt/build-release/moo ~/src/toastcore/toastcore.db core.out.db -p 9451`, reachable from Windows via `localhost:9451`. Confirmed `; return 1+1;` => 2.
- moo_client built in worktree. Scratch verbs added on `player` (#2 Wizard) via add_verb.

## Toast transcripts (RULE ZERO — captured live, ToastStunt build-release, toastcore.db)

### B2a — unknown builtin
```
; set_verb_code(player,"scratch",{"x = 1;"})            => {}
; set_verb_code(player,"scratch",{"x = foo(1,2,3);"})   => {"Line 1:  Unknown built-in function: foo"}
; verb_code(player,"scratch")                           => {"x = 1;"}   (UNCHANGED)
```
Error string: `Line 1:  Unknown built-in function: foo` — "Line", space, N, colon, TWO spaces, message. Verb left unchanged on error.

### B2b — ternary nesting (nonassoc)
```
; ...{"x = 1 == 2 ? 3 | 4 == 5 ? 6 | 7;"}     => {"Line 1:  syntax error"}     REJECT
; ...{"x = 1 == 2 ? 3 | (4 == 5 ? 6 | 7);"}   => {}                            ACCEPT
; ...{"x = a ? b | c;"}                        => {}                            ACCEPT
; ...{"x = a ? b ? c | d | e;"}                => {}    ACCEPT (ternary in CONSEQUENT/middle pos)
; ...{"x = a ? (b ? c | d) | e;"}              => {}    ACCEPT
; ...{"x = a ? b | c ? d | e;"}                => {"Line 1:  syntax error"}   REJECT (ternary in ELSE pos, no parens)
; ...{"x = (a ? b | c) ? d | e;"}             => {}    ACCEPT (parenthesized condition)
; ...{"y = 1 + (a ? b | c);"}                  => {}    ACCEPT
; ...{"y = a ? b | c, d ? e | f;"}            => {"Line 1:  syntax error"}   REJECT
```
Toast grammar (`~/src/toaststunt/src/parser.y`): line 104 `%nonassoc '?' '|'`; line 646 rule `expr '?' expr '|' expr`. Conditional operator is NON-ASSOCIATIVE: the consequent (between `?` and `|`) may be a bare ternary because it is delimited by the mandatory `|`; the else and the condition may NOT be a bare ternary without parens.

## Barn current behavior / bug location
- `parser/parser.go:501-522` ternary handling. ELSE parsed with `ParseExpression(PREC_TERNARY)` (line 513) → right-associative → wrongly accepts `a ? b | c ? d | e`.
- Fix plan B2b: parse ELSE at `PREC_TERNARY+1`; after building a TernaryExpr, if `current == TOKEN_QUESTION` it's a nonassoc syntax error. Consequent stays `PREC_LOWEST` (Toast accepts ternary there). Existing `TestTernaryRightAssociative` (a?b?1|2|3 → consequent nest) stays valid.

## B2a fix plan
- Need: where builtin calls compiled / where set_verb_code validates. `BuiltinCallExpr` built at parser.go:354. Validation of unknown builtin name absent. Must reject at COMPILE so set_verb_code returns error list and leaves verb unchanged. Locating compile path next.

## Architecture findings (fix design)
- `builtins/verbs.go:708` set_verb_code validates by calling `bytecode.CompileVerb(lines)` (parse-only, AST). Verb only updated when `len(errors)==0`, so "unchanged on error" is already guaranteed by control flow.
- `bytecode.CompileVerb` (verbcache.go:20) is parse-only -> never checks builtin names (B2a gap). Bytecode compiler DOES check unknown builtins at `compiler.go:1384-1387` but only runs at verb-CALL time, not set_verb_code.
- Registry reachable in builtins via `registry, ok := ctx.Registry.(*Registry)` (established pattern, objects.go:25 etc). `*Registry` has `GetID(name)(int,bool)` and `Has(name)bool`.
- `pass` is NOT in registry; special-cased in compiler (compiler.go:1344). Must skip `pass` in B2a name-check.
- `BuiltinCallExpr{Pos Position, Name, Args}`; `Position.Line` available for `Line N:` format.

### B2a fix design
Walk parsed AST in set_verb_code; for each `BuiltinCallExpr` whose Name is not `pass` and not in registry, emit `Line N:  Unknown built-in function: NAME` (Toast format, two spaces) using node line. Put the AST walk + check in builtins package (has registry). Leaves verb unchanged (existing flow).

### B2b fix design
`parser/parser.go:513` parse ELSE at `PREC_TERNARY+1` (was PREC_TERNARY); after building TernaryExpr, if `current==TOKEN_QUESTION` -> syntax error (nonassoc). Consequent stays PREC_LOWEST.

## Fixes applied
### B2b (parser/parser.go ~501-535)
- ELSE branch parsed at `PREC_TERNARY+1` (was PREC_TERNARY).
- After building a TernaryExpr, `current==TOKEN_QUESTION` -> `syntax error`.
- New test `parser/parser_ternary_nonassoc_b2b_test.go` (6 accept / 2 reject). `go test ./parser/` GREEN.

### B2a (bytecode + builtins)
- `bytecode/compiler.go`: new typed `UnknownBuiltinError{Name,Line}`; `compileBuiltinCall` returns it (was bare fmt error). `%w`-wrapped through CompileVerbBytecode so `errors.As` works.
- `builtins/verbs.go` set_verb_code: after parse OK, run `CompileVerbBytecode(compileLines, registry)`; if `errors.As(&UnknownBuiltinError)` -> return `{"Line N:  Unknown built-in function: NAME"}` (Toast format, two spaces). Verb left unchanged (existing control flow; returns before SetVerbCode). `pass` skipped (compiler special-case). Errors never cached (cache put only on success).
- `go build ./...` exit 0.

## Live verification on Barn (port 9500, b2test.db copy)
- B2a: `set_verb_code(player,"scratch",{"x = foo(1,2,3);"})` => `{"Line 1:  Unknown built-in function: foo"}`; verb_code still `{"x = 1;"}` (UNCHANGED). MATCHES Toast exactly.
- B2b: all 6 accept cases => `{}`; `1 == 2 ? 3 | 4 == 5 ? 6 | 7` => error list (`{"parse error: syntax error"}`). Accept/reject decisions MATCH Toast exactly.

## B2c — message format blast radius (ASSESS ONLY)
- Toast syntax-error format: `Line N:  syntax error` (line number, two spaces). Barn emits `parse error: <inner>` (no line number) via two wrap sites: `bytecode/verbcache.go:30` (set_verb_code path) and `bytecode/compiler.go:206`; conformance runner also wraps at `conformance/runner.go:139,155`.
- ~74 `fmt.Errorf` sites in the parser produce varied inner messages (e.g. "syntax error", "expected ';' ...", "expected '|' in ternary ..."). The parser does NOT attach line numbers to most errors -> a general Toast-format rewrite needs line plumbing through the parser, not just the wrap site.
- Conformance assertions on error text: only `language/looping.yaml:173,182` use `match: "(syntax error|Parse error)"` (lenient regex; Barn's `parse error: syntax error` matches the "syntax error" alternative). `json.yaml` hits are unrelated comments. So current text is NOT exact-asserted anywhere -> changing it is low conformance-risk, BUT the count gate is exact (3871/0/131) so any shift must be verified.
- DECISION/RECOMMENDATION: **SPLIT into its own task.** Rationale: (1) B2a already emits Toast's exact `Line N:  Unknown built-in function: NAME` format (verified live + unit test). (2) B2b's grammar accept/reject now matches Toast exactly; only its MESSAGE (`parse error: syntax error` vs Toast `Line 1:  syntax error`) differs, and matching it requires line-number plumbing through ~74 parser error sites (the parser does not currently attach line numbers to most errors) — i.e. the broad rewrite this task forbids. (3) Conformance risk is LOW (only `language/looping.yaml` asserts error text, via a lenient `(syntax error|Parse error)` regex that Barn's current text already satisfies) and the suite is GREEN at the exact count — so deferring is safe and reversible. A dedicated task should: add a `Line` field to parser errors, thread it through `CompileVerb`, and switch the wrap sites in `bytecode/verbcache.go:30` + `bytecode/compiler.go:206` to Toast's `Line N:  <msg>` form, re-running conformance to confirm 3871/0/131 holds.

## Gates
- `go build ./...` exit 0.
- `go vet ./...` => exactly 2 known (cmd/moo_client IPv6, vm/stack ReadByte). PASS.
- `go list -deps ./db/store | grep parser` => EMPTY. PASS.
- `go test ./...`: known fixture failures only (conformance dir not found; mongoose7_snapshot.db missing) — unrelated to changes. Affected pkgs `./parser ./bytecode ./builtins ./vm` GREEN.
- New Go tests: `parser/parser_ternary_nonassoc_b2b_test.go` (PASS), `builtins/verbs_set_code_b2a_test.go` (2 PASS, exact Toast string + verb-unchanged).
- Conformance (synchronous, managed server, `uv run --project ..\moo-conformance-tests moo-conformance --server-command "...barn.exe -db {db} -port {port}"`): **3871 passed, 0 failed, 131 skipped** in 143s. EXACT required count — fixes did NOT shift conformance; no real test relied on the lax behavior.

## Proposed conformance tests (NOT added — sacred suite)
Toast-captured expected values; propose for `builtins/verbs.yaml` (B2a) and `language/expressions.yaml` (B2b):
1. `set_verb_code_rejects_unknown_builtin`: set scratch to `{"x = 1;"}`; then `set_verb_code(o,"v",{"x = foo(1,2,3);"})` -> expect `["Line 1:  Unknown built-in function: foo"]`; then `verb_code(o,"v")` -> `["x = 1;"]` (unchanged).
2. `ternary_nested_in_else_requires_parens`: `set_verb_code(o,"v",{"x = 1 == 2 ? 3 | 4 == 5 ? 6 | 7;"})` -> expect error list matching `(syntax error|Parse error)`; and `{"x = 1 == 2 ? 3 | (4 == 5 ? 6 | 7);"}` -> `{}` (accepted).
3. `ternary_in_consequent_ok`: `{"x = a ? b ? c | d | e;"}` -> `{}` (accepted, no parens needed in middle position).

## Commit
`1bf93c797a3d7596a6155d6ea670e6ba11482261` on branch `fix/b2-set-verb-code-validation` (off master c2714e5). NOT merged — verifier next.

Files changed: `parser/parser.go` (B2b grammar), `bytecode/compiler.go` (UnknownBuiltinError type + use), `builtins/verbs.go` (B2a validation in set_verb_code), plus tests `parser/parser_ternary_nonassoc_b2b_test.go`, `builtins/verbs_set_code_b2a_test.go`, and this report.
