# toast-2.8-json-dup-keys

## Goal

Match stock ToastStunt 2.8's `parse_json()` duplicate-key behaviour
(lisdude/toaststunt PR #104, "Reject duplicate object keys in parse_json"):
a JSON object with two members that map to the same MOO key fails with
E_INVARG instead of keeping the first value. Barn compares keys as Barn
decodes them.

## Source read

- Toast `src/json.cc` `handle_end_map`: after each `mapinsert`, an unchanged
  `maplength` means the key repeated, and the parse fails (Toast commits
  "Reject duplicate object keys in parse_json" and "Check map size instead of
  looking up each JSON key").
- Toast `test/tests/test_json.rb`: the rejected and accepted cases mirrored in
  `builtins/json_test.go`, less the two noted below.
- Toast `src/dependencies/yajl/yajl_encode.c` `yajl_string_decode` /
  `Utf32toUtf8`: `\b \f \n \r` and every `\uXXXX` below U+0080 decode to MOO
  binary escape text (`\u0041` is the three characters `~41`), `\t` to a tab;
  raw text, tildes included, is copied unchanged.
- Toast `valid_type` / `handle_string`: an `|obj` value is `strtol` of the
  text after an optional `#`, so `"5|obj"` is `#5`.

## State

- Conformance branch `toast-2.8-json-dup-keys`
  (`C:/Users/Q/code/moo-conformance-tests-wt/toast-2.8-json-dup-keys`):
  `audit/divergence_builtin_edges.yaml` renamed
  `parse_json_duplicate_keys_first_wins` to
  `parse_json_duplicate_keys_rejected` and added nested, case-only, `\/`
  escape, embedded `|str` and `|obj` rejections, plus an accepted embedded
  document whose keys differ by type. All pass on the stock WSL Toast oracle
  and on Barn.
- Barn `builtins/json.go`:
  - `decodeJSONValue` fails on an exact repeated member name.
  - `jsonToMOO` reports a collision when the built map is shorter than its
    pair list (case-only names, names equal after Barn's decoding, equal
    embedded values).
  - `parseEmbeddedType` parses `|obj` with Toast's strtol rule (`cStrtol`),
    so `"5|obj"` is `#5`.
- Barn string decoding is unchanged. Barn strings are Unicode by Q's
  decision, which overrides Toast: `\u0041` stays "A", `\u0009` stays a tab,
  and a raw `~` still decodes to `~7E`. The first commit on this branch copied
  Toast's byte-escape decoding; the second commit reverts that part.

## Decisions and why

- Toast's `{"~41":1,"\u0041":2}` and `{"~0A":1,"\n":2}` collide only because
  Toast decodes those escapes to byte escape text; in Barn they are distinct
  keys. The conformance suite has no feature, option or profile marker for
  Barn's Unicode string divergence (skip_if supports `feature.*`, `option.*`
  and `missing builtin.*`; no profile in `profiles/` names one), so those two
  cases are left out of the conformance commit rather than inventing a
  marker. `{"/":1,"\/":2}` covers an escape-decoded collision that does not
  depend on byte escapes.

## Surprises

- `builtins/json_v2_migration_probe.yaml::duplicate_first_wins` named in the
  task is an untracked file in the main moo-conformance-tests checkout; it is
  not on origin/main, so the branch does not change it.
- The Edit tool decoded single-backslash printable `\u00XX` sequences in Go
  raw strings into the literal character.
- Barn's `|int`, `|float`, `|err` and `|bool` embedded parsing still differs
  from Toast's `valid_type` (Barn uses Sscanf and has no `|bool`); not
  changed here.

## Next step

Team lead publishes both branches.
