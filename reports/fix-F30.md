# Fix F30 — pcre_match() empty subject

## TL;DR
`pcre_match("", ".*")` returns **`{}`** (empty list) on Toast — and on Barn. The F30
finding was **inverted**: it assumed Toast returns a match because `.*` matches the
empty string. The Toast SOURCE proves otherwise. Barn's empty-subject short-circuit
was already correct; the **red test asserted the wrong result**. Fix = correct the test
to Toast's true behavior (and document the short-circuit's rationale).

## Toast behavior + exact result shape (SOURCE is authority; WSL oracle down)
`bf_pcre_match` runs its match loop as:

```c
int subject_length = memo_strlen(subject);     // pcre_moo.cc:193
...
Var ret = new_list(0);                          // pcre_moo.cc:188
...
while (offset < subject_length) { ... }         // pcre_moo.cc:208
...
return make_var_pack(ret);                       // pcre_moo.cc:320
```

For an empty subject, `subject_length == 0`, so `while (0 < 0)` never iterates and the
function returns its initial `new_list(0)` = `{}`. This holds for **every** pattern,
including empty-string-matching ones like `.*` and `^$` — Toast does NOT special-case,
the loop bound just shortcuts it. File: `C:/Users/Q/src/toaststunt/src/pcre_moo.cc:188,193,208,320`.

Successful (non-empty) match shape, for reference: a list of maps, one per match; each map
keyed `"0"`, `"1"`, ... (or the capture name for named groups) ->
`{"position" -> {start+1, end}, "match" -> <substring>}`
(`pcre_moo.cc` `result_indices` :350-359; `mapinsert` of position/match :259,267,292-293).

## Conformance cross-check
`moo-conformance-tests/.../builtins/pcre.yaml:201-205`:
```yaml
- name: pcre_match_empty_subject
  description: "Empty subject returns no matches"
  code: 'pcre_match("", "foo")'
  expect:
    value: []
```
Confirms empty subject => `[]` (`{}`).

## Why NOT remove the short-circuit
Go's `regexp.FindAllStringSubmatchIndex("", "(?i).*")` returns `[[0 0]]` (one match).
Removing Barn's short-circuit would make `pcre_match("", ".*")` return a **non-empty**
list — diverging from Toast. The short-circuit is required for parity and was kept.

## The fix
- `builtins/pcre.go`: kept the empty-subject short-circuit (`subject == "" -> {}`);
  upgraded its comment with the Toast `file:line` citation + conformance reference.
- `builtins/review_data_test.go` `TestReview_Data_PcreMatchEmptySubject`: corrected the
  inverted assertion to Toast's TRUE result `{}` (`got.Len() != 0` is the failure), with
  full source citation; added a genuine non-match case `pcre_match("foobar","baz") -> {}`
  to guard the normal non-match path. No change to non-empty-subject behavior.

## Gate output
```
$ go test ./builtins/ -run 'PcreMatch|Pcre|pcre' -v
=== RUN   TestReview_Data_PcreMatchEmptySubject
--- PASS: TestReview_Data_PcreMatchEmptySubject (0.00s)
ok  	barn/builtins

$ go vet ./builtins/      # clean, no output
```

### Before / after
- Before: `TestReview_Data_PcreMatchEmptySubject` FAILED ("= {} (empty), want a match").
- After: PASS.
- Full `go test ./builtins/...`: one UNRELATED pre-existing red test remains
  (`TestReview_Data_IsMemberStrCaseSensitiveBug`, a separate is_member finding in code
  this fix does not touch). No new failures introduced.

## Commit
`dbc37b7` on branch `review/branch-stocktake-2026-06-25`.
