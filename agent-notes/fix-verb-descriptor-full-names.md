# fix/verb-descriptor-full-names

## Goal
Let the verb builtins accept a verb's whole names string, as `verbs()` returns
it, as a descriptor, so `for v in (verbs(o)) verb_info(o, v)` works for
multi-name and starred verbs as on ToastStunt. Issue #888.

## State
Done. `db/store/store_txn_verbs.go` `findVerbOnObject`, the defined-verb lookup
behind `verb_info`, `verb_args`, `verb_code`, `set_verb_info`, `set_verb_code`,
`delete_verb` and the other descriptor builtins, now matches a verb whose names
joined by spaces equal the descriptor exactly. New candidate conformance file
`tests/mongoose-conformance/builtins/verb_descriptor_full_names.yaml`.

## Decisions and why
- Toast matches through a pointer check in `verbcasecmp`
  (`if (verb == word) return 1;`): `verbs()` returns the verb's own name
  string. Barn strings have no identity, so an exact, case-sensitive match on
  the whole names string is the nearest equivalent.
- This is a superset of Toast: a typed literal with the same text also matches
  on Barn, where Toast gives E_VERBNF. The conformance file pins only the
  Toast-visible part (values from `verbs()`, aliases, partial strings still
  E_VERBNF) and does not test typed full literals.
- The check runs per verb, before the alias check, in definition order, as
  Toast's does.
- Only the defined-verb lookup changed. Verb dispatch and command matching
  keep the alias matcher. Not checked: whether Toast's pointer check also lets
  `o:(verbs(o)[i])()` call a multi-name verb.

## Surprises
None.

## Next step
Check `o:(verbs(o)[i])()` on Toast. If it calls the verb there, the same check
belongs in the callable-verb lookup.
