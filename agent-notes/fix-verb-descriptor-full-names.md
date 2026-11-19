# fix/verb-descriptor-full-names

## Goal
Let the verb builtins accept a verb's whole names string, as `verbs()` or
`verb_info()` returns it, as a descriptor, so `for v in (verbs(o)) verb_info(o, v)`
works for multi-name and starred verbs as on ToastStunt. Issue #888.

## State
Done. Each `db/store` `Verb` keeps one canonical `namesString`, built in
`NewVerb` and on rename, and `verbs()` (`VerbNames`) and `verb_info()` hand that
string out. The defined-verb lookups (`StoreTxn.findVerbOnObject` and the
locked `Store.findVerbOnObjectLocked` that the writers re-resolve through) match
a descriptor that is that same string. New candidate conformance file
`tests/mongoose-conformance/builtins/verb_descriptor_full_names.yaml`.

## Decisions and why
- Toast's `verbcasecmp` begins with `if (verb == word) return 1;`, a pointer
  check: the string from `verbs()` is the verb's own name string, while a typed
  literal with the same text is a different string and gives E_VERBNF. The
  packaged case `verbs.yaml::verb_lookup_rejects_literal_wildcard_name` pins the
  literal side.
- A first version matched the text exactly. It failed that packaged case, so
  the match is now by identity: same length and the same backing bytes
  (`unsafe.StringData`). Barn's string values keep the Go string they were
  built from, so the identity survives variables and lists.
- The check runs per verb, before the alias check, in definition order, as
  Toast's does.
- Only the defined-verb lookups changed. Verb dispatch and command matching
  keep the alias matcher.

## Surprises
- `set_verb_info` and the other writers find the verb through the transaction
  and then again through `findVerbOnObjectLocked`, a second copy of the scan.
  Both needed the check.

## Next step
Not checked: whether Toast's pointer check also lets `o:(verbs(o)[i])()` call a
multi-name verb, or whether `respond_to()` (which interns its names string)
should hand out the canonical string too.
