# toast-2.8-json-dup-keys

## Goal

Match stock ToastStunt 2.8's `parse_json()` duplicate-key behaviour
(lisdude/toaststunt PR #104, "Reject duplicate object keys in parse_json"):
a JSON object with two members that map to the same MOO key fails with
E_INVARG instead of keeping the first value. Barn compares keys as Barn
decodes them. Q also asked for Barn's JSON builtins to move to Go 1.27's
json/v2 ("we want json/v2 go 1.27"), with behaviour otherwise unchanged.

## Source read

- Toast `src/json.cc` `handle_end_map`: after each `mapinsert`, an unchanged
  `maplength` means the key repeated, and the parse fails (Toast commits
  "Reject duplicate object keys in parse_json" and "Check map size instead of
  looking up each JSON key").
- Toast `test/tests/test_json.rb`: the rejected and accepted cases mirrored in
  `builtins/json_test.go`, less the two noted below.
- Toast `src/dependencies/yajl/yajl_encode.c` `yajl_string_decode` /
  `Utf32toUtf8`: the short escapes b, f, n, r and every backslash-u escape
  below U+0080 decode to MOO binary escape text (the escape for "A" becomes
  the three characters `~41`), t to a tab; raw text, tildes included, is
  copied unchanged.
- Toast `valid_type` / `handle_string`: an `|obj` value is `strtol` of the
  text after an optional `#`, so `"5|obj"` is `#5`.

## State

- Conformance branch `toast-2.8-json-dup-keys`
  (`C:/Users/Q/code/moo-conformance-tests-wt/toast-2.8-json-dup-keys`):
  `audit/divergence_builtin_edges.yaml` renamed
  `parse_json_duplicate_keys_first_wins` to
  `parse_json_duplicate_keys_rejected` and added nested, case-only, escaped
  slash, embedded `|str` and `|obj` rejections, plus an accepted embedded
  document whose keys differ by type. All pass on the stock WSL Toast oracle
  and on Barn.
- Barn string decoding is unchanged. Barn strings are Unicode by Q's
  decision, which overrides Toast: the escape for "A" stays "A", the escape
  for tab stays a tab, and a raw `~` still decodes to `~7E`. The first commit
  on this branch copied Toast's byte-escape decoding; the second commit
  reverts that part.
- Barn `builtins/json.go` is on `encoding/json/jsontext` (Go 1.27.1; the
  json/v2 packages build without GOEXPERIMENT inside a `go 1.27` module, but
  not outside one: `go list` in a module-less directory reports "build
  constraints exclude all Go files"):
  - parse_json streams tokens from a `jsontext.Decoder`
    (`AllowInvalidUTF8(true)`, default `AllowDuplicateNames(false)`) straight
    into MOO values (`decodeJSONToMOO`). jsontext rejects an exact repeated
    member name (compared after unescaping); a map shorter than its pair list
    catches the rest (case-only names, equal embedded values).
  - generate_json writes tokens to a `jsontext.Encoder` with
    `EscapeForHTML(true)`, `EscapeForJS(true)`, `AllowInvalidUTF8(true)` and
    `AllowDuplicateNames(true)`. The first three reproduce v1 `json.Marshal`
    escaping byte for byte; the last keeps `[1 -> x, "1" -> y]` writing
    `{"1":x,"1":y}` (jsontext's default fails it with E_INVARG; unit test
    `TestGenerateJsonWritesRepeatedMemberNames`). The encoder's trailing
    newline is trimmed; `normalizeJSONEscapes` is unchanged.
  - Gone: the v1 preliminary decode, the escape marker pass (marker: the
    character U+E000 followed by `barn-json-control-`), the `interface{}`
    intermediate tree and `orderedMap`. The marker pass was output-neutral (a
    tagged newline escape gave `~0A`, the same as the decoded newline byte)
    except for one bug: a JSON string whose decoded text began with the marker
    decoded wrongly. A JSON string holding U+E000 then
    `barn-json-control-0-n~0A` gave `"~0A"`; it now gives
    `"~EE~80~80barn-json-control-0-n~7E0A"` (unit test "old escape marker
    text").
  - `parseEmbeddedType` parses `|obj` with Toast's strtol rule (`cStrtol`),
    so `"5|obj"` is `#5`.

## Verification of the json/v2 move

- Differential corpus (a temporary test, not committed): 537,102 lines of
  parse_json (fixed edge cases, 60,000 random fragment strings, 20,000 random
  documents, each in three modes) and generate_json (30,000 random MOO values,
  five option sets, each output parsed back) recorded on the pre-move code,
  then compared on the jsontext code: 0 differences apart from 2,583 lines
  whose input contains the old marker text (the bug above).
- `go test ./builtins` ok; `FuzzStringBuiltins` (covers parse_json and
  generate_json) for 90 s: 2,959,701 execs, no failure.
- Managed conformance in WSL (Barn built there with go1.27.1 linux/amd64, no
  GOEXPERIMENT): `make conformance-k CONFORMANCE_SUITE=<suite wt> K="json"`
  gave 178 passed, 13721 deselected.
- Benchmarks (`builtins/json_bench_test.go`, a 2.7 KB room-update document),
  pre-move code in a temporary worktree interleaved with the new code, 2 x 5
  runs each on the workstation at 13-36% CPU load:
  - BenchmarkGenerateJSON: about 202-232 us, 76.0 KB, 1487 allocs/op before;
    about 79-92 us, 30.5 KB, 408 allocs/op after.
  - BenchmarkParseJSON: about 246-288 us, 106.9 KB, 2444 allocs/op before;
    about 106-121 us, 65.4 KB, 1284 allocs/op after.

## Decisions and why

- Toast's test cases that pair `"~41"` with the escape for "A", and `"~0A"`
  with the newline escape, collide only because Toast decodes those escapes
  to byte escape text; in Barn they are distinct keys. The conformance suite
  has no feature, option or profile marker for Barn's Unicode string
  divergence (skip_if supports `feature.*`, `option.*` and
  `missing builtin.*`; no profile in `profiles/` names one), so those two
  cases are left out of the conformance commit rather than inventing a
  marker. The escaped-slash case covers an escape-decoded collision that does
  not depend on byte escapes.

## Surprises

- `builtins/json_v2_migration_probe.yaml::duplicate_first_wins` named in the
  task is an untracked file in the main moo-conformance-tests checkout; it is
  not on origin/main, so the branch does not change it.
- The Edit and Write tools turned backslash-u escapes in the text I sent into
  the literal characters in several places (Go raw strings, this note).
  Check written bytes with `grep`/`cat -A` after writing such text.
- Barn's `|int`, `|float`, `|err` and `|bool` embedded parsing still differs
  from Toast's `valid_type` (Barn uses Sscanf and has no `|bool`); not
  changed here.

## Next step

Team lead publishes both branches.
