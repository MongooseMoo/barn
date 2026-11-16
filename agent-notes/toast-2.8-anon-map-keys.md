# toast-2.8-anon-map-keys

## Goal

Match the upstream ToastStunt commits "Allow TYPE_ANON in mapdelete and
maphaskey" and the observable part of "Fix use-after-free in mapdelete()
multi-key error path" in Barn and the conformance suite.

## State

Done on branch `toast-2.8-anon-map-keys` in both repositories; not pushed.

- Toast (current stock oracle pin) `src/map.cc` `bf_mapdelete` (single and
  multi-key) and `bf_maphaskey` reject
  `key.is_collection() && TYPE_ANON != key.type`. `is_collection()` is LIST,
  MAP or ANON (`src/include/structures.h:191`), so lists and maps stay E_TYPE
  and anonymous objects are accepted.
- The use-after-free fix only makes the multi-key missing-key raise take its
  own reference to the key; the payload is still
  `{E_RANGE, "Key <unparse> not found in map", key}`. An anonymous key
  unparses as `*anonymous*` (`src/list.cc:508`).
- Barn: `builtins/maps.go` now checks `types.IsValidMapKey` (the removed
  `types.IsValidBuiltinMapKey` rejected ANON). The multi-key message unparses
  the key with `publicLiteral`, because `Value.String()` renders an anonymous
  object as `*#N`.
- Conformance: `language/map_authority.yaml` test renamed to
  `anon_keys_work_in_vm_mapvalues_and_key_builtins`, four new anon-key tests
  for maphaskey/mapdelete present/absent and the multi-key payload;
  `builtins/map.yaml::anonymous_object_key` now expects E_RANGE.

## Evidence

- Toast oracle, selector `anon_keys_work or anonymous_object_key or anon_key`:
  7 passed; 1 failed, `reverse_inserted_anon_key_maps_expose_reverse_topology`
  (map key ordering, owned by the map-key-order work, not this branch).
- Barn managed conformance, selector `anon_keys_work or anonymous_object_key or
  (anon_key and not reverse_inserted)`: 7 passed.
- `go test ./builtins ./types` pass; the two new builtins tests failed with
  E_TYPE before the fix.

## Next step

Publish both branches (conformance first).
