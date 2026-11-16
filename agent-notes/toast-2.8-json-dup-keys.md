# toast-2.8-json-dup-keys

## Goal

Match stock ToastStunt 2.8's `parse_json()` duplicate-key behaviour
(lisdude/toaststunt PR #104, "Reject duplicate object keys in parse_json"):
a JSON object with two members that map to the same MOO key fails with
E_INVARG instead of keeping the first value.

## Source read

- Toast `src/json.cc` `handle_end_map`: after each `mapinsert`, an unchanged
  `maplength` means the key repeated, and the parse fails (Toast commits
  "Reject duplicate object keys in parse_json" and "Check map size instead of
  looking up each JSON key").
- Toast `test/tests/test_json.rb`: the rejected and accepted cases mirrored in
  `builtins/json_test.go`.
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
  `parse_json_duplicate_keys_rejected` and added nested, case-only, `\u0041`
  and `\n` escape, embedded `|str` and `|obj` rejections, plus an accepted
  embedded document whose keys differ by type. All pass on the stock WSL
  Toast oracle.
- Barn `builtins/json.go`:
  - `decodeJSONValue` fails on an exact repeated member name.
  - `jsonToMOO` reports a collision when the built map is shorter than its
    pair list (case-only names, names equal after decoding, equal embedded
    values).
  - `tagJSONBinaryEscapes` tags `\uXXXX` below U+0080 as well as the short
    controls, so they decode to `~XX` as in Toast.
  - `encodeJSONDecodedBytes` no longer turns a raw `~` into `~7E`.
  - `parseEmbeddedType` parses `|obj` with Toast's strtol rule (`cStrtol`).
  - The unit test that expected `\u0009` to decode to a tab now expects
    `~09`, per `Utf32toUtf8`.

## Surprises

- `builtins/json_v2_migration_probe.yaml::duplicate_first_wins` named in the
  task is an untracked file in the main moo-conformance-tests checkout; it is
  not on origin/main, so the branch does not change it.
- The Edit tool decoded single-backslash printable `\u00XX` sequences in Go
  raw strings into the literal character. `json_test.go` spells them with the
  `jsonBackslash` constant.
- Barn's `|int`, `|float`, `|err` and `|bool` embedded parsing still differs
  from Toast's `valid_type` (Barn uses Sscanf and has no `|bool`); not
  changed here.

## Next step

Team lead publishes both branches.
