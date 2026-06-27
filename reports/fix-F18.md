# Fix F18 — capitalize() title-cased every word instead of first-char-only

## Toast / MOO behavior (verified FIRST)

`capitalize` is **not a ToastStunt C++ builtin**. `grep -i capitalize
C:/Users/Q/src/toaststunt/src/` returns **no matches** — there is no `bf_capitalize`
anywhere in the server source. It exists only as the MOO library verb
`$string_utils:capitalize`, documented in the core db:

- `mongoose.db:73488` — `:capitalize/se(string) => string with first letter capitalized.`
- `mongoose.db:328050` — "...the standard algorithm, i.e., upcasing the first letter..."

So the authoritative behavior is: **uppercase ONLY the first character, leave the rest
unchanged** (like `unique`, this builtin is effectively Barn-only).

### Oracle cross-check (bare exprs; oracle prepends `;`)

| expr | result |
|------|--------|
| `1+1` (sanity) | `=> 2` (oracle works) |
| `capitalize("hello world")` | empty / exit 0 — **Toast rejects as unknown builtin** |
| `capitalize("it's a test")` | empty (unknown builtin) |
| `capitalize("ABC")` | empty (unknown builtin) |
| `capitalize("")` | empty (unknown builtin) |
| `capitalize("123abc")` | empty (unknown builtin) |
| `$string_utils:capitalize("hello world")` | `=> *Aborted*` (verb needs full context, not eval) |

The empty results (vs `1+1 => 2`) confirm `capitalize()` is not a builtin function in
Toast — only the library verb exists. Expected verb outputs per "upcase first letter":
`"Hello world"`, `"It's a test"`, `"ABC"`, `""`, `"123abc"`.

## The bug

`builtins/strings.go` `builtinCapitalize` used the deprecated Go `strings.Title`, which
title-cases EVERY word and even upper-cases after apostrophes:
`capitalize("it's a test")` returned `"It'S A Test"`, `capitalize("hello world")`
returned `"Hello World"`.

## The change

`builtins/strings.go` — dropped `strings.Title`. Now:

```go
s := str.Value()
if s == "" {
    return types.Ok(types.NewStr(""))
}
return types.Ok(types.NewStr(strings.ToUpper(s[:1]) + s[1:]))
```

Byte-indexed `s[:1]` matches MOO's `string[1]` semantics; remainder untouched.

## The test

`builtins/review_data_test.go` `TestReview_Data_CapitalizeDeprecatedTitle` — converted
from "demonstrate the bug" to a table asserting the true behavior, with Toast/MOO
citation in the comment:

| in | want |
|----|------|
| `"hello world"` | `"Hello world"` |
| `"it's a test"` | `"It's a test"` |
| `"ABC"` | `"ABC"` |
| `""` | `""` |
| `"123abc"` | `"123abc"` |

## Gate results

- `go test ./builtins/ -run 'Capitalize|capitalize' -v` → **PASS**
- `go vet ./builtins/` → clean (exit 0)
- `go test ./builtins/...` → capitalize passes. Remaining failures are pre-existing
  intentionally-red tests for OTHER findings (is_member case-sensitivity,
  pcre_match empty subject, file_readlines binary M1, queued_tasks H4, verb_code/add_verb
  perms). **No new failures introduced.**

### Before / after (capitalize test)

- Before: `TestReview_Data_CapitalizeDeprecatedTitle` FAILED (red, documenting the bug).
- After: PASS.

## Commit

`b119674f2ceb5e633c57fdb5bbe9a84e5d47ecf9`
