# Fix F30 notes — pcre_match empty subject

## KEY FINDING (finding is INVERTED)
- F30 claimed pcre_match("", ".*") should return a MATCH. FALSE.
- Toast SOURCE: bf_pcre_match match loop = `while (offset < subject_length)`
  (toaststunt/src/pcre_moo.cc:208). subject_length = memo_strlen(subject) = 0 for
  empty subject => loop never runs => returns initial `new_list(0)` = {}
  (pcre_moo.cc:188 init, :320 return). True for ANY pattern.
- Conformance corroborates: pcre_match_empty_subject `pcre_match("","foo")` expects
  value: [] (moo-conformance-tests/.../builtins/pcre.yaml:201-205).
- Go regexp DIVERGES: FindAllStringSubmatchIndex("", "(?i).*") = [[0 0]] (1 match).
  So REMOVING Barn's short-circuit would make Barn return a spurious match. DO NOT remove.

## Result shape (for the record, non-empty matches)
List of maps; each map keyed "0","1",... or named group -> {"position"->{start+1,end}, "match"->str}.
(pcre_moo.cc result_indices :350-359, mapinsert position/match :259/267/292/293)

## ACTION TAKEN
- Kept short-circuit in builtins/pcre.go; upgraded comment with Toast citation.
- Corrected red test TestReview_Data_PcreMatchEmptySubject to assert {} (Toast's true
  result) + added non-empty genuine non-match case ("foobar","baz" -> {}).

## STATUS
- go test ./builtins/ -run Pcre: PASS. go vet: clean.
- Full ./builtins/...: 1 unrelated PRE-EXISTING failure (IsMemberStrCaseSensitiveBug,
  a different finding, untouched code). My diff only touches pcre.go + pcre test.
