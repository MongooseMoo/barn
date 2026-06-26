# Fix F17 — sort() honors keys / natural / reverse args

## Finding
`builtins/lists.go` `builtinSort` accepted up to 4 args but silently dropped the
`keys`, `natural`, and `reverse` parameters (literal `// TODO: Implement full sort`),
so callers got an always-ascending, case-sensitive sort regardless of arguments.
Red test: `builtins/review_data_test.go:115` `TestReview_Data_SortReverseIgnored`.

## Toast authority (ToastStunt source)
- Registration — `C:/Users/Q/src/toaststunt/src/list.cc:1779`
  `register_function("sort", 1, 4, bf_sort, TYPE_LIST, TYPE_LIST, TYPE_INT, TYPE_INT)`
- Implementation — `sort_callback`, `C:/Users/Q/src/toaststunt/src/list.cc:947-1020`
  (`bf_sort` at 1022-1026 just dispatches via `background_thread`).
- Natural compare — `C:/Users/Q/src/toaststunt/src/dependencies/strnatcmp.c`
  (`strnatcasecmp` = `strnatcmp0(..., fold_case=1)`).

### Exact signature
`sort(list [, keys] [, natural] [, reverse]) -> list`, arity 1–4.

### Per-arg semantics (list.cc:949-1019)
- `list` (LIST, required): the values to return.
- `keys` (LIST, optional): a parallel list to sort *by*. `list_to_sort = (nargs>=2 &&
  len(keys) > 0) ? 2 : 1` — an **empty keys list means "sort by the list itself"**.
  When non-empty it must have the same length as `list`; results are pulled from
  `list` (arg 1) by the sorted index, not from `keys`.
- `natural` (INT, optional): `is_true` (nonzero) ⇒ strings compare with
  `strnatcasecmp` (natural order); only affects STR elements.
- `reverse` (INT, optional): `is_true` (nonzero) ⇒ sorted index order is reversed
  after sorting.
- Comparison (VarCompare, 980-1006): INT/FLOAT/OBJ numeric, ERR by error-code int,
  **STR case-insensitive** (`strcasecmp`, or `strnatcasecmp` when natural).

### Error behavior
- Bad arity → `E_ARGS`; wrong arg types (keys not LIST, natural/reverse not INT) →
  `E_TYPE` (enforced by register_function's type tokens).
- Sort-key list must be homogeneous and scalar: any element whose type differs from
  the first element's, or is LIST/MAP/ANON/WAIF → `E_TYPE` (list.cc:970-976).
- keys provided with `len(list) != len(keys)` → `E_INVARG` (list.cc:957-960).
- Empty list (or empty keys yielding an empty sort target) → `{}` (list.cc:954-956).

## Oracle outputs (WSL strict-master Toast as a server on :17799 + moo_client;
the WSL emergency-eval oracle can't be used here because `sort` suspends via
`background_thread` → `=> *Suspended*`)
```
sort({3,1,2})                       => {1, 2, 3}
sort({3,1,2}, {}, 0, 1)             => {3, 2, 1}        (reverse)
sort({3,1,2}, {}, 0, 0)             => {1, 2, 3}
sort({"b","A","c"})                 => {"A", "b", "c"}  (case-insensitive)
sort({"b","A","c"}, {}, 1)          => {"A", "b", "c"}  (natural)
sort({30,1,200}, {}, 1)            => {1, 30, 200}     (natural ignored for ints)
sort({"c","a","b"}, {3,1,2})        => {"a", "b", "c"}  (keys)
sort({"c","a","b"}, {3,1,2}, 0, 1)  => {"c", "b", "a"}  (keys + reverse)
sort({1,"a"})                       => E_TYPE
sort({1,{2}})                       => E_TYPE
sort({1,2,3}, {1,2})                => E_INVARG
sort({})                            => {}
sort({}, {}, 1, 1)                  => {}
sort({1.5,0.5,2.5})                 => {0.5, 1.5, 2.5}
sort({#3,#1,#2})                    => {#1, #2, #3}
sort({E_NONE,E_TYPE,E_PERM})        => {E_NONE, E_TYPE, E_PERM}
sort({"B","a","B","a"})             => {"a", "a", "B", "B"}
```
Conformance `generated_builtins/sort.yaml` independently confirms arity 1–4 and the
E_ARGS/E_TYPE arg-type checks.

## Implementation (`builtins/lists.go`)
- Rewrote `builtinSort` to honor keys/natural/reverse exactly per `sort_callback`:
  empty-keys-as-identity, key/list length check (E_INVARG), homogeneous-scalar
  type check (E_TYPE), sort an index vector so keys map back into `list`, reverse
  after sort, return `list` elements by sorted index. Empty target → `{}`.
- Added `sortLess` (mirrors VarCompare) and faithful byte-wise ports of libc
  `strcasecmp` and Toast's `strnatcasecmp`/`compare_left`/`compare_right`
  (`natCompareLeft`/`natCompareRight`) plus ASCII helpers.
- Removed the now-unused `compareValues` (it did case-*sensitive* string ordering,
  which contradicts Toast).

## Test correction
None. The review test's expectation matched verified Toast behavior, so it now
passes as written.

## Gate results
- `go vet ./builtins/` → clean.
- `go test ./builtins/ -run 'TestReview_Data_Sort|Sort' -v` →
  `--- PASS: TestReview_Data_SortReverseIgnored`.
- Full `go test ./builtins/`: failure count dropped 8 → 7.

### Before (baseline, lists.go stashed) — 8 failures
IsMemberStrCaseSensitiveBug, **SortReverseIgnored**, PcreMatchEmptySubject,
CapitalizeDeprecatedTitle, FileReadlinesBinaryMode, QueuedTasksSortOrder,
VerbCodeAllowsOwnerWithoutReadBit, AddVerbUsesProgNotPlayerForPerm.

### After — 7 failures (SortReverseIgnored fixed; the other 7 are unrelated
pre-existing intentionally-red review findings; no new failures)
IsMemberStrCaseSensitiveBug, PcreMatchEmptySubject, CapitalizeDeprecatedTitle,
FileReadlinesBinaryMode, QueuedTasksSortOrder, VerbCodeAllowsOwnerWithoutReadBit,
AddVerbUsesProgNotPlayerForPerm.

## Commit
<filled after commit>
